// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"os"
	"path"

	"github.com/netdata/netdata/go/go.d.plugin/agent"
	"github.com/netdata/netdata/go/go.d.plugin/agent/module"
	"github.com/netdata/netdata/go/go.d.plugin/cli"
	"github.com/netdata/netdata/go/go.d.plugin/logger"
	"github.com/netdata/netdata/go/go.d.plugin/pkg/multipath"

	"github.com/jessevdk/go-flags"
)

var version = "v0.0.1-example"

type example struct{ module.Base }

func (example) Cleanup() {}

func (example) Init() error { return nil }

func (example) Check() error { return nil }

func (example) Charts() *module.Charts {
	return &module.Charts{
		{
			ID:    "random",
			Title: "A Random Number", Units: "random", Fam: "random",
			Dims: module.Dims{
				{ID: "random0", Name: "random 0"},
				{ID: "random1", Name: "random 1"},
			},
		},
	}
}
func (example) Configuration() any { return nil }

func (e *example) Collect() map[string]int64 {
	return map[string]int64{
		"random0": rand.Int63n(100),
		"random1": rand.Int63n(100),
	}
}

var (
	cd, _    = os.Getwd()
	name     = "goplugin"
	userDir  = os.Getenv("NETDATA_USER_CONFIG_DIR")
	stockDir = os.Getenv("NETDATA_STOCK_CONFIG_DIR")
)

func confDir(dirs []string) (mpath multipath.MultiPath) {
	if len(dirs) > 0 {
		return dirs
	}
	if userDir != "" && stockDir != "" {
		return multipath.New(
			userDir,
			stockDir,
		)
	}
	return multipath.New(
		path.Join(cd, "/../../../../etc/netdata"),
		path.Join(cd, "/../../../../usr/lib/netdata/conf.d"),
	)
}

func modulesConfDir(dirs []string) multipath.MultiPath {
	if len(dirs) > 0 {
		return dirs
	}
	if userDir != "" && stockDir != "" {
		return multipath.New(
			path.Join(userDir, name),
			path.Join(stockDir, name),
		)
	}
	return multipath.New(
		path.Join(cd, "/../../../../etc/netdata", name),
		path.Join(cd, "/../../../../usr/lib/netdata/conf.d", name),
	)
}

func main() {
	opt := parseCLI()

	if opt.Debug {
		logger.Level.Set(slog.LevelDebug)
	}
	if opt.Version {
		fmt.Println(version)
		os.Exit(0)
	}

	module.Register("example", module.Creator{
		Create: func() module.Module { return &example{} }},
	)

	p := agent.New(agent.Config{
		Name:                 name,
		ConfDir:              confDir(opt.ConfDir),
		ModulesConfDir:       modulesConfDir(opt.ConfDir),
		ModulesConfWatchPath: opt.WatchPath,
		RunModule:            opt.Module,
		MinUpdateEvery:       opt.UpdateEvery,
	})

	p.Run()
}

func parseCLI() *cli.Option {
	opt, err := cli.Parse(os.Args)
	var flagsErr *flags.Error
	if errors.As(err, &flagsErr) && errors.Is(flagsErr.Type, flags.ErrHelp) {
		os.Exit(0)
	} else {
		os.Exit(1)
	}
	return opt
}
