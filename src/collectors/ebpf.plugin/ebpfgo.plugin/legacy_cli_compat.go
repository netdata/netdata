// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"strconv"
)

// moduleSelection and its parser remain as compatibility helpers for the
// legacy command-line contract. Collector lifecycle is owned by Agent.
type moduleSelection uint8

const (
	moduleSelectionNone moduleSelection = iota
	moduleSelectionDCStat
	moduleSelectionFD
)

type fdCLIOverrides struct {
	reportErrors *bool
	loadMethod   *LoadMethod
}

type fdConfigResolver func() (FDLegacyConfig, error)

func resolveFDConfigForSelection(only moduleSelection, resolve fdConfigResolver) (FDLegacyConfig, bool, error) {
	cfg, err := resolve()
	if err != nil && only == moduleSelectionDCStat {
		return defaultFDLegacyConfig(), true, nil
	}
	return cfg, false, err
}

func parsePluginArgs(args []string) (updateEvery int, only moduleSelection, fd fdCLIOverrides) {
	for _, arg := range args {
		switch arg {
		case "--dcstat", "-dcstat":
			only = moduleSelectionDCStat
		case "--fd", "-fd", "--filedescriptor", "-filedescriptor":
			only = moduleSelectionFD
		case "--return", "-return":
			fd.reportErrors = new(true)
		case "--legacy", "-legacy":
			fd.loadMethod = new(LoadLegacy)
		case "--core", "-core":
			fd.loadMethod = new(LoadCore)
		default:
			if parsed, err := strconv.Atoi(arg); err == nil && parsed > 0 && updateEvery == 0 {
				updateEvery = parsed
			}
		}
	}
	return updateEvery, only, fd
}

func resolveUpdateEvery(cliArg, cfgVal, fallback int) int {
	if cfgVal > 0 {
		return cfgVal
	}
	if cliArg > 0 {
		return cliArg
	}
	return fallback
}
