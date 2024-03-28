// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/user"
	"path/filepath"
	"strings"

	"github.com/netdata/netdata/go/go.d.plugin/agent"
	"github.com/netdata/netdata/go/go.d.plugin/agent/executable"
	"github.com/netdata/netdata/go/go.d.plugin/cli"
	"github.com/netdata/netdata/go/go.d.plugin/logger"
	"github.com/netdata/netdata/go/go.d.plugin/pkg/buildinfo"
	"github.com/netdata/netdata/go/go.d.plugin/pkg/multipath"

	"github.com/jessevdk/go-flags"
	"golang.org/x/net/http/httpproxy"

	_ "github.com/netdata/netdata/go/go.d.plugin/modules"
)

var (
	cd, _       = os.Getwd()
	name        = "go.d"
	userDir     = os.Getenv("NETDATA_USER_CONFIG_DIR")
	stockDir    = os.Getenv("NETDATA_STOCK_CONFIG_DIR")
	varLibDir   = os.Getenv("NETDATA_LIB_DIR")
	lockDir     = os.Getenv("NETDATA_LOCK_DIR")
	watchPath   = os.Getenv("NETDATA_PLUGINS_GOD_WATCH_PATH")
	envLogLevel = os.Getenv("NETDATA_LOG_LEVEL")
)

func confDir(opts *cli.Option) multipath.MultiPath {
	if len(opts.ConfDir) > 0 {
		return opts.ConfDir
	}
	if userDir != "" || stockDir != "" {
		return multipath.New(
			userDir,
			stockDir,
		)
	}
	if executable.Directory != "" {
		return multipath.New(
			filepath.Join(executable.Directory, "/../../../../etc/netdata"),
			filepath.Join(executable.Directory, "/../../../../usr/lib/netdata/conf.d"),
		)
	}
	return multipath.New(
		filepath.Join(cd, "/../../../../etc/netdata"),
		filepath.Join(cd, "/../../../../usr/lib/netdata/conf.d"),
	)
}

func modulesConfDir(opts *cli.Option) (mpath multipath.MultiPath) {
	if len(opts.ConfDir) > 0 {
		return opts.ConfDir
	}
	if userDir != "" || stockDir != "" {
		if userDir != "" {
			mpath = append(mpath, filepath.Join(userDir, name))
		}
		if stockDir != "" {
			mpath = append(mpath, filepath.Join(stockDir, name))
		}
		return multipath.New(mpath...)
	}
	if executable.Directory != "" {
		return multipath.New(
			filepath.Join(executable.Directory, "/../../../../etc/netdata", name),
			filepath.Join(executable.Directory, "/../../../../usr/lib/netdata/conf.d", name),
		)
	}
	return multipath.New(
		filepath.Join(cd, "/../../../../etc/netdata", name),
		filepath.Join(cd, "/../../../../usr/lib/netdata/conf.d", name),
	)
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
