// SPDX-License-Identifier: GPL-3.0-or-later

package promvalidation

import (
	"bytes"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/netdata/netdata/go/plugins/internal/promprofile/input"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	commonmodel "github.com/prometheus/common/model"
	promlabels "github.com/prometheus/prometheus/model/labels"
)

const (
	maxFutureInputs      = 256
	futureInputValueBase = 1_000_000_000
)

type futureInput = prominput.FutureInput

func validateFutureInputs(inputs []futureInput) error {
	if len(inputs) > maxFutureInputs {
		return fmt.Errorf("future_inputs has %d entries; maximum is %d", len(inputs), maxFutureInputs)
	}
	return prominput.ValidateFutureInputs("future_inputs", inputs)
}

func encodeFutureInputs(inputs []futureInput) ([]byte, error) {
	ordered := slices.Clone(inputs)
	slices.SortFunc(ordered, func(a, b futureInput) int {
		if cmp := strings.Compare(a.Name, b.Name); cmp != 0 {
			return cmp
		}
		return strings.Compare(promlabels.FromMap(a.Labels).String(), promlabels.FromMap(b.Labels).String())
	})

	type encodedFamily struct {
		typeValue dto.MetricType
		metrics   []*dto.Metric
	}
	byName := make(map[string]encodedFamily)
	for index, input := range ordered {
		value := float64(futureInputValueBase + index)
		metric := dto.Metric{}
		typeValue := dto.MetricType_GAUGE
		switch input.EffectiveType() {
		case commonmodel.MetricTypeCounter:
			typeValue = dto.MetricType_COUNTER
			metric.Counter = &dto.Counter{Value: &value}
		case commonmodel.MetricTypeUnknown:
			typeValue = dto.MetricType_UNTYPED
			metric.Untyped = &dto.Untyped{Value: &value}
		default:
			metric.Gauge = &dto.Gauge{Value: &value}
		}
		for _, label := range promlabels.FromMap(input.Labels) {
			labelName, labelValue := label.Name, label.Value
			metric.Label = append(metric.Label, &dto.LabelPair{Name: &labelName, Value: &labelValue})
		}
		family := byName[input.Name]
		family.typeValue = typeValue
		family.metrics = append(family.metrics, &metric)
		byName[input.Name] = family
	}

	var output bytes.Buffer
	for _, name := range sortedStringKeys(byName) {
		familyName := name
		encoded := byName[name]
		family := dto.MetricFamily{Name: &familyName, Type: &encoded.typeValue, Metric: encoded.metrics}
		if _, err := expfmt.MetricFamilyToText(&output, &family); err != nil {
			return nil, fmt.Errorf("encode future input family %q: %w", name, err)
		}
	}
	return output.Bytes(), nil
}

func sortedStringKeys[T any](values map[string]T) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

func appendFutureInputs(exposition []byte, inputs []futureInput) ([]byte, error) {
	encoded, err := encodeFutureInputs(inputs)
	if err != nil {
		return nil, err
	}
	trimmed := bytes.TrimSpace(exposition)
	if before, ok := bytes.CutSuffix(trimmed, []byte("# EOF")); ok {
		trimmed = bytes.TrimSpace(before)
	}
	if len(trimmed) == 0 {
		return nil, errors.New("current exposition is empty")
	}
	combined := make([]byte, 0, len(trimmed)+1+len(encoded))
	combined = append(combined, trimmed...)
	combined = append(combined, '\n')
	combined = append(combined, encoded...)
	return combined, nil
}
