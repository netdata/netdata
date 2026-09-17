// SPDX-License-Identifier: GPL-3.0-or-later

package redfishfunc

import (
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
)

func textColumn(name, help string, visible bool) funcapi.ColumnMeta {
	return funcapi.ColumnMeta{
		Name:      name,
		Tooltip:   help,
		Type:      funcapi.FieldTypeString,
		Visible:   visible,
		Sortable:  true,
		Filter:    funcapi.FieldFilterMultiselect,
		Transform: funcapi.FieldTransformText,
	}
}

func observedColumn() funcapi.ColumnMeta {
	return funcapi.ColumnMeta{
		Name:      "Observed",
		Tooltip:   "Collection observation time; not the BMC's internal sampling time",
		Type:      funcapi.FieldTypeTimestamp,
		Visible:   true,
		Sortable:  true,
		Transform: funcapi.FieldTransformDatetime,
	}
}

func tableResponse(columns []funcapi.ColumnMeta, rows [][]any) *funcapi.FunctionResponse {
	// The hidden sort key includes attention, display name and stable identity.
	slices.SortFunc(rows, func(a, b []any) int { return strings.Compare(a[1].(string), b[1].(string)) })
	return &funcapi.FunctionResponse{
		Status: 200,
		Columns: funcapi.Columns(columns, func(c funcapi.ColumnMeta) funcapi.ColumnMeta { return c }).
			BuildColumns(),
		Data:              rows,
		DefaultSortColumn: "Sort",
	}
}

func identityColumns() []funcapi.ColumnMeta {
	return []funcapi.ColumnMeta{
		{Name: "Key", Tooltip: "Stable row identity", Type: funcapi.FieldTypeString, UniqueKey: true},
		{Name: "Sort", Tooltip: "Attention, name and identity ordering", Type: funcapi.FieldTypeString, Sortable: true},
	}
}

func rowOptionsColumn() funcapi.ColumnMeta {
	return funcapi.ColumnMeta{
		Name:          "rowOptions",
		Tooltip:       "Row presentation",
		Type:          funcapi.FieldTypeNone,
		Visualization: funcapi.FieldVisualRowOptions,
	}
}

func rowOptions(rank int) map[string]any {
	severity := "normal"
	switch rank {
	case 0:
		severity = "critical"
	case 1:
		severity = "warning"
	case 2, 3:
		severity = "notice"
	}
	return map[string]any{"severity": severity}
}

func healthRank(health string) int {
	switch strings.ToLower(strings.TrimSpace(health)) {
	case "critical":
		return 0
	case "warning":
		return 1
	case "ok":
		return 4
	default:
		return 3
	}
}

func displayHealth(health string) string {
	switch strings.ToLower(strings.TrimSpace(health)) {
	case "":
		return "Not reported"
	case "ok":
		return "OK"
	case "critical":
		return "Critical"
	case "warning":
		return "Warning"
	case "unknown":
		return "Unknown"
	default:
		return "Unknown: " + health
	}
}

func observationTime(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return value.UnixMilli()
}

func sortKey(rank int, name, key string) string {
	return fmt.Sprintf("%d\x00%s\x00%s", rank, strings.ToLower(name), key)
}

func optionalText(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func displayWords(value string) string {
	runes := []rune(strings.ReplaceAll(value, "_", " "))
	var b strings.Builder
	for i, r := range runes {
		if i > 0 && unicode.IsUpper(r) &&
			(unicode.IsLower(runes[i-1]) || unicode.IsDigit(runes[i-1]) ||
				(unicode.IsUpper(runes[i-1]) && i+1 < len(runes) && unicode.IsLower(runes[i+1]))) {
			b.WriteByte(' ')
		}
		if i == 0 {
			r = unicode.ToUpper(r)
		}
		b.WriteRune(r)
	}
	return strings.TrimSpace(b.String())
}

func componentAvailability(value string) string {
	switch value {
	case "readable":
		return "Available"
	case "unreadable":
		return "Unreadable"
	default:
		return "Unknown"
	}
}

func minReportedRank(current int, health string) int {
	if health == "" {
		return current
	}
	return min(current, healthRank(health))
}
