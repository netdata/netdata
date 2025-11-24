package zabbixpreproc

// Logger is a minimal logging interface for preprocessing operations.
// This allows users to plug in any logging library (slog, logrus, zap, etc.)
// without forcing dependencies on the library.
//
// By default, the library uses NoopLogger (zero overhead).
// Users can provide their own logger implementation via SetLogger().
//
// Example with slog:
//
//	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
//	preprocessor.SetLogger(NewSlogAdapter(logger))
type Logger interface {
	// Debug logs debug-level messages with optional structured key-value pairs.
	// Used for: skipped SNMP lines, empty values, parsing decisions
	Debug(msg string, keysAndValues ...interface{})

	// Info logs informational messages with optional structured key-value pairs.
	// Used for: preprocessing statistics, cache hits
	Info(msg string, keysAndValues ...interface{})

	// Warn logs warning messages with optional structured key-value pairs.
	// Used for: deprecated features, performance concerns
	Warn(msg string, keysAndValues ...interface{})

	// Error logs error messages with optional structured key-value pairs.
	// Note: This is for logging only. Errors are still returned to caller.
	Error(msg string, keysAndValues ...interface{})
}

// NoopLogger is a no-operation logger that discards all log messages.
// This is the default logger (zero performance overhead).
type NoopLogger struct{}

// Debug implements Logger.Debug (no-op)
func (NoopLogger) Debug(msg string, keysAndValues ...interface{}) {}

// Info implements Logger.Info (no-op)
func (NoopLogger) Info(msg string, keysAndValues ...interface{}) {}

// Warn implements Logger.Warn (no-op)
func (NoopLogger) Warn(msg string, keysAndValues ...interface{}) {}

// Error implements Logger.Error (no-op)
func (NoopLogger) Error(msg string, keysAndValues ...interface{}) {}

// Ensure NoopLogger implements Logger
var _ Logger = (*NoopLogger)(nil)
