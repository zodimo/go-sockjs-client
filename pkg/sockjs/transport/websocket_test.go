package transport

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// setupTestServer creates a test WebSocket server for testing
func setupTestServer(t *testing.T, handler func(w http.ResponseWriter, r *http.Request)) (*httptest.Server, string) {
	t.Helper()

	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/websocket") {
			conn, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				t.Errorf("Failed to upgrade connection: %v", err)
				return
			}
			defer conn.Close()

			// Handle the WebSocket connection
			for {
				_, message, err := conn.ReadMessage()
				if err != nil {
					break
				}

				// Process message and respond
				if handler != nil {
					handler(w, r)
				} else {
					// Echo the message back by default
					if err := conn.WriteMessage(websocket.TextMessage, message); err != nil {
						break
					}
				}
			}
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))

	// Convert http to ws URL
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	wsURL += "/sockjs/test/websocket"

	return server, wsURL
}

func TestNewWebSocketTransport(t *testing.T) {
	// Test with valid URL
	_, err := NewWebSocketTransport("http://localhost:8080/sockjs")
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Test with invalid URL
	_, err = NewWebSocketTransport("://invalid-url")
	if err == nil {
		t.Errorf("Expected error for invalid URL, got nil")
	}

	// Test with options
	headers := http.Header{}
	headers.Add("X-Test", "test-value")
	transport, err := NewWebSocketTransport("http://localhost:8080/sockjs", WithHeaders(headers))
	if err != nil {
		t.Errorf("Expected no error with options, got %v", err)
	}
	if transport.headers.Get("X-Test") != "test-value" {
		t.Errorf("Expected headers to be set, got %v", transport.headers)
	}
}

func TestWebSocketTransportConnect(t *testing.T) {
	server, url := setupTestServer(t, nil)
	defer server.Close()

	transport, err := NewWebSocketTransport(url)
	if err != nil {
		t.Fatalf("Failed to create transport: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = transport.Connect(ctx)
	if err != nil {
		t.Errorf("Expected successful connection, got error: %v", err)
	}

	// Test connecting when already connected
	err = transport.Connect(ctx)
	if err != nil {
		t.Errorf("Expected no error when already connected, got: %v", err)
	}

	// Clean up
	transport.Close(ctx, 1000, "test completed")
}

func TestWebSocketTransportSend(t *testing.T) {
	messageChan := make(chan string, 1)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/websocket") {
			upgrader := websocket.Upgrader{
				CheckOrigin: func(r *http.Request) bool {
					return true
				},
			}

			conn, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				t.Errorf("Failed to upgrade connection: %v", err)
				return
			}
			defer conn.Close()

			// Read the message
			_, message, err := conn.ReadMessage()
			if err != nil {
				t.Errorf("Failed to read message: %v", err)
				return
			}

			// Unmarshal the JSON message (SockJS protocol format)
			if len(message) > 0 {
				// Remove quotes since SockJS sends quoted strings
				msgStr := string(message)
				if strings.HasPrefix(msgStr, "\"") && strings.HasSuffix(msgStr, "\"") {
					msgStr = msgStr[1 : len(msgStr)-1]
				}
				messageChan <- msgStr
			}

			// Echo back as SockJS array format
			conn.WriteMessage(websocket.TextMessage, []byte("[\"response\"]"))
		}
	}))
	defer server.Close()

	// Convert http to ws URL
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	wsURL += "/sockjs/test/websocket"

	transport, err := NewWebSocketTransport(wsURL)
	if err != nil {
		t.Fatalf("Failed to create transport: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Connect to the server
	err = transport.Connect(ctx)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}

	// Send a message
	testMessage := "test message"
	err = transport.Send(ctx, testMessage)
	if err != nil {
		t.Errorf("Failed to send message: %v", err)
	}

	// Verify the message was received by the server
	select {
	case receivedMsg := <-messageChan:
		if receivedMsg != testMessage {
			t.Errorf("Expected message %s, got %s", testMessage, receivedMsg)
		}
	case <-time.After(2 * time.Second):
		t.Error("Timed out waiting for message to be received")
	}

	// Clean up
	transport.Close(ctx, 1000, "test completed")
}

func TestWebSocketTransportClose(t *testing.T) {
	server, url := setupTestServer(t, nil)
	defer server.Close()

	transport, err := NewWebSocketTransport(url)
	if err != nil {
		t.Fatalf("Failed to create transport: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Connect to the server
	err = transport.Connect(ctx)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}

	// Close the connection
	err = transport.Close(ctx, 1000, "test completed")
	if err != nil {
		t.Errorf("Failed to close connection: %v", err)
	}

	// Verify the connection is closed
	if transport.connected {
		t.Error("Transport still marked as connected after close")
	}

	// Test closing an already closed connection (should be a no-op)
	err = transport.Close(ctx, 1000, "already closed")
	if err != nil {
		t.Errorf("Expected no error when closing already closed connection, got: %v", err)
	}
}

func TestWebSocketTransportHandlers(t *testing.T) {
	transport, err := NewWebSocketTransport("http://localhost:8080/sockjs")
	if err != nil {
		t.Fatalf("Failed to create transport: %v", err)
	}

	// Just test that handlers can be set
	transport.SetMessageHandler(func(msg string) {
		// Handler logic would go here in real usage
	})

	transport.SetErrorHandler(func(err error) {
		// Handler logic would go here in real usage
	})

	transport.SetCloseHandler(func(code int, reason string) {
		// Handler logic would go here in real usage
	})

	// Verify handlers are set
	if transport.messageHandler == nil {
		t.Error("Message handler not set")
	}

	if transport.errorHandler == nil {
		t.Error("Error handler not set")
	}

	if transport.closeHandler == nil {
		t.Error("Close handler not set")
	}
}

func TestWebSocketTransportName(t *testing.T) {
	transport, err := NewWebSocketTransport("http://localhost:8080/sockjs")
	if err != nil {
		t.Fatalf("Failed to create transport: %v", err)
	}

	name := transport.Name()
	if name != "websocket" {
		t.Errorf("Expected name 'websocket', got '%s'", name)
	}
}
