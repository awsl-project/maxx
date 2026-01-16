package kiro

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"sync"
)

// EventStreamParser AWS EventStream 二进制格式解析器
// 匹配 kiro2api/parser/robust_parser.go 实现
type EventStreamParser struct {
	buffer     *bytes.Buffer
	errorCount int
	maxErrors  int
	mu         sync.Mutex
}

// NewEventStreamParser 创建新的解析器
func NewEventStreamParser() *EventStreamParser {
	return &EventStreamParser{
		buffer:    &bytes.Buffer{},
		maxErrors: 10,
	}
}

// Reset 重置解析器状态
func (p *EventStreamParser) Reset() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.buffer.Reset()
	p.errorCount = 0
}

// EventStreamMessage 解析后的消息
type EventStreamMessage struct {
	Headers     map[string]string
	Payload     []byte
	MessageType string
	EventType   string
}

// GetMessageType 获取消息类型
func (m *EventStreamMessage) GetMessageType() string {
	return m.MessageType
}

// GetEventType 获取事件类型
func (m *EventStreamMessage) GetEventType() string {
	return m.EventType
}

const (
	// 最小消息长度：4(totalLen) + 4(headerLen) + 4(preludeCRC) + 4(msgCRC) = 16
	EventStreamMinMessageSize = 16
	// 最大消息长度：16MB
	EventStreamMaxMessageSize = 16 * 1024 * 1024
)

// ParseStream 解析流数据，返回解析出的消息
func (p *EventStreamParser) ParseStream(data []byte) ([]*EventStreamMessage, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// 写入新数据到缓冲区
	p.buffer.Write(data)

	var messages []*EventStreamMessage

	for {
		// 检查是否有足够的数据读取消息头
		if p.buffer.Len() < EventStreamMinMessageSize {
			break
		}

		// 查看消息头（不移除数据）
		bufferBytes := p.buffer.Bytes()

		// 读取消息长度
		totalLength := binary.BigEndian.Uint32(bufferBytes[:4])

		// 验证长度合理性
		if totalLength < EventStreamMinMessageSize || totalLength > EventStreamMaxMessageSize {
			// 跳过无效数据
			p.buffer.Next(1)
			p.errorCount++
			continue
		}

		// 检查是否有足够的数据
		if p.buffer.Len() < int(totalLength) {
			break
		}

		// 读取完整消息
		messageData := make([]byte, totalLength)
		p.buffer.Read(messageData)

		// 解析消息
		message, err := p.parseMessage(messageData)
		if err != nil {
			p.errorCount++
			continue
		}

		if message != nil {
			messages = append(messages, message)
		}
	}

	if p.errorCount >= p.maxErrors {
		return messages, fmt.Errorf("错误次数过多 (%d)", p.errorCount)
	}

	return messages, nil
}

// parseMessage 解析单个消息
func (p *EventStreamParser) parseMessage(data []byte) (*EventStreamMessage, error) {
	if len(data) < EventStreamMinMessageSize {
		return nil, fmt.Errorf("数据长度不足")
	}

	totalLength := binary.BigEndian.Uint32(data[:4])
	headerLength := binary.BigEndian.Uint32(data[4:8])

	// 验证长度
	if int(totalLength) != len(data) {
		return nil, fmt.Errorf("数据长度不匹配")
	}

	if headerLength > totalLength-16 {
		return nil, fmt.Errorf("头部长度异常")
	}

	// 提取各部分（跳过 Prelude CRC）
	headerData := data[12 : 12+headerLength]
	payloadStart := int(12 + headerLength)
	payloadEnd := int(totalLength) - 4

	if payloadStart > payloadEnd || payloadEnd > len(data) {
		return nil, fmt.Errorf("payload 边界异常")
	}

	payloadData := data[payloadStart:payloadEnd]

	// 解析头部
	headers := p.parseHeaders(headerData)

	message := &EventStreamMessage{
		Headers:     headers,
		Payload:     payloadData,
		MessageType: headers[":message-type"],
		EventType:   headers[":event-type"],
	}

	return message, nil
}

