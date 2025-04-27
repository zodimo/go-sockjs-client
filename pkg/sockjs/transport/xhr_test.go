package transport

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
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
	// Create a transport with a mocked HTTP client to avoid actual HTTP connections
	transport, err := NewXHRTransport("http://localhost:12345/sockjs", nil)
	if err != nil {
		t.Fatalf("Failed to create transport: %v", err)
	}

	// Disable polling for testing to avoid goroutine leaks and deadlocks
	transport.disablePolling = true

	// Override the HTTP client with a mock that returns a success response
	transport.client = &http.Client{
		Transport: &mockRoundTripper{
			RoundTripFn: func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: 200,
					Body:       io.NopCloser(strings.NewReader("o")),
					Header:     make(http.Header),
				}, nil
			},
		},
	}

	// Set error handler for debugging
	errorChannel := make(chan error, 1)
	transport.SetErrorHandler(func(err error) {
		select {
		case errorChannel <- err:
		default:
			t.Logf("Error channel full: %v", err)
		}
	})

	// Set a message handler to ensure the interface is complete
	transport.SetMessageHandler(func(msg string) {
		t.Logf("Received message: %s", msg)
	})

	// Set a close handler to ensure the interface is complete
	transport.SetCloseHandler(func(code int, reason string) {
		t.Logf("Connection closed: %d, %s", code, reason)
	})

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	// Create test cases for Connect
	testCases := []struct {
		name           string
		setupTransport func()
		expectError    bool
		errorContains  string
	}{
		{
			name: "successful connect",
			setupTransport: func() {
				transport.connected = false
				transport.client = &http.Client{
					Transport: &mockRoundTripper{
						RoundTripFn: func(req *http.Request) (*http.Response, error) {
							return &http.Response{
								StatusCode: 200,
								Body:       io.NopCloser(strings.NewReader("o")),
								Header:     make(http.Header),
							}, nil
						},
					},
				}
			},
			expectError: false,
		},
		{
			name: "already connected",
			setupTransport: func() {
				transport.connected = true
			},
			expectError: false,
		},
		{
			name: "http error",
			setupTransport: func() {
				transport.connected = false
				transport.client = &http.Client{
					Transport: &mockRoundTripper{
						RoundTripFn: func(req *http.Request) (*http.Response, error) {
							return &http.Response{
								StatusCode: 500,
								Body:       io.NopCloser(strings.NewReader("error")),
								Header:     make(http.Header),
							}, nil
						},
					},
				}
			},
			expectError:   true,
			errorContains: "500",
		},
		{
			name: "network error",
			setupTransport: func() {
				transport.connected = false
				transport.client = &http.Client{
					Transport: &mockRoundTripper{
						RoundTripFn: func(req *http.Request) (*http.Response, error) {
							return nil, fmt.Errorf("simulated network error")
						},
					},
				}
			},
			expectError:   true,
			errorContains: "simulated network error",
		},
		{
			name: "invalid response",
			setupTransport: func() {
				transport.connected = false
				transport.client = &http.Client{
					Transport: &mockRoundTripper{
						RoundTripFn: func(req *http.Request) (*http.Response, error) {
							return &http.Response{
								StatusCode: 200,
								Body:       io.NopCloser(strings.NewReader("invalid")),
								Header:     make(http.Header),
							}, nil
						},
					},
				}
			},
			expectError:   true,
			errorContains: "unexpected initial response",
		},
	}

	// Run the test cases
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Set up transport for this test case
			tc.setupTransport()

			// Now connect
			err = transport.Connect(ctx)

			// Check if error status matches expectation
			if tc.expectError && err == nil {
				t.Errorf("Expected error but got nil")
			} else if !tc.expectError && err != nil {
				t.Errorf("Expected no error but got: %v", err)
			}

			// Check error message if applicable
			if tc.expectError && err != nil && tc.errorContains != "" {
				if !strings.Contains(err.Error(), tc.errorContains) {
					t.Errorf("Expected error containing '%s', got: %v", tc.errorContains, err)
				}
			}
		})
	}

	// Verify the base properties are always set
	if transport.sessionID == "" {
		t.Error("Session ID should be set")
	}

	if transport.baseURL == "" || transport.sendURL == "" || transport.receiveURL == "" {
		t.Error("URLs should be set correctly")
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
	// Skip the actual server setup which can cause blocking issues
	// Instead, use a simplified approach with mocked transport

	// Create a transport with basic configuration
	transport, err := NewXHRTransport("http://localhost:12345/sockjs", nil)
	if err != nil {
		t.Fatalf("Failed to create transport: %v", err)
	}

	// Mock the client to return an invalid response
	transport.client = &http.Client{
		Transport: &mockRoundTripper{
			RoundTripFn: func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: 200,
					Body:       io.NopCloser(strings.NewReader("invalid")),
					Header:     make(http.Header),
				}, nil
			},
		},
	}

	// Prevent actual polling goroutines from starting
	transport.polling = true
	transport.pollCancel = func() {}

	// Create a context with a short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Add error handling without channels
	var lastError error
	transport.SetErrorHandler(func(err error) {
		lastError = err
		t.Logf("Received error: %v", err)
	})

	// Try to connect - should fail with invalid response
	err = transport.Connect(ctx)

	// Verify that we got an error
	if err == nil {
		t.Error("Expected error for invalid response, got nil")
	} else {
		t.Logf("Got expected error: %v", err)
	}

	// Verify that the error handler may have been called too
	// This is not always guaranteed as the error might be returned directly
	if lastError != nil {
		t.Logf("Error handler was also called with: %v", lastError)
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
	// Create a simple transport without real HTTP requests
	transport, err := NewXHRTransport("http://localhost:12345/sockjs", nil)
	if err != nil {
		t.Fatalf("Failed to create transport: %v", err)
	}

	// Create channels to collect handler calls
	messageChan := make(chan string, 5)
	closeChan := make(chan struct{}, 1)

	// Set handlers
	transport.SetMessageHandler(func(msg string) {
		select {
		case messageChan <- msg:
		default:
			t.Logf("Message channel full, dropping message: %s", msg)
		}
	})

	transport.SetErrorHandler(func(err error) {
		t.Logf("Error in test: %v", err)
	})

	transport.SetCloseHandler(func(code int, reason string) {
		if code == 3000 && reason == "Test close reason" {
			select {
			case closeChan <- struct{}{}:
			default:
				t.Logf("Close channel full")
			}
		} else {
			t.Errorf("Unexpected close code/reason: %d, %s", code, reason)
		}
	})

	// Simulate a connected state
	transport.connected = true

	// Test message handling - directly call the handler
	transport.handleMessage(`a["test message"]`)

	// Check if message was received with timeout
	select {
	case msg := <-messageChan:
		if msg != "test message" {
			t.Errorf("Expected message 'test message', got '%s'", msg)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Timed out waiting for message")
	}

	// Test heartbeat
	transport.handleMessage("h")
	// No assertions needed for heartbeat

	// Test close frame
	transport.handleMessage(`c[3000,"Test close reason"]`)

	// Check if close was processed
	select {
	case <-closeChan:
		// Success
		if transport.connected {
			t.Error("Transport should be disconnected after close")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Timed out waiting for close")
	}
}

func TestXHRTransportClose(t *testing.T) {
	// Use a direct approach without real servers to avoid blocking

	// Create the transport
	transport, err := NewXHRTransport("http://localhost:12345/sockjs", nil)
	if err != nil {
		t.Fatalf("Failed to create transport: %v", err)
	}

	// Mock the client to avoid any real network calls
	transport.client = &http.Client{
		Transport: &mockRoundTripper{
			RoundTripFn: func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: 204, // No Content
					Body:       io.NopCloser(strings.NewReader("")),
					Header:     make(http.Header),
				}, nil
			},
		},
	}

	// Set up handlers
	closeCalled := false
	transport.SetCloseHandler(func(code int, reason string) {
		closeCalled = true
		if code != 2000 {
			t.Errorf("Expected code 2000, got %d", code)
		}
		if reason != "test closure" {
			t.Errorf("Expected reason 'test closure', got '%s'", reason)
		}
	})

	// Simulate a connected state
	transport.connected = true

	// Set up a mock polling cancellation function
	cancelCalled := false
	transport.polling = true
	transport.pollCancel = func() {
		cancelCalled = true
	}

	// Create a short context for closing
	closeCtx, closeCancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
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
	if !closeCalled {
		t.Error("Close handler was not called")
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

func TestXHRTransportConcurrentMessageHandling(t *testing.T) {
	// Define a server that will simulate multiple concurrent messages
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/xhr") {
			// For the initial connection request
			if r.Header.Get("X-Test-Request") == "initial" {
				w.Header().Set("Content-Type", "application/javascript; charset=UTF-8")
				w.Write([]byte("o"))
				return
			}

			// For subsequent polling requests, return multiple messages
			if r.Header.Get("X-Test-Request") == "messages" {
				w.Header().Set("Content-Type", "application/javascript; charset=UTF-8")
				w.Write([]byte(`a["message1","message2","message3"]`))
				return
			}

			// Default response
			w.Header().Set("Content-Type", "application/javascript; charset=UTF-8")
			w.Write([]byte("o"))
			return
		} else if strings.HasSuffix(r.URL.Path, "/xhr_send") {
			// For send requests
			w.WriteHeader(http.StatusNoContent)
			return
		}
	}))
	defer server.Close()

	// Create the transport
	transport, err := NewXHRTransport(server.URL, http.Header{"X-Test-Request": []string{"initial"}})
	if err != nil {
		t.Fatalf("Failed to create transport: %v", err)
	}

	// Create a channel to collect received messages with sufficient buffer
	messagesChan := make(chan string, 10) // Buffer to avoid blocking

	// Set up message handler with non-blocking send
	transport.SetMessageHandler(func(msg string) {
		select {
		case messagesChan <- msg:
		default:
			t.Logf("Message channel full, dropping message: %s", msg)
		}
	})

	// Set up error handler for debugging
	transport.SetErrorHandler(func(err error) {
		t.Logf("Transport error: %v", err)
	})

	// Simulate a successful connection (without actually connecting to avoid polling issues)
	transport.connected = true

	// Set an explicit timeout for the whole test
	testDone := make(chan struct{})
	go func() {
		// Test concurrent message handling by directly calling handleMessage
		// This tests the internal message handling logic without the polling complexity
		transport.headers.Set("X-Test-Request", "messages")
		transport.handleMessage(`a["message1","message2","message3"]`)

		// Collect messages with a timeout
		receivedMessages := []string{}
		messageCollectionDone := make(chan struct{})

		go func() {
			// Try to collect all 3 messages
			for i := 0; i < 3; i++ {
				select {
				case msg := <-messagesChan:
					receivedMessages = append(receivedMessages, msg)
				case <-time.After(200 * time.Millisecond):
					// Break if we timeout
					break
				}
			}
			close(messageCollectionDone)
		}()

		// Wait for message collection with timeout
		select {
		case <-messageCollectionDone:
			// Proceed with assertions
		case <-time.After(500 * time.Millisecond):
			t.Error("Timed out collecting messages")
		}

		// Verify we received all messages
		if len(receivedMessages) != 3 {
			t.Errorf("Expected 3 messages, got %d", len(receivedMessages))
		}

		// Verify message content
		expectedMessages := []string{"message1", "message2", "message3"}
		for i, expected := range expectedMessages {
			if i < len(receivedMessages) && receivedMessages[i] != expected {
				t.Errorf("Message %d: expected %q, got %q", i, expected, receivedMessages[i])
			}
		}

		// Test error handling with malformed message
		errorChan := make(chan error, 1)
		transport.SetErrorHandler(func(err error) {
			select {
			case errorChan <- err:
			default:
				t.Logf("Error channel full, dropping error: %v", err)
			}
		})

		// Send a malformed message frame
		transport.handleMessage("invalid[frame")

		// Wait for error with timeout
		select {
		case err := <-errorChan:
			// We expect an error for malformed frame
			if err == nil {
				t.Error("Expected error for malformed message, got nil")
			}
		case <-time.After(200 * time.Millisecond):
			t.Error("Timed out waiting for error handler")
		}

		close(testDone)
	}()

	// Set an overall timeout for the entire test
	select {
	case <-testDone:
		// Test completed successfully
	case <-time.After(1 * time.Second):
		t.Fatal("Test timed out")
	}

	// Cleanup any resources
	if transport.pollCancel != nil {
		transport.pollCancel()
	}
}

