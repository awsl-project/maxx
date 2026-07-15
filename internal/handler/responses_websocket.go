package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"

	maxxctx "github.com/awsl-project/maxx/internal/context"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

const (
	responsesWebSocketRequestCreate = "response.create"
	responsesWebSocketRequestAppend = "response.append"
)

var responsesWebSocketUpgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	CheckOrigin: func(*http.Request) bool {
		return true
	},
}

type responsesWebSocketSessionState struct {
	lastRequest        []byte
	lastResponseOutput []json.RawMessage
}

func isResponsesWebSocketUpgrade(r *http.Request) bool {
	if r == nil || r.URL == nil || r.Method != http.MethodGet || !websocket.IsWebSocketUpgrade(r) {
		return false
	}
	return r.URL.Path == "/v1/responses" || r.URL.Path == "/responses"
}

func serveResponsesWebSocket(w http.ResponseWriter, r *http.Request, next http.Handler, readLimit int64) {
	if next == nil {
		writeError(w, http.StatusInternalServerError, "responses websocket proxy is not configured")
		return
	}

	conn, err := responsesWebSocketUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	if readLimit > 0 {
		conn.SetReadLimit(readLimit)
	}

	state := &responsesWebSocketSessionState{}
	sessionID := uuid.NewString()
	for {
		messageType, payload, errRead := conn.ReadMessage()
		if errRead != nil {
			return
		}
		if messageType != websocket.TextMessage && messageType != websocket.BinaryMessage {
			continue
		}

		requestBody, errNormalize := state.normalizeRequest(payload)
		if errNormalize != nil {
			if errWrite := writeResponsesWebSocketError(conn, http.StatusBadRequest, errNormalize.Error(), nil); errWrite != nil {
				return
			}
			continue
		}

		request := newResponsesWebSocketHTTPRequest(r, requestBody, sessionID, payload)
		writer := newResponsesWebSocketResponseWriter(conn)
		next.ServeHTTP(writer, request)
		if errFinish := writer.finish(); errFinish != nil {
			return
		}
		if writer.completed {
			state.lastRequest = bytes.Clone(requestBody)
			state.lastResponseOutput = cloneRawMessages(writer.completedOutput)
		}
	}
}

func newResponsesWebSocketHTTPRequest(original *http.Request, body []byte, sessionID string, payload []byte) *http.Request {
	ctx := maxxctx.WithResponsesWebSocketRequest(original.Context(), sessionID, payload)
	request := original.Clone(ctx)
	request.Method = http.MethodPost
	request.Body = http.NoBody
	if len(body) > 0 {
		request.Body = io.NopCloser(bytes.NewReader(body))
	}
	request.ContentLength = int64(len(body))
	request.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(body)), nil
	}
	request.Header = original.Header.Clone()
	for _, key := range []string{
		"Connection",
		"Upgrade",
		"Sec-Websocket-Key",
		"Sec-Websocket-Version",
		"Sec-Websocket-Extensions",
		"Sec-Websocket-Protocol",
		"Content-Length",
	} {
		request.Header.Del(key)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "text/event-stream")
	return request
}

func (s *responsesWebSocketSessionState) normalizeRequest(payload []byte) ([]byte, error) {
	request, err := decodeJSONObject(payload)
	if err != nil {
		return nil, fmt.Errorf("invalid websocket request JSON: %w", err)
	}

	requestType := jsonString(request["type"])
	switch requestType {
	case responsesWebSocketRequestCreate:
		if len(s.lastRequest) == 0 {
			return normalizeInitialResponsesWebSocketRequest(request)
		}
		return s.normalizeSubsequentResponsesWebSocketRequest(request)
	case responsesWebSocketRequestAppend:
		if len(s.lastRequest) == 0 {
			return nil, fmt.Errorf("websocket request received before response.create")
		}
		return s.normalizeSubsequentResponsesWebSocketRequest(request)
	default:
		return nil, fmt.Errorf("unsupported websocket request type: %s", requestType)
	}
}

func normalizeInitialResponsesWebSocketRequest(request map[string]json.RawMessage) ([]byte, error) {
	delete(request, "type")
	delete(request, "generate")
	request["stream"] = json.RawMessage("true")
	if _, ok := request["input"]; !ok {
		request["input"] = json.RawMessage("[]")
	}
	if strings.TrimSpace(jsonString(request["model"])) == "" {
		return nil, fmt.Errorf("missing model in response.create request")
	}
	normalized, err := json.Marshal(request)
	return normalized, err
}

