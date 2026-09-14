// SPDX-License-Identifier: GPL-3.0-or-later

package promvalidation

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/framework/chartemit"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
)

func materializeCharts(plan chartengine.Plan, owners *chartOwnershipIndex) []materializedChart {
	byID := make(map[string]*materializedChart)
	for _, action := range plan.Actions {
		switch item := action.(type) {
		case chartengine.CreateChartAction:
			profile := owners.templateProfile(item.ChartTemplateID)
			if len(owners.chartPaths(item.ChartID)) > 1 {
				profile = ""
			}
			byID[item.ChartID] = &materializedChart{
				TemplateID:    item.ChartTemplateID,
				Profile:       profile,
				Path:          owners.chartPath(item.ChartID, owners.templatePath(item.ChartTemplateID, "")),
				IDFingerprint: fingerprintID(item.ChartID),
				Context:       item.Meta.Context,
				Title:         item.Meta.Title,
				Family:        item.Meta.Family,
				Units:         item.Meta.Units,
				Priority:      item.Meta.Priority,
				Autogen:       strings.HasPrefix(item.ChartTemplateID, "__autogen__:"),
			}
		case chartengine.CreateDimensionAction:
			if chart := byID[item.ChartID]; chart != nil {
				chart.Algorithms = append(chart.Algorithms, string(item.Algorithm))
				chart.DimensionFingerprints = append(chart.DimensionFingerprints, fingerprintID(item.Name))
			}
		}
	}
	out := make([]materializedChart, 0, len(byID))
	for _, chart := range byID {
		slices.Sort(chart.Algorithms)
		chart.Algorithms = slices.Compact(chart.Algorithms)
		slices.Sort(chart.DimensionFingerprints)
		out = append(out, *chart)
	}
	return out
}

type emittedPlanInspection struct {
	inspection              chartemit.PlanInspection
	plannedCharts           int
	emittedCharts           int
	plannedDimensions       int
	emittedDimensions       int
	emptyChartIDs           []string
	emptyContexts           []string
	emptyChartPaths         []string
	emptyContextPaths       []string
	unemittedChartPaths     []string
	unemittedDimensionPaths []string
	chartCollisions         []wireChartCollisionReport
	contextCollisions       []wireContextCollisionReport
	dimensionCollisions     []dimensionCollisionReport
}

