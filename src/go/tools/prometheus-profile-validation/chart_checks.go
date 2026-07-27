// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	metrixselector "github.com/netdata/netdata/go/plugins/pkg/metrix/selector"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
)

type chartRef struct {
	groupPath  []int
	chartIndex int
	path       string
	chart      charttpl.Chart
}

type isolatedChartError struct {
	path string
	err  error
}

const (
	globalChartIdentity   = "<global>"
	wildcardChartIdentity = "<wildcard>"
)

func enumerateChartRefs(spec *charttpl.Spec) []chartRef {
	if spec == nil {
		return nil
	}
	var refs []chartRef
	var walk func(group charttpl.Group, indexes []int, path string)
	walk = func(group charttpl.Group, indexes []int, path string) {
		for i, chart := range group.Charts {
			refs = append(refs, chartRef{
				groupPath:  slices.Clone(indexes),
				chartIndex: i,
				path:       fmt.Sprintf("%s.charts[%d]", path, i),
				chart:      chart,
			})
		}
		for i, child := range group.Groups {
			walk(
				child,
				append(slices.Clone(indexes), i),
				fmt.Sprintf("%s.groups[%d](%s)", path, i, child.Family),
			)
		}
	}
	for i, group := range spec.Groups {
		walk(group, []int{i}, fmt.Sprintf("groups[%d](%s)", i, group.Family))
	}
	return refs
}

func inspectChartsInIsolation(
	spec *charttpl.Spec,
	refs []chartRef,
	reader metrix.Reader,
) (
	[]deadChartReport,
	[]deadDimensionReport,
	[]dimensionMaterializationLossReport,
	[]collisionReport,
	[]instanceMaterializationLossReport,
	[]isolatedChartError,
) {
	renderedByID := make(map[string]map[string]struct{})
	var dead []deadChartReport
	var deadDimensions []deadDimensionReport
	var dimensionLosses []dimensionMaterializationLossReport
	var instanceLosses []instanceMaterializationLossReport
	var errs []isolatedChartError

	autogen := chartengine.AutogenPolicy{Enabled: false}
	for _, ref := range refs {
		isolated := isolateChartSpec(spec, ref)
		ids, plannedChartDimensions, err := planIsolatedSpec(&isolated, reader, autogen)
		if err != nil {
			errs = append(errs, isolatedChartError{path: ref.path, err: err})
			continue
		}

		if len(ids) == 0 {
			dead = append(dead, deadChartReport{
				Path:     ref.path,
				Title:    ref.chart.Title,
				Context:  ref.chart.Context,
				Priority: ref.chart.Priority,
			})
		} else {
			observedIdentities, err := countObservedInstanceIdentities(ref.chart, reader)
			if err != nil {
				errs = append(errs, isolatedChartError{
					path: ref.path,
					err:  fmt.Errorf("resolve observed instance identities: %w", err),
				})
			} else if observedIdentities > len(ids) {
				cause := "rendered_id_collapse"
				if ref.chart.Lifecycle != nil &&
					ref.chart.Lifecycle.MaxInstances > 0 &&
					observedIdentities > ref.chart.Lifecycle.MaxInstances {
					cause = "lifecycle_limit_or_rendered_id_collapse"
				}
				instanceLosses = append(instanceLosses, instanceMaterializationLossReport{
					Path:               ref.path,
					ObservedIdentities: observedIdentities,
					RenderedIDs:        len(ids),
					Cause:              cause,
				})
			}

			observedDimensions, err := countObservedChartDimensionIdentities(ref.chart, reader)
			if err != nil {
				errs = append(errs, isolatedChartError{
					path: ref.path,
					err:  fmt.Errorf("resolve observed chart dimension identities: %w", err),
				})
			} else if observedDimensions > plannedChartDimensions {
				cause := "planner_dimension_omission"
				if ref.chart.Lifecycle != nil &&
					ref.chart.Lifecycle.Dimensions != nil &&
					ref.chart.Lifecycle.Dimensions.MaxDims > 0 {
					cause = "lifecycle_dimension_limit_or_planner_collapse"
				}
				dimensionLosses = append(dimensionLosses, dimensionMaterializationLossReport{
					Path:               ref.path,
					ObservedDimensions: observedDimensions,
					PlannedDimensions:  plannedChartDimensions,
					Cause:              cause,
				})
			}
		}

		for id := range ids {
			owners := renderedByID[id]
			if owners == nil {
				owners = make(map[string]struct{})
				renderedByID[id] = owners
			}
			owners[ref.path] = struct{}{}
		}

		for dimensionIndex, dimension := range ref.chart.Dimensions {
			dimensionPath := fmt.Sprintf("%s.dimensions[%d]", ref.path, dimensionIndex)
			dimensionSpec := isolateDimensionSpec(spec, ref, dimensionIndex)
			_, plannedDimensions, err := planIsolatedSpec(&dimensionSpec, reader, autogen)
			if err != nil {
				errs = append(errs, isolatedChartError{path: dimensionPath, err: err})
				continue
			}
			if plannedDimensions == 0 {
				name := dimension.Name
				if name == "" {
					name = dimension.NameFromLabel
				}
				deadDimensions = append(deadDimensions, deadDimensionReport{
					Path:     dimensionPath,
					Selector: dimension.Selector,
					Name:     name,
				})
			}
		}
	}

	var collisions []collisionReport
	for id, ownerSet := range renderedByID {
		if len(ownerSet) < 2 {
			continue
		}
		owners := make([]string, 0, len(ownerSet))
		for owner := range ownerSet {
			owners = append(owners, owner)
		}
		slices.Sort(owners)
		collisions = append(collisions, collisionReport{
			RenderedIDFingerprint: fingerprintID(id),
			Charts:                owners,
		})
	}
	return dead, deadDimensions, dimensionLosses, collisions, instanceLosses, errs
}

