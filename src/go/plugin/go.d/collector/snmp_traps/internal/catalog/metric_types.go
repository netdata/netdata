// SPDX-License-Identifier: GPL-3.0-or-later

package catalog

import (
	"fmt"

	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
)

const (
	MetricTypeCounter = "counter"
	MetricTypeSample  = "sample"
	MetricTypeState   = "state"

	MetricIdentitySource      = "source"
	MetricIdentitySourceLabel = "source_label"
	MetricIdentityListener    = "listener"

	MetricMissingDrop             = "drop"
	MetricMissingZero             = "zero"
	MetricMissingUnknownDimension = "unknown_dimension"
	MetricMissingError            = "error"

	DefaultMetricExpireAfterCycles = 60
	DefaultMetricChartMaxInstances = 2000
)

// MetricRule is one static profile metric definition.
type MetricRule struct {
	Name        string `yaml:"name"`
	Type        string `yaml:"type"`
	Enabled     *bool  `yaml:"enabled,omitempty"`
	OnTrap      string `yaml:"on_trap,omitempty"`
	ProblemTrap string `yaml:"problem_trap,omitempty"`
	ClearTrap   string `yaml:"clear_trap,omitempty"`

	Where MetricPredicates `yaml:"where,omitempty"`

	Identity MetricIdentity `yaml:"identity,omitempty"`
	Output   MetricOutput   `yaml:"output,omitempty"`
	State    MetricState    `yaml:"state,omitempty"`
	Scale    MetricScale    `yaml:"scale,omitempty"`

	Missing          string `yaml:"missing,omitempty"`
	ValueFromVarbind string `yaml:"value_from_varbind,omitempty"`

	SourceFile string `yaml:"-"`
}

// MetricIdentity defines the device and optional resource identity of a rule.
type MetricIdentity struct {
	Device   string          `yaml:"device,omitempty"`
	Resource *MetricResource `yaml:"resource,omitempty"`
}

// MetricResource defines a bounded resource identity within a trap source.
type MetricResource struct {
	Class          string `yaml:"class,omitempty"`
	KeyFromVarbind string `yaml:"key_from_varbind,omitempty"`
	MaxPerSource   int    `yaml:"max_per_source,omitempty"`
}

// MetricOutput defines the metric, dimension, and chart emitted by a rule.
type MetricOutput struct {
	Metric    string `yaml:"metric,omitempty"`
	Dimension string `yaml:"dimension,omitempty"`
	Chart     string `yaml:"chart,omitempty"`
}

// MetricScale applies an integer multiplier and divisor to sample values.
type MetricScale struct {
	Multiplier int `yaml:"multiplier,omitempty"`
	Divisor    int `yaml:"divisor,omitempty"`
}

// MetricState defines the transition predicates and values of a state rule.
type MetricState struct {
	SetWhen   *MetricPredicate `yaml:"set_when,omitempty"`
	ClearWhen *MetricPredicate `yaml:"clear_when,omitempty"`

	ProblemValue *float64 `yaml:"problem_value,omitempty"`
	ClearValue   float64  `yaml:"clear_value,omitempty"`
	TTL          string   `yaml:"ttl,omitempty"`
}

// MetricChart is one static chart definition from a profile.
type MetricChart struct {
	ID          string              `yaml:"id"`
	Title       string              `yaml:"title"`
	Family      string              `yaml:"family,omitempty"`
	Context     string              `yaml:"context"`
	Units       string              `yaml:"units"`
	Algorithm   string              `yaml:"algorithm,omitempty"`
	Type        string              `yaml:"type,omitempty"`
	Description string              `yaml:"description,omitempty"`
	Lifecycle   *charttpl.Lifecycle `yaml:"lifecycle,omitempty"`

	SourceFile string `yaml:"-"`
}

// MetricPredicates is an ordered AND-list of rule predicates.
type MetricPredicates []MetricPredicate

// MetricPredicate matches either a trap varbind or a synthetic trap field.
type MetricPredicate struct {
	Varbind     string `yaml:"varbind,omitempty"`
	Field       string `yaml:"field,omitempty"`
	Equals      any    `yaml:"equals,omitempty"`
	In          []any  `yaml:"in,omitempty"`
	Exists      *bool  `yaml:"exists,omitempty"`
	Absent      *bool  `yaml:"absent,omitempty"`
	GreaterThan any    `yaml:"greater_than,omitempty"`
	LessThan    any    `yaml:"less_than,omitempty"`
	Range       []any  `yaml:"range,omitempty"`
	Not         bool   `yaml:"not,omitempty"`
}

