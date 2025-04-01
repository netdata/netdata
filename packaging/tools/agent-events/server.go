package main

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/tidwall/gjson"
)

// --- Constants ---
const (
	maxRequestBodySize = 20 * 1024 // 20 KiB
)

// --- Custom Flag Type for Multi-use --dedup-key ---
type dedupPaths []string

func (d *dedupPaths) String() string { return fmt.Sprintf("%v", *d) }
func (d *dedupPaths) Set(value string) error {
	if value == "" {
		return fmt.Errorf("dedup-key path cannot be empty")
	}
	*d = append(*d, value)
	return nil
}

// --- Global variables ---
var (
	seenIDs        map[[32]byte]seenEntry
	mapMutex       = &sync.Mutex{}
	dedupWindow    time.Duration
	debugMode      bool
	keyPaths       dedupPaths
	dedupSeparator string
)

// --- Data Structures ---
type seenEntry struct {
	timestamp time.Time
}

// --- Core Logic Functions ---

// checkAndRecordHash accepts the SHA256 hash ([32]byte) for checking.
// It now REFRESHES the timestamp whenever a hash is found,
// effectively creating a sliding deduplication window.
func checkAndRecordHash(hash [32]byte) bool {
	now := time.Now()
	mapMutex.Lock()
	defer mapMutex.Unlock()

	var zeroHash [32]byte
	if hash == zeroHash {
		log.Println("Warning: checkAndRecordHash received potentially zero hash.")
		// Decide if zero hash should always be discarded, e.g. return false
	}

	// Check if the hash exists in the map
	entry, found := seenIDs[hash]

	if found {
		// --- Hash Found ---
		// Check if it was a duplicate based on the *previous* timestamp
		isRecentDuplicate := now.Sub(entry.timestamp) < dedupWindow

		// *** Always update the timestamp to 'now' to refresh the window ***
		seenIDs[hash] = seenEntry{timestamp: now}

		// Return 'false' if it was a recent duplicate (suppress processing),
		// return 'true' if it was found but expired (allow processing).
		return !isRecentDuplicate

	} else {
		// --- Hash Not Found ---
		// Record the new hash with the current timestamp
		seenIDs[hash] = seenEntry{timestamp: now}
		// Return 'true' as this is the first time (or first time after expiry)
		return true
	}
}


// cleanupExpiredEntries uses the hash ([32]byte) as the key type.
func cleanupExpiredEntries(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	cleanedCount := 0
	lastCleanupLogTime := time.Now()
	for range ticker.C {
		mapMutex.Lock()
		now := time.Now()
		for h, entry := range seenIDs {
			if now.Sub(entry.timestamp) >= dedupWindow {
				delete(seenIDs, h)
				cleanedCount++
			}
		}
		mapMutex.Unlock()
		// Simplified periodic logging for cleanup
		if cleanedCount > 0 && time.Since(lastCleanupLogTime) > time.Hour {
			if debugMode {
				log.Printf("Debug: Cleaned up %d expired entries in the past hour.", cleanedCount)
			}
			cleanedCount = 0 // Reset count after logging
			lastCleanupLogTime = time.Now()
		}
	}
}

