# go-sockjs-client

A Go client implementation of the SockJS protocol.

## Features
- Supports SockJS transports (WebSocket, XHR, etc.)
- Pluggable transport architecture
- Session management and error handling

## Installation
```bash
go get github.com/zodimo/go-sockjs-client
```

## Usage Example
```go
package main

import (
    "context"
    "github.com/zodimo/go-sockjs-client/pkg/sockjs"
)

func main() {
    opts := &sockjs.Options{
        URL: "http://localhost:8080/sockjs",
    }
    client := sockjs.NewClient(opts)
    session, err := client.Connect(context.Background())
    if err != nil {
        panic(err)
    }
    defer session.Close(2000, "Normal closure")
    // ...
}
```

## License
MIT 

## Testing

The project includes comprehensive tests for the SockJS client implementation.

### Running Unit Tests

Run all tests with Go's test command:

```bash
go test ./...
```

For more detailed output:

```bash
go test -v ./...
```

### Integration Testing

While unit tests verify individual components in isolation, integration tests validate that the client works correctly with real SockJS servers.

To run the integration test:

```bash
cd cmd/integration_test
./run_test.sh
```

This script will:
1. Start a SockJS test server
2. Run the client test against it
3. Clean up resources when done

You can specify which transport to test:
```bash
./run_test.sh --transport=websocket  # Test WebSocket transport
./run_test.sh --transport=xhr        # Test XHR transport
```

The integration test verifies that the client works correctly when connecting to an actual SockJS server, sending messages, and receiving responses.

See [cmd/integration_test/README.md](cmd/integration_test/README.md) for more details.

### Safe Test Execution

To run tests safely with timeouts to avoid blocking, use the included test script:

```bash
chmod +x run_tests.sh
./run_tests.sh
```

The script runs each test individually with a timeout, preventing any hanging tests from blocking the entire test suite.

You can also run all tests at once with a single timeout:

```bash
./run_tests.sh --all
```

This command runs the entire test suite with a 30-second timeout. This is the recommended approach for CI environments or quick checks during development. 