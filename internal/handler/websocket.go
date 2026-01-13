package handler

import (
	"bufio"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/Bowl42/maxx-next/internal/domain"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // 生产环境需要严格检查
	},
}

type WSMessage struct {
	Type string      `json:"type"` // "proxy_request_update", "stats_update"
	Data interface{} `json:"data"`
}

type WebSocketHub struct {
	clients   map[*websocket.Conn]bool
	broadcast chan WSMessage
	mu        sync.RWMutex
}

func NewWebSocketHub() *WebSocketHub {
	hub := &WebSocketHub{
		clients:   make(map[*websocket.Conn]bool),
		broadcast: make(chan WSMessage, 100),
	}
	go hub.run()
	return hub
}

func (h *WebSocketHub) run() {
	for msg := range h.broadcast {
		h.mu.RLock()
		for client := range h.clients {
			err := client.WriteJSON(msg)
			if err != nil {
				client.Close()
				delete(h.clients, client)
			}
		}
		h.mu.RUnlock()
	}
}

func (h *WebSocketHub) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}

	h.mu.Lock()
	h.clients[conn] = true
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
	h.broadcast <- WSMessage{
		Type: "proxy_request_update",
		Data: req,
	}
}

func (h *WebSocketHub) BroadcastProxyUpstreamAttempt(attempt *domain.ProxyUpstreamAttempt) {
	h.broadcast <- WSMessage{
		Type: "proxy_upstream_attempt_update",
		Data: attempt,
	}
}

func (h *WebSocketHub) BroadcastStats(stats interface{}) {
	h.broadcast <- WSMessage{
		Type: "stats_update",
		Data: stats,
	}
}

// BroadcastMessage sends a custom message with specified type to all connected clients
func (h *WebSocketHub) BroadcastMessage(messageType string, data interface{}) {
	h.broadcast <- WSMessage{
		Type: messageType,
		Data: data,
	}
}

// BroadcastLog sends a log message to all connected clients
func (h *WebSocketHub) BroadcastLog(message string) {
	h.broadcast <- WSMessage{
		Type: "log_message",
		Data: message,
	}
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
