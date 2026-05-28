// SPDX-License-Identifier: GPL-3.0-or-later

package discovery

import (
	"context"
	"fmt"

	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
)

// Discoverer is a discovery source runner.
type Discoverer interface {
	Run(ctx context.Context, in chan<- []*confgroup.Group)
}

// ProviderFactory builds optional discoverers from a shared build context.
type ProviderFactory interface {
	Name() string
	Build(ctx BuildContext) (Discoverer, bool, error)
}

type providerFactoryFunc struct {
	name  string
	build func(ctx BuildContext) (Discoverer, bool, error)
}

func (p providerFactoryFunc) Name() string {
	return p.name
}

func (p providerFactoryFunc) Build(ctx BuildContext) (Discoverer, bool, error) {
	if p.build == nil {
		return nil, false, fmt.Errorf("provider %q has nil build function", p.name)
	}
	return p.build(ctx)
}

// NewProviderFactory creates a named provider factory.
func NewProviderFactory(name string, build func(ctx BuildContext) (Discoverer, bool, error)) ProviderFactory {
	return providerFactoryFunc{name: name, build: build}
}
