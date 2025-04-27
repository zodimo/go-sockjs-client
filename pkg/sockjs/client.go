package sockjs

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/zodimo/go-sockjs-client/pkg/sockjs/transport"
)

// Default timeout and interval values
const (
	defaultConnectTimeout    = 30 * time.Second
	defaultRequestTimeout    = 10 * time.Second
	defaultMaxReconnectDelay = 30 * time.Second
	defaultHeartbeatInterval = 25 * time.Second
)

// ClientConfig holds configuration options for the SockJS client
type ClientConfig struct {
	// ServerURL is the base URL of the SockJS server
	ServerURL string

	// Transports is an ordered list of transport types to try
	// Valid values: "websocket", "xhr"
	// Default: ["websocket", "xhr"]
	Transports []string

	// ConnectTimeout is the timeout for connection establishment
	ConnectTimeout time.Duration

	// RequestTimeout is the timeout for individual HTTP requests
	RequestTimeout time.Duration

	// MaxReconnectDelay is the maximum delay between reconnection attempts
	MaxReconnectDelay time.Duration

	// HeartbeatInterval is the expected heartbeat interval from the server
	HeartbeatInterval time.Duration

	// Headers contains custom HTTP headers to include in requests
	Headers http.Header

	// Debug enables verbose logging
	Debug bool
}

// ClientV2 implements the Client interface
type ClientV2 struct {
	config    ClientConfig
	transport Transport
	session   *SessionV2
	mu        sync.RWMutex
	closed    chan struct{}
	closeOnce sync.Once
}

// NewClientWithConfig creates a SockJS client with the provided configuration
func NewClientWithConfig(config ClientConfig) (Client, error) {
	// Validate configuration
	if config.ServerURL == "" {
		return nil, errors.New("server URL is required")
	}

	// Set default values if not provided
	if len(config.Transports) == 0 {
		config.Transports = []string{"websocket", "xhr"}
	}
	if config.ConnectTimeout == 0 {
		config.ConnectTimeout = defaultConnectTimeout
	}
	if config.RequestTimeout == 0 {
		config.RequestTimeout = defaultRequestTimeout
	}
	if config.MaxReconnectDelay == 0 {
		config.MaxReconnectDelay = defaultMaxReconnectDelay
	}
	if config.HeartbeatInterval == 0 {
		config.HeartbeatInterval = defaultHeartbeatInterval
	}

	return &ClientV2{
		config: config,
		closed: make(chan struct{}),
	}, nil
}

// Connect establishes a connection to the SockJS server
func (c *ClientV2) Connect(ctx context.Context) (Session, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if already connected
	if c.session != nil && c.session.state == SessionOpen {
		return c.session, nil
	}

	// Try each transport in order
	var lastErr error
	for _, transportType := range c.config.Transports {
		var t transport.Transport
		var err error

		switch transportType {
		case "websocket":
			// Create a new WebSocket transport
			t, err = transport.NewWebSocketTransport(c.config.ServerURL, transport.WithHeaders(c.config.Headers))
		case "xhr":
			// Create a new XHR transport
			t, err = transport.NewXHRTransport(c.config.ServerURL, c.config.Headers)
		default:
			err = fmt.Errorf("unsupported transport type: %s", transportType)
		}

		if err != nil {
			lastErr = err
			continue
		}

		// Create a new session
		session := newSessionV2(generateSessionID(), transportType)

		// Set up the transport handlers
		t.SetMessageHandler(func(msg string) {
			if session.messageHandler != nil {
				// Extract messages from the frame
				messages, err := ExtractMessages(msg)
				if err == nil && len(messages) > 0 {
					for _, message := range messages {
						session.messageHandler(message)
					}
				}
			}
		})

		t.SetErrorHandler(func(err error) {
			if session.errorHandler != nil {
				session.errorHandler(err)
			}
		})

		t.SetCloseHandler(func(code int, reason string) {
			session.state = SessionClosed
			if session.closeHandler != nil {
				session.closeHandler(code, reason)
			}
			close(session.closed)
		})

		// Try to connect with this transport
		connCtx, cancel := context.WithTimeout(ctx, c.config.ConnectTimeout)
		err = t.Connect(connCtx)
		cancel()

		if err != nil {
			lastErr = err
			continue
		}

		// Successfully connected
		session.state = SessionOpen
		session.transport = t
		c.transport = t
		c.session = session
		return session, nil
	}

	// All transports failed
	if lastErr != nil {
		return nil, fmt.Errorf("failed to connect with any transport: %w", lastErr)
	}
	return nil, errors.New("no transport available")
}

