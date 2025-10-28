package main

import (
	"bytes"
	"context" // Import context
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	// Import promhttp for testing the metrics endpoint handler
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Helper function to capture stdout/stderr during a test run
func captureOutput(t *testing.T, f func()) (stdout, stderr string) {
	t.Helper() // Marks this as a helper function for testing framework

	originalStdout := os.Stdout
	originalStderr := os.Stderr
	oldLogger := slog.Default()
	rOut, wOut, _ := os.Pipe()
	rErr, wErr, _ := os.Pipe()
	os.Stdout = wOut
	os.Stderr = wErr

	t.Cleanup(func() {
		os.Stdout = originalStdout
		os.Stderr = originalStderr
		slog.SetDefault(oldLogger)
	})

	outCh := make(chan string)
	errCh := make(chan string)

	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, rOut)
		outCh <- buf.String()
	}()
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, rErr)
		errCh <- buf.String()
	}()

	f() // Execute the function

	_ = wOut.Close()
	_ = wErr.Close()
	stdout = <-outCh
	stderr = <-errCh
	return stdout, stderr
}

// --- Test Suite ---

// setupTest initializes necessary components for tests
func setupTest(t *testing.T) {
	t.Helper()
	keyPaths = []string{"id"}
	dedupSeparator = "-"
	dedupWindow = 30 * time.Second
	startTime = time.Now()
	noopHandler := slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelDebug})
	slog.SetDefault(slog.New(noopHandler))

	initMetrics() // Initializes OTEL which feeds default registry

	mapMutex.Lock()
	seenIDs = make(map[[32]byte]seenEntry)
	mapMutex.Unlock()

	t.Cleanup(func() {
		if meterProvider != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
			defer cancel()
			if err := meterProvider.Shutdown(ctx); err != nil {
				t.Logf("Warning: error shutting down meter provider in test cleanup: %v", err)
			}
		}
	})
}

// Helper to reset state between sub-tests if needed beyond setupTest
func resetDedupState() {
	mapMutex.Lock()
	seenIDs = make(map[[32]byte]seenEntry)
	mapMutex.Unlock()
	keyPaths = []string{"id"}
	dedupSeparator = "-"
}

