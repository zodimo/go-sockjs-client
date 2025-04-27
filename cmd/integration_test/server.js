// A simple SockJS server for testing the Go SockJS client
const http = require('http');
const sockjs = require('sockjs');
const colors = require('colors');

// Create a SockJS server
const sockjs_opts = {
  prefix: '/sockjs',
  log: function(severity, message) {
    console.log(colors.yellow('[SockJS] ') + colors.gray(severity + ': ') + message);
  }
};

const sockjs_server = sockjs.createServer(sockjs_opts);

// On connection handler
sockjs_server.on('connection', function(conn) {
  console.log(colors.green('Client connected: ') + conn.id);

  // Send a welcome message
  conn.write('Welcome to SockJS test server!');

  // Echo any messages back to the client
  conn.on('data', function(message) {
    console.log(colors.blue('Received: ') + message);
    
    // Echo the message back
    conn.write('Echo: ' + message);
    
    // For testing, also send a response with the current time
    conn.write('Server time: ' + new Date().toISOString());
  });

  // Handle close events
  conn.on('close', function() {
    console.log(colors.red('Client disconnected: ') + conn.id);
  });
});

// Create an HTTP server
const server = http.createServer();
sockjs_server.installHandlers(server);

// Start the server
const port = process.env.PORT || 8081;
server.listen(port, '0.0.0.0', function() {
  console.log(colors.green('SockJS test server started on port ' + port));
  console.log(colors.cyan('To test the Go client:'));
  console.log(colors.white('  go run cmd/integration_test/main.go --url=http://localhost:' + port + '/sockjs'));
});

// Print CORS and WebSocket info
console.log(colors.yellow('Note: ') + 'This server accepts connections from any origin');
console.log(colors.yellow('WebSocket endpoint: ') + 'ws://localhost:' + port + '/sockjs/websocket');
console.log(colors.yellow('XHR endpoint: ') + 'http://localhost:' + port + '/sockjs/info');

// Handle Ctrl+C gracefully
process.on('SIGINT', function() {
  console.log(colors.yellow('\nShutting down server...'));
  server.close(function() {
    console.log(colors.green('Server shut down successfully'));
    process.exit(0);
  });
}); 