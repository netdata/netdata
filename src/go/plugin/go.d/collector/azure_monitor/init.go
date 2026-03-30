// SPDX-License-Identifier: GPL-3.0-or-later

package azure_monitor

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	azcloud "github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/azure-sdk-for-go/sdk/monitor/query/azmetrics"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resourcegraph/armresourcegraph"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/azure_monitor/azureprofiles"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/cloudauth"
)

type initResult struct {
	config        Config
	resourceGraph resourceGraphClient
	queryExecutor *queryExecutor
	runtime       *collectorRuntime
}

func (c *Collector) initInstruments(runtime *collectorRuntime) error {
	if runtime == nil {
		return errors.New("nil collector runtime")
	}

	var labelKeys = []string{"resource_uid", "resource_name", "resource_group", "region", "resource_type", "profile"}

	vec := c.store.Write().SnapshotMeter("").Vec(labelKeys...)
	if runtime.Instruments == nil {
		runtime.Instruments = make(map[string]*instrumentRuntime)
	}

	for _, p := range runtime.Profiles {
		for _, m := range p.Metrics {
			for _, series := range m.Series {
				name := series.Instrument
				if _, ok := runtime.Instruments[name]; ok {
					continue
				}
				inst := &instrumentRuntime{Kind: series.Kind}
				if series.Kind == azureprofiles.SeriesKindCounter {
					inst.Counter = vec.Counter(name)
				} else {
					inst.Gauge = vec.Gauge(name)
				}
				runtime.Instruments[name] = inst
			}
		}
	}

	return nil
}

func (c *Collector) prepareInitResult(ctx context.Context) (*initResult, error) {
	cfg, catalog, autoDiscover, explicitProfiles, err := c.prepareInitConfig()
	if err != nil {
		return nil, err
	}

	resourceGraph, queryExecutor, err := c.prepareInitClients(cfg)
	if err != nil {
		return nil, err
	}

	var profileIDs []string

	if autoDiscover {
		profiles, err := resolveAutoProfiles(ctx, cfg.SubscriptionID, cfg.Timeout.Duration(), resourceGraph, catalog, explicitProfiles)
		if err != nil {
			return nil, fmt.Errorf("auto-discover resource types: %w", err)
		}
		if len(profiles) == 0 {
			return nil, errors.New("auto-discovery found no Azure resources matching any known profile")
		}
		c.Infof("auto-discovery resolved profiles: %v", profiles)
		profileIDs = profiles
	} else {
		profileIDs = explicitProfiles
	}

	runtime, err := buildCollectorRuntimeFromConfig(profileIDs, catalog)
	if err != nil {
		return nil, fmt.Errorf("build collector runtime: %w", err)
	}

	return &initResult{
		config:        cfg,
		resourceGraph: resourceGraph,
		queryExecutor: queryExecutor,
		runtime:       runtime,
	}, nil
}

func (c *Collector) prepareInitConfig() (Config, azureprofiles.Catalog, bool, []string, error) {
	cfg := c.Config
	cfg.applyDefaults()

	catalog, err := c.loadProfileCatalog()
	if err != nil {
		return Config{}, azureprofiles.Catalog{}, false, nil, fmt.Errorf("load profiles catalog: %w", err)
	}

	if err := cfg.validate(); err != nil {
		return Config{}, azureprofiles.Catalog{}, false, nil, fmt.Errorf("config validation: %w", err)
	}

	var autoDiscover bool
	var explicitProfiles []string

	switch stringsLowerTrim(cfg.ProfileSelectionMode) {
	case profileSelectionModeAuto:
		autoDiscover = true
	case profileSelectionModeExact:
		explicitProfiles = cfg.ProfileSelectionModeExact.Profiles
	case profileSelectionModeCombined:
		autoDiscover = true
		explicitProfiles = cfg.ProfileSelectionModeCombined.Profiles
	}

	return cfg, catalog, autoDiscover, explicitProfiles, nil
}

func (c *Collector) prepareInitClients(cfg Config) (resourceGraphClient, *queryExecutor, error) {
	cloudCfg, err := cloudConfigFromName(cfg.Cloud)
	if err != nil {
		return nil, nil, err
	}

	credential, err := createCredential(cfg.Auth, cloudCfg)
	if err != nil {
		return nil, nil, fmt.Errorf("create azure credential: %w", err)
	}

	resourceGraph, err := c.newResourceGraph(cfg.SubscriptionID, credential, cloudCfg)
	if err != nil {
		return nil, nil, fmt.Errorf("create resource graph client: %w", err)
	}

	return resourceGraph, newQueryExecutor(cfg.SubscriptionID, cfg.MaxConcurrency, cfg.Timeout.Duration(), credential, cloudCfg, c.newMetricsClient), nil
}

func resolveAutoProfiles(ctx context.Context, subscriptionID string, timeout time.Duration, resourceGraph resourceGraphClient, catalog azureprofiles.Catalog, explicitProfiles []string) ([]string, error) {
	types, err := discoverResourceTypes(ctx, subscriptionID, timeout, resourceGraph)
	if err != nil {
		return nil, err
	}
	matched := catalog.ProfilesForResourceTypes(types)
	return mergeProfileIDs(explicitProfiles, matched), nil
}

func createCredential(auth cloudauth.AzureADAuthConfig, cloudCfg azcloud.Configuration) (azcore.TokenCredential, error) {
	if err := auth.ValidateWithPath("auth"); err != nil {
		return nil, err
	}

	return auth.NewCredentialWithOptions(&cloudauth.AzureADCredentialOptions{
		ClientOptions: azcore.ClientOptions{Cloud: cloudCfg},
	})
}

func mergeProfileIDs(explicit, discovered []string) []string {
	seen := make(map[string]struct{}, len(explicit)+len(discovered))
	merged := make([]string, 0, len(explicit)+len(discovered))
	for _, id := range explicit {
		key := stringsLowerTrim(id)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		merged = append(merged, id)
	}
	for _, id := range discovered {
		key := stringsLowerTrim(id)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		merged = append(merged, id)
	}
	return merged
}

func defaultNewResourceGraphClient(subscriptionID string, cred azcore.TokenCredential, cloudCfg azcloud.Configuration) (resourceGraphClient, error) {
	_ = subscriptionID
	client, err := armresourcegraph.NewClient(cred, armClientOptions{Cloud: cloudCfg}.toARM())
	if err != nil {
		return nil, err
	}
	return client, nil
}

func defaultNewMetricsClient(endpoint string, cred azcore.TokenCredential, cloudCfg azcloud.Configuration) (metricsQueryClient, error) {
	client, err := azmetrics.NewClient(endpoint, cred, &azmetrics.ClientOptions{ClientOptions: azcore.ClientOptions{Cloud: cloudCfg}})
	if err != nil {
		return nil, err
	}
	return client, nil
}