func planIsolatedSpec(
	spec *charttpl.Spec,
	reader metrix.Reader,
	autogen chartengine.AutogenPolicy,
) (map[string]struct{}, int, error) {
	raw, err := spec.MarshalTemplate()
	if err != nil {
		return nil, 0, fmt.Errorf("marshal isolated template: %w", err)
	}
	engine, err := chartengine.New(
		chartengine.WithEnginePolicy(chartengine.EnginePolicy{Autogen: &autogen}),
		chartengine.WithRuntimeStore(nil),
	)
	if err != nil {
		return nil, 0, fmt.Errorf("initialize isolated engine: %w", err)
	}
	if err := engine.LoadYAML([]byte(raw), 1); err != nil {
		return nil, 0, fmt.Errorf("load isolated template: %w", err)
	}
	attempt, err := engine.PreparePlan(reader)
	if err != nil {
		return nil, 0, fmt.Errorf("plan isolated template: %w", err)
	}
	defer attempt.Abort()

	ids := make(map[string]struct{})
	dimensions := 0
	for _, action := range attempt.Plan().Actions {
		switch item := action.(type) {
		case chartengine.CreateChartAction:
			ids[item.ChartID] = struct{}{}
		case chartengine.CreateDimensionAction:
			dimensions++
		}
	}
	return ids, dimensions, nil
}

func isolateChartSpec(spec *charttpl.Spec, target chartRef) charttpl.Spec {
	out := *spec
	if spec.Engine != nil {
		engine := *spec.Engine
		if spec.Engine.Selector != nil {
			selector := *spec.Engine.Selector
			selector.Allow = slices.Clone(spec.Engine.Selector.Allow)
			selector.Deny = slices.Clone(spec.Engine.Selector.Deny)
			engine.Selector = &selector
		}
		if spec.Engine.Autogen != nil {
			autogen := *spec.Engine.Autogen
			engine.Autogen = &autogen
		}
		out.Engine = &engine
	}
	out.Groups = make([]charttpl.Group, len(spec.Groups))
	for i, group := range spec.Groups {
		out.Groups[i] = group.Clone()
		pruneCharts(&out.Groups[i], []int{i}, target)
	}
	return out
}

