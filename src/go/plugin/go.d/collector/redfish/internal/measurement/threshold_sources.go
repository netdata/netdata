// SPDX-License-Identifier: GPL-3.0-or-later

package measurement

import "strings"

// A threshold source keeps its own units when a linked excerpt supplies the reading.
type thresholdSource struct {
	data       map[string]any
	sourceType string
	units      string
	legacy     bool
}

type threshold struct {
	upper        bool
	severity     int
	limit        float64
	dwellSeconds float64
	clearSeconds float64
	clearOffset  float64
}

var thresholdFields = []struct {
	modern, legacy string
	upper          bool
	severity       int
}{
	{"LowerCaution", "LowerThresholdNonCritical", false, 1},
	{"LowerCautionUser", "", false, 1},
	{"LowerCritical", "LowerThresholdCritical", false, 2},
	{"LowerCriticalUser", "", false, 2},
	{"LowerFatal", "LowerThresholdFatal", false, 2},
	{"UpperCaution", "UpperThresholdNonCritical", true, 1},
	{"UpperCautionUser", "", true, 1},
	{"UpperCritical", "UpperThresholdCritical", true, 2},
	{"UpperCriticalUser", "", true, 2},
	{"UpperFatal", "UpperThresholdFatal", true, 2},
}

func modernThresholdSource(data map[string]any, sourceType, units string) *thresholdSource {
	if _, present := data["Thresholds"]; !present {
		return nil
	}
	return &thresholdSource{
		data:       data,
		sourceType: sourceType,
		units:      units,
	}
}

func hasLegacyThresholds(data map[string]any) bool {
	for _, field := range thresholdFields {
		if field.legacy != "" {
			if _, present := data[field.legacy]; present {
				return true
			}
		}
	}
	return false
}

func legacyThresholdSource(node *Resource, sourceType, units string) *thresholdSource {
	if node.SourcePath != "Temperatures" && node.SourcePath != "Fans" && node.SourcePath != "Voltages" {
		return nil
	}
	if !hasLegacyThresholds(node.Data) {
		return nil
	}
	return &thresholdSource{
		data:       node.Data,
		sourceType: sourceType,
		units:      units,
		legacy:     true,
	}
}

// BMCs can supply null limit placeholders. They supply no threshold, never zero.
// Unknown settings on an actual numeric threshold instead make evaluation unavailable.
func (s *thresholdSource) thresholds(family string) ([]threshold, bool) {
	// Resolve the output family after adapters have selected it. For example,
	// stored energy remains watt-hours while cumulative energy becomes joules.
	spec, ok := readingType(s.sourceType, s.units, family)
	if !ok {
		return nil, false
	}
	scale := rationalMultiplier(spec.Scale)
	data := s.data
	if !s.legacy {
		raw := data["Thresholds"]
		if raw == nil {
			return nil, true
		}
		var ok bool
		data, ok = raw.(map[string]any)
		if !ok {
			return nil, false
		}
	}
	var result []threshold
	for _, field := range thresholdFields {
		key := field.modern
		if s.legacy {
			key = field.legacy
		}
		if key == "" || data[key] == nil {
			continue
		}
		config := threshold{
			upper:    field.upper,
			severity: field.severity,
		}
		value := data[key]
		var settings map[string]any
		if !s.legacy {
			var ok bool
			settings, ok = value.(map[string]any)
			if !ok {
				return nil, false
			}
			if settings["Activation"] == "Disabled" {
				continue
			}
			value = settings["Reading"]
			if value == nil {
				continue
			}
		}
		_, limit, ok := numericValue(value)
		if !ok {
			return nil, false
		}
		config.limit = limit * scale
		if !isFinite(config.limit) || (!s.legacy && !config.readSettings(settings, scale)) {
			return nil, false
		}
		result = append(result, config)
	}
	return result, true
}

func (t *threshold) readSettings(data map[string]any, scale float64) bool {
	if direction, present := data["Activation"]; present {
		want := "Decreasing"
		if t.upper {
			want = "Increasing"
		}
		// Either and opposite-direction crossings do not define a sustained bad side.
		if direction != want {
			return false
		}
	}
	var ok bool
	if t.dwellSeconds, ok = thresholdDuration(data, "DwellTime"); !ok {
		return false
	}
	if t.clearSeconds, ok = thresholdDuration(data, "HysteresisDuration"); !ok {
		return false
	}
	if offset, present := data["HysteresisReading"]; present {
		_, value, valid := numericValue(offset)
		if !valid {
			return false
		}
		t.clearOffset = value * scale
	}
	return isFinite(t.clearOffset) && isFinite(t.limit+t.clearOffset)
}

func thresholdDuration(data map[string]any, key string) (float64, bool) {
	value, present := data[key]
	if !present {
		return 0, true
	}
	text, ok := value.(string)
	if !ok {
		return 0, false
	}
	_, seconds, ok := numericSourceValue(text, algorithmDurationPercent)
	return seconds, ok && seconds >= 0
}

func (s *thresholdSource) enabled() bool {
	// Nullable availability fields do not invalidate an otherwise usable reading.
	if enabled := s.data["Enabled"]; enabled != nil && enabled != true {
		return false
	}
	state, present := valueAt(s.data, "Status.State")
	if !present || state == nil {
		return true
	}
	text, ok := state.(string)
	if !ok {
		return false
	}
	switch strings.TrimSpace(text) {
	case "Enabled", "StandbySpare", "Qualified", "Degraded":
		return true
	default:
		return false
	}
}
