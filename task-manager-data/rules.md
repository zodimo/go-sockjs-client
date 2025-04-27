# Development Guidelines

## Project Overview

- A Go client implementation of the SockJS protocol
- Provides functionality to connect to SockJS servers using various transport mechanisms (WebSocket, XHR)
- Go version 1.20 with minimal dependencies (gorilla/websocket, golang.org/x/net)
- Repository: github.com/zodimo/go-sockjs-client

## Project Architecture

### Directory Structure

- `/pkg/sockjs/` - Core client implementation (client, session, framing)
- `/pkg/sockjs/transport/` - Transport interface and implementations
- `/cmd/integration_test/` - Integration testing with Node.js server
- Root level scripts for testing (run_tests.sh)

### Module Management

- Maintain Go 1.20 compatibility
- Explicitly version all dependencies in go.mod
- Minimize external dependencies - only use when necessary
- Current dependencies:
  - github.com/gorilla/websocket v1.5.3 - For WebSocket transport
  - golang.org/x/net v0.17.0 - For HTTP functionality

## Coding Standards

### Design Patterns

- **Interface-based design** - Define interfaces before implementations
- **Functional options pattern** - Use for configuration (see transport.Option)
- **Versioned structs** - Use V2 suffix for updated implementations (ClientV2, SessionV2)
- **Context-based APIs** - All network operations must accept context for cancellation

### Naming Conventions

- Interface names should be clear without "Interface" suffix (e.g., Transport)
- Implementation types should indicate their concrete type (e.g., WebSocketTransport)
- Option functions should use With prefix (e.g., WithReconnect)
- Default constants should use default prefix (e.g., defaultConnectTimeout)

### Error Handling

- Return explicit, descriptive errors rather than using panic
- Use fmt.Errorf with %w for error wrapping
- Check all errors from external calls
- Propagate transport-specific errors through appropriate handlers

### Concurrency

- Protect shared state with mutex (sync.Mutex/sync.RWMutex)
- Use channels for signaling (e.g., closed channel pattern)
- Every concurrent operation must be thread-safe

### Documentation

- All exported types and functions must have descriptive comments
- Comments should explain parameter usage and return values
- Configuration options must be clearly documented

## Feature Implementation Guidelines

### Transport System

- Each transport must implement the `transport.Transport` interface
- New transport types must be added to the transport type list in client.go
- Transport connection failures must return descriptive errors
- Follow existing patterns in WebSocketTransport and XHRTransport

### Client Configuration

- Use `ClientConfig` struct for configuration
- Provide sensible defaults for all configuration options
- Validate required configuration fields (e.g., ServerURL)

### Session Management

- Sessions must track their state (Open, Closing, Closed)
- All session operations must be thread-safe
- Support handler registration for messages, errors, and close events

## Testing Requirements

### Unit Testing

- Each file should have a corresponding _test.go file
- Test all exported functions and methods
- Use table-driven tests for comprehensive test cases
- Test happy paths and error conditions

### Integration Testing

- Use cmd/integration_test for testing against a real SockJS server
- Integration tests must verify cross-compatibility with Node.js SockJS
- Test all supported transports (WebSocket, XHR)

### Test Execution

- Use run_tests.sh script for safe test execution
- Set appropriate timeouts to prevent hanging tests
- Track test coverage with coverage.out files

## Key File Interactions

### Core Components

- **client.go** - Client implementation and connection establishment
- **sockjs.go** - Session interface and implementation
- **frame.go** - Protocol framing and message handling
- **transport/transport.go** - Transport interface and options
- **transport/websocket.go** - WebSocket transport implementation
- **transport/xhr.go** - XHR transport implementation

### Cross-File Dependencies

- Any changes to the Transport interface require updating all transport implementations
- Changes to session handling in client.go may require updates to sockjs.go
- Option functions in transport.go must handle all implemented transport types

## AI Decision Guidelines

### Implementation Priority

1. **Correctness** - Adhere to SockJS protocol specification
2. **Robustness** - Handle network errors, reconnection, and edge cases
3. **Extensibility** - Maintain pluggable transport architecture
4. **Performance** - Optimize for efficient connection handling and messaging

### Decision Making Process

- Follow existing patterns and conventions in the codebase
- Prioritize backward compatibility when modifying interfaces
- When adding features, consider all transport implementations
- Reference SockJS protocol documentation for protocol-specific details

## Prohibitions

- DO NOT add transports without implementing the full Transport interface
- DO NOT use global state - all state should be contained in struct fields
- DO NOT bypass the context parameter - it must be honored for cancellation
- DO NOT hardcode timeouts - use configuration parameters
- DO NOT implement protocol variations without documentation
- DO NOT modify the public API without version changes
- DO NOT ignore thread safety in client/session operations
- DO NOT add dependencies beyond the minimal set required