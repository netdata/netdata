// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmp

import (
	"fmt"
)

const (
	MetricTypeGauge                 = "gauge"
	MetricTypeRate                  = "rate"
	MetricTypePercent               = "percent"
	MetricTypeMonotonicCount        = "monotonic_count"
	MetricTypeMonotonicCountAndRate = "monotonic_count_and_rate"
	MetricTypeFlagStream            = "flag_stream" // https://datadoghq.dev/integrations-core/tutorials/snmp/profile-format/#flag-stream
)

// Profile is Datadog SNMP profile (format: https://datadoghq.dev/integrations-core/tutorials/snmp/profile-format/)
type Profile struct {
	SourceFile string

	Extends     []string          `yaml:"extends"`     // +done
	SysObjectID SysObjectIDs      `yaml:"sysobjectid"` // +done
	Metrics     []Metric          `yaml:"metrics"`
	Metadata    *Metadata         `yaml:"metadata"`    // +done
	MetricTags  []GlobalMetricTag `yaml:"metric_tags"` // +done
}

type SysObjectIDs []string

type (
	// Metric defines which metrics will be collected by the profile.
	// Can reference either a single OID (a.k.a symbol), or an SNMP table.
	// https://datadoghq.dev/integrations-core/tutorials/snmp/profile-format/#metrics
	Metric struct {
		Name string `yaml:"name"`
		OID  string `yaml:"OID"`

		// Typically a symbol will be inferred from the SNMP type
		// https://datadoghq.dev/integrations-core/tutorials/snmp/profile-format/#metric-type-inference
		// Can be overwritten using "metric_type"
		// https://datadoghq.dev/integrations-core/tutorials/snmp/profile-format/#forced-metric-types
		MetricType string `yaml:"metric_type"`

		Options map[string]string

		MIB string `yaml:"MIB"`

		// Symbol metric
		// https://datadoghq.dev/integrations-core/tutorials/snmp/profile-format/#symbol-metrics
		Symbol *Symbol `yaml:"symbol"`

		// Table metric
		// https://datadoghq.dev/integrations-core/tutorials/snmp/profile-format/#table-metrics
		Table   *MetricTable `yaml:"table"`
		Symbols []Symbol     `yaml:"symbols"`

		MetricTags []MetricTag `yaml:"metric_tags"` //TODO check for only name existing in metric tag, as there is some case for that
	}
	MetricTable struct {
		OID  string `yaml:"OID"`
		Name string `yaml:"name"`
	}
	// MetricTag used for Table metrics to identify each row's metric.
	// https://datadoghq.dev/integrations-core/tutorials/snmp/profile-format/#table-metrics-tagging
	MetricTag struct {
		MIB            string       `yaml:"mib"`
		Table          string       `yaml:"table"`
		Tag            string       `yaml:"tag"`
		Symbol         Symbol       `yaml:"symbol"`
		IndexTransform []IndexSlice `yaml:"index_transform"`

		Mapping map[int]string `yaml:"mapping"`
		Index   int            `yaml:"index"`
	}
	Symbol struct {
		OID          string  `yaml:"OID"`
		Name         string  `yaml:"name"`
		ExtractValue string  `yaml:"extract_value"`
		MatchPattern string  `yaml:"match_pattern"`
		MatchValue   string  `yaml:"match_value"`
		Format       string  `yaml:"format"`
		ScaleFactor  float64 `yaml:"scale_factor"`
	}
	IndexSlice struct {
		Start int `yaml:"start"`
		End   int `yaml:"end"`
	}
)

type (
	// Metadata used to declare where and how metadata should be collected
	// https://datadoghq.dev/integrations-core/tutorials/snmp/profile-format/#metadata
	Metadata struct {
		Device DeviceMetadata `yaml:"device"`
	}
	DeviceMetadata struct {
		Fields map[string]MetadataField `yaml:"fields"`
	}
	MetadataField struct {
		Value   *string  `yaml:"value"`
		Symbol  *Symbol  `yaml:"symbol"`
		Symbols []Symbol `yaml:"symbols"`
	}
)

// GlobalMetricTag used to apply tags to all metrics collected by the profile
// https://datadoghq.dev/integrations-core/tutorials/snmp/profile-format/#metric_tags
type GlobalMetricTag struct {
	OID    string `yaml:"OID"`
	Symbol string `yaml:"symbol"`
	Tag    string `yaml:"tag"`

	Match   string            `yaml:"match"`
	Tags    map[string]string `yaml:"tags"`
	Mapping map[int]string    `yaml:"mapping"`
}

func (s *SysObjectIDs) UnmarshalYAML(unmarshal func(any) error) error {
	var single string
	if err := unmarshal(&single); err == nil {
		*s = []string{single}
		return nil
	}

	var multiple []string
	if err := unmarshal(&multiple); err == nil {
		*s = multiple
		return nil
	}

	return fmt.Errorf("invalid sysobjectid format")
}
