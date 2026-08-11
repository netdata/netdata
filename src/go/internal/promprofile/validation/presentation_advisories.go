// SPDX-License-Identifier: GPL-3.0-or-later

package promvalidation

import (
	"fmt"
	"maps"
	"math"
	"slices"
	"strings"

	metrixselector "github.com/netdata/netdata/go/plugins/pkg/metrix/selector"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
	commonmodel "github.com/prometheus/common/model"
)

type metricDeclaration struct {
	name string
	path string
	used bool
}

// addAuthoredProfileHeuristics checks source intent before collector merge or
// compiler defaulting can hide it. It reports objective presentation failures
// as errors and leaves judgment-dependent findings as warnings.
func addAuthoredProfileHeuristics(
	root charttpl.Group,
	rootPath string,
	rawFamilies []rawFamilyReport,
	r *Report,
) {
	familyTypes := observedDistributionTypes(rawFamilies)
	var declarations []*metricDeclaration
	var walk func(group charttpl.Group, path string, active map[string]*metricDeclaration)
	walk = func(group charttpl.Group, path string, active map[string]*metricDeclaration) {
		scoped := maps.Clone(active)
		for i, name := range group.Metrics {
			decl := &metricDeclaration{
				name: strings.TrimSpace(name),
				path: fmt.Sprintf("%s.metrics[%d]", path, i),
			}
			declarations = append(declarations, decl)
			scoped[decl.name] = decl
		}

		for i, chart := range group.Charts {
			chartPath := fmt.Sprintf("%s.charts[%d]", path, i)
			reviewAuthoredChart(chart, chartPath, scoped, familyTypes, r)
		}
		for i, child := range group.Groups {
			walk(
				child,
				fmt.Sprintf("%s.groups[%d](%s)", path, i, child.Family),
				scoped,
			)
		}
	}
	walk(root, rootPath, make(map[string]*metricDeclaration))

	for _, decl := range declarations {
		if decl.name == "" || decl.used {
			continue
		}
		r.addWarning(
			"unused_metric_declaration",
			decl.path,
			fmt.Sprintf("metric %q is declared but no authored dimension in its scope selects it", decl.name),
			"A metrics declaration only authorizes selector scope; it does not keep, drop, or chart data. Unused declarations obscure ownership and can leave stale denied families looking intentionally covered.",
		)
	}
}

func reviewAuthoredChart(
	chart charttpl.Chart,
	path string,
	active map[string]*metricDeclaration,
	familyTypes map[string]commonmodel.MetricType,
	r *Report,
) {
	hasBucket := false
	hasVisibleDimension := false
	for _, dimension := range chart.Dimensions {
		if dimension.Options == nil || !dimension.Options.Hidden {
			hasVisibleDimension = true
		}
		compiled, err := metrixselector.ParseCompiled(dimension.Selector)
		if err != nil {
			continue // Authoritative template/compiler validation reports errors.
		}
		for _, name := range compiled.Meta().MetricNames {
			if decl := active[name]; decl != nil {
				decl.used = true
			}
			if _, role, ok := distributionRole(name, familyTypes); ok && role == "bucket" {
				hasBucket = true
			}
		}
	}

	if !hasVisibleDimension {
		r.addError(
			"all_dimensions_hidden",
			path,
			fmt.Sprintf("chart %q hides every authored dimension", chart.Title),
			"A chart with no visible dimensions cannot answer an operator question. Keep at least one dimension visible; hidden dimensions may support a visible comparison but cannot replace it.",
		)
	}

	if hasBucket {
		if authoredType := strings.TrimSpace(chart.Type); authoredType != "" && !strings.EqualFold(authoredType, "heatmap") {
			r.addWarning(
				"histogram_type_runtime_override",
				path,
				fmt.Sprintf("chart %q selects histogram buckets but declares type %q", chart.Title, authoredType),
				"The compiler forces bucket charts to heatmap. Omit the derived type or declare a presentation that agrees with the runtime result.",
			)
		}
		if strings.TrimSpace(chart.Units) != "observations/s" {
			r.addError(
				"histogram_bucket_units",
				path,
				fmt.Sprintf("chart %q selects histogram buckets but declares units %q", chart.Title, chart.Units),
				"Metrix exposes non-overlapping bucket counters, so the heatmap intensity is an observation rate. Use units \"observations/s\"; the bucket boundaries already carry the observed value's unit.",
			)
		}
		if algorithm := strings.TrimSpace(chart.Algorithm); algorithm != "" && algorithm != "incremental" {
			r.addError(
				"histogram_bucket_algorithm",
				path,
				fmt.Sprintf("chart %q selects histogram buckets but declares algorithm %q", chart.Title, chart.Algorithm),
				"Histogram bucket values are counters after flattening and must render as change per second. Omit the algorithm to use the runtime counter kind or declare \"incremental\".",
			)
		}
	}

	chartType := strings.ToLower(strings.TrimSpace(chart.Type))
	switch chartType {
	case "area":
		r.addWarning(
			"area_presentation_review",
			path,
			fmt.Sprintf("chart %q explicitly uses area presentation with units %q", chart.Title, chart.Units),
			"Area requires deliberate filled-magnitude intent. Units can support that decision but cannot prove or reject it; record the operator meaning in the semantic design.",
		)
	case "stacked":
		r.addWarning(
			"stacked_presentation_review",
			path,
			fmt.Sprintf("chart %q explicitly uses stacked presentation with units %q", chart.Title, chart.Units),
			"Stacked requires dimensions that form an exact disjoint, exhaustive, additive partition for one operator question. Units alone cannot establish that relationship; prove it in the semantic design.",
		)
	}
}

