// SPDX-License-Identifier: GPL-3.0-or-later

package runtimemgr

import (
	"bytes"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/framework/chartemit"
	"github.com/netdata/netdata/go/plugins/plugin/framework/runtimecomp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
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
		"registration allows empty template when autogen is enabled": {
			run: func(t *testing.T) {
				svc := New(nil)
				svc.pluginName = "go.d"
				store := metrix.NewRuntimeStore()

				err := svc.RegisterComponent(ComponentConfig{
					Name:  "component",
					Store: store,
					Autogen: runtimecomp.AutogenPolicy{
						Enabled: true,
					},
				})
				require.NoError(t, err)

				specs := svc.registry.snapshot()
				require.Len(t, specs, 1)
				assert.NotEmpty(t, specs[0].TemplateYAML)
			},
		},
		"registration rejects empty template when autogen is disabled": {
			run: func(t *testing.T) {
				svc := New(nil)
				svc.pluginName = "go.d"
				store := metrix.NewRuntimeStore()

				err := svc.RegisterComponent(ComponentConfig{
					Name:  "component",
					Store: store,
				})
				require.Error(t, err)
				assert.Contains(t, err.Error(), "template is required when autogen is disabled")
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

func TestRuntimeProducerRegistrationScenarios(t *testing.T) {
	tests := map[string]struct {
		run func(t *testing.T)
	}{
		"register producer validates input and unregisters cleanly": {
			run: func(t *testing.T) {
				svc := New(nil)
				svc.Start("go.d", &bytes.Buffer{})
				defer svc.Stop()

				require.Error(t, svc.RegisterProducer("", func() error { return nil }))
				require.Error(t, svc.RegisterProducer("x", nil))

				var calls int
				require.NoError(t, svc.RegisterProducer("x", func() error {
					calls++
					return nil
				}))
				svc.Tick(1)
				assert.Equal(t, 1, calls)

				svc.UnregisterProducer("x")
				svc.Tick(2)
				assert.Equal(t, 1, calls)
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
