package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"expvar"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	_ "net/http/pprof" // Import for side effects - registers HTTP handlers
	"os"
	"os/signal"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	// Import promhttp for the handler
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/tidwall/gjson"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
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
	// Core functionality variables
	seenIDs        map[[32]byte]seenEntry
	mapMutex       = &sync.Mutex{}
	dedupWindow    time.Duration
	keyPaths       dedupPaths
	dedupSeparator string
	startTime      time.Time
	dedupLogger    *os.File
	dedupLogFile   string // Store the deduplication log file path

	// Track active connections for graceful shutdown
	activeConnections int32

	// OpenTelemetry meter provider and metrics
	meterProvider *sdkmetric.MeterProvider
	meter         metric.Meter

	// Counters
	requestsCounter metric.Int64Counter // Unified counter with status label
	bytesReceived   metric.Int64Counter

	// Gauges
	dedupCacheSize         metric.Int64ObservableGauge
	activeConnectionsGauge metric.Int64ObservableGauge
	uptimeGauge            metric.Int64ObservableGauge

	// Histograms
	requestDuration metric.Float64Histogram

	// Keep expvar for backward compatibility
	eventMetrics = expvar.NewMap("agent_events")
)

// --- Data Structures ---
type seenEntry struct {
	timestamp time.Time
}

// --- Core Logic Functions ---

// initMetrics initializes OpenTelemetry metrics and expvar metrics for backward compatibility
func initMetrics() (*prometheus.Exporter, error) {
	// Set up the OpenTelemetry Prometheus exporter
	exporter, err := prometheus.New()
	if err != nil {
		// Return error instead of exiting directly
		return nil, fmt.Errorf("failed to create Prometheus exporter: %w", err)
	}

	// Create a new meter provider with the Prometheus exporter as a Reader
	meterProvider = sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(exporter),
	)
	otel.SetMeterProvider(meterProvider)

	// Create a new meter
	meter = meterProvider.Meter("agent-events")

	// --- Create Metric Instruments ---
	// Helper to create counters with status labels for consolidated metrics
	createLabeledCounter := func(name, desc string) (metric.Int64Counter, error) {
		counter, err := meter.Int64Counter(
			name,
			metric.WithDescription(desc),
			metric.WithUnit("1"),
		)
		if err != nil {
			slog.Error("failed to create labeled counter", "name", name, "error", err)
		}
		return counter, err
	}
	createGauge := func(name, desc string) (metric.Int64ObservableGauge, error) {
		gauge, err := meter.Int64ObservableGauge(name, metric.WithDescription(desc))
		if err != nil {
			slog.Error("failed to create gauge", "name", name, "error", err)
		}
		return gauge, err
	}
	createHistogram := func(name, desc string, buckets []float64) (metric.Float64Histogram, error) {
		hist, err := meter.Float64Histogram(name,
			metric.WithDescription(desc),
			metric.WithExplicitBucketBoundaries(buckets...),
		)
		if err != nil {
			slog.Error("failed to create histogram", "name", name, "error", err)
		}
		return hist, err
	}

	// Create unified counters with status label
	requestsCounter, _ = createLabeledCounter("agent_events_requests", "Number of requests by status")
	bytesReceived, _ = createLabeledCounter("agent_events_received_bytes", "Number of bytes received in request bodies by status")

	// Pre-initialize counters with all status labels set to zero
	ctx := context.Background()
	statusLabels := []string{"success", "duplicate", "invalid_json", "method_not_allowed", "body_too_large", "failed_to_read", "cant_marshal_output"}
	for _, status := range statusLabels {
		requestsCounter.Add(ctx, 0, metric.WithAttributes(attribute.String("status", status)))
		bytesReceived.Add(ctx, 0, metric.WithAttributes(attribute.String("status", status)))
	}

	dedupCacheSize, _ = createGauge("agent_events_dedup_cache_entries", "Current number of entries in the deduplication cache")
	activeConnectionsGauge, _ = createGauge("agent_events_active_connections", "Number of currently active connections")
	uptimeGauge, _ = createGauge("agent_events_uptime_seconds", "How long the server has been running in seconds")
	requestDuration, _ = createHistogram("agent_events_request_duration_seconds",
		"Histogram of request processing times in seconds",
		[]float64{0.00001, 0.000025, 0.00005, 0.0001, 0.00025, 0.0005, 0.001, 0.0025, 0.005, 0.01, 0.025, 0.05, 0.1},
	)
	// Basic check if any metric failed (optional, depends on how critical individual metrics are)
	// if requestsTotal == nil || ... { return exporter, fmt.Errorf("one or more metrics failed to initialize") }

	// Register callbacks for observable metrics
	_, err = meter.RegisterCallback(
		func(_ context.Context, observer metric.Observer) error {
			mapMutex.Lock()
			observer.ObserveInt64(dedupCacheSize, int64(len(seenIDs)))
			mapMutex.Unlock()
			observer.ObserveInt64(activeConnectionsGauge, int64(atomic.LoadInt32(&activeConnections)))
			observer.ObserveInt64(uptimeGauge, int64(time.Since(startTime).Seconds()))
			return nil
		},
		dedupCacheSize, activeConnectionsGauge, uptimeGauge,
	)
	if err != nil {
		// Log but don't necessarily exit, maybe gauges won't update
		slog.Error("failed to register callback for observable metrics", "error", err)
	}

	// Create expvar metrics for backward compatibility
	eventMetrics.Set("active_connections", expvar.Func(func() interface{} { return atomic.LoadInt32(&activeConnections) }))
	eventMetrics.Set("uptime_seconds", expvar.Func(func() interface{} { return time.Since(startTime).Seconds() }))
	eventMetrics.Set("start_time_seconds", expvar.Func(func() interface{} { return startTime.Unix() }))
	eventMetrics.Set("runtime_stats", expvar.Func(func() interface{} {
		memStats := &runtime.MemStats{}
		runtime.ReadMemStats(memStats)
		return *memStats
	}))

	return exporter, nil // Return the exporter (even though we use promhttp.Handler) and nil error
}

