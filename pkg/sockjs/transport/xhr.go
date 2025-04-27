package transport

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// XHRTransport implements the Transport interface using XHR polling.
type XHRTransport struct {
	serverURL      *url.URL
	sessionID      string
	baseURL        string
	connected      bool
	messageHandler func(string)
	errorHandler   func(error)
	closeHandler   func(int, string)
	client         *http.Client
	headers        http.Header
	sendURL        string
	receiveURL     string
	polling        bool
	pollCancel     context.CancelFunc
	mutex          sync.Mutex
	pollInterval   time.Duration
}

// NewXHRTransport creates a new XHR transport instance.
func NewXHRTransport(serverURL string, headers http.Header) (*XHRTransport, error) {
	parsed, err := url.Parse(serverURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse server URL: %w", err)
	}

	// Generate a random session ID
	sessionID := generateSessionID()

	// Format base URL
	baseURL := serverURL
	if !strings.HasSuffix(baseURL, "/") {
		baseURL += "/"
	}
	baseURL += sessionID

	// Create the XHR URLs
	sendURL := baseURL + "/xhr_send"
	receiveURL := baseURL + "/xhr"

	return &XHRTransport{
		serverURL:    parsed,
		sessionID:    sessionID,
		baseURL:      baseURL,
		sendURL:      sendURL,
		receiveURL:   receiveURL,
		headers:      headers,
		client:       &http.Client{Timeout: 30 * time.Second},
		connected:    false,
		polling:      false,
		pollInterval: 200 * time.Millisecond,
	}, nil
}

// generateSessionID creates a random session ID for the SockJS connection.
func generateSessionID() string {
	// Create a random string of numbers - typical format used by SockJS
	// This is a simplified version; in production you would want more entropy
	return fmt.Sprintf("%d.%d", time.Now().UnixNano(), time.Now().Unix()%1000)
}

// Connect establishes a connection to the SockJS server.
func (x *XHRTransport) Connect(ctx context.Context) error {
	x.mutex.Lock()
	defer x.mutex.Unlock()

	if x.connected {
		return nil // Already connected
	}

	// Create a new request to the receive URL
	req, err := http.NewRequestWithContext(ctx, "POST", x.receiveURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Copy headers
	for k, vv := range x.headers {
		for _, v := range vv {
			req.Header.Add(k, v)
		}
	}

	// Set required headers for XHR transport
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	// Make the request
	resp, err := x.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to connect to SockJS server: %w", err)
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status code: %d, body: %s", resp.StatusCode, string(body))
	}

	// Read and process the initial response which should contain the SockJS open frame
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	// The initial frame should be "o" (open frame)
	if string(body) != "o" {
		return fmt.Errorf("unexpected initial response: %s", string(body))
	}

	// Mark as connected
	x.connected = true

	// Start polling
	x.startPolling(ctx)

	return nil
}

// startPolling begins the XHR polling process.
func (x *XHRTransport) startPolling(ctx context.Context) {
	// Create a cancelable context for polling
	pollCtx, cancel := context.WithCancel(ctx)

	x.mutex.Lock()
	if x.polling {
		x.mutex.Unlock()
		cancel() // Cancel the new context since we're not using it
		return
	}
	x.polling = true
	x.pollCancel = cancel
	x.mutex.Unlock()

	// Start polling in a goroutine
	go func() {
		for {
			select {
			case <-pollCtx.Done():
				return
			default:
				// Poll for new messages
				err := x.poll(pollCtx)
				if err != nil {
					x.mutex.Lock()
					errHandler := x.errorHandler // Get the handler while holding the mutex
					x.mutex.Unlock()

					if errHandler != nil {
						errHandler(err)
					}

					// If we're disconnected, stop polling
					if errors.Is(err, context.Canceled) {
						return
					}

					x.mutex.Lock()
					connected := x.connected
					x.mutex.Unlock()

					if !connected {
						return
					}
				}

				// Sleep briefly before the next poll to avoid hammering the server
				select {
				case <-pollCtx.Done():
					return
				case <-time.After(x.pollInterval):
					// Continue polling
				}
			}
		}
	}()
}

// poll makes a single XHR polling request to receive messages.
func (x *XHRTransport) poll(ctx context.Context) error {
	// Create a new request to the receive URL
	req, err := http.NewRequestWithContext(ctx, "POST", x.receiveURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create poll request: %w", err)
	}

	// Copy headers
	for k, vv := range x.headers {
		for _, v := range vv {
			req.Header.Add(k, v)
		}
	}

	// Set required headers for XHR
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	// Make the request
	resp, err := x.client.Do(req)
	if err != nil {
		// Check if this is due to context cancellation
		if errors.Is(err, context.Canceled) {
			return err
		}
		return fmt.Errorf("failed to poll SockJS server: %w", err)
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status code: %d, body: %s", resp.StatusCode, string(body))
	}

	// Read and process the response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read poll response body: %w", err)
	}

	// Process the message frames
	if len(body) > 0 {
		x.handleMessage(string(body))
	}

	return nil
}

