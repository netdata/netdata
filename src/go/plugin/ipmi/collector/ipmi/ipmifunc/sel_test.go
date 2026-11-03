// SPDX-License-Identifier: GPL-3.0-or-later

package ipmifunc

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/ipmi/collector/ipmi/internal/bmc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// selLogged is an absolute BMC timestamp; with a +120 minute UTC offset the
// record was logged two hours earlier in UTC.
const selLogged = 0x60000000

func testSEL() *bmc.SEL {
	record := func(id uint16, timestamp uint32, sensor, event, direction, state string) bmc.SELRecord {
		return bmc.SELRecord{
			ID:          id,
			Timestamp:   timestamp,
			Timestamped: true,
			Sensor:      sensor,
			SensorType:  "Temperature",
			Generator:   "BMC (0x20)",
			Event:       event,
			Direction:   direction,
			State:       state,
		}
	}
	nonTimestamped := bmc.SELRecord{
		ID:    0x13,
		Event: "OEM record type 0xe0",
		State: bmc.StateUnknown,
	}
	return &bmc.SEL{
		Records: []bmc.SELRecord{
			record(0x10, selLogged, "DIMMG1_TEMP", "Upper Critical - going high", "Assertion", bmc.StateCritical),
			record(0x11, selLogged+60, "DIMMG1_TEMP", "Upper Critical - going high", "Deassertion", bmc.StateWarning),
			record(0x12, 35, "BIOS #0", "OEM System Boot Event", "Assertion", bmc.StateNominal),
			nonTimestamped,
			record(0x14, selLogged+120, "SYS_FAN1", "Lower Non-critical - going low", "Assertion", bmc.StateWarning),
		},
		Loaded:    true,
		Entries:   5,
		FreeBytes: 6300,
		UTCOffset: new(2 * time.Hour),
		ClockSkew: new(2 * time.Hour),
	}
}

// utcMillis is the UTC time of a record logged at selLogged+seconds.
func utcMillis(seconds int64) int64 {
	return (selLogged + seconds - 2*3600) * 1000
}

func handleSEL(t *testing.T, sel *bmc.SEL, req funcapi.RawMethodRequest) *funcapi.FunctionResponse {
	t.Helper()
	req.Method = MethodSEL
	r := NewRouter(&fakeDeps{
		snapshot: &bmc.Snapshot{
			SEL: sel,
		},
	}).(funcapi.RawMethodHandler)
	return r.HandleRaw(t.Context(), req)
}

func columnValues(t *testing.T, response *funcapi.FunctionResponse, name string) []any {
	t.Helper()
	column, ok := response.Columns[name].(map[string]any)
	require.True(t, ok, "column %s", name)
	var values []any
	for _, row := range response.Data.([][]any) {
		values = append(values, row[column["index"].(int)])
	}
	return values
}

