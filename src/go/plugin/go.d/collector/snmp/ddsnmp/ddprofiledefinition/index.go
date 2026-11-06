// SPDX-License-Identifier: GPL-3.0-or-later

package ddprofiledefinition

import "maps"

// MetricIndexDecode describes the complete row INDEX layout and selects one component.
// Component numbers are one-based; an octet string without Length is length-prefixed.
type MetricIndexDecode struct {
	Components []MetricIndexComponent `yaml:"components" json:"components"`
	Component  uint                   `yaml:"component" json:"component"`
}

type MetricIndexComponent struct {
	Type   string `yaml:"type" json:"type"`
	Length *uint  `yaml:"length,omitempty" json:"length,omitempty"`
}

func (c MetricIndexComponent) Clone() MetricIndexComponent {
	if c.Length != nil {
		length := *c.Length
		c.Length = &length
	}
	return c
}

func (c *MetricIndexDecode) Clone() *MetricIndexDecode {
	if c == nil {
		return nil
	}
	return &MetricIndexDecode{Components: cloneSlice(c.Components), Component: c.Component}
}

// MetricLookupScope restricts a value-keyed join to matching row-index scopes.
type MetricLookupScope struct {
	SourceIndexTransform []MetricIndexTransform `yaml:"source_index_transform" json:"source_index_transform"`
	TargetIndexTransform []MetricIndexTransform `yaml:"target_index_transform" json:"target_index_transform"`
}

func (c *MetricLookupScope) Clone() *MetricLookupScope {
	if c == nil {
		return nil
	}
	return &MetricLookupScope{SourceIndexTransform: cloneSlice(c.SourceIndexTransform), TargetIndexTransform: cloneSlice(c.TargetIndexTransform)}
}

func (c MetricIndexTransform) Clone() MetricIndexTransform {
	c.Mapping = maps.Clone(c.Mapping)
	return c
}
