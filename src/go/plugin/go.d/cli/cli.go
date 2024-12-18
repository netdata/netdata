// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"strconv"

	"github.com/jessevdk/go-flags"

	"github.com/netdata/netdata/go/plugins/pkg/executable"
)

// Option defines command line options.
type Option struct {
	UpdateEvery int
	Module      string   `short:"m" long:"modules" description:"module name to run" default:"all"`
	ConfDir     []string `short:"c" long:"config-dir" description:"config dir to read"`
	WatchPath   []string `short:"w" long:"watch-path" description:"config path to watch"`
	Debug       bool     `short:"d" long:"debug" description:"debug mode"`
	Version     bool     `short:"v" long:"version" description:"display the version and exit"`
}

// Parse returns parsed command-line flags in Option struct
func Parse(args []string) (*Option, error) {
	opt := &Option{
		UpdateEvery: 1,
	}
	parser := flags.NewParser(opt, flags.Default)
	parser.Name = executable.Name
	parser.Usage = "[OPTIONS] [update every]"

	rest, err := parser.ParseArgs(args)
	if err != nil {
		return nil, err
	}

	if len(rest) > 1 {
		if opt.UpdateEvery, err = strconv.Atoi(rest[1]); err != nil {
			return nil, err
		}
	}

	return opt, nil
}

func IsHelp(err error) bool {
	return flags.WroteHelp(err)
}
