// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package ddprofiledefinition

import "slices"

// DeviceMeta holds device related static metadata
// DEPRECATED in favour of profile metadata syntax
type DeviceMeta struct {
	// deprecated in favour of new `ProfileDefinition.Metadata` syntax
	Vendor string `yaml:"vendor,omitempty" json:"vendor,omitempty"`
}

// ProfileDefinition is the root profile structure. The ProfileDefinition is currently used in:
// 1/ SNMP Integration: the profiles are in yaml profiles. Yaml profiles include default datadog profiles and user custom profiles.
// The serialisation of yaml profiles are defined by the yaml annotation and few custom unmarshaller (see yaml_utils.go).
// 2/ Datadog backend: the profiles are in json format, they are used to store profiles created via UI.
// The serialisation of json profiles are defined by the json annotation.
type ProfileDefinition struct {
	Name                string                           `yaml:"name,omitempty" json:"name,omitempty"`
	Description         string                           `yaml:"description,omitempty" json:"description,omitempty"`
	SysObjectIDs        StringArray                      `yaml:"sysobjectid,omitempty" json:"sysobjectid,omitempty"`
	Extends             []string                         `yaml:"extends,omitempty" json:"extends,omitempty"`
	Metadata            MetadataConfig                   `yaml:"metadata,omitempty" json:"metadata,omitempty"`
	SysobjectIDMetadata []SysobjectIDMetadataEntryConfig `yaml:"sysobjectid_metadata,omitempty"`
	MetricTags          []MetricTagConfig                `yaml:"metric_tags,omitempty" json:"metric_tags,omitempty"`
	StaticTags          []string                         `yaml:"static_tags,omitempty" json:"static_tags,omitempty"`
	Metrics             []MetricsConfig                  `yaml:"metrics,omitempty" json:"metrics,omitempty"`

	VirtualMetrics []VirtualMetricConfig `yaml:"virtual_metrics,omitempty" json:"virtual_metrics,omitempty"`

	// DEPRECATED: Use metadata directly
	Device DeviceMeta `yaml:"device,omitempty" json:"device,omitempty" jsonschema:"device,omitempty"`

	// Version is the profile version.
	// It is currently used only with downloaded/RC profiles.
	Version uint64 `yaml:"version,omitempty" json:"version,omitempty"`
}

// DeviceProfileRcConfig represent the profile stored in remote config.
type DeviceProfileRcConfig struct {
	Profile ProfileDefinition `json:"profile_definition"`
}

// NewProfileDefinition creates a new ProfileDefinition
func NewProfileDefinition() *ProfileDefinition {
	p := &ProfileDefinition{}
	p.Metadata = make(MetadataConfig)
	return p
}

// SplitOIDs returns two slices (scalars, columns) of all scalar and column OIDs requested by this profile.
func (p *ProfileDefinition) SplitOIDs(includeMetadata bool) ([]string, []string) {
	if includeMetadata {
		return splitOIDs(p.Metrics, p.MetricTags, p.Metadata)
	}
	return splitOIDs(p.Metrics, p.MetricTags, nil)
}

// Clone duplicates this ProfileDefinition
func (p *ProfileDefinition) Clone() *ProfileDefinition {
	if p == nil {
		return nil
	}
	return &ProfileDefinition{
		Name:                p.Name,
		Description:         p.Description,
		SysObjectIDs:        slices.Clone(p.SysObjectIDs),
		Extends:             slices.Clone(p.Extends),
		Metadata:            CloneMap(p.Metadata),
		SysobjectIDMetadata: CloneSlice(p.SysobjectIDMetadata),
		MetricTags:          CloneSlice(p.MetricTags),
		StaticTags:          slices.Clone(p.StaticTags),
		Metrics:             CloneSlice(p.Metrics),
		VirtualMetrics:      CloneSlice(p.VirtualMetrics),
		Device: DeviceMeta{
			Vendor: p.Device.Vendor,
		},
		Version: p.Version,
	}
}
