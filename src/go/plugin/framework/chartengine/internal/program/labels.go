// SPDX-License-Identifier: GPL-3.0-or-later

package program

import (
	"errors"
	"fmt"
	"strings"
)

// PromotionMode defines how non-identity chart labels are selected.
type PromotionMode string

const (
	PromotionModeAutoIntersection     PromotionMode = "auto_intersection"
	PromotionModeExplicitIntersection PromotionMode = "explicit_intersection"
)

// LabelPolicy describes chart-label promotion and conflict behavior in IR.
type LabelPolicy struct {
	Mode PromotionMode

	// PromoteKeys is explicit allowlist for PromotionModeExplicitIntersection.
	PromoteKeys []string

	// Exclusions contains compile-resolved label keys that must never be
	// promoted (e.g. selector-constrained keys, dynamic dimension-name keys).
	Exclusions LabelExclusions

	// Precedence controls deterministic same-key merge behavior.
	Precedence LabelPrecedence
}

// LabelExclusions stores precomputed exclusion sets used by chart labeling.
type LabelExclusions struct {
	SelectorConstrainedKeys []string
	DimensionKeyLabels      []string
}

// LabelPrecedence encodes the selected conflict policy.
type LabelPrecedence struct {
	SelectedOverInstance bool
	InstanceOverJob      bool
	ReservedImmutable    bool
}

// DefaultLabelPrecedence returns the phase-1 conflict policy baseline.
func DefaultLabelPrecedence() LabelPrecedence {
	return LabelPrecedence{
		SelectedOverInstance: true,
		InstanceOverJob:      true,
		ReservedImmutable:    true,
	}
}

func validateLabelPolicy(policy LabelPolicy) error {
	switch policy.Mode {
	case PromotionModeAutoIntersection, PromotionModeExplicitIntersection:
		return nil
	default:
		return fmt.Errorf("invalid promotion mode %q", policy.Mode)
	}
}

func validateInstanceLabelSelectors(selectors []InstanceLabelSelector) error {
	if len(selectors) == 0 {
		return nil
	}

	hasPositive := false
	var errs []error
	for i, selector := range selectors {
		switch {
		case selector.IncludeAll:
			if selector.Exclude || selector.Key != "" {
				errs = append(errs, fmt.Errorf("instance selector[%d]: include-all selector must not set exclude or key", i))
				continue
			}
			hasPositive = true
		case selector.Exclude:
			if selector.Key == "" {
				errs = append(errs, fmt.Errorf("instance selector[%d]: exclude selector key is required", i))
				continue
			}
			if strings.TrimSpace(selector.Key) != selector.Key {
				errs = append(errs, fmt.Errorf("instance selector[%d]: exclude selector key must be trimmed", i))
			}
		case selector.Key != "":
			if strings.TrimSpace(selector.Key) != selector.Key {
				errs = append(errs, fmt.Errorf("instance selector[%d]: selector key must be trimmed", i))
			}
			hasPositive = true
		default:
			errs = append(errs, fmt.Errorf("instance selector[%d]: selector is empty", i))
		}
	}

	if !hasPositive {
		errs = append(errs, fmt.Errorf("instance selectors must include at least one positive selector"))
	}
	return errors.Join(errs...)
}

// InstanceLabelSelector is a normalized token from instances.by_labels.
type InstanceLabelSelector struct {
	// IncludeAll corresponds to token "*".
	IncludeAll bool
	// Exclude corresponds to token "!<key>".
	Exclude bool
	// Key is used by explicit include or exclude token.
	Key string
}

func (p LabelPolicy) clone() LabelPolicy {
	out := p
	out.PromoteKeys = append([]string(nil), p.PromoteKeys...)
	out.Exclusions = p.Exclusions.clone()
	out.Precedence = p.Precedence
	return out
}

func (e LabelExclusions) clone() LabelExclusions {
	out := e
	out.SelectorConstrainedKeys = append([]string(nil), e.SelectorConstrainedKeys...)
	out.DimensionKeyLabels = append([]string(nil), e.DimensionKeyLabels...)
	return out
}

func (s InstanceLabelSelector) clone() InstanceLabelSelector {
	return s
}
