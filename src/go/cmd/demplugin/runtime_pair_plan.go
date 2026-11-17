// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/netdata/netdata/go/plugins/pkg/pluginconfig"
	"github.com/netdata/netdata/go/plugins/plugin/dem"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/browserruntime"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/runtimehost"
)

// runPairPlan is a bounded, read-only protocol for shell installation owners.
// It does not execute a runtime, fetch bytes or initialize absent storage.
func runPairPlan(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("runtime pair-plan", flag.ContinueOnError)
	flags.SetOutput(stderr)
	configDir := flags.String("config-dir", "", "directory containing administrator dem.conf")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected positional pair-plan arguments")
	}
	var dirs []string
	if *configDir != "" {
		if err := validateSetupPlanPath(*configDir); err != nil {
			return err
		}
		dirs = []string{*configDir}
	}
	config, err := dem.LoadConfig(pluginconfig.ResolveConfigDirs(dirs))
	if err != nil {
		return err
	}
	cfg := config.Runtime
	enrollment, consent, path := "absent", "manual", "-"
	if cfg.Mode == runtimehost.ModeManaged {
		storeConfig, err := cfg.StoreConfig(varLibDir())
		if err != nil {
			return err
		}
		path, err = browserruntime.CanonicalRoot(storeConfig.Path)
		if err != nil {
			return err
		}
		if err := validateSetupPlanPath(path); err != nil {
			return err
		}
		if _, err := os.Stat(path); err == nil {
			store, err := browserruntime.OpenExisting(storeConfig)
			if err != nil {
				return err
			}
			status, err := store.Inspect()
			if err = errors.Join(err, store.Close()); err != nil {
				return err
			}
			if status.Release != nil {
				enrollment = "enrolled"
				if status.AutomaticUpdates {
					consent = "automatic"
				}
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	} else if cfg.Mode == runtimehost.ModePreloaded {
		// Protected preloads require an explicit matching path at deployment;
		// installers must not reinterpret their absence as managed enrollment.
		enrollment = "enrolled"
		path, err = filepath.EvalSymlinks(cfg.PreloadPath)
		if err != nil {
			return err
		}
		if err := validateSetupPlanPath(path); err != nil {
			return err
		}
	} else if _, err := os.Stat(browserruntime.DefaultNativeConfig().PayloadPath); err == nil {
		enrollment = "enrolled"
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	_, err = fmt.Fprintf(stdout, "netdata-dem-pair-plan-v1\n%s\n%s\n%s\n%s\n", cfg.Mode, enrollment, consent, path)
	return err
}
