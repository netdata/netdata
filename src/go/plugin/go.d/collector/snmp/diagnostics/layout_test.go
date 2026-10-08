// SPDX-License-Identifier: GPL-3.0-or-later

package diagnostics

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestListCheckpointsRecognizesOnlyCompletedFiles(t *testing.T) {
	for name, tc := range map[string]struct {
		filename  string
		directory bool
		want      bool
	}{
		"completed":    {"checkpoint-00000000000000000001.zst", false, true},
		"temporary":    {"checkpoint-00000000000000000001.zst.tmp", false, false},
		"unrelated":    {"notes.zst", false, false},
		"noncanonical": {"checkpoint-1.zst", false, false},
		"zero":         {"checkpoint-00000000000000000000.zst", false, false},
		"overflow":     {"checkpoint-99999999999999999999.zst", false, false},
		"directory":    {"checkpoint-00000000000000000001.zst", true, false},
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			directory := filepath.Join(root, TopologyDirectory)
			require.NoError(t, os.Mkdir(directory, 0700))
			path := filepath.Join(directory, tc.filename)
			if tc.directory {
				require.NoError(t, os.Mkdir(path, 0700))
			} else {
				require.NoError(t, os.WriteFile(path, []byte("not decoded when listing"), 0600))
			}
			files, err := ListCheckpoints(root)
			require.NoError(t, err)
			require.Equal(t, tc.want, len(files) == 1)
		})
	}
}