func (s *responsesWebSocketSessionState) normalizeSubsequentResponsesWebSocketRequest(request map[string]json.RawMessage) ([]byte, error) {
	nextInput, err := decodeJSONArray(request["input"])
	if err != nil {
		return nil, fmt.Errorf("websocket request requires array field: input")
	}

	lastRequest, err := decodeJSONObject(s.lastRequest)
	if err != nil {
		return nil, fmt.Errorf("invalid previous websocket request state: %w", err)
	}

	previousResponseID := strings.TrimSpace(jsonString(request["previous_response_id"]))
	var mergedInput []json.RawMessage
	if previousResponseID == "" && responsesWebSocketInputReplacesTranscript(nextInput) {
		mergedInput = cloneRawMessages(nextInput)
	} else {
		lastInput, errLastInput := decodeJSONArray(lastRequest["input"])
		if errLastInput != nil {
			return nil, fmt.Errorf("invalid previous websocket input: %w", errLastInput)
		}
		mergedInput = append(mergedInput, cloneRawMessages(lastInput)...)
		mergedInput = append(mergedInput, cloneRawMessages(s.lastResponseOutput)...)
		mergedInput = append(mergedInput, cloneRawMessages(nextInput)...)
		mergedInput = dedupeResponsesWebSocketItems(mergedInput)
	}

	delete(request, "type")
	delete(request, "generate")
	delete(request, "previous_response_id")
	request["stream"] = json.RawMessage("true")
	request["input"], err = json.Marshal(mergedInput)
	if err != nil {
		return nil, fmt.Errorf("failed to merge websocket input: %w", err)
	}
	if _, ok := request["model"]; !ok {
		request["model"] = bytes.Clone(lastRequest["model"])
	}
	if _, ok := request["instructions"]; !ok {
		if instructions, exists := lastRequest["instructions"]; exists {
			request["instructions"] = bytes.Clone(instructions)
		}
	}

	normalized, err := json.Marshal(request)
	return normalized, err
}

func responsesWebSocketInputReplacesTranscript(input []json.RawMessage) bool {
	for _, item := range input {
		object, err := decodeJSONObject(item)
		if err != nil {
			continue
		}
		switch jsonString(object["type"]) {
		case "compaction", "compaction_summary", "function_call", "custom_tool_call":
			return true
		case "message":
			if jsonString(object["role"]) == "assistant" {
				return true
			}
		}
	}
	return false
}

func dedupeResponsesWebSocketItems(items []json.RawMessage) []json.RawMessage {
	seenItemIDs := make(map[string]struct{}, len(items))
	seenCallIDs := make(map[string]struct{}, len(items))
	result := make([]json.RawMessage, 0, len(items))
	for _, item := range items {
		object, err := decodeJSONObject(item)
		if err == nil {
			if id := strings.TrimSpace(jsonString(object["id"])); id != "" {
				if _, exists := seenItemIDs[id]; exists {
					continue
				}
				seenItemIDs[id] = struct{}{}
			}
			switch jsonString(object["type"]) {
			case "function_call", "custom_tool_call":
				if callID := strings.TrimSpace(jsonString(object["call_id"])); callID != "" {
					if _, exists := seenCallIDs[callID]; exists {
						continue
					}
					seenCallIDs[callID] = struct{}{}
				}
			}
		}
		result = append(result, bytes.Clone(item))
	}
	return result
}

func newResponsesWebSocketResponseWriter(conn *websocket.Conn) *responsesWebSocketResponseWriter {
	return &responsesWebSocketResponseWriter{
		conn:               conn,
		header:             make(http.Header),
		statusCode:         http.StatusOK,
		outputItemsByIndex: make(map[int]json.RawMessage),
	}
}

type responsesWebSocketResponseWriter struct {
	conn        *websocket.Conn
	header      http.Header
	statusCode  int
	wroteHeader bool

	modeSSE    bool
	modeChosen bool
	lineBuffer []byte
	eventData  bytes.Buffer
	rawBody    bytes.Buffer

	outputItemsByIndex  map[int]json.RawMessage
	outputItemsFallback []json.RawMessage
	completed           bool
	completedOutput     []json.RawMessage
	sentError           bool
	writeErr            error
}

