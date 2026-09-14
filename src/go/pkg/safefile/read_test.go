// SPDX-License-Identifier: GPL-3.0-or-later

package safefile

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRead(t *testing.T) {
	tests := map[string]struct {
		prepare         func(t *testing.T) string
		want            []byte
		wantErrs        []error
		wantCap         int
		wantPathInError bool
	}{
		"reads a regular file at the limit": {
			prepare: func(t *testing.T) string {
				path := filepath.Join(t.TempDir(), "value")
				require.NoError(t, os.WriteFile(path, make([]byte, MaxSize), 0o600))
				return path
			},
			want:    make([]byte, MaxSize),
			wantCap: int(MaxSize) + bytes.MinRead,
		},
		"rejects a regular file over the limit": {
			prepare: func(t *testing.T) string {
				path := filepath.Join(t.TempDir(), "value")
				require.NoError(t, os.WriteFile(path, make([]byte, MaxSize+1), 0o600))
				return path
			},
			wantErrs: []error{ErrFile, ErrTooLarge},
		},
		"rejects a non-regular opened object": {
			prepare:  func(t *testing.T) string { return t.TempDir() },
			wantErrs: []error{ErrFile, ErrNotRegular},
		},
		"follows a symlink to a regular file": {
			prepare: func(t *testing.T) string {
				dir := t.TempDir()
				target := filepath.Join(dir, "target")
				link := filepath.Join(dir, "link")
				require.NoError(t, os.WriteFile(target, []byte("value"), 0o600))
				if err := os.Symlink(target, link); err != nil {
					t.Skipf("symlinks are unavailable: %v", err)
				}
				return link
			},
			want: []byte("value"),
		},
		"preserves the private filesystem cause": {
			prepare: func(t *testing.T) string {
				return filepath.Join(t.TempDir(), "missing")
			},
			wantErrs:        []error{ErrFile, os.ErrNotExist},
			wantPathInError: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			path := tc.prepare(t)

			got, err := Read(path)

			if len(tc.wantErrs) > 0 {
				require.Error(t, err)
				for _, wantErr := range tc.wantErrs {
					require.ErrorIs(t, err, wantErr)
				}
				if tc.wantPathInError {
					require.Contains(t, err.Error(), path)
				}
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
			if tc.wantCap > 0 {
				require.Equal(t, tc.wantCap, cap(got))
			}
		})
	}
}

func TestReadBoundedUnderreportedSize(t *testing.T) {
	tests := map[string]struct {
		value   string
		want    string
		wantErr error
	}{
		"grows within the bound": {
			value: strings.Repeat("x", 2048),
			want:  strings.Repeat("x", 2048),
		},
		"rejects growth over the bound": {
			value:   strings.Repeat("x", int(MaxSize+1)),
			wantErr: ErrTooLarge,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := readBounded(strings.NewReader(tc.value), 0)

			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, string(got))
		})
	}
}
