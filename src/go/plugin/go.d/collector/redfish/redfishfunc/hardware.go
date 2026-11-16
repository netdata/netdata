// SPDX-License-Identifier: GPL-3.0-or-later

package redfishfunc

import (
	"context"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/redfish/internal/measurement"
)

func hardwareTable(ctx context.Context, components []measurement.Component) *funcapi.FunctionResponse {
	columns := append(
		identityColumns(),
		textColumn("Component", "Component or logical resource name", true),
		textColumn("Type", "Resource kind", true),
		textColumn("Health", "BMC-reported health of this component", true),
		textColumn("State", "BMC-reported operational state", true),
		textColumn("Location", "BMC-reported location or slot", true),
		textColumn("Reported issue", "BMC conditions, rollup warnings or predicted failure", true),
		textColumn("Data availability", "Whether this component was read in this collection", true),
		observedColumn(),
		textColumn("Manufacturer", "Component manufacturer", false),
		textColumn("Model", "Component model", false),
		textColumn("Serial number", "Component serial number", false),
		textColumn("Part number", "Component part number", false),
		textColumn("Asset tag", "Component asset tag", false),
		textColumn("Firmware", "Firmware, software or BIOS version", false),
		textColumn("Power state", "BMC-reported power state", false),
		textColumn("Health rollup", "BMC-reported aggregate health, separate from component health", false),
		funcapi.ColumnMeta{
			Name:     "Failure predicted",
			Tooltip:  "BMC predicts component failure",
			Type:     funcapi.FieldTypeBoolean,
			Sortable: true,
			Filter:   funcapi.FieldFilterMultiselect,
		},
		textColumn("Resource URI", "Source resource address", false),
		inventoryNumberColumn("Processor cores", "Total cores in this processor", "cores"),
		inventoryNumberColumn("Enabled cores", "Enabled cores in this processor", "cores"),
		inventoryNumberColumn("Supported threads", "Execution threads supported by this processor", "threads"),
		inventoryNumberColumn("Capacity", "Capacity of this memory device, physical drive or logical volume", "bytes"),
		textColumn("Memory type", "Memory device technology, such as DDR4 or DDR5", false),
		textColumn("Media type", "Drive media type, such as SSD or HDD", false),
		textColumn("Drive protocol", "Protocol used by this drive, such as SAS, SATA or NVMe", false),
		textColumn("RAID type", "RAID configuration of this logical volume", false),
		funcapi.ColumnMeta{
			Name:      "Release date",
			Tooltip:   "Firmware or software release/production date; not installation time",
			Type:      funcapi.FieldTypeTimestamp,
			Sortable:  true,
			Transform: funcapi.FieldTransformDatetime,
		},
		textColumn("Hardware version", "Vendor-reported hardware version of this assembly", false),
		textColumn("Engineering revision", "Engineering change level or revision of this assembly", false),
		rowOptionsColumn(),
	)
	columns[2].Sticky = true
	columns[7].Wrap = true
	rows := make([][]any, 0, len(components))
	for _, component := range components {
		if ctx.Err() != nil {
			break
		}
		issue, rank := componentIssue(component)
		var failure any
		if component.FailurePredictionReported {
			failure = component.FailurePredicted
		}
		rows = append(rows, []any{
			component.Key,
			sortKey(rank, component.Name, component.Key),
			component.Name,
			displayWords(component.Kind),
			displayHealth(component.Health),
			optionalText(component.State),
			optionalText(component.Location),
			optionalText(issue),
			componentAvailability(component.Availability),
			observationTime(component.ObservedAt),
			optionalText(component.Manufacturer),
			optionalText(component.Model),
			optionalText(component.SerialNumber),
			optionalText(component.PartNumber),
			optionalText(component.AssetTag),
			optionalText(component.Firmware),
			optionalText(component.PowerState),
			displayHealth(component.HealthRollup),
			failure,
			component.URI,
			optionalNumber(component.TotalCores),
			optionalNumber(component.EnabledCores),
			optionalNumber(component.TotalThreads),
			optionalNumber(component.CapacityBytes),
			optionalText(component.MemoryType),
			optionalText(component.MediaType),
			optionalText(component.DriveProtocol),
			optionalText(component.RAIDType),
			observationTime(component.ReleaseDate),
			optionalText(component.HardwareVersion),
			optionalText(component.EngineeringRevision),
			rowOptions(rank),
		})
	}
	return tableResponse(columns, rows)
}

func inventoryNumberColumn(name, help, units string) funcapi.ColumnMeta {
	return funcapi.ColumnMeta{
		Name:      name,
		Tooltip:   help,
		Type:      funcapi.FieldTypeInteger,
		Units:     units,
		Sortable:  true,
		Filter:    funcapi.FieldFilterRange,
		Transform: funcapi.FieldTransformNumber,
	}
}

func optionalNumber(value *float64) any {
	if value == nil {
		return nil
	}
	return *value
}

func componentIssue(component measurement.Component) (string, int) {
	rank := healthRank(component.Health)
	var issues []string
	if component.HealthRollup != "" {
		rank = minReportedRank(rank, component.HealthRollup)
		if healthRank(component.HealthRollup) < 4 {
			issues = append(issues, "Health rollup: "+displayHealth(component.HealthRollup))
		}
	}
	if component.FailurePredictionReported && component.FailurePredicted {
		rank = min(rank, 1)
		issues = append(issues, "Failure predicted")
	}
	for _, condition := range component.Conditions {
		rank = minReportedRank(rank, condition.Severity)
		message := strings.TrimSpace(condition.Message)
		if message == "" {
			if condition.Severity == "" {
				continue
			}
			message = "Condition: " + displayHealth(condition.Severity)
		} else if condition.Severity != "" {
			message = displayHealth(condition.Severity) + ": " + message
		}
		issues = append(issues, message)
	}
	if component.Availability != "readable" {
		rank = min(rank, 2)
	}
	return strings.Join(issues, "; "), rank
}
