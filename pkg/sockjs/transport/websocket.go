// Package transport provides implementations of SockJS transport mechanisms.
package transport

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// WebSocketTransport implements the Transport interface using WebSocket.
type WebSocketTransport struct {
	serverURL         *url.URL
	wsURL             string
	conn              *websocket.Conn
	connected         bool
	messageHandler    func(string)
	errorHandler      func(error)
	closeHandler      func(int, string)
	sendMutex         sync.Mutex
	headers           http.Header
	connectTimeout    time.Duration
	writeWait         time.Duration
	readWait          time.Duration
	reconnectCfg      ReconnectConfig
	dialer            *websocket.Dialer
	receiveChan       chan string
	errorChan         chan error
	closeChan         chan struct{}
	closeOnce         sync.Once
	reconnecting      bool
	currentReconnects int
}

// WebSocketOption configures the WebSocket transport.
type WebSocketOption func(*WebSocketTransport)

// WithHeaders sets custom HTTP headers for the WebSocket connection.
func WithHeaders(headers http.Header) WebSocketOption {
	return func(wt *WebSocketTransport) {
		wt.headers = headers
	}
}

// WithReadWait sets the read deadline for WebSocket connections.
func WithReadWait(readWait time.Duration) WebSocketOption {
	return func(wt *WebSocketTransport) {
		wt.readWait = readWait
	}
}

// WithWriteWait sets the write deadline for WebSocket connections.
func WithWriteWait(writeWait time.Duration) WebSocketOption {
	return func(wt *WebSocketTransport) {
		wt.writeWait = writeWait
	}
}

// WithWebSocketDialer sets a custom WebSocket dialer.
func WithWebSocketDialer(dialer *websocket.Dialer) WebSocketOption {
	return func(wt *WebSocketTransport) {
		wt.dialer = dialer
	}
}

// NewWebSocketTransport creates a new WebSocket transport instance.
func NewWebSocketTransport(serverURL string, options ...WebSocketOption) (*WebSocketTransport, error) {
	parsed, err := url.Parse(serverURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse server URL: %w", err)
	}

	// Convert from SockJS URL to WebSocket URL
	scheme := "ws"
	if parsed.Scheme == "https" {
		scheme = "wss"
	}

	wsURL := url.URL{
		Scheme:   scheme,
		Host:     parsed.Host,
		Path:     strings.TrimSuffix(parsed.Path, "/") + "/websocket",
		RawQuery: parsed.RawQuery,
	}

	wt := &WebSocketTransport{
		serverURL:      parsed,
		wsURL:          wsURL.String(),
		headers:        http.Header{},
		connected:      false,
		connectTimeout: 30 * time.Second,
		writeWait:      10 * time.Second,
		readWait:       60 * time.Second,
		reconnectCfg:   DefaultReconnectConfig(),
		receiveChan:    make(chan string, 100),
		errorChan:      make(chan error, 10),
		closeChan:      make(chan struct{}),
	}

	// Apply options
	for _, option := range options {
		option(wt)
	}

	// Initialize default dialer if not provided
	if wt.dialer == nil {
		wt.dialer = &websocket.Dialer{
			HandshakeTimeout: wt.connectTimeout,
		}
	}

	return wt, nil
}

// Connect establishes a WebSocket connection to the SockJS server.
func (w *WebSocketTransport) Connect(ctx context.Context) error {
	// Reset reconnect counter on new connect
	w.currentReconnects = 0
	return w.connect(ctx)
}

// connect is the internal connect method that handles reconnection logic
func (w *WebSocketTransport) connect(ctx context.Context) error {
	if w.connected {
		return nil // Already connected
	}

	// Create a cancel context with timeout
	connectCtx, cancel := context.WithTimeout(ctx, w.connectTimeout)
	defer cancel()

	// Connect to WebSocket server
	conn, resp, err := w.dialer.DialContext(connectCtx, w.wsURL, w.headers)
	if err != nil {
		statusCode := 0
		if resp != nil {
			statusCode = resp.StatusCode
		}
		return fmt.Errorf("failed to connect to WebSocket server (status: %d): %w", statusCode, err)
	}

	w.conn = conn
	w.connected = true

	// Set connection parameters
	w.conn.SetPingHandler(func(string) error {
		return w.conn.WriteControl(websocket.PongMessage, []byte{}, time.Now().Add(w.writeWait))
	})

	// Start listening for messages
	go w.readLoop()

	return nil
}

