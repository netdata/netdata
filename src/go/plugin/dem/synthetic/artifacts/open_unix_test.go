//go:build unix

// SPDX-License-Identifier: GPL-3.0-or-later
package artifacts_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/artifacts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func TestFailedPrivateOpenRemovesItsTemporaryRoot(t *testing.T) {
	parent, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	// Leave room for MkdirTemp's root name, but not its additional /.lock path.
	// Root creation and rollback remain possible at this path length.
	target := unix.PathMax - len("/netdata-dem-artifacts-") - 10 - 1
	for len(parent) < target {
		if target-len(parent) == 1 {
			parent += "a"
		} else {
			parent = filepath.Join(parent, strings.Repeat("a", min(200, target-len(parent)-1)))
		}
		require.NoError(t, os.Mkdir(parent, 0700))
	}
	t.Setenv("TMPDIR", parent)
	store, err := artifacts.Open("")
	if store != nil {
		require.NoError(t, store.Close())
		t.Skip("temporary name was too short to reach the native path limit")
	}
	require.ErrorIs(t, err, unix.ENAMETOOLONG)
	var pathError *os.PathError
	require.ErrorAs(t, err, &pathError)
	require.NotEqual(t, "mkdir", pathError.Op, "failure must occur after root creation")
	entries, err := os.ReadDir(parent)
	require.NoError(t, err)
	assert.Empty(t, entries, "a failed private Open must not leave an ownerless root")
}
