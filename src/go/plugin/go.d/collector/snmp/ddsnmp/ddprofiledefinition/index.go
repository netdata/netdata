// SPDX-License-Identifier: GPL-3.0-or-later

package ddprofiledefinition

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
