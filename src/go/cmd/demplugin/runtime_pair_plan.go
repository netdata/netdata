// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"context"
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

// runPairPlan reports enrollment, or validates the source update strategy.
// It never executes a runtime, fetches bytes or initializes absent storage.
func runPairPlan(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("runtime pair-plan", flag.ContinueOnError)
	flags.SetOutput(stderr)
	configDir := flags.String("config-dir", "", "directory containing administrator dem.conf")
	sourceUpdate := flags.Bool("source-update", false, "validate and report the target source update strategy")
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
	if *sourceUpdate {
		strategy, err := sourceUpdateStrategy(ctx, cfg)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(stdout, strategy)
		return err
	}
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

// Only a completely unstamped source build retains an existing runtime. A
// malformed release stamp must not turn a published update into retention.
func sourceUpdateStrategy(ctx context.Context, cfg runtimehost.Config) (string, error) {
	agent := runtimehost.AgentRelease()
	if agent != (browserruntime.AgentRelease{}) {
		if err := agent.Validate(); err != nil {
			return "", fmt.Errorf("expected Agent release: %w", err)
		}
		return "paired", nil
	}
	switch cfg.Mode {
	case runtimehost.ModeManaged:
		storeConfig, err := cfg.StoreConfig(varLibDir())
		if err != nil {
			return "", err
		}
		store, err := browserruntime.OpenExisting(storeConfig)
		if err != nil {
			return "", err
		}
		if err := errors.Join(store.ValidateSelected(ctx), store.Close()); err != nil {
			return "", err
		}
	case runtimehost.ModePreloaded:
		if _, _, err := browserruntime.ReadPreload(cfg.PreloadPath, nil); err != nil {
			return "", err
		}
	default:
		return "", errors.New("native runtime source updates belong to the package manager")
	}
	return "retain", nil
}
