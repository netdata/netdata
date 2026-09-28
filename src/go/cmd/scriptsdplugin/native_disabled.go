// SPDX-License-Identifier: GPL-3.0-or-later

//go:build !scripts_native_dev

package main

import (
	"github.com/netdata/netdata/go/plugins/cmd/internal/discoveryproviders"
	"github.com/netdata/netdata/go/plugins/pkg/multipath"
	"github.com/netdata/netdata/go/plugins/plugin/agent/discovery"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

func configurePackages(
	_ multipath.MultiPath,
	registry collectorapi.Registry,
) (collectorapi.Registry, discovery.ProviderFactory, error) {
	return registry, discoveryproviders.Dummy(), nil
}
