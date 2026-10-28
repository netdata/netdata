// SPDX-License-Identifier: GPL-3.0-or-later

package receiver

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/safefile"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func initEngineStateFile(name, path string) error {
	if name == "engine-boots" {
		return (&engineBoots{path: path}).init()
	}
	return (&localEngineID{path: path}).init("")
}

func TestEngineStateRejectsSymlinks(t *testing.T) {
	for name, content := range map[string]string{
		"engine-boots":    "42\n",
		"local-engine-id": testLocalEngineIDHex + "\n",
	} {
		t.Run(name, func(t *testing.T) {
			for caseName, dangling := range map[string]bool{"existing target": false, "missing target": true} {
				t.Run(caseName, func(t *testing.T) {
					dir := t.TempDir()
					target := filepath.Join(dir, "target")
					if !dangling {
						require.NoError(t, os.WriteFile(target, []byte(content), 0600))
					}
					path := filepath.Join(dir, name)
					if err := os.Symlink(target, path); err != nil {
						t.Skipf("symlinks unavailable: %v", err)
					}

					exists, err := engineStatePathExistsChecked(path)
					assert.Error(t, err)
					assert.False(t, exists)
					assert.Error(t, initEngineStateFile(name, path))
					assert.Error(t, persistEngineStateFile(name, path, []byte("replacement\n")))
					info, err := os.Lstat(path)
					require.NoError(t, err)
					assert.NotZero(t, info.Mode()&os.ModeSymlink)
					if dangling {
						_, err = os.Stat(target)
						assert.True(t, os.IsNotExist(err))
					} else {
						data, err := os.ReadFile(target)
						require.NoError(t, err)
						assert.Equal(t, content, string(data))
					}
				})
			}
		})
	}
}

func TestEngineStateTempCollisionPreservesFiles(t *testing.T) {
	for _, name := range []string{"engine-boots", "local-engine-id"} {
		t.Run(name, func(t *testing.T) {
			for caseName, symlink := range map[string]bool{"regular": false, "symlink": true} {
				t.Run(caseName, func(t *testing.T) {
					dir := t.TempDir()
					path := filepath.Join(dir, name)
					tmpPath := path + ".tmp"
					const original = "existing state\n"
					const sentinel = "pre-existing temp contents\n"
					require.NoError(t, os.WriteFile(path, []byte(original), 0640))
					target := tmpPath
					if symlink {
						target = filepath.Join(dir, "target")
						if err := os.Symlink(target, tmpPath); err != nil {
							t.Skipf("symlinks unavailable: %v", err)
						}
					}
					require.NoError(t, os.WriteFile(target, []byte(sentinel), 0600))
					require.Error(t, persistEngineStateFile(name, path, []byte("new state\n")))
					data, err := os.ReadFile(target)
					require.NoError(t, err)
					assert.Equal(t, sentinel, string(data))
					data, err = os.ReadFile(path)
					require.NoError(t, err)
					assert.Equal(t, original, string(data))
				})
			}
		})
	}
}

func TestEngineStateErrorsOmitContents(t *testing.T) {
	for name, contents := range map[string][]string{
		"engine-boots":    {"synthetic-private-value", "9223372036854775808", "-12345", "2147483648"},
		"local-engine-id": {"synthetic-private-value", "123", "0000000000", "fffffffffff"},
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), name)
			for _, content := range contents {
				require.NoError(t, os.WriteFile(path, []byte(content+"\n"), 0600))
				err := initEngineStateFile(name, path)
				require.Error(t, err)
				assert.Contains(t, err.Error(), path)
				assert.Contains(t, err.Error(), "invalid value")
				assert.NotContains(t, strings.ReplaceAll(err.Error(), path, "<path>"), content)
				assert.NotContains(t, err.Error(), "strconv")
				assert.NotContains(t, err.Error(), "encoding/hex")
			}
		})
	}
}

