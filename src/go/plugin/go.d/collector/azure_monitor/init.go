// SPDX-License-Identifier: GPL-3.0-or-later

package azure_monitor

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	azcloud "github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/azure-sdk-for-go/sdk/monitor/query/azmetrics"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resourcegraph/armresourcegraph"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/azure_monitor/azureprofiles"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/cloudauth"
)

type initResult struct {
	config                 Config
	profileCatalog         azureprofiles.Catalog
	resourceGraph          resourceGraphClient
	queryExecutor          *queryExecutor
	supportedResourceTypes map[string]struct{}
}

type initProfileSelection struct {
	Names     []string
	AutoNames []string
	Entries   map[string]ProfileEntryConfig
}

func (c *Collector) initInstruments(runtime *collectorRuntime) error {
	if runtime == nil {
		return errors.New("nil collector runtime")
	}

	var labelKeys = []string{"resource_uid", "subscription_id", "resource_name", "resource_group", "region", "resource_type", "profile"}

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

func (c *Collector) prepareInitResult() (*initResult, error) {
	cfg, catalog, err := c.prepareInitConfig()
	if err != nil {
		return nil, err
	}

	resourceGraph, queryExecutor, err := c.prepareInitClients(cfg)
	if err != nil {
		return nil, err
	}

	supportedResourceTypes := catalogResourceTypeSet(catalog)
	return &initResult{
		config:                 cfg,
		profileCatalog:         catalog,
		resourceGraph:          resourceGraph,
		queryExecutor:          queryExecutor,
		supportedResourceTypes: supportedResourceTypes,
	}, nil
}

func (c *Collector) ensureBootstrapped(ctx context.Context) error {
	if c.runtime != nil {
		return nil
	}
	if c.resourceGraph == nil || c.queryExecutor == nil {
		return errors.New("collector is not initialized")
	}
	if len(c.supportedResourceTypes) == 0 && len(c.profileCatalog.ResourceTypes()) == 0 {
		return errors.New("collector profile catalog is not initialized")
	}

	fetched, err := fetchInitDiscovery(ctx, c.Config, c.profileCatalog, c.resourceGraph, c.supportedResourceTypes)
	if err != nil {
		return err
	}
	if len(fetched.UnsupportedTypes) > 0 {
		c.Warningf("ignoring unsupported discovered resource types: %v", fetched.UnsupportedTypes)
	}

	selection, err := resolveInitProfiles(c.Config, c.profileCatalog, fetched.ByType)
	if err != nil {
		return err
	}
	if len(selection.AutoNames) > 0 {
		c.Infof("auto-discovery resolved profiles: %v", selection.AutoNames)
	}

	// TODO(azure_monitor): Insert bootstrap-only metric-definition validation here.
	// Resolve selected profiles, fetch one representative resource per
	// (subscription_id, resource_type, metric_namespace), fail open per key on
	// lookup errors, and prune unsupported metrics/aggregations/time grains
	// before final runtime build. Because runtime is currently global per job,
	// multi-subscription capability differences need an explicit merge rule first.
	runtime, err := buildCollectorRuntimeFromConfig(selection.Names, selection.Entries, c.profileCatalog)
	if err != nil {
		return fmt.Errorf("build collector runtime: %w", err)
	}
	if err := c.initInstruments(runtime); err != nil {
		return err
	}
	now := c.now()

	c.runtime = runtime
	c.observations = newObservationState(runtime.Instruments)
	c.discovery = buildDiscoveryState(fetched.Resources, runtime, now, c.Discovery.RefreshEvery, 1)

	return nil
}

func (c *Collector) prepareInitConfig() (Config, azureprofiles.Catalog, error) {
	cfg := c.Config
	cfg.applyDefaults()

	catalog, err := c.loadProfileCatalog()
	if err != nil {
		return Config{}, azureprofiles.Catalog{}, fmt.Errorf("load profiles catalog: %w", err)
	}

	if err := cfg.validate(); err != nil {
		return Config{}, azureprofiles.Catalog{}, fmt.Errorf("config validation: %w", err)
	}
	cfg, ignoredTagPaths := sanitizeIgnoredProfileTagFilters(cfg)
	if len(ignoredTagPaths) > 0 {
		c.Warningf(
			"ignoring profile tag filters in discovery.mode %q; encode per-profile tag filtering in discovery.mode_query.kql: %s",
			discoveryModeQuery,
			strings.Join(ignoredTagPaths, ", "),
		)
	}

	return cfg, catalog, nil
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

	subscriptionID := cfg.primarySubscriptionID()
	resourceGraph, err := c.newResourceGraph(subscriptionID, credential, cloudCfg)
	if err != nil {
		return nil, nil, fmt.Errorf("create resource graph client: %w", err)
	}

	return resourceGraph, newQueryExecutor(cfg.Limits.MaxConcurrency, cfg.Timeout.Duration(), credential, cloudCfg, c.newMetricsClient), nil
}

func fetchInitDiscovery(ctx context.Context, cfg Config, catalog azureprofiles.Catalog, resourceGraph resourceGraphClient, supportedResourceTypes map[string]struct{}) (discoveryFetchResult, error) {
	if stringsLowerTrim(cfg.Discovery.Mode) == discoveryModeQuery {
		fetched, err := discoverResourcesFromQuery(
			ctx,
			cfg.subscriptionIDs(),
			cfg.Timeout.Duration(),
			resourceGraph,
			cfg.Discovery.ModeQuery.KQL,
			supportedResourceTypes,
		)
		if err != nil {
			return discoveryFetchResult{}, fmt.Errorf("discover candidate resources: %w", err)
		}
		return fetched, nil
	}

	discoveryTypes, err := initDiscoveryResourceTypes(cfg, catalog)
	if err != nil {
		return discoveryFetchResult{}, fmt.Errorf("prepare discovery scope: %w", err)
	}

	resources, byType, err := discoverResources(
		ctx,
		cfg.subscriptionIDs(),
		cfg.Timeout.Duration(),
		resourceGraph,
		discoveryTypes,
		cfg.Discovery.ModeFilters,
	)
	if err != nil {
		return discoveryFetchResult{}, fmt.Errorf("discover candidate resources: %w", err)
	}

	return discoveryFetchResult{Resources: resources, ByType: byType}, nil
}

func initDiscoveryResourceTypes(cfg Config, catalog azureprofiles.Catalog) ([]string, error) {
	switch stringsLowerTrim(cfg.Profiles.Mode) {
	case profilesModeAuto, profilesModeCombined:
		return catalog.ResourceTypes(), nil
	case profilesModeExact:
		return catalog.ResourceTypesForProfileBaseNames(entryNames(modeEntries(cfg.Profiles.ModeExact)))
	default:
		return nil, fmt.Errorf("unsupported profiles.mode %q", cfg.Profiles.Mode)
	}
}

func resolveInitProfiles(cfg Config, catalog azureprofiles.Catalog, byType map[string][]resourceInfo) (initProfileSelection, error) {
	discoveredTypes := make(map[string]struct{}, len(byType))
	for key := range byType {
		discoveredTypes[key] = struct{}{}
	}

	autoProfiles := catalog.ProfilesForResourceTypes(discoveredTypes)
	switch stringsLowerTrim(cfg.Profiles.Mode) {
	case profilesModeAuto:
		if len(autoProfiles) == 0 {
			return initProfileSelection{}, errors.New("auto-discovery found no Azure resources matching any known profile")
		}
		return initProfileSelection{
			Names:     autoProfiles,
			AutoNames: autoProfiles,
			Entries:   filterEntryMap(entryMap(modeEntries(cfg.Profiles.ModeAuto)), autoProfiles),
		}, nil
	case profilesModeExact:
		explicitNames := entryNames(modeEntries(cfg.Profiles.ModeExact))
		return initProfileSelection{
			Names:   explicitNames,
			Entries: entryMap(modeEntries(cfg.Profiles.ModeExact)),
		}, nil
	case profilesModeCombined:
		explicitNames := entryNames(modeEntries(cfg.Profiles.ModeCombined))
		return initProfileSelection{
			Names:     mergeProfileNames(explicitNames, autoProfiles),
			AutoNames: autoProfiles,
			Entries:   entryMap(modeEntries(cfg.Profiles.ModeCombined)),
		}, nil
	default:
		return initProfileSelection{}, fmt.Errorf("unsupported profiles.mode %q", cfg.Profiles.Mode)
	}
}

func createCredential(auth cloudauth.AzureADAuthConfig, cloudCfg azcloud.Configuration) (azcore.TokenCredential, error) {
	if err := auth.ValidateWithPath("auth"); err != nil {
		return nil, err
	}

	return auth.NewCredentialWithOptions(&cloudauth.AzureADCredentialOptions{
		ClientOptions: azcore.ClientOptions{Cloud: cloudCfg},
	})
}

func mergeProfileNames(explicit, discovered []string) []string {
	seen := make(map[string]struct{}, len(explicit)+len(discovered))
	merged := make([]string, 0, len(explicit)+len(discovered))
	for _, name := range explicit {
		key := stringsLowerTrim(name)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		merged = append(merged, name)
	}
	for _, name := range discovered {
		key := stringsLowerTrim(name)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		merged = append(merged, name)
	}
	return merged
}

func filterEntryMap(entries map[string]ProfileEntryConfig, active []string) map[string]ProfileEntryConfig {
	if len(entries) == 0 || len(active) == 0 {
		return nil
	}

	filtered := make(map[string]ProfileEntryConfig)
	for _, name := range active {
		entry, ok := entries[name]
		if !ok {
			continue
		}
		filtered[name] = entry
	}
	if len(filtered) == 0 {
		return nil
	}
	return filtered
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