func (p *MetricPredicates) UnmarshalYAML(unmarshal func(any) error) error {
	var predicates []MetricPredicate
	if err := unmarshal(&predicates); err != nil {
		return err
	}
	*p = predicates
	return nil
}

func (p *MetricPredicate) UnmarshalYAML(unmarshal func(any) error) error {
	var raw map[any]any
	if err := unmarshal(&raw); err != nil {
		return err
	}
	pred, err := normalizeMetricPredicateMap(raw)
	if err != nil {
		return err
	}
	*p = pred
	return nil
}

func normalizeMetricPredicateMap(m map[any]any) (MetricPredicate, error) {
	pred := MetricPredicate{}
	var hasVarbind, hasField bool
	for rawKey, rawVal := range m {
		key, ok := rawKey.(string)
		if !ok {
			return pred, fmt.Errorf("predicate key %v is not a string", rawKey)
		}
		switch key {
		case "varbind":
			hasVarbind = true
			value, ok := rawVal.(string)
			if !ok {
				return pred, fmt.Errorf("varbind must be a string")
			}
			pred.Varbind = value
		case "field":
			hasField = true
			value, ok := rawVal.(string)
			if !ok {
				return pred, fmt.Errorf("field must be a string")
			}
			pred.Field = value
		case "equals":
			pred.Equals = rawVal
		case "in":
			values, ok := rawVal.([]any)
			if !ok {
				return pred, fmt.Errorf("in must be a list")
			}
			pred.In = append([]any(nil), values...)
		case "exists":
			b, ok := rawVal.(bool)
			if !ok {
				return pred, fmt.Errorf("exists must be boolean")
			}
			pred.Exists = &b
		case "absent":
			b, ok := rawVal.(bool)
			if !ok {
				return pred, fmt.Errorf("absent must be boolean")
			}
			pred.Absent = &b
		case "greater_than":
			pred.GreaterThan = rawVal
		case "less_than":
			pred.LessThan = rawVal
		case "range":
			values, ok := rawVal.([]any)
			if !ok || len(values) != 2 {
				return pred, fmt.Errorf("range must be a two-element list")
			}
			pred.Range = append([]any(nil), values...)
		case "not":
			b, ok := rawVal.(bool)
			if !ok {
				return pred, fmt.Errorf("not must be boolean")
			}
			pred.Not = b
		default:
			return pred, fmt.Errorf("unknown predicate key %q", key)
		}
	}
	if hasVarbind == hasField {
		return pred, fmt.Errorf("predicate requires exactly one of varbind or field")
	}
	if err := validateMetricPredicateSelector(pred); err != nil {
		return pred, err
	}
	return pred, nil
}

func validateMetricPredicateSelector(pred MetricPredicate) error {
	if (pred.Varbind == "") == (pred.Field == "") {
		return fmt.Errorf("predicate requires exactly one of varbind or field")
	}
	return nil
}

// Source returns the profile file that defined the rule.
func (r *MetricRule) Source() string {
	if r == nil {
		return ""
	}
	return r.SourceFile
}

// Disabled reports whether the profile explicitly disabled the rule.
func (r *MetricRule) Disabled() bool { return r != nil && r.Enabled != nil && !*r.Enabled }

// StateProblemValue returns the configured problem value or its canonical default.
func (r *MetricRule) StateProblemValue() float64 {
	if r != nil && r.State.ProblemValue != nil {
		return *r.State.ProblemValue
	}
	return 1
}

// StateClearValue returns the configured clear value.
func (r *MetricRule) StateClearValue() float64 {
	if r == nil {
		return 0
	}
	return r.State.ClearValue
}

// Apply applies the normalized scale to v.
func (s MetricScale) Apply(v float64) float64 {
	mul := s.Multiplier
	if mul == 0 {
		mul = 1
	}
	div := s.Divisor
	if div == 0 {
		div = 1
	}
	return v * float64(mul) / float64(div)
}

// Source returns the profile file that defined the chart.
func (c *MetricChart) Source() string {
	if c == nil {
		return ""
	}
	return c.SourceFile
}

type profileMetricRule = MetricRule
type profileMetricIdentity = MetricIdentity
type profileMetricResource = MetricResource
type profileMetricOutput = MetricOutput
type profileMetricScale = MetricScale
type profileMetricState = MetricState
type profileMetricChart = MetricChart
type profileMetricPredicates = MetricPredicates
type profileMetricPredicate = MetricPredicate
