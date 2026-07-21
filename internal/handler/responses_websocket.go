package handler

import (
	"bytes"
	"context"
	stdjson "encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	maxxctx "github.com/awsl-project/maxx/internal/context"
	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/flow"
	"github.com/awsl-project/maxx/internal/jsonutil"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const (
	responsesWebSocketRequestCreate = "response.create"
	responsesWSMaxPendingFrames     = 64
	responsesWSMaxPendingBytes      = 64 << 20
	responsesWSWriteTimeout         = 30 * time.Second
)

var responsesWebSocketUpgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	CheckOrigin:     checkResponsesWebSocketOrigin,
}

type responsesWebSocketClient struct {
	conn      *websocket.Conn
	writeMu   sync.Mutex
	closeOnce sync.Once
	done      chan struct{}
}

func newResponsesWebSocketClient(conn *websocket.Conn) *responsesWebSocketClient {
	return &responsesWebSocketClient{conn: conn, done: make(chan struct{})}
}

func (c *responsesWebSocketClient) WriteTextFrame(payload []byte) error {
	if c == nil || c.conn == nil {
		return errors.New("downstream websocket is unavailable")
	}
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if err := c.conn.SetWriteDeadline(time.Now().Add(responsesWSWriteTimeout)); err != nil {
		return err
	}
	return c.conn.WriteMessage(websocket.TextMessage, payload)
}

func (c *responsesWebSocketClient) writeControl(messageType int, payload []byte) error {
	if c == nil || c.conn == nil {
		return errors.New("downstream websocket is unavailable")
	}
	return c.conn.WriteControl(messageType, payload, time.Now().Add(10*time.Second))
}

func (c *responsesWebSocketClient) close(code int, reason string) {
	if c == nil {
		return
	}
	c.closeOnce.Do(func() {
		close(c.done)
		if c.conn != nil {
			_ = c.conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(code, reason), time.Now().Add(10*time.Second))
			_ = c.conn.Close()
		}
	})
}

type responsesWebSocketInboundFrame struct {
	payload []byte
	size    int64
}

func isResponsesWebSocketUpgrade(r *http.Request) bool {
	if r == nil || r.URL == nil || r.Method != http.MethodGet || !websocket.IsWebSocketUpgrade(r) {
		return false
	}
	return r.URL.Path == "/v1/responses" || r.URL.Path == "/responses"
}

func checkResponsesWebSocketOrigin(r *http.Request) bool {
	if r == nil {
		return false
	}
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return true
	}
	parsed, err := url.Parse(origin)
	if err == nil && strings.EqualFold(parsed.Host, r.Host) {
		return true
	}
	return ParseCORSOrigins(os.Getenv("MAXX_CORS_ALLOW_ORIGINS")).allows(strings.TrimRight(origin, "/"))
}

func (c *responsesWebSocketClient) readPump(
	ctx context.Context,
	cancel context.CancelFunc,
	frames chan<- responsesWebSocketInboundFrame,
	queuedBytes *atomic.Int64,
	readLimit int64,
) {
	defer close(frames)
	defer cancel()
	c.conn.SetReadLimit(responsesWebSocketReadLimit(readLimit))
	c.conn.SetPingHandler(func(data string) error {
		return c.writeControl(websocket.PongMessage, []byte(data))
	})
	for {
		messageType, payload, err := c.conn.ReadMessage()
		if err != nil {
			return
		}
		if messageType != websocket.TextMessage {
			_ = writeResponsesWebSocketClientError(c, http.StatusBadRequest, "binary websocket frames are not supported", nil)
			c.close(websocket.CloseUnsupportedData, "text frames required")
			return
		}
		size := int64(len(payload))
		if queuedBytes.Add(size) > responsesWSMaxPendingBytes {
			queuedBytes.Add(-size)
			_ = writeResponsesWebSocketClientError(c, http.StatusTooManyRequests, "websocket request queue byte limit exceeded", nil)
			c.close(websocket.ClosePolicyViolation, "request queue limit exceeded")
			return
		}
		select {
		case frames <- responsesWebSocketInboundFrame{payload: payload, size: size}:
		case <-ctx.Done():
			queuedBytes.Add(-size)
			return
		default:
			queuedBytes.Add(-size)
			_ = writeResponsesWebSocketClientError(c, http.StatusTooManyRequests, "websocket request queue is full", nil)
			c.close(websocket.ClosePolicyViolation, "request queue is full")
			return
		}
	}
}

func responsesWebSocketReadLimit(configured int64) int64 {
	if configured <= 0 || configured > responsesWSMaxPendingBytes {
		return responsesWSMaxPendingBytes
	}
	return configured
}