// attemptReconnect implements exponential backoff reconnection
func (w *WebSocketTransport) attemptReconnect(ctx context.Context) bool {
	if w.reconnectCfg.MaxAttempts > 0 && w.currentReconnects >= w.reconnectCfg.MaxAttempts {
		return false
	}

	if w.reconnecting {
		return false // Already in a reconnect attempt
	}

	w.reconnecting = true
	defer func() { w.reconnecting = false }()

	w.currentReconnects++

	// Calculate delay with exponential backoff and jitter
	delay := w.reconnectCfg.InitialDelay
	for i := 1; i < w.currentReconnects; i++ {
		delay = time.Duration(float64(delay) * w.reconnectCfg.Multiplier)
		if delay > w.reconnectCfg.MaxDelay {
			delay = w.reconnectCfg.MaxDelay
			break
		}
	}

	// Add jitter to prevent thundering herd
	if w.reconnectCfg.Jitter > 0 {
		jitter := float64(delay) * w.reconnectCfg.Jitter
		delay = time.Duration(float64(delay) + rand.Float64()*jitter - jitter/2)
	}

	// Create a timer for the backoff
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-timer.C:
		// Try to reconnect
		err := w.connect(ctx)
		if err == nil {
			// Successfully reconnected
			return true
		}

		if w.errorHandler != nil {
			w.errorHandler(fmt.Errorf("reconnect attempt %d failed: %w", w.currentReconnects, err))
		}
		return false

	case <-w.closeChan:
		// Transport was closed during the reconnect attempt
		return false

	case <-ctx.Done():
		// Context cancelled
		return false
	}
}

// readLoop continuously reads messages from the WebSocket connection.
func (w *WebSocketTransport) readLoop() {
	defer func() {
		w.connected = false
		if w.conn != nil {
			w.conn.Close()
		}
	}()

	if w.conn == nil {
		return
	}

	w.conn.SetReadDeadline(time.Now().Add(w.readWait))
	w.conn.SetPongHandler(func(string) error {
		w.conn.SetReadDeadline(time.Now().Add(w.readWait))
		return nil
	})

	for {
		_, message, err := w.conn.ReadMessage()
		if err != nil {
			disconnected := true

			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				if w.errorHandler != nil {
					w.errorHandler(err)
				}
			}

			code := websocket.CloseNormalClosure
			reason := "connection closed"
			if closeErr, ok := err.(*websocket.CloseError); ok {
				code = closeErr.Code
				reason = closeErr.Text
			}

			// Try to reconnect unless this is a normal closure
			if code != websocket.CloseNormalClosure && code != websocket.CloseGoingAway {
				// Attempt reconnection
				ctx := context.Background()
				disconnected = !w.attemptReconnect(ctx)
			}

			if disconnected {
				if w.closeHandler != nil {
					w.closeHandler(code, reason)
				}
				break
			}
			continue
		}

		// Process message - SockJS websocket frame is a JSON string or array of strings
		w.handleMessage(string(message))
	}
}

