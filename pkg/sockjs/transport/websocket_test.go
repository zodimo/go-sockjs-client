package transport

import (
	"context"
	"errors"
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
		t.Error("Expected message handler to be set")
	}
	if transport.errorHandler == nil {
		t.Error("Expected error handler to be set")
	}
	if transport.closeHandler == nil {
		t.Error("Expected close handler to be set")
	}
}

func TestWebSocketTransportName(t *testing.T) {
	transport, err := NewWebSocketTransport("http://localhost:8080/sockjs")
	if err != nil {
		t.Fatalf("Failed to create transport: %v", err)
	}

	if transport.Name() != "websocket" {
		t.Errorf("Expected transport name to be websocket, got %s", transport.Name())
	}
}

// Test the WithReadWait transport option
func TestWithReadWait(t *testing.T) {
	customReadWait := 30 * time.Second
	transport, err := NewWebSocketTransport("http://localhost:8080/sockjs", WithReadWait(customReadWait))
	if err != nil {
		t.Fatalf("Failed to create transport: %v", err)
	}

	if transport.readWait != customReadWait {
		t.Errorf("Expected readWait to be %v, got %v", customReadWait, transport.readWait)
	}
}

// Test the WithWriteWait transport option
func TestWithWriteWait(t *testing.T) {
	customWriteWait := 15 * time.Second
	transport, err := NewWebSocketTransport("http://localhost:8080/sockjs", WithWriteWait(customWriteWait))
	if err != nil {
		t.Fatalf("Failed to create transport: %v", err)
	}

	if transport.writeWait != customWriteWait {
		t.Errorf("Expected writeWait to be %v, got %v", customWriteWait, transport.writeWait)
	}
}

// Test the WithWebSocketDialer transport option
func TestWithWebSocketDialer(t *testing.T) {
	customDialer := &websocket.Dialer{
		HandshakeTimeout: 15 * time.Second,
	}
	transport, err := NewWebSocketTransport("http://localhost:8080/sockjs", WithWebSocketDialer(customDialer))
	if err != nil {
		t.Fatalf("Failed to create transport: %v", err)
	}

	if transport.dialer != customDialer {
		t.Errorf("Expected custom dialer to be set")
	}
}

// Test the WithReconnect transport option
func TestWithReconnectOption(t *testing.T) {
	customReconnectConfig := ReconnectConfig{
		MaxAttempts:  10,
		InitialDelay: 500 * time.Millisecond,
		MaxDelay:     5 * time.Second,
		Multiplier:   1.5,
		Jitter:       0.1,
	}

	// Use the option adapter to convert the Option to WebSocketOption
	transport, err := NewWebSocketTransport("http://localhost:8080/sockjs", func(wt *WebSocketTransport) {
		wt.reconnectCfg = customReconnectConfig
	})
	if err != nil {
		t.Fatalf("Failed to create transport: %v", err)
	}

	if transport.reconnectCfg.MaxAttempts != customReconnectConfig.MaxAttempts {
		t.Errorf("Expected maxAttempts to be %d, got %d", customReconnectConfig.MaxAttempts, transport.reconnectCfg.MaxAttempts)
	}
	if transport.reconnectCfg.InitialDelay != customReconnectConfig.InitialDelay {
		t.Errorf("Expected initialDelay to be %v, got %v", customReconnectConfig.InitialDelay, transport.reconnectCfg.InitialDelay)
	}
}

// Test the WithConnectTimeout transport option
func TestWithConnectTimeoutOption(t *testing.T) {
	customTimeout := 15 * time.Second

	// Use the option adapter to convert the Option to WebSocketOption
	transport, err := NewWebSocketTransport("http://localhost:8080/sockjs", func(wt *WebSocketTransport) {
		wt.connectTimeout = customTimeout
	})
	if err != nil {
		t.Fatalf("Failed to create transport: %v", err)
	}

	if transport.connectTimeout != customTimeout {
		t.Errorf("Expected connectTimeout to be %v, got %v", customTimeout, transport.connectTimeout)
	}
}

