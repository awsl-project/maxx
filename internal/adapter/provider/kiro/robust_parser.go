package kiro

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"sync"
)

// RobustEventStreamParser parses AWS EventStream frames with error recovery.
type RobustEventStreamParser struct {
	buffer     *bytes.Buffer
	errorCount int
	maxErrors  int
	mu         sync.Mutex
}

// NewRobustEventStreamParser creates a parser instance.
func NewRobustEventStreamParser() *RobustEventStreamParser {
	return &RobustEventStreamParser{
		buffer:    &bytes.Buffer{},
		maxErrors: 10,
	}
}

// SetMaxErrors overrides the maximum tolerated errors.
func (rp *RobustEventStreamParser) SetMaxErrors(maxErrors int) {
	rp.maxErrors = maxErrors
}

// Reset clears parser state.
func (rp *RobustEventStreamParser) Reset() {
	rp.mu.Lock()
	defer rp.mu.Unlock()
	rp.buffer.Reset()
	rp.errorCount = 0
}

// ParseStream parses incoming bytes into EventStream messages.
func (rp *RobustEventStreamParser) ParseStream(data []byte) ([]*EventStreamMessage, error) {
	rp.mu.Lock()
	defer rp.mu.Unlock()

	if _, err := rp.buffer.Write(data); err != nil {
		return nil, err
	}

	messages := make([]*EventStreamMessage, 0, 8)

	for rp.buffer.Len() >= EventStreamMinMessageSize {

		bufferBytes := rp.buffer.Bytes()
		if len(bufferBytes) < EventStreamMinMessageSize {
			break
		}

		totalLength := binary.BigEndian.Uint32(bufferBytes[:4])
		if totalLength < EventStreamMinMessageSize || totalLength > EventStreamMaxMessageSize {
			rp.buffer.Next(1)
			rp.errorCount++
			continue
		}

		if rp.buffer.Len() < int(totalLength) {
			break
		}

		messageData := make([]byte, totalLength)
		n, err := rp.buffer.Read(messageData)
		if err != nil || n != int(totalLength) {
			break
		}

		message, err := rp.parseMessage(messageData)
		if err != nil {
			rp.errorCount++
			continue
		}

		if message != nil {
			messages = append(messages, message)
		}
	}

	if rp.errorCount >= rp.maxErrors {
		return messages, fmt.Errorf("too many parse errors (%d)", rp.errorCount)
	}

	return messages, nil
}

func (rp *RobustEventStreamParser) parseMessage(data []byte) (*EventStreamMessage, error) {
	if len(data) < EventStreamMinMessageSize {
		return nil, NewParseError("eventstream data too short", nil)
	}

	totalLength := binary.BigEndian.Uint32(data[:4])
	headerLength := binary.BigEndian.Uint32(data[4:8])

	if int(totalLength) != len(data) {
		return nil, NewParseError("eventstream length mismatch", nil)
	}

	if headerLength > totalLength-16 {
		return nil, NewParseError("eventstream header length invalid", nil)
	}

	headerData := data[12 : 12+headerLength]
	payloadStart := int(12 + headerLength)
	payloadEnd := int(totalLength) - 4
	if payloadStart > payloadEnd || payloadEnd > len(data) {
		return nil, NewParseError("eventstream payload bounds invalid", nil)
	}

	payloadData := data[payloadStart:payloadEnd]
	headers := parseHeaders(headerData)

	message := &EventStreamMessage{
		Headers: headers,
		Payload: payloadData,
	}
	message.MessageType = message.GetMessageType()
	message.EventType = message.GetEventType()
	message.ContentType = message.GetContentType()

	return message, nil
}

func parseHeaders(data []byte) map[string]HeaderValue {
	if len(data) == 0 {
		return defaultHeaders()
	}

	headers := make(map[string]HeaderValue)
	offset := 0
	for offset < len(data) {
		if offset >= len(data) {
			break
		}
		nameLen := int(data[offset])
		offset++
		if nameLen == 0 || offset+nameLen > len(data) {
			break
		}
		name := string(data[offset : offset+nameLen])
		offset += nameLen

		if offset >= len(data) {
			break
		}
		valueType := ValueType(data[offset])
		offset++

		switch valueType {
		case ValueTypeString:
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
			headers[name] = HeaderValue{Type: valueType, Value: value}
		default:
			// Unsupported value types are skipped.
			return defaultHeaders()
		}
	}

	if len(headers) == 0 {
		return defaultHeaders()
	}

	return headers
}

func defaultHeaders() map[string]HeaderValue {
	return map[string]HeaderValue{
		":message-type": {Type: ValueTypeString, Value: "event"},
		":event-type":   {Type: ValueTypeString, Value: "assistantResponseEvent"},
		":content-type": {Type: ValueTypeString, Value: "application/json"},
	}
}
