// SPDX-License-Identifier: GPL-3.0-or-later

package chartengine

import (
	"fmt"
	"slices"

	metrixselector "github.com/netdata/netdata/go/plugins/pkg/metrix/selector"
	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
)

type effectiveEnginePolicy struct {
	autogen      AutogenPolicy
	autogenRules []charttpl.ValidatedAutogenRule
	selector     metrixselector.Selector
}

func resolveEffectivePolicy(
	cfg engineConfig,
	templatePolicy *charttpl.Engine,
	validatedRules []charttpl.ValidatedAutogenRule,
) (effectiveEnginePolicy, error) {
	autogen := defaultAutogenPolicy()
	var autogenRules []charttpl.ValidatedAutogenRule
	var selector metrixselector.Selector

	if templatePolicy != nil {
		if templatePolicy.Autogen != nil {
			normalized, rules, err := normalizeAutogenPolicyWithRules(
				AutogenPolicy{
					Enabled:                  templatePolicy.Autogen.Enabled,
					Rules:                    templatePolicy.Autogen.Rules,
					MaxTypeIDLen:             templatePolicy.Autogen.MaxTypeIDLen,
					ExpireAfterSuccessCycles: templatePolicy.Autogen.ExpireAfterSuccessCycles,
				},
				validatedRules,
			)
			if err != nil {
				return effectiveEnginePolicy{}, fmt.Errorf("template engine.autogen: %w", err)
			}
			autogen = normalized
			autogenRules = rules
		}

		if templatePolicy.Selector != nil {
			compiled, err := compileEngineSelector(*templatePolicy.Selector)
			if err != nil {
				return effectiveEnginePolicy{}, fmt.Errorf("template engine.selector: %w", err)
			}
			selector = compiled
		}
	}

	if cfg.autogenOverride.set {
		autogen = cloneAutogenPolicy(cfg.autogenOverride.value)
		autogenRules = slices.Clone(cfg.autogenRulesOverride.value)
	}
	if cfg.selectorOverride.set {
		selector = cfg.selectorOverride.value
	}

	return effectiveEnginePolicy{
		autogen:      autogen,
		autogenRules: autogenRules,
		selector:     selector,
	}, nil
}
