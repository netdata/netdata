// SPDX-License-Identifier: GPL-3.0-or-later

package runtimechartemit

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/agent/internal/naming"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartemit"
	"github.com/netdata/netdata/go/plugins/plugin/framework/runtimecomp"
)

// Minimal valid template for runtime components that rely on autogen.
// It intentionally matches no real metric; autogen handles real series.
const defaultAutogenTemplateYAML = `
version: v1
groups:
  - family: Internal
    metrics:
      - __runtime_dummy_metric
    charts:
      - id: runtime_dummy
        title: Runtime Dummy
        context: netdata.go.plugin.runtime_internal.runtime_dummy
        units: "1"
        dimensions:
          - selector: __runtime_dummy_metric
            name: value
`

// ComponentConfig registers one internal/runtime metrics component.
//
// The component provides:
//   - RuntimeStore: writer-owned internal metrics store.
//   - TemplateYAML: chart template for turning that store into chart actions.
//   - EmitEnv fields: chart emission metadata (TypeID, plugin/module/job labels).
//
// Emission cadence is owned by the dedicated runtime metrics job.
type ComponentConfig = runtimecomp.ComponentConfig

type componentSpec struct {
	Name       string
	Generation uint64

	Store        metrix.RuntimeStore
	TemplateYAML []byte
	UpdateEvery  int
	Autogen      runtimecomp.AutogenPolicy
	EmitEnv      chartemit.EmitEnv
}

type componentRegistry struct {
	mu    sync.RWMutex
	rev   uint64
	items map[string]componentSpec
}

func newComponentRegistry() *componentRegistry {
	return &componentRegistry{
		items: make(map[string]componentSpec),
	}
}

func (r *componentRegistry) upsert(spec componentSpec) uint64 {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.rev++
	spec.Generation = r.rev
	r.items[spec.Name] = spec
	return spec.Generation
}

func (r *componentRegistry) remove(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.items, name)
}

func (r *componentRegistry) snapshot() []componentSpec {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]componentSpec, 0, len(r.items))
	for _, spec := range r.items {
		out = append(out, componentSpec{
			Name:       spec.Name,
			Generation: spec.Generation,
			Store:      spec.Store,
			TemplateYAML: append([]byte(nil),
				spec.TemplateYAML...),
			UpdateEvery: spec.UpdateEvery,
			Autogen:     spec.Autogen,
			EmitEnv:     cloneEmitEnv(spec.EmitEnv),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func cloneEmitEnv(env chartemit.EmitEnv) chartemit.EmitEnv {
	out := env
	if env.JobLabels != nil {
		out.JobLabels = make(map[string]string, len(env.JobLabels))
		for k, v := range env.JobLabels {
			out.JobLabels[k] = v
		}
	}
	return out
}

func normalizeComponent(cfg ComponentConfig, pluginName string) (componentSpec, error) {
	name := strings.TrimSpace(cfg.Name)
	if name == "" {
		return componentSpec{}, fmt.Errorf("runtimemgr: runtime component name is required")
	}
	if cfg.Store == nil {
		return componentSpec{}, fmt.Errorf("runtimemgr: runtime component %q store is required", name)
	}
	templateYAML := cfg.TemplateYAML
	if len(templateYAML) == 0 {
		if !cfg.Autogen.Enabled {
			return componentSpec{}, fmt.Errorf("runtimemgr: runtime component %q template is required when autogen is disabled", name)
		}
		templateYAML = []byte(defaultAutogenTemplateYAML)
	}
	updateEvery := cfg.UpdateEvery
	if updateEvery <= 0 {
		updateEvery = 1
	}

	typeID := strings.TrimSpace(cfg.TypeID)
	if typeID == "" {
		typeID = defaultInternalTypeID(pluginName, name)
	}
	env := chartemit.EmitEnv{
		TypeID:      typeID,
		UpdateEvery: updateEvery,
		Plugin:      firstNotEmpty(strings.TrimSpace(cfg.Plugin), pluginName),
		Module:      firstNotEmpty(strings.TrimSpace(cfg.Module), "internal"),
		JobName:     firstNotEmpty(strings.TrimSpace(cfg.JobName), name),
		JobLabels:   cloneStringMap(cfg.JobLabels),
	}
	autogen := cfg.Autogen
	// Chartengine autogen type.id budget must use the actual emitted type.id.
	autogen.TypeID = typeID

	return componentSpec{
		Name:         name,
		Store:        cfg.Store,
		TemplateYAML: append([]byte(nil), templateYAML...),
		UpdateEvery:  updateEvery,
		Autogen:      autogen,
		EmitEnv:      env,
	}, nil
}

func cloneStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func firstNotEmpty(items ...string) string {
	for _, item := range items {
		if item = strings.TrimSpace(item); item != "" {
			return item
		}
	}
	return ""
}

func defaultInternalTypeID(pluginName, componentName string) string {
	plugin := naming.Sanitize(firstNotEmpty(pluginName, "go.d"))
	component := naming.Sanitize(componentName)
	return fmt.Sprintf("netdata.%s.internal.%s", plugin, component)
}
