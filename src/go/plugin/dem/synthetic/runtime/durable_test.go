// SPDX-License-Identifier: GPL-3.0-or-later

package runtime

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidLockFile(t *testing.T) {
	tests := map[string]struct {
		create func(t *testing.T, path string)
		valid  bool
	}{
		"empty private file": {
			create: func(t *testing.T, path string) { require.NoError(t, os.WriteFile(path, nil, 0600)) },
			valid:  true,
		},
		"readable by others": {
			create: func(t *testing.T, path string) {
				require.NoError(t, os.WriteFile(path, nil, 0600))
				require.NoError(t, os.Chmod(path, 0644))
			},
		},
		"not empty": {
			create: func(t *testing.T, path string) { require.NoError(t, os.WriteFile(path, []byte("x"), 0600)) },
		},
		"directory": {
			create: func(t *testing.T, path string) { require.NoError(t, os.Mkdir(path, 0700)) },
		},
		"symlink": {
			create: func(t *testing.T, path string) {
				target := path + "-target"
				require.NoError(t, os.WriteFile(target, nil, 0600))
				require.NoError(t, os.Symlink(target, path))
			},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "owner.lock")
			test.create(t, path)
			info, err := os.Lstat(path)
			require.NoError(t, err)
			assert.Equal(t, test.valid, validLockFile(info))
		})
	}
}

func TestPublishJSONRecordModeAndFailedWriteCleanup(t *testing.T) {
	root, err := os.OpenRoot(t.TempDir())
	require.NoError(t, err)
	defer root.Close()

	require.NoError(t, publishJSON(root, "record.json", map[string]int{"value": 1}, 0644))
	info, err := root.Lstat("record.json")
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0644), info.Mode().Perm())
	data, err := root.ReadFile("record.json")
	require.NoError(t, err)
	assert.Equal(t, "{\"value\":1}\n", string(data))

	// A directory in place of the record makes the final rename fail.
	require.NoError(t, root.Mkdir("blocked.json", 0700))
	require.NoError(t, root.WriteFile("blocked.json/keep", nil, 0600))
	require.Error(t, publishJSON(root, "blocked.json", map[string]int{"value": 2}, 0600))
	entries, err := os.ReadDir(root.Name())
	require.NoError(t, err)
	var names []string
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	assert.ElementsMatch(t, []string{"record.json", "blocked.json"}, names)
}
