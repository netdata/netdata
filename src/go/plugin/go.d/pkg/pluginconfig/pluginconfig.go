// SPDX-License-Identifier: GPL-3.0-or-later

// Package pluginconfig centralizes runtime configuration for the plugin,
// including environment-derived values, CLI overrides, and computed directory
// paths. Precedence inside "user" paths: CLI --config-dir (first), then
// NETDATA_USER_CONFIG_DIR, then fallback. Overall precedence:
// User (multipath) → Stock (single).
package pluginconfig

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/netdata/netdata/go/plugins/pkg/buildinfo"
	"github.com/netdata/netdata/go/plugins/pkg/executable"
	"github.com/netdata/netdata/go/plugins/pkg/multipath"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/cli"
)

var (
	initOnce sync.Once
	env      envData
	dirs     directories
)

type envData struct {
	cygwinBase string
	userDir    string
	stockDir   string
	varLibDir  string
	watchPath  string
	logLevel   string
}

type directories struct {
	// Root config
	userConfigDirs multipath.MultiPath // includes CLI dirs (+ env user + fallback)
	stockConfigDir string

	// Collectors (derived from roots)
	collectorsUserDirs multipath.MultiPath
	collectorsStockDir string

	// Service discovery (derived from roots)
	sdUserDirs multipath.MultiPath
	sdStockDir string

	// Misc
	collectorsWatch []string
	varLibDir       string
}

// MustInit parses env, applies CLI overrides, discovers directories, and stores them.
// Safe to call multiple times; only the first call has effect.
func MustInit(opts *cli.Option) {
	initOnce.Do(func() {
		env = readEnvFromOS(executable.Directory)
		var d directories
		if err := d.build(opts, env, executable.Name, executable.Directory); err != nil {
			// fail fast during startup (internal invariant was broken)
			panic(fmt.Errorf("pluginconfig initialization failed: %w", err))
		}
		dirs = d
	})
}

func EnvLogLevel() string { return env.logLevel }

func UserConfigDirs() multipath.MultiPath { return dirs.userConfigDirsClone() }
func StockConfigDir() string              { return dirs.stockConfigDir }
func ConfigDir() multipath.MultiPath      { return dirs.configDir() }

func CollectorsUserDirs() multipath.MultiPath { return dirs.collectorsUserDirsClone() }
func CollectorsStockDir() string              { return dirs.collectorsStockDir }
func CollectorsDir() multipath.MultiPath      { return dirs.collectorsDir() }

func ServiceDiscoveryUserDirs() multipath.MultiPath { return dirs.sdUserDirsClone() }
func ServiceDiscoveryStockDir() string              { return dirs.sdStockDir }
func ServiceDiscoveryDir() multipath.MultiPath      { return dirs.serviceDiscoveryDir() }

func CollectorsConfigWatchPaths() []string { return slices.Clone(dirs.collectorsWatch) }
func VarLibDir() string                    { return dirs.varLibDir }

func (d *directories) userConfigDirsClone() multipath.MultiPath {
	return slices.Clone(d.userConfigDirs)
}
func (d *directories) collectorsUserDirsClone() multipath.MultiPath {
	return slices.Clone(d.collectorsUserDirs)
}
func (d *directories) sdUserDirsClone() multipath.MultiPath { return slices.Clone(d.sdUserDirs) }

func (d *directories) configDir() multipath.MultiPath {
	combined := append(d.userConfigDirsClone(), d.stockConfigDir)
	return multipath.New(combined...)
}
func (d *directories) collectorsDir() multipath.MultiPath {
	combined := append(d.collectorsUserDirsClone(), d.collectorsStockDir)
	return multipath.New(combined...)
}
func (d *directories) serviceDiscoveryDir() multipath.MultiPath {
	combined := append(d.sdUserDirsClone(), d.sdStockDir)
	return multipath.New(combined...)
}

func (d *directories) build(opts *cli.Option, env envData, execName, execDir string) error {
	d.initUserRoots(opts, env, execDir)
	d.initStockRoot(env, execDir)
	d.deriveCollectors(execName)
	d.deriveServiceDiscovery(execName)
	d.initWatchPaths(opts, env)
	d.initVarLib(env)
	return d.validate()
}

// Build step 1: initialize "user" roots as a multipath: CLI (highest), env, fallback.
func (d *directories) initUserRoots(opts *cli.Option, env envData, execDir string) {
	var roots multipath.MultiPath

	// 1) CLI dirs
	for _, p := range opts.ConfDir {
		p = safePathClean(handleDirOnWin(env.cygwinBase, p, execDir))
		roots = append(roots, p)
	}

	// 2) NETDATA_USER_CONFIG_DIR
	if buildinfo.UserConfigDir != "" {
		roots = append(roots, safePathClean(buildinfo.UserConfigDir))
	} else if dir := safePathClean(env.userDir); dir != "" {
		roots = append(roots, dir)
	}

	if len(roots) != 0 {
		d.userConfigDirs = multipath.New(roots...)
		return
	}

	relDir := safePathClean(filepath.Join(execDir, "..", "..", "..", "..", "etc", "netdata"))
	if isDirExists(relDir) {
		d.userConfigDirs = multipath.New(relDir)
		return
	}

	// 3) Fallback if empty
	for _, dir := range []string{
		handleDirOnWin(env.cygwinBase, "/etc/netdata", execDir),
		handleDirOnWin(env.cygwinBase, "/opt/netdata/etc/netdata", execDir),
	} {
		if isDirExists(dir) {
			d.userConfigDirs = multipath.New(dir)
			return
		}
	}

	d.userConfigDirs = multipath.New(relDir)
}

