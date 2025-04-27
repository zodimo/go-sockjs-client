package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/zodimo/go-sockjs-client/pkg/sockjs/transport"
)

// This is an integration test that connects to a real SockJS server
// and verifies that the polling mechanism works correctly.
//
// Usage:
//
//	go run cmd/integration_test/main.go --url=http://your-sockjs-server/sockjs
//
// You can use the following NodeJS SockJS server for testing:
// https://github.com/sockjs/sockjs-node

func init() {
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage of %s:\n", os.Args[0])
		fmt.Fprintf(flag.CommandLine.Output(), "  --url=URL           SockJS server URL (default: http://localhost:8081/sockjs)\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  --timeout=N         Test duration in seconds (default: 30)\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  --transport=TYPE    Transport type: 'websocket' or 'xhr' (default: websocket)\n")
		fmt.Fprintf(flag.CommandLine.Output(), "\nExample:\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  go run main.go --url=http://localhost:8081/sockjs --timeout=10 --transport=xhr\n")
	}
}

func main() {
	url := flag.String("url", "http://localhost:8081/sockjs", "SockJS server URL")
	timeout := flag.Int("timeout", 30, "Test timeout in seconds")
	transportType := flag.String("transport", "websocket", "Transport type: 'websocket' or 'xhr'")
	flag.Parse()

	// Validate transport type
	if *transportType != "websocket" && *transportType != "xhr" {
		fmt.Printf("Invalid transport type: %s. Must be 'websocket' or 'xhr'.\n", *transportType)
		flag.Usage()
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(*timeout)*time.Second)
	defer cancel()

	// Set up signal handling for graceful termination
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-signalCh
		fmt.Println("Signal received, shutting down...")
		cancel()
	}()

	fmt.Println("Running SockJS client integration test")
	fmt.Printf("Connecting to: %s\n", *url)
	fmt.Printf("Transport: %s\n", *transportType)
	fmt.Printf("Test will run for up to %d seconds\n", *timeout)

	// Create headers for proper connection
	headers := http.Header{}
	headers.Add("Content-Type", "application/x-www-form-urlencoded")

	// Create the transport based on the specified type
	var t transport.Transport
	var err error

	switch *transportType {
	case "websocket":
		t, err = transport.NewWebSocketTransport(*url, transport.WithHeaders(headers))
	case "xhr":
		t, err = transport.NewXHRTransport(*url, headers)
	}

	if err != nil {
		fmt.Printf("Error creating transport: %v\n", err)
		os.Exit(1)
	}

	// Message queue and synchronization
	var (
		receivedMessages []string
		messagesMutex    sync.Mutex
		connected        bool = false
		connectedCh           = make(chan struct{})
		disconnectedCh        = make(chan struct{})
		errorCh               = make(chan error, 100) // Increased buffer size
	)

	// Set up handlers
	t.SetMessageHandler(func(msg string) {
		messagesMutex.Lock()
		defer messagesMutex.Unlock()

		fmt.Printf("Received message: %s\n", msg)
		receivedMessages = append(receivedMessages, msg)
	})

	t.SetErrorHandler(func(err error) {
		errMsg := err.Error()

		// Handle "unrecognized message format" errors as actual messages
		if strings.HasPrefix(errMsg, "unrecognized message format:") {
			// Extract the actual message content
			actualMsg := strings.TrimPrefix(errMsg, "unrecognized message format: ")
			fmt.Printf("Received raw message: %s\n", actualMsg)

			messagesMutex.Lock()
			receivedMessages = append(receivedMessages, actualMsg)
			messagesMutex.Unlock()
			return
		}

		// Handle other real errors
		fmt.Printf("Error: %v\n", err)
		select {
		case errorCh <- err:
		default:
			fmt.Printf("Error channel full, dropping: %v\n", err)
		}
	})

	t.SetCloseHandler(func(code int, reason string) {
		fmt.Printf("Connection closed: code=%d, reason=%s\n", code, reason)
		connected = false
		select {
		case disconnectedCh <- struct{}{}:
		default:
		}
	})

	// Connect to the SockJS server
	fmt.Println("Connecting...")
	err = t.Connect(ctx)
	if err != nil {
		fmt.Printf("Failed to connect: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Connected!")
	connected = true
	// Send non-blocking signal to connectedCh
	select {
	case connectedCh <- struct{}{}:
	default:
		// Channel not ready, that's fine
	}

	// Keep the test running and periodically send messages
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	messageCount := 0

	for {
		select {
		case <-ctx.Done():
			fmt.Println("Test completed: context done")
			printResults(receivedMessages, errorCh, connected)
			return

		case <-ticker.C:
			if connected {
				messageCount++
				message := fmt.Sprintf("Test message %d", messageCount)
				fmt.Printf("Sending: %s\n", message)
				err := t.Send(ctx, message)
				if err != nil {
					fmt.Printf("Failed to send message: %v\n", err)
				}
			}

		case <-disconnectedCh:
			fmt.Println("Disconnected from server")
			printResults(receivedMessages, errorCh, connected)
			return
		}
	}
}

func printResults(messages []string, errorCh chan error, connected bool) {
	fmt.Println("\n--- Test Results ---")
	fmt.Printf("Connected: %v\n", connected)

	fmt.Printf("Received %d messages:\n", len(messages))
	for i, msg := range messages {
		fmt.Printf("  %d: %s\n", i+1, msg)
	}

	fmt.Println("Errors:")
	errorCount := 0
	for {
		select {
		case err := <-errorCh:
			errorCount++
			fmt.Printf("  %d: %v\n", errorCount, err)
		default:
			if errorCount == 0 {
				fmt.Println("  None")
			}
			return
		}
	}
}
