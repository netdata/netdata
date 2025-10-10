package pluginconfig

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/cli"
)

// helpers

func mkdir(t *testing.T, p string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(p, 0o755))
}

const (
	testPluginName = "test.plugin"
	testExecDir    = "/opt/netdata/bin"
)

func TestDirectoriesBuild(t *testing.T) {
	tmp := t.TempDir()

	// Probe locations (used ONLY when build() falls back to cygwinBase discovery).
	// These dirs must exist for probe-based selection to succeed; otherwise,
	// build() will skip them and fall back to build-relative defaults.
	probeUser := filepath.Join(tmp, "etc", "netdata")
	probeStock := filepath.Join(tmp, "usr", "lib", "netdata", "conf.d")
	mkdir(t, probeUser)
	mkdir(t, probeStock)

	tests := map[string]struct {
		opts    *cli.Option
		env     envData
		want    directories
		wantErr bool
	}{
		// ─────────────────────────────────── core (no cygwinBase) ───────────────────────────────────
		"cli_dirs_only": {
			opts: &cli.Option{
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
			opts: &cli.Option{},
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
			opts: &cli.Option{
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
			opts: &cli.Option{},
			env:  envData{},
			// build-relative fallback from testExecDir
			want: directories{
				userConfigDirs:     []string{filepath.Join(testExecDir, "..", "..", "..", "..", "etc", "netdata")},
				stockConfigDir:     filepath.Join(testExecDir, "..", "..", "..", "..", "usr", "lib", "netdata", "conf.d"),
				collectorsUserDirs: []string{filepath.Join(testExecDir, "..", "..", "..", "..", "etc", "netdata", testPluginName)},
				collectorsStockDir: filepath.Join(testExecDir, "..", "..", "..", "..", "usr", "lib", "netdata", "conf.d", testPluginName),
				sdUserDirs:         []string{filepath.Join(testExecDir, "..", "..", "..", "..", "etc", "netdata", testPluginName, "sd")},
				sdStockDir:         filepath.Join(testExecDir, "..", "..", "..", "..", "usr", "lib", "netdata", "conf.d", testPluginName, "sd"),
				collectorsWatch:    []string{},
				varLibDir:          "",
			},
		},
		"multiple_cli_dirs": {
			opts: &cli.Option{
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
			opts: &cli.Option{
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
			opts: &cli.Option{},
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
			opts: &cli.Option{},
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
			opts: &cli.Option{},
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
			opts: &cli.Option{
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
			err := got.build(tc.opts, tc.env, testPluginName, testExecDir)

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
