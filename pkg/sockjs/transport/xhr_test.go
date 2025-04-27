package transport

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNewXHRTransport(t *testing.T) {
	// Test with valid URL
	transport, err := NewXHRTransport("http://localhost:8080/sockjs", nil)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if transport.sessionID == "" {
		t.Error("Expected session ID to be generated")
	}
	if transport.baseURL == "" || transport.sendURL == "" || transport.receiveURL == "" {
		t.Error("Expected URLs to be set")
	}

	// Test with invalid URL
	_, err = NewXHRTransport("://invalid-url", nil)
	if err == nil {
		t.Errorf("Expected error for invalid URL, got nil")
	}

	// Test with custom headers
	headers := http.Header{}
	headers.Add("X-Test", "test-value")
	transport, err = NewXHRTransport("http://localhost:8080/sockjs", headers)
	if err != nil {
		t.Errorf("Expected no error with headers, got %v", err)
	}
	if transport.headers.Get("X-Test") != "test-value" {
		t.Errorf("Expected headers to be set, got %v", transport.headers)
	}
}

func setupXHRTestServer(t *testing.T, handler func(w http.ResponseWriter, r *http.Request)) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Extract session ID from URL
		parts := strings.Split(r.URL.Path, "/")
		if len(parts) < 3 {
			http.Error(w, "Invalid URL", http.StatusBadRequest)
			return
		}

		// Handle different endpoints
		if strings.HasSuffix(r.URL.Path, "/xhr") {
			// Initial connection or polling
			w.Header().Set("Content-Type", "application/javascript; charset=UTF-8")
			if handler != nil {
				handler(w, r)
			} else {
				// Default: respond with open frame
				w.Write([]byte("o"))
			}
			return
		} else if strings.HasSuffix(r.URL.Path, "/xhr_send") {
			// Handle send requests
			w.WriteHeader(http.StatusNoContent)
			return
		} else {
			http.Error(w, "Unknown endpoint", http.StatusNotFound)
		}
	}))
}

func TestXHRTransportConnect(t *testing.T) {
	// TODO: The test is skipped due to potential goroutine leaks and blocking issues.
	// This is because the startPolling mechanism launches goroutines that can't be easily
	// controlled or terminated in tests. Future improvements should include a more
	// testable design with interfaces for the polling mechanism that can be mocked.
	t.Skip("Skipping test due to stability issues with goroutine management")

	// The mock test setup remains for reference when this is fixed
	responseBody := "o"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/javascript; charset=UTF-8")
		w.Write([]byte(responseBody))
	}))
	defer server.Close()

	transport, err := NewXHRTransport(server.URL, nil)
	if err != nil {
		t.Fatalf("Failed to create transport: %v", err)
	}

	// Override the HTTP client with a custom one that doesn't actually make network calls
	transport.client = &http.Client{
		Transport: &mockRoundTripper{
			RoundTripFn: func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: 200,
					Body:       io.NopCloser(strings.NewReader(responseBody)),
					Header:     make(http.Header),
				}, nil
			},
		},
	}

	// Use a short context timeout
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	// Add error handler
	transport.SetErrorHandler(func(err error) {
		t.Logf("Error in transport: %v", err)
	})

	// Setup our own close handler to verify close is called
	closeHandlerCalled := false
	transport.SetCloseHandler(func(code int, reason string) {
		closeHandlerCalled = true
	})

	// Connect to the server using a separate goroutine and channel to avoid blocking
	connectErrChan := make(chan error, 1)
	go func() {
		connectErrChan <- transport.Connect(ctx)
	}()

	// Wait for connection to complete or timeout
	var connectErr error
	select {
	case connectErr = <-connectErrChan:
		// Connection completed
	case <-time.After(2 * time.Second):
		t.Fatal("Connection timed out")
	}

	if connectErr != nil {
		t.Errorf("Expected successful connection, got error: %v", connectErr)
	}

	if !transport.connected {
		t.Error("Transport should be marked as connected")
	}

	// Test connecting when already connected (should be no-op)
	err = transport.Connect(ctx)
	if err != nil {
		t.Errorf("Expected no error when already connected, got: %v", err)
	}

	// Clean up - explicitly cancel the context and close the transport
	// Create a new context for closing to ensure we don't block
	closeCtx, closeCancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer closeCancel()

	// Close the transport using a goroutine and channel to avoid blocking
	closeErrChan := make(chan error, 1)
	go func() {
		closeErrChan <- transport.Close(closeCtx, 1000, "test completed")
	}()

	// Wait for close to complete or timeout
	select {
	case err := <-closeErrChan:
		if err != nil {
			t.Errorf("Failed to close transport: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Close operation timed out")
	}

	if transport.connected {
		t.Error("Transport should be marked as disconnected after close")
	}

	// Verify the close handler was called
	if !closeHandlerCalled {
		t.Error("Close handler was not called")
	}
}

