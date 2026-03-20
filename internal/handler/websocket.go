package handler

import (
	"bufio"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/event"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // 生产环境需要严格检查
	},
}

type WSMessage struct {
	Type string      `json:"type"` // "proxy_request_update", "proxy_upstream_attempt_update", etc.
	Data interface{} `json:"data"`
}

type wsClient struct {
	tenantID uint64
	userID   uint64
	role     string
}

type queuedWSMessage struct {
	message   WSMessage
	tenantID  uint64
	scoped    bool
	defaultOnly bool
}

type WebSocketHub struct {
	clients   map[*websocket.Conn]wsClient
	broadcast chan queuedWSMessage
	mu        sync.RWMutex

	authMiddleware *AuthMiddleware

	// broadcast channel 满时的丢弃计数（热路径：只做原子累加）
	broadcastDroppedTotal atomic.Uint64
}

const websocketWriteTimeout = 5 * time.Second

func NewWebSocketHub() *WebSocketHub {
	hub := &WebSocketHub{
		clients:   make(map[*websocket.Conn]wsClient),
		broadcast: make(chan queuedWSMessage, 100),
	}
	go hub.run()
	return hub
}

func (h *WebSocketHub) SetAuthMiddleware(auth *AuthMiddleware) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.authMiddleware = auth
}

func (h *WebSocketHub) run() {
	for queued := range h.broadcast {
		// 避免在持锁状态下进行网络写入；同时修复 RLock 下 delete map 的数据竞争风险
		h.mu.RLock()
		targets := make([]struct {
			conn   *websocket.Conn
			client wsClient
		}, 0, len(h.clients))
		for conn, client := range h.clients {
			if queued.defaultOnly && client.tenantID != domain.DefaultTenantID {
				continue
			}
			if queued.scoped && client.tenantID != queued.tenantID {
				continue
			}
			targets = append(targets, struct {
				conn   *websocket.Conn
				client wsClient
			}{conn: conn, client: client})
		}
		h.mu.RUnlock()

		var toRemove []*websocket.Conn
		for _, target := range targets {
			_ = target.conn.SetWriteDeadline(time.Now().Add(websocketWriteTimeout))
			if err := target.conn.WriteJSON(queued.message); err != nil {
				_ = target.conn.Close()
				toRemove = append(toRemove, target.conn)
			}
		}

		if len(toRemove) > 0 {
			h.mu.Lock()
			for _, conn := range toRemove {
				delete(h.clients, conn)
			}
			h.mu.Unlock()
		}
	}
}

func (h *WebSocketHub) tryEnqueueBroadcast(msg queuedWSMessage, meta string) {
	select {
	case h.broadcast <- msg:
	default:
		// Only increment counter; do NOT call log.Printf here.
		// This function is called from within WebSocketLogWriter.Write(),
		// which is invoked by log.Printf while holding the log mutex.
		// Calling log.Printf again would deadlock (sync.Mutex is not reentrant).
		h.broadcastDroppedTotal.Add(1)
	}
}

func (h *WebSocketHub) authenticateRequest(r *http.Request) (wsClient, bool) {
	h.mu.RLock()
	auth := h.authMiddleware
	h.mu.RUnlock()

	if auth == nil {
		return wsClient{tenantID: domain.DefaultTenantID, userID: domain.DefaultUserID, role: string(domain.UserRoleAdmin)}, true
	}

	token := strings.TrimSpace(r.URL.Query().Get("access_token"))
	if token == "" {
		authHeader := r.Header.Get(AuthHeader)
		if strings.HasPrefix(authHeader, "Bearer ") {
			token = strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer "))
		}
	}
	if token == "" {
		return wsClient{}, false
	}

	claims, valid := auth.ValidateToken(token)
	if !valid {
		return wsClient{}, false
	}

	return wsClient{tenantID: claims.TenantID, userID: claims.UserID, role: claims.Role}, true
}