func TestXHRTransportThreadSafety(t *testing.T) {
	// Create a transport instance with a simulated server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/xhr") {
			w.Header().Set("Content-Type", "application/javascript; charset=UTF-8")
			w.Write([]byte("o"))
			return
		} else if strings.HasSuffix(r.URL.Path, "/xhr_send") {
			w.WriteHeader(http.StatusNoContent)
			return
		}
	}))
	defer server.Close()

	transport, err := NewXHRTransport(server.URL, nil)
	if err != nil {
		t.Fatalf("Failed to create transport: %v", err)
	}

	// Override polling behavior to avoid actual network calls
	transport.polling = true
	transport.pollCancel = func() {}
	transport.connected = true // Simulate connected state

	// Create channels to collect results and synchronize tests
	messagesChan := make(chan string, 100)
	errorsChan := make(chan error, 100)
	wg := sync.WaitGroup{}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	// Set up thread-safe handlers
	var messageHandlerMutex sync.Mutex
	receivedMessages := []string{}

	transport.SetMessageHandler(func(msg string) {
		messageHandlerMutex.Lock()
		defer messageHandlerMutex.Unlock()
		receivedMessages = append(receivedMessages, msg)
		messagesChan <- msg
	})

	transport.SetErrorHandler(func(err error) {
		errorsChan <- err
	})

	// Test concurrent message handling
	wg.Add(10)
	for i := 0; i < 10; i++ {
		go func(index int) {
			defer wg.Done()
			// Simulate receiving different messages concurrently
			msg := fmt.Sprintf(`a["message%d"]`, index)
			transport.handleMessage(msg)
		}(i)
	}

	// Wait for all goroutines to complete
	wg.Wait()

	// Count received messages (should be 10)
	close(messagesChan)
	messageCount := 0
	for range messagesChan {
		messageCount++
	}

	if messageCount != 10 {
		t.Errorf("Expected 10 messages to be processed, got %d", messageCount)
	}

	// Test concurrent sending with a mutex to protect the transport
	wg.Add(5)
	sendErrors := []error{}
	var sendErrorsMutex sync.Mutex

	for i := 0; i < 5; i++ {
		go func(index int) {
			defer wg.Done()
			// We'll manually invoke handleMessage instead of using Send
			// to avoid making actual HTTP requests
			msg := fmt.Sprintf("message%d", index)
			err := transport.Send(ctx, msg)
			if err != nil {
				sendErrorsMutex.Lock()
				sendErrors = append(sendErrors, err)
				sendErrorsMutex.Unlock()
			}
		}(i)
	}

	wg.Wait()

	// Verify no errors during concurrent sends
	sendErrorsMutex.Lock()
	errorCount := len(sendErrors)
	sendErrorsMutex.Unlock()

	if errorCount > 0 {
		t.Errorf("Expected no errors during concurrent sends, got %d errors", errorCount)
		for i, err := range sendErrors {
			t.Errorf("Error %d: %v", i, err)
		}
	}

	// Test concurrent handler setting
	wg.Add(5)
	for i := 0; i < 5; i++ {
		go func(index int) {
			defer wg.Done()
			transport.SetMessageHandler(func(msg string) {
				// Just a dummy handler for testing thread safety
				messagesChan <- fmt.Sprintf("handler%d: %s", index, msg)
			})
		}(i)
	}

	wg.Wait()

	// Clean up
	if transport.pollCancel != nil {
		transport.pollCancel()
	}
}

