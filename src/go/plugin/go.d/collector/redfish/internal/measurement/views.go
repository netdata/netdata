// SPDX-License-Identifier: GPL-3.0-or-later

package measurement

import (
	"cmp"
	"strings"
	"time"
)

// Component and Sensor contain copied facts and evaluated results for current-state views. They
// never retain acquired documents or measurement history. Consumers treat them as immutable.
type Component struct {
	Key                       string
	Kind                      string
	Name                      string
	URI                       string
	Health                    string
	HealthRollup              string
	State                     string
	PowerState                string
	FailurePredicted          bool
	FailurePredictionReported bool
	Conditions                []ReportedCondition
	Availability              string
	ObservedAt                time.Time
	Manufacturer              string
	Model                     string
	SerialNumber              string
	PartNumber                string
	AssetTag                  string
	Firmware                  string
	Location                  string
	TotalCores                *float64
	EnabledCores              *float64
	TotalThreads              *float64
	CapacityBytes             *float64
	MemoryType                string
	MediaType                 string
	DriveProtocol             string
	RAIDType                  string
	ReleaseDate               time.Time
	HardwareVersion           string
	EngineeringRevision       string
}

type ReportedCondition struct {
	Message, Severity string
}

type Sensor struct {
	Key           string
	Resource      string
	URI           string
	Family        string
	Units         string
	Basis         string
	Role          string
	SourcePath    string
	Health        string
	DerivedHealth string
	Location      string
	Value         float64
	Valid         bool
	Calculated    bool
	ObservedAt    time.Time
}

func componentView(node *Resource, observedAt time.Time) Component {
	v := Component{
		Key:          node.Key,
		Kind:         node.Kind,
		Name:         cmp.Or(node.Doc.Name, node.Doc.ID, node.URI),
		URI:          node.URI,
		Availability: cmp.Or(node.AcquisitionState, "unknown"),
	}
	if node.Data == nil || v.Availability != "readable" {
		return v
	}
	v.ObservedAt = observedAt
	v.Health = sourceState(node.Data, "Status.Health")
	v.HealthRollup = sourceState(node.Data, "Status.HealthRollup")
	v.State = sourceState(node.Data, "Status.State")
	v.PowerState = sourceState(node.Data, "PowerState")
	if raw, ok := Properties(node.Data).Lookup("FailurePredicted"); ok {
		v.FailurePredicted, v.FailurePredictionReported = raw.(bool)
	}
	for _, condition := range node.Doc.Status.Conditions {
		severity, _ := normalizedConditionSeverity(condition.Severity)
		v.Conditions = append(v.Conditions, ReportedCondition{
			Message:  cmp.Or(condition.Message, condition.MessageID),
			Severity: severity,
		})
	}
	// Identity fields are shown in full here; chart labels get a subset.
	addResourceIdentity(func(key, value string) {
		switch key {
		case "manufacturer":
			v.Manufacturer = value
		case "model":
			v.Model = value
		case "serial_number":
			v.SerialNumber = value
		case "part_number":
			v.PartNumber = value
		case "asset_tag":
			v.AssetTag = value
		case "firmware_version":
			v.Firmware = value
		case "bios_version":
			v.Firmware = cmp.Or(v.Firmware, value)
		case "slot":
			v.Location = value
		case "location":
			v.Location = value
		}
	}, node)
	v.addInventory(node)
	return v
}

func sensorView(node *Resource, reading normalizedReading, observedAt time.Time) Sensor {
	return Sensor{
		Key:           reading.Key,
		Resource:      cmp.Or(node.Doc.Name, node.Doc.ID, node.URI),
		URI:           node.URI,
		Family:        reading.Family,
		Units:         reading.Units,
		Basis:         reading.Basis,
		Role:          reading.Role,
		SourcePath:    reading.SourcePath,
		Health:        reading.Health,
		DerivedHealth: reading.DerivedHealth,
		Location:      strings.TrimSpace(reading.PhysicalContext + " " + reading.PhysicalSubcontext),
		Value:         reading.Value,
		Valid:         reading.Valid,
		Calculated:    reading.SemanticSourceClass == "energy_rate",
		ObservedAt:    observedAt,
	}
}

func sourceState(data map[string]any, path string) string {
	raw, present := Properties(data).Lookup(path)
	if !present || raw == nil {
		return ""
	}
	if value, ok := raw.(string); ok {
		return strings.TrimSpace(value)
	}
	return "Unknown"
}