func (h *WebSocketHub) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	clientMeta, ok := h.authenticateRequest(r)
	if !ok {
		writeUnauthorized(w)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}

	h.mu.Lock()
	h.clients[conn] = clientMeta
	h.mu.Unlock()

	defer func() {
		h.mu.Lock()
		delete(h.clients, conn)
		h.mu.Unlock()
		conn.Close()
	}()

	// 保持连接，处理客户端消息（心跳等）
	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			break
		}
	}
}

func (h *WebSocketHub) BroadcastProxyRequest(req *domain.ProxyRequest) {
	sanitized := event.SanitizeProxyRequestForBroadcast(req)
	var data interface{} = sanitized
	var meta string
	if sanitized != nil {
		// 无论 Sanitize 是否返回原指针，都强制做一次浅拷贝快照，避免异步消费者读到后续可变更的数据。
		snapshot := *sanitized
		data = snapshot
		meta = "requestID=" + snapshot.RequestID
		if snapshot.ID != 0 {
			meta += " requestDbID=" + strconv.FormatUint(snapshot.ID, 10)
		}
	}
	msg := queuedWSMessage{
		message: WSMessage{
			Type: "proxy_request_update",
			Data: data,
		},
	}
	if sanitized != nil && sanitized.TenantID != 0 {
		msg.tenantID = sanitized.TenantID
		msg.scoped = true
	}
	h.tryEnqueueBroadcast(msg, meta)
}

func (h *WebSocketHub) BroadcastProxyUpstreamAttempt(attempt *domain.ProxyUpstreamAttempt) {
	sanitized := event.SanitizeProxyUpstreamAttemptForBroadcast(attempt)
	var data interface{} = sanitized
	var meta string
	if sanitized != nil {
		snapshot := *sanitized
		data = snapshot
		if snapshot.ProxyRequestID != 0 {
			meta = "proxyRequestID=" + strconv.FormatUint(snapshot.ProxyRequestID, 10)
		}
		if snapshot.ID != 0 {
			if meta != "" {
				meta += " "
			}
			meta += "attemptDbID=" + strconv.FormatUint(snapshot.ID, 10)
		}
	}
	msg := queuedWSMessage{
		message: WSMessage{
			Type: "proxy_upstream_attempt_update",
			Data: data,
		},
	}
	if sanitized != nil && sanitized.TenantID != 0 {
		msg.tenantID = sanitized.TenantID
		msg.scoped = true
	}
	h.tryEnqueueBroadcast(msg, meta)
}

// BroadcastMessage sends a custom message with specified type to all connected clients
func (h *WebSocketHub) BroadcastMessage(messageType string, data interface{}) {
	h.broadcastJSONMessage(0, false, false, messageType, data)
}

// BroadcastMessageToTenant sends a custom message to a specific tenant only.
func (h *WebSocketHub) BroadcastMessageToTenant(tenantID uint64, messageType string, data interface{}) {
	h.broadcastJSONMessage(tenantID, true, false, messageType, data)
}

func (h *WebSocketHub) broadcastJSONMessage(tenantID uint64, scoped bool, defaultOnly bool, messageType string, data interface{}) {
	// 约定：BroadcastMessage 允许调用方传入 map/struct/指针等可变对象。
	//
	// 但由于实际发送是异步的（入队后由 run() 写到各连接），如果这里直接把可变指针放进 channel，
	// 调用方在入队后继续修改数据，会导致与 BroadcastProxyRequest 类似的数据竞态。
	//
	// 因此这里先把 data 预先序列化为 json.RawMessage，形成不可变快照；后续 WriteJSON 会直接写入该快照。
	var snapshot interface{} = data
	if data != nil {
		if raw, ok := data.(json.RawMessage); ok {
			snapshot = raw
		} else {
			b, err := json.Marshal(data)
			if err != nil {
				log.Printf("[WebSocket] drop broadcast message type=%s: marshal snapshot failed: %v", messageType, err)
				return
			}
			snapshot = json.RawMessage(b)
		}
	}
	msg := queuedWSMessage{
		message: WSMessage{
			Type: messageType,
			Data: snapshot,
		},
		tenantID: tenantID,
		scoped: scoped,
		defaultOnly: defaultOnly,
	}
	h.tryEnqueueBroadcast(msg, "")
}

