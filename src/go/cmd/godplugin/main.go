// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"os/user"
	"path/filepath"
	"strings"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/buildinfo"
	"github.com/netdata/netdata/go/plugins/pkg/executable"
	"github.com/netdata/netdata/go/plugins/pkg/multipath"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/cli"

	"github.com/jessevdk/go-flags"
	"go.uber.org/automaxprocs/maxprocs"
	"golang.org/x/net/http/httpproxy"

	_ "github.com/netdata/netdata/go/plugins/plugin/go.d/modules"
)

var (
	cygwinBasePath = os.Getenv("NETDATA_CYGWIN_BASE_PATH")

	name        = "go.d"
	userDir     = os.Getenv("NETDATA_USER_CONFIG_DIR")
	stockDir    = os.Getenv("NETDATA_STOCK_CONFIG_DIR")
	varLibDir   = os.Getenv("NETDATA_LIB_DIR")
	lockDir     = os.Getenv("NETDATA_LOCK_DIR")
	watchPath   = os.Getenv("NETDATA_PLUGINS_GOD_WATCH_PATH")
	envLogLevel = os.Getenv("NETDATA_LOG_LEVEL")
)

func init() {
	userDir = handleDirOnWin(userDir)
	stockDir = handleDirOnWin(stockDir)
	varLibDir = handleDirOnWin(varLibDir)
	lockDir = handleDirOnWin(lockDir)
	watchPath = handleDirOnWin(watchPath)
}

func confDir(opts *cli.Option) multipath.MultiPath {
	if len(opts.ConfDir) > 0 {
		return opts.ConfDir
	}

	if userDir != "" || stockDir != "" {
		return multipath.New(userDir, stockDir)
	}

	var dirs []string

	dirs = append(dirs, filepath.Join(executable.Directory, "/../../../../etc/netdata"))

	for _, dir := range []string{
		handleDirOnWin("/etc/netdata"),
		handleDirOnWin("/opt/netdata/etc/netdata"),
	} {
		if isDirExists(dir) {
			dirs = append(dirs, dir)
			break
		}
	}

	dirs = append(dirs, filepath.Join(executable.Directory, "/../../../../usr/lib/netdata/conf.d"))

	for _, dir := range []string{
		handleDirOnWin("/usr/lib/netdata/conf.d"),
		handleDirOnWin("/opt/netdata/usr/lib/netdata/conf.d"),
	} {
		if isDirExists(dir) {
			dirs = append(dirs, dir)
			break
		}
	}

	return multipath.New(dirs...)
}

func modulesConfDir(opts *cli.Option) (mpath multipath.MultiPath) {
	if len(opts.ConfDir) > 0 {
		return opts.ConfDir
	}

	dirs := confDir(opts)
	for _, dir := range dirs {
		mpath = append(mpath, filepath.Join(dir, name))
	}

	return multipath.New(mpath...)
}

func modulesConfSDDir(confDir multipath.MultiPath) (mpath multipath.MultiPath) {
	for _, v := range confDir {
		mpath = append(mpath, filepath.Join(v, "sd"))
	}
	return mpath
}

func watchPaths(opts *cli.Option) []string {
	if watchPath == "" {
		return opts.WatchPath
	}
	return append(opts.WatchPath, watchPath)
}

func stateFile() string {
	if varLibDir == "" {
		return ""
	}
	return filepath.Join(varLibDir, "god-jobs-statuses.json")
}

func init() {
	// https://github.com/netdata/netdata/issues/8949#issuecomment-638294959
	if v := os.Getenv("TZ"); strings.HasPrefix(v, ":") {
		_ = os.Unsetenv("TZ")
	}
}

func main() {
	_, _ = maxprocs.Set(maxprocs.Logger(func(s string, args ...interface{}) {}))

	opts := parseCLI()

	if opts.Version {
		fmt.Printf("go.d.plugin, version: %s\n", buildinfo.Version)
		return
	}

	if envLogLevel != "" {
		logger.Level.SetByName(envLogLevel)
	}

	if opts.Debug {
		logger.Level.Set(slog.LevelDebug)
	}

	dir := modulesConfDir(opts)

	a := agent.New(agent.Config{
		Name:                 name,
		ConfDir:              confDir(opts),
		ModulesConfDir:       dir,
		ModulesConfSDDir:     modulesConfSDDir(dir),
		ModulesConfWatchPath: watchPaths(opts),
		VnodesConfDir:        confDir(opts),
		StateFile:            stateFile(),
		LockDir:              lockDir,
		RunModule:            opts.Module,
		MinUpdateEvery:       opts.UpdateEvery,
	})

	a.Debugf("plugin: name=%s, version=%s", a.Name, buildinfo.Version)
	if u, err := user.Current(); err == nil {
		a.Debugf("current user: name=%s, uid=%s", u.Username, u.Uid)
	}

	cfg := httpproxy.FromEnvironment()
	a.Infof("env HTTP_PROXY '%s', HTTPS_PROXY '%s'", cfg.HTTPProxy, cfg.HTTPSProxy)

	a.Run()
}

func parseCLI() *cli.Option {
	opt, err := cli.Parse(os.Args)
	if err != nil {
		var flagsErr *flags.Error
		if errors.As(err, &flagsErr) && errors.Is(flagsErr.Type, flags.ErrHelp) {
			os.Exit(0)
		} else {
			os.Exit(1)
		}
	}
	return opt
}

func isDirExists(dir string) bool {
	fi, err := os.Stat(dir)
	if err == nil {
		return fi.Mode().IsDir()
	}
	return !errors.Is(err, fs.ErrNotExist)
}

func handleDirOnWin(path string) string {
	base := cygwinBasePath

	// TODO: temp workaround for debug mode
	if base == "" && strings.HasPrefix(executable.Directory, "C:\\msys64") {
		base = "C:\\msys64"
	}

	if base == "" || !strings.HasPrefix(path, "/") {
		return path
	}

	return filepath.Join(base, path)
}
