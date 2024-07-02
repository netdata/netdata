package logger

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNew(t *testing.T) {
	tests := map[string]*Logger{
		"default logger": New(),
		"nil logger":     nil,
	}

	for name, logger := range tests {
		t.Run(name, func(t *testing.T) {
			f := func() { logger.Infof("test %s", "test") }
			assert.NotPanics(t, f)
		})
	}
}
