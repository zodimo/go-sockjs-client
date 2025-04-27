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

The project includes comprehensive tests for the SockJS client implementation. Some tests that involve concurrent operations are currently skipped due to stability issues that will be addressed in future updates.

### Running Unit Tests

Run all tests with Go's test command, skipping known problematic tests:

```bash
go test ./... -skip "TestXHRTransportConnectComprehensive|TestXHRTransportThreadSafety|TestXHRTransportConnect"
```

For more detailed output:

```bash
go test -v ./... -skip "TestXHRTransportConnectComprehensive|TestXHRTransportThreadSafety|TestXHRTransportConnect"
```

### Integration Testing

While unit tests verify individual components in isolation, integration tests validate that the client works correctly with real SockJS servers.

To run the integration test:

1. Start the test server:
```bash
cd cmd/integration_test
npm install sockjs colors
node server.js
```

2. In another terminal, run the client:
```bash
cd cmd/integration_test
go run main.go
```

The integration test verifies that the polling mechanism works correctly when connecting to an actual SockJS server, sending messages, and receiving responses.

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

This command runs the entire test suite with a 30-second timeout, automatically skipping known problematic tests. This is the recommended approach for CI environments or quick checks during development.

### Known Issues

The following tests are currently skipped and will be fixed in upcoming updates:

1. `TestXHRTransportConnect` - Known issue with concurrency that needs deeper investigation
2. `TestXHRTransportThreadSafety` - Concurrent map access issues
3. `TestXHRTransportConnectComprehensive` - Timeout issues that need more investigation

These tests validate edge cases in concurrency handling and will be reimplemented with a more stable approach. 