func TestRouter_SELTable(t *testing.T) {
	response := handleSEL(t, testSEL(), funcapi.RawMethodRequest{})
	require.Equal(t, 200, response.Status)

	assert.Equal(t, []any{"0x0014", "0x0013", "0x0012", "0x0011", "0x0010"}, columnValues(t, response, "Record ID"),
		"log order, newest first")
	assert.Equal(t, []any{0, 1, 2, 3, 4}, columnValues(t, response, "Order"))
	assert.Equal(t, []any{utcMillis(120), nil, nil, utcMillis(60), utcMillis(0)}, columnValues(t, response, "Time"))
	assert.Equal(
		t,
		[]any{"SYS_FAN1", nil, "BIOS #0", "DIMMG1_TEMP", "DIMMG1_TEMP"},
		columnValues(t, response, "Sensor"),
	)
	assert.Equal(
		t,
		[]any{"Assertion", nil, "Assertion", "Deassertion", "Assertion"},
		columnValues(t, response, "Direction"),
	)
	assert.Equal(t,
		[]any{bmc.StateWarning, bmc.StateUnknown, bmc.StateNominal, bmc.StateWarning, bmc.StateCritical},
		columnValues(t, response, "State"))
	assert.Equal(t,
		[]any{"2021-01-14 08:27:36 (BMC clock)", "Not timestamped", "35 s after BMC initialization",
			"2021-01-14 08:26:36 (BMC clock)", "2021-01-14 08:25:36 (BMC clock)"},
		columnValues(t, response, "BMC Time"))
	assert.Equal(t,
		[]any{
			map[string]string{"severity": "warning"},
			map[string]string{"severity": "normal"},
			map[string]string{"severity": "normal"},
			map[string]string{"severity": "warning"},
			map[string]string{"severity": "critical"},
		},
		columnValues(t, response, "rowOptions"))
	assert.Equal(t, "Order", response.DefaultSortColumn)

	visible := map[string]bool{}
	for name, raw := range response.Columns {
		column := raw.(map[string]any)
		visible[name] = column["visible"].(bool)
		assert.Equal(t, name == "Record ID", column["unique_key"], "column %s: only Record ID identifies rows", name)
	}
	assert.Equal(t, map[string]bool{
		"Order":       false,
		"Time":        true,
		"Sensor":      true,
		"Sensor Type": true,
		"Event":       true,
		"Direction":   true,
		"State":       true,
		"Record ID":   false,
		"Generator":   false,
		"BMC Time":    false,
		"Raw":         false,
		"rowOptions":  false,
	}, visible)

	assert.Contains(t, response.Help, "converted with the BMC's SEL UTC offset of +120 minutes")
	assert.Contains(t, response.Help, "The BMC reports 5 SEL entries and 6300 bytes free.")
	assert.Contains(t, response.Help, "Returned 5 matching records of 5 read.")
	assert.Contains(t, response.Help, "The dashboard time range does not filter this table")
	assert.NotContains(t, response.Help, "Reading the SEL")
	require.Len(t, response.RequiredParams, 1)
	assert.Equal(t, paramState, response.RequiredParams[0].ID)

	// The table envelope the framework serializes for this method.
	method := selMethod(10)
	assert.False(t, method.HasHistory, "SEL records are often weeks apart; a time range would hide them")
	assert.Empty(t, method.Tags, "listed with the other IPMI Functions")
	validateFunctionPayload(t, map[string]any{
		"v":                   3,
		"update_every":        method.UpdateEvery,
		"status":              response.Status,
		"type":                method.ResponseType,
		"has_history":         method.HasHistory,
		"help":                response.Help,
		"accepted_params":     append([]string{paramState}, method.AcceptedParams...),
		"required_params":     []map[string]any{response.RequiredParams[0].RequiredParam()},
		"columns":             response.Columns,
		"data":                response.Data,
		"default_sort_column": response.DefaultSortColumn,
	})
}

func TestRouter_SELFilters(t *testing.T) {
	tests := map[string]struct {
		req     funcapi.RawMethodRequest
		wantIDs []any
		// wantOrder, when set, is the log position of each returned record.
		wantOrder []any
	}{
		"state selection": {
			req: funcapi.RawMethodRequest{
				Payload: []byte(`{"selections":{"state":["warning"]}}`),
			},
			wantIDs:   []any{"0x0014", "0x0011"},
			wantOrder: []any{0, 3},
		},
		"all states": {
			req: funcapi.RawMethodRequest{
				Payload: []byte(`{"state":"all"}`),
			},
			wantIDs: []any{"0x0014", "0x0013", "0x0012", "0x0011", "0x0010"},
		},
		"arguments win over payload": {
			req: funcapi.RawMethodRequest{
				Args:    []string{"state:critical"},
				Payload: []byte(`{"selections":{"state":["warning"]}}`),
			},
			wantIDs: []any{"0x0010"},
		},
		"selections win over payload fields": {
			req:     funcapi.RawMethodRequest{Payload: []byte(`{"state":"warning","selections":{"state":["critical"]}}`)},
			wantIDs: []any{"0x0010"},
		},
		"text query": {
			req: funcapi.RawMethodRequest{
				Payload: []byte(`{"query":"*FAN*"}`),
			},
			wantIDs: []any{"0x0014"},
		},
		"text query matches the record ID": {
			req: funcapi.RawMethodRequest{
				Payload: []byte(`{"query":"*0x0012*"}`),
			},
			wantIDs: []any{"0x0012"},
		},
		"case-sensitive text query": {
			req: funcapi.RawMethodRequest{
				Payload: []byte(`{"query":"*fan*"}`),
			},
		},
		"dashboard time range is ignored": {
			req: funcapi.RawMethodRequest{
				Args: []string{"after:-900", "before:0"},
				Payload: []byte(`{"after":` + strconv.FormatInt(utcMillis(30)/1000, 10) +
					`,"before":` + strconv.FormatInt(utcMillis(90)/1000, 10) + `}`),
			},
			wantIDs: []any{"0x0014", "0x0013", "0x0012", "0x0011", "0x0010"},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			response := handleSEL(t, testSEL(), tc.req)
			require.Equal(t, 200, response.Status, response.Message)
			assert.Equal(t, tc.wantIDs, columnValues(t, response, "Record ID"))
			if tc.wantOrder != nil {
				assert.Equal(t, tc.wantOrder, columnValues(t, response, "Order"), "filters keep log positions")
			}
		})
	}
}

