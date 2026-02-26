package logger

import (
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