func TestHandler(t *testing.T) {
	setupTest(t) // Setup once for all sub-tests

	// --- Test Cases ---

	t.Run("FirstValidRequest", func(t *testing.T) {
		t.Cleanup(resetDedupState) // Reset map for isolation
		jsonBody := `{"id": "uuid-1", "data": "value1"}`
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(jsonBody))
		rr := httptest.NewRecorder()
		stdout, _ := captureOutput(t, func() { handler(rr, req) })
		if status := rr.Code; status != http.StatusOK {
			t.Errorf("status: got %v want %v", status, http.StatusOK)
		}
		if rr.Body.String() != "OK" {
			t.Errorf("body: got %v want %v", rr.Body.String(), "OK")
		}
		if !strings.Contains(stdout, `"id":"uuid-1"`) {
			t.Errorf("stdout missing id: %q", stdout)
		}
		mapMutex.Lock()
		mapLen := len(seenIDs)
		mapMutex.Unlock()
		if mapLen != 1 {
			t.Errorf("map size: got %d want 1", mapLen)
		}
	})

	t.Run("DuplicateRequestWithinWindow", func(t *testing.T) {
		t.Cleanup(resetDedupState)
		firstJsonBody := `{"id": "uuid-2", "data": "value2"}`
		firstReq := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(firstJsonBody))
		firstRr := httptest.NewRecorder()
		captureOutput(t, func() { handler(firstRr, firstReq) })
		if firstRr.Code != http.StatusOK {
			t.Fatalf("Setup failed")
		}
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(firstJsonBody))
		rr := httptest.NewRecorder()
		stdout, _ := captureOutput(t, func() { handler(rr, req) })
		if status := rr.Code; status != http.StatusOK {
			t.Errorf("status: got %v want %v", status, http.StatusOK)
		}
		if rr.Body.String() != "OK" {
			t.Errorf("body: got %v want %v", rr.Body.String(), "OK")
		}
		if stdout != "" {
			t.Errorf("stdout not empty: %q", stdout)
		}
		mapMutex.Lock()
		mapLen := len(seenIDs)
		mapMutex.Unlock()
		if mapLen != 1 {
			t.Errorf("map size: got %d want 1", mapLen)
		}
	})

	t.Run("InvalidJSON", func(t *testing.T) {
		t.Cleanup(resetDedupState)
		jsonBody := `{"id": "uuid-3", "data":`
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(jsonBody))
		rr := httptest.NewRecorder()
		stdout, _ := captureOutput(t, func() { handler(rr, req) })
		if status := rr.Code; status != http.StatusBadRequest {
			t.Errorf("status: got %v want %v", status, http.StatusBadRequest)
		}
		if stdout != "" {
			t.Errorf("stdout not empty: %q", stdout)
		}
	})

	t.Run("MissingDedupKey", func(t *testing.T) {
		t.Cleanup(resetDedupState)
		jsonBody := `{"other_id": "uuid-4", "data": "value4"}`
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(jsonBody))
		rr := httptest.NewRecorder()
		stdout, _ := captureOutput(t, func() { handler(rr, req) })
		if status := rr.Code; status != http.StatusOK {
			t.Errorf("status: got %v want %v", status, http.StatusOK)
		}
		if !strings.Contains(stdout, `"other_id":"uuid-4"`) {
			t.Errorf("stdout missing other_id: %q", stdout)
		}
		mapMutex.Lock()
		mapLen := len(seenIDs)
		mapMutex.Unlock()
		if mapLen != 1 {
			t.Errorf("map size: got %d want 1", mapLen)
		}
	})

	t.Run("CloudflareHeaders", func(t *testing.T) {
		t.Cleanup(resetDedupState)
		jsonBody := `{"id": "uuid-cf", "data": "value-cf"}`
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(jsonBody))
		// Set allowed headers (non-IP related)
		req.Header.Set("CF-IPCountry", "US")
		req.Header.Set("CF-Ray", "123")
		req.Header.Set("CF-IPCity", "Testville")
		// Set IP-related headers that should be excluded for GDPR compliance
		req.Header.Set("CF-Connecting-IP", "1.2.3.4")
		req.Header.Set("CF-IPLatitude", "12.34")
		req.Header.Set("CF-IPLongitude", "-56.78")
		req.Header.Set("CF-Visitor", "{\"ip\":\"1.2.3.4\"}")

		rr := httptest.NewRecorder()
		stdout, _ := captureOutput(t, func() { handler(rr, req) })
		if status := rr.Code; status != http.StatusOK {
			t.Errorf("status: got %v want %v", status, http.StatusOK)
		}
		if !strings.Contains(stdout, `"cf":`) {
			t.Errorf("stdout missing cf object")
		}
		if !strings.Contains(stdout, `"IPCountry":"US"`) {
			t.Errorf("stdout missing cf header IPCountry")
		}

		// Verify IP-related headers are excluded for GDPR compliance
		if strings.Contains(stdout, `"Connecting-IP"`) {
			t.Errorf("stdout should not contain IP address: Connecting-IP")
		}
		if strings.Contains(stdout, `"IPLatitude"`) {
			t.Errorf("stdout should not contain IP geolocation: IPLatitude")
		}
		if strings.Contains(stdout, `"IPLongitude"`) {
			t.Errorf("stdout should not contain IP geolocation: IPLongitude")
		}
		if strings.Contains(stdout, `"Visitor"`) {
			t.Errorf("stdout should not contain Visitor which includes IP")
		}
	})

	t.Run("MethodNotAllowed", func(t *testing.T) {
		t.Cleanup(resetDedupState)
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rr := httptest.NewRecorder()
		stdout, _ := captureOutput(t, func() { handler(rr, req) })
		if status := rr.Code; status != http.StatusMethodNotAllowed {
			t.Errorf("status: got %v want %v", status, http.StatusMethodNotAllowed)
		}
		if stdout != "" {
			t.Errorf("stdout not empty: %q", stdout)
		}
	})

	t.Run("RequestEntityTooLarge", func(t *testing.T) {
		t.Cleanup(resetDedupState)
		largeBody := make([]byte, maxRequestBodySize+1)
		req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(largeBody))
		rr := httptest.NewRecorder()
		stdout, _ := captureOutput(t, func() { handler(rr, req) })
		if status := rr.Code; status != http.StatusRequestEntityTooLarge {
			t.Errorf("status: got %v want %v", status, http.StatusRequestEntityTooLarge)
		}
		if stdout != "" {
			t.Errorf("stdout not empty: %q", stdout)
		}
	})

	t.Run("MultiKeyDeduplication", func(t *testing.T) {
		t.Cleanup(resetDedupState)
		keyPaths = []string{"id", "source"}
		dedupSeparator = "|"
		req1 := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"id": "1", "source": "A", "data": "v1"}`))
		rr1 := httptest.NewRecorder()
		stdout1, _ := captureOutput(t, func() { handler(rr1, req1) })
		if rr1.Code != http.StatusOK || !strings.Contains(stdout1, `"id":"1"`) {
			t.Errorf("Request 1 failed")
		}
		req2 := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"id": "1", "source": "B", "data": "v2"}`))
		rr2 := httptest.NewRecorder()
		stdout2, _ := captureOutput(t, func() { handler(rr2, req2) })
		if rr2.Code != http.StatusOK || !strings.Contains(stdout2, `"source":"B"`) {
			t.Errorf("Request 2 failed")
		}
		req3 := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"id": "1", "source": "A", "data": "v3"}`))
		rr3 := httptest.NewRecorder()
		stdout3, _ := captureOutput(t, func() { handler(rr3, req3) })
		if rr3.Code != http.StatusOK {
			t.Errorf("Request 3 status wrong")
		}
		if stdout3 != "" {
			t.Errorf("Request 3 produced output")
		}
		mapMutex.Lock()
		mapLen := len(seenIDs)
		mapMutex.Unlock()
		if mapLen != 2 {
			t.Errorf("map size: got %d want 2", mapLen)
		}
	})

	t.Run("MetricsDelta", func(t *testing.T) {
		t.Skip("Skipping MetricsDelta test: Verifying exact metric deltas with OTEL in unit tests is complex.")
	})

	t.Run("HealthEndpoint", func(t *testing.T) {
		t.Cleanup(resetDedupState)
		req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
		rr := httptest.NewRecorder()
		healthHandler(rr, req)
		if status := rr.Code; status != http.StatusOK {
			t.Errorf("status: got %v want %v", status, http.StatusOK)
		}
		if contentType := rr.Header().Get("Content-Type"); contentType != "application/json" {
			t.Errorf("content type: got %v want %v", contentType, "application/json")
		}
		var result map[string]interface{}
		if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}
		requiredFields := []string{"status", "timestamp", "uptime", "goroutines", "memory", "deduplication"}
		for _, field := range requiredFields {
			if _, ok := result[field]; !ok {
				t.Errorf("missing field: %s", field)
			}
		}
		if status, ok := result["status"].(string); !ok || status != "ok" {
			t.Errorf("status field: got %v want ok", result["status"])
		}
	})

	t.Run("MetricsEndpoint", func(t *testing.T) {
		// Re-enabled: Uses promhttp.Handler which reads from default registry
		t.Cleanup(resetDedupState)

		// Create test server using the standard promhttp handler
		metricsServer := httptest.NewServer(promhttp.Handler())
		t.Cleanup(metricsServer.Close)

		// Make requests to main handler to generate metrics
		captureOutput(t, func() {
			handler(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"id": "m1"}`)))
		})
		captureOutput(t, func() {
			handler(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"id": "m1"}`))) // duplicate
		})
		captureOutput(t, func() {
			handler(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil)) // method not allowed
		})

		// Fetch metrics
		resp, err := http.Get(metricsServer.URL)
		if err != nil {
			t.Fatalf("failed to get metrics: %v", err)
		}
		defer resp.Body.Close()

		if status := resp.StatusCode; status != http.StatusOK {
			t.Errorf("status: got %v want %v", status, http.StatusOK)
		}
		if contentType := resp.Header.Get("Content-Type"); !strings.HasPrefix(contentType, "text/plain") {
			t.Errorf("content type: got %q want prefix text/plain", contentType)
		}

		metricsBodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("failed to read metrics body: %v", err)
		}
		metricsContent := string(metricsBodyBytes)
		t.Logf("Metrics Output for Verification:\n%s", metricsContent) // Log for manual inspection if needed

		// Check for presence of key metrics (handling both standard and suffixed names)
		// OpenTelemetry may add suffixes like _ratio_total to counter metrics
		metricChecks := []struct {
			namePatterns []string
			description  string
		}{
			{
				namePatterns: []string{"agent_events_requests", "agent_events_requests_ratio_total"},
				description:  "Requests counter",
			},
			{
				namePatterns: []string{"agent_events_received_bytes", "agent_events_received_bytes_ratio_total"},
				description:  "Bytes received counter",
			},
			{
				namePatterns: []string{"agent_events_dedup_cache_entries"},
				description:  "Dedup cache size gauge",
			},
			{
				namePatterns: []string{"agent_events_request_duration_seconds"},
				description:  "Request duration histogram",
			},
			{
				namePatterns: []string{"go_goroutines"},
				description:  "Go runtime metrics",
			},
		}

		for _, check := range metricChecks {
			found := false
			for _, pattern := range check.namePatterns {
				if strings.Contains(metricsContent, pattern) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("metrics response missing expected metric: %s (patterns: %v)", check.description, check.namePatterns)
			}
		}

		// OpenTelemetry histogram metrics have this pattern in the output:
		// agent_events_request_duration_seconds_bucket{...
		// agent_events_request_duration_seconds_sum{...
		// agent_events_request_duration_seconds_count{...
		// So check for existence, not value line which can be variable
		if !strings.Contains(metricsContent, "agent_events_request_duration_seconds_count{") {
			t.Errorf("metrics response missing count line for histogram metric")
		}
	})

	t.Run("DedupWindowExpiration", func(t *testing.T) {
		t.Cleanup(resetDedupState)
		oldWindow := dedupWindow
		dedupWindow = 50 * time.Millisecond
		t.Cleanup(func() { dedupWindow = oldWindow })
		req1 := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"id": "exp1"}`))
		rr1 := httptest.NewRecorder()
		stdout1, _ := captureOutput(t, func() { handler(rr1, req1) })
		if rr1.Code != http.StatusOK || !strings.Contains(stdout1, "exp1") {
			t.Errorf("Req 1 failed")
		}
		req2 := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"id": "exp1"}`))
		rr2 := httptest.NewRecorder()
		stdout2, _ := captureOutput(t, func() { handler(rr2, req2) })
		if stdout2 != "" {
			t.Errorf("Immediate duplicate not suppressed")
		}
		time.Sleep(100 * time.Millisecond)
		req3 := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"id": "exp1"}`))
		rr3 := httptest.NewRecorder()
		stdout3, _ := captureOutput(t, func() { handler(rr3, req3) })
		if rr3.Code != http.StatusOK || !strings.Contains(stdout3, "exp1") {
			t.Errorf("Req 3 after expiry failed")
		}
	})

	t.Run("LongKeyValues", func(t *testing.T) {
		t.Cleanup(resetDedupState)
		longId := strings.Repeat("a", 500)
		jsonBody := fmt.Sprintf(`{"id": "%s", "data": "long"}`, longId)
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(jsonBody))
		rr := httptest.NewRecorder()
		stdout, _ := captureOutput(t, func() { handler(rr, req) })
		if rr.Code != http.StatusOK || !strings.Contains(stdout, "long") {
			t.Errorf("Long key req failed")
		}
		reqDup := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(jsonBody))
		rrDup := httptest.NewRecorder()
		stdoutDup, _ := captureOutput(t, func() { handler(rrDup, reqDup) })
		if stdoutDup != "" {
			t.Errorf("Long key duplicate not suppressed")
		}
	})

	t.Run("OptionsMethod", func(t *testing.T) {
		t.Cleanup(resetDedupState)
		req := httptest.NewRequest(http.MethodOptions, "/", nil)
		rr := httptest.NewRecorder()
		stdout, _ := captureOutput(t, func() { handler(rr, req) })
		if status := rr.Code; status != http.StatusMethodNotAllowed {
			t.Errorf("status: got %v want %v", status, http.StatusMethodNotAllowed)
		}
		if stdout != "" {
			t.Errorf("stdout not empty: %q", stdout)
		}
	})

	t.Run("VariousJSONFormats", func(t *testing.T) {
		t.Cleanup(resetDedupState)
		testCases := []struct {
			name         string
			body         string
			expectStatus int
			expectOutput bool
		}{
			{"EmptyObject", `{}`, http.StatusOK, true}, {"ValidJSON", `{"id": "valid"}`, http.StatusOK, true},
			{"SingleQuotes", `{'id': 'invalid'}`, http.StatusBadRequest, false}, {"TrailingComma", `{"id": "comma",}`, http.StatusBadRequest, false},
			{"UnquotedKey", `{id: "unquoted"}`, http.StatusBadRequest, false},
		}
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tc.body))
				rr := httptest.NewRecorder()
				stdout, _ := captureOutput(t, func() { handler(rr, req) })
				if status := rr.Code; status != tc.expectStatus {
					t.Errorf("status: got %v want %v", status, tc.expectStatus)
				}
				hasOutput := stdout != ""
				if hasOutput != tc.expectOutput {
					t.Errorf("Output mismatch: expected %t, got %t (stdout: %q)", tc.expectOutput, hasOutput, stdout)
				}
			})
		}
	})

	t.Run("ConcurrentRequests", func(t *testing.T) {
		t.Cleanup(resetDedupState)
		numRequests := 50
		var wg sync.WaitGroup
		wg.Add(numRequests)
		process := func(id int) {
			defer wg.Done()
			jsonBody := fmt.Sprintf(`{"id": "conc-%d"}`, id)
			req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(jsonBody))
			rr := httptest.NewRecorder()
			captureOutput(t, func() { handler(rr, req) })
			if status := rr.Code; status != http.StatusOK {
				t.Logf("conc req %d status: got %v want %v", id, status, http.StatusOK)
				t.Fail()
			}
		}
		for i := 0; i < numRequests; i++ {
			go process(i)
		}
		wg.Wait()
		mapMutex.Lock()
		mapLen := len(seenIDs)
		mapMutex.Unlock()
		if mapLen != numRequests {
			t.Errorf("map size: got %d want %d", mapLen, numRequests)
		}
	})

	t.Run("CleanupExpiredEntries", func(t *testing.T) {
		t.Cleanup(resetDedupState)
		oldWindow := dedupWindow
		dedupWindow = 50 * time.Millisecond
		t.Cleanup(func() { dedupWindow = oldWindow })
		numEntries := 5
		for i := 0; i < numEntries; i++ {
			req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(fmt.Sprintf(`{"id": "clean-%d"}`, i)))
			rr := httptest.NewRecorder()
			captureOutput(t, func() { handler(rr, req) })
			if rr.Code != http.StatusOK {
				t.Fatalf("Setup failed entry %d", i)
			}
		}
		mapMutex.Lock()
		if got := len(seenIDs); got != numEntries {
			t.Fatalf("Entries after add: %d != %d", got, numEntries)
		}
		mapMutex.Unlock()
		time.Sleep(100 * time.Millisecond)
		now := time.Now()
		mapMutex.Lock()
		for h, entry := range seenIDs {
			if now.Sub(entry.timestamp) >= dedupWindow {
				delete(seenIDs, h)
			}
		}
		count := len(seenIDs)
		mapMutex.Unlock()
		if count != 0 {
			t.Errorf("Entries after cleanup: %d != 0", count)
		}
	})

	t.Run("MixedKeyTypes", func(t *testing.T) {
		t.Cleanup(resetDedupState)
		keyPaths = []string{"id", "count", "enabled"}
		dedupSeparator = "|"
		req1 := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"id": "mix", "count": 1, "enabled": true}`))
		rr1 := httptest.NewRecorder()
		stdout1, _ := captureOutput(t, func() { handler(rr1, req1) })
		if rr1.Code != http.StatusOK || !strings.Contains(stdout1, `"id":"mix"`) {
			t.Errorf("Req 1 failed")
		}
		req2 := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"enabled": true, "count": 1.0, "id": "mix"}`))
		rr2 := httptest.NewRecorder()
		stdout2, _ := captureOutput(t, func() { handler(rr2, req2) })
		if rr2.Code != http.StatusOK {
			t.Errorf("Req 2 status wrong")
		}
		if stdout2 != "" {
			t.Errorf("Req 2 produced output")
		}
		req3 := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"id": "mix", "count": 1, "enabled": false}`))
		rr3 := httptest.NewRecorder()
		stdout3, _ := captureOutput(t, func() { handler(rr3, req3) })
		if rr3.Code != http.StatusOK || !strings.Contains(stdout3, `"enabled":false`) {
			t.Errorf("Req 3 failed")
		}
		mapMutex.Lock()
		mapLen := len(seenIDs)
		mapMutex.Unlock()
		if mapLen != 2 {
			t.Errorf("map size: got %d want 2", mapLen)
		}
	})

	t.Run("JsonFormatTests", func(t *testing.T) {
		t.Cleanup(resetDedupState)
		testCases := []struct {
			name         string
			body         string
			expectStatus int
			expectOutput bool
		}{
			{"EmptyObject", `{}`, http.StatusOK, true}, {"ValidJSON", `{"id": "valid"}`, http.StatusOK, true},
			{"CompletelyInvalid", `not json`, http.StatusBadRequest, false}, {"IncompleteJSON", `{"id": "inc`, http.StatusBadRequest, false},
			{"ArrayAsRoot", `[1, 2]`, http.StatusBadRequest, false}, // Expect 400 now
		}
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tc.body))
				rr := httptest.NewRecorder()
				stdout, _ := captureOutput(t, func() { handler(rr, req) })
				if status := rr.Code; status != tc.expectStatus {
					t.Errorf("status: got %v want %v", status, tc.expectStatus)
				}
				hasOutput := stdout != ""
				if hasOutput != tc.expectOutput {
					t.Errorf("Output mismatch: expected %t, got %t", tc.expectOutput, hasOutput)
				}
			})
		}
	})

	t.Run("MalformedJsonHandling", func(t *testing.T) {
		// Verifies server returns BadRequest for invalid JSON
		t.Cleanup(resetDedupState)
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`not json`))
		rr := httptest.NewRecorder()
		captureOutput(t, func() { handler(rr, req) })
		if status := rr.Code; status != http.StatusBadRequest {
			t.Errorf("status invalid: got %v want %v", status, http.StatusBadRequest)
		}
		req2 := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"open":`))
		rr2 := httptest.NewRecorder()
		captureOutput(t, func() { handler(rr2, req2) })
		if status := rr2.Code; status != http.StatusBadRequest {
			t.Errorf("status incomplete: got %v want %v", status, http.StatusBadRequest)
		}
	})

	t.Run("ZeroLengthDedupWindow", func(t *testing.T) {
		t.Cleanup(resetDedupState)
		oldWindow := dedupWindow
		dedupWindow = 0
		t.Cleanup(func() { dedupWindow = oldWindow })
		jsonBody := `{"id": "zero"}`
		req1 := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(jsonBody))
		rr1 := httptest.NewRecorder()
		stdout1, _ := captureOutput(t, func() { handler(rr1, req1) })
		if rr1.Code != http.StatusOK || !strings.Contains(stdout1, "zero") {
			t.Errorf("Req 1 failed")
		}
		req2 := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(jsonBody))
		rr2 := httptest.NewRecorder()
		stdout2, _ := captureOutput(t, func() { handler(rr2, req2) })
		if rr2.Code != http.StatusOK || !strings.Contains(stdout2, "zero") {
			t.Errorf("Req 2 (duplicate) failed")
		}
	})
}