func TestRouter_SELRequestErrors(t *testing.T) {
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	tests := map[string]struct {
		ctx        context.Context
		snapshot   *bmc.Snapshot
		req        funcapi.RawMethodRequest
		wantStatus int
	}{
		"no successful collection": {
			req: funcapi.RawMethodRequest{
				Method: MethodSEL,
			},
			wantStatus: 503,
		},
		"no SEL": {
			snapshot: &bmc.Snapshot{},
			req: funcapi.RawMethodRequest{
				Method: MethodSEL,
			},
			wantStatus: 503,
		},
		"unknown method": {
			snapshot: &bmc.Snapshot{
				SEL: testSEL(),
			},
			req: funcapi.RawMethodRequest{
				Method: methodSensors,
			},
			wantStatus: 404,
		},
		"canceled": {
			ctx: canceled,
			snapshot: &bmc.Snapshot{
				SEL: testSEL(),
			},
			req: funcapi.RawMethodRequest{
				Method: MethodSEL,
			},
			wantStatus: 499,
		},
	}
	for name, payload := range map[string]string{
		"invalid JSON":           `[`,
		"unknown state":          `{"state":"error"}`,
		"several states":         `{"state":["warning","critical"]}`,
		"invalid pattern":        `{"query":"["}`,
		"several query patterns": `{"query":["a","b"]}`,
	} {
		tests[name] = struct {
			ctx        context.Context
			snapshot   *bmc.Snapshot
			req        funcapi.RawMethodRequest
			wantStatus int
		}{
			snapshot: &bmc.Snapshot{
				SEL: testSEL(),
			},
			req: funcapi.RawMethodRequest{
				Method:  MethodSEL,
				Payload: []byte(payload),
			},
			wantStatus: 400,
		}
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			ctx := tc.ctx
			if ctx == nil {
				ctx = context.Background()
			}
			r := NewRouter(&fakeDeps{
				snapshot: tc.snapshot,
			}).(funcapi.RawMethodHandler)
			response := r.HandleRaw(ctx, tc.req)
			assert.Equal(t, tc.wantStatus, response.Status)
			assert.Nil(t, response.Data)
		})
	}
}

func TestRouter_SELInfo(t *testing.T) {
	r := NewRouter(&fakeDeps{}).(funcapi.RawMethodHandler)
	response := r.HandleRaw(t.Context(), funcapi.RawMethodRequest{
		Method: MethodSEL,
		Info:   true,
	})
	require.Equal(t, 200, response.Status, "info needs no collection")
	assert.Nil(t, response.Data)
	assert.Equal(t, []funcapi.ParamConfig{selStateParam()}, response.RequiredParams)
}

func TestRouter_SELHelp(t *testing.T) {
	tests := map[string]struct {
		change func(*bmc.SEL)
		want   []string
	}{
		"loading": {
			change: func(s *bmc.SEL) {
				s.Loaded, s.Entries = false, 672
			},
			want: []string{"Reading the SEL: the oldest 5 of 672 records are read so far"},
		},
		"overflow and failed read": {
			change: func(s *bmc.SEL) {
				s.Overflow, s.ReadFailed = true, true
			},
			want: []string{
				"The latest SEL read failed; newer records may be missing.",
				"The overflow flag is set: the BMC dropped events because the SEL was full.",
			},
		},
		"no offset": {
			change: func(s *bmc.SEL) {
				s.UTCOffset = nil
			},
			want: []string{
				"The BMC reports no SEL UTC offset, so times are the BMC clock shown as UTC.",
				"The BMC clock is 2h0m0s ahead of the Agent clock.",
			},
		},
		"clock not read yet": {
			change: func(s *bmc.SEL) {
				s.UTCOffset, s.ClockSkew = nil, nil
			},
			want: []string{"The BMC SEL clock has not been read yet, so times are the BMC clock shown as UTC."},
		},
		"no offset, clock behind": {
			change: func(s *bmc.SEL) {
				s.UTCOffset, s.ClockSkew = nil, new(-90*time.Second)
			},
			want: []string{"The BMC clock is 1m30s behind the Agent clock."},
		},
		"offset with a wrong clock": {
			change: func(s *bmc.SEL) {
				s.ClockSkew = new(2*time.Hour + 5*time.Minute)
			},
			want: []string{"After the offset, the BMC clock is 5m0s ahead of the Agent clock."},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			sel := testSEL()
			tc.change(sel)
			response := handleSEL(t, sel, funcapi.RawMethodRequest{})
			require.Equal(t, 200, response.Status)
			for _, want := range tc.want {
				assert.Contains(t, response.Help, want)
			}
		})
	}

	response := handleSEL(t, testSEL(), funcapi.RawMethodRequest{})
	assert.NotContains(t, response.Help, "After the offset", "a clock in step with its offset is not reported")
}