// BroadcastLog sends a log message to all connected clients.
func (h *WebSocketHub) BroadcastLog(message string) {
	h.broadcastJSONMessage(0, false, false, "log_message", message)
}

// WebSocketLogWriter implements io.Writer to capture logs and broadcast via WebSocket
type WebSocketLogWriter struct {
	hub      *WebSocketHub
	stdout   io.Writer
	logFile  *os.File
	filePath string
}

// NewWebSocketLogWriter creates a writer that broadcasts logs via WebSocket and writes to file
func NewWebSocketLogWriter(hub *WebSocketHub, stdout io.Writer, logPath string) *WebSocketLogWriter {
	// Open log file in append mode
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		log.Printf("Warning: Failed to open log file %s: %v", logPath, err)
	}

	return &WebSocketLogWriter{
		hub:      hub,
		stdout:   stdout,
		logFile:  logFile,
		filePath: logPath,
	}
}

// Write implements io.Writer
func (w *WebSocketLogWriter) Write(p []byte) (n int, err error) {
	// Write to stdout first
	n, err = w.stdout.Write(p)
	if err != nil {
		return n, err
	}

	// Write to log file
	if w.logFile != nil {
		w.logFile.Write(p)
	}

	// Broadcast to WebSocket clients
	msg := strings.TrimSpace(string(p))
	if msg != "" {
		w.hub.BroadcastLog(msg)
	}

	return n, nil
}

// ReadLastNLines reads the last n lines from the specified log file
func ReadLastNLines(logPath string, n int) ([]string, error) {
	file, err := os.Open(logPath)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}
	defer file.Close()

	// Get file info for size
	stat, err := file.Stat()
	if err != nil {
		return nil, err
	}

	// For small files, read all lines
	if stat.Size() < 1024*1024 { // Less than 1MB
		var lines []string
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			lines = append(lines, scanner.Text())
		}
		if err := scanner.Err(); err != nil {
			return nil, err
		}

		// Return last n lines
		if len(lines) <= n {
			return lines, nil
		}
		return lines[len(lines)-n:], nil
	}

	// For large files, seek from the end
	// Read backwards in chunks to find enough newlines
	chunkSize := int64(8192)
	offset := stat.Size()
	var chunks [][]byte

	for offset > 0 && countNewlines(chunks) < n+1 {
		readSize := chunkSize
		if offset < chunkSize {
			readSize = offset
		}
		offset -= readSize

		chunk := make([]byte, readSize)
		_, err := file.ReadAt(chunk, offset)
		if err != nil && err != io.EOF {
			return nil, err
		}
		chunks = append([][]byte{chunk}, chunks...)
	}

	// Combine chunks and split into lines
	var allData []byte
	for _, chunk := range chunks {
		allData = append(allData, chunk...)
	}

	lines := strings.Split(string(allData), "\n")
	// Filter empty lines
	var nonEmptyLines []string
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			nonEmptyLines = append(nonEmptyLines, line)
		}
	}

	// Return last n lines
	if len(nonEmptyLines) <= n {
		return nonEmptyLines, nil
	}
	return nonEmptyLines[len(nonEmptyLines)-n:], nil
}

func countNewlines(chunks [][]byte) int {
	count := 0
	for _, chunk := range chunks {
		for _, b := range chunk {
			if b == '\n' {
				count++
			}
		}
	}
	return count
}

func appendAccessToken(rawURL string, token string) string {
	if strings.TrimSpace(token) == "" {
		return rawURL
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	q := parsed.Query()
	q.Set("access_token", token)
	parsed.RawQuery = q.Encode()
	return parsed.String()
}