// --- HTTP Handler ---
func handler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		log.Printf("Discarded: Method not allowed (%s) from %s", r.Method, r.RemoteAddr)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySize)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			http.Error(w, fmt.Sprintf("Request body exceeds limit (%d bytes)", maxRequestBodySize), http.StatusRequestEntityTooLarge)
			log.Printf("Discarded: Request body too large (limit %d bytes) from %s", maxRequestBodySize, r.RemoteAddr)
		} else {
			http.Error(w, "Error reading request", http.StatusInternalServerError)
			log.Printf("Discarded: Error reading request body: %v", err)
		}
		return
	}

	shouldProcess := true
	if len(keyPaths) > 0 {
		var keyBuilder strings.Builder
		for i, path := range keyPaths {
			result := gjson.GetBytes(body, path)
			var valueStr string
			if result.Exists() { valueStr = result.String() } else { valueStr = "" }
			keyBuilder.WriteString(valueStr)
			if i < len(keyPaths)-1 { keyBuilder.WriteString(dedupSeparator) }
		}
		finalKeyString := keyBuilder.String()
		dedupHash := sha256.Sum256([]byte(finalKeyString))
		if debugMode {
			log.Printf("Debug: Generated dedup key string: \"%s\"", finalKeyString)
			log.Printf("Debug: Generated dedup hash: %x", dedupHash)
		}

		// Call the updated checkAndRecordHash function
		if !checkAndRecordHash(dedupHash) {
			// It was determined to be a duplicate (based on previous timestamp)
			shouldProcess = false
			if debugMode { log.Printf("Debug: Discarded duplicate hash: %x (timestamp refreshed)", dedupHash) } // Updated log message
			// Respond OK for duplicate and stop processing
			if _, err := w.Write([]byte("OK")); err != nil { log.Printf("Error writing response after duplicate discard: %v", err) }
			return // Exit handler early for duplicates
		}
		// If we reach here, it was not a recent duplicate (new or expired)
	} else {
		if debugMode { log.Println("Debug: No --dedup-key flags provided, skipping deduplication.") }
	}

	if shouldProcess {
		var fullData interface{}
		if err := json.Unmarshal(body, &fullData); err != nil {
			http.Error(w, "Invalid JSON for full parsing", http.StatusBadRequest)
			bodyDetail := ""
			if debugMode { bodyDetail = fmt.Sprintf(", Body: %s", string(body)) } else { bodyDetail = fmt.Sprintf(", Body snippet: %s", limitString(string(body), 100)) }
			log.Printf("Discarded: Failed to fully parse JSON (post-dedup): %v%s", err, bodyDetail)
			return
		}
		outputBytes, err := json.Marshal(fullData)
		if err != nil {
			http.Error(w, "Internal Server Error during output marshal", http.StatusInternalServerError)
			log.Printf("Discarded: Failed to marshal JSON for output: %v", err)
			return
		}
		fmt.Println(string(outputBytes))
		if _, err := w.Write([]byte("OK")); err != nil { log.Printf("Error writing OK response: %v", err) }
	}
}

// --- Main Function ---
func main() {
	log.SetOutput(os.Stderr)
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	// --- Command Line Flags ---
	port := flag.Int("port", 8080, "Port to listen on")
	dedupSeconds := flag.Int("dedup-window", 1800, "Deduplication window in seconds (e.g., 1800 for 30 minutes)")
	flag.BoolVar(&debugMode, "debug", false, "Enable debug mode for verbose logging")
	flag.Var(&keyPaths, "dedup-key", "JSON path (dot-notation) for deduplication key (can be used multiple times)")
	flag.StringVar(&dedupSeparator, "dedup-separator", "-", "Separator used between values from multiple --dedup-key paths")
	flag.Parse()

	seenIDs = make(map[[32]byte]seenEntry)
	dedupWindow = time.Duration(*dedupSeconds) * time.Second

	if dedupWindow > 0 && len(keyPaths) > 0 {
		cleanupInterval := dedupWindow / 10
		if cleanupInterval < 1*time.Minute { cleanupInterval = 1 * time.Minute } else if cleanupInterval > 15*time.Minute { cleanupInterval = 15 * time.Minute }
		log.Printf("Cleanup goroutine started. Interval: %v", cleanupInterval)
		go cleanupExpiredEntries(cleanupInterval)
	} else if dedupWindow <= 0 && len(keyPaths) > 0 {
        log.Println("Warning: Deduplication keys provided, but window is zero or negative. Deduplication effectively disabled.")
    }

	// --- Configure HTTP Server ---
	readTimeout := 10 * time.Second
	writeTimeout := 10 * time.Second
	idleTimeout := 60 * time.Second
	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", *port),
		Handler:      http.DefaultServeMux,
		ReadTimeout:  readTimeout,
		WriteTimeout: writeTimeout,
		IdleTimeout:  idleTimeout,
	}
	http.HandleFunc("/", handler)

	// --- Start Server ---
	log.Printf("Server listening on port %d", *port)
	log.Printf("Maximum request body size: %d bytes", maxRequestBodySize)
	if len(keyPaths) > 0 {
		log.Printf("Deduplication enabled: Keys=%v, Separator='%s', Window=%v (Sliding window: timestamp refreshed on duplicate)", keyPaths, dedupSeparator, dedupWindow) // Updated log message
	} else {
		log.Println("Deduplication disabled (no --dedup-key specified).")
	}
	log.Printf("Debug mode enabled: %t", debugMode)
	log.Printf("Server timeouts -> Read: %v, Write: %v, Idle: %v", readTimeout, writeTimeout, idleTimeout)
	log.Fatal(server.ListenAndServe())
}

// --- Helper Functions ---
func limitString(s string, maxLen int) string {
	if len(s) <= maxLen { return s }
	return s[:maxLen] + "..."
}
