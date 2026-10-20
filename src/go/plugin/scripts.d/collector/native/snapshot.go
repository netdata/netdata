// SPDX-License-Identifier: GPL-3.0-or-later

package native

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/scripts.d/collector/native/internal/strictjson"
)

// A snapshot is the complete current observation, including family metadata.
type snapshot struct {
	Version string         `json:"version"`
	Metrics []metricFamily `json:"metrics"`
	Checks  []checkFamily  `json:"checks"`
}

type metricContract struct {
	Type      string        `json:"type,omitempty"`
	Unit      string        `json:"unit,omitempty"`
	Mode      string        `json:"mode,omitempty"`
	States    []string      `json:"states,omitempty"`
	ChartMeta chartMetadata `json:"chart_meta,omitempty"`
}

type chartMetadata struct {
	Title    string `json:"title,omitempty"`
	Family   string `json:"family,omitempty"`
	Priority *int   `json:"priority,omitempty"`
}

type metricFamily struct {
	metricContract
	Name    string         `json:"name"`
	Samples []metricSample `json:"samples"`
}

type metricSample struct {
	Value  *float64          `json:"value,omitempty"`
	Active []string          `json:"active,omitempty"`
	Labels map[string]string `json:"labels,omitempty"`
}

type checkDefinition struct {
	ID       string   `json:"id"`
	Title    string   `json:"title"`
	ByLabels []string `json:"by_labels,omitempty"`
}

type checkFamily struct {
	checkDefinition
	Samples []checkSample `json:"samples"`
}

type checkSample struct {
	State  string            `json:"state"`
	Labels map[string]string `json:"labels,omitempty"`
}

var (
	labelsShape   = strictjson.Map(strictjson.Scalar())
	snapshotShape = strictjson.Object(strictjson.Fields{
		"metrics": strictjson.Object(strictjson.Fields{
			"chart_meta": strictjson.Object(nil, "title", "family", "priority"),
			"samples": strictjson.Object(strictjson.Fields{
				"labels": labelsShape,
			}, "value", "active"),
		}, "name", "type", "unit", "mode", "states"),
		"checks": strictjson.Object(strictjson.Fields{
			"samples": strictjson.Object(strictjson.Fields{
				"labels": labelsShape,
			}, "state"),
		}, "id", "title", "by_labels"),
	}, "version")
)

// decodeSnapshot normalizes and validates every family before any store writes.
func decodeSnapshot(data []byte) (snapshot, error) {
	var result snapshot
	if err := strictjson.Decode(data, snapshotShape, &result); err != nil {
		return result, err
	}
	return result, result.validate()
}

// validate is shared by every snapshot encoding, before any store writes.
func (result *snapshot) validate() error {
	if result.Version != "v1" {
		return errors.New("unsupported response version")
	}
	seen := make(map[string]bool, len(result.Metrics))
	for i := range result.Metrics {
		f := &result.Metrics[i]
		if seen[f.Name] {
			return errors.New("duplicate metric family")
		}
		seen[f.Name] = true
		if err := f.validate(); err != nil {
			return fmt.Errorf("metric family %d: %w", i, err)
		}
	}
	clear(seen)
	for i := range result.Checks {
		f := &result.Checks[i]
		if seen[f.ID] {
			return errors.New("duplicate check family")
		}
		seen[f.ID] = true
		if err := f.validate(); err != nil {
			return fmt.Errorf("check family %d: %w", i, err)
		}
	}
	return nil
}