// handleMessage processes a raw message frame from the SockJS server.
func (x *XHRTransport) handleMessage(data string) {
	// Check for heartbeat frame first (most common case)
	if data == "h" {
		// Heartbeat - nothing to do
		return
	}

	x.mutex.Lock()
	if !x.connected || x.messageHandler == nil {
		x.mutex.Unlock()
		return
	}

	// Check for close frame
	if strings.HasPrefix(data, "c[") {
		// Close frame, format: c[code, "reason"]
		var closeFrame []interface{}
		if err := json.Unmarshal([]byte(data[1:]), &closeFrame); err != nil {
			errorHandler := x.errorHandler
			x.mutex.Unlock()

			if errorHandler != nil {
				errorHandler(fmt.Errorf("failed to unmarshal close frame: %w", err))
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

			// Mark as disconnected before calling handler
			x.connected = false

			// Capture handlers and cancel function
			closeHandler := x.closeHandler
			cancelFunc := x.pollCancel

			// Clear state
			if cancelFunc != nil {
				x.pollCancel = nil
				x.polling = false
			}

			x.mutex.Unlock()

			// Call close handler outside mutex
			if closeHandler != nil {
				closeHandler(code, reason)
			}

			// Stop polling outside mutex
			if cancelFunc != nil {
				cancelFunc()
			}

			return
		}
		x.mutex.Unlock()
		return
	}

	// Check for array message
	if strings.HasPrefix(data, "a[") {
		// Array frame, format: a["message1","message2",...]
		var messages []string
		if err := json.Unmarshal([]byte(data[1:]), &messages); err != nil {
			errorHandler := x.errorHandler
			x.mutex.Unlock()

			if errorHandler != nil {
				errorHandler(fmt.Errorf("failed to unmarshal message array: %w", err))
			}
			return
		}

		// Get message handler while holding lock
		messageHandler := x.messageHandler
		x.mutex.Unlock()

		// Process each message outside the lock
		for _, msg := range messages {
			messageHandler(msg)
		}
		return
	}

	// Unrecognized message format
	errorHandler := x.errorHandler
	x.mutex.Unlock()

	if errorHandler != nil {
		errorHandler(fmt.Errorf("unrecognized message format: %s", data))
	}
}

// Send transmits a message to the server via XHR.
func (x *XHRTransport) Send(ctx context.Context, data string) error {
	x.mutex.Lock()
	defer x.mutex.Unlock()

	if !x.connected {
		return errors.New("not connected")
	}

	// Create JSON array containing the message
	jsonMessages, err := json.Marshal([]string{data})
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	// Create a new request to the send URL
	req, err := http.NewRequestWithContext(ctx, "POST", x.sendURL, strings.NewReader(string(jsonMessages)))
	if err != nil {
		return fmt.Errorf("failed to create send request: %w", err)
	}

	// Copy headers
	for k, vv := range x.headers {
		for _, v := range vv {
			req.Header.Add(k, v)
		}
	}

	// Set required headers for XHR
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	// Make the request
	resp, err := x.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send message: %w", err)
	}
	defer resp.Body.Close()

	// Check response status - SockJS XHR_SEND should return 204 No Content
	if resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status code for send: %d, body: %s", resp.StatusCode, string(body))
	}

	return nil
}

// Close terminates the XHR connection.
func (x *XHRTransport) Close(ctx context.Context, code int, reason string) error {
	// Get what we need under lock
	x.mutex.Lock()
	if !x.connected {
		x.mutex.Unlock()
		return nil // Already closed
	}

	// Mark as disconnected and capture what we need
	x.connected = false
	cancelFunc := x.pollCancel
	closeHandler := x.closeHandler
	x.pollCancel = nil
	x.polling = false
	x.mutex.Unlock()

	// Stop polling - do this outside the mutex lock
	if cancelFunc != nil {
		cancelFunc()
	}

	// No explicit close message is required as the server will detect connection loss,
	// but we'll notify our local close handler
	if closeHandler != nil {
		closeHandler(code, reason)
	}

	return nil
}

// SetMessageHandler registers a handler for incoming messages.
func (x *XHRTransport) SetMessageHandler(handler func(string)) {
	x.mutex.Lock()
	defer x.mutex.Unlock()
	x.messageHandler = handler
}

// SetErrorHandler registers a handler for transport errors.
func (x *XHRTransport) SetErrorHandler(handler func(error)) {
	x.mutex.Lock()
	defer x.mutex.Unlock()
	x.errorHandler = handler
}

// SetCloseHandler registers a handler for connection close events.
func (x *XHRTransport) SetCloseHandler(handler func(int, string)) {
	x.mutex.Lock()
	defer x.mutex.Unlock()
	x.closeHandler = handler
}

// Name returns the transport type name.
func (x *XHRTransport) Name() string {
	return "xhr"
}
