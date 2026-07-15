package handler

import (
	"bytes"
	stdjson "encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"

	maxxctx "github.com/awsl-project/maxx/internal/context"
	"github.com/awsl-project/maxx/internal/jsonutil"
	"github.com/bytedance/sonic"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/tidwall/gjson"
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
	initialized        bool
	lastInput          []sonic.NoCopyRawMessage
	model              sonic.NoCopyRawMessage
	instructions       sonic.NoCopyRawMessage
	lastResponseOutput []stdjson.RawMessage
}

type responsesWebSocketNormalizedRequest struct {
	body         []byte
	input        []sonic.NoCopyRawMessage
	model        sonic.NoCopyRawMessage
	instructions sonic.NoCopyRawMessage
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

		normalized, errNormalize := state.normalizeRequest(payload)
		if errNormalize != nil {
			if errWrite := writeResponsesWebSocketError(conn, http.StatusBadRequest, errNormalize.Error(), nil); errWrite != nil {
				return
			}
			continue
		}

		request := newResponsesWebSocketHTTPRequest(r, normalized.body, sessionID, payload)
		writer := newResponsesWebSocketResponseWriter(conn)
		next.ServeHTTP(writer, request)
		if errFinish := writer.finish(); errFinish != nil {
			return
		}
		if writer.completed {
			state.initialized = true
			state.lastInput = normalized.input
			state.model = normalized.model
			state.instructions = normalized.instructions
			state.lastResponseOutput = writer.completedOutput
		}
	}
}

