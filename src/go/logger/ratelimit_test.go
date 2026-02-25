package logger

import (
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestLimitUsesFixedWindow(t *testing.T) {
	setTestLevel(t, slog.LevelDebug)

	l, h := newTestLogger(slog.LevelDebug)
	now := time.Unix(100, 0)
	l.rl.now = func() time.Time { return now }

	for i := 0; i < 3; i++ {
		l.Limit("k", 2, 5*time.Second).Info("msg")
	}
	assert.Equal(t, 2, h.count())

	now = now.Add(6 * time.Second)
	l.Limit("k", 2, 5*time.Second).Info("msg")
	assert.Equal(t, 3, h.count())
}

func TestLimitDZeroIsInfiniteWindow(t *testing.T) {
	setTestLevel(t, slog.LevelDebug)

	l, h := newTestLogger(slog.LevelDebug)
	now := time.Unix(200, 0)
	l.rl.now = func() time.Time { return now }

	l.Limit("k", 2, 0).Info("1")
	now = now.Add(time.Hour)
	l.Limit("k", 2, 0).Info("2")
	now = now.Add(time.Hour)
	l.Limit("k", 2, 0).Info("3")

	assert.Equal(t, 2, h.count())
}

func TestLimitClampsInvalidInputs(t *testing.T) {
	setTestLevel(t, slog.LevelDebug)

	l, h := newTestLogger(slog.LevelDebug)
	l.Limit("k", 0, -time.Second).Info("a")
	l.Limit("k", 0, -time.Second).Info("b")

	assert.Equal(t, 1, h.count())
}

func TestOnceIsWrapperAndSkipsFormattingWhenSuppressed(t *testing.T) {
	setTestLevel(t, slog.LevelDebug)

	l, h := newTestLogger(slog.LevelDebug)
	var hits atomic.Int32
	p := formatProbe{hits: &hits}

	l.Once("k").Infof("probe %s", p)
	l.Once("k").Infof("probe %s", p)

	assert.Equal(t, int32(1), hits.Load())
	assert.Equal(t, 1, h.count())
}

func TestResetAllOnceDoesNotResetLimitState(t *testing.T) {
	setTestLevel(t, slog.LevelDebug)

	l, h := newTestLogger(slog.LevelDebug)
	now := time.Unix(300, 0)
	l.rl.now = func() time.Time { return now }

	l.Once("k").Info("once-1")
	l.Limit("k", 1, time.Hour).Info("limit-1")
	l.Once("k").Info("once-2-suppressed")
	l.Limit("k", 1, time.Hour).Info("limit-2-suppressed")

	assert.Equal(t, 2, h.count())

	l.ResetAllOnce()

	l.Once("k").Info("once-3")
	l.Limit("k", 1, time.Hour).Info("limit-3-still-suppressed")

	assert.Equal(t, 3, h.count())
	assert.Equal(t, "once-3", h.last().msg)
}

func TestModeNamespacesAndFirstWriterWinsParams(t *testing.T) {
	setTestLevel(t, slog.LevelDebug)

	l, h := newTestLogger(slog.LevelDebug)
	now := time.Unix(400, 0)
	l.rl.now = func() time.Time { return now }

	// Same key in different modes should be independent.
	l.Once("shared").Info("once")
	l.Limit("shared", 1, time.Hour).Info("limit")
	assert.Equal(t, 2, h.count())

	// First writer wins params for same mode+key.
	l.Limit("k", 1, time.Hour).Info("first")
	l.Limit("k", 5, time.Millisecond).Info("second-suppressed")
	assert.Equal(t, 3, h.count())

	now = now.Add(2 * time.Hour)
	l.Limit("k", 5, time.Millisecond).Info("third-after-first-window")
	assert.Equal(t, 4, h.count())

	l.rl.mu.Lock()
	entry := l.rl.entries[limitKey{mode: modeLimit, key: "k"}]
	l.rl.mu.Unlock()
	assert.NotNil(t, entry)
	assert.Equal(t, 1, entry.limit)
	assert.Equal(t, time.Hour, entry.window)
}

func TestRateLimiterSweepRemovesStaleEntries(t *testing.T) {
	setTestLevel(t, slog.LevelDebug)

	l, _ := newTestLogger(slog.LevelDebug)
	now := time.Unix(500, 0)
	l.rl.now = func() time.Time { return now }
	l.rl.ttl = time.Second
	l.rl.sweepEvery = 1

	l.Once("stale").Info("s")
	now = now.Add(2 * time.Second)
	l.Once("fresh").Info("f")

	l.rl.mu.Lock()
	_, staleOK := l.rl.entries[limitKey{mode: modeOnce, key: "stale"}]
	_, freshOK := l.rl.entries[limitKey{mode: modeOnce, key: "fresh"}]
	l.rl.mu.Unlock()

	assert.False(t, staleOK)
	assert.True(t, freshOK)
}

func TestWithSharesRateLimiterState(t *testing.T) {
	setTestLevel(t, slog.LevelDebug)

	parent, h := newTestLogger(slog.LevelDebug)
	child := parent.With("k", "v")

	parent.Once("k").Info("first")
	child.Once("k").Info("second-suppressed")

	assert.Same(t, parent.rl, child.rl)
	assert.Equal(t, 1, h.count())
}

func TestOnceConcurrentLogsOnlyOnce(t *testing.T) {
	setTestLevel(t, slog.LevelDebug)

	l, h := newTestLogger(slog.LevelDebug)
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			l.Once("concurrent").Info("x")
		}()
	}
	wg.Wait()

	assert.Equal(t, 1, h.count())
}

func TestRateLimitNilLoggerDoesNotPanic(t *testing.T) {
	setTestLevel(t, slog.LevelDebug)

	var l *Logger
	assert.NotPanics(t, func() {
		l.Once("k").Infof("x=%d", 1)
		l.Limit("k", 2, time.Second).Warning("y")
		l.ResetAllOnce()
	})
}
