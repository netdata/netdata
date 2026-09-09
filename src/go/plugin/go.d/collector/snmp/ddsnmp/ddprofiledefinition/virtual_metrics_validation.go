// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ddprofiledefinition

import (
	"errors"
	"fmt"
	"strings"
)

func validateEnrichVirtualMetrics(
	metrics []MetricsConfig,
	topology []TopologyConfig,
	vmetrics []VirtualMetricConfig,
) error {
	var errs []error

	metricSources := collectVirtualMetricSourceSpecs(metrics)
	topologySources := collectTopologyMetricSourceNames(topology)

	seenNames := make(map[string]int)

	for i := range vmetrics {
		vm := &vmetrics[i]

		if vm.Name == "" {
			errs = append(errs, fmt.Errorf("virtual_metrics[%d]: missing name", i))
		} else {
			if firstIdx, ok := seenNames[vm.Name]; ok {
				errs = append(errs, fmt.Errorf("virtual_metrics[%d]: duplicate name %q (first occurrence at index %d)", i, vm.Name, firstIdx))
			} else {
				seenNames[vm.Name] = i
			}
			if _, ok := metricSources[vm.Name]; ok {
				errs = append(errs, fmt.Errorf("virtual_metrics[%d]: name %q conflicts with an existing metric", i, vm.Name))
			}
		}

		for j, label := range vm.GroupBy {
			if label == "" {
				errs = append(errs, fmt.Errorf("virtual_metrics[%d].group_by[%d]: label cannot be empty", i, j))
			}
		}

		for j, emitTag := range vm.EmitTags {
			if emitTag.Tag == "" {
				errs = append(errs, fmt.Errorf("virtual_metrics[%d].emit_tags[%d]: missing tag", i, j))
			}
			if emitTag.From == "" {
				errs = append(errs, fmt.Errorf("virtual_metrics[%d].emit_tags[%d]: missing from", i, j))
			}
		}

		grouped := vm.PerRow || len(vm.GroupBy) > 0

		switch {
		case len(vm.Sources) == 0 && len(vm.Alternatives) == 0:
			errs = append(errs, fmt.Errorf("virtual_metrics[%d]: must define sources or alternatives", i))
		case len(vm.Alternatives) == 0:
			errs = append(
				errs,
				validateVirtualMetricSourcesNotTopology(
					fmt.Sprintf("virtual_metrics[%d].sources", i),
					vm.Sources,
					topologySources,
				),
			)
			errs = append(
				errs,
				validateVirtualMetricSources(
					fmt.Sprintf("virtual_metrics[%d].sources", i),
					vm.Sources,
					metricSources,
					grouped,
				),
			)
		default:
			for j, alt := range vm.Alternatives {
				if len(alt.Sources) == 0 {
					errs = append(errs, fmt.Errorf("virtual_metrics[%d].alternatives[%d]: must define sources", i, j))
					continue
				}
				errs = append(
					errs,
					validateVirtualMetricSourcesNotTopology(
						fmt.Sprintf("virtual_metrics[%d].alternatives[%d].sources", i, j),
						alt.Sources,
						topologySources,
					),
				)
				errs = append(
					errs,
					validateVirtualMetricSources(
						fmt.Sprintf("virtual_metrics[%d].alternatives[%d].sources", i, j),
						alt.Sources,
						metricSources,
						grouped,
					),
				)
			}
		}
	}

	return errors.Join(errs...)
}

func collectTopologyMetricSourceNames(topology []TopologyConfig) map[string]struct{} {
	names := make(map[string]struct{})
	for _, topo := range topology {
		metric := &topo.MetricsConfig
		switch {
		case metric.IsScalar():
			names[metric.Symbol.Name] = struct{}{}
		case metric.IsColumn():
			for _, sym := range metric.Symbols {
				names[sym.Name] = struct{}{}
			}
		}
	}
	return names
}

func validateVirtualMetricSourcesNotTopology(
	path string,
	sources []VirtualMetricSourceConfig,
	topologySources map[string]struct{},
) error {
	var errs []error
	for i, src := range sources {
		if _, ok := topologySources[src.Metric]; ok {
			errs = append(
				errs,
				fmt.Errorf("%s[%d]: topology metric source %q cannot be used by virtual_metrics", path, i, src.Metric),
			)
		}
	}
	return errors.Join(errs...)
}

func validateVirtualMetricSources(
	path string,
	sources []VirtualMetricSourceConfig,
	metricSources map[string]map[string]virtualMetricSourceSpec,
	grouped bool,
) error {
	var errs []error

	var groupTable string
	for i, src := range sources {
		if src.Metric == "" {
			errs = append(errs, fmt.Errorf("%s[%d]: missing metric", path, i))
		}
		if grouped && src.Table == "" {
			errs = append(errs, fmt.Errorf("%s[%d]: missing table", path, i))
		}

		if src.Metric != "" {
			tables, ok := metricSources[src.Metric]
			switch {
			case !ok:
				errs = append(errs, fmt.Errorf("%s[%d]: unknown metric source %q", path, i, src.Metric))
			case src.Table == "":
				if _, ok := tables[""]; !ok {
					errs = append(
						errs,
						fmt.Errorf("%s[%d]: missing table for non-scalar source %q", path, i, src.Metric),
					)
				}
			default:
				if _, ok := tables[src.Table]; !ok {
					errs = append(
						errs,
						fmt.Errorf("%s[%d]: unknown metric/table source %q/%q", path, i, src.Metric, src.Table),
					)
				}
			}
		}

		if src.Dim != "" && src.Metric != "" {
			tables, ok := metricSources[src.Metric]
			if !ok {
				continue
			}

			spec, ok := tables[src.Table]
			if !ok && src.Table == "" {
				spec, ok = tables[""]
			}
			if !ok {
				continue
			}

			switch spec.dimSupport.mode {
			case virtualMetricDimUnsupported:
				errs = append(
					errs,
					fmt.Errorf(
						"%s[%d]: dim %q requires a MultiValue source metric (%s)",
						path,
						i,
						src.Dim,
						formatVirtualMetricSourceRef(src),
					),
				)
			case virtualMetricDimKnown:
				if !spec.dimSupport.dims[src.Dim] {
					errs = append(
						errs,
						fmt.Errorf(
							"%s[%d]: dim %q is not available on %s (available: %s)",
							path,
							i,
							src.Dim,
							formatVirtualMetricSourceRef(src),
							strings.Join(virtualMetricSourceAvailableDims(spec), ", "),
						),
					)
				}
			}
		}

		if grouped && src.Table != "" {
			if groupTable == "" {
				groupTable = src.Table
			} else if src.Table != groupTable {
				errs = append(errs, fmt.Errorf("%s[%d]: grouped virtual metrics require all sources to use the same table (saw %q and %q)", path, i, groupTable, src.Table))
			}
		}
	}

	return errors.Join(errs...)
}

func formatVirtualMetricSourceRef(src VirtualMetricSourceConfig) string {
	if src.Table == "" {
		return fmt.Sprintf("metric %q", src.Metric)
	}
	return fmt.Sprintf("metric/table %q/%q", src.Metric, src.Table)
}
