package kiro

import (
	"crypto/md5"
	cryptoRand "crypto/rand"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// ConversationIDManager 会话ID管理器 (匹配 kiro2api/utils/conversation_id.go)
type ConversationIDManager struct {
	mu    sync.RWMutex
	cache map[string]string
}

// NewConversationIDManager 创建新的会话ID管理器
func NewConversationIDManager() *ConversationIDManager {
	return &ConversationIDManager{
		cache: make(map[string]string),
	}
}

// GenerateConversationID 基于客户端信息生成稳定的会话ID
// 匹配 kiro2api/utils/conversation_id.go:25-62
func (c *ConversationIDManager) GenerateConversationID(req *http.Request) string {
	// 从请求头中获取客户端标识信息
	clientIP := getClientIP(req)
	userAgent := req.Header.Get("User-Agent")

	// 检查是否有自定义的会话ID头（优先级最高）
	if customConvID := req.Header.Get("X-Conversation-ID"); customConvID != "" {
		return customConvID
	}

	// 为避免过于细粒度的会话分割，使用时间窗口来保持会话持久性
	// 每小时内的同一客户端使用相同的conversationId
	timeWindow := time.Now().Format("2006010215") // 精确到小时

	// 构建客户端特征字符串
	clientSignature := fmt.Sprintf("%s|%s|%s", clientIP, userAgent, timeWindow)

	// 检查缓存 (使用读锁)
	c.mu.RLock()
	if cachedID, exists := c.cache[clientSignature]; exists {
		c.mu.RUnlock()
		return cachedID
	}
	c.mu.RUnlock()

	// 生成基于特征的MD5哈希
	hash := md5.Sum([]byte(clientSignature))
	conversationID := fmt.Sprintf("conv-%x", hash[:8]) // 使用前8字节，保持简洁

	// 缓存结果 (使用写锁)
	c.mu.Lock()
	c.cache[clientSignature] = conversationID
	c.mu.Unlock()

	return conversationID
}

// GenerateAgentContinuationID 生成稳定的代理延续GUID
// 匹配 kiro2api/utils/conversation_id.go:88-106
func (c *ConversationIDManager) GenerateAgentContinuationID(req *http.Request) string {
	if req == nil {
		return generateUUID()
	}

	// 检查是否有自定义的代理延续ID头（优先级最高）
	if customAgentID := req.Header.Get("X-Agent-Continuation-ID"); customAgentID != "" {
		return customAgentID
	}

	// 提取客户端特征信息
	clientSignature := c.buildAgentClientSignature(req)

	// 生成确定性GUID
	return generateDeterministicGUID(clientSignature, "agent")
}

// buildAgentClientSignature 构建代理客户端特征签名
func (c *ConversationIDManager) buildAgentClientSignature(req *http.Request) string {
	clientIP := getClientIP(req)
	userAgent := req.Header.Get("User-Agent")

	// 统一使用1小时时间窗口，与ConversationId保持一致
	timeWindow := time.Now().Format("2006010215") // 精确到小时

	return fmt.Sprintf("agent|%s|%s|%s", clientIP, userAgent, timeWindow)
}

// generateDeterministicGUID 基于输入字符串生成确定性GUID
// 匹配 kiro2api/utils/conversation_id.go:120-137
func generateDeterministicGUID(input, namespace string) string {
	// 在输入中加入命名空间以避免冲突
	namespacedInput := fmt.Sprintf("%s|%s", namespace, input)

	// 生成MD5哈希
	hash := md5.Sum([]byte(namespacedInput))

	// 按照UUID格式重新排列字节
	// 设置版本位 (Version 5 - 基于命名空间的UUID)
	hash[6] = (hash[6] & 0x0f) | 0x50 // Version 5
	hash[8] = (hash[8] & 0x3f) | 0x80 // Variant bits

	// 格式化为标准GUID格式: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
	return fmt.Sprintf("%x-%x-%x-%x-%x",
		hash[0:4], hash[4:6], hash[6:8], hash[8:10], hash[10:16])
}

// getClientIP 从请求中提取客户端IP
func getClientIP(req *http.Request) string {
	// 优先使用 X-Forwarded-For
	if xff := req.Header.Get("X-Forwarded-For"); xff != "" {
		return xff
	}
	// 其次使用 X-Real-IP
	if xri := req.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}
	// 最后使用 RemoteAddr
	return req.RemoteAddr
}

// generateUUID 生成 UUID v4 (匹配 kiro2api utils/uuid.go)
func generateUUID() string {
	b := make([]byte, 16)
	cryptoRand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40 // Version 4
	b[8] = (b[8] & 0x3f) | 0x80 // Variant bits
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

// InvalidateOldSessions 清理过期的会话缓存
func (c *ConversationIDManager) InvalidateOldSessions() {
	c.mu.Lock()
	c.cache = make(map[string]string)
	c.mu.Unlock()
}

// 全局实例 - 单例模式
var globalConversationIDManager = NewConversationIDManager()

// GenerateStableConversationID 生成稳定的会话ID的全局函数
func GenerateStableConversationID(req *http.Request) string {
	return globalConversationIDManager.GenerateConversationID(req)
}

// GenerateStableAgentContinuationID 生成稳定的代理延续ID的全局函数
func GenerateStableAgentContinuationID(req *http.Request) string {
	return globalConversationIDManager.GenerateAgentContinuationID(req)
}
