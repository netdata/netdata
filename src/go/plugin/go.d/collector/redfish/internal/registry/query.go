// SPDX-License-Identifier: GPL-3.0-or-later

package registry

import "slices"

// MatchReadingType returns the exact standard type/unit normalization.
func MatchReadingType(sourceType, sourceUnits string) (ReadingTypeSpec, bool) {
	for _, item := range readingTypeSpecs {
		if item.SourceType != sourceType {
			continue
		}
		if slices.Contains(item.SourceUnits, sourceUnits) {
			result := item
			result.SourceUnits = append([]string(nil), item.SourceUnits...)
			return result, true
		}
	}
	return ReadingTypeSpec{}, false
}

// MatchFixedReadingFamily resolves the closed Sensor excerpt families which
// are not members of the standard ReadingType enumeration.
func MatchFixedReadingFamily(family, sourceType, sourceUnits string) (ReadingTypeSpec, bool) {
	units := map[string]string{
		"apparent_power":      "volt-amperes",
		"reactive_power":      "vars",
		"apparent_energy":     "joule-equivalent",
		"reactive_energy":     "joule-equivalent",
		"crest_factor":        "ratio",
		"power_factor":        "ratio",
		"phase_angle":         "degrees",
		"harmonic_distortion": "percentage",
		"rotational_speed":    "RPM",
		"stored_energy":       "watt-hours",
	}[family]
	if units == "" {
		return ReadingTypeSpec{}, false
	}
	scale := Identity
	if family == "apparent_energy" || family == "reactive_energy" {
		scale = Rational{Num: 3_600_000, Den: 1}
	}
	return ReadingTypeSpec{
		SourceType: sourceType, SourceUnits: []string{sourceUnits},
		Family: family, Units: units, Scale: scale,
	}, true
}

// MatchReadingSurface resolves an exact reading semantic. Common sensor
// contexts are selected only by their explicit semantic class; otherwise the
// closed Redfish-owned context is returned.
func MatchReadingSurface(family, basis, role, semanticClass string) (ReadingSurfaceSpec, bool) {
	contract := MustCompile()
	var generic ReadingSurfaceSpec
	for _, item := range contract.Readings {
		if item.Family != family || item.Basis != basis || item.Role != role {
			continue
		}
		if item.SemanticClass == semanticClass && semanticClass != "" {
			return item, true
		}
		if item.SemanticClass == "" {
			generic = item
		}
	}
	if generic.Metric != "" {
		return generic, true
	}
	return ReadingSurfaceSpec{}, false
}

// Histogram returns a copy of a fixed distribution contract.
func Histogram(id string) (HistogramSpec, bool) {
	for _, item := range histogramSpecs {
		if item.ID == id {
			result := cloneHistograms([]HistogramSpec{item})
			return result[0], true
		}
	}
	return HistogramSpec{}, false
}
