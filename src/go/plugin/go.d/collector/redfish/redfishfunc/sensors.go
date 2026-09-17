// SPDX-License-Identifier: GPL-3.0-or-later

package redfishfunc

import (
	"context"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/redfish/internal/measurement"
)

func sensorsTable(ctx context.Context, sensors []measurement.Sensor) *funcapi.FunctionResponse {
	columns := append(
		identityColumns(),
		textColumn("Sensor", "Sensor or component reading", true),
		textColumn("Type", "Measured quantity", true),
		funcapi.ColumnMeta{
			Name:          "Reading",
			Tooltip:       "Current value; blank when unavailable",
			Type:          funcapi.FieldTypeFloat,
			Visible:       true,
			Sortable:      true,
			Transform:     funcapi.FieldTransformNumber,
			DecimalPoints: 3,
			Filter:        funcapi.FieldFilterRange,
		},
		textColumn("Units", "Reading units", true),
		textColumn("Health", "BMC-reported health of this reading; not inherited component health", true),
		textColumn("Data availability", "Whether this reading has a usable value", true),
		observedColumn(),
		textColumn("Resource", "Resource supplying this reading", false),
		textColumn("Resource URI", "Source resource address", false),
		textColumn("Source property", "Source reading property", false),
		textColumn("Location", "BMC-reported physical context", false),
		textColumn("Source", "Whether this value was reported or calculated", false),
		rowOptionsColumn(),
	)
	columns[2].Sticky = true
	rows := make([][]any, 0, len(sensors))
	for _, sensor := range sensors {
		if ctx.Err() != nil {
			break
		}
		name := sensorName(sensor)
		var value any
		availability := "Unavailable"
		rank := healthRank(sensor.Health)
		if sensor.Valid {
			value, availability = sensor.Value, "Available"
		} else {
			rank = min(rank, 2)
		}
		source := "BMC"
		health := displayHealth(sensor.Health)
		if sensor.Calculated {
			source, health = "Calculated from energy", "Not applicable"
			if sensor.Valid {
				rank = 4
			}
		}
		rows = append(rows, []any{
			sensor.Key, sortKey(rank, name, sensor.Key), name, displayWords(sensor.Family), value, sensor.Units,
			health, availability, observationTime(sensor.ObservedAt), sensor.Resource, sensor.URI, sensor.SourcePath,
			optionalText(sensor.Location), source, rowOptions(rank),
		})
	}
	return tableResponse(columns, rows)
}

func sensorName(sensor measurement.Sensor) string {
	parts := strings.Split(sensor.SourcePath, ".")
	property := parts[len(parts)-1]
	if property == "Reading" && len(parts) > 1 {
		property = parts[len(parts)-2]
	}
	if property == "Sensor" || property == "Reading" {
		property = sensor.Family
	}
	// Polyphase properties must retain both the quantity and the phase name.
	if strings.HasPrefix(property, "Line") || property == "Neutral" {
		property = sensor.Role + " " + property
	}
	name := sensor.Resource + " — " + displayWords(property)
	if sensor.Basis != "" && sensor.Basis != "zero" {
		name += " (" + sensor.Basis + ")"
	}
	if sensor.Calculated {
		name = sensor.Resource + " — Power (calculated from energy; " + displayWords(property) + ")"
	}
	return name
}