// checkAndRecordHash accepts the SHA256 hash ([32]byte) for checking.
// It now REFRESHES the timestamp whenever a hash is found,
// effectively creating a sliding deduplication window.
func checkAndRecordHash(hash [32]byte) bool {
	now := time.Now()
	mapMutex.Lock()
	defer mapMutex.Unlock()
	var zeroHash [32]byte
	if hash == zeroHash {
		slog.Warn("potentially zero hash received", "hash", fmt.Sprintf("%x", hash))
	}
	entry, found := seenIDs[hash]
	if found {
		isRecentDuplicate := now.Sub(entry.timestamp) < dedupWindow
		seenIDs[hash] = seenEntry{timestamp: now}
		return !isRecentDuplicate
	}
	seenIDs[hash] = seenEntry{timestamp: now}
	return true
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
		currentMapSize := len(seenIDs)
		deletedInCycle := 0 // Track deletes per cycle for more granular debug
		for h, entry := range seenIDs {
			if now.Sub(entry.timestamp) >= dedupWindow {
				delete(seenIDs, h)
				cleanedCount++
				deletedInCycle++
			}
		}
		mapMutex.Unlock()

		// Log hourly summary if any were cleaned in the last hour
		if cleanedCount > 0 && time.Since(lastCleanupLogTime) >= time.Hour {
			slog.Debug("cleaned up expired entries",
				"count_past_hour", cleanedCount,
				"remaining_entries", currentMapSize-deletedInCycle) // Use size before delete for consistency
			cleanedCount = 0 // Reset hourly count
			lastCleanupLogTime = time.Now()
		} else if deletedInCycle > 0 {
			// Optional: Log every cycle if debugging cleanup
			// slog.Debug("cleanup cycle completed", "deleted", deletedInCycle, "remaining", currentMapSize-deletedInCycle)
		}
	}
}

// connectionTracker is middleware that wraps an http.Handler to track active connections
func connectionTracker(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&activeConnections, 1)
		defer atomic.AddInt32(&activeConnections, -1)
		next.ServeHTTP(w, r)
	})
}