func (w *responsesWebSocketResponseWriter) Header() http.Header {
	return w.header
}

func (w *responsesWebSocketResponseWriter) WriteHeader(statusCode int) {
	if w.wroteHeader {
		return
	}
	w.statusCode = statusCode
	w.wroteHeader = true
}

func (w *responsesWebSocketResponseWriter) Write(p []byte) (int, error) {
	if w.writeErr != nil {
		return 0, w.writeErr
	}
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	if !w.modeChosen {
		contentType := strings.ToLower(w.header.Get("Content-Type"))
		trimmed := bytes.TrimSpace(p)
		w.modeSSE = strings.Contains(contentType, "text/event-stream") || bytes.HasPrefix(trimmed, []byte("data:"))
		w.modeChosen = true
	}
	if !w.modeSSE {
		_, _ = w.rawBody.Write(p)
		return len(p), nil
	}

	w.lineBuffer = append(w.lineBuffer, p...)
	w.processSSELines(false)
	if w.writeErr != nil {
		return 0, w.writeErr
	}
	return len(p), nil
}

func (w *responsesWebSocketResponseWriter) Flush() {
	if w.modeSSE {
		w.processSSELines(false)
	}
}

func (w *responsesWebSocketResponseWriter) finish() error {
	if w.writeErr != nil {
		return w.writeErr
	}
	if w.modeSSE {
		w.processSSELines(true)
	} else if w.rawBody.Len() > 0 {
		w.forwardRawResponse()
	}
	if w.writeErr != nil {
		return w.writeErr
	}
	if !w.completed && !w.sentError {
		w.writeErr = writeResponsesWebSocketError(w.conn, http.StatusBadGateway, "stream closed before response.completed", nil)
	}
	return w.writeErr
}

func (w *responsesWebSocketResponseWriter) processSSELines(final bool) {
	for {
		index := bytes.IndexByte(w.lineBuffer, '\n')
		if index < 0 {
			break
		}
		line := bytes.TrimSuffix(w.lineBuffer[:index], []byte("\r"))
		w.lineBuffer = w.lineBuffer[index+1:]
		w.processSSELine(line)
		if w.writeErr != nil {
			return
		}
	}
	if final {
		if len(w.lineBuffer) > 0 {
			w.processSSELine(bytes.TrimSuffix(w.lineBuffer, []byte("\r")))
			w.lineBuffer = nil
		}
		w.flushSSEEvent()
	}
}

func (w *responsesWebSocketResponseWriter) processSSELine(line []byte) {
	if len(line) == 0 {
		w.flushSSEEvent()
		return
	}
	if !bytes.HasPrefix(line, []byte("data:")) {
		return
	}
	data := bytes.TrimSpace(line[len("data:"):])
	if w.eventData.Len() > 0 {
		w.eventData.WriteByte('\n')
	}
	w.eventData.Write(data)
}

func (w *responsesWebSocketResponseWriter) flushSSEEvent() {
	if w.eventData.Len() == 0 || w.writeErr != nil {
		w.eventData.Reset()
		return
	}
	payload := bytes.Clone(w.eventData.Bytes())
	w.eventData.Reset()
	if bytes.Equal(bytes.TrimSpace(payload), []byte("[DONE]")) {
		return
	}
	w.forwardPayload(payload)
}

