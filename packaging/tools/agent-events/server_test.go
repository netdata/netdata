package main

import (
	"bytes"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

// Helper function to capture stdout/stderr during a test run
func captureOutput(t *testing.T, f func()) (stdout, stderr string) {
	t.Helper() // Marks this as a helper function for testing framework

	originalStdout := os.Stdout
	originalStderr := os.Stderr
	originalLogOutput := log.Writer() // Get current log output writer

	// Create pipes to capture output
	rOut, wOut, _ := os.Pipe()
	rErr, wErr, _ := os.Pipe()

	// Redirect stdout and stderr
	os.Stdout = wOut
	os.Stderr = wErr
	log.SetOutput(wErr) // Redirect default logger to stderr pipe

	// Use t.Cleanup to ensure restoration even if the test panics
	t.Cleanup(func() {
		os.Stdout = originalStdout
		os.Stderr = originalStderr
		log.SetOutput(originalLogOutput) // Restore original log output
	})

	// Channels to signal when reading is done
	outCh := make(chan string)
	errCh := make(chan string)

	// Goroutine to read stdout
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, rOut)
		outCh <- buf.String()
	}()

	// Goroutine to read stderr
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, rErr)
		errCh <- buf.String()
	}()

	// --- Execute the function under test ---
	f()
	// ---                                ---

	// Close the writers to signal EOF to the readers
	_ = wOut.Close()
	_ = wErr.Close()

	// Read captured output
	stdout = <-outCh
	stderr = <-errCh

	// Optional: Print captured output via test logger if needed for debugging
	// t.Logf("Captured Stdout:\n%s", stdout)
	// t.Logf("Captured Stderr:\n%s", stderr)

	return stdout, stderr
}

// --- Test Suite ---

