// SPDX-License-Identifier: GPL-3.0-or-later

// Package prominput owns validation-facing inputs shared by proof
// orchestration and the standalone Prometheus profile validator.
package prominput

import (
	"fmt"

	prompkg "github.com/netdata/netdata/go/plugins/pkg/prometheus"
	commonmodel "github.com/prometheus/common/model"
	promlabels "github.com/prometheus/prometheus/model/labels"
)

// FutureInput is one synthetic raw scalar used to prove future namespace
// behavior without adding it to a source-complete fixture.
type FutureInput struct {
	Name   string            `yaml:"name"`
	Type   string            `yaml:"type,omitempty"`
	Labels map[string]string `yaml:"labels,omitempty"`
}

func (input FutureInput) EffectiveType() commonmodel.MetricType {
	switch input.Type {
	case "", "gauge":
		return commonmodel.MetricTypeGauge
	case "counter":
		return commonmodel.MetricTypeCounter
	case "untyped":
		return commonmodel.MetricTypeUnknown
	default:
		return commonmodel.MetricType(input.Type)
	}
}

// ValidateFutureInputs validates one ordered synthetic-input set.
func ValidateFutureInputs(field string, inputs []FutureInput) error {
	seen := make(map[prompkg.RawSampleIdentity]struct{}, len(inputs))
	typesByName := make(map[string]commonmodel.MetricType, len(inputs))
	for index, input := range inputs {
		itemField := fmt.Sprintf("%s[%d]", field, index)
		if !commonmodel.UTF8Validation.IsValidMetricName(input.Name) {
			return fmt.Errorf("%s.name %q is not a valid UTF-8 Prometheus metric name", itemField, input.Name)
		}
		switch input.EffectiveType() {
		case commonmodel.MetricTypeGauge, commonmodel.MetricTypeCounter, commonmodel.MetricTypeUnknown:
		default:
			return fmt.Errorf("%s.type %q is not supported; use gauge, counter, or untyped", itemField, input.Type)
		}
		if previous, ok := typesByName[input.Name]; ok && previous != input.EffectiveType() {
			return fmt.Errorf("%s gives metric family %q more than one type", itemField, input.Name)
		}
		typesByName[input.Name] = input.EffectiveType()
		labelSet := promlabels.FromMap(input.Labels)
		for _, label := range labelSet {
			if label.Name == promlabels.MetricName || !commonmodel.UTF8Validation.IsValidLabelName(label.Name) {
				return fmt.Errorf("%s.labels contains invalid label name %q", itemField, label.Name)
			}
		}
		key := prompkg.IdentifyRawSample(input.Name, labelSet)
		if _, ok := seen[key]; ok {
			return fmt.Errorf("%s duplicates an earlier raw metric identity", itemField)
		}
		seen[key] = struct{}{}
	}
	return nil
}