func newResponsesWebSocketHTTPRequest(original *http.Request, body []byte, sessionID string, payload []byte) *http.Request {
	ctx := maxxctx.WithResponsesWebSocketRequest(original.Context(), sessionID, payload)
	ctx = maxxctx.WithRequestBody(ctx, body)
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

func (s *responsesWebSocketSessionState) normalizeRequest(payload []byte) (responsesWebSocketNormalizedRequest, error) {
	request, err := decodeJSONObjectNoCopy(payload)
	if err != nil {
		return responsesWebSocketNormalizedRequest{}, fmt.Errorf("invalid websocket request JSON: %w", err)
	}

	requestType := jsonString(request["type"])
	switch requestType {
	case responsesWebSocketRequestCreate:
		if !s.initialized {
			return normalizeInitialResponsesWebSocketRequest(request)
		}
		return s.normalizeSubsequentResponsesWebSocketRequest(request)
	case responsesWebSocketRequestAppend:
		if !s.initialized {
			return responsesWebSocketNormalizedRequest{}, fmt.Errorf("websocket request received before response.create")
		}
		return s.normalizeSubsequentResponsesWebSocketRequest(request)
	default:
		return responsesWebSocketNormalizedRequest{}, fmt.Errorf("unsupported websocket request type: %s", requestType)
	}
}

func normalizeInitialResponsesWebSocketRequest(request map[string]sonic.NoCopyRawMessage) (responsesWebSocketNormalizedRequest, error) {
	delete(request, "type")
	delete(request, "generate")
	request["stream"] = sonic.NoCopyRawMessage("true")
	if _, ok := request["input"]; !ok {
		request["input"] = sonic.NoCopyRawMessage("[]")
	}
	if strings.TrimSpace(jsonString(request["model"])) == "" {
		return responsesWebSocketNormalizedRequest{}, fmt.Errorf("missing model in response.create request")
	}
	input, err := decodeJSONArrayNoCopy(request["input"])
	if err != nil {
		return responsesWebSocketNormalizedRequest{}, fmt.Errorf("websocket request requires array field: input")
	}
	normalized, err := marshalResponsesWebSocketRequest(request, input)
	if err != nil {
		return responsesWebSocketNormalizedRequest{}, err
	}
	return responsesWebSocketNormalizedRequest{
		body:         normalized,
		input:        input,
		model:        request["model"],
		instructions: request["instructions"],
	}, nil
}

func (s *responsesWebSocketSessionState) normalizeSubsequentResponsesWebSocketRequest(request map[string]sonic.NoCopyRawMessage) (responsesWebSocketNormalizedRequest, error) {
	nextInput, err := decodeJSONArrayNoCopy(request["input"])
	if err != nil {
		return responsesWebSocketNormalizedRequest{}, fmt.Errorf("websocket request requires array field: input")
	}

	previousResponseID := strings.TrimSpace(jsonString(request["previous_response_id"]))
	var mergedInput []sonic.NoCopyRawMessage
	if previousResponseID == "" && responsesWebSocketInputReplacesTranscript(nextInput) {
		mergedInput = nextInput
	} else {
		mergedInput = make([]sonic.NoCopyRawMessage, 0, len(s.lastInput)+len(s.lastResponseOutput)+len(nextInput))
		mergedInput = append(mergedInput, s.lastInput...)
		for _, output := range s.lastResponseOutput {
			mergedInput = append(mergedInput, sonic.NoCopyRawMessage(output))
		}
		mergedInput = append(mergedInput, nextInput...)
		mergedInput = dedupeResponsesWebSocketItems(mergedInput)
	}

	delete(request, "type")
	delete(request, "generate")
	delete(request, "previous_response_id")
	request["stream"] = sonic.NoCopyRawMessage("true")
	if _, ok := request["model"]; !ok {
		request["model"] = s.model
	}
	if _, ok := request["instructions"]; !ok {
		if len(s.instructions) > 0 {
			request["instructions"] = s.instructions
		}
	}

	normalized, err := marshalResponsesWebSocketRequest(request, mergedInput)
	if err != nil {
		return responsesWebSocketNormalizedRequest{}, fmt.Errorf("failed to merge websocket input: %w", err)
	}
	return responsesWebSocketNormalizedRequest{
		body:         normalized,
		input:        mergedInput,
		model:        request["model"],
		instructions: request["instructions"],
	}, nil
}

func marshalResponsesWebSocketRequest(request map[string]sonic.NoCopyRawMessage, input []sonic.NoCopyRawMessage) ([]byte, error) {
	wireRequest := make(map[string]interface{}, len(request))
	for key, value := range request {
		if key != "input" {
			wireRequest[key] = value
		}
	}
	wireRequest["input"] = input
	return jsonutil.Marshal(wireRequest)
}

func responsesWebSocketInputReplacesTranscript(input []sonic.NoCopyRawMessage) bool {
	for _, item := range input {
		switch gjson.GetBytes(item, "type").String() {
		case "compaction", "compaction_summary", "function_call", "custom_tool_call":
			return true
		case "message":
			if gjson.GetBytes(item, "role").String() == "assistant" {
				return true
			}
		}
	}
	return false
}

func dedupeResponsesWebSocketItems(items []sonic.NoCopyRawMessage) []sonic.NoCopyRawMessage {
	seenItemIDs := make(map[string]struct{}, len(items))
	seenCallIDs := make(map[string]struct{}, len(items))
	result := make([]sonic.NoCopyRawMessage, 0, len(items))
	for _, item := range items {
		if id := strings.TrimSpace(gjson.GetBytes(item, "id").String()); id != "" {
			if _, exists := seenItemIDs[id]; exists {
				continue
			}
			seenItemIDs[id] = struct{}{}
		}
		switch gjson.GetBytes(item, "type").String() {
		case "function_call", "custom_tool_call":
			if callID := strings.TrimSpace(gjson.GetBytes(item, "call_id").String()); callID != "" {
				if _, exists := seenCallIDs[callID]; exists {
					continue
				}
				seenCallIDs[callID] = struct{}{}
			}
		}
		result = append(result, item)
	}
	return result
}

func newResponsesWebSocketResponseWriter(conn *websocket.Conn) *responsesWebSocketResponseWriter {
	return &responsesWebSocketResponseWriter{
		conn:               conn,
		header:             make(http.Header),
		statusCode:         http.StatusOK,
		outputItemsByIndex: make(map[int]stdjson.RawMessage),
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

	outputItemsByIndex  map[int]stdjson.RawMessage
	outputItemsFallback []stdjson.RawMessage
	completed           bool
	completedOutput     []stdjson.RawMessage
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
	payload := w.eventData.Bytes()
	if bytes.Equal(bytes.TrimSpace(payload), []byte("[DONE]")) {
		w.eventData.Reset()
		return
	}
	w.forwardPayload(payload)
	w.eventData.Reset()
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
				w.outputItemsByIndex[outputIndex] = item
			} else {
				w.outputItemsFallback = append(w.outputItemsFallback, item)
			}
		}
	}

	if eventType == "response.completed" || eventType == "response.done" {
		response, errResponse := decodeJSONObject(object["response"])
		if errResponse == nil {
			output, errOutput := decodeJSONArray(response["output"])
			if errOutput != nil || len(output) == 0 {
				output = w.collectedOutput()
				if encodedOutput, errMarshal := jsonutil.Marshal(output); errMarshal == nil {
					response["output"] = encodedOutput
					if encodedResponse, errMarshalResponse := jsonutil.Marshal(response); errMarshalResponse == nil {
						object["response"] = encodedResponse
					}
				}
			}
			w.completedOutput = output
		}
		w.completed = true
	}
	if eventType == "error" {
		w.sentError = true
	}

	encoded, errMarshal := jsonutil.Marshal(object)
	if errMarshal != nil {
		w.writeErr = errMarshal
		return
	}
	w.writeErr = w.conn.WriteMessage(websocket.TextMessage, encoded)
}

