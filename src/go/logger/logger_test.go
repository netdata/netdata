package logger

import (
	"bytes"
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
		WithMessageSanitizer(func(string) string { return "sanitized" }).
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
	log = NewWithWriter(&buf).WithMessageSanitizer(func(string) string {
		panic("raw panic marker")
	})
	log.Error("raw message marker")
	require.NotContains(t, buf.String(), "raw panic marker")
	require.NotContains(t, buf.String(), "raw message marker")
	require.Contains(t, buf.String(), "log message redacted")
}