func TestXHRTransportConnectStable(t *testing.T) {
	// Create a server that returns a valid open frame
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/xhr") {
			w.Header().Set("Content-Type", "application/javascript; charset=UTF-8")
			w.Write([]byte("o"))
			return
		} else if strings.HasSuffix(r.URL.Path, "/xhr_send") {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.Error(w, "Not found", http.StatusNotFound)
	}))
	defer server.Close()

	// Create a transport with a special client
	transport, err := NewXHRTransport(server.URL, nil)
	if err != nil {
		t.Fatalf("Failed to create transport: %v", err)
	}

	// Disable automatic polling for testing
	transport.disablePolling = true

	// We also provide a custom client that only simulates the connection
	// request and doesn't actually make HTTP calls, to avoid any network issues
	transport.client = &http.Client{
		Transport: &mockRoundTripper{
			RoundTripFn: func(req *http.Request) (*http.Response, error) {
				// Only handle the connect request (to /xhr)
				if strings.HasSuffix(req.URL.Path, "/xhr") {
					return &http.Response{
						StatusCode: 200,
						Body:       io.NopCloser(strings.NewReader("o")),
						Header:     make(http.Header),
					}, nil
				}
				return nil, fmt.Errorf("unexpected request URL: %s", req.URL.String())
			},
		},
	}

	// Create a context with a reasonable timeout
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	// Track error channel
	errorChan := make(chan error, 1)

	// Set up handlers
	transport.SetMessageHandler(func(msg string) {
		t.Logf("Message received: %s", msg)
	})
	transport.SetErrorHandler(func(err error) {
		t.Logf("Error received: %v", err)
		select {
		case errorChan <- err:
		default:
			// Don't block if the channel is full
		}
	})
	transport.SetCloseHandler(func(code int, reason string) {
		t.Logf("Close received: %d, %s", code, reason)
	})

	// Connect
	err = transport.Connect(ctx)
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	// Verify connection state
	if !transport.connected {
		t.Error("Transport should be connected")
	}

	// Now test the case where the transport is already connected
	err = transport.Connect(ctx)
	if err != nil {
		t.Errorf("Second Connect call should be a no-op: %v", err)
	}

	// Now test with an invalid response
	transport.connected = false // Reset connection state

	// Create a new context for the invalid response test
	invalidCtx, invalidCancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer invalidCancel()

	// Replace client with one that returns an invalid response
	transport.client = &http.Client{
		Transport: &mockRoundTripper{
			RoundTripFn: func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: 200,
					Body:       io.NopCloser(strings.NewReader("invalid")),
					Header:     make(http.Header),
				}, nil
			},
		},
	}

	// Connect should now fail
	err = transport.Connect(invalidCtx)
	if err == nil {
		t.Error("Connect should fail with invalid response")
	}

	// Finally test with a network error
	transport.connected = false // Reset connection state

	// Create a new context for the network error test
	errorCtx, errorCancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer errorCancel()

	// Replace client with one that returns a network error
	transport.client = &http.Client{
		Transport: &mockRoundTripper{
			RoundTripFn: func(req *http.Request) (*http.Response, error) {
				return nil, fmt.Errorf("simulated network error")
			},
		},
	}

	// Connect should now fail with network error
	err = transport.Connect(errorCtx)
	if err == nil {
		t.Error("Connect should fail with network error")
	}
	if !strings.Contains(err.Error(), "simulated network error") {
		t.Errorf("Expected network error, got: %v", err)
	}
}