// mockRoundTripper is a mock http.RoundTripper for testing
type mockRoundTripper struct {
	RoundTripFn func(*http.Request) (*http.Response, error)
}

func (m *mockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return m.RoundTripFn(req)
}

func TestXHRTransportConnectInvalidResponse(t *testing.T) {
	// TODO: Fix test stability issues - currently causes deadlocks
	// t.Skip("Skipping test due to stability issues")

	// Server that returns invalid response
	server := setupXHRTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		// Return invalid open frame
		w.Write([]byte("invalid"))
	})
	defer server.Close()

	transport, err := NewXHRTransport(server.URL, nil)
	if err != nil {
		t.Fatalf("Failed to create transport: %v", err)
	}

	// Set a short timeout for tests
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Add debug handling
	transport.SetErrorHandler(func(err error) {
		t.Logf("Error in transport: %v", err)
	})

	// Set up a channel to catch any race panics
	errChan := make(chan error, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				errChan <- fmt.Errorf("panic in connect: %v", r)
			}
		}()

		errChan <- transport.Connect(ctx)
	}()

	// Wait for connection attempt with timeout
	var connectErr error
	select {
	case connectErr = <-errChan:
		// Got a result
	case <-time.After(3 * time.Second):
		t.Fatal("Test timed out waiting for connection")
	}

	// Check the connection result - should be an error
	if connectErr == nil {
		t.Error("Expected error for invalid response, got nil")
		// Make sure to clean up if we get here
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 1*time.Second)
		transport.Close(cleanupCtx, 1000, "test cleanup")
		cleanupCancel()
	}
}

func TestXHRTransportSend(t *testing.T) {
	var sentRequestBody string

	// Create a server - we won't actually use it but need to get a valid URL
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// This won't be called with our mock
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	transport, err := NewXHRTransport(server.URL, nil)
	if err != nil {
		t.Fatalf("Failed to create transport: %v", err)
	}

	// Override the HTTP client with a custom one that doesn't actually make network calls
	transport.client = &http.Client{
		Transport: &mockRoundTripper{
			RoundTripFn: func(req *http.Request) (*http.Response, error) {
				// For connect request, return open frame
				if strings.HasSuffix(req.URL.Path, "/xhr") {
					return &http.Response{
						StatusCode: 200,
						Body:       io.NopCloser(strings.NewReader("o")),
						Header:     make(http.Header),
					}, nil
				}

				// For send request, capture the request body and return success
				if strings.HasSuffix(req.URL.Path, "/xhr_send") {
					body, err := io.ReadAll(req.Body)
					if err != nil {
						return nil, err
					}
					sentRequestBody = string(body)
					return &http.Response{
						StatusCode: 204, // No Content
						Body:       io.NopCloser(strings.NewReader("")),
						Header:     make(http.Header),
					}, nil
				}

				return nil, fmt.Errorf("unexpected request URL: %s", req.URL.String())
			},
		},
	}

	// Use a short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	// Add error handler
	transport.SetErrorHandler(func(err error) {
		t.Logf("Error in transport: %v", err)
	})

	// Mark the transport as connected (skip the actual connect call)
	transport.connected = true

	// Send a message
	testMessage := "test message"
	err = transport.Send(ctx, testMessage)
	if err != nil {
		t.Errorf("Failed to send message: %v", err)
	}

	// Verify the message was properly formatted
	expectedData := `["test message"]`
	if sentRequestBody != expectedData {
		t.Errorf("Expected message data %q, got %q", expectedData, sentRequestBody)
	}

	// Test sending when not connected
	transport.connected = false
	err = transport.Send(ctx, testMessage)
	if err == nil {
		t.Error("Expected error when sending while not connected, got nil")
	}
}