func (h *ProxyHandler) serveResponsesWebSocket(w http.ResponseWriter, r *http.Request, readLimit int64) {
	if h == nil || h.engine == nil || h.executor == nil {
		writeError(w, http.StatusInternalServerError, "responses websocket proxy is not configured")
		return
	}
	conn, err := responsesWebSocketUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	client := newResponsesWebSocketClient(conn)
	connectionID := uuid.NewString()
	connectionCtx, cancel := context.WithCancel(r.Context())
	frames := make(chan responsesWebSocketInboundFrame, responsesWSMaxPendingFrames)
	var queuedBytes atomic.Int64
	go client.readPump(connectionCtx, cancel, frames, &queuedBytes, readLimit)
	defer func() {
		cancel()
		h.executor.CloseResponsesWebSocketConnection(connectionID)
		client.close(websocket.CloseNormalClosure, "")
	}()

	exchange := &domain.ResponsesWebSocketExchange{
		ConnectionID: connectionID,
		Sink:         client,
	}
	for {
		select {
		case <-connectionCtx.Done():
			return
		case frame, ok := <-frames:
			if !ok {
				return
			}
			queuedBytes.Add(-frame.size)
			previousResponseID, errValidate := validateResponsesWebSocketFrame(frame.payload)
			if errValidate != nil {
				if writeErr := writeResponsesWebSocketClientError(client, http.StatusBadRequest, errValidate.Error(), nil); writeErr != nil {
					return
				}
				continue
			}
			logicalBody, errBody := responsesWebSocketLogicalBody(frame.payload)
			if errBody != nil {
				if writeErr := writeResponsesWebSocketClientError(client, http.StatusBadRequest, errBody.Error(), nil); writeErr != nil {
					return
				}
				continue
			}
			exchange.Frame = frame.payload
			exchange.PreviousResponseID = previousResponseID
			flowCtx, writer := h.runResponsesWebSocketTurn(r, connectionCtx, logicalBody, exchange)
			if flowCtx.Err == nil {
				continue
			}
			if responsesWebSocketErrorAlreadySent(flowCtx.Err) {
				continue
			}
			status, message, errorBody := responsesWebSocketTurnError(flowCtx.Err, writer)
			if writeErr := writeResponsesWebSocketClientError(client, status, message, errorBody); writeErr != nil {
				return
			}
			// No WS-capable providers left: close so the next handshake can return
			// 426 and Codex falls back to HTTP/SSE instead of spinning reconnects.
			if responsesWebSocketShouldForceReconnectForHTTPFallback(flowCtx.Err) {
				client.close(websocket.CloseTryAgainLater, "websocket not supported; reconnect for HTTP fallback")
				return
			}
			if responsesWebSocketTurnCommitted(flowCtx.Err) {
				client.close(websocket.CloseInternalServerErr, "upstream websocket turn failed")
				return
			}
		}
	}
}

func validateResponsesWebSocketFrame(payload []byte) (string, error) {
	if !gjson.ValidBytes(payload) || !gjson.ParseBytes(payload).IsObject() {
		return "", fmt.Errorf("invalid websocket request JSON: %w", domain.ErrResponsesWebSocketProtocol)
	}
	if gjson.GetBytes(payload, "type").String() != responsesWebSocketRequestCreate {
		return "", fmt.Errorf("unsupported websocket request type: only response.create is accepted")
	}
	model := gjson.GetBytes(payload, "model")
	if model.Type != gjson.String || strings.TrimSpace(model.String()) == "" {
		return "", fmt.Errorf("missing model in response.create request")
	}
	if input := gjson.GetBytes(payload, "input"); input.Exists() && !input.IsArray() {
		return "", fmt.Errorf("websocket request requires array field: input")
	}
	previous := gjson.GetBytes(payload, "previous_response_id")
	if previous.Exists() && previous.Type != gjson.String {
		return "", fmt.Errorf("previous_response_id must be a string")
	}
	return strings.TrimSpace(previous.String()), nil
}

func responsesWebSocketLogicalBody(payload []byte) ([]byte, error) {
	if !gjson.ValidBytes(payload) || !gjson.ParseBytes(payload).IsObject() {
		return nil, domain.ErrResponsesWebSocketProtocol
	}
	return sjson.DeleteBytes(bytes.Clone(payload), "type")
}

func newResponsesWebSocketTurnRequest(
	original *http.Request,
	connectionCtx context.Context,
	logicalBody []byte,
	connectionID string,
	rawFrame []byte,
) *http.Request {
	ctx := maxxctx.WithResponsesWebSocketRequest(connectionCtx, connectionID, rawFrame)
	ctx = maxxctx.WithRequestBody(ctx, logicalBody)
	request := original.Clone(ctx)
	request.Method = http.MethodPost
	request.Body = io.NopCloser(bytes.NewReader(logicalBody))
	request.ContentLength = int64(len(logicalBody))
	request.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(logicalBody)), nil
	}
	request.Header = original.Header.Clone()
	for key := range request.Header {
		lower := strings.ToLower(key)
		if lower == "connection" || lower == "upgrade" || lower == "host" ||
			lower == "content-length" || lower == "transfer-encoding" || strings.HasPrefix(lower, "sec-websocket-") {
			request.Header.Del(key)
		}
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	return request
}

