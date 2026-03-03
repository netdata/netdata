package pluginconfig

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// helpers

func mkdir(t *testing.T, p string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(p, 0o755))
}

const testPluginName = "test.plugin"

func TestDirectoriesBuild(t *testing.T) {
	tmp := t.TempDir()
	execDir := filepath.Join(tmp, "root", "opt", "netdata", "bin")
	mkdir(t, execDir)

	// Probe locations (used ONLY when build() falls back to cygwinBase discovery).
	// These dirs must exist for probe-based selection to succeed; otherwise,
	// build() will skip them and fall back to build-relative defaults.
	probeUser := filepath.Join(tmp, "etc", "netdata")
	probeStock := filepath.Join(tmp, "usr", "lib", "netdata", "conf.d")
	mkdir(t, probeUser)
	mkdir(t, probeStock)

	tests := map[string]struct {
		input   InitInput
		env     envData
		want    directories
		wantErr bool
	}{
		// ─────────────────────────────────── core (no cygwinBase) ───────────────────────────────────
		"cli_dirs_only": {
			input: InitInput{
				ConfDir: []string{"/tmp/user1", "/tmp/user2"},
			},
			env: envData{
				stockDir: probeStock, // avoid probe/mapping; keep paths clean in expectations
			},
			want: directories{
				userConfigDirs:     []string{"/tmp/user1", "/tmp/user2"},
				stockConfigDir:     probeStock,
				collectorsUserDirs: []string{"/tmp/user1/" + testPluginName, "/tmp/user2/" + testPluginName},
				collectorsStockDir: filepath.Join(probeStock, testPluginName),
				sdUserDirs:         []string{"/tmp/user1/" + testPluginName + "/sd", "/tmp/user2/" + testPluginName + "/sd"},
				sdStockDir:         filepath.Join(probeStock, testPluginName, "sd"),
				collectorsWatch:    []string{},
				varLibDir:          "",
			},
		},
		"env_dirs_only": {
			input: InitInput{},
			env: envData{
				userDir:  "/tmp/env/user",
				stockDir: "/tmp/env/stock",
			},
			want: directories{
				userConfigDirs:     []string{"/tmp/env/user"},
				stockConfigDir:     "/tmp/env/stock",
				collectorsUserDirs: []string{"/tmp/env/user/" + testPluginName},
				collectorsStockDir: "/tmp/env/stock/" + testPluginName,
				sdUserDirs:         []string{"/tmp/env/user/" + testPluginName + "/sd"},
				sdStockDir:         "/tmp/env/stock/" + testPluginName + "/sd",
				collectorsWatch:    []string{},
				varLibDir:          "",
			},
		},
		"cli_overrides_env": {
			input: InitInput{
				ConfDir: []string{"/tmp/cli/dir"},
			},
			env: envData{
				userDir:  "/tmp/env/user",
				stockDir: "/tmp/env/stock",
			},
			want: directories{
				userConfigDirs:     []string{"/tmp/cli/dir", "/tmp/env/user"},
				stockConfigDir:     "/tmp/env/stock",
				collectorsUserDirs: []string{"/tmp/cli/dir/" + testPluginName, "/tmp/env/user/" + testPluginName},
				collectorsStockDir: "/tmp/env/stock/" + testPluginName,
				sdUserDirs:         []string{"/tmp/cli/dir/" + testPluginName + "/sd", "/tmp/env/user/" + testPluginName + "/sd"},
				sdStockDir:         "/tmp/env/stock/" + testPluginName + "/sd",
				collectorsWatch:    []string{},
				varLibDir:          "",
			},
		},
		"fallback_dirs_when_no_config": {
			input: InitInput{},
			env:   envData{},
			// build-relative fallback from execDir
			want: directories{
				userConfigDirs:     []string{filepath.Join(execDir, "..", "..", "..", "..", "etc", "netdata")},
				stockConfigDir:     filepath.Join(execDir, "..", "..", "..", "..", "usr", "lib", "netdata", "conf.d"),
				collectorsUserDirs: []string{filepath.Join(execDir, "..", "..", "..", "..", "etc", "netdata", testPluginName)},
				collectorsStockDir: filepath.Join(execDir, "..", "..", "..", "..", "usr", "lib", "netdata", "conf.d", testPluginName),
				sdUserDirs:         []string{filepath.Join(execDir, "..", "..", "..", "..", "etc", "netdata", testPluginName, "sd")},
				sdStockDir:         filepath.Join(execDir, "..", "..", "..", "..", "usr", "lib", "netdata", "conf.d", testPluginName, "sd"),
				collectorsWatch:    []string{},
				varLibDir:          "",
			},
		},
		"multiple_cli_dirs": {
			input: InitInput{
				ConfDir: []string{"/tmp/dir1", "/tmp/dir2", "/tmp/dir3"},
			},
			env: envData{
				stockDir: probeStock,
			},
			want: directories{
				userConfigDirs: []string{"/tmp/dir1", "/tmp/dir2", "/tmp/dir3"},
				stockConfigDir: probeStock,
				collectorsUserDirs: []string{
					"/tmp/dir1/" + testPluginName,
					"/tmp/dir2/" + testPluginName,
					"/tmp/dir3/" + testPluginName,
				},
				collectorsStockDir: filepath.Join(probeStock, testPluginName),
				sdUserDirs: []string{
					"/tmp/dir1/" + testPluginName + "/sd",
					"/tmp/dir2/" + testPluginName + "/sd",
					"/tmp/dir3/" + testPluginName + "/sd",
				},
				sdStockDir:      filepath.Join(probeStock, testPluginName, "sd"),
				collectorsWatch: []string{},
				varLibDir:       "",
			},
		},

		// ─────────────────────────────── explicit cygwinBase cases ───────────────────────────────
		"watch_paths_from_cli_and_env (cygwinBase)": {
			input: InitInput{
				WatchPath: []string{"/tmp/watch1", "/tmp/watch2"},
			},
			env: envData{
				cygwinBase: tmp, // exercise probe discovery
				watchPath:  "/tmp/watch/env",
			},
			want: directories{
				userConfigDirs:     []string{probeUser}, // from probe discovery
				stockConfigDir:     probeStock,
				collectorsUserDirs: []string{filepath.Join(probeUser, testPluginName)},
				collectorsStockDir: filepath.Join(probeStock, testPluginName),
				sdUserDirs:         []string{filepath.Join(probeUser, testPluginName, "sd")},
				sdStockDir:         filepath.Join(probeStock, testPluginName, "sd"),
				collectorsWatch:    []string{"/tmp/watch1", "/tmp/watch2", "/tmp/watch/env"}, // dedup preserved
				varLibDir:          "",
			},
		},
		"varlib_from_env (cygwinBase + probes)": {
			input: InitInput{},
			env: envData{
				cygwinBase: tmp,
				varLibDir:  "/var/lib/netdata",
			},
			want: directories{
				userConfigDirs:     []string{probeUser},
				stockConfigDir:     probeStock,
				collectorsUserDirs: []string{filepath.Join(probeUser, testPluginName)},
				collectorsStockDir: filepath.Join(probeStock, testPluginName),
				sdUserDirs:         []string{filepath.Join(probeUser, testPluginName, "sd")},
				sdStockDir:         filepath.Join(probeStock, testPluginName, "sd"),
				collectorsWatch:    []string{},
				varLibDir:          "/var/lib/netdata",
			},
		},
		"empty_stock_dir_from_env (cygwinBase + probes)": {
			input: InitInput{},
			env: envData{
				userDir:    "/tmp/user", // explicit user
				stockDir:   "",          // missing -> probe for stock
				cygwinBase: tmp,
			},
			want: directories{
				userConfigDirs:     []string{"/tmp/user"},
				stockConfigDir:     probeStock, // discovered
				collectorsUserDirs: []string{"/tmp/user/" + testPluginName},
				collectorsStockDir: filepath.Join(probeStock, testPluginName),
				sdUserDirs:         []string{"/tmp/user/" + testPluginName + "/sd"},
				sdStockDir:         filepath.Join(probeStock, testPluginName, "sd"),
				collectorsWatch:    []string{},
				varLibDir:          "",
			},
		},
		"env_user_pre_normalized (cygwinBase set)": {
			input: InitInput{},
			env: envData{
				cygwinBase: tmp,                                   // present but build() expects env already normalized
				userDir:    filepath.Join(tmp, "etc", "netdata"),  // pre-normalized
				stockDir:   filepath.Join(tmp, "custom", "stock"), // pre-normalized
			},
			want: directories{
				userConfigDirs:     []string{filepath.Join(tmp, "etc", "netdata")},
				stockConfigDir:     filepath.Join(tmp, "custom", "stock"),
				collectorsUserDirs: []string{filepath.Join(tmp, "etc", "netdata", testPluginName)},
				collectorsStockDir: filepath.Join(tmp, "custom", "stock", testPluginName),
				sdUserDirs:         []string{filepath.Join(tmp, "etc", "netdata", testPluginName, "sd")},
				sdStockDir:         filepath.Join(tmp, "custom", "stock", testPluginName, "sd"),
				collectorsWatch:    []string{},
				varLibDir:          "",
			},
		},
		"cli_dirs_remapped (cygwinBase)": {
			input: InitInput{
				ConfDir: []string{"/etc/netdata", "/opt/netdata/etc/netdata"},
			},
			env: envData{
				cygwinBase: tmp, // remap CLI POSIX to <tmp> paths
				stockDir:   probeStock,
			},
			want: directories{
				userConfigDirs: []string{
					filepath.Join(tmp, "etc", "netdata"),
					filepath.Join(tmp, "opt", "netdata", "etc", "netdata"),
				},
				stockConfigDir: probeStock,
				collectorsUserDirs: []string{
					filepath.Join(tmp, "etc", "netdata", testPluginName),
					filepath.Join(tmp, "opt", "netdata", "etc", "netdata", testPluginName),
				},
				collectorsStockDir: filepath.Join(probeStock, testPluginName),
				sdUserDirs: []string{
					filepath.Join(tmp, "etc", "netdata", testPluginName, "sd"),
					filepath.Join(tmp, "opt", "netdata", "etc", "netdata", testPluginName, "sd"),
				},
				sdStockDir:      filepath.Join(probeStock, testPluginName, "sd"),
				collectorsWatch: []string{},
				varLibDir:       "",
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel() // safe: build() uses only inputs, no globals
			var got directories
			err := got.build(tc.input, tc.env, testPluginName, execDir)

			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestDirectoriesBuildValidation(t *testing.T) {
	tests := map[string]struct {
		dirs    directories
		wantErr bool
	}{
		"empty_user_config_dirs": {
			wantErr: true,
			dirs: directories{
				userConfigDirs: nil,
				stockConfigDir: "/stock",
			}},
		"empty_stock_config_dir": {
			wantErr: true,
			dirs: directories{
				userConfigDirs:     []string{"/user"},
				stockConfigDir:     "",
				collectorsUserDirs: []string{"/user/plugin"},
				collectorsStockDir: "/stock/plugin",
				sdUserDirs:         []string{"/user/plugin/sd"},
				sdStockDir:         "/stock/plugin/sd",
			}},
		"empty_collectors_user_dirs": {
			wantErr: true,
			dirs: directories{
				userConfigDirs:     []string{"/user"},
				stockConfigDir:     "/stock",
				collectorsUserDirs: nil,
				collectorsStockDir: "/stock/plugin",
				sdUserDirs:         []string{"/user/plugin/sd"},
				sdStockDir:         "/stock/plugin/sd",
			},
		},
		"empty_collectors_stock_dir": {
			wantErr: true,
			dirs: directories{
				userConfigDirs:     []string{"/user"},
				stockConfigDir:     "/stock",
				collectorsUserDirs: []string{"/user/plugin"},
				collectorsStockDir: "",
				sdUserDirs:         []string{"/user/plugin/sd"},
				sdStockDir:         "/stock/plugin/sd",
			},
		},
		"empty_sd_user_dirs": {
			wantErr: true,
			dirs: directories{
				userConfigDirs:     []string{"/user"},
				stockConfigDir:     "/stock",
				collectorsUserDirs: []string{"/user/plugin"},
				collectorsStockDir: "/stock/plugin",
				sdUserDirs:         nil,
				sdStockDir:         "/stock/plugin/sd",
			},
		},
		"empty_sd_stock_dir": {
			wantErr: true,
			dirs: directories{
				userConfigDirs:     []string{"/user"},
				stockConfigDir:     "/stock",
				collectorsUserDirs: []string{"/user/plugin"},
				collectorsStockDir: "/stock/plugin",
				sdUserDirs:         []string{"/user/plugin/sd"},
				sdStockDir:         "",
			},
		},
		"valid_directories": {
			wantErr: false,
			dirs: directories{
				userConfigDirs:     []string{"/user"},
				stockConfigDir:     "/stock",
				collectorsUserDirs: []string{"/user/plugin"},
				collectorsStockDir: "/stock/plugin",
				sdUserDirs:         []string{"/user/plugin/sd"},
				sdStockDir:         "/stock/plugin/sd",
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			err := tc.dirs.validate()
			if tc.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestIsStock(t *testing.T) {
	orig := dirs
	t.Cleanup(func() { dirs = orig })

	t.Run("fallback heuristic when stock dir is not initialized", func(t *testing.T) {
		dirs = directories{}
		assert.True(t, IsStock("/usr/lib/netdata/conf.d/go.d/module.conf"))
		assert.False(t, IsStock("/etc/netdata/go.d/module.conf"))
	})

	t.Run("uses configured stock root when available", func(t *testing.T) {
		dirs = directories{stockConfigDir: "/custom/stock"}
		assert.True(t, IsStock("/custom/stock/go.d/module.conf"))
		assert.False(t, IsStock("/usr/lib/netdata/conf.d/go.d/module.conf"))
		assert.False(t, IsStock("/etc/netdata/go.d/module.conf"))
	})
}
