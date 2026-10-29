// SPDX-License-Identifier: GPL-3.0-or-later

//go:build unix

package filelock

import (
	"syscall"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/safefile"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLockerRejectsFIFO(t *testing.T) {
	l := New(t.TempDir())
	require.NoError(t, syscall.Mkfifo(l.filename("name"), 0600))
	ok, err := l.Lock("name")
	assert.False(t, ok)
	assert.ErrorIs(t, err, safefile.ErrNotRegular)
	assert.False(t, l.isLocked("name"))
}
