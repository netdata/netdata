// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package ddprofiledefinition

// MetadataDeviceResource is the device resource name
const MetadataDeviceResource = "device"

// MetadataConfig holds configs per resource type
type MetadataConfig map[string]MetadataResourceConfig

// Clone duplicates this MetadataConfig
func (mc MetadataConfig) Clone() MetadataConfig {
	return cloneMap(mc)
}

// MetadataResourceConfig holds configs for a metadata resource
type MetadataResourceConfig struct {
	Fields map[string]MetadataField `yaml:"fields" json:"fields"`
	IDTags MetricTagConfigList      `yaml:"id_tags,omitempty" json:"id_tags,omitempty"`
}

// Clone duplicates this MetadataResourceConfig
func (c MetadataResourceConfig) Clone() MetadataResourceConfig {
	return MetadataResourceConfig{
		Fields: cloneMap(c.Fields),
		IDTags: cloneSlice(c.IDTags),
	}
}

// MetadataField holds configs for a metadata field
type MetadataField struct {
	Symbol  SymbolConfig   `yaml:"symbol,omitempty" json:"symbol,omitempty"`
	Symbols []SymbolConfig `yaml:"symbols,omitempty" json:"symbols,omitempty"`
	Value   string         `yaml:"value,omitempty" json:"value,omitempty"`
}

// Clone duplicates this MetadataField
func (c MetadataField) Clone() MetadataField {
	return MetadataField{
		Symbol:  c.Symbol.Clone(),
		Symbols: cloneSlice(c.Symbols),
		Value:   c.Value,
	}
}

type SysobjectIDMetadataEntryConfig struct {
	SysobjectID string                   `yaml:"sysobjectid"`
	Metadata    map[string]MetadataField `yaml:"metadata"`
}

func (e SysobjectIDMetadataEntryConfig) Clone() SysobjectIDMetadataEntryConfig {
	return SysobjectIDMetadataEntryConfig{
		SysobjectID: e.SysobjectID,
		Metadata:    cloneMap(e.Metadata),
	}
}
