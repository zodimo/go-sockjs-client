// Package sockjs provides a client implementation of the SockJS protocol.
package sockjs

import (
	"context"
	"net/http"
	"sync"
	"time"
)

// Protocol constants based on SockJS specification
const (
	// Framing format
	OpenFrame        = "o"
	HeartbeatFrame   = "h"
	CloseFrame       = "c"
	ArrayFrameStart  = "a"
	ArrayFrameEnd    = "]"
	MessageFrameChar = "m"

	// Default timeout values
	DefaultConnectTimeout    = 30 * time.Second
	DefaultHeartbeatInterval = 25 * time.Second

	// Session states
	SessionConnecting = 0
	SessionOpen       = 1
	SessionClosing    = 2
	SessionClosed     = 3
)

// Protocol error codes
const (
	// 2000-2999: All protocol errors
	ErrCodeNormal           = 2000 // Normal closure
	ErrCodeGoingAway        = 2001 // Server is going away
	ErrCodeProtocolError    = 2002 // Protocol error
	ErrCodeUnsupportedData  = 2003 // Unsupported data
	ErrCodeReserved         = 2004 // Reserved error
	ErrCodeNoStatus         = 2005 // No status received
	ErrCodeAbnormalClosure  = 2006 // Abnormal closure
	ErrCodeInvalidFrameData = 2007 // Invalid frame payload data
	ErrCodePolicyViolation  = 2008 // Policy violation
	ErrCodeMessageTooBig    = 2009 // Message too big
	ErrCodeExtensionMissing = 2010 // Extension required

	// 3000-3999: Custom status codes
	ErrCodeServerError    = 3000 // Server error
	ErrCodeClientError    = 3001 // Client error
	ErrCodeTransportError = 3002 // Transport error
	ErrCodeTimeout        = 3003 // Connection timeout
)

// SockJSError represents an error that occurred during SockJS operations
type SockJSError struct {
	Code    int
	Message string
	Err     error
}

// Error returns the error message
func (e *SockJSError) Error() string {
	if e.Err != nil {
		return e.Message + ": " + e.Err.Error()
	}
	return e.Message
}

// Unwrap returns the underlying error
func (e *SockJSError) Unwrap() error {
	return e.Err
}

// Configuration options for SockJS client
type Options struct {
	// Server URL prefix
	URL string

	// HTTP client used for XHR transport
	HTTPClient *http.Client

	// Connection timeout
	ConnectTimeout time.Duration

	// Server heartbeat interval
	HeartbeatInterval time.Duration

	// Transport preferences in order of priority
	TransportOrder []string

	// Extra HTTP headers to include in requests
	Headers http.Header

	// Debug mode enables verbose logging
	Debug bool
}

// MessageHandler is a function that handles incoming SockJS messages
type MessageHandler func(message string)

// ErrorHandler is a function that handles SockJS connection errors
type ErrorHandler func(err error)

// Transport defines the interface for different SockJS transports (WebSocket, XHR, etc.)
type Transport interface {
	// Connect establishes the transport connection
	Connect(ctx context.Context) error

	// Send sends data through the transport
	Send(ctx context.Context, data string) error

	// Close terminates the transport connection
	Close(ctx context.Context, code int, reason string) error

	// Name returns the transport name
	Name() string
}

// Session represents a SockJS session
type Session interface {
	// ID returns the session identifier
	ID() string

	// Send sends a message to the server
	Send(message string) error

	// Close terminates the session connection
	Close(code int, reason string) error

	// State returns the current session state
	State() int

	// OnMessage sets a handler for incoming messages
	OnMessage(handler MessageHandler)

	// OnError sets a handler for session errors
	OnError(handler ErrorHandler)

	// OnClose sets a handler for session close events
	OnClose(code int, reason string)

	// Wait blocks until the session is closed
	Wait() error
}

// Client represents a SockJS client
type Client interface {
	// Connect establishes a connection to the SockJS server
	Connect(ctx context.Context) (Session, error)

	// Options returns the client options
	Options() *Options
}

// ClientImpl implements the Client interface
type ClientImpl struct {
	options *Options
	mu      sync.Mutex
}

// NewClient creates a new SockJS client with the given options
func NewClient(opts *Options) Client {
	// Set default options if not provided
	if opts.ConnectTimeout == 0 {
		opts.ConnectTimeout = DefaultConnectTimeout
	}
	if opts.HeartbeatInterval == 0 {
		opts.HeartbeatInterval = DefaultHeartbeatInterval
	}
	if opts.HTTPClient == nil {
		opts.HTTPClient = http.DefaultClient
	}
	if opts.TransportOrder == nil {
		// Default transport order: WebSocket, XHR
		opts.TransportOrder = []string{"websocket", "xhr"}
	}

	return &ClientImpl{
		options: opts,
	}
}

// Options returns the client configuration options
func (c *ClientImpl) Options() *Options {
	return c.options
}

// Connect establishes a connection to the SockJS server
// This is a stub that will be implemented in the next task
func (c *ClientImpl) Connect(ctx context.Context) (Session, error) {
	return nil, &SockJSError{
		Code:    ErrCodeClientError,
		Message: "Connect method not implemented yet",
	}
}

// SessionImpl implements the Session interface
type SessionImpl struct {
	id             string
	client         *ClientImpl
	transport      Transport
	state          int
	messageQueue   []string
	messageHandler MessageHandler
	errorHandler   ErrorHandler
	closeHandler   func(code int, reason string)
	closeCh        chan struct{}
	closeErr       error
	mu             sync.Mutex
}

// ID returns the session identifier
func (s *SessionImpl) ID() string {
	return s.id
}

// State returns the current session state
func (s *SessionImpl) State() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state
}

// OnMessage sets a handler for incoming messages
func (s *SessionImpl) OnMessage(handler MessageHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messageHandler = handler
}

// OnError sets a handler for session errors
func (s *SessionImpl) OnError(handler ErrorHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.errorHandler = handler
}

// OnClose sets a handler for session close events
func (s *SessionImpl) OnClose(code int, reason string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closeHandler(code, reason)
}

// Send sends a message to the server
// This is a stub that will be implemented in the next task
func (s *SessionImpl) Send(message string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state != SessionOpen {
		return &SockJSError{
			Code:    ErrCodeClientError,
			Message: "Session not open",
		}
	}

	// Queue messages if transport is not ready
	s.messageQueue = append(s.messageQueue, message)

	return nil
}

// Close terminates the session connection
// This is a stub that will be implemented in the next task
func (s *SessionImpl) Close(code int, reason string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state == SessionClosed {
		return nil
	}

	s.state = SessionClosing

	// Actual close implementation will be added in the next task
	return nil
}

// Wait blocks until the session is closed
func (s *SessionImpl) Wait() error {
	<-s.closeCh
	return s.closeErr
}
