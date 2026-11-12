// SPDX-License-Identifier: GPL-3.0-or-later

package funcapi

import (
	"fmt"
	"slices"
	"strconv"
)

const (
	FocusRoleRoots = "roots"
	FocusRoleDepth = "depth"
)

// FocusConfig declares aggregate-first graph controls without topology-name inference.
type FocusConfig struct {
	Role            string          `json:"role"`
	UnfocusedOption string          `json:"unfocused_option,omitempty"`
	UnlimitedOption string          `json:"unlimited_option,omitempty"`
	Bindings        *FocusBindings  `json:"bindings,omitempty"`
	Traversal       *FocusTraversal `json:"traversal,omitempty"`
}

// FocusBindings resolves choices against actor occurrences in the same response.
type FocusBindings struct {
	Table        string `json:"table"`
	OptionColumn string `json:"option_column"`
	ActorColumn  string `json:"actor_column"`
}

// FocusTraversal describes undirected depth and connecting-path selection.
// The complete traversal semantics are defined by the topology developer guide.
type FocusTraversal struct {
	ConnectorTypes       []string                   `json:"connector_types,omitempty"`
	BoundaryTypes        []string                   `json:"boundary_types,omitempty"`
	ConnectRoots         bool                       `json:"connect_roots"`
	PruneLeafTypes       []string                   `json:"prune_leaf_types,omitempty"`
	ProtectedMemberships []FocusProtectedMembership `json:"protected_memberships,omitempty"`
}

type FocusProtectedMembership struct {
	ActorType string `json:"actor_type"`
	LinkType  string `json:"link_type"`
}

func (f *FocusConfig) clone() *FocusConfig {
	copy := *f
	if f.Bindings != nil {
		bindings := *f.Bindings
		copy.Bindings = &bindings
	}
	if f.Traversal != nil {
		traversal := *f.Traversal
		traversal.ConnectorTypes = slices.Clone(traversal.ConnectorTypes)
		traversal.BoundaryTypes = slices.Clone(traversal.BoundaryTypes)
		traversal.PruneLeafTypes = slices.Clone(traversal.PruneLeafTypes)
		traversal.ProtectedMemberships = slices.Clone(traversal.ProtectedMemberships)
		copy.Traversal = &traversal
	}
	return &copy
}

// ValidateFocus checks declaration semantics; graph-bound references need the data payload.
func (p ParamConfig) ValidateFocus() error {
	f := p.Focus
	if f == nil {
		return nil
	}
	optionIDs := make(map[string]struct{}, len(p.Options))
	for _, option := range p.Options {
		if option.ID == "" {
			return fmt.Errorf("focus option ID is empty")
		}
		if _, exists := optionIDs[option.ID]; exists {
			return fmt.Errorf("duplicate focus option %q", option.ID)
		}
		optionIDs[option.ID] = struct{}{}
	}
	switch f.Role {
	case FocusRoleRoots:
		if p.Selection != ParamSelect && p.Selection != ParamMultiSelect {
			return fmt.Errorf("roots focus requires select or multiselect")
		}
		if f.UnfocusedOption == "" {
			return fmt.Errorf("roots focus requires an unfocused option")
		}
		if _, ok := optionIDs[f.UnfocusedOption]; !ok {
			return fmt.Errorf("unfocused option %q is not declared", f.UnfocusedOption)
		}
		// Match request resolution, including first-default precedence for select.
		if !slices.ContainsFunc(defaultOptions(p), func(option ParamOption) bool {
			return option.ID == f.UnfocusedOption
		}) {
			return fmt.Errorf("roots focus must default to its unfocused option")
		}
		if f.UnlimitedOption != "" || f.Bindings == nil || f.Traversal == nil {
			return fmt.Errorf("roots focus requires bindings and traversal, not unlimited_option")
		}
		if f.Bindings.Table == "" || f.Bindings.OptionColumn == "" || f.Bindings.ActorColumn == "" {
			return fmt.Errorf("focus bindings require table, option_column and actor_column")
		}
		if f.Bindings.OptionColumn == f.Bindings.ActorColumn {
			return fmt.Errorf("focus binding columns must be distinct")
		}
		return f.Traversal.validate()
	case FocusRoleDepth:
		if p.Selection != ParamSelect || f.UnlimitedOption == "" {
			return fmt.Errorf("depth focus requires single selection and an unlimited option")
		}
		if f.UnfocusedOption != "" || f.Bindings != nil || f.Traversal != nil {
			return fmt.Errorf("depth focus cannot declare roots bindings or traversal")
		}
		if _, ok := optionIDs[f.UnlimitedOption]; !ok {
			return fmt.Errorf("unlimited option %q is not declared", f.UnlimitedOption)
		}
		for id := range optionIDs {
			if id == f.UnlimitedOption {
				continue
			}
			depth, err := strconv.Atoi(id)
			if err != nil || depth < 0 {
				return fmt.Errorf("depth option %q must be a non-negative integer", id)
			}
		}
		return nil
	default:
		return fmt.Errorf("unknown focus role %q", f.Role)
	}
}

func (f FocusTraversal) validate() error {
	for name, values := range map[string][]string{
		"connector_types": f.ConnectorTypes, "boundary_types": f.BoundaryTypes, "prune_leaf_types": f.PruneLeafTypes,
	} {
		seen := make(map[string]struct{}, len(values))
		for _, value := range values {
			if value == "" {
				return fmt.Errorf("focus %s contains an empty type", name)
			}
			if _, exists := seen[value]; exists {
				return fmt.Errorf("focus %s contains duplicate type %q", name, value)
			}
			seen[value] = struct{}{}
		}
	}
	for _, boundary := range f.BoundaryTypes {
		if slices.Contains(f.ConnectorTypes, boundary) {
			return fmt.Errorf("focus type %q cannot be both connector and boundary", boundary)
		}
	}
	seen := make(map[FocusProtectedMembership]struct{}, len(f.ProtectedMemberships))
	for _, membership := range f.ProtectedMemberships {
		if membership.ActorType == "" || membership.LinkType == "" {
			return fmt.Errorf("protected focus membership requires actor_type and link_type")
		}
		if _, exists := seen[membership]; exists {
			return fmt.Errorf("duplicate protected focus membership")
		}
		seen[membership] = struct{}{}
	}
	return nil
}

func ValidateFocusParams(params []ParamConfig) error {
	roles := make(map[string]struct{})
	for _, param := range params {
		if err := param.ValidateFocus(); err != nil {
			return fmt.Errorf("parameter %q: %w", param.ID, err)
		}
		if param.Focus == nil {
			continue
		}
		if _, exists := roles[param.Focus.Role]; exists {
			return fmt.Errorf("multiple parameters declare focus role %q", param.Focus.Role)
		}
		roles[param.Focus.Role] = struct{}{}
	}
	if _, depth := roles[FocusRoleDepth]; depth {
		if _, roots := roles[FocusRoleRoots]; !roots {
			return fmt.Errorf("depth focus requires a roots parameter")
		}
	}
	return nil
}
