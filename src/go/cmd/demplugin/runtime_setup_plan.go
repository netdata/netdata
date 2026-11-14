// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/netdata/netdata/go/plugins/pkg/buildinfo"
	"github.com/netdata/netdata/go/plugins/pkg/pluginconfig"
	"github.com/netdata/netdata/go/plugins/plugin/dem"
	runtime "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/runtime"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/runtimehost"
)

// setup-plan is a read-only, bounded protocol for the administrator-owned host
// policy helper. It must never open a store or acquire/qualify executable bytes.
func runRuntimeSetupPlan(args []string, stdout, stderr io.Writer) (bool, error) {
	if len(args) < 2 || args[0] != "runtime" || args[1] != "setup-plan" {
		return false, nil
	}
	flags := flag.NewFlagSet("runtime setup-plan", flag.ContinueOnError)
	flags.SetOutput(stderr)
	configDir := flags.String("config-dir", "", "directory containing administrator dem.conf")
	cache := flags.String("cache", "", "managed installation directory")
	root := flags.String("root", "", "preload final root")
	if err := flags.Parse(args[2:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return true, nil
		}
		return true, err
	}
	if flags.NArg() != 0 {
		return true, errors.New("unexpected positional setup-plan arguments")
	}
	var argumentError error
	flags.Visit(func(f *flag.Flag) {
		argumentError = errors.Join(argumentError, validateSetupPlanPath(f.Value.String()))
	})
	if argumentError != nil {
		return true, argumentError
	}
	if *cache != "" && *root != "" {
		return true, errors.New("setup-plan accepts either --cache or --root")
	}
	var dirs []string
	if *configDir != "" {
		dirs = []string{*configDir}
	}
	config, err := dem.LoadConfig(pluginconfig.ResolveConfigDirs(dirs))
	if err != nil {
		return true, err
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
		varlib := os.Getenv("NETDATA_LIB_DIR")
		if varlib == "" {
			varlib = buildinfo.VarLibDir
		}
		if varlib == "" {
			varlib = buildinfo.DefaultVarLibDir
		}
		// StoreConfig joins the default suffix; validate the input before it is cleaned.
		if cfg.CachePath == "" {
			if err := validateSetupPlanPath(varlib); err != nil {
				return true, err
			}
		}
		storeConfig, err := cfg.StoreConfig(varlib)
		if err != nil {
			return true, err
		}
		path = storeConfig.Path
	}
	if err := validateSetupPlanPath(path); err != nil {
		return true, err
	}
	canonical, err := runtime.CanonicalRoot(path)
	if err != nil {
		return true, err
	}
	if err := validateSetupPlanPath(canonical); err != nil {
		return true, err
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
		return true, errors.New("unknown runtime build default")
	}
	_, err = fmt.Fprintf(stdout, "netdata-dem-runtime-setup-v1\n%s\n%s\n%s\n", delivery, mode, canonical)
	return true, err
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
