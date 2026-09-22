// SPDX-License-Identifier: GPL-3.0-or-later

package chartengine

import (
	"testing"

	metrixselector "github.com/netdata/netdata/go/plugins/pkg/metrix/selector"
	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func nativeEntry(id, chartID, metric string) TemplateEntry {
	return TemplateEntry{
		ID:               id,
		ContextNamespace: "test",
		Groups:           []charttpl.Group{{Family: "Test", Metrics: []string{metric}, Charts: []charttpl.Chart{{ID: chartID, Title: "Requests", Context: chartID, Units: "requests", Dimensions: []charttpl.Dimension{{Selector: metric, Name: "value"}}}}}},
	}
}

func TestTemplateSetOwnsNormalizedInput(t *testing.T) {
	entry := nativeEntry("profile", "requests", "requests")
	entry.Groups[0].ChartDefaults = &charttpl.ChartDefaults{
		Priority: 42,
		Instances: &charttpl.Instances{
			ByLabels: []string{"node"},
		},
	}
	entry.AutogenRules = []charttpl.EngineAutogenRule{{Scope: "private_*", Selector: metrixselector.Expr{
		Deny: []string{"*"},
	}}}
	set, err := NewTemplateSet(TemplateSetSpec{
		Entries: []TemplateEntry{entry},
	})
	require.NoError(t, err)
	entries := set.Entries()
	require.Equal(t, 42, entries[0].Groups[0].Charts[0].Priority)
	assert.Equal(t, "line", entries[0].Groups[0].Charts[0].Type)
	assert.Nil(t, entries[0].Groups[0].ChartDefaults)
	assert.Empty(t, entry.Groups[0].Charts[0].Type)
	entry.Groups[0].ChartDefaults.Instances.ByLabels[0] = "changed"
	entry.Groups[0].Charts[0].Dimensions[0].Selector = "changed"
	entry.AutogenRules[0].Selector.Deny[0] = "changed"
	entries[0].Groups[0].Charts[0].Instances.ByLabels[0] = "also_changed"
	entries[0].AutogenRules[0].Selector.Deny[0] = "also_changed"
	got := set.Entries()[0]
	assert.Equal(t, []string{"node"}, got.Groups[0].Charts[0].Instances.ByLabels)
	assert.Equal(t, "requests", got.Groups[0].Charts[0].Dimensions[0].Selector)
	assert.Equal(t, []string{"*"}, got.AutogenRules[0].Selector.Deny)
	require.Len(t, set.policy.autogenRules, 1)
	assert.False(t, set.policy.autogenRules[0].Selects("private_value", labelSliceView{}))
	rebuilt, err := NewTemplateSet(TemplateSetSpec{
		Entries: set.Entries(),
	})
	require.NoError(t, err)
	assert.True(t, rebuilt.preservesEntry(set, "profile"))
}

func TestTemplateSetValidation(t *testing.T) {
	valid := nativeEntry("one", "shared", "requests")
	for name, entries := range map[string][]TemplateEntry{
		"empty":                 nil,
		"empty entry":           {{ID: "one"}},
		"missing ID":            {nativeEntry("", "shared", "requests")},
		"duplicate ID":          {valid, valid},
		"overlapping public ID": {valid, nativeEntry("two", "shared", "requests")},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := NewTemplateSet(TemplateSetSpec{
				Entries: entries,
			})
			if name == "empty" || name == "overlapping public ID" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
	_, err := NewTemplateSetYAML([]byte("version: v1\ngroups: []"))
	require.Error(t, err, "legacy documents still require groups")
	_, err = PrepareTemplateSet(nil)
	require.Error(t, err)
}

func TestTemplateSetLegacyCompilerParity(t *testing.T) {
	data := []byte(validTemplateYAML())
	set, err := NewTemplateSetYAML(data)
	require.NoError(t, err)
	spec, err := charttpl.DecodeYAML(data)
	require.NoError(t, err)
	compiled, err := Compile(spec, 0)
	require.NoError(t, err)
	assert.Equal(t, compiled.Charts(), set.program.Charts())
	assert.Equal(t, compiled.MetricNames(), set.program.MetricNames())
}

func TestTemplateSetIdentityAndLayout(t *testing.T) {
	a, b := nativeEntry("z", "shared", "requests"), nativeEntry("a", "shared", "requests")
	first, err := NewTemplateSet(TemplateSetSpec{
		Entries: []TemplateEntry{a, b},
	})
	require.NoError(t, err)
	reordered, err := NewTemplateSet(TemplateSetSpec{
		Entries: []TemplateEntry{b, a},
	})
	require.NoError(t, err)
	assert.True(t, reordered.preservesEntry(first, "z"))
	assert.True(t, reordered.preservesEntry(first, "a"))
	x, y := first.program.Charts()[0], reordered.program.Charts()[1]
	assert.Equal(t, x.TemplateID, y.TemplateID)
	assert.Equal(t, "g0.c0", x.LocalTemplateID)
	assert.Equal(t, "g0.c0", x.RoutingOrder)
	assert.Equal(t, "g1.c0", y.RoutingOrder)
	assert.Equal(t, "g10.2.c1", templateRoutingOrder("g1.2.c1", 9))
}

func TestTemplateSetPolicyBinding(t *testing.T) {
	entry := nativeEntry("profile", "requests", "requests")
	entry.AutogenRules = []charttpl.EngineAutogenRule{{Scope: "private_*", Selector: metrixselector.Expr{
		Deny: []string{"*"},
	}}}
	globalRules := []AutogenRule{{Scope: "global_*", Selector: metrixselector.Expr{
		Deny: []string{"*"},
	}}}
	set, err := NewTemplateSet(TemplateSetSpec{
		Entries: []TemplateEntry{entry},
		Policy: EnginePolicy{
			Autogen: &AutogenPolicy{
				Enabled: true,
				Rules:   globalRules,
			},
		},
	})
	require.NoError(t, err)
	override := WithEnginePolicy(EnginePolicy{
		Autogen: &AutogenPolicy{
			Enabled: true,
		},
	})
	bound, err := PrepareTemplateSet(set, override)
	require.NoError(t, err)
	require.Len(t, bound.policy.autogenRules, 1)
	assert.True(t, bound.policy.autogenRules[0].ScopeMatches("private_value"))
	assert.False(t, bound.policy.autogenRules[0].ScopeMatches("global_value"))
	require.Len(t, set.policy.autogenRules, 2, "binding leaves the source immutable")
	assert.Same(t, set.program, bound.program)
	entry.AutogenRules = nil
	changed, err := NewTemplateSet(TemplateSetSpec{
		Entries: []TemplateEntry{entry},
		Policy: EnginePolicy{
			Autogen: &AutogenPolicy{
				Enabled: false,
			},
		},
	})
	require.NoError(t, err)
	changed, err = PrepareTemplateSet(changed, override)
	require.NoError(t, err)
	assert.True(t, bound.SameGlobalPolicy(changed), "entry rules and masked global changes are dynamic")
	assert.False(t, changed.preservesEntry(bound, "profile"), "entry rules participate in replacement")

	document, err := NewTemplateSetYAML([]byte("version: v1\nengine:\n  autogen:\n    enabled: true\n    rules:\n      - scope: '*'\n        selector:\n          deny: ['*']\ngroups:\n  - family: Empty\n"))
	require.NoError(t, err)
	document, err = PrepareTemplateSet(document, override)
	require.NoError(t, err)
	assert.Empty(t, document.policy.autogenRules, "global override replaces document rules")
}

func TestTemplateSetGlobalNormalization(t *testing.T) {
	a, err := NewTemplateSet(TemplateSetSpec{})
	require.NoError(t, err)
	b, err := NewTemplateSet(TemplateSetSpec{
		Policy: EnginePolicy{
			Autogen:  &AutogenPolicy{},
			Selector: &metrixselector.Expr{},
		},
	})
	require.NoError(t, err)
	assert.True(t, a.SameGlobalPolicy(b))
	c, err := NewTemplateSet(TemplateSetSpec{
		FallbackContextNamespace: "different",
	})
	require.NoError(t, err)
	assert.False(t, a.SameGlobalPolicy(c))
}

func TestTemplateSetCompilerNormalizedEquality(t *testing.T) {
	entry := nativeEntry("profile", "requests", "requests")
	original, err := NewTemplateSet(TemplateSetSpec{
		Entries: []TemplateEntry{entry},
	})
	require.NoError(t, err)
	entry.Groups[0].Charts[0].Priority = 70000
	entry.Groups[0].Charts[0].Title = " Requests "
	explicit, err := NewTemplateSet(TemplateSetSpec{
		Entries: []TemplateEntry{entry},
	})
	require.NoError(t, err)
	assert.True(t, explicit.preservesEntry(original, "profile"))
}

func TestTemplateSetNormalizedGlobalRuleEquality(t *testing.T) {
	policy := EnginePolicy{
		Autogen: &AutogenPolicy{
			Enabled: true,
			Rules: []AutogenRule{{Scope: "private_*", Selector: metrixselector.Expr{
				Deny: []string{"*"},
			}}},
		},
	}
	original, err := NewTemplateSet(TemplateSetSpec{
		Policy: policy,
	})
	require.NoError(t, err)
	policy.Autogen.Rules[0].Scope = " private_* "
	policy.Autogen.Rules[0].Selector.Deny[0] = " * "
	explicit, err := NewTemplateSet(TemplateSetSpec{
		Policy: policy,
	})
	require.NoError(t, err)
	assert.True(t, explicit.SameGlobalPolicy(original))
}
