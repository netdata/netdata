// SPDX-License-Identifier: GPL-3.0-or-later

package logger

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestContextWithLoggerAndLoggerFromContext(t *testing.T) {
	log := New()

	ctx := ContextWithLogger(nil, log)
	got, ok := LoggerFromContext(ctx)

	assert.True(t, ok)
	assert.Same(t, log, got)
}

func TestContextWithLogger_IgnoresNilLogger(t *testing.T) {
	ctx := ContextWithLogger(context.Background(), nil)
	got, ok := LoggerFromContext(ctx)

	assert.False(t, ok)
	assert.Nil(t, got)
}