// handleMessage processes a raw WebSocket message according to SockJS protocol.
func (w *WebSocketTransport) handleMessage(data string) {
	if !w.connected || w.messageHandler == nil {
		return
	}

	// Check if it's an array message
	if strings.HasPrefix(data, "[") {
		var messages []string
		if err := json.Unmarshal([]byte(data), &messages); err != nil {
			if w.errorHandler != nil {
				w.errorHandler(fmt.Errorf("failed to unmarshal message array: %w", err))
			}
			return
		}

		for _, msg := range messages {
			w.messageHandler(msg)
		}
		return
	}

	// Handle single message (quotes are part of the SockJS protocol)
	if strings.HasPrefix(data, "\"") && strings.HasSuffix(data, "\"") {
		var message string
		if err := json.Unmarshal([]byte(data), &message); err != nil {
			if w.errorHandler != nil {
				w.errorHandler(fmt.Errorf("failed to unmarshal message: %w", err))
			}
			return
		}
		w.messageHandler(message)
		return
	}

	// Handle special protocol messages
	if data == "h" { // heartbeat
		return
	}

	if strings.HasPrefix(data, "c[") {
		// Close frame, format: c[code, "reason"]
		var closeFrame []interface{}
		if err := json.Unmarshal([]byte(data[1:]), &closeFrame); err != nil {
			if w.errorHandler != nil {
				w.errorHandler(fmt.Errorf("failed to unmarshal close frame: %w", err))
			}
			return
		}

		if len(closeFrame) >= 2 {
			code := 1000 // Normal closure
			if codeFloat, ok := closeFrame[0].(float64); ok {
				code = int(codeFloat)
			}

			reason := ""
			if reasonStr, ok := closeFrame[1].(string); ok {
				reason = reasonStr
			}

			if w.closeHandler != nil {
				w.closeHandler(code, reason)
			}
		}
		return
	}

	// Unrecognized message format
	if w.errorHandler != nil {
		w.errorHandler(fmt.Errorf("unrecognized message format: %s", data))
	}
}

// Send transmits a message to the server via WebSocket.
func (w *WebSocketTransport) Send(ctx context.Context, data string) error {
	if !w.connected || w.conn == nil {
		return errors.New("not connected")
	}

	w.sendMutex.Lock()
	defer w.sendMutex.Unlock()

	// Set write deadline for this message
	writeCtx, cancel := context.WithTimeout(ctx, w.writeWait)
	defer cancel()

	select {
	case <-writeCtx.Done():
		return fmt.Errorf("send timed out: %w", writeCtx.Err())
	default:
		w.conn.SetWriteDeadline(time.Now().Add(w.writeWait))

		// SockJS protocol requires sending messages as JSON strings
		jsonData, err := json.Marshal(data)
		if err != nil {
			return fmt.Errorf("failed to marshal message: %w", err)
		}

		if err := w.conn.WriteMessage(websocket.TextMessage, jsonData); err != nil {
			return fmt.Errorf("failed to send message: %w", err)
		}

		return nil
	}
}

// Close terminates the WebSocket connection.
func (w *WebSocketTransport) Close(ctx context.Context, code int, reason string) error {
	w.closeOnce.Do(func() {
		close(w.closeChan)
	})

	if !w.connected || w.conn == nil {
		return nil // Already closed
	}

	w.sendMutex.Lock()
	defer w.sendMutex.Unlock()

	// Send close message
	message := websocket.FormatCloseMessage(code, reason)
	err := w.conn.WriteControl(websocket.CloseMessage, message, time.Now().Add(w.writeWait))

	// Even if we failed to send the close message, ensure the connection is closed
	w.connected = false
	w.conn.Close()

	if err != nil && !errors.Is(err, websocket.ErrCloseSent) {
		return fmt.Errorf("failed to send close message: %w", err)
	}

	return nil
}

// SetMessageHandler registers a handler for incoming messages.
func (w *WebSocketTransport) SetMessageHandler(handler func(string)) {
	w.messageHandler = handler
}

// SetErrorHandler registers a handler for transport errors.
func (w *WebSocketTransport) SetErrorHandler(handler func(error)) {
	w.errorHandler = handler
}

// SetCloseHandler registers a handler for connection close events.
func (w *WebSocketTransport) SetCloseHandler(handler func(int, string)) {
	w.closeHandler = handler
}

// Name returns the transport type name.
func (w *WebSocketTransport) Name() string {
	return "websocket"
}