func isolateDimensionSpec(spec *charttpl.Spec, target chartRef, dimensionIndex int) charttpl.Spec {
	out := isolateChartSpec(spec, target)
	group := groupAtPath(&out, target.groupPath)
	if group == nil || len(group.Charts) != 1 {
		return out
	}
	chart := &group.Charts[0]
	if dimensionIndex < 0 || dimensionIndex >= len(chart.Dimensions) {
		chart.Dimensions = nil
		return out
	}
	chart.Dimensions = []charttpl.Dimension{chart.Dimensions[dimensionIndex]}
	return out
}

func groupAtPath(spec *charttpl.Spec, path []int) *charttpl.Group {
	if spec == nil || len(path) == 0 || path[0] < 0 || path[0] >= len(spec.Groups) {
		return nil
	}
	group := &spec.Groups[path[0]]
	for _, index := range path[1:] {
		if index < 0 || index >= len(group.Groups) {
			return nil
		}
		group = &group.Groups[index]
	}
	return group
}

func pruneCharts(group *charttpl.Group, path []int, target chartRef) {
	if slices.Equal(path, target.groupPath) {
		group.Charts = []charttpl.Chart{group.Charts[target.chartIndex]}
	} else {
		group.Charts = nil
	}
	for i := range group.Groups {
		pruneCharts(&group.Groups[i], append(slices.Clone(path), i), target)
	}
}

type observedDimension struct {
	spec     charttpl.Dimension
	selector metrixselector.Selector
}

func countObservedInstanceIdentities(chart charttpl.Chart, reader metrix.Reader) (int, error) {
	dimensions := make([]observedDimension, 0, len(chart.Dimensions))
	for i, dimension := range chart.Dimensions {
		selector, err := metrixselector.Parse(dimension.Selector)
		if err != nil {
			return 0, fmt.Errorf("dimension %d selector: %w", i, err)
		}
		dimensions = append(dimensions, observedDimension{spec: dimension, selector: selector})
	}

	identities := make(map[string]struct{})
	reader.ForEachSeriesIdentity(func(
		_ metrix.SeriesIdentity,
		meta metrix.SeriesMeta,
		name string,
		labels metrix.LabelView,
		_ metrix.SampleValue,
	) {
		for _, dimension := range dimensions {
			if !dimension.selector.Matches(name, labels) ||
				!dimensionNameMaterializes(dimension.spec, name, labels, meta) {
				continue
			}
			key, ok := rawInstanceIdentityKey(chart.Instances, labels)
			if ok {
				identities[key] = struct{}{}
			}
			break
		}
	})
	return len(identities), nil
}

func countObservedChartDimensionIdentities(chart charttpl.Chart, reader metrix.Reader) (int, error) {
	dimensions := make([]observedDimension, 0, len(chart.Dimensions))
	for i, dimension := range chart.Dimensions {
		selector, err := metrixselector.Parse(dimension.Selector)
		if err != nil {
			return 0, fmt.Errorf("dimension %d selector: %w", i, err)
		}
		dimensions = append(dimensions, observedDimension{spec: dimension, selector: selector})
	}

	identities := make(map[string]struct{})
	reader.ForEachSeriesIdentity(func(
		_ metrix.SeriesIdentity,
		meta metrix.SeriesMeta,
		metricName string,
		labels metrix.LabelView,
		_ metrix.SampleValue,
	) {
		instanceKey, ok := rawInstanceIdentityKey(chart.Instances, labels)
		if !ok {
			return
		}
		for dimensionIndex, dimension := range dimensions {
			if !dimension.selector.Matches(metricName, labels) {
				continue
			}
			dimensionName, ok := observedDimensionName(dimension.spec, metricName, labels, meta)
			if !ok {
				continue
			}
			key := fmt.Sprintf(
				"%d:%s=%d:%s@%d",
				len(instanceKey),
				instanceKey,
				len(dimensionName),
				dimensionName,
				dimensionIndex,
			)
			identities[key] = struct{}{}
		}
	})
	return len(identities), nil
}

func dimensionNameMaterializes(
	dimension charttpl.Dimension,
	metricName string,
	labels metrix.LabelView,
	meta metrix.SeriesMeta,
) bool {
	_, ok := observedDimensionName(dimension, metricName, labels, meta)
	return ok
}

