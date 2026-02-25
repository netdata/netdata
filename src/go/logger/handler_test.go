package logger

import (
	"context"
	"log/slog"
	"runtime"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

type pcCaptureHandler struct {
	mu  sync.Mutex
	pcs []uintptr
}

func (h *pcCaptureHandler) Enabled(context.Context, slog.Level) bool {
	return true
}

func (h *pcCaptureHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.pcs = append(h.pcs, r.PC)
	return nil
}

func (h *pcCaptureHandler) WithAttrs(_ []slog.Attr) slog.Handler {
	return h
}

func (h *pcCaptureHandler) WithGroup(_ string) slog.Handler {
	return h
}

func (h *pcCaptureHandler) lastFunction() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.pcs) == 0 {
		return ""
	}
	fn := runtime.FuncForPC(h.pcs[len(h.pcs)-1])
	if fn == nil {
		return ""
	}
	return fn.Name()
}

func newDepthTestLogger(isTerminal bool) (*Logger, *pcCaptureHandler) {
	h := &pcCaptureHandler{}
	var sh slog.Handler = h
	if isTerminal {
		sh = withTerminalCallDepth(4, sh)
	} else {
		sh = withCallDepth(4, sh)
	}
	return &Logger{sl: slog.New(sh), rl: newRateLimiter()}, h
}

func TestCallDepthTerminalDebugUsesDynamicResolverForWhen(t *testing.T) {
	setTestLevel(t, slog.LevelDebug)

	l, h := newDepthTestLogger(true)
	l.When(true).Info("x")

	fn := h.lastFunction()
	assert.NotEmpty(t, fn)
	assert.False(t, isSkippedCaller(fn), "expected non-internal caller, got %q", fn)
}

func TestCallDepthTerminalDebugUsesDynamicResolverForOnce(t *testing.T) {
	setTestLevel(t, slog.LevelDebug)

	l, h := newDepthTestLogger(true)
	l.Once("k").Info("x")

	fn := h.lastFunction()
	assert.NotEmpty(t, fn)
	assert.False(t, isSkippedCaller(fn), "expected non-internal caller, got %q", fn)
}

func TestCallDepthGatingUsesFixedPathOutsideTerminalDebug(t *testing.T) {
	t.Run("terminal non-debug", func(t *testing.T) {
		setTestLevel(t, slog.LevelInfo)

		l, h := newDepthTestLogger(true)
		l.When(true).Info("x")

		fn := h.lastFunction()
		assert.NotEmpty(t, fn)
		assert.True(t, isSkippedCaller(fn), "expected internal frame with fixed path, got %q", fn)
	})

	t.Run("non-terminal debug", func(t *testing.T) {
		setTestLevel(t, slog.LevelDebug)

		l, h := newDepthTestLogger(false)
		l.When(true).Info("x")

		fn := h.lastFunction()
		assert.NotEmpty(t, fn)
		assert.True(t, isSkippedCaller(fn), "expected internal frame with fixed path, got %q", fn)
	})
}
