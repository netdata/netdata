// SPDX-License-Identifier: GPL-3.0-or-later

package azure_monitor

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/azure_monitor/azureprofiles"
)

func (c *Collector) collect(ctx context.Context) error {
	hasResources, err := c.refreshCollectResources(ctx)
	if err != nil {
		return err
	}
	if !hasResources {
		return nil
	}

	now := c.now()
	queryBatches := c.buildQueryBatches(now)

	dueInstruments := dueInstrumentsForBatches(queryBatches)
	markResourceStatesDue(dueInstruments, c.runtime)
	samples, err := c.collectQuerySamples(ctx, queryBatches, now)
	if err != nil {
		return err
	}
	samples = append(samples, resourceStateSamples(c.runtime, c.discovery.ByProfile)...)

	observedThisCycle := c.observations.observeSamples(samples)
	c.observations.reobserveCachedObservations(dueInstruments, observedThisCycle)

	return nil
}

func (c *Collector) refreshCollectResources(ctx context.Context) (bool, error) {
	if err := c.ensureBootstrapped(ctx); err != nil {
		return false, err
	}

	prevFetchCounter := c.discovery.FetchCounter
	resources, err := c.refreshDiscovery(ctx, false)
	if err != nil {
		if c.discovery.FetchedAt.IsZero() {
			return false, fmt.Errorf("resource discovery: %w", err)
		}
		c.Warningf("resource discovery refresh failed, continuing with last known discovery snapshot: %v", err)
		resources = c.discovery.Resources
	}
	if c.discovery.FetchCounter != prevFetchCounter {
		c.observations.pruneStaleResources(c.discovery.ByProfile)
	}
	return len(resources) > 0, nil
}

func (c *Collector) collectQuerySamples(ctx context.Context, batches []queryBatch, queryNow time.Time) ([]metricSample, error) {
	if len(batches) == 0 {
		return nil, nil
	}

	results := c.queryExecutor.runQueryBatches(ctx, batches, queryNow, c.QueryOffset)

	var (
		allSamples []metricSample
		errCount   int
	)
	for _, res := range results {
		if res.Err != nil {
			errCount++
			c.Warningf("collection batch failed: %v", res.Err)
			continue
		}
		allSamples = append(allSamples, res.Samples...)
	}

	if errCount == len(results) {
		return nil, errors.New("all Azure Monitor batch queries failed")
	}

	return allSamples, nil
}

// markResourceStatesDue marks state series as reported every cycle, so a state that discovery
// no longer returns is dropped instead of replayed from the observation cache.
func markResourceStatesDue(dueInstruments map[string]bool, runtime *collectorRuntime) {
	if runtime == nil {
		return
	}
	for _, profile := range runtime.Profiles {
		for _, state := range profile.ResourceStates {
			for _, option := range state.Options {
				dueInstruments[option.Instrument] = true
			}
		}
	}
}

// resourceStateSamples reports each resource state from discovery, so resources without
// Azure Monitor data (for example deallocated VMs) are still counted in their state.
func resourceStateSamples(runtime *collectorRuntime, byProfile map[string][]resourceInfo) []metricSample {
	if runtime == nil {
		return nil
	}

	var samples []metricSample
	for _, profile := range runtime.Profiles {
		if len(profile.ResourceStates) == 0 {
			continue
		}
		for _, resource := range byProfile[profile.Name] {
			labels := profileResourceLabels(resource, profile)
			for _, state := range profile.ResourceStates {
				current := resource.Properties[state.Property]
				if current == "" {
					continue
				}
				for _, option := range state.Options {
					value := 0.0
					if strings.EqualFold(current, option.Value) {
						value = 1
					}
					samples = append(samples, metricSample{
						Instrument: option.Instrument,
						Kind:       azureprofiles.SeriesKindGauge,
						Labels:     labels,
						Scope:      resource.HostScope,
						Value:      value,
					})
				}
			}
		}
	}
	return samples
}
