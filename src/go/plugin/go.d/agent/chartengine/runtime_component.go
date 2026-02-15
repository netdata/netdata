// SPDX-License-Identifier: GPL-3.0-or-later

package chartengine

import (
	"log/slog"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/runtimecomp"
)

const InternalRuntimeComponentName = "chartengine_internal"

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

// RegisterInternalRuntimeComponent wires chartengine self-observability into
// a runtime component service.
func RegisterInternalRuntimeComponent(service runtimecomp.Service, log *logger.Logger) error {
	if log == nil {
		log = logger.New().With(slog.String("component", "runtime-producer"))
	}

	engineLog := log.With(
		slog.String("component", "runtime-producer"),
		slog.String("runtime_component", InternalRuntimeComponentName),
	)
	engine, err := New(WithLogger(engineLog))
	if err != nil {
		return err
	}
	if err := engine.LoadYAML([]byte(internalRuntimeAutogenTemplateYAML), 1); err != nil {
		return err
	}

	cfg := runtimecomp.ComponentConfig{
		Name:         InternalRuntimeComponentName,
		Store:        engine.RuntimeStore(),
		TemplateYAML: []byte(internalRuntimeAutogenTemplateYAML),
		UpdateEvery:  1,
		Autogen: runtimecomp.AutogenPolicy{
			Enabled: true,
		},
		Module:  "internal",
		JobName: InternalRuntimeComponentName,
		JobLabels: map[string]string{
			"source": "chartengine",
		},
	}
	if err := service.RegisterComponent(cfg); err != nil {
		return err
	}

	return service.RegisterProducer(cfg.Name, func() error {
		_, err := engine.BuildPlan(engine.RuntimeStore().Read(metrix.ReadRaw(), metrix.ReadFlatten()))
		return err
	})
}