// --- HTTP Handler ---
func handler(w http.ResponseWriter, r *http.Request) {
	requestStartTime := time.Now()
	ctx := r.Context()

	defer func() {
		requestDuration.Record(ctx, time.Since(requestStartTime).Seconds())
	}()

	// Method Check
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		slog.Info("request discarded", "reason", "method_not_allowed", "method", r.Method, "remote_addr", r.RemoteAddr)
		requestsCounter.Add(ctx, 1, metric.WithAttributes(attribute.String("status", "method_not_allowed")))
		return
	}

	// Read Body & Size Check
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySize)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			http.Error(w, fmt.Sprintf("Request body exceeds limit (%d bytes)", maxRequestBodySize), http.StatusRequestEntityTooLarge)
			slog.Info("request discarded", "reason", "body_too_large", "limit", maxRequestBodySize, "remote_addr", r.RemoteAddr)
			requestsCounter.Add(ctx, 1, metric.WithAttributes(attribute.String("status", "body_too_large")))
		} else {
			http.Error(w, "Error reading request", http.StatusInternalServerError)
			slog.Error("request discarded", "reason", "error_reading_body", "error", err)
			requestsCounter.Add(ctx, 1, metric.WithAttributes(attribute.String("status", "failed_to_read")))
		}
		return
	}

	// JSON Validation - Attempt to unmarshal directly to map (requires object)
	var fullData map[string]interface{}
	if err := json.Unmarshal(body, &fullData); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		bodyDetail := ""
		if slog.Default().Enabled(context.Background(), slog.LevelDebug) {
			bodyDetail = fmt.Sprintf(", Body: %s", string(body))
		} else {
			bodyDetail = fmt.Sprintf(", Body snippet: %s", limitString(string(body), 100))
		}
		slog.Warn("request discarded", "reason", "invalid_json", "error", err.Error(), "body_detail", bodyDetail)
		bytesReceived.Add(ctx, int64(len(body)), metric.WithAttributes(attribute.String("status", "invalid_json")))
		requestsCounter.Add(ctx, 1, metric.WithAttributes(attribute.String("status", "invalid_json")))
		return
	}

	bytesReceived.Add(ctx, int64(len(body)), metric.WithAttributes(attribute.String("status", "success")))

	// Add Cloudflare Headers to all requests, excluding IP addresses for GDPR compliance
	cfHeaders := make(map[string]string)
	cfHeaderPrefixes := []string{"CF-IPCountry", "CF-Ray", "CF-IPCity", "CF-IPContinent", "CF-IPRegion", "CF-IPTimeZone", "CF-IPCOLO"}
	// Explicitly excluding IP-related headers: CF-Connecting-IP, CF-IPLatitude, CF-IPLongitude, CF-Visitor
	for _, name := range cfHeaderPrefixes {
		if value := r.Header.Get(name); value != "" {
			key := strings.TrimPrefix(name, "CF-")
			cfHeaders[key] = value
		}
	}
	for name, values := range r.Header {
		if strings.HasPrefix(name, "CF-") && len(values) > 0 {
			// Skip IP-related headers for GDPR compliance
			if name == "CF-Connecting-IP" || name == "CF-IPLatitude" || name == "CF-IPLongitude" || name == "CF-Visitor" {
				continue
			}
			key := strings.TrimPrefix(name, "CF-")
			if _, exists := cfHeaders[key]; !exists {
				cfHeaders[key] = values[0]
			}
		}
	}
	if len(cfHeaders) > 0 {
		fullData["cf"] = cfHeaders
		slog.Debug("added cloudflare headers", "count", len(cfHeaders))
	}

	// Deduplication Logic
	shouldProcess := true
	var finalKeyString string
	var dedupHash [32]byte
	if len(keyPaths) > 0 {
		var keyBuilder strings.Builder
		for i, path := range keyPaths {
			result := gjson.GetBytes(body, path)
			keyBuilder.WriteString(result.String()) // gjson returns "" for non-existent paths
			if i < len(keyPaths)-1 {
				keyBuilder.WriteString(dedupSeparator)
			}
		}
		finalKeyString = keyBuilder.String()
		dedupHash = sha256.Sum256([]byte(finalKeyString))
		slog.Debug("generated dedup key", "key_string", finalKeyString, "hash", fmt.Sprintf("%x", dedupHash))

		// Add _dedup key to all requests (regardless of duplicate status)
		fullData["_dedup"] = map[string]interface{}{
			"key":  finalKeyString,
			"hash": fmt.Sprintf("%x", dedupHash),
		}

		// Check if this is a duplicate
		if !checkAndRecordHash(dedupHash) {
			shouldProcess = false
			bytesReceived.Add(ctx, int64(len(body)), metric.WithAttributes(attribute.String("status", "duplicate")))
			requestsCounter.Add(ctx, 1, metric.WithAttributes(attribute.String("status", "duplicate")))
			slog.Debug("discarded duplicate request", "hash", fmt.Sprintf("%x", dedupHash), "note", "timestamp refreshed")
		}
	} else {
		slog.Debug("skipping deduplication", "reason", "no dedup keys provided")
	}

	// Marshal the fully prepared object for either stdout or dedup log
	outputBytes, err := json.Marshal(fullData)
	if err != nil {
		http.Error(w, "Internal Server Error during output marshal", http.StatusInternalServerError)
		slog.Error("request discarded", "reason", "json_marshal_failed", "error", err)
		requestsCounter.Add(ctx, 1, metric.WithAttributes(attribute.String("status", "cant_marshal_output")))
		return
	}

	// Decide where to output based on deduplication status
	if shouldProcess {
		// Write to stdout for normal processing
		fmt.Println(string(outputBytes))
		requestsCounter.Add(ctx, 1, metric.WithAttributes(attribute.String("status", "success")))
	} else if dedupLogger != nil {
		// Write to dedup log file if it's a duplicate and logging is enabled
		if _, err := fmt.Fprintln(dedupLogger, string(outputBytes)); err != nil {
			slog.Error("failed to write to deduplication log file", "error", err)
		}
	}

	// Send response
	if _, err := w.Write([]byte("OK")); err != nil {
		if shouldProcess {
			slog.Error("error writing response", "context", "after_successful_processing", "error", err)
		} else {
			slog.Error("error writing response", "context", "after_duplicate_discard", "error", err)
		}
	}

	// For duplicates, return early
	if !shouldProcess {
		return
	}
}

