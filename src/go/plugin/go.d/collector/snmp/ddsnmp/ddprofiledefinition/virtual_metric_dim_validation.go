// SPDX-License-Identifier: GPL-3.0-or-later

package ddprofiledefinition

import (
	"regexp"
	"slices"
	"strconv"
	"strings"
)

type virtualMetricDimSupportMode uint8

const (
	virtualMetricDimUnsupported virtualMetricDimSupportMode = iota
	virtualMetricDimKnown
	virtualMetricDimDynamic
)

type virtualMetricDimSupport struct {
	mode virtualMetricDimSupportMode
	dims map[string]bool
}

type virtualMetricSourceSpec struct {
	dimSupport virtualMetricDimSupport
}

var (
	virtualMetricI64MapPattern       = regexp.MustCompile(`i64map\b([^)]*)\)`)
	virtualMetricQuotedStringPattern = regexp.MustCompile(`"(?:[^"\\]|\\.)*"`)
)

func collectVirtualMetricSourceSpecs(metrics []MetricsConfig) map[string]map[string]virtualMetricSourceSpec {
	specs := make(map[string]map[string]virtualMetricSourceSpec)

	for _, metric := range metrics {
		switch {
		case metric.IsScalar():
			addVirtualMetricSourceSpec(specs, metric.Symbol.Name, "", buildVirtualMetricSourceSpec(metric.Symbol))
		case metric.IsColumn():
			for _, sym := range metric.Symbols {
				addVirtualMetricSourceSpec(specs, sym.Name, metric.Table.Name, buildVirtualMetricSourceSpec(sym))
			}
		}
	}

	return specs
}

func addVirtualMetricSourceSpec(specs map[string]map[string]virtualMetricSourceSpec, metricName, tableName string, spec virtualMetricSourceSpec) {
	tables, ok := specs[metricName]
	if !ok {
		tables = make(map[string]virtualMetricSourceSpec)
		specs[metricName] = tables
	}
	tables[tableName] = spec
}

func buildVirtualMetricSourceSpec(sym SymbolConfig) virtualMetricSourceSpec {
	return virtualMetricSourceSpec{
		dimSupport: mergeVirtualMetricDimSupport(
			buildMappingVirtualMetricDimSupport(sym.Mapping),
			buildTransformVirtualMetricDimSupport(sym.Transform),
		),
	}
}

func buildMappingVirtualMetricDimSupport(mapping MappingConfig) virtualMetricDimSupport {
	if !mapping.HasItems() {
		return virtualMetricDimSupport{mode: virtualMetricDimUnsupported}
	}

	if mapping.EffectiveMode() == MappingModeBitmask {
		dims := make(map[string]bool)
		for _, value := range mapping.Items {
			if value != "" {
				dims[value] = true
			}
		}
		if len(dims) == 0 {
			return virtualMetricDimSupport{mode: virtualMetricDimUnsupported}
		}
		return virtualMetricDimSupport{mode: virtualMetricDimKnown, dims: dims}
	}

	keysNumeric := true
	valuesNumeric := true
	for key, value := range mapping.Items {
		if !isIntegerString(key) {
			keysNumeric = false
		}
		if !isIntegerString(value) {
			valuesNumeric = false
		}
	}

	switch {
	case keysNumeric && valuesNumeric:
		return virtualMetricDimSupport{mode: virtualMetricDimUnsupported}
	case keysNumeric:
		dims := make(map[string]bool)
		for _, value := range mapping.Items {
			dims[value] = true
		}
		return virtualMetricDimSupport{mode: virtualMetricDimKnown, dims: dims}
	default:
		dims := make(map[string]bool)
		for key, value := range mapping.Items {
			if isIntegerString(value) {
				dims[key] = true
			}
		}
		if len(dims) == 0 {
			return virtualMetricDimSupport{mode: virtualMetricDimUnsupported}
		}
		return virtualMetricDimSupport{mode: virtualMetricDimKnown, dims: dims}
	}
}

func buildTransformVirtualMetricDimSupport(transform string) virtualMetricDimSupport {
	if transform == "" {
		return virtualMetricDimSupport{mode: virtualMetricDimUnsupported}
	}

	if strings.Contains(transform, "transformEntitySensorValue") {
		return virtualMetricDimSupport{mode: virtualMetricDimDynamic}
	}

	if !strings.Contains(transform, "setMultivalue") {
		return virtualMetricDimSupport{mode: virtualMetricDimUnsupported}
	}

	dims := extractTransformMultiValueDims(transform)
	if len(dims) == 0 {
		return virtualMetricDimSupport{mode: virtualMetricDimDynamic}
	}

	return virtualMetricDimSupport{mode: virtualMetricDimKnown, dims: dims}
}

func mergeVirtualMetricDimSupport(mappingSupport, transformSupport virtualMetricDimSupport) virtualMetricDimSupport {
	if transformSupport.mode == virtualMetricDimDynamic {
		return transformSupport
	}

	if transformSupport.mode == virtualMetricDimKnown {
		if mappingSupport.mode == virtualMetricDimKnown {
			dims := make(map[string]bool, len(mappingSupport.dims)+len(transformSupport.dims))
			for dim := range mappingSupport.dims {
				dims[dim] = true
			}
			for dim := range transformSupport.dims {
				dims[dim] = true
			}
			return virtualMetricDimSupport{mode: virtualMetricDimKnown, dims: dims}
		}
		return transformSupport
	}

	return mappingSupport
}

func extractTransformMultiValueDims(transform string) map[string]bool {
	dims := make(map[string]bool)

	for _, match := range virtualMetricI64MapPattern.FindAllStringSubmatch(transform, -1) {
		if len(match) < 2 {
			continue
		}

		for _, token := range virtualMetricQuotedStringPattern.FindAllString(match[1], -1) {
			dim, err := strconv.Unquote(token)
			if err != nil || dim == "" {
				continue
			}
			dims[dim] = true
		}
	}

	return dims
}

func virtualMetricSourceAvailableDims(spec virtualMetricSourceSpec) []string {
	dims := make([]string, 0, len(spec.dimSupport.dims))
	for dim := range spec.dimSupport.dims {
		dims = append(dims, dim)
	}
	slices.Sort(dims)
	return dims
}

func isIntegerString(value string) bool {
	_, err := strconv.ParseInt(value, 10, 64)
	return err == nil
}
