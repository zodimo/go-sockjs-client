package transport

import (
	"context"
	"time"
)

// Transport defines the interface for SockJS transports.
// All transport implementations (WebSocket, XHR, etc.) must satisfy this interface.
type Transport interface {
	// Connect establishes a connection to the SockJS server using the specific transport.
	Connect(ctx context.Context) error

	// Send transmits a message to the server.
	Send(ctx context.Context, data string) error

	// Close terminates the connection with the server.
	Close(ctx context.Context, code int, reason string) error

	// SetMessageHandler registers a handler for incoming messages.
	SetMessageHandler(handler func(string))

	// SetErrorHandler registers a handler for transport errors.
	SetErrorHandler(handler func(error))

	// SetCloseHandler registers a handler for connection close events.
	SetCloseHandler(handler func(int, string))

	// Name returns the transport type name (e.g., "websocket", "xhr").
	Name() string
}

// ReconnectConfig contains settings for automatic reconnection.
type ReconnectConfig struct {
	// MaxAttempts is the maximum number of reconnection attempts (0 means no limit).
	MaxAttempts int

	// Initial delay before the first reconnection attempt.
	InitialDelay time.Duration

	// MaxDelay is the maximum delay between reconnection attempts.
	MaxDelay time.Duration

	// Multiplier is used to increase the delay after each failed attempt.
	Multiplier float64

	// Jitter adds randomness to the delay to prevent thundering herd.
	Jitter float64
}

// DefaultReconnectConfig returns a ReconnectConfig with sensible defaults.
func DefaultReconnectConfig() ReconnectConfig {
	return ReconnectConfig{
		MaxAttempts:  10,
		InitialDelay: 500 * time.Millisecond,
		MaxDelay:     30 * time.Second,
		Multiplier:   1.5,
		Jitter:       0.2,
	}
}

// Option is a function that configures a transport.
type Option func(interface{})

// WithReconnect configures reconnection settings for a transport.
func WithReconnect(config ReconnectConfig) Option {
	return func(t interface{}) {
		if wt, ok := t.(*WebSocketTransport); ok {
			wt.reconnectCfg = config
		}
		// Add other transport types here as they're implemented
		if _, ok := t.(*XHRTransport); ok {
			// We're not using reconnectCfg for XHR yet, but could add it in the future
			// For now, just record that it was set for future implementation
		}
	}
}

// WithConnectTimeout sets the connection timeout.
func WithConnectTimeout(timeout time.Duration) Option {
	return func(t interface{}) {
		if wt, ok := t.(*WebSocketTransport); ok {
			wt.connectTimeout = timeout
		}
		// Add other transport types here as they're implemented
		if xt, ok := t.(*XHRTransport); ok {
			// Update client timeout for XHR
			xt.client.Timeout = timeout
		}
	}
}
