// SPDX-License-Identifier: GPL-3.0-or-later

//go:build cgo
// +build cgo

package websphere_jmx

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"
)

// circuitBreaker implements the circuit breaker pattern
type circuitBreaker struct {
	mu               sync.RWMutex
	state            circuitState
	failures         int
	lastFailure      time.Time
	maxFailures      int
	resetTimeout     time.Duration
	halfOpenAttempts int
	halfOpenMaxCalls int
}

type circuitState int

const (
	stateClosed circuitState = iota
	stateOpen
	stateHalfOpen
)

func newCircuitBreaker(threshold int, resetTimeout time.Duration) *circuitBreaker {
	return &circuitBreaker{
		state:            stateClosed,
		maxFailures:      threshold,
		resetTimeout:     resetTimeout,
		halfOpenMaxCalls: 1,
	}
}

func (cb *circuitBreaker) recordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failures = 0
	cb.halfOpenAttempts = 0
	if cb.state == stateHalfOpen {
		cb.state = stateClosed
	}
}

func (cb *circuitBreaker) recordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failures++
	cb.lastFailure = time.Now()

	if cb.state == stateHalfOpen {
		cb.state = stateOpen
		cb.halfOpenAttempts = 0
	} else if cb.failures >= cb.maxFailures {
		cb.state = stateOpen
	}
}

func (cb *circuitBreaker) canAttempt() bool {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	switch cb.state {
	case stateClosed:
		return true
	case stateOpen:
		return time.Since(cb.lastFailure) > cb.resetTimeout
	case stateHalfOpen:
		return cb.halfOpenAttempts < cb.halfOpenMaxCalls
	}
	return false
}

func (cb *circuitBreaker) beforeCall() error {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if cb.state == stateOpen && time.Since(cb.lastFailure) > cb.resetTimeout {
		cb.state = stateHalfOpen
		cb.halfOpenAttempts = 0
	}

	switch cb.state {
	case stateClosed:
		return nil
	case stateOpen:
		return fmt.Errorf("circuit breaker is open")
	case stateHalfOpen:
		if cb.halfOpenAttempts >= cb.halfOpenMaxCalls {
			return fmt.Errorf("circuit breaker is half-open, max attempts reached")
		}
		cb.halfOpenAttempts++
		return nil
	}
	return nil
}

func (cb *circuitBreaker) getState() string {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	switch cb.state {
	case stateClosed:
		return "closed"
	case stateOpen:
		return "open"
	case stateHalfOpen:
		return "half_open"
	}
	return "unknown"
}

// helperMonitor monitors and manages the Java helper process
type helperMonitor struct {
	collector     *WebSphereJMX
	checkInterval time.Duration
	restartDelay  time.Duration
	maxRestarts   int

	mu           sync.Mutex
	restartCount int
	lastRestart  time.Time
	monitoring   bool
	stopCh       chan struct{}
}

func newHelperMonitor(collector *WebSphereJMX) *helperMonitor {
	return &helperMonitor{
		collector:     collector,
		checkInterval: 30 * time.Second,
		restartDelay:  5 * time.Second,
		maxRestarts:   collector.HelperRestartMax,
		stopCh:        make(chan struct{}),
	}
}

func (hm *helperMonitor) start(ctx context.Context) {
	hm.mu.Lock()
	if hm.monitoring {
		hm.mu.Unlock()
		return
	}
	hm.monitoring = true
	hm.mu.Unlock()

	go hm.monitorLoop(ctx)
}

func (hm *helperMonitor) stop() {
	hm.mu.Lock()
	defer hm.mu.Unlock()

	if hm.monitoring {
		close(hm.stopCh)
		hm.monitoring = false
	}
}

func (hm *helperMonitor) monitorLoop(ctx context.Context) {
	ticker := time.NewTicker(hm.checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-hm.stopCh:
			return
		case <-ticker.C:
			if err := hm.checkAndRestart(ctx); err != nil {
				hm.collector.Warningf("helper monitor error: %v", err)
			}
		}
	}
}

func (hm *helperMonitor) checkAndRestart(ctx context.Context) error {
	// Check if helper is alive
	if !hm.isHelperAlive() {
		hm.collector.Warningf("Java helper process is not responding")
		return hm.restartHelper(ctx)
	}

	// Memory monitoring removed - not applicable for remote JMX connections
	// and local process memory is already monitored by Netdata

	return nil
}

