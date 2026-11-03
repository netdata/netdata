// SPDX-License-Identifier: GPL-3.0-or-later

package bmc

import (
	"encoding/csv"
	"fmt"
	"os"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSELState_FreeIPMIDefaults(t *testing.T) {
	rows := readSELStatesFixture(t)
	require.Len(t, rows, 474, "FreeIPMI default SEL entries")

	type table struct{ event, sensor int }
	lengths := make(map[table]int)
	for _, row := range rows {
		event, sensor, offset := atoi(t, row[0]), atoi(t, row[1]), atoi(t, row[2])
		tbl := table{event, sensor}
		require.Equal(t, lengths[tbl], offset, "offsets are consecutive from zero")
		lengths[tbl]++

		sensorTypes := []int{sensor}
		if sensor == -1 {
			// The threshold table applies to standard and OEM sensor types alike.
			sensorTypes = []int{0x01, 0x04, 0xc0}
		}
		for _, sensorType := range sensorTypes {
			t.Run(fmt.Sprintf("%02x/%02x/%d", event, sensorType, offset), func(t *testing.T) {
				assert.Equal(t, row[4], selRecordState(event, sensorType, offset, false), "assertion")
				assert.Equal(t, row[5], selRecordState(event, sensorType, offset, true), "deassertion")
			})
		}
	}
	require.Len(t, lengths, 82, "FreeIPMI default SEL tables")
	require.Len(t, selRules, 81, "dispatch pairs besides the threshold table")

	for tbl, length := range lengths {
		sensorType := max(tbl.sensor, 0x01)
		if length < 16 {
			assert.Equal(t, StateUnknown, selRecordState(tbl.event, sensorType, length, false),
				"offset past the end of %v", tbl)
		}
	}
}

func TestSELState_Dispatch(t *testing.T) {
	tests := map[string]struct {
		record [selRecordSize]byte
		want   string
	}{
		"system event": {
			record: selEvent(1, 0, 0x0020, 0x01, 1, 0x01, 0x09, 0, 0),
			want:   StateCritical,
		},
		"unlisted generic pair": {
			record: selEvent(1, 0, 0x0020, 0x2c, 1, 0x03, 0x01, 0, 0),
			want:   StateUnknown,
		},
		"unlisted sensor-specific type": {
			record: selEvent(1, 0, 0x0020, 0x11, 1, 0x6f, 0x00, 0, 0),
			want:   StateUnknown,
		},
		"OEM event type": {
			record: selEvent(1, 0, 0x0020, 0x01, 1, 0x70, 0x00, 0, 0),
			want:   StateUnknown,
		},
		"unspecified event type": {
			record: selEvent(1, 0, 0x0020, 0x01, 1, 0x00, 0x00, 0, 0),
			want:   StateUnknown,
		},
		"OEM sensor type with sensor-specific events": {
			record: selEvent(1, 0, 0x0020, 0xc0, 1, 0x6f, 0x00, 0, 0),
			want:   StateUnknown,
		},
		"reserved record type with event bytes": {
			record: withRecordType(selEvent(1, 0, 0x0020, 0x01, 1, 0x01, 0x09, 0, 0), 0x00),
			want:   StateUnknown,
		},
		"OEM record": {
			record: withRecordType(selEvent(1, 0, 0x0020, 0x01, 1, 0x01, 0x09, 0, 0), 0xc0),
			want:   StateUnknown,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, decodeSELRecord(tc.record, nil).State)
		})
	}
}

// readSELStatesFixture returns the rows of the fixture that enumerates
// FreeIPMI's named default SEL states, independently of the compressed masks
// in sel_states.go: event, sensor (-1 for the threshold table), offset,
// option, assertion and deassertion state. Provenance: sel_states.go and README.md.
func readSELStatesFixture(t *testing.T) [][]string {
	t.Helper()
	f, err := os.Open("testdata/freeipmi-default-sel-states.csv")
	require.NoError(t, err)
	defer func() { _ = f.Close() }()
	reader := csv.NewReader(f)
	reader.Comment = '#'
	rows, err := reader.ReadAll()
	require.NoError(t, err)
	require.Equal(t, []string{"event", "sensor", "offset", "option", "assertion", "deassertion"}, rows[0])
	return rows[1:]
}

// selRecordState decodes a system event record built from the arguments.
func selRecordState(eventType, sensorType, offset int, deassertion bool) string {
	code := byte(eventType)
	if deassertion {
		code |= 0x80
	}
	return decodeSELRecord(selEvent(1, 0x20000000, 0x0020, byte(sensorType), 1, code, byte(offset), 0, 0), nil).State
}

func withRecordType(r [selRecordSize]byte, recordType byte) [selRecordSize]byte {
	r[2] = recordType
	return r
}

func atoi(t *testing.T, s string) int {
	t.Helper()
	v, err := strconv.Atoi(s)
	require.NoError(t, err)
	return v
}
