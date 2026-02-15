// SPDX-License-Identifier: GPL-3.0-or-later

package runtimemgr

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/chartemit"
)

func TestRuntimeComponentRegistrationScenarios(t *testing.T) {
	tests := map[string]struct {
		run func(t *testing.T)
	}{
		"registration applies defaults and stores normalized emit env": {
			run: func(t *testing.T) {
				svc := New(nil)
				svc.pluginName = "go.d"
				store := metrix.NewRuntimeStore()

				err := svc.RegisterComponent(ComponentConfig{
					Name:         "chartengine",
					Store:        store,
					TemplateYAML: []byte(runtimeGaugeTemplateYAML()),
				})
				require.NoError(t, err)

				specs := svc.registry.snapshot()
				require.Len(t, specs, 1)

				spec := specs[0]
				assert.Equal(t, "chartengine", spec.Name)
				assert.Equal(t, 1, spec.UpdateEvery)
				assert.Equal(t, "netdata.go.d.internal.chartengine", spec.EmitEnv.TypeID)
				assert.Equal(t, "go.d", spec.EmitEnv.Plugin)
				assert.Equal(t, "internal", spec.EmitEnv.Module)
				assert.Equal(t, "chartengine", spec.EmitEnv.JobName)
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, tc.run)
	}
}

func TestRuntimeMetricsJobScenarios(t *testing.T) {
	tests := map[string]struct {
		run func(t *testing.T)
	}{
		"runtime job respects component cadence and emits on scheduled tick": {
			run: func(t *testing.T) {
				reg := newComponentRegistry()
				store := metrix.NewRuntimeStore()
				g := store.Write().StatefulMeter("component").Gauge("load")
				g.Set(5)

				reg.upsert(componentSpec{
					Name:         "component",
					Store:        store,
					TemplateYAML: []byte(runtimeGaugeTemplateYAML()),
					UpdateEvery:  2,
					EmitEnv: chartemit.EmitEnv{
						TypeID:      "netdata.go.d.internal.component",
						UpdateEvery: 2,
						Plugin:      "go.d",
						Module:      "internal",
						JobName:     "component",
					},
				})

				var out bytes.Buffer
				job := newRuntimeMetricsJob(&out, reg, nil)

				job.runOnce(1)
				assert.Equal(t, 0, out.Len())

				job.runOnce(2)
				result := out.String()
				assert.Contains(t, result, "CHART")
				assert.Contains(t, result, "BEGIN")
			},
		},
		"runtime job observes all visible runtime series (not only latest seq)": {
			run: func(t *testing.T) {
				reg := newComponentRegistry()
				store := metrix.NewRuntimeStore()
				vec := store.Write().StatefulMeter("component").Vec("id").Gauge("load")
				vec.WithLabelValues("a").Set(1)
				vec.WithLabelValues("b").Set(2)

				reg.upsert(componentSpec{
					Name:         "component",
					Store:        store,
					TemplateYAML: []byte(runtimeDynamicDimTemplateYAML()),
					UpdateEvery:  1,
					EmitEnv: chartemit.EmitEnv{
						TypeID:      "netdata.go.d.internal.component",
						UpdateEvery: 1,
						Plugin:      "go.d",
						Module:      "internal",
						JobName:     "component",
					},
				})

				var out bytes.Buffer
				job := newRuntimeMetricsJob(&out, reg, nil)
				job.runOnce(1)

				result := out.String()
				assert.Contains(t, result, "DIMENSION 'a' 'a' 'absolute'")
				assert.Contains(t, result, "DIMENSION 'b' 'b' 'absolute'")
			},
		},
		"runtime job emits obsolete chart when component is removed": {
			run: func(t *testing.T) {
				reg := newComponentRegistry()
				store := metrix.NewRuntimeStore()
				store.Write().StatefulMeter("component").Gauge("load").Set(3)

				reg.upsert(componentSpec{
					Name:         "component",
					Store:        store,
					TemplateYAML: []byte(runtimeGaugeTemplateYAML()),
					UpdateEvery:  1,
					EmitEnv: chartemit.EmitEnv{
						TypeID:      "netdata.go.d.internal.component",
						UpdateEvery: 1,
						Plugin:      "go.d",
						Module:      "internal",
						JobName:     "component",
					},
				})

				var out bytes.Buffer
				job := newRuntimeMetricsJob(&out, reg, nil)
				job.runOnce(1)
				require.Contains(t, out.String(), "CHART 'netdata.go.d.internal.component.component_load'")

				out.Reset()
				reg.remove("component")
				job.runOnce(2)
				result := out.String()
				assert.Contains(t, result, "CHART 'netdata.go.d.internal.component.component_load'")
				assert.Contains(t, result, "'obsolete'")
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, tc.run)
	}
}

func TestRuntimeBootstrapScenarios(t *testing.T) {
	tests := map[string]struct {
		run func(t *testing.T)
	}{
		"bootstrap registers chartengine producer component and ticking produces metrics": {
			run: func(t *testing.T) {
				svc := New(nil)
				svc.Start("go.d", &bytes.Buffer{})
				defer svc.Stop()

				specs := svc.registry.snapshot()
				require.Len(t, specs, 1)
				assert.Equal(t, chartengineInternalComponentName, specs[0].Name)

				svc.Tick(1)
				value, ok := specs[0].Store.ReadRaw().Value("netdata.go.plugin.chartengine.build_calls_total", nil)
				require.True(t, ok)
				assert.GreaterOrEqual(t, value, float64(1))

				// Idempotent bootstrap.
				svc.mu.Lock()
				svc.bootstrapDefaults()
				svc.mu.Unlock()
				assert.Len(t, svc.producers, 1)
				assert.Len(t, svc.registry.snapshot(), 1)
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, tc.run)
	}
}

func runtimeGaugeTemplateYAML() string {
	return `
version: v1
groups:
  - family: Runtime
    metrics:
      - component.load
    charts:
      - id: component_load
        title: Component Load
        context: netdata.go.plugin.component.component_load
        units: load
        dimensions:
          - selector: component.load
            name: value
`
}

func runtimeDynamicDimTemplateYAML() string {
	return `
version: v1
groups:
  - family: Runtime
    metrics:
      - component.load
    charts:
      - id: component_load
        title: Component Load
        context: netdata.go.plugin.component.component_load
        units: load
        dimensions:
          - selector: component.load
            name_from_label: id
`
}
