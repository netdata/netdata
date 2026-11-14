// SPDX-License-Identifier: GPL-3.0-or-later

package runtime

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCanonicalRootMatchesPublishedStore(t *testing.T) {
	dir := t.TempDir()
	real, alias := filepath.Join(dir, "real"), filepath.Join(dir, "alias")
	require.NoError(t, os.Mkdir(real, 0700))
	require.NoError(t, os.Symlink(real, alias))
	path := filepath.Join(alias, "new", "runtime")
	expected, err := CanonicalRoot(path)
	require.NoError(t, err)
	assert.NoDirExists(t, filepath.Join(real, "new"))
	actual, root, err := openRuntimeRoot(path, managedRootLayout)
	require.NoError(t, err)
	require.NoError(t, root.Close())
	assert.Equal(t, expected, actual)
	after, err := CanonicalRoot(path)
	require.NoError(t, err)
	assert.Equal(t, expected, after)
}

func TestEnsureRuntimeRootRejectsNonDirectory(t *testing.T) {
	dir := t.TempDir()
	file, alias := filepath.Join(dir, "file"), filepath.Join(dir, "alias")
	require.NoError(t, os.WriteFile(file, []byte("preserved"), 0600))
	require.NoError(t, os.Symlink(file, alias))
	for _, layout := range []runtimeRootLayout{managedRootLayout, ownershipRootLayout} {
		for _, path := range []string{file, alias, filepath.Join(file, "child"), filepath.Join(alias, "child")} {
			canonical, err := ensureRuntimeRoot(path, layout)
			require.Error(t, err, "%s", path)
			assert.Empty(t, canonical)
		}
	}
	data, err := os.ReadFile(file)
	require.NoError(t, err)
	assert.Equal(t, "preserved", string(data))
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Len(t, entries, 2)
}
