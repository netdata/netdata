// SPDX-License-Identifier: GPL-3.0-or-later

package chartengine

import (
	"fmt"

	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	metrixselector "github.com/netdata/netdata/go/plugins/pkg/metrix/selector"
	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
)

type effectiveEnginePolicy struct {
	autogen        AutogenPolicy
	autogenExclude matcher.PositivePatternList
	selector       metrixselector.Selector
}

func resolveEffectivePolicy(cfg engineConfig, templatePolicy *charttpl.Engine) (effectiveEnginePolicy, error) {
	autogen := defaultAutogenPolicy()
	var selector metrixselector.Selector

	if templatePolicy != nil {
		if templatePolicy.Autogen != nil {
			normalized, err := normalizeAutogenPolicy(AutogenPolicy{
				Enabled:                  templatePolicy.Autogen.Enabled,
				Exclude:                  templatePolicy.Autogen.Exclude,
				MaxTypeIDLen:             templatePolicy.Autogen.MaxTypeIDLen,
				ExpireAfterSuccessCycles: templatePolicy.Autogen.ExpireAfterSuccessCycles,
			})
			if err != nil {
				return effectiveEnginePolicy{}, fmt.Errorf("template engine.autogen: %w", err)
			}
			autogen = normalized
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
		autogen = cfg.autogenOverride.value
	}
	if cfg.selectorOverride.set {
		selector = cfg.selectorOverride.value
	}

	exclude, err := matcher.CompilePositivePatternList(autogen.Exclude)
	if err != nil {
		return effectiveEnginePolicy{}, fmt.Errorf("effective engine.autogen.exclude: %w", err)
	}
	return effectiveEnginePolicy{
		autogen:        autogen,
		autogenExclude: exclude,
		selector:       selector,
	}, nil
}