func observedDimensionName(
	dimension charttpl.Dimension,
	metricName string,
	labels metrix.LabelView,
	meta metrix.SeriesMeta,
) (string, bool) {
	if strings.TrimSpace(dimension.Name) != "" {
		return dimension.Name, true
	}
	if key := strings.TrimSpace(dimension.NameFromLabel); key != "" {
		value, ok := labels.Get(key)
		return value, ok && strings.TrimSpace(value) != ""
	}

	key := ""
	switch meta.FlattenRole {
	case metrix.FlattenRoleHistogramBucket:
		key = metrix.HistogramBucketLabel
	case metrix.FlattenRoleSummaryQuantile:
		key = metrix.SummaryQuantileLabel
	case metrix.FlattenRoleStateSetState:
		key = metricName
	default:
		return "", false
	}
	value, ok := labels.Get(key)
	return value, ok && strings.TrimSpace(value) != ""
}

func rawInstanceIdentityKey(instances *charttpl.Instances, labels metrix.LabelView) (string, bool) {
	if instances == nil || len(instances.ByLabels) == 0 {
		return "global", true
	}

	excluded := make(map[string]struct{})
	includeAll := false
	for _, labelSpec := range instances.ByLabels {
		labelSpec = strings.TrimSpace(labelSpec)
		switch {
		case labelSpec == "*":
			includeAll = true
		case strings.HasPrefix(labelSpec, "!"):
			key := strings.TrimSpace(strings.TrimPrefix(labelSpec, "!"))
			if key != "" {
				excluded[key] = struct{}{}
			}
		}
	}

	var keys []string
	explicit := make(map[string]struct{})
	for _, labelSpec := range instances.ByLabels {
		key := strings.TrimSpace(labelSpec)
		if key == "" || key == "*" || strings.HasPrefix(key, "!") {
			continue
		}
		if _, blocked := excluded[key]; blocked {
			continue
		}
		if _, seen := explicit[key]; seen {
			continue
		}
		if _, ok := labels.Get(key); !ok {
			return "", false
		}
		explicit[key] = struct{}{}
		keys = append(keys, key)
	}

	if includeAll {
		var wildcard []string
		labels.Range(func(key, _ string) bool {
			if _, blocked := excluded[key]; blocked {
				return true
			}
			if _, seen := explicit[key]; seen {
				return true
			}
			wildcard = append(wildcard, key)
			return true
		})
		slices.Sort(wildcard)
		keys = append(keys, wildcard...)
	}

	var b strings.Builder
	for _, key := range keys {
		value, ok := labels.Get(key)
		if !ok {
			return "", false
		}
		fmt.Fprintf(&b, "%d:%s=%d:%s;", len(key), key, len(value), value)
	}
	return b.String(), true
}

