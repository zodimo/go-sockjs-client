package sockjs

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// Frame type constants
const (
	// Frame type identifiers
	FrameOpen      = 'o'
	FrameClose     = 'c'
	FrameHeartbeat = 'h'
	FrameMessage   = 'a'
	FrameSingle    = 'm' // For single message frame (some transports use this)
)

// CloseInfo represents a SockJS close frame with status code and reason
type CloseInfo struct {
	Code   int
	Reason string
}

// ParseFrame extracts the frame type and payload from raw data
func ParseFrame(data []byte) (frameType byte, payload []byte, err error) {
	if len(data) == 0 {
		return 0, nil, errors.New("empty frame")
	}
	return data[0], data[1:], nil
}

// ParseMessageFrame parses a message frame in the format a["msg1","msg2",...]
func ParseMessageFrame(data []byte) ([]string, error) {
	if len(data) < 2 {
		return nil, errors.New("invalid message frame: too short")
	}

	// Check if it's a proper array frame
	if data[0] != FrameMessage && data[0] != FrameSingle {
		return nil, fmt.Errorf("invalid message frame type: %c", data[0])
	}

	// For 'm' frames, content is just a single message in JSON string format
	if data[0] == FrameSingle {
		var message string
		if err := json.Unmarshal(data[1:], &message); err != nil {
			return nil, fmt.Errorf("failed to unmarshal single message frame: %w", err)
		}
		return []string{message}, nil
	}

	// For 'a' frames, content is an array of strings
	var messages []string
	if err := json.Unmarshal(data[1:], &messages); err != nil {
		return nil, fmt.Errorf("failed to unmarshal message array: %w", err)
	}
	return messages, nil
}

// ParseCloseFrame parses a close frame in the format c[code,"reason"]
func ParseCloseFrame(data []byte) (CloseInfo, error) {
	if len(data) < 2 {
		return CloseInfo{}, errors.New("invalid close frame: too short")
	}

	if data[0] != FrameClose {
		return CloseInfo{}, fmt.Errorf("invalid close frame type: %c", data[0])
	}

	// Parse JSON array [code,"reason"]
	var closeData []json.RawMessage
	if err := json.Unmarshal(data[1:], &closeData); err != nil {
		return CloseInfo{}, fmt.Errorf("failed to unmarshal close frame: %w", err)
	}

	if len(closeData) != 2 {
		return CloseInfo{}, errors.New("invalid close frame: expected [code,\"reason\"]")
	}

	// Parse code (integer)
	var code int
	if err := json.Unmarshal(closeData[0], &code); err != nil {
		return CloseInfo{}, fmt.Errorf("failed to unmarshal close code: %w", err)
	}

	// Parse reason (string)
	var reason string
	if err := json.Unmarshal(closeData[1], &reason); err != nil {
		return CloseInfo{}, fmt.Errorf("failed to unmarshal close reason: %w", err)
	}

	return CloseInfo{Code: code, Reason: reason}, nil
}

// IsOpenFrame checks if the data is an open frame ("o")
func IsOpenFrame(data []byte) bool {
	return len(data) > 0 && data[0] == FrameOpen
}

// IsHeartbeatFrame checks if the data is a heartbeat frame ("h")
func IsHeartbeatFrame(data []byte) bool {
	return len(data) > 0 && data[0] == FrameHeartbeat
}

// IsMessageFrame checks if the data is a message frame (starts with "a")
func IsMessageFrame(data []byte) bool {
	return len(data) > 0 && data[0] == FrameMessage
}

// IsSingleMessageFrame checks if the data is a single message frame (starts with "m")
func IsSingleMessageFrame(data []byte) bool {
	return len(data) > 0 && data[0] == FrameSingle
}

// IsCloseFrame checks if the data is a close frame (starts with "c")
func IsCloseFrame(data []byte) bool {
	return len(data) > 0 && data[0] == FrameClose
}

// EncodeOpenFrame encodes an open frame
func EncodeOpenFrame() []byte {
	return []byte{'o'}
}

// EncodeHeartbeatFrame encodes a heartbeat frame
func EncodeHeartbeatFrame() []byte {
	return []byte{'h'}
}

// EncodeMessageFrame encodes multiple messages in a message frame
func EncodeMessageFrame(messages []string) ([]byte, error) {
	messagesJSON, err := json.Marshal(messages)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal messages: %w", err)
	}

	result := make([]byte, len(messagesJSON)+1)
	result[0] = FrameMessage
	copy(result[1:], messagesJSON)

	return result, nil
}

// EncodeSingleMessageFrame encodes a single message frame
func EncodeSingleMessageFrame(message string) ([]byte, error) {
	messageJSON, err := json.Marshal(message)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal message: %w", err)
	}

	result := make([]byte, len(messageJSON)+1)
	result[0] = FrameSingle
	copy(result[1:], messageJSON)

	return result, nil
}

// EncodeCloseFrame encodes a close frame with code and reason
func EncodeCloseFrame(ci CloseInfo) ([]byte, error) {
	closeData := []interface{}{ci.Code, ci.Reason}
	closeJSON, err := json.Marshal(closeData)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal close frame: %w", err)
	}

	result := make([]byte, len(closeJSON)+1)
	result[0] = FrameClose
	copy(result[1:], closeJSON)

	return result, nil
}

// ParseFrameString parses a frame from a string representation
func ParseFrameString(data string) (frameType byte, payload string, err error) {
	if len(data) == 0 {
		return 0, "", errors.New("empty frame")
	}
	return data[0], data[1:], nil
}

// ExtractMessages extracts messages from a SockJS protocol message string
// Can handle both array format a["msg1","msg2"] and single message format "msg"
func ExtractMessages(data string) ([]string, error) {
	// Handle empty data
	if len(data) == 0 {
		return nil, errors.New("empty data")
	}

	// Handle array message format: a["msg1","msg2",...]
	if strings.HasPrefix(data, "a[") {
		var messages []string
		// Parse the JSON array part
		if err := json.Unmarshal([]byte(data[1:]), &messages); err != nil {
			return nil, fmt.Errorf("failed to unmarshal message array: %w", err)
		}
		return messages, nil
	}

	// Handle single message format: "msg" (quoted string)
	if (strings.HasPrefix(data, "\"") && strings.HasSuffix(data, "\"")) ||
		(strings.HasPrefix(data, "m\"") && strings.HasSuffix(data, "\"")) {

		startIndex := 0
		if strings.HasPrefix(data, "m") {
			startIndex = 1
		}

		var message string
		if err := json.Unmarshal([]byte(data[startIndex:]), &message); err != nil {
			return nil, fmt.Errorf("failed to unmarshal single message: %w", err)
		}
		return []string{message}, nil
	}

	return nil, fmt.Errorf("unsupported message format: %s", data)
}
