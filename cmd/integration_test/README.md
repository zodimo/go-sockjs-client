# SockJS Client Integration Test

This directory contains an integration test for the SockJS client, allowing you to test the client against a real SockJS server to ensure that it works correctly, including the polling mechanism.

## Prerequisites

For the Node.js test server:
```
npm install
```

## Quick Start

For the easiest way to run the integration test, use the provided script:

```bash
# From the integration_test directory
./run_test.sh

# To specify a timeout duration
./run_test.sh --timeout=15

# To use a different port
./run_test.sh --port=9000

# To test with XHR transport instead of WebSocket
./run_test.sh --transport=xhr

# To see all options
./run_test.sh --help
```

The script will:
1. Install required dependencies if missing
2. Start the SockJS server
3. Run the test client with the selected transport
4. Clean up resources when done

## Project Structure

```
cmd/integration_test/
├── main.go           # Go test client
├── server.js         # Node.js SockJS test server
├── run_test.sh       # Test automation script
├── package.json      # Node.js dependencies
├── .gitignore        # Ignores logs and dependencies
└── README.md         # This documentation
```

## Manual Testing

To run the test manually, follow these steps:

### 1. Start the test server

```bash
node server.js
```

This will start a SockJS server on port 8081 (or the port specified in the PORT environment variable).

### 2. Run the Go client integration test

```bash
go run main.go --url=http://localhost:8081/sockjs --transport=websocket
```

You can customize the test with these flags:
- `--url`: The URL of the SockJS server (default: `http://localhost:8081/sockjs`)
- `--timeout`: Test duration in seconds (default: 30)
- `--transport`: Transport type, either "websocket" or "xhr" (default: "websocket")

## Expected Behavior

When running correctly:

1. The Go client will connect to the SockJS server
2. It will send a test message every 2 seconds
3. The server will echo back the message and send a timestamp
4. The client should receive these messages through the selected transport
5. After the timeout expires (or when you press Ctrl+C), the client will print the test results

## Troubleshooting

If the test fails to connect:
- Make sure the SockJS server is running
- Check that the port numbers match
- Verify there are no firewall issues
- Try the other transport type (some environments may block WebSockets)

## Testing with External SockJS Servers

You can also test against any compliant SockJS server by specifying its URL:

```bash
go run main.go --url=http://your-server.example.com/sockjs
```

## Understanding What This Tests

This integration test verifies:

1. **Connection Establishment**: The client can connect to a real SockJS server
2. **Transport Mechanisms**: Both WebSocket and XHR transports work correctly
3. **Message Sending**: The client can send messages to the server
4. **Message Receiving**: The client can receive messages from the server
5. **Connection Handling**: The client handles connection events properly

The test validates that the client implementation works in a real-world scenario, not just in isolated unit tests.