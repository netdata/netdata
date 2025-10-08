package framework

import (
	"time"
)

// ProtocolClient provides automatic instrumentation for protocol operations
type ProtocolClient struct {
	name                string
	metrics             *ProtocolMetrics
	state               *CollectorState
	backoff             *ExponentialBackoff
	connectionIteration int64 // Iteration when this protocol connected
}

// NewProtocolClient creates a new instrumented protocol client
func NewProtocolClient(name string, state *CollectorState) *ProtocolClient {
	return &ProtocolClient{
		name:    name,
		metrics: state.RegisterProtocol(name),
		state:   state,
		backoff: NewExponentialBackoff(),
	}
}

// GetBackoff returns the exponential backoff for this protocol
func (p *ProtocolClient) GetBackoff() *ExponentialBackoff {
	p.backoff.protocol = p // Set reference for logging
	return p.backoff
}

// MarkConnected should be called when the protocol successfully connects
func (p *ProtocolClient) MarkConnected() {
	p.connectionIteration = p.state.GetIteration()
	p.backoff.Reset()
}

// Debugf logs a debug message prefixed with the protocol name
func (p *ProtocolClient) Debugf(format string, args ...interface{}) {
	p.state.Debugf("[%s] "+format, append([]interface{}{p.name}, args...)...)
}

// Warningf logs a warning message prefixed with the protocol name
func (p *ProtocolClient) Warningf(format string, args ...interface{}) {
	p.state.Warningf("[%s] "+format, append([]interface{}{p.name}, args...)...)
}

// Errorf logs an error message prefixed with the protocol name
func (p *ProtocolClient) Errorf(format string, args ...interface{}) {
	p.state.Errorf("[%s] "+format, append([]interface{}{p.name}, args...)...)
}

// Infof logs an info message prefixed with the protocol name
func (p *ProtocolClient) Infof(format string, args ...interface{}) {
	p.state.Infof("[%s] "+format, append([]interface{}{p.name}, args...)...)
}

// IsReconnect returns true if this is the same iteration as when connected
func (p *ProtocolClient) IsReconnect() bool {
	return p.connectionIteration == p.state.GetIteration()
}

// Track wraps a protocol operation with automatic metrics collection
func (p *ProtocolClient) Track(operationName string, fn func() error) error {
	start := time.Now()

	// Execute the operation
	err := fn()

	// Track metrics
	elapsed := time.Since(start).Microseconds()
	p.metrics.RequestCount++
	p.metrics.TotalLatency += elapsed

	if elapsed > p.metrics.MaxLatency {
		p.metrics.MaxLatency = elapsed
	}

	if err != nil {
		p.metrics.ErrorCount++
	}

	return err
}

// TrackWithSize wraps operations that have request/response sizes
func (p *ProtocolClient) TrackWithSize(operationName string, requestSize int64, fn func() (int64, error)) error {
	start := time.Now()

	// Track request size
	p.metrics.BytesSent += requestSize

	// Execute the operation
	responseSize, err := fn()

	// Track response size
	if responseSize > 0 {
		p.metrics.BytesReceived += responseSize
	}

	// Track timing
	elapsed := time.Since(start).Microseconds()
	p.metrics.RequestCount++
	p.metrics.TotalLatency += elapsed

	if elapsed > p.metrics.MaxLatency {
		p.metrics.MaxLatency = elapsed
	}

	if err != nil {
		p.metrics.ErrorCount++
	}

	return err
}

// GetProtocolMetrics returns current protocol metrics for charting
func (p *ProtocolClient) GetProtocolMetrics() map[string]int64 {
	metrics := make(map[string]int64)

	// Operation counts
	metrics["requests"] = p.metrics.RequestCount
	metrics["errors"] = p.metrics.ErrorCount

	// Latency metrics
	if p.metrics.RequestCount > 0 {
		metrics["avg_latency"] = p.metrics.TotalLatency / p.metrics.RequestCount
	}
	metrics["max_latency"] = p.metrics.MaxLatency

	// Size metrics
	metrics["bytes_sent"] = p.metrics.BytesSent
	metrics["bytes_received"] = p.metrics.BytesReceived

	return metrics
}

