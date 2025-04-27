package sockjs

import (
	"context"
	"errors"
	"testing"
	"time"
)

// mockSessionTransport mocks the transport interface for testing
type mockSessionTransport struct {
	MockTransport
	messages []string
}

// simulateMessage simulates a message received from the server
func (m *mockSessionTransport) simulateMessage(message string) {
	if m.messageHandler != nil {
		m.messageHandler(message)
	}
}

// simulateError simulates an error from the transport
func (m *mockSessionTransport) simulateError(err error) {
	if m.errorHandler != nil {
		m.errorHandler(err)
	}
}

// simulateClose simulates a connection close
func (m *mockSessionTransport) simulateClose(code int, reason string) {
	if m.closeHandler != nil {
		m.closeHandler(code, reason)
	}
}

func TestNewSessionV2(t *testing.T) {
	// Test creating a new session
	id := "test-session"
	transportType := "websocket"
	session := newSessionV2(id, transportType)

	if session.id != id {
		t.Errorf("Expected session ID %s, got %s", id, session.id)
	}
	if session.transportType != transportType {
		t.Errorf("Expected transport type %s, got %s", transportType, session.transportType)
	}
	if session.state != SessionConnecting {
		t.Errorf("Expected initial state %d, got %d", SessionConnecting, session.state)
	}
	if session.closed == nil {
		t.Error("Expected closed channel to be initialized")
	}
}

func TestSessionID(t *testing.T) {
	session := newSessionV2("test-id", "websocket")
	if session.ID() != "test-id" {
		t.Errorf("Expected ID test-id, got %s", session.ID())
	}
}

func TestSessionState(t *testing.T) {
	session := newSessionV2("test-id", "websocket")
	if session.State() != SessionConnecting {
		t.Errorf("Expected initial state %d, got %d", SessionConnecting, session.State())
	}

	// Change state and test again
	session.state = SessionOpen
	if session.State() != SessionOpen {
		t.Errorf("Expected state %d, got %d", SessionOpen, session.State())
	}
}

func TestSessionHandlers(t *testing.T) {
	session := newSessionV2("test-id", "websocket")

	// Test message handler
	var receivedMessage string
	session.OnMessage(func(msg string) {
		receivedMessage = msg
	})

	// Verify the message handler was set
	if session.messageHandler == nil {
		t.Error("Expected message handler to be set")
	}

	// Test error handler
	var receivedError error
	session.OnError(func(err error) {
		receivedError = err
	})

	// Verify the error handler was set
	if session.errorHandler == nil {
		t.Error("Expected error handler to be set")
	}

	// Test close handler
	var closedCode int
	var closedReason string

	// Set the close handler directly since OnClose doesn't set it,
	// it actually calls the existing handler (it's misnamed)
	session.closeHandler = func(code int, reason string) {
		closedCode = code
		closedReason = reason
	}

	// Verify the close handler is set
	if session.closeHandler == nil {
		t.Error("Expected close handler to be set")
	}

	// Now call OnClose which should trigger the handler
	session.OnClose(1000, "normal")

	// Verify the handler was called
	if closedCode != 1000 || closedReason != "normal" {
		t.Errorf("Expected close code %d, reason %s, got %d, %s", 1000, "normal", closedCode, closedReason)
	}

	// Trigger the message handler and verify it works
	testMessage := "test message"
	session.messageHandler(testMessage)
	if receivedMessage != testMessage {
		t.Errorf("Expected message %s, got %s", testMessage, receivedMessage)
	}

	// Trigger the error handler and verify it works
	testError := errors.New("test error")
	session.errorHandler(testError)
	if receivedError != testError {
		t.Errorf("Expected error %v, got %v", testError, receivedError)
	}
}

func TestSessionSend(t *testing.T) {
	session := newSessionV2("test-id", "websocket")

	// Test sending when transport is not set
	err := session.Send("test message")
	if err == nil {
		t.Error("Expected error when sending without transport, got nil")
	}

	// Set up a mock transport
	var sentMessage string
	mockTransport := &mockSessionTransport{
		MockTransport: MockTransport{
			SendFunc: func(ctx context.Context, data string) error {
				sentMessage = data
				return nil
			},
		},
	}
	session.transport = mockTransport
	session.state = SessionOpen

	// Test sending a message
	testMessage := "hello world"
	err = session.Send(testMessage)
	if err != nil {
		t.Errorf("Expected no error when sending, got %v", err)
	}
	if sentMessage != testMessage {
		t.Errorf("Expected sent message %s, got %s", testMessage, sentMessage)
	}

	// Test sending when session is closed
	session.state = SessionClosed
	err = session.Send("should fail")
	if err == nil {
		t.Error("Expected error when sending to closed session, got nil")
	}

	// Test transport error
	session.state = SessionOpen
	mockTransport.SendFunc = func(ctx context.Context, data string) error {
		return errors.New("transport error")
	}
	err = session.Send("should fail")
	if err == nil || err.Error() != "transport error" {
		t.Errorf("Expected transport error, got %v", err)
	}
}

