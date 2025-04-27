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