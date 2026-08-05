// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
)

type chartRef struct {
	groupPath  []int
	chartIndex int
	path       string
	family     string
	chart      charttpl.Chart
}

type chartRouteDiagnosticError struct {
	path string
	err  error
}

type unavailableInstanceIdentity struct {
	path          string
	chartTitle    string
	selector      string
	missingLabels []string
	series        int
}

type instanceIdentityPolicy struct {
	explicitKeys []string
	explicitSet  map[string]struct{}
	excludedKeys map[string]struct{}
	includeAll   bool
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
	var walk func(group charttpl.Group, indexes []int, path string, familyParts []string)
	walk = func(group charttpl.Group, indexes []int, path string, familyParts []string) {
		parts := appendFamilyPart(familyParts, group.Family)
		for i, chart := range group.Charts {
			refs = append(refs, chartRef{
				groupPath:  slices.Clone(indexes),
				chartIndex: i,
				path:       fmt.Sprintf("%s.charts[%d]", path, i),
				family:     composeDisplayedFamily(parts, chart.Family),
				chart:      chart,
			})
		}
		for i, child := range group.Groups {
			walk(
				child,
				append(slices.Clone(indexes), i),
				fmt.Sprintf("%s.groups[%d](%s)", path, i, child.Family),
				parts,
			)
		}
	}
	for i, group := range spec.Groups {
		walk(group, []int{i}, fmt.Sprintf("groups[%d](%s)", i, group.Family), nil)
	}
	return refs
}

func buildAuthoredMapping(refs []chartRef) []authoredChartMappingReport {
	mapping := make([]authoredChartMappingReport, 0, len(refs))
	for _, ref := range refs {
		item := authoredChartMappingReport{
			Path:             ref.path,
			DisplayedFamily:  ref.family,
			Title:            ref.chart.Title,
			Context:          ref.chart.Context,
			Units:            ref.chart.Units,
			Algorithm:        ref.chart.Algorithm,
			Type:             ref.chart.Type,
			Priority:         ref.chart.Priority,
			InstanceByLabels: make([]string, 0),
			Dimensions:       make([]authoredDimensionMappingReport, 0, len(ref.chart.Dimensions)),
		}
		if ref.chart.Instances != nil {
			item.InstanceByLabels = slices.Clone(ref.chart.Instances.ByLabels)
		}
		for _, dimension := range ref.chart.Dimensions {
			item.Dimensions = append(item.Dimensions, authoredDimensionMappingReport{
				Selector:      dimension.Selector,
				Name:          dimension.Name,
				NameFromLabel: dimension.NameFromLabel,
				Hidden:        dimension.Options != nil && dimension.Options.Hidden,
			})
		}
		mapping = append(mapping, item)
	}
	return mapping
}

func parseInstanceIdentityPolicy(instances *charttpl.Instances) instanceIdentityPolicy {
	policy := instanceIdentityPolicy{
		explicitSet:  make(map[string]struct{}),
		excludedKeys: make(map[string]struct{}),
	}
	if instances == nil {
		return policy
	}
	for _, labelSpec := range instances.ByLabels {
		labelSpec = strings.TrimSpace(labelSpec)
		switch {
		case labelSpec == "*":
			policy.includeAll = true
		case strings.HasPrefix(labelSpec, "!"):
			if label := strings.TrimSpace(strings.TrimPrefix(labelSpec, "!")); label != "" {
				policy.excludedKeys[label] = struct{}{}
			}
		}
	}
	for _, labelSpec := range instances.ByLabels {
		label := strings.TrimSpace(labelSpec)
		if label == "" || label == "*" || strings.HasPrefix(label, "!") {
			continue
		}
		if _, blocked := policy.excludedKeys[label]; blocked {
			continue
		}
		if _, ok := policy.explicitSet[label]; ok {
			continue
		}
		policy.explicitSet[label] = struct{}{}
		policy.explicitKeys = append(policy.explicitKeys, label)
	}
	return policy
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

// addDashboardHeuristics reports review prompts rather than release failures.
// The engine can render these designs, but the resulting section-wide filters
// may surprise an operator.
func addDashboardHeuristics(spec *charttpl.Spec, r *report) {
	if spec == nil {
		return
	}
	addDisplayedFamilyIdentityHeuristics(spec, r)
	addDuplicateSiblingFamilyWarnings(spec.Groups, "groups", r)

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

		addDuplicateSiblingFamilyWarnings(group.Groups, path+".groups", r)

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

// addDuplicateSiblingFamilyWarnings reports repeated non-empty sibling family
// names because they compose the same displayed navigation path. Repeating a
// path can be valid source organization, but it cannot communicate distinct
// semantic branches to the dashboard reader.
func addDuplicateSiblingFamilyWarnings(groups []charttpl.Group, path string, r *report) {
	positions := make(map[string][]int)
	for i, group := range groups {
		family := strings.TrimSpace(group.Family)
		if family == "" {
			continue
		}
		positions[family] = append(positions[family], i)
	}
	for _, family := range slices.Sorted(maps.Keys(positions)) {
		indexes := positions[family]
		if len(indexes) < 2 {
			continue
		}
		r.addWarning(
			"duplicate_sibling_family",
			path,
			fmt.Sprintf("sibling family %q is declared %d times at indexes %v", family, len(indexes), indexes),
			"Identical sibling names compose the same displayed NIDL path, so repetition cannot express separate operator concepts. Give distinct domain branches explicit names, nest them under their semantic owner, or explain why one displayed path is intentional.",
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
	policy := parseInstanceIdentityPolicy(instances)
	if policy.includeAll {
		out[wildcardChartIdentity] = struct{}{}
		return out
	}
	for _, label := range policy.explicitKeys {
		out[label] = struct{}{}
	}
	if len(out) == 0 {
		out[globalChartIdentity] = struct{}{}
	}
	return out
}
