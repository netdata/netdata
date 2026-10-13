// SPDX-License-Identifier: GPL-3.0-or-later

//go:build !linux

package ndexec

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestProcessTreeUnsupportedHasNoFallback(t *testing.T) {
	process, err := StartUnprivilegedProcessTree(context.Background(), ProcessOptions{}, "unused")
	assert.Nil(t, process)
	assert.ErrorIs(t, err, ErrTreeSupervisionUnsupported)
}