// addDashboardHeuristics reports review prompts rather than release failures.
// The engine can render these designs, but the resulting section-wide filters
// may surprise an operator.
func addDashboardHeuristics(spec *charttpl.Spec, r *report) {
	if spec == nil {
		return
	}
	addDisplayedFamilyIdentityHeuristics(spec, r)

	var walk func(group charttpl.Group, path string, parentDefault *charttpl.Instances)
	walk = func(group charttpl.Group, path string, parentDefault *charttpl.Instances) {
		effectiveDefault := parentDefault
		if group.ChartDefaults != nil && group.ChartDefaults.Instances != nil {
			effectiveDefault = group.ChartDefaults.Instances
			if parentDefault != nil &&
				!identityRetainsParent(
					chartIdentityLabels(effectiveDefault),
					chartIdentityLabels(parentDefault),
				) {
				addParentIdentityLossWarning(
					r,
					path+".chart_defaults.instances.by_labels",
					"group identity",
					chartIdentityLabels(parentDefault),
					chartIdentityLabels(effectiveDefault),
				)
			}
		}

		for i, chart := range group.Charts {
			if chart.Instances != nil && slices.Contains(chart.Instances.ByLabels, "*") {
				r.addWarning(
					"wildcard_instance_identity",
					fmt.Sprintf("%s.charts[%d]", path, i),
					fmt.Sprintf("chart %q derives instance identity from every non-excluded label", chart.Title),
					"Future exporter labels can silently change chart identity and cardinality; prefer explicit entity labels unless the open-ended identity is intentional.",
				)
			}
			if effectiveDefault != nil &&
				!identityRetainsParent(
					chartIdentityLabels(chart.Instances),
					chartIdentityLabels(effectiveDefault),
				) {
				addParentIdentityLossWarning(
					r,
					fmt.Sprintf("%s.charts[%d].instances.by_labels", path, i),
					fmt.Sprintf("chart %q", chart.Title),
					chartIdentityLabels(effectiveDefault),
					chartIdentityLabels(chart.Instances),
				)
			}
		}

		if len(group.Groups) > 1 {
			type childIdentity struct {
				path   string
				labels map[string]struct{}
				charts int
			}
			children := make([]childIdentity, 0, len(group.Groups))
			for i, child := range group.Groups {
				childPath := fmt.Sprintf("%s.groups[%d](%s)", path, i, child.Family)
				labels, charts := subtreeCommonIdentity(child)
				children = append(children, childIdentity{path: childPath, labels: labels, charts: charts})
			}

			var common map[string]struct{}
			chartedChildren := 0
			detail := make([]string, 0, len(children))
			for _, child := range children {
				if child.charts == 0 {
					continue
				}
				chartedChildren++
				if common == nil {
					common = maps.Clone(child.labels)
				} else {
					for label := range common {
						if _, ok := child.labels[label]; !ok {
							delete(common, label)
						}
					}
				}
				names := make([]string, 0, len(child.labels))
				for label := range child.labels {
					names = append(names, label)
				}
				slices.Sort(names)
				if len(names) == 0 {
					names = append(names, "<none-shared>")
				}
				detail = append(detail, fmt.Sprintf("%s=%v", child.path, names))
			}
			if chartedChildren > 1 && len(common) == 0 {
				r.addWarning(
					"sibling_identity_mismatch",
					path,
					"sibling family subtrees share no common chart identity across all of their charts: "+strings.Join(detail, "; "),
					"Section-wide filtering works best when sibling families share the parent entity identity. Different identities can be correct for different entity levels, but the split should be deliberate.",
				)
			}
		}

		for i, child := range group.Groups {
			walk(child, fmt.Sprintf("%s.groups[%d](%s)", path, i, child.Family), effectiveDefault)
		}
	}
	for i, group := range spec.Groups {
		walk(group, fmt.Sprintf("groups[%d](%s)", i, group.Family), nil)
	}
}

type displayedFamilyIdentity struct {
	charts     int
	identities map[string]identitySetSummary
}

type identitySetSummary struct {
	labels []string
	charts int
}

// addDisplayedFamilyIdentityHeuristics checks the actual family path rendered
// by group and chart family composition. Charts in one displayed leaf share a
// filter scope, so different effective identities deserve explicit review.
func addDisplayedFamilyIdentityHeuristics(spec *charttpl.Spec, r *report) {
	families := make(map[string]*displayedFamilyIdentity)
	var walk func(group charttpl.Group, familyParts []string)
	walk = func(group charttpl.Group, familyParts []string) {
		parts := appendFamilyPart(familyParts, group.Family)
		for _, chart := range group.Charts {
			family := composeDisplayedFamily(parts, chart.Family)
			item := families[family]
			if item == nil {
				item = &displayedFamilyIdentity{identities: make(map[string]identitySetSummary)}
				families[family] = item
			}
			item.charts++
			labels := identityLabelNames(chartIdentityLabels(chart.Instances))
			key := strings.Join(labels, "\x00")
			summary := item.identities[key]
			if summary.labels == nil {
				summary.labels = labels
			}
			summary.charts++
			item.identities[key] = summary
		}
		for _, child := range group.Groups {
			walk(child, parts)
		}
	}
	for _, group := range spec.Groups {
		walk(group, nil)
	}

	for _, family := range slices.Sorted(maps.Keys(families)) {
		item := families[family]
		if len(item.identities) < 2 {
			continue
		}
		keys := slices.Sorted(maps.Keys(item.identities))
		detail := make([]string, 0, len(keys))
		for _, key := range keys {
			summary := item.identities[key]
			detail = append(detail, fmt.Sprintf("%v (%d charts)", summary.labels, summary.charts))
		}
		r.addWarning(
			"family_identity_mixed",
			family,
			fmt.Sprintf(
				"displayed family %q contains %d charts with different effective identities: %s",
				family,
				item.charts,
				strings.Join(detail, "; "),
			),
			"One displayed leaf should represent one entity type so its charts filter together. Move charts to explicit entity-level branches or use a common identity only when every selected series truly carries it.",
		)
	}
}

