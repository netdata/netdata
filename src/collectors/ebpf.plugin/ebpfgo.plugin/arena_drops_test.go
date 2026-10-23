package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
)

func TestArenaDropsChartAndCumulativeValue(t *testing.T) {
	var buf bytes.Buffer
	api := netdataapi.New(&buf)
	createArenaDropsChart(api, "dcstat", 1)
	publishArenaDrops(api, "dcstat", 42, 0)

	output := buf.String()
	for _, want := range []string{
		"ebpf.dcstat_arena_drops",
		"ebpf.dcstat_arena_drops",
		"DIMENSION 'dropped'",
		"incremental",
		"SET 'dropped' = 42",
	} {
		if !strings.Contains(output, want) {
			t.Errorf("arena drop output missing %q: %s", want, output)
		}
	}
}
