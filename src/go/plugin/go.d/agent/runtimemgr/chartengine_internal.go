// SPDX-License-Identifier: GPL-3.0-or-later

package runtimemgr

import (
	"log/slog"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/chartengine"
)

const ChartengineInternalComponentName = "chartengine_internal"

// Minimal valid template for runtime components that rely on autogen.
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
        context: netdata.go.plugin.chartengine_internal.runtime_dummy
        units: "1"
        dimensions:
          - selector: __runtime_dummy_metric
            name: value
`

// NewChartengineInternalComponent creates runtime component config + producer tick
// for chartengine self-observability.
func NewChartengineInternalComponent(log *logger.Logger) (ComponentConfig, func() error, error) {
	if log == nil {
		log = logger.New().With(slog.String("component", "runtime-producer"))
	}

	engineLog := log.With(
		slog.String("component", "runtime-producer"),
		slog.String("runtime_component", ChartengineInternalComponentName),
	)
	engine, err := chartengine.New(chartengine.WithLogger(engineLog))
	if err != nil {
		return ComponentConfig{}, nil, err
	}
	if err := engine.LoadYAML([]byte(internalRuntimeAutogenTemplateYAML), 1); err != nil {
		return ComponentConfig{}, nil, err
	}

	cfg := ComponentConfig{
		Name:         ChartengineInternalComponentName,
		Store:        engine.RuntimeStore(),
		TemplateYAML: []byte(internalRuntimeAutogenTemplateYAML),
		UpdateEvery:  1,
		Autogen: chartengine.AutogenPolicy{
			Enabled: true,
		},
		Module:  "internal",
		JobName: ChartengineInternalComponentName,
		JobLabels: map[string]string{
			"source": "chartengine",
		},
	}

	producerTick := func() error {
		_, err := engine.BuildPlan(engine.RuntimeStore().Read(metrix.ReadRaw(), metrix.ReadFlatten()))
		return err
	}

	return cfg, producerTick, nil
}
