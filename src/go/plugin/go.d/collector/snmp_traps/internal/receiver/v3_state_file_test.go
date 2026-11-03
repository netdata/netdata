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
		return (&engineBoots{
			path: path,
		}).init()
	}
	return (&localEngineID{
		path: path,
	}).init("")
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

func TestEngineStatePersistencePreservesForeignTemps(t *testing.T) {
	for _, name := range []string{"engine-boots", "local-engine-id"} {
		t.Run(name, func(t *testing.T) {
			for caseName, tc := range map[string]struct {
				suffix  string
				symlink bool
			}{
				"legacy file":        {suffix: ".tmp"},
				"legacy symlink":     {suffix: ".tmp", symlink: true},
				"randomized file":    {suffix: ".orphan.tmp"},
				"randomized symlink": {suffix: ".orphan.tmp", symlink: true},
			} {
				t.Run(caseName, func(t *testing.T) {
					dir := t.TempDir()
					path := filepath.Join(dir, name)
					tmpPath := path + tc.suffix
					const original = "existing state\n"
					const sentinel = "pre-existing temp contents\n"
					require.NoError(t, os.WriteFile(path, []byte(original), 0640))
					target := tmpPath
					if tc.symlink {
						target = filepath.Join(dir, "target")
						if err := os.Symlink(target, tmpPath); err != nil {
							t.Skipf("symlinks unavailable: %v", err)
						}
					}
					require.NoError(t, os.WriteFile(target, []byte(sentinel), 0600))
					require.NoError(t, persistEngineStateFile(name, path, []byte("new state\n")))
					data, err := os.ReadFile(target)
					require.NoError(t, err)
					assert.Equal(t, sentinel, string(data))
					info, err := os.Lstat(tmpPath)
					require.NoError(t, err)
					assert.Equal(t, tc.symlink, info.Mode()&os.ModeSymlink != 0)
					data, err = os.ReadFile(path)
					require.NoError(t, err)
					assert.Equal(t, "new state\n", string(data))
					temps, err := filepath.Glob(path + "*.tmp")
					require.NoError(t, err)
					assert.Equal(t, []string{tmpPath}, temps)
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
			const invalidBoots = "not-a-number\n"
			require.NoError(t, os.WriteFile(paths.engineBoots, []byte(invalidBoots), 0640))
			tmpPath := filepath.Join(paths.dir, name+".tmp")
			const sentinel = "pre-existing temporary file"
			require.NoError(t, os.WriteFile(tmpPath, []byte(sentinel), 0600))
			recv := New(NewPolicy(PolicyConfig{
				Versions:          []string{"v3"},
				USMUsers:          []USMUser{{Username: "testuser", EngineID: testEngineIDHex, AuthProto: "none", PrivProto: "none"}},
				EngineIDWhitelist: []string{testEngineIDHex},
			}), nil)
			require.ErrorContains(t, recv.PrepareV3(root, "job"), "engine-boots: invalid value")
			data, err := os.ReadFile(tmpPath)
			require.NoError(t, err)
			assert.Equal(t, sentinel, string(data))
			_, err = os.Lstat(paths.localEngineID)
			assert.True(t, os.IsNotExist(err))
			data, err = os.ReadFile(paths.engineBoots)
			require.NoError(t, err)
			assert.Equal(t, invalidBoots, string(data))
			temps, err := filepath.Glob(filepath.Join(paths.dir, "*.tmp"))
			require.NoError(t, err)
			assert.Equal(t, []string{tmpPath}, temps)
		})
	}
}

func TestPrepareV3RecoversOrphanedTemps(t *testing.T) {
	for _, name := range []string{"engine-boots", "local-engine-id"} {
		t.Run(name, func(t *testing.T) {
			for caseName, randomized := range map[string]bool{"legacy": false, "randomized": true} {
				t.Run(caseName, func(t *testing.T) {
					root := t.TempDir()
					paths := newEngineStatePaths(root, "job")
					policy := NewPolicy(PolicyConfig{
						Versions:          []string{"v3"},
						USMUsers:          []USMUser{{Username: "testuser", EngineID: testEngineIDHex, AuthProto: "none", PrivProto: "none"}},
						EngineIDWhitelist: []string{testEngineIDHex},
					})
					recv := New(policy, nil)
					require.NoError(t, recv.PrepareV3(root, "job"))
					recv.CommitPreparedState()
					engineID := recv.localEngineID.hexString()

					// Reconstruct the on-disk state after interruption before rename.
					var f *os.File
					var err error
					if randomized {
						f, err = os.CreateTemp(paths.dir, name+".*.tmp")
					} else {
						f, err = os.OpenFile(filepath.Join(paths.dir, name+".tmp"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0640)
					}
					require.NoError(t, err)
					t.Cleanup(func() { _ = f.Close() })
					const orphanContents = "unfinished state write\n"
					_, err = f.WriteString(orphanContents)
					require.NoError(t, err)
					require.NoError(t, f.Sync())
					require.NoError(t, f.Close())

					for _, boots := range []string{"2\n", "3\n"} {
						recv = New(policy, nil)
						require.NoError(t, recv.PrepareV3(root, "job"))
						recv.CommitPreparedState()
						assert.Equal(t, engineID, recv.localEngineID.hexString())
						for path, want := range map[string]string{
							paths.engineBoots:   boots,
							paths.localEngineID: engineID + "\n",
							f.Name():            orphanContents,
						} {
							data, err := os.ReadFile(path)
							require.NoError(t, err)
							assert.Equal(t, want, string(data))
						}
						temps, err := filepath.Glob(filepath.Join(paths.dir, "*.tmp"))
						require.NoError(t, err)
						assert.Equal(t, []string{f.Name()}, temps)
					}
				})
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
			temps, err := filepath.Glob(path + "*.tmp")
			require.NoError(t, err)
			assert.Empty(t, temps)
		}
	}
}
