package logger

import (
	"context"
	"log/slog"
	"sync"
	"testing"
)

type capturedRecord struct {
	level slog.Level
	msg   string
}

type captureHandler struct {
	mu      sync.Mutex
	minLvl  slog.Level
	records []capturedRecord
}

func newCaptureHandler(minLvl slog.Level) *captureHandler {
	return &captureHandler{minLvl: minLvl}
}

func (h *captureHandler) Enabled(_ context.Context, lvl slog.Level) bool {
	return lvl >= h.minLvl
}

func (h *captureHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, capturedRecord{
		level: r.Level,
		msg:   r.Message,
	})
	return nil
}

func (h *captureHandler) WithAttrs(_ []slog.Attr) slog.Handler {
	return h
}

func (h *captureHandler) WithGroup(_ string) slog.Handler {
	return h
}

func (h *captureHandler) count() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.records)
}

func (h *captureHandler) last() capturedRecord {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.records) == 0 {
		return capturedRecord{}
	}
	return h.records[len(h.records)-1]
}

func setTestLevel(t *testing.T, level slog.Level) {
	t.Helper()
	prev := Level.lvl.Level()
	Level.Set(level)
	t.Cleanup(func() {
		Level.Set(prev)
	})
}

func newTestLogger(level slog.Level) (*Logger, *captureHandler) {
	handler := newCaptureHandler(level)
	return &Logger{sl: slog.New(handler), rl: newRateLimiter()}, handler
}
