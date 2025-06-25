// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/executable"
	"github.com/netdata/netdata/go/plugins/pkg/multipath"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/cli"
)

type envConfig struct {
	cygwinBase string
	userDir    string
	stockDir   string
	varLibDir  string
	watchPath  string
	logLevel   string
}

func newEnvConfig() *envConfig {
	cfg := &envConfig{
		cygwinBase: os.Getenv("NETDATA_CYGWIN_BASE_PATH"),
		userDir:    os.Getenv("NETDATA_USER_CONFIG_DIR"),
		stockDir:   os.Getenv("NETDATA_STOCK_CONFIG_DIR"),
		varLibDir:  os.Getenv("NETDATA_LIB_DIR"),
		watchPath:  os.Getenv("NETDATA_PLUGINS_IBM_D_WATCH_PATH"),
		logLevel:   os.Getenv("NETDATA_LOG_LEVEL"),
	}

	cfg.userDir = cfg.handleDirOnWin(cfg.userDir)
	cfg.stockDir = cfg.handleDirOnWin(cfg.stockDir)
	cfg.varLibDir = cfg.handleDirOnWin(cfg.varLibDir)
	cfg.watchPath = cfg.handleDirOnWin(cfg.watchPath)

	return cfg
}

func (c *envConfig) handleDirOnWin(path string) string {
	base := c.cygwinBase

	// TODO: temp workaround for debug mode
	if base == "" && strings.HasPrefix(executable.Directory, "C:\\msys64") {
		base = "C:\\msys64"
	}

	if base == "" || !strings.HasPrefix(path, "/") {
		return path
	}

	return filepath.Join(base, path)
}

type config struct {
	name                string
	pluginDir           multipath.MultiPath
	collectorsDir       multipath.MultiPath
	collectorsWatchPath []string
	serviceDiscoveryDir multipath.MultiPath
	varLibDir           string
}

func newConfig(opts *cli.Option, env *envConfig) *config {
	cfg := &config{
		name: "ibm.d",
	}

	cfg.pluginDir = cfg.initPluginDir(opts, env)
	cfg.collectorsDir = cfg.initCollectorsDir(opts)
	cfg.collectorsWatchPath = cfg.initCollectorsWatchPaths(opts, env)
	cfg.serviceDiscoveryDir = cfg.initServiceDiscoveryConfigDir()
	cfg.varLibDir = env.varLibDir

	return cfg
}

func (c *config) initPluginDir(opts *cli.Option, env *envConfig) multipath.MultiPath {
	if len(opts.ConfDir) > 0 {
		return opts.ConfDir
	}

	if env.userDir != "" || env.stockDir != "" {
		return multipath.New(env.userDir, env.stockDir)
	}

	dirs := []string{
		filepath.Join(executable.Directory, "/../../../../etc/netdata"),
	}

	// Find the first existing standard directory
	standardDirs := []string{
		env.handleDirOnWin("/etc/netdata"),
		env.handleDirOnWin("/opt/netdata/etc/netdata"),
	}
	for _, dir := range standardDirs {
		if isDirExists(dir) {
			dirs = append(dirs, dir)
			break
		}
	}

	dirs = append(dirs, filepath.Join(executable.Directory, "/../../../../usr/lib/netdata/conf.d"))

	// Find the first existing lib directory
	libDirs := []string{
		env.handleDirOnWin("/usr/lib/netdata/conf.d"),
		env.handleDirOnWin("/opt/netdata/usr/lib/netdata/conf.d"),
	}
	for _, dir := range libDirs {
		if isDirExists(dir) {
			dirs = append(dirs, dir)
			break
		}
	}

	return multipath.New(dirs...)
}

func (c *config) initCollectorsDir(opts *cli.Option) multipath.MultiPath {
	if len(opts.ConfDir) > 0 {
		return opts.ConfDir
	}

	c.mustPluginDir()

	var mpath multipath.MultiPath

	for _, dir := range c.pluginDir {
		mpath = append(mpath, filepath.Join(dir, c.name))
	}

	return multipath.New(mpath...)
}

func (c *config) initServiceDiscoveryConfigDir() multipath.MultiPath {
	c.mustPluginDir()

	var mpath multipath.MultiPath

	for _, v := range c.pluginDir {
		mpath = append(mpath, filepath.Join(v, c.name, "sd"))
	}

	return mpath
}

func (c *config) initCollectorsWatchPaths(opts *cli.Option, env *envConfig) []string {
	if env.watchPath == "" {
		return opts.WatchPath
	}
	return append(opts.WatchPath, env.watchPath)
}

func (c *config) mustPluginDir() {
	if len(c.pluginDir) == 0 {
		panic("plugin config init: plugin dir is empty")
	}
}

func isDirExists(dir string) bool {
	fi, err := os.Stat(dir)
	if err != nil {
		return !errors.Is(err, fs.ErrNotExist)
	}
	return fi.Mode().IsDir()
}

