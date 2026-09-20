// SPDX-License-Identifier: GPL-3.0-or-later

package smbiosfunc

import (
	"context"
	"fmt"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/smbios_memory/internal/inventory"
)

type Deps interface{ CurrentSnapshot() *inventory.Snapshot }
type router struct{ deps Deps }

func NewRouter(deps Deps) funcapi.MethodHandler {
	return &router{
		deps: deps,
	}
}
func Methods(updateEvery int) []funcapi.FunctionConfig {
	return []funcapi.FunctionConfig{{ID: "inventory", Name: "Memory Inventory", UpdateEvery: updateEvery,
		Help: "Boot-time SMBIOS physical system memory inventory and accepted baseline comparison", ResponseType: "table"}}
}
func (*router) MethodParams(_ context.Context, method string) ([]funcapi.ParamConfig, error) {
	if method != "inventory" {
		return nil, fmt.Errorf("unknown method: %s", method)
	}
	return nil, nil
}
func (*router) Cleanup(context.Context) {}
func (r *router) Handle(ctx context.Context, method string, _ funcapi.ResolvedParams) *funcapi.FunctionResponse {
	if method != "inventory" {
		return funcapi.NotFoundResponse(method)
	}
	if ctx.Err() != nil {
		return funcapi.ErrorResponse(499, "Memory inventory request canceled")
	}
	s := r.deps.CurrentSnapshot()
	if s == nil {
		return funcapi.UnavailableResponse("Waiting for the first memory inventory collection")
	}
	columns := []funcapi.ColumnMeta{
		{
			Name:      "Key",
			Tooltip:   "Identity within this table snapshot, not a persistent hardware identity",
			Type:      funcapi.FieldTypeString,
			UniqueKey: true,
		},
		textColumn("Slot", "Firmware device locator; unique nonempty locators identify comparable slots", true),
		textColumn("Bank", "Firmware bank locator", false),
		textColumn("Population", "Firmware socket population; unknown for baseline-only rows", true),
		numberColumn("Capacity", "Current firmware-reported physical capacity", "bytes", true),
		numberColumn("Accepted capacity", "Capacity of the populated slot in the durable baseline", "bytes", true),
		textColumn("Comparison", "Comparison with the accepted baseline; not memory health", true),
		textColumn("Data availability", "Whether this row was read or retained from the baseline", true),
		textColumn("Memory type", "Firmware-reported memory technology", true),
		textColumn("Form factor", "Firmware-reported form factor; not every device is removable", false),
		textColumn("Manufacturer", "Current device manufacturer, or accepted device for baseline-only rows", false),
		textColumn("Part number", "Current device part, or accepted device for baseline-only rows", false),
		textColumn("Serial number", "Current device serial, or accepted device for baseline-only rows", false),
		numberColumn("Ranks", "Number of ranks in the memory device", "ranks", false),
		numberColumn("Rated speed", "Maximum capable transfer rate reported by firmware", "MT/s", true),
		numberColumn("Configured speed", "Transfer rate configured at boot; not a live frequency", "MT/s", true),
		textColumn("ECC association", "No verified mapping from EDAC ranks to physical devices is available", false),
	}
	columns = append(
		columns,
		funcapi.ColumnMeta{
			Name:      "Read attempt",
			Tooltip:   "When Netdata last attempted to read the boot-time firmware table",
			Type:      funcapi.FieldTypeTimestamp,
			Sortable:  true,
			Transform: funcapi.FieldTransformDatetime,
		},
		textColumn("Inventory status", "Availability of the current firmware table", false),
		funcapi.ColumnMeta{
			Name:      "Loss recorded",
			Tooltip:   "When the retained loss evidence last changed; blank if no retained loss",
			Type:      funcapi.FieldTypeTimestamp,
			Sortable:  true,
			Transform: funcapi.FieldTransformDatetime,
		},
	)
	rows := make([][]any, 0, len(s.Rows))
	for i, row := range s.Rows {
		d := row.Device
		rows = append(
			rows,
			[]any{
				fmt.Sprintf("%d:%04x", i, d.Handle),
				nullable(d.Locator),
				nullable(d.Bank),
				d.Population,
				number(d.Capacity),
				number(row.BaselineCapacity),
				row.Comparison,
				row.Availability,
				nullable(d.MemoryType),
				nullable(d.FormFactor),
				nullable(d.Manufacturer),
				nullable(d.Part),
				nullable(d.Serial),
				number(d.Ranks),
				number(d.RatedSpeed),
				number(d.ConfiguredSpeed),
				"unavailable",
				s.ReadAt.UnixMilli(), s.InventoryStatus, timestamp(s.LossAt),
			},
		)
	}
	help := fmt.Sprintf(
		"Boot-time firmware inventory, not live memory health. Last read attempt: %s. Inventory: %s. Comparison: %s.",
		s.ReadAt.UTC().Format(time.RFC3339),
		s.InventoryStatus,
		s.ComparisonStatus,
	)
	if !s.LossAt.IsZero() {
		help += " Retained loss evidence recorded at " + s.LossAt.UTC().Format(time.RFC3339) + "."
	}
	if s.Detail != "" {
		help += " " + s.Detail
	}
	return &funcapi.FunctionResponse{
		Status: 200,
		Columns: funcapi.Columns(columns, func(c funcapi.ColumnMeta) funcapi.ColumnMeta { return c }).
			BuildColumns(),
		Data:              rows,
		DefaultSortColumn: "Slot",
		Help:              help,
	}
}
func textColumn(name, help string, visible bool) funcapi.ColumnMeta {
	return funcapi.ColumnMeta{
		Name:     name,
		Tooltip:  help,
		Type:     funcapi.FieldTypeString,
		Sortable: true,
		Visible:  visible,
		Filter:   funcapi.FieldFilterMultiselect,
	}
}
func numberColumn(name, help, units string, visible bool) funcapi.ColumnMeta {
	return funcapi.ColumnMeta{
		Name:      name,
		Tooltip:   help,
		Type:      funcapi.FieldTypeInteger,
		Sortable:  true,
		Visible:   visible,
		Units:     units,
		Filter:    funcapi.FieldFilterRange,
		Transform: funcapi.FieldTransformNumber,
	}
}
func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}
func number(n *uint64) any {
	if n == nil {
		return nil
	}
	return *n
}

func timestamp(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t.UnixMilli()
}
