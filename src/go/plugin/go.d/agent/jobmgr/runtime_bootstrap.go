// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

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

func (m *Manager) bootstrapRuntimeComponents() {
	if m == nil {
		return
	}
	if m.hasRuntimeProducer(chartengineInternalComponentName) {
		return
	}

	engineLog := m.Logger.With(
		slog.String("component", "runtime-producer"),
		slog.String("runtime_component", chartengineInternalComponentName),
	)
	engine, err := chartengine.New(chartengine.WithLogger(engineLog))
	if err != nil {
		m.Warningf("bootstrap runtime component %q failed: %v", chartengineInternalComponentName, err)
		return
	}
	if err := engine.LoadYAML([]byte(internalRuntimeAutogenTemplateYAML), 1); err != nil {
		m.Warningf("bootstrap runtime component %q template load failed: %v", chartengineInternalComponentName, err)
		return
	}

	pluginName := firstNotEmpty(m.PluginName, "go.d")
	typeID := fmt.Sprintf("%s.internal.%s", sanitizeName(pluginName), sanitizeName(chartengineInternalComponentName))
	autogen := chartengine.AutogenPolicy{
		Enabled: true,
		TypeID:  typeID,
	}
	err = m.RegisterRuntimeComponent(RuntimeComponentConfig{
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
	})
	if err != nil {
		m.Warningf("bootstrap runtime component %q registration failed: %v", chartengineInternalComponentName, err)
		return
	}

	m.runtimeProducers = append(m.runtimeProducers, &chartengineRuntimeProducer{
		name:   chartengineInternalComponentName,
		engine: engine,
	})
}

func (m *Manager) tickRuntimeProducers() {
	if m == nil || len(m.runtimeProducers) == 0 {
		return
	}
	for _, producer := range m.runtimeProducers {
		if producer == nil {
			continue
		}
		if err := producer.Tick(); err != nil {
			m.Warningf("runtime producer %q tick failed: %v", producer.Name(), err)
		}
	}
}

func (m *Manager) hasRuntimeProducer(name string) bool {
	for _, producer := range m.runtimeProducers {
		if producer == nil {
			continue
		}
		if producer.Name() == name {
			return true
		}
	}
	return false
}