// parseHeaders 解析头部
func (p *EventStreamParser) parseHeaders(data []byte) map[string]string {
	headers := make(map[string]string)

	if len(data) == 0 {
		// 默认头部
		headers[":message-type"] = "event"
		headers[":event-type"] = "assistantResponseEvent"
		headers[":content-type"] = "application/json"
		return headers
	}

	offset := 0
	for offset < len(data) {
		// 读取头部名称长度
		if offset >= len(data) {
			break
		}
		nameLen := int(data[offset])
		offset++

		if offset+nameLen > len(data) {
			break
		}

		// 读取头部名称
		name := string(data[offset : offset+nameLen])
		offset += nameLen

		// 读取值类型
		if offset >= len(data) {
			break
		}
		valueType := data[offset]
		offset++

		// 根据值类型读取值
		switch valueType {
		case 7: // STRING
			if offset+2 > len(data) {
				break
			}
			valueLen := int(binary.BigEndian.Uint16(data[offset : offset+2]))
			offset += 2

			if offset+valueLen > len(data) {
				break
			}
			value := string(data[offset : offset+valueLen])
			offset += valueLen

			headers[name] = value

		default:
			// 跳过其他类型
			break
		}
	}

	return headers
}

// ParsedEvent 解析后的事件
type ParsedEvent struct {
	Type string
	Data map[string]interface{}
}

// ParsePayload 解析消息负载
func ParsePayload(payload []byte) (*ParsedEvent, error) {
	var data map[string]interface{}
	if err := json.Unmarshal(payload, &data); err != nil {
		return nil, fmt.Errorf("解析 JSON 失败: %w", err)
	}

	// 检查是否是嵌套格式
	if eventData, ok := data["assistantResponseEvent"].(map[string]interface{}); ok {
		return &ParsedEvent{
			Type: "assistantResponseEvent",
			Data: eventData,
		}, nil
	}

	if eventData, ok := data["toolUseEvent"].(map[string]interface{}); ok {
		return &ParsedEvent{
			Type: "toolUseEvent",
			Data: eventData,
		}, nil
	}

	if eventData, ok := data["codeEvent"].(map[string]interface{}); ok {
		return &ParsedEvent{
			Type: "codeEvent",
			Data: eventData,
		}, nil
	}

	if eventData, ok := data["endOfTurnEvent"].(map[string]interface{}); ok {
		return &ParsedEvent{
			Type: "endOfTurnEvent",
			Data: eventData,
		}, nil
	}

	// 检查是否是直接格式（非嵌套）
	// 匹配 kiro2api 中的处理：检查 content 字段判断是否为 assistantResponseEvent
	if content, ok := data["content"].(string); ok && content != "" {
		return &ParsedEvent{
			Type: "assistantResponseEvent",
			Data: data,
		}, nil
	}

	// 检查是否是 toolUseEvent（直接格式）
	if _, ok := data["toolUseId"].(string); ok {
		if _, nameOk := data["name"].(string); nameOk {
			return &ParsedEvent{
				Type: "toolUseEvent",
				Data: data,
			}, nil
		}
	}

	// 直接返回数据
	return &ParsedEvent{
		Type: "unknown",
		Data: data,
	}, nil
}

// CompliantEventStreamParser 符合规范的事件流解析器
// 匹配 kiro2api/parser/compliant_event_stream_parser.go
// 结合 EventStreamParser (二进制解析) 和 CompliantMessageProcessor (事件转换)
type CompliantEventStreamParser struct {
	rawParser        *EventStreamParser
	messageProcessor *CompliantMessageProcessor
}

// NewCompliantEventStreamParser 创建符合规范的事件流解析器
func NewCompliantEventStreamParser() *CompliantEventStreamParser {
	return &CompliantEventStreamParser{
		rawParser:        NewEventStreamParser(),
		messageProcessor: NewCompliantMessageProcessor(),
	}
}

// Reset 重置解析器状态
func (cesp *CompliantEventStreamParser) Reset() {
	cesp.rawParser.Reset()
	cesp.messageProcessor.Reset()
}

// ParseStream 解析流式数据，返回 Claude SSE 事件
// 匹配 kiro2api/parser/compliant_event_stream_parser.go:ParseStream
func (cesp *CompliantEventStreamParser) ParseStream(data []byte) ([]SSEEvent, error) {
	// 1. 解析二进制事件流
	messages, err := cesp.rawParser.ParseStream(data)
	if err != nil {
		// 继续处理已解析的消息
	}

	var allEvents []SSEEvent

	// 2. 处理每个消息，转换为 Claude SSE 事件
	for _, message := range messages {
		events, processErr := cesp.messageProcessor.ProcessMessage(message)
		if processErr != nil {
			continue
		}
		allEvents = append(allEvents, events...)
	}

	return allEvents, nil
}

// GetMessageProcessor 获取消息处理器
func (cesp *CompliantEventStreamParser) GetMessageProcessor() *CompliantMessageProcessor {
	return cesp.messageProcessor
}
