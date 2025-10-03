package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"math/big"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/coder/websocket"
)

// Connection states
const (
	stateDisconnected = iota
	stateConnecting
	stateConnected
)

// WebSocket message types (these are defined in the websocket package but we need to define them here)
const (
	MessageText   = websocket.MessageText
	MessageBinary = websocket.MessageBinary
)

// JSON-RPC 2.0 related structures
type JsonRpcMessage struct {
	JsonRpc string        `json:"jsonrpc"`
	Id      interface{}   `json:"id,omitempty"`
	Method  string        `json:"method,omitempty"`
	Result  interface{}   `json:"result,omitempty"`
	Error   *JsonRpcError `json:"error,omitempty"`
}

type JsonRpcError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// ConnectionTimeout is the timeout for initial connection attempts
const ConnectionTimeout = 5 * time.Second

// PendingRequest represents a message waiting for the connection to be established
type PendingRequest struct {
	ID      interface{}
	Message string
	Timer   *time.Timer
}

func generateWebSocketKey() string {
	key := make([]byte, 16)
	_, err := rand.Read(key)
	if err != nil {
		log.Fatalf("failed to generate WebSocket key: %v", err)
	}
	return base64.StdEncoding.EncodeToString(key)
}

func main() {
	// Get program name for logs
	programName := "nd-mcp-golang"
	if len(os.Args) > 0 {
		programName = os.Args[0]
	}

	args := os.Args[1:]
	var targetURL string
	var bearerToken string

	for len(args) > 0 {
		arg := args[0]
		switch {
		case arg == "--bearer":
			if len(args) < 2 {
				fmt.Fprintf(os.Stderr, "%s: Usage: %s [--bearer TOKEN] ws://host/path\n", programName, programName)
				os.Exit(1)
			}
			bearerToken = strings.TrimSpace(args[1])
			args = args[2:]
		case strings.HasPrefix(arg, "--bearer="):
			bearerToken = strings.TrimSpace(strings.TrimPrefix(arg, "--bearer="))
			args = args[1:]
		default:
			if targetURL != "" {
				fmt.Fprintf(os.Stderr, "%s: Unexpected argument '%s'\n", programName, arg)
				fmt.Fprintf(os.Stderr, "%s: Usage: %s [--bearer TOKEN] ws://host/path\n", programName, programName)
				os.Exit(1)
			}
			targetURL = arg
			args = args[1:]
		}
	}

	if targetURL == "" {
		fmt.Fprintf(os.Stderr, "%s: Usage: %s [--bearer TOKEN] ws://host/path\n", programName, programName)
		os.Exit(1)
	}

	if bearerToken == "" {
		bearerToken = strings.TrimSpace(os.Getenv("ND_MCP_BEARER_TOKEN"))
	}

	if bearerToken != "" {
		fmt.Fprintf(os.Stderr, "%s: Authorization header enabled for MCP connection\n", programName)
	}

	// Set up channels for communication
	stdinCh := make(chan string, 100)     // Buffer stdin messages
	reconnectCh := make(chan struct{}, 1) // Signal for immediate reconnection
	doneCh := make(chan struct{})         // Signal for program termination
	stdinClosedCh := make(chan struct{})  // Signal that stdin is closed

	// Global state
	var state int // Connection state
	var stateMu sync.Mutex
	var messageQueueMu sync.Mutex
	messageQueue := []string{}
	stdinActive := true

	// Pending requests that are waiting for connection to be established
	var pendingMu sync.Mutex
	pendingRequests := make(map[interface{}]*PendingRequest)

	// Parse a JSON-RPC message and extract the ID and method
	parseJsonRpcMessage := func(message string) (interface{}, string) {
		var msg JsonRpcMessage
		err := json.Unmarshal([]byte(message), &msg)
		if err != nil || msg.JsonRpc != "2.0" {
			return nil, ""
		}
		return msg.Id, msg.Method
	}

	// Create a JSON-RPC error response
	createJsonRpcError := func(id interface{}, code int, message string, data interface{}) string {
		response := JsonRpcMessage{
			JsonRpc: "2.0",
			Id:      id,
			Error: &JsonRpcError{
				Code:    code,
				Message: message,
				Data:    data,
			},
		}
		responseJson, err := json.Marshal(response)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: ERROR: Failed to marshal error response: %v\n", programName, err)
			return fmt.Sprintf("{\"jsonrpc\":\"2.0\",\"id\":%v,\"error\":{\"code\":%d,\"message\":\"%s\"}}", id, code, message)
		}
		return string(responseJson)
	}

	// Handle a request timeout for connection establishment ONLY
	handleRequestTimeout := func(msgId interface{}) {
		stateMu.Lock()
		currentState := state
		stateMu.Unlock()

		// This timeout ONLY applies if the connection is not established yet
		if currentState != stateConnected {
			pendingMu.Lock()
			if req, exists := pendingRequests[msgId]; exists {
				fmt.Fprintf(os.Stderr, "%s: Connection timeout for request ID %v, sending error response\n", programName, msgId)

				// Create and send error response
				errorResponse := createJsonRpcError(
					msgId,
					-32000, // Server error code
					"MCP server connection failed",
					map[string]string{"details": "Could not establish connection to Netdata within timeout period"},
				)

				fmt.Println(errorResponse)

				// Get the original message
				originalMessage := req.Message

				// Remove request from pending
				delete(pendingRequests, msgId)
				pendingMu.Unlock()

				// Also remove from messageQueue if it exists there
				messageQueueMu.Lock()
				for i, msg := range messageQueue {
					if msg == originalMessage {
						// Remove from queue by creating a new slice without this element
						messageQueue = append(messageQueue[:i], messageQueue[i+1:]...)
						fmt.Fprintf(os.Stderr, "%s: Removed timed-out request from message queue\n", programName)
						break
					}
				}
				messageQueueMu.Unlock()
				return
			}
			pendingMu.Unlock()
		} else {
			// The connection was established before the timeout - just clean up
			pendingMu.Lock()
			if _, exists := pendingRequests[msgId]; exists {
				delete(pendingRequests, msgId)
			}
			pendingMu.Unlock()
		}
	}

	// Set up signal handling
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		fmt.Fprintf(os.Stderr, "%s: Received signal %v, shutting down\n", programName, sig)
		close(doneCh)
	}()

	// Start reading from stdin in a separate goroutine
	go func() {
		scanner := bufio.NewScanner(os.Stdin)
		scannerRunning := true

		// Make scanner buffer larger to handle large messages
		const maxScanBufferSize = 1024 * 1024 // 1MB
		buf := make([]byte, maxScanBufferSize)
		scanner.Buffer(buf, maxScanBufferSize)

		for scannerRunning && scanner.Scan() {
			text := scanner.Text()

			// Parse for JSON-RPC ID
			msgId, _ := parseJsonRpcMessage(text)

			// Check connection state
			stateMu.Lock()
			currentState := state
			stateMu.Unlock()

			if currentState != stateConnected && msgId != nil {
				// Store as pending request with timeout
				pendingMu.Lock()
				req := &PendingRequest{
					ID:      msgId,
					Message: text,
					Timer:   time.AfterFunc(ConnectionTimeout, func() { handleRequestTimeout(msgId) }),
				}
				pendingRequests[msgId] = req
				pendingMu.Unlock()

				fmt.Fprintf(os.Stderr, "%s: Received request with ID %v, setting response timeout\n", programName, msgId)
			}

			// Queue the message
			stdinCh <- text

			// If we're not connected, trigger immediate reconnection
			if currentState != stateConnected {
				select {
				case reconnectCh <- struct{}{}:
					fmt.Fprintf(os.Stderr, "%s: Received stdin data, attempting immediate reconnection\n", programName)
				default:
					// Channel already has reconnection request, ignore
				}
			}
		}

		// Check for scanner error
		if err := scanner.Err(); err != nil {
			fmt.Fprintf(os.Stderr, "%s: ERROR: stdin read error: %v\n", programName, err)
		} else {
			fmt.Fprintf(os.Stderr, "%s: End of stdin\n", programName)
		}

		// Signal that stdin is closed
		stdinActive = false
		close(stdinClosedCh)
	}()

	// Reconnection parameters
	baseDelay := 1 * time.Second
	maxDelay := 60 * time.Second
	attempt := 0

	// Timer for reconnection backoff
	var timer *time.Timer

	// Main connection loop
	for {
		select {
		case <-doneCh:
			// Program termination requested
			return
		case <-stdinClosedCh:
			// Stdin closed, continue running until websocket disconnects
			// and then exit on the next reconnection attempt
			stdinClosedCh = nil // Prevent duplicate handling
		case <-reconnectCh:
			// Immediate reconnection requested (e.g. from stdin activity)
			if timer != nil {
				timer.Stop()
			}

			// Only proceed with immediate reconnection if we're not already connecting/connected
			stateMu.Lock()
			currentState := state
			stateMu.Unlock()

			if currentState == stateDisconnected {
				// Reset the reconnection timer
				attempt = 0
				// Fall through to connection attempt
			} else {
				// Already connecting or connected
				continue
			}
		default:
			// Calculate backoff delay with jitter
			if attempt > 0 {
				// Check if stdin is closed and we're disconnected - if so, exit
				if !stdinActive {
					fmt.Fprintf(os.Stderr, "%s: Stdin closed and disconnected, exiting\n", programName)
					return
				}

				delaySeconds := math.Min(float64(maxDelay.Seconds()),
					float64(baseDelay.Seconds())*math.Pow(2, float64(attempt-1))*(0.5+jitter()))
				delay := time.Duration(delaySeconds * float64(time.Second))

				fmt.Fprintf(os.Stderr, "%s: Reconnecting in %.1f seconds (attempt %d)...\n",
					programName, delaySeconds, attempt)

				// Create timer and wait for it to expire, or for signals
				timer = time.NewTimer(delay)

				select {
				case <-timer.C:
					// Timer expired, continue to connection attempt
				case <-reconnectCh:
					// Immediate reconnection requested
					timer.Stop()
				case <-doneCh:
					// Program termination requested
					timer.Stop()
					return
				case <-stdinClosedCh:
					// Stdin closed
					timer.Stop()
					stdinClosedCh = nil
					continue
				}
			}
		}

		// Update state to connecting
		stateMu.Lock()
		state = stateConnecting
		stateMu.Unlock()

		// Set up connection context with cancellation
		ctx, cancel := context.WithCancel(context.Background())

		// Set up connection timeout
		connectionCtx, connectionCancel := context.WithTimeout(ctx, 15*time.Second)
		defer connectionCancel()

		fmt.Fprintf(os.Stderr, "%s: Connecting to %s...\n", programName, targetURL)

		// Create a custom header with the WebSocket key
		header := http.Header{}
		header.Set("Sec-WebSocket-Key", generateWebSocketKey())
		header.Set("Sec-WebSocket-Version", "13")
		if bearerToken != "" {
			header.Set("Authorization", "Bearer "+bearerToken)
		}

		// Connect to WebSocket
		conn, _, err := websocket.Dial(connectionCtx, targetURL, &websocket.DialOptions{
			CompressionMode: websocket.CompressionContextTakeover,
			HTTPHeader:      header,
		})

		// Connection failed
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: ERROR: websocket connection failed: %v\n", programName, err)
			cancel()

			// Increment attempt counter and try again
			attempt++

			// Update state to disconnected
			stateMu.Lock()
			state = stateDisconnected
			stateMu.Unlock()
			continue
		}

		// Increase the read limit to match Netdata's WS_MAX_INCOMING_FRAME_SIZE (16MB)
		conn.SetReadLimit(16 * 1024 * 1024)

		// Update state to connected
		stateMu.Lock()
		state = stateConnected
		stateMu.Unlock()

		fmt.Fprintf(os.Stderr, "%s: Connected\n", programName)
		attempt = 0 // Reset attempt counter on successful connection

		// Process any queued messages, including pending requests
		messageQueueMu.Lock()
		if len(messageQueue) > 0 {
			fmt.Fprintf(os.Stderr, "%s: Sending %d queued message(s)\n", programName, len(messageQueue))
			for _, msg := range messageQueue {
				if err := conn.Write(ctx, websocket.MessageText, []byte(msg)); err != nil {
					fmt.Fprintf(os.Stderr, "%s: ERROR: failed to send queued message: %v\n", programName, err)
				} else {
					// Check if this was a pending request with an ID
					msgId, _ := parseJsonRpcMessage(msg)
					if msgId != nil {
						pendingMu.Lock()
						if req, exists := pendingRequests[msgId]; exists {
							// Stop the timer
							if req.Timer != nil {
								req.Timer.Stop()
							}
							delete(pendingRequests, msgId)
						}
						pendingMu.Unlock()
					}
				}
			}
			messageQueue = nil // Clear the queue
		}
		messageQueueMu.Unlock()

		// Memory management: Limit size of pendingRequests to prevent memory leaks
		pendingMu.Lock()
		if len(pendingRequests) > 1000 {
			fmt.Fprintf(os.Stderr, "%s: Too many pending requests (%d), cleaning up older ones\n", programName, len(pendingRequests))
			// Remove older entries (this is a bit tricky in Go without order guarantee)
			// For simplicity, we'll just clear it all in this extreme case
			pendingRequests = make(map[interface{}]*PendingRequest)
		}
		pendingMu.Unlock()

		// Signal for connection closure
		connClosed := make(chan struct{})

		// Create wait group for goroutines
		var wg sync.WaitGroup

		// Goroutine for handling termination signals during connection
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case <-ctx.Done():
				return
			case <-doneCh:
				// Clean up the connection
				conn.Close(websocket.StatusNormalClosure, "Shutdown requested")
				cancel()
				return
			case <-stdinClosedCh:
				// Stdin closed, but keep connection active
				stdinClosedCh = nil
				return
			}
		}()

		// Goroutine for sending messages to websocket
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case <-connClosed:
					return
				case msg, ok := <-stdinCh:
					if !ok {
						// Stdin channel was closed
						return
					}

					err := conn.Write(ctx, websocket.MessageText, []byte(msg))
					if err != nil {
						// Check if this was a message with ID
						msgId, _ := parseJsonRpcMessage(msg)

						select {
						case <-connClosed:
							// Connection already known to be closed, queue the message
							messageQueueMu.Lock()
							messageQueue = append(messageQueue, msg)
							messageQueueMu.Unlock()

							// If the message had an ID, keep the pending status
							if msgId != nil {
								pendingMu.Lock()
								if _, exists := pendingRequests[msgId]; !exists {
									// Create a new pending request with timeout if it doesn't exist
									req := &PendingRequest{
										ID:      msgId,
										Message: msg,
										Timer:   time.AfterFunc(ConnectionTimeout, func() { handleRequestTimeout(msgId) }),
									}
									pendingRequests[msgId] = req
								}
								pendingMu.Unlock()
							}
						default:
							fmt.Fprintf(os.Stderr, "%s: ERROR: write to websocket: %v\n", programName, err)
							messageQueueMu.Lock()
							messageQueue = append(messageQueue, msg)
							messageQueueMu.Unlock()

							// Same ID handling as above
							if msgId != nil {
								pendingMu.Lock()
								if _, exists := pendingRequests[msgId]; !exists {
									req := &PendingRequest{
										ID:      msgId,
										Message: msg,
										Timer:   time.AfterFunc(ConnectionTimeout, func() { handleRequestTimeout(msgId) }),
									}
									pendingRequests[msgId] = req
								}
								pendingMu.Unlock()
							}
						}
					} else {
						// Message was sent successfully, remove from pending if it had an ID
						msgId, _ := parseJsonRpcMessage(msg)
						if msgId != nil {
							pendingMu.Lock()
							if req, exists := pendingRequests[msgId]; exists {
								// Stop the connection establishment timer (if any)
								if req.Timer != nil {
									req.Timer.Stop()
									req.Timer = nil // Clear timer to prevent memory leaks
								}
								// Keep in pendingRequests until we get a response
							}
							pendingMu.Unlock()
						}
					}
				}
			}
		}()

		// Goroutine for receiving messages from websocket
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer close(connClosed)

			for {
				messageType, message, err := conn.Read(ctx)
				if err != nil {
					// Don't report error if context was cancelled
					if ctx.Err() == nil {
						fmt.Fprintf(os.Stderr, "%s: ERROR: read from websocket: %v\n", programName, err)
					}
					return
				}

				if messageType == websocket.MessageText {
					fmt.Println(string(message))

					// Check if this is a response to a request with an ID
					var response JsonRpcMessage
					if err := json.Unmarshal(message, &response); err == nil && response.JsonRpc == "2.0" && response.Id != nil {
						pendingMu.Lock()
						if req, exists := pendingRequests[response.Id]; exists {
							// Stop the timer
							if req.Timer != nil {
								req.Timer.Stop()
							}
							delete(pendingRequests, response.Id)
						}
						pendingMu.Unlock()
					}
				}
			}
		}()

		// Set up periodic pinger to keep connection alive
		pingInterval := 30 * time.Second
		wg.Add(1)
		go func() {
			defer wg.Done()
			ticker := time.NewTicker(pingInterval)
			defer ticker.Stop()

			for {
				select {
				case <-ticker.C:
					// Send ping using a text message with a special ping format
					err := conn.Write(ctx, websocket.MessageText, []byte("PING"))
					if err != nil {
						fmt.Fprintf(os.Stderr, "%s: ERROR: ping failed: %v\n", programName, err)
						// Connection will be closed by the read loop
					}
				case <-ctx.Done():
					return
				case <-connClosed:
					return
				}
			}
		}()

		// Wait for connection closed
		<-connClosed

		// Update state to disconnected
		stateMu.Lock()
		state = stateDisconnected
		stateMu.Unlock()

		// Clean up
		cancel()
		wg.Wait()

		// Don't automatically reconnect if stdin closed and no messages in queue
		messageQueueMu.Lock()
		hasMessages := len(messageQueue) > 0
		messageQueueMu.Unlock()

		if !stdinActive && !hasMessages {
			fmt.Fprintf(os.Stderr, "%s: Stdin closed and no pending messages, exiting\n", programName)
			return
		}

		// Increment attempt counter for next reconnection
		attempt++
	}
}

// jitter returns a random value between 0.0 and 1.0
func jitter() float64 {
	n, err := rand.Int(rand.Reader, big.NewInt(1000))
	if err != nil {
		return 0.5 // Fall back to 0.5 if there's an error
	}
	return float64(n.Int64()) / 1000.0
}