// healthHandler provides a simple health check endpoint
func healthHandler(w http.ResponseWriter, r *http.Request) {
	status := map[string]interface{}{
		"status":      "ok",
		"timestamp":   time.Now().Format(time.RFC3339),
		"uptime":      time.Since(startTime).String(),
		"goroutines":  runtime.NumGoroutine(),
		"connections": atomic.LoadInt32(&activeConnections),
		"version":     "agent-events v1.0",
	}
	memStats := &runtime.MemStats{}
	runtime.ReadMemStats(memStats)
	status["memory"] = map[string]interface{}{"alloc": memStats.Alloc, "total_alloc": memStats.TotalAlloc, "sys": memStats.Sys, "heap_alloc": memStats.HeapAlloc, "gc_cycles": memStats.NumGC}
	mapMutex.Lock()
	mapSize := len(seenIDs)
	mapMutex.Unlock()
	dedupLogEnabled := dedupLogger != nil
	dedupLogPath := ""
	if dedupLogEnabled {
		dedupLogPath = dedupLogFile
	}
	status["deduplication"] = map[string]interface{}{
		"enabled":     len(keyPaths) > 0,
		"keys":        keyPaths,
		"window":      dedupWindow.String(),
		"map_size":    mapSize,
		"log_enabled": dedupLogEnabled,
		"log_file":    dedupLogPath,
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(status); err != nil {
		slog.Error("error encoding health check response", "error", err)
	}
}

// main is the entry point of the application
func main() {
	// Setup initial logger before flag parsing
	initialLogLevel := slog.LevelInfo
	initialLogHandler := slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: initialLogLevel, AddSource: true})
	slog.SetDefault(slog.New(initialLogHandler))

	startTime = time.Now()

	// Define flags
	port := flag.Int("port", 8080, "Port to listen on")
	dedupSeconds := flag.Int("dedup-window", 1800, "Deduplication window in seconds")
	metricsPath := flag.String("metrics-path", "/metrics", "Path for OpenTelemetry Prometheus metrics endpoint")
	expvarPath := flag.String("expvar-path", "/debug/vars", "Path for expvar metrics endpoint (empty to disable)")
	healthPath := flag.String("health-path", "/healthz", "Path for health check endpoint")
	logFormat := flag.String("log-format", "json", "Log format: 'json' or 'text'")
	logLevelFlag := flag.String("log-level", "info", "Log level: 'debug', 'info', 'warn', 'error'")
	// Use the global dedupLogFile variable
	flag.StringVar(&dedupLogFile, "dedup-logfile", "", "File to log deduplicated requests (empty to disable)")
	flag.Var(&keyPaths, "dedup-key", "JSON path (dot-notation) for deduplication key (can be used multiple times)")
	flag.StringVar(&dedupSeparator, "dedup-separator", "-", "Separator used between multi-key values")
	flag.Parse()

	// Configure final logger based on flags
	var level slog.Level
	switch strings.ToLower(*logLevelFlag) {
	case "debug":
		level = slog.LevelDebug
	case "info":
		level = slog.LevelInfo
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		slog.Warn("invalid log level specified, defaulting to info", "value", *logLevelFlag)
		level = slog.LevelInfo
	}
	var logHandler slog.Handler
	if *logFormat == "text" {
		logHandler = slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level, AddSource: true})
	} else {
		logHandler = slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: level, AddSource: true})
	}
	slog.SetDefault(slog.New(logHandler))

	// Initialize core components
	seenIDs = make(map[[32]byte]seenEntry)
	dedupWindow = time.Duration(*dedupSeconds) * time.Second

	// Open deduplication log file if specified
	if dedupLogFile != "" {
		var err error
		dedupLogger, err = os.OpenFile(dedupLogFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			slog.Error("failed to open deduplication log file", "file", dedupLogFile, "error", err)
			os.Exit(1)
		}
		slog.Info("deduplication log file opened", "path", dedupLogFile)
	}

	if _, err := initMetrics(); err != nil { // Handle potential error from initMetrics
		slog.Error("failed to initialize metrics", "error", err)
		os.Exit(1)
	}

	// Start background tasks
	if dedupWindow > 0 && len(keyPaths) > 0 {
		cleanupInterval := dedupWindow / 10
		if cleanupInterval < 1*time.Minute {
			cleanupInterval = 1 * time.Minute
		} else if cleanupInterval > 15*time.Minute {
			cleanupInterval = 15 * time.Minute
		}
		slog.Info("cleanup goroutine started", "interval", cleanupInterval)
		go cleanupExpiredEntries(cleanupInterval)
	} else if dedupWindow <= 0 && len(keyPaths) > 0 {
		slog.Warn("deduplication keys provided, but window is zero or negative", "keys", keyPaths, "window", dedupWindow)
	}

	// Configure HTTP server
	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", *port),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/", handler)
	mux.HandleFunc(*healthPath, healthHandler)
	if *expvarPath != "" {
		mux.Handle(*expvarPath, expvar.Handler())
	} // Register expvar if path not empty
	mux.Handle(*metricsPath, promhttp.Handler()) // Use promhttp handler for OTEL metrics
	// Add pprof handlers to custom mux
	mux.HandleFunc("/debug/pprof/", http.DefaultServeMux.ServeHTTP)
	mux.HandleFunc("/debug/pprof/cmdline", http.DefaultServeMux.ServeHTTP)
	mux.HandleFunc("/debug/pprof/profile", http.DefaultServeMux.ServeHTTP)
	mux.HandleFunc("/debug/pprof/symbol", http.DefaultServeMux.ServeHTTP)
	mux.HandleFunc("/debug/pprof/trace", http.DefaultServeMux.ServeHTTP)
	server.Handler = connectionTracker(mux) // Apply middleware

	// Start server and handle shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	serverErrors := make(chan error, 1)

	slog.Info("server starting", "port", *port, "metrics_path", *metricsPath, "expvar_path", *expvarPath, "health_path", *healthPath, "log_level", level.String()) // Simplified startup log

	go func() {
		slog.Info("server listening", "addr", server.Addr)
		serverErrors <- server.ListenAndServe()
	}()

	select {
	case err := <-serverErrors:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server error", "error", err)
		}
	case sig := <-stop:
		slog.Info("shutdown initiated", "signal", sig.String())
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// Shutdown meter provider first
		if meterProvider != nil {
			slog.Info("shutting down OpenTelemetry meter provider")
			if err := meterProvider.Shutdown(shutdownCtx); err != nil {
				slog.Error("meter provider shutdown failed", "error", err)
			}
		}

		// Shutdown HTTP server
		slog.Info("shutting down HTTP server")
		if err := server.Shutdown(shutdownCtx); err != nil {
			slog.Error("server shutdown failed", "error", err)
		} else {
			slog.Info("server shutdown completed gracefully")
		}

		// Close deduplication log file if open
		if dedupLogger != nil {
			slog.Info("closing deduplication log file")
			if err := dedupLogger.Close(); err != nil {
				slog.Error("failed to close deduplication log file", "error", err)
			}
		}
	}

	slog.Info("server exiting")
}

// --- Helper Functions ---
func limitString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
