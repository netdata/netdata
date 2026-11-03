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

func TestSensorState_FreeIPMIDefaults(t *testing.T) {
	// The fixture enumerates FreeIPMI's named default states, independently of
	// the compressed masks in stateRules. Provenance: states.go and README.md.
	f, err := os.Open("testdata/freeipmi-default-states.csv")
	require.NoError(t, err)
	defer func() { _ = f.Close() }()
	reader := csv.NewReader(f)
	reader.Comment = '#'
	rows, err := reader.ReadAll()
	require.NoError(t, err)

	require.Len(t, rows, 1+516, "header and source cases")
	require.Len(t, stateRules, 76, "FreeIPMI default dispatch pairs")

	for _, row := range rows[1:] {
		eventType, err := strconv.Atoi(row[0])
		require.NoError(t, err)
		sensorType, err := strconv.Atoi(row[1])
		require.NoError(t, err)
		offset, err := strconv.Atoi(row[2])
		require.NoError(t, err)
		var asserted uint16
		if offset >= 0 {
			asserted = 1 << offset
		}

		t.Run(fmt.Sprintf("%02x/%02x/%d", eventType, sensorType, offset), func(t *testing.T) {
			reading := decodedReading(t, 0, 0xc0, byte(asserted), byte(asserted>>8))
			assert.Equal(t, row[3], sensorState(uint8(eventType), uint8(sensorType), reading))
		})
	}
}

func TestSensorState(t *testing.T) {
	tests := map[string]struct {
		eventType  uint8
		sensorType uint8
		response   []byte
		want       string
	}{
		"threshold none": {
			eventType:  0x01,
			sensorType: 0x01,
			response:   []byte{25, 0xc0, 0x00},
			want:       StateNominal,
		},
		"lower non-critical": {
			eventType:  0x01,
			sensorType: 0x01,
			response:   []byte{25, 0xc0, 0x01},
			want:       StateNominal,
		},
		"upper non-critical": {
			eventType:  0x01,
			sensorType: 0x01,
			response:   []byte{25, 0xc0, 0x08},
			want:       StateNominal,
		},
		"lower critical": {
			eventType:  0x01,
			sensorType: 0x01,
			response:   []byte{25, 0xc0, 0x02},
			want:       StateCritical,
		},
		"lower non-recoverable": {
			eventType:  0x01,
			sensorType: 0x01,
			response:   []byte{25, 0xc0, 0x04},
			want:       StateCritical,
		},
		"upper critical": {
			eventType:  0x01,
			sensorType: 0x01,
			response:   []byte{25, 0xc0, 0x10},
			want:       StateCritical,
		},
		"upper non-recoverable": {
			eventType:  0x01,
			sensorType: 0x01,
			response:   []byte{25, 0xc0, 0x20},
			want:       StateCritical,
		},
		"overlapping thresholds": {
			eventType:  0x01,
			sensorType: 0x01,
			response:   []byte{25, 0xc0, 0x3f},
			want:       StateCritical,
		},
		"reserved threshold bits": {
			eventType:  0x01,
			sensorType: 0x01,
			response:   []byte{25, 0xc0, 0xc0},
			want:       StateNominal,
		},
		"missing state byte": {
			eventType:  0x01,
			sensorType: 0x01,
			response:   []byte{25, 0xc0},
			want:       StateUnknown,
		},
		"reading unavailable": {
			eventType:  0x01,
			sensorType: 0x01,
			response:   []byte{25, 0xe0, 0x10},
			want:       StateUnknown,
		},
		"scanning disabled": {
			eventType:  0x01,
			sensorType: 0x01,
			response:   []byte{25, 0x80, 0x10},
			want:       StateUnknown,
		},
		"event messages disabled": {
			eventType:  0x01,
			sensorType: 0x01,
			response:   []byte{25, 0x40, 0x10},
			want:       StateCritical,
		},
		"unsupported pair": {
			eventType:  0x03,
			sensorType: 0xff,
			response:   []byte{0, 0xc0, 0x00, 0x00},
			want:       StateUnknown,
		},
		"unknown offset overrides critical": {
			eventType:  0x6f,
			sensorType: 0x08,
			response:   []byte{0, 0xc0, 0x02, 0x01},
			want:       StateUnknown,
		},
		"reserved offset 15 ignored": {
			eventType:  0x6f,
			sensorType: 0x08,
			response:   []byte{0, 0xc0, 0x01, 0x80},
			want:       StateNominal,
		},
		"worst mapped severity wins": {
			eventType:  0x6f,
			sensorType: 0x23,
			response:   []byte{0, 0xc0, 0x03, 0x00},
			want:       StateCritical,
		},
		"extended byte required, missing": {
			eventType:  0x6f,
			sensorType: 0x07,
			response:   []byte{0, 0xc0, 0x80},
			want:       StateUnknown,
		},
		"extended byte optional, missing": {
			eventType:  0x6f,
			sensorType: 0x08,
			response:   []byte{0, 0xc0, 0x01},
			want:       StateNominal,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, sensorState(tc.eventType, tc.sensorType, decodedReading(t, tc.response...)))
		})
	}
}

func decodedReading(t *testing.T, data ...byte) readingResponse {
	t.Helper()
	var r readingResponse
	require.NoError(t, r.Unpack(data))
	return r
}
