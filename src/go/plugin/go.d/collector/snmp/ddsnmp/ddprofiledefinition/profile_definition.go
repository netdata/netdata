// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package ddprofiledefinition

import "slices"

// ProfileDefinition is the root profile structure.
type ProfileDefinition struct {
	Selector            SelectorSpec                     `yaml:"selector,omitempty" json:"selector,omitempty"`
	Extends             []string                         `yaml:"extends,omitempty" json:"extends,omitempty"`
	Metadata            MetadataConfig                   `yaml:"metadata,omitempty" json:"metadata,omitempty"`
	SysobjectIDMetadata []SysobjectIDMetadataEntryConfig `yaml:"sysobjectid_metadata,omitempty"`
	Metrics             []MetricsConfig                  `yaml:"metrics,omitempty" json:"metrics,omitempty"`
	MetricTags          []MetricTagConfig                `yaml:"metric_tags,omitempty" json:"metric_tags,omitempty"`
	StaticTags          []string                         `yaml:"static_tags,omitempty" json:"static_tags,omitempty"`

	VirtualMetrics []VirtualMetricConfig `yaml:"virtual_metrics,omitempty" json:"virtual_metrics,omitempty"`

	// DEPRECATED: Keep legacy field for backward compatibility
	SysObjectIDs StringArray `yaml:"sysobjectid,omitempty" json:"sysobjectid,omitempty"`
}

// Clone duplicates this ProfileDefinition
func (p *ProfileDefinition) Clone() *ProfileDefinition {
	if p == nil {
		return nil
	}
	return &ProfileDefinition{
		SysObjectIDs:        slices.Clone(p.SysObjectIDs),
		Selector:            p.Selector.Clone(),
		Extends:             slices.Clone(p.Extends),
		Metadata:            cloneMap(p.Metadata),
		SysobjectIDMetadata: cloneSlice(p.SysobjectIDMetadata),
		MetricTags:          cloneSlice(p.MetricTags),
		StaticTags:          slices.Clone(p.StaticTags),
		Metrics:             cloneSlice(p.Metrics),
		VirtualMetrics:      cloneSlice(p.VirtualMetrics),
	}
}