func (w *responsesWebSocketResponseWriter) collectedOutput() []stdjson.RawMessage {
	indexes := make([]int, 0, len(w.outputItemsByIndex))
	for index := range w.outputItemsByIndex {
		indexes = append(indexes, index)
	}
	sort.Ints(indexes)
	result := make([]stdjson.RawMessage, 0, len(indexes)+len(w.outputItemsFallback))
	for _, index := range indexes {
		result = append(result, w.outputItemsByIndex[index])
	}
	result = append(result, w.outputItemsFallback...)
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
		"response":        stdjson.RawMessage(payload),
	}
	encoded, errMarshal := jsonutil.Marshal(completion)
	if errMarshal != nil {
		w.writeErr = errMarshal
		return
	}
	w.forwardPayload(encoded)
}

func writeResponsesWebSocketError(conn *websocket.Conn, status int, message string, errorBody stdjson.RawMessage) error {
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
	return conn.WriteMessage(websocket.TextMessage, payload)
}

func decodeJSONObject(raw []byte) (map[string]stdjson.RawMessage, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, fmt.Errorf("empty JSON object")
	}
	var object map[string]stdjson.RawMessage
	if err := jsonutil.Unmarshal(raw, &object); err != nil {
		return nil, err
	}
	if object == nil {
		return nil, fmt.Errorf("expected JSON object")
	}
	return object, nil
}

func decodeJSONObjectNoCopy(raw []byte) (map[string]sonic.NoCopyRawMessage, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, fmt.Errorf("empty JSON object")
	}
	if !gjson.ValidBytes(raw) {
		return nil, fmt.Errorf("invalid JSON object")
	}
	var object map[string]sonic.NoCopyRawMessage
	if err := jsonutil.Unmarshal(raw, &object); err != nil {
		return nil, err
	}
	if object == nil {
		return nil, fmt.Errorf("expected JSON object")
	}
	return object, nil
}

func decodeJSONArray(raw []byte) ([]stdjson.RawMessage, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, fmt.Errorf("missing JSON array")
	}
	var array []stdjson.RawMessage
	if err := jsonutil.Unmarshal(raw, &array); err != nil {
		return nil, err
	}
	if array == nil {
		array = []stdjson.RawMessage{}
	}
	return array, nil
}

func decodeJSONArrayNoCopy(raw []byte) ([]sonic.NoCopyRawMessage, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, fmt.Errorf("missing JSON array")
	}
	var array []sonic.NoCopyRawMessage
	if err := jsonutil.Unmarshal(raw, &array); err != nil {
		return nil, err
	}
	if array == nil {
		array = []sonic.NoCopyRawMessage{}
	}
	return array, nil
}

func jsonString(raw []byte) string {
	var value string
	if jsonutil.Unmarshal(raw, &value) != nil {
		return ""
	}
	return value
}

func jsonInt(raw []byte) (int, bool) {
	var value int
	if jsonutil.Unmarshal(raw, &value) != nil {
		return 0, false
	}
	return value, true
}
