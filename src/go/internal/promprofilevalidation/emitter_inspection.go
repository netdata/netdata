// SPDX-License-Identifier: GPL-3.0-or-later

package promprofilevalidation

import (
	"crypto/sha256"
	"fmt"
	"slices"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/framework/chartemit"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
)

func materializeCharts(plan chartengine.Plan) []materializedChart {
	byID := make(map[string]*materializedChart)
	for _, action := range plan.Actions {
		switch item := action.(type) {
		case chartengine.CreateChartAction:
			byID[item.ChartID] = &materializedChart{
				TemplateID:    item.ChartTemplateID,
				IDFingerprint: fingerprintID(item.ChartID),
				Context:       item.Meta.Context,
				Title:         item.Meta.Title,
				Family:        item.Meta.Family,
				Units:         item.Meta.Units,
				Algorithm:     string(item.Meta.Algorithm),
				Priority:      item.Meta.Priority,
				Autogen:       strings.HasPrefix(item.ChartTemplateID, "__autogen__:"),
			}
		case chartengine.CreateDimensionAction:
			if chart := byID[item.ChartID]; chart != nil {
				chart.DimensionFingerprints = append(chart.DimensionFingerprints, fingerprintID(item.Name))
			}
		}
	}
	out := make([]materializedChart, 0, len(byID))
	for _, chart := range byID {
		slices.Sort(chart.DimensionFingerprints)
		out = append(out, *chart)
	}
	return out
}

type emittedPlanInspection struct {
	plannedCharts       int
	emittedCharts       int
	plannedDimensions   int
	emittedDimensions   int
	emptyChartIDs       []string
	emptyContexts       []string
	chartCollisions     []wireChartCollisionReport
	contextCollisions   []wireContextCollisionReport
	dimensionCollisions []dimensionCollisionReport
}

func inspectEmittedPlan(plan chartengine.Plan, typeID, jobName string) (emittedPlanInspection, error) {
	var result emittedPlanInspection
	emissionPlan := chartengine.Plan{}
	for _, action := range plan.Actions {
		switch action.(type) {
		case chartengine.CreateChartAction:
			result.plannedCharts++
			emissionPlan.Actions = append(emissionPlan.Actions, action)
		case chartengine.CreateDimensionAction:
			result.plannedDimensions++
			emissionPlan.Actions = append(emissionPlan.Actions, action)
		}
	}

	inspection, err := chartemit.InspectPlan(emissionPlan, chartemit.EmitEnv{
		TypeID:      typeID,
		UpdateEvery: 1,
		Plugin:      "go.d.plugin",
		Module:      "prometheus",
		JobName:     jobName,
	})
	if err != nil {
		return result, safePublicEmitterError(err)
	}

	type dimensionCount struct {
		fingerprint string
		count       int
	}
	wireChartCounts := make(map[string]int)
	rawContextsByWire := make(map[string]map[string]struct{})
	perChart := make(map[string]map[string]*dimensionCount)
	for _, chart := range inspection.Charts {
		wireChartID := chart.WireTypeID + "." + chart.WireChartID
		result.emittedCharts++
		wireChartCounts[wireChartID]++
		chartFingerprint := fingerprintID(wireChartID)
		if perChart[chartFingerprint] == nil {
			perChart[chartFingerprint] = make(map[string]*dimensionCount)
		}
		if chart.WireChartID == "" {
			result.emptyChartIDs = append(result.emptyChartIDs, fingerprintID(chart.SourceChartID))
		}
		if chart.WireContext == "" {
			result.emptyContexts = append(result.emptyContexts, fingerprintID(chart.SourceChartID))
		}
		rawContexts := rawContextsByWire[chart.WireContext]
		if rawContexts == nil {
			rawContexts = make(map[string]struct{})
			rawContextsByWire[chart.WireContext] = rawContexts
		}
		rawContexts[chart.SourceContext] = struct{}{}
	}
	for _, dimension := range inspection.Dimensions {
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

	slices.Sort(result.emptyChartIDs)
	slices.Sort(result.emptyContexts)
	for wireID, count := range wireChartCounts {
		if count < 2 {
			continue
		}
		result.chartCollisions = append(result.chartCollisions, wireChartCollisionReport{
			WireIDFingerprint: fingerprintID(wireID),
			Occurrences:       count,
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
			})
		}
	}
	return result, nil
}

func collectorJobFullName(jobName string) string {
	cfg := confgroup.Config{}
	cfg.SetModule("prometheus").SetName(jobName)
	cfg.ApplyDefaults(confgroup.Default{})
	return cfg.FullName()
}

func safePublicEmitterError(err error) error {
	message := "public chart emitter rejected the plan"
	if strings.Contains(err.Error(), "type.id exceeds max length") {
		message = "public chart emitter rejected a type.id that exceeds the maximum length"
	}
	return fmt.Errorf("%s (error fingerprint %s)", message, fingerprintID(err.Error()))
}

func fingerprintID(value string) string {
	sum := sha256.Sum256([]byte(value))
	return "sha256:" + fmt.Sprintf("%x", sum[:8])
}
