package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/src/collectors/ebpf.plugin/ebpfgo.plugin/libbpfloader"
)

// TestFDGlobalChartContract pins the chart identity the C module published.  A
// change to any id, context, unit, priority, dimension name or algorithm splits
// every existing chart instance on upgrade, so these values are a contract and
// not a preference.
func TestFDGlobalChartContract(t *testing.T) {
	want := map[string]struct {
		context    string
		units      string
		order      int
		dimensions []string
		algorithm  string
		errorChart bool
	}{
		"file_descriptor": {
			context:    "filesystem.file_descriptor",
			units:      "calls/s",
			order:      20270,
			dimensions: []string{"open", "close"},
			algorithm:  "incremental",
		},
		"file_error": {
			context:    "filesystem.file_error",
			units:      "calls/s",
			order:      20271,
			dimensions: []string{"open", "close"},
			algorithm:  "incremental",
			errorChart: true,
		},
	}

	if len(fdGlobalCharts) != len(want) {
		t.Fatalf("fdGlobalCharts has %d charts, want %d", len(fdGlobalCharts), len(want))
	}

	for _, chart := range fdGlobalCharts {
		expected, ok := want[chart.id]
		if !ok {
			t.Fatalf("unexpected chart id %q", chart.id)
		}
		if chart.context != expected.context {
			t.Errorf("%s context = %q, want %q", chart.id, chart.context, expected.context)
		}
		if chart.units != expected.units {
			t.Errorf("%s units = %q, want %q", chart.id, chart.units, expected.units)
		}
		if chart.order != expected.order {
			t.Errorf("%s order = %d, want %d", chart.id, chart.order, expected.order)
		}
		if chart.errorChart != expected.errorChart {
			t.Errorf("%s errorChart = %t, want %t", chart.id, chart.errorChart, expected.errorChart)
		}
		if len(chart.dimensions) != len(expected.dimensions) {
			t.Fatalf("%s has %d dimensions, want %d", chart.id, len(chart.dimensions), len(expected.dimensions))
		}
		for i, dim := range chart.dimensions {
			if dim.id != expected.dimensions[i] {
				t.Errorf("%s dimension %d = %q, want %q", chart.id, i, dim.id, expected.dimensions[i])
			}
			if dim.algorithm != expected.algorithm {
				t.Errorf("%s dimension %q algorithm = %q, want %q",
					chart.id, dim.id, dim.algorithm, expected.algorithm)
			}
		}
	}
}

// TestWriteFDGlobalPublishesRawCounters pins D4: the dimensions are
// `incremental`, so the raw cumulative counters are emitted and the agent derives
// the rate.  Emitting a delta here would double-differentiate.
func TestWriteFDGlobalPublishesRawCounters(t *testing.T) {
	snapshot := libbpfloader.FDSnapshot{OpenCall: 1000, OpenErr: 7, CloseCall: 900, CloseErr: 3}

	var buf bytes.Buffer
	writeFDGlobal(netdataapi.New(&buf), snapshot, 0, true)
	got := buf.String()

	for _, want := range []string{
		"BEGIN 'filesystem.file_descriptor'",
		"SET 'open' = 1000",
		"SET 'close' = 900",
		"BEGIN 'filesystem.file_error'",
		"SET 'open' = 7",
		"SET 'close' = 3",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q:\n%s", want, got)
		}
	}
}

// TestWriteFDGlobalSkipsErrorChartInEntryMode is the D1 contract at the emit
// site: `entry` mode must not emit the error chart, because it was never created.
// Emitting values for an undeclared chart makes pluginsd log an error every cycle.
func TestWriteFDGlobalSkipsErrorChartInEntryMode(t *testing.T) {
	snapshot := libbpfloader.FDSnapshot{OpenCall: 10, OpenErr: 2, CloseCall: 9, CloseErr: 1}

	var buf bytes.Buffer
	writeFDGlobal(netdataapi.New(&buf), snapshot, 0, false)
	got := buf.String()

	if !strings.Contains(got, "filesystem.file_descriptor") {
		t.Fatalf("the count chart must always be emitted:\n%s", got)
	}
	if strings.Contains(got, "filesystem.file_error") {
		t.Fatalf("entry mode emitted the error chart:\n%s", got)
	}
}

// TestWriteFDGlobalNilAPIIsNoOp guards the emit path against a nil API, which is
// how the collector runs when pluginsd output is unavailable.
func TestWriteFDGlobalNilAPIIsNoOp(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("nil api panicked: %v", r)
		}
	}()
	writeFDGlobal(nil, libbpfloader.FDSnapshot{}, 0, true)
}

func TestRunFDGlobalCollectorNilHandleIsNoOp(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("nil handle panicked: %v", r)
		}
	}()
	runFDGlobalCollector(nil, nil, make(chan struct{}), nil, 5, false)
	runFDGlobalCollector(nil, &FDLegacyHandle{}, make(chan struct{}), nil, 5, false)
}