// Options returns the client configuration
func (c *ClientV2) Options() *Options {
	// Convert internal config to public Options struct
	return &Options{
		URL:               c.config.ServerURL,
		TransportOrder:    c.config.Transports,
		ConnectTimeout:    c.config.ConnectTimeout,
		HeartbeatInterval: c.config.HeartbeatInterval,
		Headers:           c.config.Headers,
		Debug:             c.config.Debug,
	}
}

// SessionV2 represents a SockJS session
type SessionV2 struct {
	id             string
	transportType  string
	transport      transport.Transport
	state          int
	messageHandler MessageHandler
	errorHandler   ErrorHandler
	closeHandler   func(code int, reason string)
	closed         chan struct{}
	closeOnce      sync.Once
	mu             sync.RWMutex
}

// newSessionV2 creates a new session
func newSessionV2(id string, transportType string) *SessionV2 {
	return &SessionV2{
		id:            id,
		transportType: transportType,
		state:         SessionConnecting,
		closed:        make(chan struct{}),
	}
}

// generateSessionID creates a random session ID
func generateSessionID() string {
	return fmt.Sprintf("%d.%d", time.Now().UnixNano(), time.Now().Unix()%1000)
}

// ID returns the session identifier
func (s *SessionV2) ID() string {
	return s.id
}

// Send sends a message to the server
func (s *SessionV2) Send(message string) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.state != SessionOpen {
		return errors.New("session is not open")
	}

	ctx, cancel := context.WithTimeout(context.Background(), defaultRequestTimeout)
	defer cancel()

	return s.transport.Send(ctx, message)
}

// Close terminates the session
func (s *SessionV2) Close(code int, reason string) error {
	var err error
	s.closeOnce.Do(func() {
		s.mu.Lock()
		s.state = SessionClosing
		transport := s.transport
		s.mu.Unlock()

		if transport != nil {
			ctx, cancel := context.WithTimeout(context.Background(), defaultRequestTimeout)
			err = transport.Close(ctx, code, reason)
			cancel()
		}

		s.mu.Lock()
		s.state = SessionClosed
		s.mu.Unlock()

		close(s.closed)
	})
	return err
}

// State returns the current session state
func (s *SessionV2) State() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state
}

// OnMessage sets a handler for incoming messages
func (s *SessionV2) OnMessage(handler MessageHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messageHandler = handler
}

// OnError sets a handler for session errors
func (s *SessionV2) OnError(handler ErrorHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.errorHandler = handler
}

// OnClose sets a handler for session close events
func (s *SessionV2) OnClose(code int, reason string) {
	s.mu.RLock()
	handler := s.closeHandler
	s.mu.RUnlock()

	if handler != nil {
		handler(code, reason)
	}
}

// Wait blocks until the session is closed
func (s *SessionV2) Wait() error {
	<-s.closed
	return nil
}

// Session returns the current session
func (c *ClientV2) Session() Session {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.session
}

// Close terminates the client connection
func (c *ClientV2) Close() error {
	var err error
	c.closeOnce.Do(func() {
		c.mu.Lock()
		transport := c.transport
		c.mu.Unlock()

		if transport != nil {
			ctx, cancel := context.WithTimeout(context.Background(), c.config.RequestTimeout)
			err = transport.Close(ctx, ErrCodeNormal, "Client closed connection")
			cancel()
		}

		close(c.closed)
	})
	return err
}
