// SPDX-License-Identifier: GPL-3.0-or-later

package collectorapi

import (
	"fmt"
	"slices"

	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
)

// ChartTemplateSource resolves the two collector capabilities and binds fixed
// job policy. It is owned by the job goroutine. It captures desired snapshots;
// each scope's engine independently owns its last published snapshot.
type ChartTemplateSource struct {
	static   StaticChartTemplateProvider
	dynamic  ChartTemplateSetProvider
	options  []chartengine.Option
	input    *chartengine.TemplateSet
	prepared *chartengine.TemplateSet
}

// NewChartTemplateSource requires exactly one provider and captures optional
// CollectorV2EnginePolicy once. Call Capture after Check and successful Collect.
// Function-only jobs do not need a source.
func NewChartTemplateSource(collector any, opts ...chartengine.Option) (*ChartTemplateSource, error) {
	static, hasStatic := collector.(StaticChartTemplateProvider)
	dynamic, hasDynamic := collector.(ChartTemplateSetProvider)
	if hasStatic == hasDynamic {
		return nil, fmt.Errorf("collector must implement exactly one of StaticChartTemplateProvider or ChartTemplateSetProvider")
	}
	options := slices.Clone(opts)
	if policy, ok := collector.(CollectorV2EnginePolicy); ok {
		options = append(options, chartengine.WithEnginePolicy(policy.EnginePolicy()))
	}
	return &ChartTemplateSource{
		static:  static,
		dynamic: dynamic,
		options: options,
	}, nil
}

// Capture reads a dynamic provider once, or reuses the captured static document.
// Invalid snapshots leave the previous binding intact. The caller must abort the
// staged metric cycle on error and must not publish before metric commit succeeds.
func (s *ChartTemplateSource) Capture() (*chartengine.TemplateSet, error) {
	if s == nil {
		return nil, fmt.Errorf("nil chart template source")
	}
	var input *chartengine.TemplateSet
	if s.dynamic != nil {
		input = s.dynamic.ChartTemplateSet()
		if input == nil {
			return nil, fmt.Errorf("collector returned nil chart template set")
		}
		if input == s.input {
			return s.prepared, nil
		}
	} else {
		if s.prepared != nil {
			return s.prepared, nil
		}
		var err error
		input, err = chartengine.NewTemplateSetYAML([]byte(s.static.ChartTemplateYAML()))
		if err != nil {
			return nil, err
		}
	}
	prepared, err := chartengine.PrepareTemplateSet(input, s.options...)
	if err != nil {
		return nil, err
	}
	if s.prepared != nil && !s.prepared.SameGlobalPolicy(prepared) {
		return nil, fmt.Errorf("collector changed fixed chart engine policy or fallback namespace; recreate the job")
	}
	s.input, s.prepared = input, prepared
	return prepared, nil
}
