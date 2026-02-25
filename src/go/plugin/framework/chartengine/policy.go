// SPDX-License-Identifier: GPL-3.0-or-later

package chartengine

import (
	"fmt"
	"strings"

	metrixselector "github.com/netdata/netdata/go/plugins/pkg/metrix/selector"
	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
)

func resolveEffectivePolicy(cfg engineConfig, templatePolicy *charttpl.Engine) (AutogenPolicy, metrixselector.Selector, error) {
	autogen := defaultAutogenPolicy()
	var selector metrixselector.Selector

	if templatePolicy != nil {
		if templatePolicy.Autogen != nil {
			normalized, err := normalizeAutogenPolicy(AutogenPolicy{
				Enabled:                  templatePolicy.Autogen.Enabled,
				TypeID:                   strings.TrimSpace(templatePolicy.Autogen.TypeID),
				MaxTypeIDLen:             templatePolicy.Autogen.MaxTypeIDLen,
				ExpireAfterSuccessCycles: templatePolicy.Autogen.ExpireAfterSuccessCycles,
			})
			if err != nil {
				return AutogenPolicy{}, nil, fmt.Errorf("template engine.autogen: %w", err)
			}
			autogen = normalized
		}

		if templatePolicy.Selector != nil {
			compiled, err := compileEngineSelector(templatePolicy.Selector)
			if err != nil {
				return AutogenPolicy{}, nil, fmt.Errorf("template engine.selector: %w", err)
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

	return autogen, selector, nil
}