func (f *metricFamily) validate() error {
	if !reMetricName.MatchString(f.Name) || strings.HasPrefix(f.Name, reservedMetricPrefix) ||
		strings.HasPrefix(f.Name, checkChartIDPrefix) || f.Name == "check_state" {
		return errors.New("invalid or reserved metric name")
	}
	if f.Type == "" {
		f.Type = metricGauge
	}
	if f.Samples == nil {
		return errors.New("samples array is required")
	}
	if f.ChartMeta.Priority != nil && *f.ChartMeta.Priority <= 0 {
		return errors.New("chart priority must be positive")
	}
	var domain map[string]bool
	switch f.Type {
	case metricGauge, metricCounter:
		if f.States != nil || f.Mode != "" {
			return errors.New("scalar metrics cannot declare states or mode")
		}
		if f.Unit == "" {
			f.Unit = "value"
		}
		if strings.TrimSpace(f.Unit) == "" {
			return errors.New("unit must not be blank")
		}
	case metricStateSet:
		if f.Unit != "" {
			return errors.New("stateset units are fixed to state")
		}
		if f.Mode == "" {
			f.Mode = "enum"
		}
		if f.Mode != "enum" && f.Mode != "bitset" {
			return errors.New("stateset mode must be enum or bitset")
		}
		if len(f.States) == 0 {
			return errors.New("stateset requires states")
		}
		domain = make(map[string]bool, len(f.States))
		for _, state := range f.States {
			// Autogen uses states as dimension IDs. Require names unchanged by
			// chartemit's wire-ID sanitation so distinct states cannot collapse.
			if state == "" || strings.TrimSpace(state) != state || strings.ContainsAny(state, "'\\\n\r\x00") {
				return errors.New(
					"states must be nonempty, without surrounding whitespace, apostrophes, backslashes, newlines, carriage returns or NUL",
				)
			}
			if domain[state] {
				return errors.New("states must be unique")
			}
			domain[state] = true
		}
		slices.Sort(f.States)
	default:
		return errors.New("type must be gauge, counter or stateset")
	}
	seen := make(map[string]bool, len(f.Samples))
	for i, sample := range f.Samples {
		if err := validateLabelKeys(sample.Labels); err != nil {
			return err
		}
		if _, collision := sample.Labels[f.Name]; collision && f.Type == metricStateSet {
			return errors.New("stateset name is reserved as its state label")
		}
		if f.Type == metricStateSet {
			if sample.Value != nil || sample.Active == nil || (f.Mode == "enum" && len(sample.Active) != 1) {
				return fmt.Errorf("sample %d requires active states for its mode", i)
			}
			active := make(map[string]bool, len(sample.Active))
			for _, state := range sample.Active {
				if !domain[state] || active[state] {
					return fmt.Errorf("sample %d has unknown or duplicate active states", i)
				}
				active[state] = true
			}
		} else if sample.Active != nil || sample.Value == nil || math.IsNaN(*sample.Value) || math.IsInf(*sample.Value, 0) ||
			(f.Type == metricCounter && *sample.Value < 0) {
			return fmt.Errorf("sample %d has an invalid scalar value", i)
		}
		key := seriesIdentity(sample.Labels)
		if seen[key] {
			return errors.New("duplicate metric identity")
		}
		seen[key] = true
	}
	return nil
}

func (f *checkFamily) validate() error {
	if !reIdentifier.MatchString(f.ID) || strings.TrimSpace(f.Title) == "" {
		return errors.New("check requires a valid id and title")
	}
	if f.Samples == nil {
		return errors.New("samples array is required")
	}
	keys := make(map[string]bool, len(f.ByLabels))
	for _, key := range f.ByLabels {
		if !reIdentifier.MatchString(key) || key == "_collect_job" || keys[key] {
			return errors.New("invalid or duplicate identity label")
		}
		keys[key] = true
	}
	seen := make(map[string]bool, len(f.Samples))
	for i := range f.Samples {
		sample := &f.Samples[i]
		if !slices.Contains(checkStates, sample.State) {
			return fmt.Errorf("sample %d has an invalid state", i)
		}
		if err := validateLabelKeys(sample.Labels); err != nil {
			return err
		}
		labels := make(map[string]string, len(f.ByLabels))
		for _, key := range f.ByLabels {
			if strings.TrimSpace(sample.Labels[key]) == "" {
				return fmt.Errorf("sample %d is missing an identity label", i)
			}
			labels[key] = sample.Labels[key]
		}
		key := seriesIdentity(labels)
		if seen[key] {
			return errors.New("duplicate check identity")
		}
		seen[key] = true
		// Only identity labels belong to a built-in check chart or its series.
		sample.Labels = labels
	}
	return nil
}

func validateLabelKeys(labels map[string]string) error {
	for key := range labels {
		if !reIdentifier.MatchString(key) || key == "_collect_job" {
			return errors.New("invalid or reserved label key")
		}
	}
	return nil
}

func seriesIdentity(labels map[string]string) string {
	if len(labels) == 0 {
		return "{}"
	}
	// JSON sorts keys and escapes delimiters, giving an unambiguous identity.
	encoded, _ := json.Marshal(labels)
	return string(encoded)
}
