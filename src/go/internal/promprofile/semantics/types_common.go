// SPDX-License-Identifier: GPL-3.0-or-later

package promsemantics

import (
	"fmt"

	"github.com/netdata/netdata/go/plugins/internal/promprofile/yaml"
	"gopkg.in/yaml.v3"
)

type ConditionUse struct {
	Policy string
	Inline *EnvironmentCondition
}

func (c ConditionUse) IsZero() bool {
	return c.Policy == "" && c.Inline == nil
}

func (c *ConditionUse) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.ScalarNode:
		if node.Tag != "!!str" || node.Value == "" {
			return fmt.Errorf("condition policy reference must be a nonempty string")
		}
		c.Policy = node.Value
	case yaml.MappingNode:
		condition, err := promyaml.DecodeNode[EnvironmentCondition]("condition", node, "any")
		if err != nil {
			return err
		}
		c.Inline = &condition
	default:
		return fmt.Errorf("condition must be a policy reference or mapping")
	}
	return nil
}

type EnvironmentCondition struct {
	Any []EnvironmentClause `yaml:"any"`
}

type EnvironmentClause struct {
	All []EnvironmentPredicate `yaml:"all"`
}

type EnvironmentPredicate struct {
	Axis   string      `yaml:"axis"`
	Op     string      `yaml:"op"`
	Value  *AxisValue  `yaml:"value,omitempty"`
	Values []AxisValue `yaml:"values,omitempty"`
}

type AxisValue struct {
	String  *string
	Integer *int
	Strings []string
}

func (v *AxisValue) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.ScalarNode:
		switch node.Tag {
		case "!!str":
			value := node.Value
			v.String = &value
		case "!!int":
			var value int
			if err := node.Decode(&value); err != nil {
				return fmt.Errorf("axis integer: %w", err)
			}
			v.Integer = &value
		default:
			return fmt.Errorf("axis value must be a string or integer")
		}
	case yaml.SequenceNode:
		var values []string
		if err := node.Decode(&values); err != nil {
			return fmt.Errorf("axis set: %w", err)
		}
		v.Strings = values
	default:
		return fmt.Errorf("axis value must be a string, integer, or string sequence")
	}
	return nil
}

type LabelCondition struct {
	Any []LabelClause `yaml:"any"`
}

type LabelClause struct {
	All []LabelPredicate `yaml:"all"`
}

type LabelPredicate struct {
	Label  string   `yaml:"label"`
	Op     string   `yaml:"op"`
	Value  *string  `yaml:"value,omitempty"`
	Values []string `yaml:"values,omitempty"`
}

type SourceReference struct {
	Signal     string          `yaml:"signal"`
	Components []string        `yaml:"components"`
	Where      *LabelCondition `yaml:"where,omitempty"`
}

type PrometheusContract struct {
	Type           string `yaml:"type"`
	Shape          string `yaml:"shape"`
	Classification string `yaml:"classification,omitempty"`
}

type FamilySelector struct {
	Exact   string `yaml:"exact,omitempty"`
	Grammar string `yaml:"grammar,omitempty"`
	Form    string `yaml:"form,omitempty"`
}

type LabelPresence struct {
	Kind string
	When ConditionUse
}

func (p *LabelPresence) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.ScalarNode:
		if node.Tag != "!!str" {
			return fmt.Errorf("label presence must be required, present, optional, or a condition mapping")
		}
		p.Kind = node.Value
	case yaml.MappingNode:
		value, err := promyaml.DecodeNode[struct {
			When ConditionUse `yaml:"when"`
		}]("label presence", node, "when")
		if err != nil {
			return err
		}
		p.When = value.When
	default:
		return fmt.Errorf("label presence must be required, present, optional, or a condition mapping")
	}
	return nil
}

func (p LabelPresence) keyMayBeAbsent() bool {
	return p.Kind == "optional" || p.Kind == ""
}

func (p LabelPresence) keyIsAlwaysPresent() bool {
	return p.Kind == "required" || p.Kind == "present"
}

type LabelDomain struct {
	Kind   string   `yaml:"kind"`
	Values []string `yaml:"values,omitempty"`
}

type EndpointCardinality struct {
	Kind string `yaml:"kind"`
	Max  *int   `yaml:"max,omitempty"`
	Axis string `yaml:"axis,omitempty"`
}
