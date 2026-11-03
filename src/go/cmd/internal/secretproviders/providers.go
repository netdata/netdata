// SPDX-License-Identifier: GPL-3.0-or-later

package secretproviders

import (
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets"
	secretresolver "github.com/netdata/netdata/go/plugins/plugin/agent/secrets/resolver"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore/backends"
)

// Default selects the shipped providers for hosts that support secrets.
func Default() (*secrets.Config, error) {
	resolver, err := secretresolver.NewDefaultAtomicResolver()
	if err != nil {
		return nil, err
	}
	creators, err := secretstore.NewCreatorCatalog(backends.Creators())
	if err != nil {
		return nil, err
	}
	return &secrets.Config{
		Resolver: resolver,
		Creators: creators,
	}, nil
}