func rateLikeUnits(units string) bool {
	units = strings.ToLower(strings.TrimSpace(units))
	return strings.Contains(units, "/s") ||
		strings.Contains(units, "/sec") ||
		strings.Contains(units, "per second")
}

// addIncrementalUnitHeuristics uses the compiler-resolved runtime algorithm,
// avoiding a second implementation of chartengine's selector-kind inference.
func addIncrementalUnitHeuristics(charts []materializedChart, owners *chartOwnershipIndex, r *Report) {
	type templateSummary struct {
		title     string
		units     string
		instances int
	}
	templates := make(map[string]*templateSummary)
	for _, chart := range charts {
		if chart.Autogen || !slices.Contains(chart.Algorithms, "incremental") {
			continue
		}
		item := templates[chart.TemplateID]
		if item == nil {
			item = &templateSummary{title: chart.Title, units: chart.Units}
			templates[chart.TemplateID] = item
		}
		item.instances++
	}
	for _, templateID := range slices.Sorted(maps.Keys(templates)) {
		item := templates[templateID]
		if rateLikeUnits(item.units) || incrementalRateEquivalentUnits(item.units) {
			continue
		}
		r.addWarning(
			"incremental_units_review",
			owners.templatePath(templateID, templateID),
			fmt.Sprintf(
				"incremental chart %q materializes %d instance(s) with non-rate units %q",
				item.title,
				item.instances,
				item.units,
			),
			"Netdata renders incremental values as per-second deltas. Use rate-bearing units that preserve the measured object, or document a truthful derived equivalent such as CPU cores or utilization; plain count/quantity units hide the temporal denominator.",
		)
	}
}

func incrementalRateEquivalentUnits(units string) bool {
	units = strings.ToLower(strings.TrimSpace(units))
	switch units {
	case "%", "percent", "percentage", "ratio", "utilization",
		"core", "cores", "concurrent", "concurrency", "in-flight", "in flight":
		return true
	default:
		return false
	}
}

func addObservedDistributionHeuristics(
	root charttpl.Group,
	rootPath string,
	rawFamilies []rawFamilyReport,
	r *Report,
) {
	familyTypes := observedDistributionTypes(rawFamilies)

	var walk func(group charttpl.Group, path string)
	walk = func(group charttpl.Group, path string) {
		for i, chart := range group.Charts {
			chartPath := fmt.Sprintf("%s.charts[%d]", path, i)
			rolesByFamily := make(map[string]map[string]struct{})
			for _, dimension := range chart.Dimensions {
				compiled, err := metrixselector.ParseCompiled(dimension.Selector)
				if err != nil {
					continue
				}
				for _, name := range compiled.Meta().MetricNames {
					family, role, ok := distributionRole(name, familyTypes)
					if !ok {
						continue
					}
					roles := rolesByFamily[family]
					if roles == nil {
						roles = make(map[string]struct{})
						rolesByFamily[family] = roles
					}
					roles[role] = struct{}{}
				}
			}
			for _, roles := range rolesByFamily {
				if len(roles) < 2 {
					continue
				}
				names := make([]string, 0, len(roles))
				for role := range roles {
					names = append(names, role)
				}
				slices.Sort(names)
				r.addWarning(
					"distribution_role_mixing",
					chartPath,
					fmt.Sprintf("chart %q mixes distribution roles %v from one source family", chart.Title, names),
					"Buckets/quantiles describe distribution shape, count describes observations, and sum carries observed units. One chart unit and axis cannot make those roles semantically interchangeable.",
				)
			}
		}
		for i, child := range group.Groups {
			walk(child, fmt.Sprintf("%s.groups[%d](%s)", path, i, child.Family))
		}
	}
	walk(root, rootPath)
}

