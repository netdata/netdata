// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package ddprofiledefinition

import (
	"fmt"
	"maps"
	"reflect"
)

type MappingMode string

const (
	MappingModeExact   MappingMode = "exact"
	MappingModeBitmask MappingMode = "bitmask"
)

// MappingConfig defines mapping behavior for metric values, tags, and metadata.
// Legacy flat-map YAML remains supported and is interpreted as exact mode.
type MappingConfig struct {
	Mode  MappingMode       `yaml:"mode,omitempty" json:"mode,omitempty"`
	Items map[string]string `yaml:"items,omitempty" json:"items,omitempty"`
}

func NewExactMapping(items map[string]string) MappingConfig {
	return MappingConfig{
		Mode:  MappingModeExact,
		Items: maps.Clone(items),
	}
}

func NewBitmaskMapping(items map[string]string) MappingConfig {
	return MappingConfig{
		Mode:  MappingModeBitmask,
		Items: maps.Clone(items),
	}
}

func (m MappingConfig) Clone() MappingConfig {
	return MappingConfig{
		Mode:  m.Mode,
		Items: maps.Clone(m.Items),
	}
}

func (m MappingConfig) HasItems() bool {
	return len(m.Items) > 0
}

func (m MappingConfig) Lookup(key string) (string, bool) {
	if len(m.Items) == 0 {
		return "", false
	}
	v, ok := m.Items[key]
	return v, ok
}

func (m MappingConfig) EffectiveMode() MappingMode {
	if m.Mode == "" {
		return MappingModeExact
	}
	return m.Mode
}

func (m MappingConfig) String() string {
	if m.Mode == "" || m.Mode == MappingModeExact {
		return fmt.Sprintf("%v", m.Items)
	}
	return fmt.Sprintf("{mode:%s items:%v}", m.Mode, m.Items)
}

// UnmarshalYAML supports both the legacy flat-map syntax and the structured object syntax.
func (m *MappingConfig) UnmarshalYAML(unmarshal func(any) error) error {
	var raw map[string]any
	if err := unmarshal(&raw); err == nil {
		if items, ok := raw["items"]; ok && isStructuredMappingItems(items) {
			return m.unmarshalStructured(unmarshal)
		}
	}

	var items map[string]string
	if err := unmarshal(&items); err != nil {
		return err
	}

	if len(items) == 0 {
		*m = MappingConfig{}
		return nil
	}

	*m = MappingConfig{
		Mode:  MappingModeExact,
		Items: items,
	}

	return nil
}

func (m *MappingConfig) unmarshalStructured(unmarshal func(any) error) error {
	type plain MappingConfig

	var cfg plain
	if err := unmarshal(&cfg); err != nil {
		return err
	}

	mode := MappingConfig(cfg).Mode
	items := MappingConfig(cfg).Items
	if len(items) == 0 {
		items = nil
	}
	if len(items) > 0 && mode == "" {
		mode = MappingModeExact
	}

	*m = MappingConfig{Mode: mode, Items: items}
	return nil
}

func isStructuredMappingItems(v any) bool {
	rv := reflect.ValueOf(v)
	return rv.IsValid() && rv.Kind() == reflect.Map
}
