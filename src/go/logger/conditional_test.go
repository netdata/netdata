package logger

import (
	"log/slog"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
)

type formatProbe struct {
	hits *atomic.Int32
}

func (p formatProbe) String() string {
	p.hits.Add(1)
	return "probe"
}

func TestWhenElseLogsOnlyChosenBranch(t *testing.T) {
	setTestLevel(t, slog.LevelDebug)

	l, h := newTestLogger(slog.LevelDebug)

	l.When(true).Warning("warn1").Else().Info("info1")
	assert.Equal(t, 1, h.count())
	assert.Equal(t, slog.LevelWarn, h.last().level)
	assert.Equal(t, "warn1", h.last().msg)

	l.When(false).Warning("warn2").Else().Info("info2")
	assert.Equal(t, 2, h.count())
	assert.Equal(t, slog.LevelInfo, h.last().level)
	assert.Equal(t, "info2", h.last().msg)
}

func TestWhenElseSkipsFormattingOnSuppressedBranch(t *testing.T) {
	setTestLevel(t, slog.LevelDebug)

	l, h := newTestLogger(slog.LevelDebug)
	var firstHits atomic.Int32
	var secondHits atomic.Int32

	l.When(true).Warningf("warn %s", formatProbe{hits: &firstHits}).Else().Infof("info %s", formatProbe{hits: &secondHits})

	assert.Equal(t, int32(1), firstHits.Load())
	assert.Equal(t, int32(0), secondHits.Load())
	assert.Equal(t, 1, h.count())
	assert.Equal(t, slog.LevelWarn, h.last().level)
}

func TestWhenWithoutElseDoesNotLogWhenConditionFalse(t *testing.T) {
	setTestLevel(t, slog.LevelDebug)

	l, h := newTestLogger(slog.LevelDebug)
	l.When(false).Infof("suppressed %d", 1)

	assert.Equal(t, 0, h.count())
}

func TestWhenNilLoggerDoesNotPanic(t *testing.T) {
	setTestLevel(t, slog.LevelDebug)

	var l *Logger
	assert.NotPanics(t, func() {
		l.When(true).Noticef("hello %s", "world").Else().Info("ignored")
	})
}
