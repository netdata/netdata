// SPDX-License-Identifier: GPL-3.0-or-later

package charttpl

import "gopkg.in/yaml.v2"

// MarshalYAML preserves nil versus explicit-empty label promotion. The yaml
// library otherwise applies slice-length omitempty semantics and would erase an
// explicit empty allowlist during a template round-trip.
func (d ChartDefaults) MarshalYAML() (any, error) {
	fields := yaml.MapSlice{}
	if d.LabelPromoted != nil {
		fields = append(fields, yaml.MapItem{Key: "label_promotion", Value: d.LabelPromoted})
	}
	if d.Instances != nil {
		fields = append(fields, yaml.MapItem{Key: "instances", Value: d.Instances})
	}
	return fields, nil
}

// MarshalYAML preserves nil versus explicit-empty label promotion while
// retaining the user-facing chart field order.
func (c Chart) MarshalYAML() (any, error) {
	fields := yaml.MapSlice{}
	appendField := func(key string, value any) {
		fields = append(fields, yaml.MapItem{Key: key, Value: value})
	}
	if c.ID != "" {
		appendField("id", c.ID)
	}
	appendField("title", c.Title)
	if c.Family != "" {
		appendField("family", c.Family)
	}
	appendField("context", c.Context)
	appendField("units", c.Units)
	if c.Algorithm != "" {
		appendField("algorithm", c.Algorithm)
	}
	if c.Aggregation != "" {
		appendField("aggregation", c.Aggregation)
	}
	if c.Type != "" {
		appendField("type", c.Type)
	}
	if c.Priority != 0 {
		appendField("priority", c.Priority)
	}
	if c.LabelPromoted != nil {
		appendField("label_promotion", c.LabelPromoted)
	}
	if c.Instances != nil {
		appendField("instances", c.Instances)
	}
	if c.Lifecycle != nil {
		appendField("lifecycle", c.Lifecycle)
	}
	appendField("dimensions", c.Dimensions)
	return fields, nil
}

// MarshalTemplate validates the spec and serializes it with the same yaml
// library the decoder uses, so the emitted template round-trips through
// DecodeYAML unchanged. It runs Validate only — it does NOT apply the
// decode-time defaults — so a field left unset (for example a chart omitting
// type) is emitted unset, exactly as the caller assembled it.
func (s Spec) MarshalTemplate() (string, error) {
	if err := s.Validate(); err != nil {
		return "", err
	}
	data, err := yaml.Marshal(s)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
