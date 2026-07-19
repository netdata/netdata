// SPDX-License-Identifier: GPL-3.0-or-later

package filepersister

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSave(t *testing.T) {
	tests := map[string]struct {
		path     func(t *testing.T) string
		data     saveTestData
		wantFile bool
		want     string
	}{
		"empty path": {
			path: func(*testing.T) string { return "" },
			data: saveTestData{value: "ignored"},
		},
		"bytes error": {
			path: func(t *testing.T) string {
				return filepath.Join(t.TempDir(), "status")
			},
			data: saveTestData{err: errors.New("marshal failed")},
		},
		"successful save": {
			path: func(t *testing.T) string {
				return filepath.Join(t.TempDir(), "status")
			},
			data:     saveTestData{value: "persisted"},
			wantFile: true,
			want:     "persisted",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			path := test.path(t)
			Save(path, test.data)
			if !test.wantFile {
				if path != "" {
					_, err := os.Stat(path)
					require.ErrorIs(t, err, os.ErrNotExist)
				}
				return
			}
			got, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Equal(t, test.want, string(got))
		})
	}
}

type saveTestData struct {
	value string
	err   error
}

func (data saveTestData) Bytes() ([]byte, error) {
	return []byte(data.value), data.err
}
