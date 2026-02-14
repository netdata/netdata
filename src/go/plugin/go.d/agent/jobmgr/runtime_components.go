// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/chartemit"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/chartengine"
)

// RuntimeComponentConfig registers one internal/runtime metrics component.
//
// The component provides:
//   - RuntimeStore: writer-owned internal metrics store.
//   - TemplateYAML: chart template for turning that store into chart actions.
//   - EmitEnv fields: chart emission metadata (TypeID, plugin/module/job labels).
//
// Emission cadence is owned by the dedicated runtime metrics job.
type RuntimeComponentConfig struct {
	Name string

	Store        metrix.RuntimeStore
	TemplateYAML []byte
	UpdateEvery  int
	Autogen      chartengine.AutogenPolicy

	TypeID    string
	Plugin    string
	Module    string
	JobName   string
	JobLabels map[string]string
}

type runtimeComponentSpec struct {
	Name       string
	Generation uint64

	Store        metrix.RuntimeStore
	TemplateYAML []byte
	UpdateEvery  int
	Autogen      chartengine.AutogenPolicy
	EmitEnv      chartemit.EmitEnv
}

type runtimeComponentRegistry struct {
	mu    sync.RWMutex
	rev   uint64
	items map[string]runtimeComponentSpec
}

func newRuntimeComponentRegistry() *runtimeComponentRegistry {
	return &runtimeComponentRegistry{
		items: make(map[string]runtimeComponentSpec),
	}
}

func (r *runtimeComponentRegistry) upsert(spec runtimeComponentSpec) uint64 {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.rev++
	spec.Generation = r.rev
	r.items[spec.Name] = spec
	return spec.Generation
}

func (r *runtimeComponentRegistry) remove(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.items, name)
}

func (r *runtimeComponentRegistry) snapshot() []runtimeComponentSpec {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]runtimeComponentSpec, 0, len(r.items))
	for _, spec := range r.items {
		out = append(out, runtimeComponentSpec{
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

func (m *Manager) RegisterRuntimeComponent(cfg RuntimeComponentConfig) error {
	if m == nil {
		return fmt.Errorf("jobmgr: nil manager")
	}
	spec, err := m.normalizeRuntimeComponent(cfg)
	if err != nil {
		return err
	}
	m.runtimeComponents.upsert(spec)
	return nil
}

func (m *Manager) UnregisterRuntimeComponent(name string) {
	if m == nil {
		return
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return
	}
	m.runtimeComponents.remove(name)
}

func (m *Manager) normalizeRuntimeComponent(cfg RuntimeComponentConfig) (runtimeComponentSpec, error) {
	name := strings.TrimSpace(cfg.Name)
	if name == "" {
		return runtimeComponentSpec{}, fmt.Errorf("jobmgr: runtime component name is required")
	}
	if cfg.Store == nil {
		return runtimeComponentSpec{}, fmt.Errorf("jobmgr: runtime component %q store is required", name)
	}
	if len(cfg.TemplateYAML) == 0 {
		return runtimeComponentSpec{}, fmt.Errorf("jobmgr: runtime component %q template is required", name)
	}
	updateEvery := cfg.UpdateEvery
	if updateEvery <= 0 {
		updateEvery = 1
	}

	typeID := strings.TrimSpace(cfg.TypeID)
	if typeID == "" {
		typeID = fmt.Sprintf("%s.internal.%s", sanitizeName(m.PluginName), sanitizeName(name))
	}
	env := chartemit.EmitEnv{
		TypeID:      typeID,
		UpdateEvery: updateEvery,
		Plugin:      firstNotEmpty(strings.TrimSpace(cfg.Plugin), m.PluginName),
		Module:      firstNotEmpty(strings.TrimSpace(cfg.Module), "internal"),
		JobName:     firstNotEmpty(strings.TrimSpace(cfg.JobName), name),
		JobLabels:   cloneStringMap(cfg.JobLabels),
	}

	return runtimeComponentSpec{
		Name:         name,
		Store:        cfg.Store,
		TemplateYAML: append([]byte(nil), cfg.TemplateYAML...),
		UpdateEvery:  updateEvery,
		Autogen:      cfg.Autogen,
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
