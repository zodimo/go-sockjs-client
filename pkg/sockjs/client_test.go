package sockjs

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/zodimo/go-sockjs-client/pkg/sockjs/transport"
)

// Mock transport implementation for testing
type MockTransport struct {
	ConnectFunc    func(ctx context.Context) error
	SendFunc       func(ctx context.Context, data string) error
	CloseFunc      func(ctx context.Context, code int, reason string) error
	NameFunc       func() string
	messageHandler func(msg string)
	errorHandler   func(err error)
	closeHandler   func(code int, reason string)
}

func (m *MockTransport) Connect(ctx context.Context) error {
	if m.ConnectFunc != nil {
		return m.ConnectFunc(ctx)
	}
	return nil
}

func (m *MockTransport) Send(ctx context.Context, data string) error {
	if m.SendFunc != nil {
		return m.SendFunc(ctx, data)
	}
	return nil
}

func (m *MockTransport) Close(ctx context.Context, code int, reason string) error {
	if m.CloseFunc != nil {
		return m.CloseFunc(ctx, code, reason)
	}
	return nil
}

func (m *MockTransport) Name() string {
	if m.NameFunc != nil {
		return m.NameFunc()
	}
	return "mock"
}

func (m *MockTransport) SetMessageHandler(handler func(msg string)) {
	m.messageHandler = handler
}

func (m *MockTransport) SetErrorHandler(handler func(err error)) {
	m.errorHandler = handler
}

func (m *MockTransport) SetCloseHandler(handler func(code int, reason string)) {
	m.closeHandler = handler
}

func TestNewClientWithConfig(t *testing.T) {
	// Test with minimal config
	config := ClientConfig{
		ServerURL: "http://localhost:8080/sockjs",
	}
	client, err := NewClientWithConfig(config)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if client == nil {
		t.Error("Expected client to be created")
	}

	// Test with invalid config
	invalidConfig := ClientConfig{
		ServerURL: "", // Empty URL, which should cause an error
	}
	client, err = NewClientWithConfig(invalidConfig)
	if err == nil {
		t.Error("Expected error for invalid config, got nil")
	}
	if client != nil {
		t.Error("Expected nil client for invalid config")
	}

	// Test with custom values
	customConfig := ClientConfig{
		ServerURL:         "http://localhost:8080/sockjs",
		Transports:        []string{"xhr", "websocket"},
		ConnectTimeout:    5 * time.Second,
		RequestTimeout:    3 * time.Second,
		MaxReconnectDelay: 10 * time.Second,
		HeartbeatInterval: 10 * time.Second,
		Headers:           http.Header{"X-Test": []string{"test-value"}},
		Debug:             true,
	}
	client, err = NewClientWithConfig(customConfig)
	if err != nil {
		t.Errorf("Expected no error for custom config, got %v", err)
	}
	if client == nil {
		t.Error("Expected client to be created with custom config")
	}

	// Check if options are returned correctly
	options := client.Options()
	if options.URL != customConfig.ServerURL {
		t.Errorf("Expected URL %s, got %s", customConfig.ServerURL, options.URL)
	}
	if options.Debug != customConfig.Debug {
		t.Errorf("Expected Debug %v, got %v", customConfig.Debug, options.Debug)
	}
	if options.Headers.Get("X-Test") != "test-value" {
		t.Errorf("Expected header X-Test: test-value, got %v", options.Headers)
	}
}

// This test will not actually connect to a server, as it would require a running SockJS server
// Instead, we just verify that the Connect method doesn't panic and handles basic input validation
func TestClientConnectValidation(t *testing.T) {
	config := ClientConfig{
		ServerURL: "http://localhost:8080/sockjs", // This URL doesn't exist in test
	}
	client, err := NewClientWithConfig(config)
	if err != nil {
		t.Errorf("Failed to create client: %v", err)
		return
	}

	// We expect this to fail, but not panic
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	_, err = client.Connect(ctx)
	// We expect an error since there's no real server, but we're just testing that the method
	// handles the call properly without panicking
	if err == nil {
		t.Error("Expected error when connecting to non-existent server, got nil")
	}
}

// To test the client with mock transports, we need to modify the client's transport factories
// However, since the transport initialization is internal to the Connect method,
// we'll need to create specialized tests that simulate the behavior instead of directly injecting mocks

func TestClientMessageHandling(t *testing.T) {
	// Create a real client as the base
	config := ClientConfig{
		ServerURL:      "http://localhost:8080/sockjs",
		ConnectTimeout: 1 * time.Second,
	}
	client, err := NewClientWithConfig(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Create a mock session to simulate connection
	mockSession := &SessionV2{
		id:            "test-session",
		transportType: "mock",
		state:         SessionOpen,
		closed:        make(chan struct{}),
	}

	// Set client session for testing
	clientV2, ok := client.(*ClientV2)
	if !ok {
		t.Fatal("Client is not a ClientV2")
	}

	// Manually set the session
	clientV2.session = mockSession

	// Create a message handler to verify message delivery
	receivedMessage := ""
	mockSession.OnMessage(func(msg string) {
		receivedMessage = msg
	})

	// Test message handling by directly invoking handler
	testMessage := "test message"
	if mockSession.messageHandler != nil {
		mockSession.messageHandler(testMessage)
	}

	// Verify the message was received
	if receivedMessage != testMessage {
		t.Errorf("Expected message %q, got %q", testMessage, receivedMessage)
	}
}

func TestClientErrorHandling(t *testing.T) {
	// Create a real client as the base
	config := ClientConfig{
		ServerURL:      "http://localhost:8080/sockjs",
		ConnectTimeout: 1 * time.Second,
	}
	client, err := NewClientWithConfig(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Create a mock session to simulate connection
	mockSession := &SessionV2{
		id:            "test-session",
		transportType: "mock",
		state:         SessionOpen,
		closed:        make(chan struct{}),
	}

	// Set client session for testing
	clientV2, ok := client.(*ClientV2)
	if !ok {
		t.Fatal("Client is not a ClientV2")
	}

	// Manually set the session
	clientV2.session = mockSession

	// Create an error handler to verify error delivery
	var capturedError error
	expectedError := errors.New("test error")
	mockSession.OnError(func(err error) {
		capturedError = err
	})

	// Test error handling by directly invoking handler
	if mockSession.errorHandler != nil {
		mockSession.errorHandler(expectedError)
	}

	// Verify the error was handled
	if capturedError == nil || capturedError.Error() != expectedError.Error() {
		t.Errorf("Expected error %v, got %v", expectedError, capturedError)
	}
}

// Test creating a separate Go transport package mock
type GoMockTransport struct {
	transport.Transport
	connected bool
}

func (m *GoMockTransport) Connect(ctx context.Context) error {
	m.connected = true
	return nil
}

func (m *GoMockTransport) Send(ctx context.Context, data string) error {
	if !m.connected {
		return errors.New("not connected")
	}
	return nil
}

func (m *GoMockTransport) Close(ctx context.Context, code int, reason string) error {
	m.connected = false
	return nil
}

func (m *GoMockTransport) SetMessageHandler(handler func(msg string))            {}
func (m *GoMockTransport) SetErrorHandler(handler func(err error))               {}
func (m *GoMockTransport) SetCloseHandler(handler func(code int, reason string)) {}
func (m *GoMockTransport) Name() string                                          { return "go-mock" }