func TestXHRTransportConnectComprehensive(t *testing.T) {
	// Create a transport with basic configuration
	transport, err := NewXHRTransport("http://localhost:12345/sockjs", nil)
	if err != nil {
		t.Fatalf("Failed to create transport: %v", err)
	}

	// Disable automatic polling for testing
	transport.disablePolling = true

	// Set up channels to collect events
	messagesChan := make(chan string, 10)
	errorsChan := make(chan error, 10)
	closeChan := make(chan struct{}, 1)

	// Set up handlers
	transport.SetMessageHandler(func(msg string) {
		t.Logf("Message handler received: %s", msg)
		messagesChan <- msg
	})

	transport.SetErrorHandler(func(err error) {
		t.Logf("Error handler received: %v", err)
		errorsChan <- err
	})

	transport.SetCloseHandler(func(code int, reason string) {
		t.Logf("Close handler received: %d, %s", code, reason)
		closeChan <- struct{}{}
	})

	// Set up a controlled test sequence - the mock client will respond with different
	// responses based on the request count and URL pattern
	requestCount := 0

	// Map to track requests to different endpoints
	pollRequestCount := 0

	transport.client = &http.Client{
		Transport: &mockRoundTripper{
			RoundTripFn: func(req *http.Request) (*http.Response, error) {
				// Keep track of request count
				requestCount++
				t.Logf("Request #%d to URL: %s", requestCount, req.URL.String())

				// Handle initial connection request (xhr)
				if strings.HasSuffix(req.URL.Path, "/xhr") {
					// First xhr request is the connection request
					if pollRequestCount == 0 {
						pollRequestCount++
						t.Logf("Returning open frame 'o' for initial connect")
						return &http.Response{
							StatusCode: 200,
							Body:       io.NopCloser(strings.NewReader("o")),
							Header:     make(http.Header),
						}, nil
					}

					// Second xhr request is the first poll - return message
					if pollRequestCount == 1 {
						pollRequestCount++
						t.Logf("Returning message frame 'a[\"hello world\"]' for first poll")
						return &http.Response{
							StatusCode: 200,
							Body:       io.NopCloser(strings.NewReader(`a["hello world"]`)),
							Header:     make(http.Header),
						}, nil
					}

					// Third xhr request is the second poll - return close frame
					if pollRequestCount == 2 {
						pollRequestCount++
						t.Logf("Returning close frame 'c[1000,\"Normal closure\"]' for second poll")
						return &http.Response{
							StatusCode: 200,
							Body:       io.NopCloser(strings.NewReader(`c[1000,"Normal closure"]`)),
							Header:     make(http.Header),
						}, nil
					}

					// Any further xhr requests - return heartbeat
					t.Logf("Returning heartbeat 'h' for subsequent poll")
					return &http.Response{
						StatusCode: 200,
						Body:       io.NopCloser(strings.NewReader("h")),
						Header:     make(http.Header),
					}, nil
				}

				// Handle send request (xhr_send)
				if strings.HasSuffix(req.URL.Path, "/xhr_send") {
					body, _ := io.ReadAll(req.Body)
					t.Logf("Received send request with body: %s", string(body))
					return &http.Response{
						StatusCode: 204,
						Body:       io.NopCloser(strings.NewReader("")),
						Header:     make(http.Header),
					}, nil
				}

				// Unknown endpoint
				t.Logf("Unknown endpoint requested: %s", req.URL.Path)
				return &http.Response{
					StatusCode: 404,
					Body:       io.NopCloser(strings.NewReader("Not found")),
					Header:     make(http.Header),
				}, nil
			},
		},
	}

	// Create a context with timeout for the test
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	// Connect (without starting real polling)
	t.Log("Connecting...")
	err = transport.Connect(ctx)
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	if !transport.connected {
		t.Error("Transport should be connected")
	}

	// Send a message
	t.Log("Sending test message...")
	err = transport.Send(ctx, "test message")
	if err != nil {
		t.Errorf("Send failed: %v", err)
	}

	// Manually invoke poll to simulate receiving messages
	// First poll - should receive a message
	t.Log("Starting first poll - expecting message")
	err = transport.poll(ctx)
	if err != nil {
		t.Errorf("First poll failed: %v", err)
	}

	// Check for received message with longer timeout
	t.Log("Waiting for message")
	select {
	case msg := <-messagesChan:
		t.Logf("Received message: %s", msg)
		if msg != "hello world" {
			t.Errorf("Expected message 'hello world', got '%s'", msg)
		}
	case err := <-errorsChan:
		t.Errorf("Received error instead of message: %v", err)
	case <-time.After(100 * time.Millisecond):
		t.Error("Did not receive expected message")
	}

	// Second poll - should receive a close frame
	t.Log("Starting second poll - expecting close frame")
	err = transport.poll(ctx)
	if err != nil {
		t.Errorf("Second poll failed: %v", err)
	}

	// Check for close event
	t.Log("Waiting for close event")
	select {
	case <-closeChan:
		t.Log("Received close event as expected")
	case err := <-errorsChan:
		t.Errorf("Received error instead of close: %v", err)
	case <-time.After(100 * time.Millisecond):
		t.Error("Did not receive close event")
	}

	// Verify final state
	if transport.connected {
		t.Error("Transport should be disconnected after close frame")
	}
}
