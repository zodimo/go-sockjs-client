package sockjs

import (
	"context"
	"net/http"
	"testing"
	"time"
)

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
