package sockjs

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestParseFrame(t *testing.T) {
	tests := []struct {
		name          string
		data          []byte
		wantFrameType byte
		wantPayload   []byte
		wantErr       bool
	}{
		{
			name:          "open frame",
			data:          []byte("o"),
			wantFrameType: 'o',
			wantPayload:   []byte{},
			wantErr:       false,
		},
		{
			name:          "heartbeat frame",
			data:          []byte("h"),
			wantFrameType: 'h',
			wantPayload:   []byte{},
			wantErr:       false,
		},
		{
			name:          "close frame",
			data:          []byte("c[3000,\"Go away!\"]"),
			wantFrameType: 'c',
			wantPayload:   []byte("[3000,\"Go away!\"]"),
			wantErr:       false,
		},
		{
			name:          "message frame",
			data:          []byte("a[\"Hello, world!\"]"),
			wantFrameType: 'a',
			wantPayload:   []byte("[\"Hello, world!\"]"),
			wantErr:       false,
		},
		{
			name:          "empty frame",
			data:          []byte{},
			wantFrameType: 0,
			wantPayload:   nil,
			wantErr:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotFrameType, gotPayload, err := ParseFrame(tt.data)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseFrame() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if gotFrameType != tt.wantFrameType {
				t.Errorf("ParseFrame() gotFrameType = %v, want %v", gotFrameType, tt.wantFrameType)
			}
			if !reflect.DeepEqual(gotPayload, tt.wantPayload) {
				t.Errorf("ParseFrame() gotPayload = %v, want %v", gotPayload, tt.wantPayload)
			}
		})
	}
}

func TestParseMessageFrame(t *testing.T) {
	tests := []struct {
		name         string
		data         []byte
		wantMessages []string
		wantErr      bool
	}{
		{
			name:         "valid message array",
			data:         []byte("a[\"Hello\",\"World\"]"),
			wantMessages: []string{"Hello", "World"},
			wantErr:      false,
		},
		{
			name:         "valid single message",
			data:         []byte("m\"Hello\""),
			wantMessages: []string{"Hello"},
			wantErr:      false,
		},
		{
			name:         "empty message array",
			data:         []byte("a[]"),
			wantMessages: []string{},
			wantErr:      false,
		},
		{
			name:         "invalid format",
			data:         []byte("x[\"Hello\"]"),
			wantMessages: nil,
			wantErr:      true,
		},
		{
			name:         "invalid json",
			data:         []byte("a[\"Hello"),
			wantMessages: nil,
			wantErr:      true,
		},
		{
			name:         "too short",
			data:         []byte("a"),
			wantMessages: nil,
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotMessages, err := ParseMessageFrame(tt.data)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseMessageFrame() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(gotMessages, tt.wantMessages) {
				t.Errorf("ParseMessageFrame() = %v, want %v", gotMessages, tt.wantMessages)
			}
		})
	}
}

func TestParseCloseFrame(t *testing.T) {
	tests := []struct {
		name      string
		data      []byte
		wantFrame CloseInfo
		wantErr   bool
	}{
		{
			name:      "valid close frame",
			data:      []byte("c[3000,\"Go away!\"]"),
			wantFrame: CloseInfo{Code: 3000, Reason: "Go away!"},
			wantErr:   false,
		},
		{
			name:      "empty reason",
			data:      []byte("c[2000,\"\"]"),
			wantFrame: CloseInfo{Code: 2000, Reason: ""},
			wantErr:   false,
		},
		{
			name:      "invalid format",
			data:      []byte("x[3000,\"Go away!\"]"),
			wantFrame: CloseInfo{},
			wantErr:   true,
		},
		{
			name:      "invalid json",
			data:      []byte("c[3000,\"Go away!\""),
			wantFrame: CloseInfo{},
			wantErr:   true,
		},
		{
			name:      "too short",
			data:      []byte("c"),
			wantFrame: CloseInfo{},
			wantErr:   true,
		},
		{
			name:      "wrong array length",
			data:      []byte("c[3000]"),
			wantFrame: CloseInfo{},
			wantErr:   true,
		},
		{
			name:      "invalid code type",
			data:      []byte("c[\"3000\",\"Go away!\"]"),
			wantFrame: CloseInfo{},
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotFrame, err := ParseCloseFrame(tt.data)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseCloseFrame() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && !reflect.DeepEqual(gotFrame, tt.wantFrame) {
				t.Errorf("ParseCloseFrame() = %v, want %v", gotFrame, tt.wantFrame)
			}
		})
	}
}

