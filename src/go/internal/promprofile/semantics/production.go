// SPDX-License-Identifier: GPL-3.0-or-later

package promsemantics

import (
	"fmt"
	"slices"

	"github.com/netdata/netdata/go/plugins/internal/promprofile/replay"
	"github.com/netdata/netdata/go/plugins/pkg/matcher"
)

// ValidateProductionProfileHeader compares independent design identity with a
// decoded production profile before fixture replay.
func (c *CompiledSemanticContract) ValidateProductionProfileHeader(profile promreplay.SemanticProfile) error {
	if c == nil {
		return fmt.Errorf("production profile header: compiled semantic contract is nil")
	}
	if profile.Name != c.profile {
		return fmt.Errorf("production profile header: name got %q, want %q", profile.Name, c.profile)
	}
	if profile.Match != c.header.match {
		return fmt.Errorf("production profile %q match got %q, want %q", c.profile, profile.Match, c.header.match)
	}
	if profile.HasApp != c.header.hasApp || profile.App != c.header.app {
		return fmt.Errorf("production profile %q app got present=%t value=%q, want present=%t value=%q",
			c.profile, profile.HasApp, profile.App, c.header.hasApp, c.header.app)
	}
	if profile.ContextNamespace != c.header.namespace {
		return fmt.Errorf("production profile %q context namespace got %q, want %q",
			c.profile, profile.ContextNamespace, c.header.namespace)
	}
	if err := validateProductionAutogenSelector(profile); err != nil {
		return err
	}
	return c.validateProductionFallbacks(profile)
}

func validateProductionAutogenSelector(profile promreplay.SemanticProfile) error {
	if len(profile.AutogenSelectorAllow) != 0 {
		return fmt.Errorf("production profile %q must not declare autogen.selector.allow", profile.Name)
	}
	seen := make(map[string]struct{}, len(profile.AutogenSelectorDeny))
	for index, expression := range profile.AutogenSelectorDeny {
		family, exact := exactRouteMetricName(expression)
		if !exact {
			return fmt.Errorf(
				"production profile %q autogen.selector.deny[%d] %q must name one exact metric family",
				profile.Name, index, expression,
			)
		}
		if _, ok := seen[family]; ok {
			return fmt.Errorf("production profile %q autogen.selector.deny duplicates family %q", profile.Name, family)
		}
		seen[family] = struct{}{}
	}
	return nil
}

func (c *CompiledSemanticContract) validateProductionFallbacks(profile promreplay.SemanticProfile) error {
	want := make(map[string]string)
	for _, registration := range sortedMapKeys(c.fallbacks) {
		fallback := c.fallbacks[registration]
		for _, family := range fallback.exactFamilies {
			pattern := matcher.QuoteGlobLiteral(family)
			if previous, ok := want[pattern]; ok && previous != fallback.classification {
				return fmt.Errorf("semantic fallback family %q has classifications %q and %q",
					family, previous, fallback.classification)
			}
			want[pattern] = fallback.classification
		}
		if fallback.embedded != nil {
			form := fallback.embedded.form
			pattern := matcher.QuoteGlobLiteral(form.Prefix) + "?*" +
				matcher.QuoteGlobLiteral(form.Separator+form.Suffix)
			if previous, ok := want[pattern]; ok && previous != fallback.classification {
				return fmt.Errorf("semantic fallback grammar pattern %q has classifications %q and %q",
					pattern, previous, fallback.classification)
			}
			want[pattern] = fallback.classification
		}
	}

	got := make(map[string]string, len(profile.FallbackRules))
	for _, rule := range profile.FallbackRules {
		if previous, ok := got[rule.Pattern]; ok {
			return fmt.Errorf("production profile %q fallback pattern %q is duplicated with classifications %q and %q",
				c.profile, rule.Pattern, previous, rule.AssertedType)
		}
		got[rule.Pattern] = rule.AssertedType
	}
	missing := make([]string, 0)
	extra := make([]string, 0)
	for pattern, classification := range want {
		if actual, ok := got[pattern]; !ok {
			missing = append(missing, pattern+"="+classification)
		} else if actual != classification {
			return fmt.Errorf("production profile %q fallback pattern %q classifies as %q, want %q",
				c.profile, pattern, actual, classification)
		}
	}
	for pattern, classification := range got {
		if _, ok := want[pattern]; !ok {
			extra = append(extra, pattern+"="+classification)
		}
	}
	slices.Sort(missing)
	slices.Sort(extra)
	if len(missing) != 0 || len(extra) != 0 {
		return fmt.Errorf("production profile %q fallback mapping differs: missing=%v extra=%v",
			c.profile, missing, extra)
	}
	return nil
}
