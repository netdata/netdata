// SPDX-License-Identifier: GPL-3.0-or-later

package runtime

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCanonicalRootResolvesMissingSuffixWithoutMutation(t *testing.T) {
	dir := t.TempDir()
	real, alias := filepath.Join(dir, "real"), filepath.Join(dir, "alias")
	require.NoError(t, os.Mkdir(real, 0700))
	require.NoError(t, os.Symlink(real, alias))
	canonicalReal, err := filepath.EvalSymlinks(real)
	require.NoError(t, err)
	for _, suffix := range []string{"", "one", "one/two"} {
		path := filepath.Join(alias, suffix)
		canonical, err := CanonicalRoot(path)
		require.NoError(t, err)
		assert.Equal(t, filepath.Join(canonicalReal, suffix), canonical)
		ancestor, missing, err := resolveRuntimeAncestor(path)
		require.NoError(t, err)
		assert.Equal(t, canonical, filepath.Join(append([]string{ancestor}, missing...)...))
	}
	entries, err := os.ReadDir(real)
	require.NoError(t, err)
	assert.Empty(t, entries)
	_, err = CanonicalRoot("relative")
	require.Error(t, err)
	require.NoError(t, os.Symlink(filepath.Join(real, "absent"), filepath.Join(dir, "dangling")))
	_, err = CanonicalRoot(filepath.Join(dir, "dangling", "child"))
	require.Error(t, err)
}

func TestCanonicalRootRejectsNonDirectoryWithoutMutation(t *testing.T) {
	dir := t.TempDir()
	file, alias := filepath.Join(dir, "file"), filepath.Join(dir, "alias")
	require.NoError(t, os.WriteFile(file, []byte("preserved"), 0600))
	require.NoError(t, os.Symlink(file, alias))
	for _, path := range []string{file, alias, filepath.Join(file, "child"), filepath.Join(alias, "child")} {
		canonical, err := CanonicalRoot(path)
		require.Error(t, err, "%s", path)
		assert.Empty(t, canonical)
	}
	data, err := os.ReadFile(file)
	require.NoError(t, err)
	assert.Equal(t, "preserved", string(data))
	target, err := os.Readlink(alias)
	require.NoError(t, err)
	assert.Equal(t, file, target)
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Len(t, entries, 2)
}
