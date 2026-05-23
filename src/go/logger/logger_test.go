package logger

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
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