func (hm *helperMonitor) isHelperAlive() bool {
	if hm.collector.jmxHelper == nil || !hm.collector.jmxHelper.running {
		return false
	}

	// Try a PING command to check responsiveness
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := hm.collector.jmxHelper.sendCommand(ctx, jmxCommand{Command: "PING"})
	if err != nil {
		return false
	}

	return resp != nil && resp.Status == "OK"
}

func (hm *helperMonitor) restartHelper(ctx context.Context) error {
	hm.mu.Lock()
	defer hm.mu.Unlock()

	// Check restart limits
	if hm.restartCount >= hm.maxRestarts {
		if time.Since(hm.lastRestart) < 1*time.Hour {
			return fmt.Errorf("max restarts (%d) reached within last hour", hm.maxRestarts)
		}
		// Reset counter after an hour
		hm.restartCount = 0
	}

	// Exponential backoff
	backoffDuration := time.Duration(math.Pow(2, float64(hm.restartCount))) * hm.restartDelay
	if backoffDuration > 5*time.Minute {
		backoffDuration = 5 * time.Minute
	}

	hm.collector.Infof("waiting %v before restart attempt %d", backoffDuration, hm.restartCount+1)
	time.Sleep(backoffDuration)

	// Stop current helper
	if hm.collector.jmxHelper != nil {
		hm.collector.jmxHelper.shutdown()
	}

	// Start new helper
	if err := hm.collector.startJMXHelper(ctx); err != nil {
		return fmt.Errorf("failed to restart helper: %w", err)
	}

	hm.restartCount++
	hm.lastRestart = time.Now()
	hm.collector.Infof("Java helper restarted successfully (attempt %d/%d)", hm.restartCount, hm.maxRestarts)

	return nil
}

// collectWithResilience wraps collection with circuit breaker and retry logic
func (w *WebSphereJMX) collectWithResilience(ctx context.Context) (map[string]int64, error) {
	// Check circuit breaker
	if err := w.circuitBreaker.beforeCall(); err != nil {
		w.Debugf("circuit breaker preventing collection: %v", err)
		return w.getCachedMetrics(), nil
	}

	// Try collection with exponential backoff
	mx, err := w.collectWithBackoff(ctx)
	if err != nil {
		w.circuitBreaker.recordFailure()
		// Return cached metrics on failure
		if len(w.lastGoodMetrics) > 0 {
			w.Warningf("collection failed, using cached metrics: %v", err)
			return w.getCachedMetrics(), nil
		}
		return nil, err
	}

	// Success - record it
	w.circuitBreaker.recordSuccess()
	w.lastGoodMetrics = mx
	w.lastSuccessTime = time.Now()

	// Add connection health metric
	mx["connection_health"] = 1
	mx["circuit_breaker_state"] = circuitStateToInt(w.circuitBreaker.getState())

	return mx, nil
}

func (w *WebSphereJMX) collectWithBackoff(ctx context.Context) (map[string]int64, error) {
	var lastErr error

	for attempt := 0; attempt < w.MaxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff: 1s, 2s, 4s, 8s...
			backoffDuration := time.Duration(math.Pow(w.RetryBackoffMultiplier, float64(attempt-1))) * time.Second
			if backoffDuration > 30*time.Second {
				backoffDuration = 30 * time.Second
			}

			w.Debugf("retry attempt %d/%d after %v", attempt+1, w.MaxRetries, backoffDuration)

			select {
			case <-time.After(backoffDuration):
				// Continue with retry
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}

		mx, err := w.collect(ctx)
		if err == nil {
			return mx, nil
		}

		lastErr = err
		w.Debugf("collection attempt %d failed: %v", attempt+1, err)
	}

	return nil, fmt.Errorf("failed after %d attempts: %w", w.MaxRetries, lastErr)
}

func (w *WebSphereJMX) getCachedMetrics() map[string]int64 {
	mx := make(map[string]int64)

	// Copy last good metrics
	for k, v := range w.lastGoodMetrics {
		mx[k] = v
	}

	// Add staleness indicators
	mx["connection_health"] = 0
	mx["seconds_since_last_success"] = int64(time.Since(w.lastSuccessTime).Seconds())

	return mx
}

func circuitStateToInt(state string) int64 {
	switch state {
	case "closed":
		return 0
	case "half_open":
		return 1
	case "open":
		return 2
	default:
		return -1
	}
}
