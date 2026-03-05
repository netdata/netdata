// SPDX-License-Identifier: GPL-3.0-or-later

package parity

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"gopkg.in/yaml.v3"
)

// GoldenVersion is the current parity golden schema version.
const GoldenVersion = "v1"

// GoldenDocument is the source-of-truth YAML structure for one scenario.
type GoldenDocument struct {
	Version      string            `yaml:"version" json:"version"`
	ScenarioID   string            `yaml:"scenario_id" json:"scenario_id"`
	Description  string            `yaml:"description,omitempty" json:"description,omitempty"`
	Devices      []GoldenDevice    `yaml:"devices" json:"devices"`
	Adjacencies  []GoldenAdjacency `yaml:"adjacencies" json:"adjacencies"`
	Expectations GoldenCounts      `yaml:"expectations" json:"expectations"`
}

// GoldenCounts stores aggregate assertions used by parity tests.
type GoldenCounts struct {
	DirectionalAdjacencies int `yaml:"directional_adjacencies" json:"directional_adjacencies"`
	BidirectionalPairs     int `yaml:"bidirectional_pairs" json:"bidirectional_pairs"`
	Devices                int `yaml:"devices" json:"devices"`
}

// GoldenDevice is one expected device in the scenario.
type GoldenDevice struct {
	ID       string `yaml:"id" json:"id"`
	Hostname string `yaml:"hostname" json:"hostname"`
}

// GoldenAdjacency is one expected directed adjacency observation.
type GoldenAdjacency struct {
	Protocol     string `yaml:"protocol" json:"protocol"`
	SourceDevice string `yaml:"source_device" json:"source_device"`
	SourcePort   string `yaml:"source_port" json:"source_port"`
	TargetDevice string `yaml:"target_device" json:"target_device"`
	TargetPort   string `yaml:"target_port" json:"target_port"`
}

// LoadGoldenYAML loads and validates a golden YAML source file.
func LoadGoldenYAML(path string) (GoldenDocument, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return GoldenDocument{}, fmt.Errorf("read golden yaml %q: %w", path, err)
	}
	var doc GoldenDocument
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return GoldenDocument{}, fmt.Errorf("decode golden yaml %q: %w", path, err)
	}
	if err := doc.validate(); err != nil {
		return GoldenDocument{}, fmt.Errorf("validate golden yaml %q: %w", path, err)
	}
	return doc.Canonical(), nil
}

// LoadGoldenJSON loads a generated canonical JSON cache.
func LoadGoldenJSON(path string) (GoldenDocument, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return GoldenDocument{}, fmt.Errorf("read golden json %q: %w", path, err)
	}
	var doc GoldenDocument
	if err := json.Unmarshal(data, &doc); err != nil {
		return GoldenDocument{}, fmt.Errorf("decode golden json %q: %w", path, err)
	}
	if err := doc.validate(); err != nil {
		return GoldenDocument{}, fmt.Errorf("validate golden json %q: %w", path, err)
	}
	return doc.Canonical(), nil
}

// CanonicalJSON marshals a deterministic canonical JSON representation.
func (d GoldenDocument) CanonicalJSON() ([]byte, error) {
	c := d.Canonical()
	out, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(out, '\n'), nil
}

// Canonical returns a deterministic ordering for slices in the document.
func (d GoldenDocument) Canonical() GoldenDocument {
	out := d
	out.Devices = append([]GoldenDevice(nil), d.Devices...)
	out.Adjacencies = append([]GoldenAdjacency(nil), d.Adjacencies...)

	sort.Slice(out.Devices, func(i, j int) bool {
		if out.Devices[i].ID != out.Devices[j].ID {
			return out.Devices[i].ID < out.Devices[j].ID
		}
		return out.Devices[i].Hostname < out.Devices[j].Hostname
	})

	sort.Slice(out.Adjacencies, func(i, j int) bool {
		ai := out.Adjacencies[i]
		aj := out.Adjacencies[j]
		if ai.Protocol != aj.Protocol {
			return ai.Protocol < aj.Protocol
		}
		if ai.SourceDevice != aj.SourceDevice {
			return ai.SourceDevice < aj.SourceDevice
		}
		if ai.SourcePort != aj.SourcePort {
			return ai.SourcePort < aj.SourcePort
		}
		if ai.TargetDevice != aj.TargetDevice {
			return ai.TargetDevice < aj.TargetDevice
		}
		return ai.TargetPort < aj.TargetPort
	})
	return out
}

// ValidateCache compares authored YAML against generated JSON cache.
func ValidateCache(yamlPath, jsonPath string) error {
	yamlDoc, err := LoadGoldenYAML(yamlPath)
	if err != nil {
		return err
	}
	jsonDoc, err := LoadGoldenJSON(jsonPath)
	if err != nil {
		return err
	}
	yamlJSON, err := yamlDoc.CanonicalJSON()
	if err != nil {
		return fmt.Errorf("marshal canonical yaml document: %w", err)
	}
	jsonJSON, err := jsonDoc.CanonicalJSON()
	if err != nil {
		return fmt.Errorf("marshal canonical json document: %w", err)
	}
	if !bytes.Equal(yamlJSON, jsonJSON) {
		return fmt.Errorf("golden cache mismatch between %q and %q", yamlPath, jsonPath)
	}
	return nil
}

func (d GoldenDocument) validate() error {
	if d.Version == "" {
		return fmt.Errorf("version is required")
	}
	if d.Version != GoldenVersion {
		return fmt.Errorf("unsupported version %q (want %q)", d.Version, GoldenVersion)
	}
	if d.ScenarioID == "" {
		return fmt.Errorf("scenario_id is required")
	}
	if len(d.Devices) == 0 {
		return fmt.Errorf("at least one device is required")
	}

	seenDevices := make(map[string]struct{}, len(d.Devices))
	for _, dev := range d.Devices {
		if dev.ID == "" {
			return fmt.Errorf("device id is required")
		}
		if _, ok := seenDevices[dev.ID]; ok {
			return fmt.Errorf("duplicate device id %q", dev.ID)
		}
		seenDevices[dev.ID] = struct{}{}
	}

	for _, adj := range d.Adjacencies {
		if adj.Protocol == "" {
			return fmt.Errorf("adjacency protocol is required")
		}
		if _, ok := seenDevices[adj.SourceDevice]; !ok {
			return fmt.Errorf("adjacency source_device %q is not in devices", adj.SourceDevice)
		}
		if _, ok := seenDevices[adj.TargetDevice]; !ok {
			return fmt.Errorf("adjacency target_device %q is not in devices", adj.TargetDevice)
		}
	}

	if d.Expectations.DirectionalAdjacencies != len(d.Adjacencies) {
		return fmt.Errorf("expectations.directional_adjacencies=%d does not match adjacencies=%d", d.Expectations.DirectionalAdjacencies, len(d.Adjacencies))
	}
	if d.Expectations.Devices != len(d.Devices) {
		return fmt.Errorf("expectations.devices=%d does not match devices=%d", d.Expectations.Devices, len(d.Devices))
	}

	return nil
}
