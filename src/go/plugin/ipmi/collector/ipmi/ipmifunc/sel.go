// SPDX-License-Identifier: GPL-3.0-or-later

package ipmifunc

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	"github.com/netdata/netdata/go/plugins/plugin/ipmi/collector/ipmi/internal/bmc"
)

// SELFunctionName follows the hyphenated IPMI Function names.
const SELFunctionName = "ipmi-sel"

// MethodSEL is the System Event Log method.
const MethodSEL = "sel"

// selDescription is the short help of the Function list.
const selDescription = "IPMI System Event Log records and their health states"

const selHelp = "Browse every System Event Log record the BMC retains, newest first, read by the collector without extra BMC requests. The dashboard time range does not filter this table: SEL records are often weeks apart. Clearing or rollover on the BMC removes records."

const (
	paramState = "state"
	stateAll   = "all"
)

// selStates are the FreeIPMI default interpretation states, worst first.
var selStates = []string{bmc.StateCritical, bmc.StateWarning, bmc.StateNominal, bmc.StateUnknown}

func selMethod(updateEvery int) funcapi.FunctionConfig {
	return funcapi.FunctionConfig{
		ID:             MethodSEL,
		FunctionName:   SELFunctionName,
		Name:           "IPMI System Event Log",
		Help:           selDescription,
		RequireCloud:   true,
		UpdateEvery:    updateEvery,
		ResponseType:   "table",
		RawRequest:     true,
		ManagedInfo:    true,
		AcceptedParams: []string{funcapi.ParamQuery},
		RequiredParams: []funcapi.ParamConfig{selStateParam()},
	}
}

func selStateParam() funcapi.ParamConfig {
	options := []funcapi.ParamOption{{ID: stateAll, Name: "All states", Default: true}}
	for _, state := range selStates {
		options = append(options, funcapi.ParamOption{
			ID:   state,
			Name: strings.ToUpper(state[:1]) + state[1:],
		})
	}
	return funcapi.ParamConfig{
		ID:      paramState,
		Name:    "State",
		Help:    "Default FreeIPMI interpretation of the record",
		Options: options,
	}
}

func (r *router) HandleRaw(ctx context.Context, req funcapi.RawMethodRequest) *funcapi.FunctionResponse {
	if req.Method != MethodSEL {
		return funcapi.NotFoundResponse(req.Method)
	}
	if ctx.Err() != nil {
		return funcapi.ErrorResponse(499, "IPMI SEL request canceled")
	}
	if req.Info {
		return &funcapi.FunctionResponse{
			Status:         200,
			Help:           selHelp,
			RequiredParams: []funcapi.ParamConfig{selStateParam()},
		}
	}
	s := r.deps.CurrentSnapshot()
	if s == nil {
		return funcapi.UnavailableResponse("Waiting for a successful IPMI collection")
	}
	if s.SEL == nil {
		return funcapi.UnavailableResponse("The System Event Log is unavailable: the BMC has not answered SEL requests")
	}
	query, err := parseSELQuery(req)
	if err != nil {
		return funcapi.ErrorResponse(400, "%s", err)
	}
	return selTable(ctx, s.SEL, query)
}

func selTable(ctx context.Context, sel *bmc.SEL, query selQuery) *funcapi.FunctionResponse {
	rows := make([][]any, 0, len(sel.Records))
	// Log order, newest first: timestamps are absent from some records and
	// follow the BMC clock, which can be set backwards.
	for i := len(sel.Records) - 1; i >= 0 && ctx.Err() == nil; i-- {
		rec := sel.Records[i]
		if query.state != "" && query.state != stateAll && query.state != rec.State {
			continue
		}
		id := fmt.Sprintf("%#04x", rec.ID)
		if query.text != nil && !query.text.MatchString(strings.Join([]string{
			rec.Sensor, rec.SensorType, rec.Event, rec.Direction, rec.State, rec.Generator, id,
		}, " ")) {
			continue
		}
		var timestamp any
		if at, dated := sel.Time(rec); dated {
			timestamp = at.UnixMilli()
		}
		rows = append(rows, []any{
			len(sel.Records) - 1 - i,
			timestamp,
			optionalText(rec.Sensor),
			optionalText(rec.SensorType),
			rec.Event,
			optionalText(rec.Direction),
			rec.State,
			id,
			optionalText(rec.Generator),
			rec.BMCTime(),
			hex.EncodeToString(rec.Raw[:]),
			map[string]string{"severity": rowSeverity(rec.State)},
		})
	}
	if ctx.Err() != nil {
		return funcapi.ErrorResponse(499, "IPMI SEL request canceled")
	}

	help := []string{selHelp, selStatus(sel), selClock(sel)}
	help = append(help, fmt.Sprintf("Returned %d matching records of %d read.", len(rows), len(sel.Records)))
	help = append(help, "Text search uses case-sensitive, ordered simple patterns against sensor, type, event, "+
		"direction, state, generator and record ID (for example *DIMM*; exclusions need a following positive pattern).")
	return &funcapi.FunctionResponse{
		Status:            200,
		Help:              strings.Join(help, " "),
		Columns:           selColumns(),
		Data:              rows,
		DefaultSortColumn: "Order",
		RequiredParams:    []funcapi.ParamConfig{selStateParam()},
	}
}