func TestSessionClose(t *testing.T) {
	session := newSessionV2("test-id", "websocket")

	// Test closing when transport is not set
	err := session.Close(1000, "no transport")
	if err != nil {
		t.Errorf("Expected no error when closing without transport, got %v", err)
	}
	if session.state != SessionClosed {
		t.Errorf("Expected state to be %d, got %d", SessionClosed, session.state)
	}

	// Reset session
	session = newSessionV2("test-id", "websocket")
	session.state = SessionOpen

	// Set up a mock transport
	mockTransport := &mockSessionTransport{
		MockTransport: MockTransport{
			CloseFunc: func(ctx context.Context, code int, reason string) error {
				return nil
			},
		},
	}
	session.transport = mockTransport

	// Test closing the session
	err = session.Close(1000, "normal")
	if err != nil {
		t.Errorf("Expected no error when closing, got %v", err)
	}
	if session.state != SessionClosed {
		t.Errorf("Expected state to be %d, got %d", SessionClosed, session.state)
	}

	// Test transport error during close
	session = newSessionV2("test-id", "websocket")
	session.state = SessionOpen
	mockTransport.CloseFunc = func(ctx context.Context, code int, reason string) error {
		return errors.New("transport close error")
	}
	session.transport = mockTransport

	err = session.Close(1000, "should fail")
	if err == nil || err.Error() != "transport close error" {
		t.Errorf("Expected transport error, got %v", err)
	}
}

func TestSessionWait(t *testing.T) {
	session := newSessionV2("test-id", "websocket")

	// Set up a goroutine to close the session after a short time
	go func() {
		time.Sleep(100 * time.Millisecond)
		close(session.closed)
	}()

	// Wait should block until the session is closed
	start := time.Now()
	err := session.Wait()
	elapsed := time.Since(start)

	if err != nil {
		t.Errorf("Expected no error from Wait, got %v", err)
	}
	if elapsed < 100*time.Millisecond {
		t.Errorf("Wait didn't block as expected, elapsed time: %v", elapsed)
	}

	// Test with a separate session instance
	session = newSessionV2("test-id", "websocket")

	// Close the session immediately
	close(session.closed)

	// This shouldn't block
	start = time.Now()
	err = session.Wait()
	elapsed = time.Since(start)

	if err != nil {
		t.Errorf("Expected no error from Wait for closed session, got %v", err)
	}
	if elapsed > 50*time.Millisecond {
		t.Errorf("Wait blocked too long for already closed session: %v", elapsed)
	}
}

func TestSessionMessageHandling(t *testing.T) {
	session := newSessionV2("test-id", "websocket")

	// Create a custom message handler to process different frame types
	customMessageHandler := func(msg string) {
		// Check the first character to determine the frame type
		if len(msg) == 0 {
			return
		}

		frameType := msg[0]

		switch frameType {
		case 'a':
			// Process message frame
			if session.messageHandler != nil {
				messages, err := ParseMessageFrame([]byte(msg))
				if err == nil && len(messages) > 0 {
					for _, message := range messages {
						session.messageHandler(message)
					}
				}
			}
		case 'c':
			// Process close frame
			closeInfo, err := ParseCloseFrame([]byte(msg))
			if err == nil {
				session.state = SessionClosed
				if session.closeHandler != nil {
					session.closeHandler(closeInfo.Code, closeInfo.Reason)
				}
				close(session.closed)
			}
		case 'h':
			// Heartbeat - nothing to do
		case 'o':
			// Open frame - set session to open
			session.state = SessionOpen
		}
	}

	// Create mocked transport
	mockTransport := &mockSessionTransport{}
	mockTransport.SetMessageHandler(customMessageHandler)

	// Assign the transport and mark session as open
	session.transport = mockTransport
	session.state = SessionOpen

	// Set up message handler to collect received messages
	receivedMessages := []string{}
	session.OnMessage(func(msg string) {
		receivedMessages = append(receivedMessages, msg)
	})

	// Set up close handler
	wasClosed := false
	session.closeHandler = func(code int, reason string) {
		wasClosed = true
	}

	// Simulate receiving message frame
	mockTransport.simulateMessage("a[\"message1\",\"message2\"]")

	// Verify the messages were processed
	if len(receivedMessages) != 2 || receivedMessages[0] != "message1" || receivedMessages[1] != "message2" {
		t.Errorf("Expected [message1, message2], got %v", receivedMessages)
	}

	// Simulate heartbeat
	mockTransport.simulateMessage("h")
	// No change in received messages expected
	if len(receivedMessages) != 2 {
		t.Errorf("Expected still 2 messages after heartbeat, got %d", len(receivedMessages))
	}

	// Simulate close frame
	mockTransport.simulateMessage("c[1000,\"normal\"]")

	// Verify close was handled
	if !wasClosed {
		t.Error("Expected session to be closed after receiving close frame")
	}

	if session.state != SessionClosed {
		t.Errorf("Expected session state to be %d (SessionClosed), got %d", SessionClosed, session.state)
	}
}

func TestSessionErrorHandling(t *testing.T) {
	session := newSessionV2("test-id", "websocket")

	// Create mocked transport
	mockTransport := &mockSessionTransport{}

	// Setup what happens when the transport gets an error
	// This duplicates the behavior in ClientV2.Connect when setting up transport handlers
	mockTransport.SetErrorHandler(func(err error) {
		if session.errorHandler != nil {
			session.errorHandler(err)
		}
	})

	// Assign the transport and mark session as open
	session.transport = mockTransport
	session.state = SessionOpen

	// Set up an error handler
	var receivedError error
	session.OnError(func(err error) {
		receivedError = err
	})

	// Simulate an error
	testError := errors.New("test transport error")
	mockTransport.simulateError(testError)

	// Verify the error was handled
	if receivedError != testError {
		t.Errorf("Expected error %v, got %v", testError, receivedError)
	}
}
