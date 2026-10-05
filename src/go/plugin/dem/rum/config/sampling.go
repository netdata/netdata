// SPDX-License-Identifier: GPL-3.0-or-later

package config

// Investigate is investigate sampling.
type Investigate struct {
	// SampleRate is the share of measured sessions kept in full; 0 means 1.
	SampleRate float64 `yaml:"sample_rate,omitempty" json:"sample_rate"`
	// AlwaysKeep lists conditions that keep a session regardless of
	// SampleRate: "errors", "poor_vitals". Unset means both; an empty
	// list means none.
	AlwaysKeep []string `yaml:"always_keep,omitempty" json:"always_keep"`
}

// MarshalYAML preserves unset (default conditions) versus explicitly empty
// conditions; ordinary slice omitempty treats both as absent.
func (i Investigate) MarshalYAML() (any, error) {
	var keep *[]string
	if i.AlwaysKeep != nil {
		keep = &i.AlwaysKeep
	}
	return struct {
		SampleRate float64   `yaml:"sample_rate,omitempty"`
		AlwaysKeep *[]string `yaml:"always_keep,omitempty"`
	}{SampleRate: i.SampleRate, AlwaysKeep: keep}, nil
}

// Investigate always-keep conditions.
const (
	KeepErrors     = "errors"
	KeepPoorVitals = "poor_vitals"
)

// MeasureRate is the effective measure sampling rate.
func (s Site) MeasureRate() float64 {
	if s.MeasureSampleRate <= 0 {
		return 1
	}
	return s.MeasureSampleRate
}

// InvestigateRate is the effective investigate sampling rate.
func (s Site) InvestigateRate() float64 {
	if s.Investigate == nil || s.Investigate.SampleRate <= 0 {
		return 1
	}
	return s.Investigate.SampleRate
}

// KeepsErrors reports whether sessions with errors are always kept.
func (s Site) KeepsErrors() bool { return s.keeps(KeepErrors) }

// KeepsPoorVitals reports whether sessions with a poor vital are always kept.
func (s Site) KeepsPoorVitals() bool { return s.keeps(KeepPoorVitals) }

func (s Site) keeps(cond string) bool {
	if s.Investigate == nil || s.Investigate.AlwaysKeep == nil {
		return true
	}
	for _, c := range s.Investigate.AlwaysKeep {
		if c == cond {
			return true
		}
	}
	return false
}
