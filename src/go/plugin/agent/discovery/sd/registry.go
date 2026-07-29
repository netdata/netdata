// SPDX-License-Identifier: GPL-3.0-or-later

package sd

import (
	"context"
	"encoding/json"
	"slices"

	"github.com/netdata/netdata/go/plugins/plugin/agent/discovery/sd/model"
)

type Descriptor struct {
	Type            string
	Schema          string
	ParseJSONConfig func(raw json.RawMessage) (any, error)
	TestConfig      func(ctx context.Context, cfg any) error
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
				return nil, &typedConfigError{typ: typ}
			}
			return newDiscs(v, source)
		},
	}
}

func NewDescriptorWithTest[T any](
	typ, schema string,
	parseJSON func(raw json.RawMessage) (T, error),
	testConfig func(ctx context.Context, cfg T) error,
	newDiscs func(cfg T, source string) ([]model.Discoverer, error),
) Descriptor {
	desc := NewDescriptor(typ, schema, parseJSON, newDiscs)
	desc.TestConfig = func(ctx context.Context, cfg any) error {
		v, ok := cfg.(T)
		if !ok {
			return &typedConfigError{typ: typ}
		}
		return testConfig(ctx, v)
	}
	return desc
}

type typedConfigError struct {
	typ string
}

func (e *typedConfigError) Error() string {
	return "discoverer '" + e.typ + "': invalid parsed config type"
}
