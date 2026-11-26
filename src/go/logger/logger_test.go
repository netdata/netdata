package logger

import (
	"bytes"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	tests := map[string]*Logger{
		"default logger": New(),
		"nil logger":     nil,
	}

	for name, logger := range tests {
		t.Run(name, func(t *testing.T) {
			f := func() {
				logger.Infof("test %s", "test")
				logger.When(true).Warning("warn").Else().Info("info")
				logger.Once("k").Info("once")
				logger.Limit("k", 1, time.Second).Info("limit")
				logger.ResetAllOnce()
			}
			assert.NotPanics(t, f)
		})
	}
}

func TestNewWithWriter(t *testing.T) {
	var buf bytes.Buffer

	log := NewWithWriter(&buf)
	log.Info("captured")

	assert.Contains(t, buf.String(), "captured")
}

func TestNewWithWriter_WithKeepsWriter(t *testing.T) {
	var buf bytes.Buffer

	log := NewWithWriter(&buf).With("component", "test")
	log.Info("captured")

	out := buf.String()
	assert.Contains(t, out, "captured")
	assert.True(t, strings.Contains(out, "component=test") || strings.Contains(out, "\"component\":\"test\""))
}

func TestMessageSanitizerSurvivesWithAndFailsClosed(t *testing.T) {
	var buf bytes.Buffer
	log := NewWithWriter(&buf).
		WithMessageSanitizer(func(string, []any) string { return "sanitized" }).
		With("component", "test")
	log.Error("raw marker")
	require.NotContains(t, buf.String(), "raw marker")
	require.Contains(t, buf.String(), "sanitized")

	buf.Reset()
	log.With("endpoint", "raw attribute marker").Error("raw marker")
	require.NotContains(t, buf.String(), "raw attribute marker")
	require.NotContains(t, buf.String(), "endpoint")
	require.Contains(t, buf.String(), "redacted_attribute=sanitized")

	buf.Reset()
	log = NewWithWriter(&buf).WithMessageSanitizer(func(string, []any) string {
		panic("raw panic marker")
	})
	log.Error("raw message marker")
	require.NotContains(t, buf.String(), "raw panic marker")
	require.NotContains(t, buf.String(), "raw message marker")
	require.Contains(t, buf.String(), "log message redacted")
}

func TestMessageSanitizerReceivesOperands(t *testing.T) {
	operand := errors.New("typed operand")
	tests := map[string]struct {
		log  func(*Logger)
		want []any
	}{
		"Error":               {log: func(l *Logger) { l.Error("text", operand) }, want: []any{"text", operand}},
		"Warning":             {log: func(l *Logger) { l.Warning(operand) }, want: []any{operand}},
		"Notice":              {log: func(l *Logger) { l.Notice(operand) }, want: []any{operand}},
		"Info":                {log: func(l *Logger) { l.Info(operand) }, want: []any{operand}},
		"Errorf":              {log: func(l *Logger) { l.Errorf("text %v", operand) }, want: []any{operand}},
		"Warningf":            {log: func(l *Logger) { l.Warningf("text %d %v", 1, operand) }, want: []any{1, operand}},
		"Noticef":             {log: func(l *Logger) { l.Noticef("text %v", operand) }, want: []any{operand}},
		"Infof":               {log: func(l *Logger) { l.Infof("text %v", operand) }, want: []any{operand}},
		"When":                {log: func(l *Logger) { l.When(true).Warning(operand) }, want: []any{operand}},
		"When Else formatted": {log: func(l *Logger) { l.When(false).Info("skipped").Else().Errorf("text %v", operand) }, want: []any{operand}},
		"Limit":               {log: func(l *Logger) { l.Limit("key", 1, time.Minute).Warning(operand) }, want: []any{operand}},
		"Once formatted":      {log: func(l *Logger) { l.Once("key").Noticef("text %v", operand) }, want: []any{operand}},
		"slog attribute":      {log: func(l *Logger) { l.With(slog.Any("err", operand)) }, want: []any{operand}},
		"key value attribute": {log: func(l *Logger) { l.With("err", operand) }, want: []any{operand}},
		"bare attribute":      {log: func(l *Logger) { l.With(operand) }, want: []any{operand}},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			var got [][]any
			log := NewWithWriter(&buf).WithMessageSanitizer(func(_ string, operands []any) string {
				got = append(got, operands)
				return "sanitized"
			})

			test.log(log)

			assert.Equal(t, [][]any{test.want}, got)
			assert.NotContains(t, buf.String(), "typed operand")
		})
	}
}
