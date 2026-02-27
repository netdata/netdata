package logger

import (
	"context"
	"log/slog"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

type pcCaptureHandler struct {
	mu  sync.Mutex
	pcs []uintptr
}

const expectedLoggerPrefix = "github.com/netdata/netdata/go/plugins/logger."

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

//go:noinline
func emitWhenInfo(l *Logger) {
	l.When(true).Info("x")
}

func TestCallDepthTerminalDebugUsesDynamicResolverForWhen(t *testing.T) {
	setTestLevel(t, slog.LevelDebug)

	l, h := newDepthTestLogger(true)
	emitWhenInfo(l)

	fn := h.lastFunction()
	assert.NotEmpty(t, fn)
	assert.False(t, strings.HasPrefix(fn, expectedLoggerPrefix), "expected non-logger caller, got %q", fn)
}

func TestCallDepthTerminalDebugUsesDynamicResolverForOnce(t *testing.T) {
	setTestLevel(t, slog.LevelDebug)

	l, h := newDepthTestLogger(true)
	l.Once("k").Info("x")

	fn := h.lastFunction()
	assert.NotEmpty(t, fn)
	assert.False(t, strings.HasPrefix(fn, expectedLoggerPrefix), "expected non-logger caller, got %q", fn)
}

func TestCallDepthGatingUsesFixedPathOutsideTerminalDebug(t *testing.T) {
	t.Run("terminal non-debug", func(t *testing.T) {
		setTestLevel(t, slog.LevelInfo)

		l, h := newDepthTestLogger(true)
		emitWhenInfo(l)

		fn := h.lastFunction()
		assert.NotEmpty(t, fn)
		assert.True(t, strings.HasPrefix(fn, expectedLoggerPrefix), "expected logger frame with fixed path, got %q", fn)
	})

	t.Run("non-terminal debug", func(t *testing.T) {
		setTestLevel(t, slog.LevelDebug)

		l, h := newDepthTestLogger(false)
		emitWhenInfo(l)

		fn := h.lastFunction()
		assert.NotEmpty(t, fn)
		assert.True(t, strings.HasPrefix(fn, expectedLoggerPrefix), "expected logger frame with fixed path, got %q", fn)
	})
}

func TestCallerSkipPrefixMatchesRuntimeLoggerPath(t *testing.T) {
	fn := runtime.FuncForPC(reflect.ValueOf((*Logger).Info).Pointer())
	if assert.NotNil(t, fn) {
		assert.True(t, strings.HasPrefix(fn.Name(), expectedLoggerPrefix))
	}
	assert.Equal(t, expectedLoggerPrefix, callerSkipPrefixes[0])
}

func TestResolveCallerPCFallbackMatchesFixedPath(t *testing.T) {
	setTestLevel(t, slog.LevelDebug)

	orig := callerSkipPrefixes
	callerSkipPrefixes = []string{""} // force fallback branch
	t.Cleanup(func() {
		callerSkipPrefixes = orig
	})

	fixedLogger, fixedHandler := newDepthTestLogger(false)
	emitWhenInfo(fixedLogger)
	expected := fixedHandler.lastFunction()
	assert.NotEmpty(t, expected)

	dynamicLogger, dynamicHandler := newDepthTestLogger(true)
	emitWhenInfo(dynamicLogger)
	actual := dynamicHandler.lastFunction()
	assert.NotEmpty(t, actual)

	assert.Equal(t, expected, actual)
}