func TestEncodeMessageFrame(t *testing.T) {
	tests := []struct {
		name     string
		messages []string
		wantErr  bool
	}{
		{
			name:     "multiple messages",
			messages: []string{"Hello", "World"},
			wantErr:  false,
		},
		{
			name:     "single message",
			messages: []string{"Hello"},
			wantErr:  false,
		},
		{
			name:     "empty message array",
			messages: []string{},
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded, err := EncodeMessageFrame(tt.messages)
			if (err != nil) != tt.wantErr {
				t.Errorf("EncodeMessageFrame() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				// Verify the frame type
				if encoded[0] != FrameMessage {
					t.Errorf("EncodeMessageFrame() frame type = %c, want %c", encoded[0], FrameMessage)
				}

				// Verify the payload can be parsed back
				var decoded []string
				err = json.Unmarshal(encoded[1:], &decoded)
				if err != nil {
					t.Errorf("EncodeMessageFrame() produced invalid JSON: %v", err)
				}

				if !reflect.DeepEqual(decoded, tt.messages) {
					t.Errorf("EncodeMessageFrame() decoded = %v, want %v", decoded, tt.messages)
				}
			}
		})
	}
}

func TestEncodeCloseFrame(t *testing.T) {
	tests := []struct {
		name      string
		closeInfo CloseInfo
		wantErr   bool
	}{
		{
			name:      "normal close",
			closeInfo: CloseInfo{Code: 2000, Reason: "Normal closure"},
			wantErr:   false,
		},
		{
			name:      "error close",
			closeInfo: CloseInfo{Code: 3000, Reason: "Server error"},
			wantErr:   false,
		},
		{
			name:      "empty reason",
			closeInfo: CloseInfo{Code: 2000, Reason: ""},
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded, err := EncodeCloseFrame(tt.closeInfo)
			if (err != nil) != tt.wantErr {
				t.Errorf("EncodeCloseFrame() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				// Verify the frame type
				if encoded[0] != FrameClose {
					t.Errorf("EncodeCloseFrame() frame type = %c, want %c", encoded[0], FrameClose)
				}

				// Verify the payload can be parsed back
				var decoded []interface{}
				err = json.Unmarshal(encoded[1:], &decoded)
				if err != nil {
					t.Errorf("EncodeCloseFrame() produced invalid JSON: %v", err)
				}

				if len(decoded) != 2 {
					t.Errorf("EncodeCloseFrame() decoded length = %d, want 2", len(decoded))
				} else {
					if int(decoded[0].(float64)) != tt.closeInfo.Code {
						t.Errorf("EncodeCloseFrame() code = %v, want %v", decoded[0], tt.closeInfo.Code)
					}
					if decoded[1] != tt.closeInfo.Reason {
						t.Errorf("EncodeCloseFrame() reason = %v, want %v", decoded[1], tt.closeInfo.Reason)
					}
				}
			}
		})
	}
}

func TestExtractMessages(t *testing.T) {
	tests := []struct {
		name         string
		data         string
		wantMessages []string
		wantErr      bool
	}{
		{
			name:         "array message format",
			data:         "a[\"Hello\",\"World\"]",
			wantMessages: []string{"Hello", "World"},
			wantErr:      false,
		},
		{
			name:         "single message format with m prefix",
			data:         "m\"Hello\"",
			wantMessages: []string{"Hello"},
			wantErr:      false,
		},
		{
			name:         "single message format without prefix",
			data:         "\"Hello\"",
			wantMessages: []string{"Hello"},
			wantErr:      false,
		},
		{
			name:         "empty array",
			data:         "a[]",
			wantMessages: []string{},
			wantErr:      false,
		},
		{
			name:         "empty data",
			data:         "",
			wantMessages: nil,
			wantErr:      true,
		},
		{
			name:         "unsupported format",
			data:         "x\"Hello\"",
			wantMessages: nil,
			wantErr:      true,
		},
		{
			name:         "invalid json in array",
			data:         "a[\"Hello",
			wantMessages: nil,
			wantErr:      true,
		},
		{
			name:         "invalid json in single message",
			data:         "\"Hello",
			wantMessages: nil,
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotMessages, err := ExtractMessages(tt.data)
			if (err != nil) != tt.wantErr {
				t.Errorf("ExtractMessages() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(gotMessages, tt.wantMessages) {
				t.Errorf("ExtractMessages() = %v, want %v", gotMessages, tt.wantMessages)
			}
		})
	}
}
