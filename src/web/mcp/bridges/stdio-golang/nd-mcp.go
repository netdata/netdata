package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"log"
	"net/http"
	"os"

	"nhooyr.io/websocket"
)

func generateWebSocketKey() string {
	key := make([]byte, 16)
	_, err := rand.Read(key)
	if err != nil {
		log.Fatalf("failed to generate WebSocket key: %v", err)
	}
	return base64.StdEncoding.EncodeToString(key)
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintf(os.Stderr, "Usage: %s ws://host/path\n", os.Args[0])
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create a custom header with the WebSocket key
	header := http.Header{}
	header.Set("Sec-WebSocket-Key", generateWebSocketKey())
	header.Set("Sec-WebSocket-Version", "13")

	conn, _, err := websocket.Dial(ctx, os.Args[1], &websocket.DialOptions{
		CompressionMode: websocket.CompressionContextTakeover,
		HTTPHeader:      header,
	})
	if err != nil {
		log.Fatalf("websocket connection failed: %v", err)
	}
	defer conn.Close(websocket.StatusInternalError, "closing")

	errc := make(chan error, 2)

	// stdin → websocket
	go func() {
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			line := scanner.Text()
			// Send the raw text directly without JSON serialization
			if err := conn.Write(ctx, websocket.MessageText, []byte(line)); err != nil {
				errc <- fmt.Errorf("write to websocket: %w", err)
				return
			}
		}
		if err := scanner.Err(); err != nil {
			errc <- fmt.Errorf("stdin read error: %w", err)
		}
	}()

	// websocket → stdout
	go func() {
		for {
			// Read the raw message without JSON deserialization
			messageType, message, err := conn.Read(ctx)
			if err != nil {
				errc <- fmt.Errorf("read from websocket: %w", err)
				return
			}
			if messageType == websocket.MessageText {
				fmt.Println(string(message))
			}
		}
	}()

	if err := <-errc; err != nil {
		log.Fatalf("error: %v", err)
	}
	conn.Close(websocket.StatusNormalClosure, "")
}