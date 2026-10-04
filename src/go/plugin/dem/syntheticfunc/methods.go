// SPDX-License-Identifier: GPL-3.0-or-later

package syntheticfunc

import (
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
)

type method struct {
	id, title, help, sort string
	params                []string
	history               bool
	columns               map[string]any
}

type column struct {
	id, title string
	kind      funcapi.FieldType
	units     string
}

func columns(fields ...column) map[string]any {
	out := make(map[string]any, len(fields))
	for i, f := range fields {
		transform := funcapi.FieldTransformNone
		if f.kind == funcapi.FieldTypeTimestamp {
			transform = funcapi.FieldTransformDatetimeUsec
			if strings.HasSuffix(f.id, "_ms") {
				transform = funcapi.FieldTransformDatetime
			}
		}
		out[f.id] = (funcapi.Column{
			ValueOptions: funcapi.ValueOptions{
				Transform: transform,
			},
			Index:         i,
			Name:          f.title,
			Type:          f.kind,
			Units:         f.units,
			Visualization: funcapi.FieldVisualValue,
			Visible:       true,
			Sortable:      true,
		}).BuildColumn()
	}
	return out
}

var methods = []method{
	{
		id:     "synthetics-checks",
		title:  "Synthetic monitors",
		help:   "Active native monitors and their latest observation. Disabled and failed-startup configuration remains in native job status. Freshness belongs to the last observation, independently of a waiting or running attempt.",
		sort:   "job_id",
		params: []string{"job_id", "kind", "limit"},
		columns: columns(
			column{
				"job_id",
				"Monitor",
				funcapi.FieldTypeString,
				"",
			},
			column{"kind", "Kind", funcapi.FieldTypeString, ""},
			column{"name", "Name", funcapi.FieldTypeString, ""},
			column{"target", "Target", funcapi.FieldTypeString, ""},
			column{"state", "Current state", funcapi.FieldTypeString, ""},
			column{"fresh", "Last observation fresh", funcapi.FieldTypeBoolean, ""},
			column{"run_id", "Last run", funcapi.FieldTypeString, ""},
			column{"outcome", "Last outcome", funcapi.FieldTypeString, ""},
			column{"completed_us", "Last completion", funcapi.FieldTypeTimestamp, ""},
			column{"duration_ms", "Attempt duration", funcapi.FieldTypeFloat, "ms"},
			column{"last_success_us", "Last success in this activation", funcapi.FieldTypeTimestamp, ""},
			column{"last_failure_us", "Last failure in this activation", funcapi.FieldTypeTimestamp, ""},
			column{"error", "Last error", funcapi.FieldTypeString, ""},
			column{"history_error", "History error", funcapi.FieldTypeString, ""},
		),
	},
	{
		id:      "synthetics-runs",
		title:   "Synthetic runs",
		help:    "Retained attempts filtered by journal saved time. after and before accept Unix seconds or negative offsets from now; omitted bounds cover retained history through now. Results are capped at 2000 and disclose truncation.",
		sort:    "started_us",
		history: true,
		params:  []string{"job_id", "kind", "outcome", "after", "before", "limit"},
		columns: columns(
			column{
				"run_id",
				"Run",
				funcapi.FieldTypeString,
				"",
			},
			column{"job_id", "Monitor", funcapi.FieldTypeString, ""},
			column{"kind", "Kind", funcapi.FieldTypeString, ""},
			column{"name", "Name", funcapi.FieldTypeString, ""},
			column{"target", "Target", funcapi.FieldTypeString, ""},
			column{"started_us", "Started", funcapi.FieldTypeTimestamp, ""},
			column{"completed_us", "Completed", funcapi.FieldTypeTimestamp, ""},
			column{"outcome", "Outcome", funcapi.FieldTypeString, ""},
			column{"duration_ms", "Attempt duration", funcapi.FieldTypeFloat, "ms"},
			column{"capture_state", "Capture state", funcapi.FieldTypeString, ""},
			column{"error", "Error", funcapi.FieldTypeString, ""},
			column{"history_error", "History error", funcapi.FieldTypeString, ""},
		),
	},
	{
		id:     "synthetics-run",
		title:  "Synthetic run detail",
		help:   "One retained or latest active attempt with original test phases, errors, counts, nullable lab measurements and bounded reporter events. Missing completion means no verified terminal observation.",
		sort:   "at_ms",
		params: []string{"job_id", "run_id"},
		columns: columns(
			column{
				"at_ms",
				"Event time",
				funcapi.FieldTypeTimestamp,
				"",
			},
			column{"kind", "Event", funcapi.FieldTypeString, ""},
			column{"test_id", "Test", funcapi.FieldTypeString, ""},
			column{"title", "Title", funcapi.FieldTypeString, ""},
			column{"phase", "Phase", funcapi.FieldTypeString, ""},
			column{"status", "Actual status", funcapi.FieldTypeString, ""},
			column{"expected_status", "Expected status", funcapi.FieldTypeString, ""},
			column{"duration_ms", "Duration", funcapi.FieldTypeFloat, "ms"},
			column{"message", "Message", funcapi.FieldTypeString, ""},
		),
	},
	{
		id:     "synthetics-artifact",
		title:  "Synthetic artifacts",
		help:   "Retained capture metadata for one run; file bytes are verified when fetched. Supplying artifact_id requests base64-encoded bytes and requires administrator-equivalent permissions. Screenshots and HTML reports may contain sensitive content. Missing retained files are expired or unavailable; fetches are limited to 5 MiB.",
		sort:   "artifact_id",
		params: []string{"job_id", "run_id", "artifact_id"},
		columns: columns(
			column{
				"artifact_id",
				"Artifact",
				funcapi.FieldTypeString,
				"",
			},
			column{"kind", "Kind", funcapi.FieldTypeString, ""},
			column{"mime", "Content type", funcapi.FieldTypeString, ""},
			column{"bytes", "Size", funcapi.FieldTypeInteger, "bytes"},
			column{"sha256", "SHA-256", funcapi.FieldTypeString, ""},
			column{"availability", "Availability", funcapi.FieldTypeString, ""},
		),
	},
}

func Declarations() []funcapi.FunctionConfig {
	out := make([]funcapi.FunctionConfig, 0, len(methods))
	for _, m := range methods {
		out = append(
			out,
			funcapi.FunctionConfig{
				ID:             m.id,
				FunctionName:   m.id,
				Name:           m.title,
				UpdateEvery:    10,
				Help:           m.help,
				Tags:           "synthetics",
				RawRequest:     true,
				ManagedInfo:    true,
				HasHistory:     m.history,
				AcceptedParams: append([]string(nil), m.params...),
			},
		)
	}
	return out
}