// ClassifyError wraps errors with retry classification
func ClassifyError(err error, errType ErrorType) error {
	if err == nil {
		return nil
	}
	return CollectorError{Err: err, Type: errType}
}

// IsTemporary checks if an error is temporary and should be retried
func IsTemporary(err error) bool {
	if collErr, ok := err.(CollectorError); ok {
		return collErr.Type == ErrorTemporary
	}
	return false
}

// IsFatal checks if an error is fatal and module should be disabled
func IsFatal(err error) bool {
	if collErr, ok := err.(CollectorError); ok {
		return collErr.Type == ErrorFatal || collErr.Type == ErrorAuth
	}
	return false
}

// ExponentialBackoff implements retry logic with backoff
type ExponentialBackoff struct {
	InitialInterval time.Duration
	MaxInterval     time.Duration
	Multiplier      float64
	currentInterval time.Duration
	attempt         int
	protocol        *ProtocolClient // For logging
}

// NewExponentialBackoff creates a new backoff handler
func NewExponentialBackoff() *ExponentialBackoff {
	return &ExponentialBackoff{
		InitialInterval: time.Second,
		MaxInterval:     5 * time.Minute,
		Multiplier:      2.0,
		currentInterval: time.Second,
	}
}

// NextInterval returns the next backoff interval
func (b *ExponentialBackoff) NextInterval() time.Duration {
	defer func() {
		b.currentInterval = time.Duration(float64(b.currentInterval) * b.Multiplier)
		if b.currentInterval > b.MaxInterval {
			b.currentInterval = b.MaxInterval
		}
		b.attempt++
	}()

	// Add jitter (±10%)
	jitter := time.Duration(float64(b.currentInterval) * 0.1)
	return b.currentInterval + jitter
}

// Reset resets the backoff to initial state
func (b *ExponentialBackoff) Reset() {
	b.currentInterval = b.InitialInterval
	b.attempt = 0
}

// ShouldRetry determines if we should retry based on attempt count
func (b *ExponentialBackoff) ShouldRetry() bool {
	// Retry up to 10 times (reaches max interval after ~8 attempts)
	return b.attempt < 10
}

// Retry executes a function with exponential backoff
func (b *ExponentialBackoff) Retry(fn func() error) error {
	var lastErr error

	if b.protocol != nil {
		b.protocol.Debugf("Starting retry sequence with initial interval %v, max interval %v",
			b.InitialInterval, b.MaxInterval)
	}

	for {
		if b.protocol != nil && b.attempt > 0 {
			b.protocol.Debugf("Retry attempt #%d (after %d failed attempts)", b.attempt+1, b.attempt)
		}

		err := fn()
		if err == nil {
			if b.protocol != nil && b.attempt > 0 {
				b.protocol.Infof("Operation succeeded after %d retry attempts", b.attempt)
			}
			b.Reset()
			return nil
		}

		lastErr = err

		// If this is a fatal error, don't retry
		if IsFatal(err) {
			if b.protocol != nil {
				b.protocol.Errorf("Fatal error encountered, stopping retries: %v", err)
			}
			return err
		}

		// Check if we should continue retrying
		if !b.ShouldRetry() {
			if b.protocol != nil {
				b.protocol.Errorf("Maximum retry attempts (%d) reached, giving up: %v", b.attempt, lastErr)
			}
			return lastErr
		}

		// Wait before next attempt
		interval := b.NextInterval()
		if b.protocol != nil {
			b.protocol.Debugf("Waiting %v before retry attempt #%d (error was: %v)",
				interval, b.attempt+1, err)
		}
		time.Sleep(interval)
	}
}