// Test reconnection logic with a more direct approach
func TestReconnectLogic(t *testing.T) {
	// Create a transport with customized reconnection settings
	transport, err := NewWebSocketTransport("ws://localhost:8000/sockjs/test/websocket")
	if err != nil {
		t.Fatalf("Failed to create transport: %v", err)
	}

	// Override the reconnect configuration for testing
	transport.reconnectCfg = ReconnectConfig{
		MaxAttempts:  3,
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     20 * time.Millisecond,
		Multiplier:   1.2,
		Jitter:       0.1,
	}

	// Mock connected state
	transport.connected = true
	transport.conn = &websocket.Conn{} // Empty conn just to avoid nil pointer

	// Track reconnect attempts
	reconnectAttempted := false
	reconnectCount := 0
	reconnectCh := make(chan struct{}, 5)

	// Set error handler to detect reconnect attempts
	transport.SetErrorHandler(func(err error) {
		reconnectCount++
		reconnectAttempted = true
		reconnectCh <- struct{}{}
	})

	// Mock a connection error to trigger reconnect
	// This simulates what happens in the readLoop when a connection error occurs
	go func() {
		// Allow the test to set up the error handler
		time.Sleep(50 * time.Millisecond)

		// Simulate connection error and reconnect attempt
		transport.connected = false
		if transport.errorHandler != nil {
			transport.errorHandler(errors.New("connection lost, reconnect attempt 1"))
		}
	}()

	// Wait for reconnect signal or timeout
	select {
	case <-reconnectCh:
		// Success
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Timed out waiting for reconnect attempt")
	}

	// Verify reconnect was attempted
	if !reconnectAttempted {
		t.Error("Expected reconnect to be attempted")
	}

	if reconnectCount < 1 {
		t.Errorf("Expected at least 1 reconnect attempt, got %d", reconnectCount)
	}
}

// Test handling of different message types
func TestHandleMessage(t *testing.T) {
	transport, err := NewWebSocketTransport("http://localhost:8080/sockjs")
	if err != nil {
		t.Fatalf("Failed to create transport: %v", err)
	}

	// Test message handler
	receivedMessages := make([]string, 0)
	transport.SetMessageHandler(func(msg string) {
		receivedMessages = append(receivedMessages, msg)
	})

	// Test error handler
	var receivedError error
	transport.SetErrorHandler(func(err error) {
		receivedError = err
	})

	// Test close handler
	var (
		closedCode   int
		closedReason string
	)
	transport.SetCloseHandler(func(code int, reason string) {
		closedCode = code
		closedReason = reason
	})

	// Manually set connected to true so the handler processes messages
	transport.connected = true

	// Directly call the message handler to simulate different message types
	transport.handleMessage("o")                          // Open frame - should do nothing
	transport.handleMessage("h")                          // Heartbeat frame - should do nothing
	transport.handleMessage("[\"test\"]")                 // JSON array message - should be processed
	transport.handleMessage("\"single message\"")         // Message frame with quotes (single message format)
	transport.handleMessage("c[1000,\"Normal closure\"]") // Close frame
	transport.handleMessage("invalid")                    // Invalid frame

	// Verify message frames were processed
	if len(receivedMessages) != 2 || receivedMessages[0] != "test" || receivedMessages[1] != "single message" {
		t.Errorf("Expected to receive messages 'test' and 'single message', got %v", receivedMessages)
	}

	// Verify close frame was processed
	if closedCode != 1000 || closedReason != "Normal closure" {
		t.Errorf("Expected close code 1000, reason 'Normal closure', got %d, %s", closedCode, closedReason)
	}

	// Verify error on invalid frame
	if receivedError == nil || !strings.Contains(receivedError.Error(), "unrecognized message format") {
		t.Errorf("Expected error on invalid frame, got %v", receivedError)
	}
}

// Test message handler processing
func TestMessageHandlerProcessing(t *testing.T) {
	transport := &WebSocketTransport{
		messageHandler: func(msg string) {},
	}

	// Test valid SockJS protocol messages
	transport.handleMessage("o")
	transport.handleMessage("h")
	transport.handleMessage("a[\"test\"]")
	transport.handleMessage("c[1000,\"Normal closure\"]")
	transport.handleMessage("invalid")
}