// selStatus describes the read progress and SEL usage.
func selStatus(sel *bmc.SEL) string {
	var parts []string
	if sel.ReadFailed {
		parts = append(parts, "The latest SEL read failed; newer records may be missing.")
	}
	if !sel.Loaded {
		parts = append(parts, fmt.Sprintf(
			"Reading the SEL: the oldest %d of %d records are read so far; the rest follow in the next collections.",
			len(sel.Records), sel.Entries))
	}
	usage := fmt.Sprintf("The BMC reports %d SEL entries and %d bytes free.", sel.Entries, sel.FreeBytes)
	if sel.Overflow {
		usage += " The overflow flag is set: the BMC dropped events because the SEL was full."
	}
	return strings.Join(append(parts, usage), " ")
}

// selClock describes how record times relate to UTC. Without a reported
// offset, the measured clock difference is shown instead of a guessed offset.
func selClock(sel *bmc.SEL) string {
	if sel.UTCOffset != nil {
		text := fmt.Sprintf("Times are UTC, converted with the BMC's SEL UTC offset of %+d minutes.",
			int(sel.UTCOffset.Minutes()))
		if sel.ClockSkew != nil {
			if residual := *sel.ClockSkew - *sel.UTCOffset; residual.Abs() >= time.Minute {
				text += " After the offset, the BMC clock " + clockRelation(residual) + " the Agent clock."
			}
		}
		return text
	}
	// The offset and the clock are read together.
	if sel.ClockSkew == nil {
		return "The BMC SEL clock has not been read yet, so times are the BMC clock shown as UTC."
	}
	return "The BMC reports no SEL UTC offset, so times are the BMC clock shown as UTC. The BMC clock " +
		clockRelation(*sel.ClockSkew) + " the Agent clock."
}

func clockRelation(d time.Duration) string {
	switch {
	case d.Abs() < time.Second:
		return "matches"
	case d > 0:
		return "is " + d.String() + " ahead of"
	default:
		return "is " + (-d).String() + " behind"
	}
}

func selColumns() map[string]any {
	event := textColumn("Event", "Event description")
	event.FullWidth, event.Wrap = true, true
	timeColumn := funcapi.ColumnMeta{
		Name:      "Time",
		Tooltip:   "When the BMC logged the record, in UTC",
		Type:      funcapi.FieldTypeTimestamp,
		Visible:   true,
		Sortable:  true,
		Sticky:    true,
		Transform: funcapi.FieldTransformDatetime,
	}
	hidden := func(c funcapi.ColumnMeta) funcapi.ColumnMeta {
		c.Visible = false
		return c
	}
	columns := []funcapi.ColumnMeta{
		{
			Name:     "Order",
			Tooltip:  "Position in the log, newest first",
			Type:     funcapi.FieldTypeInteger,
			Sortable: true,
		},
		timeColumn,
		textColumn("Sensor", "SDR sensor name, or the event generator and sensor number"),
		textColumn("Sensor Type", "IPMI sensor type"),
		event,
		textColumn("Direction", "Assertion or deassertion of the event"),
		textColumn("State", "Default FreeIPMI interpretation"),
		{
			Name:      "Record ID",
			Tooltip:   "SEL record identifier",
			Type:      funcapi.FieldTypeString,
			Sortable:  true,
			UniqueKey: true,
		},
		hidden(textColumn("Generator", "Event generator and its ID")),
		hidden(textColumn("BMC Time", "Timestamp as recorded by the BMC")),
		hidden(textColumn("Raw", "Record bytes as returned by the BMC")),
	}
	result := funcapi.Columns(columns, func(c funcapi.ColumnMeta) funcapi.ColumnMeta { return c }).BuildColumns()
	result["Time"].(map[string]any)["value_options"].(map[string]any)["default_value"] = "Unavailable"
	result["rowOptions"] = rowOptionsColumn(len(columns))
	return result
}

func optionalText(value string) any {
	if value == "" {
		return nil
	}
	return value
}

type selQuery struct {
	state string // empty or stateAll selects every state
	text  matcher.Matcher
}

// parseSELQuery reads the state selection and the text query. Other inputs,
// such as the dashboard's after and before, are ignored.
func parseSELQuery(req funcapi.RawMethodRequest) (selQuery, error) {
	params, err := funcapi.ParseRawParams(req)
	if err != nil {
		return selQuery{}, err
	}
	var q selQuery
	if q.state, err = params.String(paramState); err != nil {
		return q, err
	}
	if q.state != "" && q.state != stateAll && !slices.Contains(selStates, q.state) {
		return q, errors.New("invalid state selection")
	}
	if q.text, err = params.QueryMatcher(); err != nil {
		return q, err
	}
	return q, nil
}
