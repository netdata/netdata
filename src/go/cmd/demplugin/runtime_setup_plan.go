// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"path/filepath"

	"github.com/netdata/netdata/go/plugins/pkg/pluginconfig"
	"github.com/netdata/netdata/go/plugins/plugin/dem"
	runtime "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/runtime"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/runtimehost"
)

// runSetupPlan serves a read-only, bounded protocol for the administrator-owned
// host policy helper. It must never open a store or acquire/qualify executable bytes.
func runSetupPlan(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("runtime setup-plan", flag.ContinueOnError)
	flags.SetOutput(stderr)
	configDir := flags.String("config-dir", "", "directory containing administrator dem.conf")
	cache := flags.String("cache", "", "managed installation directory")
	root := flags.String("root", "", "preload final root")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected positional setup-plan arguments")
	}
	var argumentError error
	flags.Visit(func(f *flag.Flag) {
		argumentError = errors.Join(argumentError, validateSetupPlanPath(f.Value.String()))
	})
	if argumentError != nil {
		return argumentError
	}
	if *cache != "" && *root != "" {
		return errors.New("setup-plan accepts either --cache or --root")
	}
	var dirs []string
	if *configDir != "" {
		dirs = []string{*configDir}
	}
	config, err := dem.LoadConfig(pluginconfig.ResolveConfigDirs(dirs))
	if err != nil {
		return err
	}
	cfg := config.Runtime
	mode := cfg.Mode
	var path string
	switch {
	case *root != "":
		mode, path = runtimehost.ModePreloaded, *root
	case mode == runtimehost.ModeNative:
		path = runtime.DefaultNativeConfig().PayloadPath
	case mode == runtimehost.ModePreloaded:
		path = cfg.PreloadPath
	default:
		if *cache != "" {
			cfg.CachePath = *cache
		}
		varlib := varLibDir()
		// StoreConfig joins the default suffix; validate the input before it is cleaned.
		if cfg.CachePath == "" {
			if err := validateSetupPlanPath(varlib); err != nil {
				return err
			}
		}
		storeConfig, err := cfg.StoreConfig(varlib)
		if err != nil {
			return err
		}
		path = storeConfig.Path
	}
	if err := validateSetupPlanPath(path); err != nil {
		return err
	}
	canonical, err := runtime.CanonicalRoot(path)
	if err != nil {
		return err
	}
	if err := validateSetupPlanPath(canonical); err != nil {
		return err
	}
	// Native packaging fixes this build default. Runtime configuration may change
	// the selected mode, but must not change who owns automatic host setup.
	var delivery string
	switch runtimehost.Mode(runtimehost.DefaultMode) {
	case runtimehost.ModeManaged:
		delivery = "standalone"
	case runtimehost.ModeNative:
		delivery = "native-package"
	default:
		return errors.New("unknown runtime build default")
	}
	_, err = fmt.Fprintf(stdout, "netdata-dem-runtime-setup-v1\n%s\n%s\n%s\n", delivery, mode, canonical)
	return err
}

func validateSetupPlanPath(path string) error {
	if len(path) > 4096 || !filepath.IsAbs(path) {
		return errors.New("setup-plan paths must be absolute and at most 4096 bytes")
	}
	for i := range path {
		if path[i] < 0x20 || path[i] == 0x7f {
			return errors.New("setup-plan paths must not contain control bytes")
		}
	}
	return nil
}