func TestHandler(t *testing.T) {
	// --- Test Setup ---
	// Configure global variables for the tests
	keyPaths = []string{"id"} // Simple dedup key for testing
	dedupSeparator = "-"
	dedupWindow = 30 * time.Second // Use a reasonable window for tests
	debugMode = false              // Start with debug off, can enable per test case

	// Ensure the map is initialized and clean before starting tests
	mapMutex.Lock()
	seenIDs = make(map[[32]byte]seenEntry)
	mapMutex.Unlock()

	// Helper to reset state between sub-tests
	resetState := func() {
		mapMutex.Lock()
		seenIDs = make(map[[32]byte]seenEntry) // Clear the map
		mapMutex.Unlock()
		keyPaths = []string{"id"} // Reset paths just in case
		debugMode = false
		// Reset other globals if they were modified
	}

	// --- Test Cases ---

	t.Run("FirstValidRequest", func(t *testing.T) {
		t.Cleanup(resetState) // Ensure state is reset after this sub-test

		// Prepare request
		jsonBody := `{"id": "uuid-1", "data": "value1"}`
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(jsonBody))
		rr := httptest.NewRecorder() // Records the HTTP response

		// Execute handler and capture output
		stdout, stderr := captureOutput(t, func() {
			handler(rr, req)
		})

		// Assertions
		if status := rr.Code; status != http.StatusOK {
			t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
		}
		expectedResponse := `OK`
		if rr.Body.String() != expectedResponse {
			t.Errorf("handler returned unexpected body: got %v want %v", rr.Body.String(), expectedResponse)
		}
		// Check stdout contains the *exact* JSON (json.Marshal might reorder fields)
		// A simpler check is that it's not empty and maybe contains key parts.
		// For exact match, we'd need to unmarshal stdout and compare.
		if !strings.Contains(stdout, `"id":"uuid-1"`) || !strings.Contains(stdout, `"data":"value1"`) {
			t.Errorf("handler produced unexpected stdout:\ngot: %q\nwant it to contain parts of: %q", stdout, jsonBody)
		}
		if stderr != "" {
			t.Errorf("handler produced unexpected stderr: got %q want empty", stderr)
		}

		// Check internal state (optional, needs mutex)
		mapMutex.Lock()
		if len(seenIDs) != 1 {
			t.Errorf("expected 1 entry in seenIDs map, got %d", len(seenIDs))
		}
		mapMutex.Unlock()
	})

	t.Run("DuplicateRequestWithinWindow", func(t *testing.T) {
		t.Cleanup(resetState)

		// --- Setup: Simulate the first request having happened ---
		firstJsonBody := `{"id": "uuid-2", "data": "value2"}`
		firstReq := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(firstJsonBody))
		firstRr := httptest.NewRecorder()
		// Run handler once, ignore output for this setup run
		captureOutput(t, func() { handler(firstRr, firstReq) })
		if firstRr.Code != http.StatusOK {
			t.Fatalf("Setup failed: first request did not return OK")
		}
		// Verify setup placed item in map
		mapMutex.Lock()
		if len(seenIDs) != 1 {
			t.Fatalf("Setup failed: map size not 1 after first request")
		}
		mapMutex.Unlock()
		// --- End Setup ---


		// Prepare the duplicate request
		// Note: Using the *same* body string as the first request
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(firstJsonBody))
		rr := httptest.NewRecorder()

		// Execute handler and capture output
		stdout, stderr := captureOutput(t, func() {
			handler(rr, req)
		})

		// Assertions
		if status := rr.Code; status != http.StatusOK {
			t.Errorf("handler returned wrong status code for duplicate: got %v want %v", status, http.StatusOK)
		}
		expectedResponse := `OK`
		if rr.Body.String() != expectedResponse {
			t.Errorf("handler returned unexpected body for duplicate: got %v want %v", rr.Body.String(), expectedResponse)
		}
		// Stdout should be empty for a duplicate
		if stdout != "" {
			t.Errorf("handler produced unexpected stdout for duplicate: got %q want empty", stdout)
		}
		// Stderr should be empty (unless debug mode logs duplicates)
		if stderr != "" {
			t.Errorf("handler produced unexpected stderr for duplicate: got %q want empty", stderr)
		}
        // Map size should remain 1
        mapMutex.Lock()
		if len(seenIDs) != 1 {
			t.Errorf("expected 1 entry in seenIDs map after duplicate, got %d", len(seenIDs))
		}
		mapMutex.Unlock()
	})

	t.Run("InvalidJSON", func(t *testing.T) {
		t.Cleanup(resetState)

		// Prepare request
		jsonBody := `{"id": "uuid-3", "data":` // Invalid JSON
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(jsonBody))
		rr := httptest.NewRecorder()

		// Execute handler and capture output
		stdout, stderr := captureOutput(t, func() {
			handler(rr, req)
		})

		// Assertions
		if status := rr.Code; status != http.StatusBadRequest {
			t.Errorf("handler returned wrong status code for invalid JSON: got %v want %v", status, http.StatusBadRequest)
		}
		// Stdout should be empty
		if stdout != "" {
			t.Errorf("handler produced unexpected stdout for invalid JSON: got %q want empty", stdout)
		}
		// Stderr should contain the parsing error log message
		if !strings.Contains(stderr, "Failed to fully parse JSON") {
			t.Errorf("handler did not produce expected stderr log for invalid JSON: got %q", stderr)
		}
	})

    t.Run("MissingDedupKey", func(t *testing.T) {
        t.Cleanup(resetState)

		// Prepare request - JSON is valid but missing the 'id' field used by keyPaths
		jsonBody := `{"other_id": "uuid-4", "data": "value4"}`
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(jsonBody))
		rr := httptest.NewRecorder()

		// Execute handler and capture output
		stdout, stderr := captureOutput(t, func() {
			handler(rr, req)
		})

		// Assertions for missing key (results in empty string "" for the key part)
		// This *should* be processed correctly, as "" is a valid key string before hashing
        // The hash of "" will be deduplicated like any other hash.

		if status := rr.Code; status != http.StatusOK {
			t.Errorf("handler returned wrong status code for missing key: got %v want %v", status, http.StatusOK)
		}
        // Stdout should contain the JSON
        if !strings.Contains(stdout, `"other_id":"uuid-4"`) || !strings.Contains(stdout, `"data":"value4"`) {
			t.Errorf("handler produced unexpected stdout for missing key:\ngot: %q\nwant it to contain parts of: %q", stdout, jsonBody)
		}
		if stderr != "" {
			t.Errorf("handler produced unexpected stderr for missing key: got %q want empty", stderr)
		}
        // Check map (hash of "" should be present)
        mapMutex.Lock()
		if len(seenIDs) != 1 {
			t.Errorf("expected 1 entry in seenIDs map for missing key, got %d", len(seenIDs))
		}
		mapMutex.Unlock()
	})

	// Add more test cases: Wrong method (GET), expired duplicate, multiple dedup keys, etc.
}