func TestPrepareV3RollbackPreservesForeignTemp(t *testing.T) {
	for _, name := range []string{"engine-boots", "local-engine-id"} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			paths := newEngineStatePaths(root, "job")
			require.NoError(t, os.MkdirAll(paths.dir, 0750))
			tmpPath := filepath.Join(paths.dir, name+".tmp")
			const sentinel = "pre-existing temporary file"
			require.NoError(t, os.WriteFile(tmpPath, []byte(sentinel), 0600))
			recv := New(NewPolicy(PolicyConfig{
				Versions:          []string{"v3"},
				USMUsers:          []USMUser{{Username: "testuser", EngineID: testEngineIDHex, AuthProto: "none", PrivProto: "none"}},
				EngineIDWhitelist: []string{testEngineIDHex},
			}), nil)
			require.Error(t, recv.PrepareV3(root, "job"))
			data, err := os.ReadFile(tmpPath)
			require.NoError(t, err)
			assert.Equal(t, sentinel, string(data))
			for _, path := range []string{paths.engineBoots, paths.localEngineID} {
				_, err := os.Lstat(path)
				assert.True(t, os.IsNotExist(err), "%s: %v", path, err)
			}
		})
	}
}

func TestEngineStateFileSizeLimit(t *testing.T) {
	for _, name := range []string{"engine-boots", "local-engine-id"} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), name)
			value := "42"
			if name == "local-engine-id" {
				value = testLocalEngineIDHex
			}
			content := value + strings.Repeat(" ", int(safefile.MaxSize)-len(value))
			for caseName, tc := range map[string]struct {
				content string
				wantErr error
			}{
				"at limit":   {content: content},
				"over limit": {content: content + " ", wantErr: safefile.ErrTooLarge},
			} {
				t.Run(caseName, func(t *testing.T) {
					require.NoError(t, os.WriteFile(path, []byte(tc.content), 0600))
					require.ErrorIs(t, initEngineStateFile(name, path), tc.wantErr)
				})
			}
		})
	}
}

func TestEngineStateRejectsNonRegularFiles(t *testing.T) {
	for _, name := range []string{"engine-boots", "local-engine-id"} {
		t.Run(name, func(t *testing.T) {
			for caseName, path := range map[string]string{"directory": t.TempDir(), "device": os.DevNull} {
				t.Run(caseName, func(t *testing.T) {
					require.ErrorIs(t, initEngineStateFile(name, path), safefile.ErrNotRegular)
					require.ErrorIs(t, persistEngineStateFile(name, path, []byte("state")), safefile.ErrNotRegular)
				})
			}
		})
	}
}

func TestOpenEngineStateFileDoesNotFollowSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	require.NoError(t, os.WriteFile(target, []byte("synthetic contents"), 0600))
	path := filepath.Join(dir, "state")
	if err := os.Symlink(target, path); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	// Exercise the open separately: an Lstat check alone cannot protect a raced-in link.
	f, err := openEngineStateFile(path)
	if err == nil {
		defer f.Close()
		info, err := f.Stat()
		require.NoError(t, err)
		assert.False(t, info.Mode().IsRegular(), "must open the link itself, never its regular-file target")
	}
}

func TestPrepareV3StateRoundTrip(t *testing.T) {
	root := t.TempDir()
	paths := newEngineStatePaths(root, "job")
	var engineID string
	for _, boots := range []string{"1\n", "2\n"} {
		recv := New(NewPolicy(PolicyConfig{
			Versions:          []string{"v3"},
			USMUsers:          []USMUser{{Username: "testuser", EngineID: testEngineIDHex, AuthProto: "none", PrivProto: "none"}},
			EngineIDWhitelist: []string{testEngineIDHex},
		}), nil)
		require.NoError(t, recv.PrepareV3(root, "job"))
		recv.CommitPreparedState()
		if engineID == "" {
			engineID = recv.localEngineID.hexString()
		}
		assert.Equal(t, engineID, recv.localEngineID.hexString())
		for path, want := range map[string]string{paths.engineBoots: boots, paths.localEngineID: engineID + "\n"} {
			data, err := os.ReadFile(path)
			require.NoError(t, err)
			assert.Equal(t, want, string(data))
			_, err = os.Lstat(path + ".tmp")
			assert.True(t, os.IsNotExist(err))
		}
	}
}
