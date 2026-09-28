// SPDX-License-Identifier: GPL-3.0-or-later

//go:build scripts_native_dev

package main

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/netdata/netdata/go/plugins/cmd/internal/discoveryproviders"
	"github.com/netdata/netdata/go/plugins/pkg/multipath"
	"github.com/netdata/netdata/go/plugins/plugin/agent/discovery"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/scripts.d/collector/native"
)

// Native packages are explicit startup inputs, never production registrations.
func configurePackages(
	paths multipath.MultiPath,
	base collectorapi.Registry,
) (collectorapi.Registry, discovery.ProviderFactory, error) {
	registry := base
	for _, dir := range paths {
		path := filepath.Join(dir, "scripts.d.packages.yaml")
		if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
			continue
		} else if err != nil {
			return nil, nil, err
		}
		var err error
		registry, err = native.LoadPackages(path, base)
		if err != nil {
			return nil, nil, err
		}
		break // The first configuration directory wins, just like scripts.d.conf.
	}
	dummy := discoveryproviders.Dummy()
	filtered := discovery.NewProviderFactory(
		dummy.Name(),
		func(ctx discovery.BuildContext) (discovery.Discoverer, bool, error) {
			names := make([]string, 0, len(ctx.DummyNames))
			for _, name := range ctx.DummyNames {
				if _, existing := base[name]; existing {
					names = append(names, name)
				}
			}
			ctx.DummyNames = names
			return dummy.Build(ctx)
		},
	)
	return registry, filtered, nil
}
