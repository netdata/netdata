// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"context"
	"fmt"

	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/resolver"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"gopkg.in/yaml.v2"
)

func newConfigModule(creator collectorapi.Creator) (configModule, error) {
	if creator.CreateV2 != nil {
		mod := creator.CreateV2()
		if mod == nil {
			return nil, fmt.Errorf("CreateV2 returned nil")
		}
		return mod, nil
	}
	if creator.Create == nil {
		return nil, fmt.Errorf("no module creator is defined")
	}
	mod := creator.Create()
	if mod == nil {
		return nil, fmt.Errorf("Create returned nil")
	}
	return mod, nil
}

func applyConfig(
	ctx context.Context,
	cfg confgroup.Config,
	module any,
	resolver *secretresolver.Resolver,
	storeService secretstore.Service,
	storeSnapshot *secretstore.Snapshot,
) error {
	if resolver == nil {
		return fmt.Errorf("secret resolver is nil")
	}
	cfgResolved, err := cfg.Clone()
	if err != nil {
		return fmt.Errorf("cloning config: %w", err)
	}
	storeResolver := secretresolver.StoreRefResolver(nil)
	if storeService != nil {
		storeResolver = func(resolveCtx context.Context, ref, original string) (string, error) {
			if resolveCtx == nil {
				resolveCtx = ctx
			}
			return storeService.Resolve(resolveCtx, storeSnapshot, ref, original)
		}
	}
	if err := resolver.ResolveWithStoreResolver(ctx, cfgResolved, storeResolver); err != nil {
		return fmt.Errorf("resolving secrets: %w", err)
	}
	bs, err := yaml.Marshal(cfgResolved)
	if err != nil {
		return err
	}
	return yaml.Unmarshal(bs, module)
}

// applyConfigRaw applies config without resolving secrets.
// Used by dyncfg get to avoid exposing resolved secret values.
func applyConfigRaw(cfg confgroup.Config, module any) error {
	bs, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return yaml.Unmarshal(bs, module)
}

func makeLabels(cfg confgroup.Config) map[string]string {
	labels := make(map[string]string)
	for name, value := range cfg.Labels() {
		n, ok1 := name.(string)
		v, ok2 := value.(string)
		if ok1 && ok2 {
			labels[n] = v
		}
	}
	return labels
}
