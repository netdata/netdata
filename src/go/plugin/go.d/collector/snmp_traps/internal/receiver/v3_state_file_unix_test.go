// SPDX-License-Identifier: GPL-3.0-or-later

//go:build unix

package receiver

import (
	"path/filepath"
	"syscall"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/safefile"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEngineStateRejectsFIFO(t *testing.T) {
	for _, name := range []string{"engine-boots", "local-engine-id"} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), name)
			require.NoError(t, syscall.Mkfifo(path, 0600))
			require.ErrorIs(t, initEngineStateFile(name, path), safefile.ErrNotRegular)
			require.ErrorIs(t, persistEngineStateFile(name, path, []byte("state")), safefile.ErrNotRegular)
			// Simulate a FIFO appearing after the preliminary Lstat. Opening must
			// return without waiting for a writer so the descriptor can be rejected.
			f, err := openEngineStateFile(path)
			require.NoError(t, err)
			defer f.Close()
			info, err := f.Stat()
			require.NoError(t, err)
			assert.False(t, info.Mode().IsRegular())
		})
	}
}