// Build step 2: initialize single "stock" root: env, common locations, build-relative fallback.
func (d *directories) initStockRoot(env envData, execDir string) {
	if buildinfo.StockConfigDir != "" {
		d.stockConfigDir = safePathClean(buildinfo.StockConfigDir)
		return
	}
	if stock := safePathClean(env.stockDir); stock != "" {
		d.stockConfigDir = stock
		return
	}

	relDir := safePathClean(filepath.Join(execDir, "..", "..", "..", "..", "usr", "lib", "netdata", "conf.d"))
	if isDirExists(relDir) {
		d.stockConfigDir = relDir
		return
	}

	for _, dir := range []string{
		handleDirOnWin(env.cygwinBase, "/usr/lib/netdata/conf.d", execDir),
		handleDirOnWin(env.cygwinBase, "/opt/netdata/usr/lib/netdata/conf.d", execDir),
	} {
		if isDirExists(dir) {
			d.stockConfigDir = safePathClean(dir)
			return
		}
	}

	d.stockConfigDir = relDir
}

// Build step 3: derive collectors dirs from roots.
func (d *directories) deriveCollectors(execName string) {
	var user multipath.MultiPath
	for _, r := range d.userConfigDirs {
		user = append(user, filepath.Join(safePathClean(r), execName))
	}
	d.collectorsUserDirs = multipath.New(user...)

	if d.stockConfigDir != "" {
		d.collectorsStockDir = filepath.Join(d.stockConfigDir, execName)
	}
}

// Build step 4: derive service-discovery dirs from roots.
func (d *directories) deriveServiceDiscovery(execName string) {
	var user multipath.MultiPath
	for _, r := range d.userConfigDirs {
		user = append(user, filepath.Join(safePathClean(r), execName, "sd"))
	}
	d.sdUserDirs = multipath.New(user...)

	if d.stockConfigDir != "" {
		d.sdStockDir = filepath.Join(d.stockConfigDir, execName, "sd")
	}
}

// Build step 5: init watchers (normalize + dedupe via multipath.New)
func (d *directories) initWatchPaths(opts *cli.Option, env envData) {
	in := append([]string{}, opts.WatchPath...)
	if env.watchPath != "" {
		in = append(in, env.watchPath)
	}
	d.collectorsWatch = multipath.New(in...)
}

// Build step 6: carry varlib
func (d *directories) initVarLib(env envData) {
	d.varLibDir = env.varLibDir
}

func (d *directories) validate() error {
	if len(d.userConfigDirs) == 0 {
		return fmt.Errorf("pluginconfig: user config dirs not initialized")
	}
	if d.stockConfigDir == "" {
		return fmt.Errorf("pluginconfig: stock config dir not initialized")
	}
	if len(d.collectorsUserDirs) == 0 {
		return fmt.Errorf("pluginconfig: collectors user dirs not derived")
	}
	if d.collectorsStockDir == "" {
		return fmt.Errorf("pluginconfig: collectors stock dir not derived")
	}
	if len(d.sdUserDirs) == 0 {
		return fmt.Errorf("pluginconfig: sd user dirs not derived")
	}
	if d.sdStockDir == "" {
		return fmt.Errorf("pluginconfig: sd stock dir not derived")
	}
	return nil
}

func readEnvFromOS(execDir string) envData {
	e := envData{
		cygwinBase: os.Getenv("NETDATA_CYGWIN_BASE_PATH"),
		userDir:    os.Getenv("NETDATA_USER_CONFIG_DIR"),
		stockDir:   os.Getenv("NETDATA_STOCK_CONFIG_DIR"),
		varLibDir:  os.Getenv("NETDATA_LIB_DIR"),
		watchPath:  os.Getenv("NETDATA_PLUGINS_GOD_WATCH_PATH"),
		logLevel:   os.Getenv("NETDATA_LOG_LEVEL"),
	}
	e.userDir = handleDirOnWin(e.cygwinBase, safePathClean(e.userDir), execDir)
	e.stockDir = handleDirOnWin(e.cygwinBase, safePathClean(e.stockDir), execDir)
	e.varLibDir = handleDirOnWin(e.cygwinBase, safePathClean(e.varLibDir), execDir)
	e.watchPath = handleDirOnWin(e.cygwinBase, safePathClean(e.watchPath), execDir)
	return e
}

// Convert a POSIX absolute (/foo) to Windows under base (e.g., C:\msys64\foo).
// If base is empty or p doesn’t start with '/', return p unchanged.
func handleDirOnWin(base, p string, execDir string) string {
	// TODO: Temporary workaround to preserve existing behavior for debug builds running under msys64.
	if base == "" && strings.HasPrefix(execDir, "C:\\msys64") {
		base = "C:\\msys64"
	}
	if base == "" || !strings.HasPrefix(p, "/") {
		return p
	}
	return filepath.Join(base, p)
}

func isDirExists(dir string) bool {
	fi, err := os.Stat(dir)
	if err != nil {
		return false
	}
	return fi.Mode().IsDir()
}

func safePathClean(p string) string {
	if p == "" {
		return ""
	}
	return filepath.Clean(p)
}
