#!/bin/bash

# Colors for terminal output
GREEN='\033[0;32m'
BLUE='\033[0;34m'
RED='\033[0;31m'
NC='\033[0m' # No Color
YELLOW='\033[1;33m'
CYAN='\033[0;36m'

# Set error handling
set -e
trap cleanup EXIT INT TERM

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LOG_FILE="${SCRIPT_DIR}/server.log"
SERVER_PID=""

cleanup() {
  # Kill server if running
  if [ ! -z "$SERVER_PID" ] && ps -p $SERVER_PID > /dev/null; then
    echo -e "\n${YELLOW}Stopping SockJS server (PID: $SERVER_PID)...${NC}"
    kill $SERVER_PID 2>/dev/null || true
    wait $SERVER_PID 2>/dev/null || true
  fi
}

echo -e "${BLUE}=== SockJS Client Integration Test ===${NC}"

# Change to the script directory
cd "$SCRIPT_DIR"

# Check if required NPM packages are installed
if [ ! -d "node_modules" ] || [ ! -d "node_modules/sockjs" ] || [ ! -d "node_modules/colors" ]; then
  echo -e "${YELLOW}Installing required NPM packages...${NC}"
  npm install
fi

# Check for flags
TEST_DURATION=10
URL="http://localhost:8081/sockjs"
SERVER_PORT=8081
TRANSPORT="websocket"  # Default to WebSocket transport

function show_help {
  echo -e "Usage: ./run_test.sh [OPTIONS]"
  echo -e "Options:"
  echo -e "  ${CYAN}--timeout=N${NC}        Set test duration in seconds (default: 10)"
  echo -e "  ${CYAN}--url=URL${NC}          Custom SockJS server URL (default: http://localhost:8081/sockjs)"
  echo -e "  ${CYAN}--port=PORT${NC}        Server port (default: 8081)"
  echo -e "  ${CYAN}--transport=TYPE${NC}   Transport type: 'websocket' or 'xhr' (default: websocket)"
  echo -e "  ${CYAN}--help${NC}             Show this help message"
}

while [[ "$#" -gt 0 ]]; do
  case $1 in
    --timeout=*|--duration=*) TEST_DURATION="${1#*=}"; shift ;;
    --url=*) URL="${1#*=}"; shift ;;
    --port=*) SERVER_PORT="${1#*=}"; shift ;;
    --transport=*) 
      TRANSPORT="${1#*=}"
      # Validate transport
      if [[ "$TRANSPORT" != "websocket" && "$TRANSPORT" != "xhr" ]]; then
        echo -e "${RED}Invalid transport type: $TRANSPORT. Must be 'websocket' or 'xhr'.${NC}"
        show_help
        exit 1
      fi
      shift ;;
    --help) 
      show_help
      exit 0
      ;;
    *) echo -e "${RED}Unknown parameter: $1${NC}"; show_help; exit 1 ;;
  esac
done

# Kill any existing server process
existing_pid=$(pgrep -f "node $SCRIPT_DIR/server.js" || echo "")
if [ ! -z "$existing_pid" ]; then
  echo -e "${YELLOW}Stopping existing SockJS server (PID: $existing_pid)...${NC}"
  kill $existing_pid 2>/dev/null || true
  sleep 1
fi

# Start the server in the background
echo -e "${GREEN}Starting SockJS test server on port $SERVER_PORT...${NC}"
echo "" > "$LOG_FILE"  # Clear log file
PORT=$SERVER_PORT node server.js > "$LOG_FILE" 2>&1 &
SERVER_PID=$!

# Wait for the server to start
echo -e "${YELLOW}Waiting for server to start...${NC}"
sleep 2

# Check if server is running
if ! ps -p $SERVER_PID > /dev/null; then
  echo -e "${RED}Failed to start server. Check server.log for details.${NC}"
  cat "$LOG_FILE"
  exit 1
fi

# Verify server is accepting connections
if ! curl -s "http://localhost:$SERVER_PORT/sockjs/info" > /dev/null; then
  echo -e "${RED}Server is not responding to requests. Check server.log for details.${NC}"
  cat "$LOG_FILE"
  kill $SERVER_PID 2>/dev/null || true
  exit 1
fi

echo -e "${GREEN}Server started successfully with PID $SERVER_PID${NC}"
echo -e "${CYAN}Server log: $LOG_FILE${NC}"

# Run the client
echo -e "${GREEN}Running test client with $TRANSPORT transport (timeout: ${TEST_DURATION}s)...${NC}"
echo -e "${BLUE}----------------------------------------${NC}"

# Set transport option
TRANSPORT_FLAG=""
if [ "$TRANSPORT" = "xhr" ]; then
  TRANSPORT_FLAG="--transport=xhr"
elif [ "$TRANSPORT" = "websocket" ]; then
  TRANSPORT_FLAG="--transport=websocket"
fi

# Run the test client
cd ../..
go run cmd/integration_test/main.go --url="$URL" --timeout="$TEST_DURATION" $TRANSPORT_FLAG
TEST_EXIT_CODE=$?
echo -e "${BLUE}----------------------------------------${NC}"

# Show success/failure message
if [ $TEST_EXIT_CODE -eq 0 ]; then
  echo -e "${GREEN}Test completed successfully! 🎉${NC}"
else
  echo -e "${RED}Test failed with exit code $TEST_EXIT_CODE${NC}"
  echo -e "${YELLOW}Check the server log for details: ${LOG_FILE}${NC}"
fi

exit $TEST_EXIT_CODE 