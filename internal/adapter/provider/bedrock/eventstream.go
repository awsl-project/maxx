package bedrock

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"hash/crc32"
	"io"
)

// eventStreamDecoder decodes AWS Event Stream binary messages.
// See: https://docs.aws.amazon.com/transcribe/latest/dg/event-stream.html
//
// Each message has the format:
//   [total_length:4][headers_length:4][prelude_crc:4][headers:*][payload:*][message_crc:4]
type eventStreamDecoder struct {
	r io.Reader
}

func newEventStreamDecoder(r io.Reader) *eventStreamDecoder {
	return &eventStreamDecoder{r: r}
}


// Decode reads the next event stream message and returns the payload bytes.
// Returns io.EOF when the stream ends.
func (d *eventStreamDecoder) Decode() ([]byte, error) {
	// Read prelude (12 bytes): total_length(4) + headers_length(4) + prelude_crc(4)
	prelude := make([]byte, 12)
	if _, err := io.ReadFull(d.r, prelude); err != nil {
		return nil, err
	}

	totalLen := binary.BigEndian.Uint32(prelude[0:4])
	headersLen := binary.BigEndian.Uint32(prelude[4:8])
	preludeCRC := binary.BigEndian.Uint32(prelude[8:12])

	// Guard against unreasonably large messages (max 16 MB)
	const maxMessageSize = 16 * 1024 * 1024
	if totalLen > maxMessageSize {
		return nil, fmt.Errorf("event stream message too large: %d bytes", totalLen)
	}

	// Validate prelude CRC
	crc := crc32.NewIEEE()
	crc.Write(prelude[0:8])
	if crc.Sum32() != preludeCRC {
		return nil, fmt.Errorf("event stream prelude CRC mismatch")
	}

	// Read the rest of the message: headers + payload + message_crc(4)
	remaining := int(totalLen) - 12
	if remaining < 4 {
		return nil, fmt.Errorf("event stream message too short: total_length=%d", totalLen)
	}
	buf := make([]byte, remaining)
	if _, err := io.ReadFull(d.r, buf); err != nil {
		return nil, err
	}

	// Validate message CRC
	messageCRC := binary.BigEndian.Uint32(buf[len(buf)-4:])
	msgCRC := crc32.NewIEEE()
	msgCRC.Write(prelude)
	msgCRC.Write(buf[:len(buf)-4])
	if msgCRC.Sum32() != messageCRC {
		return nil, fmt.Errorf("event stream message CRC mismatch")
	}

	// Extract raw payload (after headers, before message CRC)
	rawPayload := buf[headersLen : len(buf)-4]

	// Check headers for exception type
	headers := parseHeaders(buf[:headersLen])
	if msgType, ok := headers[":message-type"]; ok && msgType == "exception" {
		return nil, fmt.Errorf("bedrock stream exception: %s", string(rawPayload))
	}

	// Bedrock wraps the actual content in {"bytes":"<base64>"}, decode it
	payload := extractPayloadBytes(rawPayload)

	return payload, nil
}

// extractPayloadBytes decodes the base64 content from {"bytes":"<base64>"} wrapper.
func extractPayloadBytes(raw []byte) []byte {
	var wrapper struct {
		Bytes string `json:"bytes"`
	}
	if err := json.Unmarshal(raw, &wrapper); err != nil || wrapper.Bytes == "" {
		return raw // Not wrapped, return as-is
	}
	decoded, err := base64.StdEncoding.DecodeString(wrapper.Bytes)
	if err != nil {
		return raw
	}
	return decoded
}

// parseHeaders extracts string headers from the AWS event stream headers section.
func parseHeaders(data []byte) map[string]string {
	headers := make(map[string]string)
	offset := 0
	for offset < len(data) {
		// Header name length (1 byte)
		nameLen := int(data[offset])
		offset++
		if offset+nameLen > len(data) {
			break
		}
		name := string(data[offset : offset+nameLen])
		offset += nameLen

		if offset >= len(data) {
			break
		}
		// Header type (1 byte)
		headerType := data[offset]
		offset++

		switch headerType {
		case 7: // String type
			if offset+2 > len(data) {
				break
			}
			valueLen := int(binary.BigEndian.Uint16(data[offset : offset+2]))
			offset += 2
			if offset+valueLen > len(data) {
				break
			}
			headers[name] = string(data[offset : offset+valueLen])
			offset += valueLen
		default:
			// Skip unknown header types — for our use case we only need strings
			// Other types: 0=bool_true, 1=bool_false, 2=byte, 3=short, 4=int, 5=long,
			//              6=bytes, 7=string, 8=timestamp, 9=uuid
			switch headerType {
			case 0, 1: // bool
				// no value bytes
			case 2: // byte
				offset++
			case 3: // short
				offset += 2
			case 4: // int
				offset += 4
			case 5, 8: // long, timestamp
				offset += 8
			case 9: // uuid
				offset += 16
			case 6: // bytes
				if offset+2 > len(data) {
					return headers
				}
				bLen := int(binary.BigEndian.Uint16(data[offset : offset+2]))
				offset += 2 + bLen
			}
		}
	}
	return headers
}