func (w *responsesWebSocketResponseWriter) forwardPayload(payload []byte) {
	object, err := decodeJSONObject(payload)
	if err != nil {
		w.writeErr = writeResponsesWebSocketError(w.conn, http.StatusBadGateway, "invalid JSON event from upstream", nil)
		w.sentError = true
		return
	}

	eventType := jsonString(object["type"])
	if eventType == "response.output_item.done" {
		if item, ok := object["item"]; ok && len(item) > 0 {
			if outputIndex, okIndex := jsonInt(object["output_index"]); okIndex {
				w.outputItemsByIndex[outputIndex] = bytes.Clone(item)
			} else {
				w.outputItemsFallback = append(w.outputItemsFallback, bytes.Clone(item))
			}
		}
	}

	if eventType == "response.completed" || eventType == "response.done" {
		response, errResponse := decodeJSONObject(object["response"])
		if errResponse == nil {
			output, errOutput := decodeJSONArray(response["output"])
			if errOutput != nil || len(output) == 0 {
				output = w.collectedOutput()
				if encodedOutput, errMarshal := json.Marshal(output); errMarshal == nil {
					response["output"] = encodedOutput
					if encodedResponse, errMarshalResponse := json.Marshal(response); errMarshalResponse == nil {
						object["response"] = encodedResponse
					}
				}
			}
			w.completedOutput = cloneRawMessages(output)
		}
		w.completed = true
	}
	if eventType == "error" {
		w.sentError = true
	}

	encoded, errMarshal := json.Marshal(object)
	if errMarshal != nil {
		w.writeErr = errMarshal
		return
	}
	w.writeErr = w.conn.WriteMessage(websocket.TextMessage, encoded)
}

func (w *responsesWebSocketResponseWriter) collectedOutput() []json.RawMessage {
	indexes := make([]int, 0, len(w.outputItemsByIndex))
	for index := range w.outputItemsByIndex {
		indexes = append(indexes, index)
	}
	sort.Ints(indexes)
	result := make([]json.RawMessage, 0, len(indexes)+len(w.outputItemsFallback))
	for _, index := range indexes {
		result = append(result, bytes.Clone(w.outputItemsByIndex[index]))
	}
	result = append(result, cloneRawMessages(w.outputItemsFallback)...)
	return result
}

func (w *responsesWebSocketResponseWriter) forwardRawResponse() {
	payload := bytes.TrimSpace(w.rawBody.Bytes())
	if len(payload) == 0 {
		return
	}
	object, err := decodeJSONObject(payload)
	if err != nil {
		w.writeErr = writeResponsesWebSocketError(w.conn, w.statusCode, string(payload), nil)
		w.sentError = true
		return
	}
	if _, ok := object["type"]; ok {
		w.forwardPayload(payload)
		return
	}
	if errorBody, ok := object["error"]; ok || w.statusCode >= http.StatusBadRequest {
		w.writeErr = writeResponsesWebSocketError(w.conn, w.statusCode, http.StatusText(w.statusCode), errorBody)
		w.sentError = true
		return
	}

	completion := map[string]any{
		"type":            "response.completed",
		"sequence_number": 0,
		"response":        json.RawMessage(payload),
	}
	encoded, errMarshal := json.Marshal(completion)
	if errMarshal != nil {
		w.writeErr = errMarshal
		return
	}
	w.forwardPayload(encoded)
}

func writeResponsesWebSocketError(conn *websocket.Conn, status int, message string, errorBody json.RawMessage) error {
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
	if len(bytes.TrimSpace(errorBody)) > 0 && json.Valid(errorBody) {
		detail = json.RawMessage(errorBody)
	}
	payload, err := json.Marshal(map[string]any{
		"type":   "error",
		"status": status,
		"error":  detail,
	})
	if err != nil {
		return err
	}
	return conn.WriteMessage(websocket.TextMessage, payload)
}

func decodeJSONObject(raw []byte) (map[string]json.RawMessage, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, fmt.Errorf("empty JSON object")
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil {
		return nil, err
	}
	if object == nil {
		return nil, fmt.Errorf("expected JSON object")
	}
	return object, nil
}

func decodeJSONArray(raw []byte) ([]json.RawMessage, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, fmt.Errorf("missing JSON array")
	}
	var array []json.RawMessage
	if err := json.Unmarshal(raw, &array); err != nil {
		return nil, err
	}
	if array == nil {
		array = []json.RawMessage{}
	}
	return array, nil
}

func jsonString(raw json.RawMessage) string {
	var value string
	if json.Unmarshal(raw, &value) != nil {
		return ""
	}
	return value
}

func jsonInt(raw json.RawMessage) (int, bool) {
	var value int
	if json.Unmarshal(raw, &value) != nil {
		return 0, false
	}
	return value, true
}

func cloneRawMessages(messages []json.RawMessage) []json.RawMessage {
	result := make([]json.RawMessage, len(messages))
	for index := range messages {
		result[index] = bytes.Clone(messages[index])
	}
	return result
}