func inspectEmittedPlan(
	plan chartengine.Plan,
	typeID, jobName string,
	owners *chartOwnershipIndex,
) (emittedPlanInspection, error) {
	var result emittedPlanInspection
	type chartDefinitionKey struct {
		id      string
		context string
	}
	type dimensionDefinitionKey struct {
		chartID string
		name    string
	}
	createdCharts := make(map[chartDefinitionKey]int)
	createdDimensions := make(map[dimensionDefinitionKey]int)
	for _, action := range plan.Actions {
		switch value := action.(type) {
		case chartengine.CreateChartAction:
			result.plannedCharts++
			createdCharts[chartDefinitionKey{id: value.ChartID, context: value.Meta.Context}]++
		case chartengine.CreateDimensionAction:
			result.plannedDimensions++
			createdDimensions[dimensionDefinitionKey{chartID: value.ChartID, name: value.Name}]++
		}
	}

	inspection, err := chartemit.InspectPlan(plan, chartemit.EmitEnv{
		TypeID:      typeID,
		UpdateEvery: 1,
		Plugin:      "go.d.plugin",
		Module:      "prometheus",
		JobName:     jobName,
	})
	if err != nil {
		return result, safePublicEmitterError(err)
	}
	result.inspection = inspection

	type dimensionCount struct {
		fingerprint string
		count       int
	}
	wireChartCounts := make(map[string]int)
	rawContextsByWire := make(map[string]map[string]struct{})
	chartIDsByWire := make(map[string]map[string]struct{})
	chartIDsByContext := make(map[string]map[string]struct{})
	ownerPathsByChartFingerprint := make(map[string]map[string]struct{})
	perChart := make(map[string]map[string]*dimensionCount)
	for _, chart := range inspection.Charts {
		key := chartDefinitionKey{id: chart.SourceChartID, context: chart.SourceContext}
		if chart.Obsolete || createdCharts[key] == 0 {
			continue
		}
		createdCharts[key]--
		wireChartID := chart.WireTypeID + "." + chart.WireChartID
		result.emittedCharts++
		wireChartCounts[wireChartID]++
		if chartIDsByWire[wireChartID] == nil {
			chartIDsByWire[wireChartID] = make(map[string]struct{})
		}
		chartIDsByWire[wireChartID][chart.SourceChartID] = struct{}{}
		if chartIDsByContext[chart.WireContext] == nil {
			chartIDsByContext[chart.WireContext] = make(map[string]struct{})
		}
		chartIDsByContext[chart.WireContext][chart.SourceChartID] = struct{}{}
		chartFingerprint := fingerprintID(wireChartID)
		paths := ownerPathsByChartFingerprint[chartFingerprint]
		if paths == nil {
			paths = make(map[string]struct{})
			ownerPathsByChartFingerprint[chartFingerprint] = paths
		}
		for _, path := range owners.chartPaths(chart.SourceChartID) {
			paths[path] = struct{}{}
		}
		if perChart[chartFingerprint] == nil {
			perChart[chartFingerprint] = make(map[string]*dimensionCount)
		}
		if chart.WireChartID == "" {
			result.emptyChartIDs = append(result.emptyChartIDs, fingerprintID(chart.SourceChartID))
			result.emptyChartPaths = append(result.emptyChartPaths, owners.chartPaths(chart.SourceChartID)...)
		}
		if chart.WireContext == "" {
			result.emptyContexts = append(result.emptyContexts, fingerprintID(chart.SourceChartID))
			result.emptyContextPaths = append(result.emptyContextPaths, owners.chartPaths(chart.SourceChartID)...)
		}
		rawContexts := rawContextsByWire[chart.WireContext]
		if rawContexts == nil {
			rawContexts = make(map[string]struct{})
			rawContextsByWire[chart.WireContext] = rawContexts
		}
		rawContexts[chart.SourceContext] = struct{}{}
	}
	for _, dimension := range inspection.Dimensions {
		key := dimensionDefinitionKey{chartID: dimension.SourceChartID, name: dimension.SourceName}
		if dimension.Obsolete || createdDimensions[key] == 0 {
			continue
		}
		createdDimensions[key]--
		result.emittedDimensions++
		wireChartID := dimension.WireTypeID + "." + dimension.WireChartID
		chartFingerprint := fingerprintID(wireChartID)
		if perChart[chartFingerprint] == nil {
			perChart[chartFingerprint] = make(map[string]*dimensionCount)
		}
		item := perChart[chartFingerprint][dimension.WireName]
		if item == nil {
			item = &dimensionCount{fingerprint: fingerprintID(dimension.WireName)}
			perChart[chartFingerprint][dimension.WireName] = item
		}
		item.count++
	}
	for key, count := range createdCharts {
		if count > 0 {
			result.unemittedChartPaths = append(result.unemittedChartPaths, owners.chartPaths(key.id)...)
		}
	}
	for key, count := range createdDimensions {
		if count > 0 {
			result.unemittedDimensionPaths = append(result.unemittedDimensionPaths, owners.chartPaths(key.chartID)...)
		}
	}

	slices.Sort(result.emptyChartIDs)
	slices.Sort(result.emptyContexts)
	slices.Sort(result.emptyChartPaths)
	result.emptyChartPaths = slices.Compact(result.emptyChartPaths)
	slices.Sort(result.emptyContextPaths)
	result.emptyContextPaths = slices.Compact(result.emptyContextPaths)
	slices.Sort(result.unemittedChartPaths)
	result.unemittedChartPaths = slices.Compact(result.unemittedChartPaths)
	slices.Sort(result.unemittedDimensionPaths)
	result.unemittedDimensionPaths = slices.Compact(result.unemittedDimensionPaths)
	for wireID, count := range wireChartCounts {
		if count < 2 {
			continue
		}
		result.chartCollisions = append(result.chartCollisions, wireChartCollisionReport{
			WireIDFingerprint: fingerprintID(wireID),
			Occurrences:       count,
			Paths:             ownerPathsForChartIDs(owners, chartIDsByWire[wireID]),
		})
	}
	for wireContext, rawContexts := range rawContextsByWire {
		if len(rawContexts) < 2 {
			continue
		}
		rawFingerprints := make([]string, 0, len(rawContexts))
		for rawContext := range rawContexts {
			rawFingerprints = append(rawFingerprints, fingerprintID(rawContext))
		}
		slices.Sort(rawFingerprints)
		result.contextCollisions = append(result.contextCollisions, wireContextCollisionReport{
			WireContextFingerprint: fingerprintID(wireContext),
			RawContextFingerprints: rawFingerprints,
			Paths:                  ownerPathsForChartIDs(owners, chartIDsByContext[wireContext]),
		})
	}
	for chartFingerprint, dimensions := range perChart {
		for _, item := range dimensions {
			if item.count < 2 {
				continue
			}
			result.dimensionCollisions = append(result.dimensionCollisions, dimensionCollisionReport{
				ChartIDFingerprint:     chartFingerprint,
				DimensionIDFingerprint: item.fingerprint,
				Occurrences:            item.count,
				Paths:                  slices.Sorted(maps.Keys(ownerPathsByChartFingerprint[chartFingerprint])),
			})
		}
	}
	return result, nil
}

func ownerPathsForChartIDs(owners *chartOwnershipIndex, chartIDs map[string]struct{}) []string {
	var paths [][]string
	for chartID := range chartIDs {
		paths = append(paths, owners.chartPaths(chartID))
	}
	return joinedOwnerPaths(paths...)
}

func collectorJobFullName(jobName string) string {
	cfg := confgroup.Config{}
	cfg.SetModule("prometheus").SetName(jobName)
	cfg.ApplyDefaults(confgroup.Default{})
	return cfg.FullName()
}

func safePublicEmitterError(err error) error {
	message := "public chart emitter rejected the plan"
	if errors.Is(err, chartemit.ErrTypeIDBudgetExceeded) {
		message = "public chart emitter rejected a type.id that exceeds the maximum length"
	}
	return fmt.Errorf("%s (error fingerprint %s)", message, fingerprintID(err.Error()))
}

func fingerprintID(value string) string {
	sum := sha256.Sum256([]byte(value))
	return "sha256:" + fmt.Sprintf("%x", sum[:8])
}