func appendFamilyPart(parts []string, part string) []string {
	out := slices.Clone(parts)
	if part = strings.TrimSpace(part); part != "" {
		out = append(out, part)
	}
	return out
}

func composeDisplayedFamily(parts []string, leaf string) string {
	out := appendFamilyPart(parts, leaf)
	return strings.Join(out, "/")
}

func addParentIdentityLossWarning(
	r *report,
	path string,
	subject string,
	parent map[string]struct{},
	child map[string]struct{},
) {
	r.addWarning(
		"identity_parent_labels_dropped",
		path,
		fmt.Sprintf(
			"%s identity %v does not retain parent identity %v",
			subject,
			identityLabelNames(child),
			identityLabelNames(parent),
		),
		"A narrower entity should retain its parent identity labels so parent-level filtering remains valid for descendants. Repeat inherited labels in an overriding by_labels list, or move the chart to an intentional entity-level sibling.",
	)
}

func identityRetainsParent(child, parent map[string]struct{}) bool {
	if _, ok := parent[globalChartIdentity]; ok {
		return true
	}
	if _, ok := parent[wildcardChartIdentity]; ok {
		return true // The effective parent label set cannot be inferred statically.
	}
	if _, ok := child[wildcardChartIdentity]; ok {
		return true // The wildcard may retain the parent; its own warning covers uncertainty.
	}
	if _, ok := child[globalChartIdentity]; ok {
		return false
	}
	for label := range parent {
		if _, ok := child[label]; !ok {
			return false
		}
	}
	return true
}

func identityLabelNames(identity map[string]struct{}) []string {
	return slices.Sorted(maps.Keys(identity))
}

func subtreeCommonIdentity(group charttpl.Group) (map[string]struct{}, int) {
	var common map[string]struct{}
	charts := 0
	var walk func(charttpl.Group)
	walk = func(current charttpl.Group) {
		for _, chart := range current.Charts {
			charts++
			labels := chartIdentityLabels(chart.Instances)
			if common == nil {
				common = labels
				continue
			}
			for label := range common {
				if _, ok := labels[label]; !ok {
					delete(common, label)
				}
			}
		}
		for _, child := range current.Groups {
			walk(child)
		}
	}
	walk(group)
	if common == nil {
		common = make(map[string]struct{})
	}
	return common, charts
}

func chartIdentityLabels(instances *charttpl.Instances) map[string]struct{} {
	out := make(map[string]struct{})
	if instances == nil || len(instances.ByLabels) == 0 {
		out[globalChartIdentity] = struct{}{}
		return out
	}
	excluded := make(map[string]struct{})
	wildcard := false
	for _, labelSpec := range instances.ByLabels {
		labelSpec = strings.TrimSpace(labelSpec)
		if labelSpec == "*" {
			wildcard = true
		} else if strings.HasPrefix(labelSpec, "!") {
			excluded[strings.TrimSpace(strings.TrimPrefix(labelSpec, "!"))] = struct{}{}
		}
	}
	if wildcard {
		out[wildcardChartIdentity] = struct{}{}
		return out
	}
	for _, labelSpec := range instances.ByLabels {
		labelSpec = strings.TrimSpace(labelSpec)
		if labelSpec == "" || labelSpec == "*" || strings.HasPrefix(labelSpec, "!") {
			continue
		}
		if _, ok := excluded[labelSpec]; !ok {
			out[labelSpec] = struct{}{}
		}
	}
	if len(out) == 0 {
		out[globalChartIdentity] = struct{}{}
	}
	return out
}
