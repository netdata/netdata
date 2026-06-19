// SPDX-License-Identifier: GPL-3.0-or-later

package sd

import (
	"encoding/json"
	"slices"

	"github.com/netdata/netdata/go/plugins/plugin/agent/discovery/sd/model"
)

type Descriptor struct {
	Type            string
	Schema          string
	ParseJSONConfig func(raw json.RawMessage) (any, error)
	NewDiscoverers  func(cfg any, source string) ([]model.Discoverer, error)
}

type Registry interface {
	Types() []string
	Get(typ string) (Descriptor, bool)
}

type mapRegistry struct {
	types []string
	desc  map[string]Descriptor
}

func NewRegistry(descriptors ...Descriptor) Registry {
	reg := &mapRegistry{
		types: make([]string, 0, len(descriptors)),
		desc:  make(map[string]Descriptor, len(descriptors)),
	}
	for _, d := range descriptors {
		if d.Type == "" {
			continue
		}
		if _, ok := reg.desc[d.Type]; !ok {
			reg.types = append(reg.types, d.Type)
		}
		reg.desc[d.Type] = d
	}
	return reg
}

func (r *mapRegistry) Types() []string {
	return slices.Clone(r.types)
}

func (r *mapRegistry) Get(typ string) (Descriptor, bool) {
	d, ok := r.desc[typ]
	return d, ok
}

func NewDescriptor[T any](
	typ, schema string,
	parseJSON func(raw json.RawMessage) (T, error),
	newDiscs func(cfg T, source string) ([]model.Discoverer, error),
) Descriptor {
	return Descriptor{
		Type:   typ,
		Schema: schema,
		ParseJSONConfig: func(raw json.RawMessage) (any, error) {
			return parseJSON(raw)
		},
		NewDiscoverers: func(cfg any, source string) ([]model.Discoverer, error) {
			v, ok := cfg.(T)
			if !ok {
				var z T
				return nil, &typedConfigError{typ: typ, got: cfg, want: z}
			}
			return newDiscs(v, source)
		},
	}
}

type typedConfigError struct {
	typ  string
	got  any
	want any
}

func (e *typedConfigError) Error() string {
	return "discoverer '" + e.typ + "': invalid parsed config type"
}
