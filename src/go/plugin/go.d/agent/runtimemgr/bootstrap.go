// SPDX-License-Identifier: GPL-3.0-or-later

package runtimemgr

import (
	"fmt"
	"log/slog"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/chartengine"
)

const chartengineInternalComponentName = "chartengine_internal"

// Minimal valid template for bootstrapped internal runtime components.
// It intentionally matches no real metric; autogen handles real series.
const internalRuntimeAutogenTemplateYAML = `
version: v1
groups:
  - family: Internal
    metrics:
      - __runtime_dummy_metric
    charts:
      - id: runtime_dummy
        title: Runtime Dummy
        context: runtime_dummy
        units: "1"
        dimensions:
          - selector: __runtime_dummy_metric
            name: value
`

type runtimeProducer interface {
	Name() string
	Tick() error
}

type chartengineRuntimeProducer struct {
	name   string
	engine *chartengine.Engine
}

func (p *chartengineRuntimeProducer) Name() string { return p.name }

func (p *chartengineRuntimeProducer) Tick() error {
	if p == nil || p.engine == nil {
		return nil
	}
	_, err := p.engine.BuildPlan(p.engine.RuntimeStore().ReadRaw())
	return err
}

func (s *Service) bootstrapDefaults() {
	if s == nil {
		return
	}
	if s.hasRuntimeProducer(chartengineInternalComponentName) {
		return
	}

	engineLog := s.Logger.With(
		slog.String("component", "runtime-producer"),
		slog.String("runtime_component", chartengineInternalComponentName),
	)
	engine, err := chartengine.New(chartengine.WithLogger(engineLog))
	if err != nil {
		s.Warningf("bootstrap runtime component %q failed: %v", chartengineInternalComponentName, err)
		return
	}
	if err := engine.LoadYAML([]byte(internalRuntimeAutogenTemplateYAML), 1); err != nil {
		s.Warningf("bootstrap runtime component %q template load failed: %v", chartengineInternalComponentName, err)
		return
	}

	pluginName := firstNotEmpty(s.pluginName, "go.d")
	typeID := fmt.Sprintf("%s.internal.%s", sanitizeName(pluginName), sanitizeName(chartengineInternalComponentName))
	autogen := chartengine.AutogenPolicy{
		Enabled: true,
		TypeID:  typeID,
	}
	spec, err := normalizeComponent(ComponentConfig{
		Name:         chartengineInternalComponentName,
		Store:        engine.RuntimeStore(),
		TemplateYAML: []byte(internalRuntimeAutogenTemplateYAML),
		UpdateEvery:  1,
		Autogen:      autogen,
		TypeID:       typeID,
		Plugin:       pluginName,
		Module:       "internal",
		JobName:      chartengineInternalComponentName,
		JobLabels: map[string]string{
			"source": "chartengine",
		},
	}, pluginName)
	if err != nil {
		s.Warningf("bootstrap runtime component %q registration failed: %v", chartengineInternalComponentName, err)
		return
	}
	s.registry.upsert(spec)

	s.producers = append(s.producers, &chartengineRuntimeProducer{
		name:   chartengineInternalComponentName,
		engine: engine,
	})
}

func (s *Service) hasRuntimeProducer(name string) bool {
	for _, producer := range s.producers {
		if producer == nil {
			continue
		}
		if producer.Name() == name {
			return true
		}
	}
	return false
}
