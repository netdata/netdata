// SPDX-License-Identifier: GPL-3.0-or-later

package logger

import (
	"fmt"
	"log/slog"
	"sync"
	"time"
)

const (
	rateLimiterSweepEvery = 4096
	rateLimiterTTL        = time.Hour
)

type limitMode uint8

const (
	modeOnce limitMode = iota + 1
	modeLimit
)

type limitKey struct {
	mode limitMode
	key  string
}

type limitEntry struct {
	limit       int
	window      time.Duration
	count       int
	windowStart time.Time
	lastSeen    time.Time
}

type rateLimiter struct {
	mu         sync.Mutex
	entries    map[limitKey]*limitEntry
	sweepEvery uint64
	ttl        time.Duration
	calls      uint64
	now        func() time.Time
}

func newRateLimiter() *rateLimiter {
	return &rateLimiter{
		entries:    make(map[limitKey]*limitEntry),
		sweepEvery: rateLimiterSweepEvery,
		ttl:        rateLimiterTTL,
		now:        time.Now,
	}
}

type LimitedLogger struct {
	l      *Logger
	mode   limitMode
	key    string
	limit  int
	window time.Duration
}

func (l *Logger) Once(key string) LimitedLogger {
	return LimitedLogger{
		l:      l,
		mode:   modeOnce,
		key:    key,
		limit:  1,
		window: 0,
	}
}

func (l *Logger) Limit(key string, n int, d time.Duration) LimitedLogger {
	if n <= 0 {
		n = 1
	}
	if d < 0 {
		d = 0
	}
	return LimitedLogger{
		l:      l,
		mode:   modeLimit,
		key:    key,
		limit:  n,
		window: d,
	}
}

func (l *Logger) ResetAllOnce() {
	if l == nil || l.rl == nil {
		return
	}
	l.rl.resetAllMode(modeOnce)
}

func (l LimitedLogger) Error(a ...any) {
	l.logArgs(slog.LevelError, a...)
}

func (l LimitedLogger) Warning(a ...any) {
	l.logArgs(slog.LevelWarn, a...)
}

func (l LimitedLogger) Notice(a ...any) {
	l.logArgs(levelNotice, a...)
}

func (l LimitedLogger) Info(a ...any) {
	l.logArgs(slog.LevelInfo, a...)
}

func (l LimitedLogger) Debug(a ...any) {
	l.logArgs(slog.LevelDebug, a...)
}

func (l LimitedLogger) Errorf(format string, a ...any) {
	l.logf(slog.LevelError, format, a...)
}

func (l LimitedLogger) Warningf(format string, a ...any) {
	l.logf(slog.LevelWarn, format, a...)
}

func (l LimitedLogger) Noticef(format string, a ...any) {
	l.logf(levelNotice, format, a...)
}

func (l LimitedLogger) Infof(format string, a ...any) {
	l.logf(slog.LevelInfo, format, a...)
}

func (l LimitedLogger) Debugf(format string, a ...any) {
	l.logf(slog.LevelDebug, format, a...)
}

func (l LimitedLogger) logArgs(level slog.Level, a ...any) {
	if !l.l.canLog(level) || !l.allow() {
		return
	}
	l.l.log(level, fmt.Sprint(a...))
}

func (l LimitedLogger) logf(level slog.Level, format string, a ...any) {
	if !l.l.canLog(level) || !l.allow() {
		return
	}
	l.l.log(level, fmt.Sprintf(format, a...))
}

func (l LimitedLogger) allow() bool {
	if l.l == nil || l.l.rl == nil {
		// Preserve nil logger behavior: no panic and no additional suppression.
		return true
	}
	return l.l.rl.allow(l.mode, l.key, l.limit, l.window)
}

func (r *rateLimiter) allow(mode limitMode, key string, limit int, window time.Duration) bool {
	now := r.now()
	allow := false

	r.mu.Lock()
	defer r.mu.Unlock()

	r.calls++
	if r.sweepEvery > 0 && r.calls%r.sweepEvery == 0 {
		r.sweep(now)
	}

	k := limitKey{mode: mode, key: key}
	e, ok := r.entries[k]
	if !ok {
		e = &limitEntry{
			limit:       limit,
			window:      window,
			windowStart: now,
		}
		r.entries[k] = e
	}

	e.lastSeen = now

	// d==0 means infinite window, so rollover must be disabled.
	if e.window > 0 && now.Sub(e.windowStart) >= e.window {
		e.windowStart = now
		e.count = 0
	}

	if e.count < e.limit {
		e.count++
		allow = true
	}

	return allow
}

func (r *rateLimiter) resetAllMode(mode limitMode) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for k := range r.entries {
		if k.mode == mode {
			delete(r.entries, k)
		}
	}
}

func (r *rateLimiter) sweep(now time.Time) {
	cutoff := now.Add(-r.ttl)
	for k, e := range r.entries {
		if e.lastSeen.Before(cutoff) {
			delete(r.entries, k)
		}
	}
}
