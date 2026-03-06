// SPDX-License-Identifier: GPL-3.0-or-later

package azure_monitor

import (
	"context"
	"errors"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	azcloud "github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/monitor/query/azmetrics"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resourcegraph/armresourcegraph"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/azure_monitor/azureprofiles"
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
	cfg, catalog, autoDiscover, err := c.prepareInitConfig()
	if err != nil {
		return nil, err
	}

	resourceGraph, queryExecutor, err := c.prepareInitClients(cfg)
	if err != nil {
		return nil, err
	}

	if autoDiscover {
		profiles, err := resolveAutoProfiles(ctx, cfg.SubscriptionID, resourceGraph, catalog, cfg.Profiles)
		if err != nil {
			return nil, fmt.Errorf("auto-discover resource types: %w", err)
		}
		cfg.Profiles = profiles
		if len(cfg.Profiles) == 0 {
			return nil, errors.New("auto-discovery found no Azure resources matching any known profile")
		}
		c.Infof("auto-discovery resolved profiles: %v", cfg.Profiles)
	}

	runtime, err := buildCollectorRuntimeFromConfig(cfg, catalog)
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

func (c *Collector) prepareInitConfig() (Config, azureprofiles.Catalog, bool, error) {
	cfg := c.Config
	cfg.applyDefaults()

	catalog, err := c.loadProfileCatalog()
	if err != nil {
		return Config{}, azureprofiles.Catalog{}, false, fmt.Errorf("load profiles catalog: %w", err)
	}

	autoDiscover, explicitProfiles := extractAutoKeyword(cfg.Profiles)
	cfg.Profiles = explicitProfiles

	if !autoDiscover && len(cfg.Profiles) == 0 {
		return Config{}, azureprofiles.Catalog{}, false, errors.New("no profiles configured; use 'auto' for auto-discovery or specify profile ids")
	}

	if err := cfg.validate(); err != nil {
		return Config{}, azureprofiles.Catalog{}, false, fmt.Errorf("config validation: %w", err)
	}

	return cfg, catalog, autoDiscover, nil
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

	return resourceGraph, newQueryExecutor(cfg.SubscriptionID, cfg.MaxConcurrency, credential, cloudCfg, c.newMetricsClient), nil
}

func resolveAutoProfiles(ctx context.Context, subscriptionID string, resourceGraph resourceGraphClient, catalog azureprofiles.Catalog, explicitProfiles []string) ([]string, error) {
	types, err := discoverResourceTypes(ctx, subscriptionID, resourceGraph)
	if err != nil {
		return nil, err
	}
	matched := catalog.ProfilesForResourceTypes(types)
	return mergeProfileIDs(explicitProfiles, matched), nil
}

func createCredential(auth AuthConfig, cloudCfg azcloud.Configuration) (azcore.TokenCredential, error) {
	coreOpts := azcore.ClientOptions{Cloud: cloudCfg}

	switch stringsLowerTrim(auth.Mode) {
	case authModeServicePrincipal:
		opts := &azidentity.ClientSecretCredentialOptions{ClientOptions: coreOpts}
		return azidentity.NewClientSecretCredential(auth.TenantID, auth.ClientID, auth.ClientSecret, opts)
	case authModeManagedIdentity:
		miOpts := &azidentity.ManagedIdentityCredentialOptions{ClientOptions: coreOpts}
		if stringsTrim(auth.ManagedIdentityClientID) != "" {
			miOpts.ID = azidentity.ClientID(stringsTrim(auth.ManagedIdentityClientID))
		}
		return azidentity.NewManagedIdentityCredential(miOpts)
	case authModeDefault, "":
		opts := &azidentity.DefaultAzureCredentialOptions{ClientOptions: coreOpts}
		return azidentity.NewDefaultAzureCredential(opts)
	default:
		return nil, fmt.Errorf("unsupported auth.mode: %q", auth.Mode)
	}
}

func extractAutoKeyword(profiles []string) (bool, []string) {
	auto := false
	filtered := make([]string, 0, len(profiles))
	for _, p := range profiles {
		if stringsLowerTrim(p) == profileAutoKeyword {
			auto = true
			continue
		}
		filtered = append(filtered, p)
	}
	return auto, filtered
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
