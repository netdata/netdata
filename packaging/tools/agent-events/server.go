package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
)

func handler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		log.Printf("Method not allowed: %s\n", r.Method)
		return
	}

	// Read the entire request body.
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Error reading request", http.StatusInternalServerError)
		log.Printf("Error reading request: %v\n", err)
		return
	}
	defer r.Body.Close()

	// Remove all newline characters.
	cleaned := strings.ReplaceAll(string(body), "\n", "")
	cleaned = strings.ReplaceAll(cleaned, "\r", "")

	// Write to stdout with a single newline at the end.
	fmt.Println(cleaned)

	// Respond with OK.
	if _, err := w.Write([]byte("OK")); err != nil {
		log.Printf("Error writing response: %v\n", err)
		return
	}
}

func main() {
	// Configure logging to write to stderr.
	log.SetOutput(os.Stderr)
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	// Parse the port from the command line.
	port := flag.Int("port", 8080, "Port to listen on")
	flag.Parse()

	http.HandleFunc("/", handler)
	log.Printf("Server listening on port %d\n", *port)
	if err := http.ListenAndServe(fmt.Sprintf(":%d", *port), nil); err != nil {
		log.Fatalf("Server failed: %v\n", err)
	}
}