func observedDistributionTypes(rawFamilies []rawFamilyReport) map[string]commonmodel.MetricType {
	familyTypes := make(map[string]commonmodel.MetricType)
	for _, family := range rawFamilies {
		typ := commonmodel.MetricType(family.Type)
		if typ == commonmodel.MetricTypeHistogram || typ == commonmodel.MetricTypeSummary {
			familyTypes[family.Name] = typ
		}
	}
	return familyTypes
}

func distributionRole(
	name string,
	familyTypes map[string]commonmodel.MetricType,
) (string, string, bool) {
	if base := strings.TrimSuffix(name, "_bucket"); base != name &&
		familyTypes[base] == commonmodel.MetricTypeHistogram {
		return base, "bucket", true
	}
	if base, ok := strings.CutSuffix(name, "_count"); ok {
		if typ := familyTypes[base]; typ == commonmodel.MetricTypeHistogram || typ == commonmodel.MetricTypeSummary {
			return base, "count", true
		}
	}
	if base, ok := strings.CutSuffix(name, "_sum"); ok {
		if typ := familyTypes[base]; typ == commonmodel.MetricTypeHistogram || typ == commonmodel.MetricTypeSummary {
			return base, "sum", true
		}
	}
	if familyTypes[name] == commonmodel.MetricTypeSummary {
		return name, "quantile", true
	}
	return "", "", false
}

type chartScaleMeta struct {
	title     string
	heatmap   bool
	dimension map[string]dimensionScale
}

type dimensionScale struct {
	multiplier int
	divisor    int
	hidden     bool
	algorithm  string
}

// addObservedScaleHeuristics uses the exact values already routed by the
// planner. It reports only a ratio and fingerprints the chart ID so observed
// label-derived identities and dynamic dimension names remain private.
func addObservedScaleHeuristics(plan chartengine.Plan, owners *chartOwnershipIndex, r *Report) {
	charts := make(map[string]*chartScaleMeta)
	for _, action := range plan.Actions {
		switch item := action.(type) {
		case chartengine.CreateChartAction:
			charts[item.ChartID] = &chartScaleMeta{
				title:     item.Meta.Title,
				heatmap:   string(item.Meta.Type) == "heatmap",
				dimension: make(map[string]dimensionScale),
			}
		case chartengine.CreateDimensionAction:
			meta := charts[item.ChartID]
			if meta == nil {
				continue
			}
			meta.dimension[item.Name] = dimensionScale{
				multiplier: item.Multiplier,
				divisor:    item.Divisor,
				hidden:     item.Hidden,
				algorithm:  string(item.Algorithm),
			}
		}
	}

	warned := make(map[string]struct{})
	for _, action := range plan.Actions {
		update, ok := action.(chartengine.UpdateChartAction)
		if !ok {
			continue
		}
		meta := charts[update.ChartID]
		if meta == nil || meta.heatmap {
			continue
		}

		var magnitudes []float64
		for _, value := range update.Values {
			if value.IsEmpty {
				continue
			}
			v := float64(value.Int64)
			if value.IsFloat {
				v = value.Float64
			}
			scale := meta.dimension[value.Name]
			if scale.hidden || scale.algorithm != "absolute" {
				continue
			}
			multiplier, divisor := scale.multiplier, scale.divisor
			if multiplier == 0 {
				multiplier = 1
			}
			if divisor == 0 {
				divisor = 1
			}
			v = math.Abs(v * float64(multiplier) / float64(divisor))
			if v > 0 && !math.IsInf(v, 0) && !math.IsNaN(v) {
				magnitudes = append(magnitudes, v)
			}
		}
		if len(magnitudes) < 2 {
			continue
		}
		minimum, maximum := slices.Min(magnitudes), slices.Max(magnitudes)
		ratio := maximum / minimum
		if ratio < 20 {
			continue
		}
		if _, ok := warned[update.ChartID]; ok {
			continue
		}
		warned[update.ChartID] = struct{}{}
		r.addWarning(
			"observed_scale_gap",
			owners.chartPath(update.ChartID, fingerprintID(update.ChartID)),
			fmt.Sprintf("chart %q has non-zero absolute dimensions differing by about %.0fx in the supplied dump", meta.title, ratio),
			"A shared axis can flatten the smaller signal. Split dimensions, normalize a meaningful ratio, or explain why the capacity/composition comparison remains useful.",
		)
	}
}