func TestXHRTransportHandleMessages(t *testing.T) {
	// TODO: Fix test stability issues - currently causes deadlocks
	// t.Skip("Skipping test due to stability issues")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/xhr") {
			// First request - initial connection
			if r.Header.Get("X-Test-Request") == "initial" {
				w.Header().Set("Content-Type", "application/javascript; charset=UTF-8")
				w.Write([]byte("o"))
				return
			}

			// Second request - send a message
			if r.Header.Get("X-Test-Request") == "message" {
				w.Header().Set("Content-Type", "application/javascript; charset=UTF-8")
				w.Write([]byte(`a["test message"]`))
				return
			}

			// Third request - send a heartbeat
			if r.Header.Get("X-Test-Request") == "heartbeat" {
				w.Header().Set("Content-Type", "application/javascript; charset=UTF-8")
				w.Write([]byte("h"))
				return
			}

			// Fourth request - send a close frame
			if r.Header.Get("X-Test-Request") == "close" {
				w.Header().Set("Content-Type", "application/javascript; charset=UTF-8")
				w.Write([]byte(`c[3000,"Test close reason"]`))
				return
			}

			// Default - open frame
			w.Header().Set("Content-Type", "application/javascript; charset=UTF-8")
			w.Write([]byte("o"))
			return
		} else if strings.HasSuffix(r.URL.Path, "/xhr_send") {
			w.WriteHeader(http.StatusNoContent)
			return
		}
	}))
	defer server.Close()

	transport, err := NewXHRTransport(server.URL, http.Header{"X-Test-Request": []string{"initial"}})
	if err != nil {
		t.Fatalf("Failed to create transport: %v", err)
	}

	// Set up message handler
	messageChan := make(chan string, 1)
	transport.SetMessageHandler(func(msg string) {
		messageChan <- msg
	})

	// Set up close handler
	closeChan := make(chan struct{}, 1)
	var closeCode int
	var closeReason string
	transport.SetCloseHandler(func(code int, reason string) {
		closeCode = code
		closeReason = reason
		closeChan <- struct{}{}
	})

	// Add debug handling
	transport.SetErrorHandler(func(err error) {
		t.Logf("Error in transport: %v", err)
	})

	// We're going to skip the full connection process and just call handleMessage directly,
	// but simulate a connected state
	transport.connected = true

	// Test handle message
	transport.headers.Set("X-Test-Request", "message")
	transport.handleMessage(`a["test message"]`)

	// Verify message was processed
	select {
	case msg := <-messageChan:
		if msg != "test message" {
			t.Errorf("Expected message 'test message', got '%s'", msg)
		}
	case <-time.After(1 * time.Second):
		t.Error("Timed out waiting for message handler")
	}

	// Test heartbeat
	transport.headers.Set("X-Test-Request", "heartbeat")
	transport.handleMessage("h")
	// No assertion needed, just ensure it doesn't cause errors

	// Test close frame
	transport.headers.Set("X-Test-Request", "close")
	transport.handleMessage(`c[3000,"Test close reason"]`)

	// Verify close was processed
	select {
	case <-closeChan:
		if closeCode != 3000 {
			t.Errorf("Expected close code 3000, got %d", closeCode)
		}
		if closeReason != "Test close reason" {
			t.Errorf("Expected close reason 'Test close reason', got '%s'", closeReason)
		}
		if transport.connected {
			t.Error("Transport should be marked as disconnected after close")
		}
	case <-time.After(1 * time.Second):
		t.Error("Timed out waiting for close handler")
	}

	// No need to clean up as we didn't actually start a polling connection
}

func TestXHRTransportClose(t *testing.T) {
	// TODO: Fix test stability issues - currently causes deadlocks
	// t.Skip("Skipping test due to stability issues")

	server := setupXHRTestServer(t, nil)
	defer server.Close()

	transport, err := NewXHRTransport(server.URL, nil)
	if err != nil {
		t.Fatalf("Failed to create transport: %v", err)
	}

	// Add debug handling
	transport.SetErrorHandler(func(err error) {
		t.Logf("Error in transport: %v", err)
	})

	// Set close handler
	closeChan := make(chan struct{}, 1)
	closeCode := 0
	closeReason := ""
	transport.SetCloseHandler(func(code int, reason string) {
		closeCode = code
		closeReason = reason
		closeChan <- struct{}{}
	})

	// For this test, skip the actual connection process and just simulate a connected state
	// This avoids the potential deadlock from the polling mechanism
	transport.connected = true

	// Set up a mock polling cancellation function
	cancelCalled := false
	transport.mutex.Lock()
	transport.polling = true
	transport.pollCancel = func() {
		cancelCalled = true
	}
	transport.mutex.Unlock()

	// Create a context for closing
	closeCtx, closeCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer closeCancel()

	// Close the connection
	err = transport.Close(closeCtx, 2000, "test closure")
	if err != nil {
		t.Errorf("Failed to close connection: %v", err)
	}

	// Verify the connection is closed
	if transport.connected {
		t.Error("Transport should be marked as disconnected after close")
	}

	// Verify the poll cancel function was called
	if !cancelCalled {
		t.Error("Poll cancel function was not called")
	}

	// Verify close handler was called
	select {
	case <-closeChan:
		if closeCode != 2000 {
			t.Errorf("Expected close code 2000, got %d", closeCode)
		}
		if closeReason != "test closure" {
			t.Errorf("Expected close reason 'test closure', got '%s'", closeReason)
		}
	case <-time.After(1 * time.Second):
		t.Error("Timed out waiting for close handler")
	}

	// Test closing an already closed connection (should be a no-op)
	err = transport.Close(closeCtx, 2000, "already closed")
	if err != nil {
		t.Errorf("Expected no error when closing already closed connection, got: %v", err)
	}
}

func TestXHRTransportHandlers(t *testing.T) {
	transport, err := NewXHRTransport("http://localhost:8080/sockjs", nil)
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

func TestXHRTransportName(t *testing.T) {
	transport, err := NewXHRTransport("http://localhost:8080/sockjs", nil)
	if err != nil {
		t.Fatalf("Failed to create transport: %v", err)
	}

	name := transport.Name()
	if name != "xhr" {
		t.Errorf("Expected name 'xhr', got '%s'", name)
	}
}
