// SPDX-License-Identifier: GPL-3.0-or-later

package ipmiapi

import (
	"encoding/csv"
	"fmt"
	"os"
	"strconv"
	"testing"
)

func decodedReading(t *testing.T, data ...byte) readingResponse {
	t.Helper()
	var r readingResponse
	if err := r.Unpack(data); err != nil {
		t.Fatal(err)
	}
	return r
}
func TestFreeIPMIDefaultDispatch(t *testing.T) {
	// Fixture enumerates the upstream named states, separately from the runtime
	// compressed masks. The notice/provenance is in states.go and README.md.
	f, err := os.Open("testdata/freeipmi-default-states.csv")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	reader := csv.NewReader(f)
	reader.Comment = '#'
	rows, err := reader.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 517 {
		t.Fatalf("source fixture has %d cases, want 516", len(rows)-1)
	}
	for _, row := range rows[1:] {
		event, _ := strconv.Atoi(row[0])
		kind, _ := strconv.Atoi(row[1])
		offset, _ := strconv.Atoi(row[2])
		var bits uint16
		if offset >= 0 {
			bits = 1 << offset
		}
		t.Run(fmt.Sprintf("%02x/%02x/%d", event, kind, offset), func(t *testing.T) {
			got := sensorState(uint8(event), uint8(kind), decodedReading(t, 0, 0xc0, byte(bits), byte(bits>>8)))
			if got != row[3] {
				t.Fatalf("got %s, want %s", got, row[3])
			}
		})
	}
	if len(stateRules) != 76 {
		t.Fatalf("dispatch count %d", len(stateRules))
	}
}
func TestThresholdAndDiscreteStatus(t *testing.T) {
	for _, tc := range []struct {
		name        string
		event, kind byte
		data        []byte
		want        string
	}{
		{"threshold none", 1, 1, []byte{25, 0xc0, 0}, "nominal"},
		{"LNC", 1, 1, []byte{25, 0xc0, 1}, "nominal"},
		{"UNC", 1, 1, []byte{25, 0xc0, 8}, "nominal"},
		{"LCR", 1, 1, []byte{25, 0xc0, 2}, "critical"},
		{"LNR", 1, 1, []byte{25, 0xc0, 4}, "critical"},
		{"UCR", 1, 1, []byte{25, 0xc0, 16}, "critical"},
		{"UNR", 1, 1, []byte{25, 0xc0, 32}, "critical"},
		{"overlapping", 1, 1, []byte{25, 0xc0, 63}, "critical"},
		{"reserved threshold", 1, 1, []byte{25, 0xc0, 0xc0}, "nominal"},
		{"missing status", 1, 1, []byte{25, 0xc0}, "unknown"},
		{"unavailable", 1, 1, []byte{25, 0xe0, 16}, "unknown"},
		{"scan disabled", 1, 1, []byte{25, 0x80, 16}, "unknown"},
		{"events disabled", 1, 1, []byte{25, 0x40, 16}, "critical"},
		{"unsupported pair", 3, 0xff, []byte{0, 0xc0, 0, 0}, "unknown"},
		{"unknown overrides critical", 0x6f, 8, []byte{0, 0xc0, 2, 1}, "unknown"},
		{"reserved bit15 ignored", 0x6f, 8, []byte{0, 0xc0, 1, 0x80}, "nominal"},
		{"watchdog mixed severity", 0x6f, 0x23, []byte{0, 0xc0, 3, 0}, "critical"},
		{"processor missing high byte", 0x6f, 7, []byte{0, 0xc0, 128}, "unknown"},
		{"power supply optional high byte", 0x6f, 8, []byte{0, 0xc0, 1}, "nominal"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := sensorState(tc.event, tc.kind, decodedReading(t, tc.data...)); got != tc.want {
				t.Fatalf("got %s, want %s", got, tc.want)
			}
		})
	}
}
