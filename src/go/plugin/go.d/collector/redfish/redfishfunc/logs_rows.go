// SPDX-License-Identifier: GPL-3.0-or-later

package redfishfunc

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/redfish/internal/acquisition"
)

type logRow struct {
	entry           acquisition.LogEntry
	timestamp       time.Time
	basis, severity string
}

func logsTable(ctx context.Context, entries []acquisition.LogEntry, query logQuery) *funcapi.FunctionResponse {
	selected := make([]logRow, 0, len(entries))
	undated := 0
	for _, entry := range entries {
		if ctx.Err() != nil {
			break
		}
		row := logRow{
			entry:    entry,
			severity: logSeverity(entry.Severity),
		}
		row.timestamp, row.basis = logTime(entry)
		if !row.timestamp.IsZero() {
			seconds := row.timestamp.Unix()
			if query.after != nil && seconds < *query.after || query.before != nil && seconds > *query.before {
				continue
			}
		}
		if query.severity != "" && query.severity != "all" && query.severity != row.severity {
			continue
		}
		if query.text != nil && !query.text.MatchString(strings.Join([]string{
			entry.Message, entry.ID, entry.URI, entry.MessageID, entry.EntryType, entry.Severity, query.service,
		}, " ")) {
			continue
		}
		if row.timestamp.IsZero() {
			undated++
		}
		selected = append(selected, row)
	}
	slices.SortStableFunc(selected, func(a, b logRow) int {
		if a.timestamp.IsZero() != b.timestamp.IsZero() {
			if a.timestamp.IsZero() {
				return 1
			}
			return -1
		}
		if n := b.timestamp.Compare(a.timestamp); n != 0 {
			return n
		}
		return cmp.Or(strings.Compare(a.entry.URI, b.entry.URI), strings.Compare(a.entry.ID, b.entry.ID))
	})
	rows := make([][]any, 0, len(selected))
	for index, row := range selected {
		e := row.entry
		rows = append(rows, []any{index, observationTime(row.timestamp), row.severity, optionalText(e.Message),
			optionalText(
				e.ID,
			), optionalText(e.EntryType), query.service, optionalText(e.URI), optionalText(e.MessageID),
			optionalText(
				e.EventTimestamp,
			), optionalText(e.Created), row.basis, optionalText(e.Severity), rowOptions(healthRank(row.severity))})
	}
	help := fmt.Sprintf(
		"%s Returned %d matching entries from %d read. Times use EventTimestamp, falling back to Created. Text search uses case-sensitive, ordered simple patterns against the combined message and identity fields (for example *failure*; exclusions need a following positive pattern).",
		logsHelp,
		len(rows),
		len(entries),
	)
	if undated > 0 {
		help += fmt.Sprintf(
			" %d entries have unavailable time and remain visible because the time range cannot filter them.",
			undated,
		)
	}
	return &funcapi.FunctionResponse{
		Status:            200,
		Help:              help,
		Columns:           logColumns(),
		Data:              rows,
		DefaultSortColumn: "Order",
	}
}

func logTime(e acquisition.LogEntry) (time.Time, string) {
	for _, candidate := range []struct{ value, basis string }{{e.EventTimestamp, "EventTimestamp"}, {e.Created, "Created"}} {
		if parsed, err := time.Parse(time.RFC3339Nano, candidate.value); err == nil && !parsed.IsZero() {
			return parsed, candidate.basis
		}
	}
	return time.Time{}, "Unavailable"
}

func logSeverity(value string) string {
	switch value {
	case "OK", "Warning", "Critical":
		return value
	default:
		return "Unknown"
	}
}

func logColumns() map[string]any {
	columns := []funcapi.ColumnMeta{
		{
			Name:      "Order",
			Tooltip:   "Newest first, then undated entries; row position in this query",
			Type:      funcapi.FieldTypeInteger,
			UniqueKey: true,
			Sortable:  true,
		},
		{
			Name:      "Time",
			Tooltip:   "EventTimestamp, with valid Created fallback",
			Type:      funcapi.FieldTypeTimestamp,
			Visible:   true,
			Sortable:  true,
			Sticky:    true,
			Transform: funcapi.FieldTransformDatetime,
		},
		textColumn("Severity", "Normalized BMC severity; missing and unrecognized values are Unknown", true),
		textColumn("Message", "BMC event message", true),
		textColumn("Entry ID", "Identifier assigned by this log service", true),
		textColumn("Entry type", "BMC event format", false),
		textColumn("Log service", "Selected source resource", false),
		textColumn("Entry URI", "Source entry resource", false),
		textColumn("Message ID", "BMC message registry identifier", false),
		textColumn("EventTimestamp", "Original BMC event time, including invalid values", false),
		textColumn("Created", "Original BMC entry creation time, including invalid values", false),
		textColumn("Time basis", "Which BMC timestamp supplies Time", false),
		textColumn("Reported severity", "Original BMC severity", false),
		rowOptionsColumn(),
	}
	columns[3].FullWidth, columns[3].Wrap = true, true
	result := funcapi.Columns(columns, func(c funcapi.ColumnMeta) funcapi.ColumnMeta { return c }).BuildColumns()
	timeColumn := result["Time"].(map[string]any)
	timeColumn["value_options"].(map[string]any)["default_value"] = "Unavailable"
	return result
}