type responsesWebSocketTurnWriter struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func newResponsesWebSocketTurnWriter() *responsesWebSocketTurnWriter {
	return &responsesWebSocketTurnWriter{header: make(http.Header), status: http.StatusOK}
}

func (w *responsesWebSocketTurnWriter) Header() http.Header { return w.header }

func (w *responsesWebSocketTurnWriter) WriteHeader(status int) {
	if w.status == http.StatusOK {
		w.status = status
	}
}

func (w *responsesWebSocketTurnWriter) Write(payload []byte) (int, error) {
	return w.body.Write(payload)
}

func (w *responsesWebSocketTurnWriter) Flush() {}

func (h *ProxyHandler) runResponsesWebSocketTurn(
	original *http.Request,
	connectionCtx context.Context,
	logicalBody []byte,
	exchange *domain.ResponsesWebSocketExchange,
) (*flow.Ctx, *responsesWebSocketTurnWriter) {
	writer := newResponsesWebSocketTurnWriter()
	request := newResponsesWebSocketTurnRequest(
		original,
		connectionCtx,
		logicalBody,
		exchange.ConnectionID,
		exchange.Frame,
	)
	c := flow.NewCtx(writer, request)
	c.Set(flow.KeyResponsesWebSocketExchange, exchange)
	h.engine.HandleWith(c, h.proxyHandlers()...)
	return c, writer
}

func responsesWebSocketErrorAlreadySent(err error) bool {
	var wsErr *domain.ResponsesWebSocketAttemptError
	return errors.As(err, &wsErr) && wsErr.TerminalErrorEventSent
}

func responsesWebSocketTurnCommitted(err error) bool {
	var wsErr *domain.ResponsesWebSocketAttemptError
	return errors.As(err, &wsErr) &&
		(wsErr.RequestFrameMayHaveBeenSent || wsErr.FirstEventReceived || wsErr.ClientEventSent)
}

func responsesWebSocketTurnError(err error, writer *responsesWebSocketTurnWriter) (int, string, stdjson.RawMessage) {
	status := http.StatusInternalServerError
	message := err.Error()
	var errorBody stdjson.RawMessage
	if proxyErr, ok := asHandlerProxyError(err); ok {
		if proxyErr.HTTPStatusCode >= 400 && proxyErr.HTTPStatusCode <= 599 {
			status = proxyErr.HTTPStatusCode
		}
		if strings.TrimSpace(proxyErr.Message) != "" {
			message = proxyErr.Message
		}
		detail := map[string]any{
			"type":      "proxy_error",
			"message":   message,
			"retryable": proxyErr.Retryable,
		}
		if proxyErr.Code != "" {
			detail["code"] = proxyErr.Code
		}
		// Hint Codex clients to use HTTP/SSE when no WS-capable provider exists.
		if proxyErr.Code == "websocket_not_supported" || proxyErr.Code == "websocket_transport_unavailable" {
			detail["fallback"] = "http_sse"
		}
		if encoded, marshalErr := jsonutil.Marshal(detail); marshalErr == nil {
			errorBody = encoded
		}
	}
	if writer != nil {
		if writer.status >= 400 && writer.status <= 599 {
			status = writer.status
		}
		if body := bytes.TrimSpace(writer.body.Bytes()); len(body) > 0 && stdjson.Valid(body) {
			var object map[string]stdjson.RawMessage
			if jsonutil.Unmarshal(body, &object) == nil {
				if rawError := object["error"]; len(rawError) > 0 {
					if got := gjson.GetBytes(rawError, "message").String(); got != "" {
						message = got
					}
					return status, message, rawError
				}
			}
		}
	}
	return status, message, errorBody
}

func responsesWebSocketShouldForceReconnectForHTTPFallback(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, domain.ErrNoResponsesWebSocketProviders) {
		return true
	}
	if proxyErr, ok := asHandlerProxyError(err); ok {
		switch proxyErr.Code {
		case "websocket_not_supported", "websocket_transport_unavailable":
			return true
		}
	}
	return false
}

func writeResponsesWebSocketClientError(client *responsesWebSocketClient, status int, message string, errorBody stdjson.RawMessage) error {
	if status < http.StatusBadRequest || status > 599 {
		status = http.StatusInternalServerError
	}
	if strings.TrimSpace(message) == "" {
		message = http.StatusText(status)
	}
	var detail any = map[string]any{
		"type":    "proxy_error",
		"message": message,
	}
	if len(bytes.TrimSpace(errorBody)) > 0 && stdjson.Valid(errorBody) {
		detail = stdjson.RawMessage(errorBody)
	}
	payload, err := jsonutil.Marshal(map[string]any{
		"type":   "error",
		"status": status,
		"error":  detail,
	})
	if err != nil {
		return err
	}
	return client.WriteTextFrame(payload)
}
