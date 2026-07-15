// SPDX-License-Identifier: GPL-3.0-or-later

// Layer 0 — harness self-test: fixture data round-trips through the real
// streaming protocol byte-exact, on the live path, the replication path and
// across a daemon restart (journal replay). It also re-verifies the settle
// discipline: since the burst-retention fix (#23096) a brand-new metric
// receiving a burst must be fully visible without the historical
// "first point alone, then wait" workaround.
//
// Pusher discipline (CASE-015): connections stay open until the pushed data
// has settled and been verified — the receiver DISCARDS in-flight data when
// the child disconnects right after writing (see case015_test.go). Closing
// only after the settle barrier makes every green case deterministic.
package corpus

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/netdata/netdata/tests/query-corpus/canon"
	"github.com/netdata/netdata/tests/query-corpus/daemon"
	"github.com/netdata/netdata/tests/query-corpus/fixture"
	"github.com/netdata/netdata/tests/query-corpus/stream"
)

var td *daemon.Daemon

// roundTripOK gates the restart verification on the round-trip tests having
// actually stored their fixtures (t.Failed() cannot see other tests).
var roundTripOK bool

func TestMain(m *testing.M) {
	binary := os.Getenv("QUERY_CORPUS_NETDATA")
	if binary == "" {
		binary = "../../build/netdata"
	}
	abs, err := filepath.Abs(binary)
	if err == nil {
		_, err = os.Stat(abs)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "netdata binary not usable (%v)\nbuild it or set QUERY_CORPUS_NETDATA\n", err)
		os.Exit(1)
	}

	runDir, err := os.MkdirTemp("", "query-corpus-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	td, err = daemon.Start(daemon.Options{Binary: abs, RunDir: runDir})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	code := m.Run()
	_ = td.Stop()

	if code == 0 && os.Getenv("QUERY_CORPUS_KEEP") == "" {
		_ = os.RemoveAll(runDir)
	} else {
		fmt.Fprintf(os.Stderr, "daemon run dir kept: %s\n", runDir)
	}
	os.Exit(code)
}

// guid returns a deterministic fixture machine GUID.
func guid(n int) string {
	return fmt.Sprintf("11111111-1111-4111-8111-%012d", n)
}

// connect opens a pusher connection that closes at test cleanup — i.e.
// AFTER the settle barrier and assertions, per the CASE-015 discipline.
func connect(t *testing.T, hostname, machineGUID string, caps uint32) *stream.Conn {
	t.Helper()
	conn, err := stream.Connect(td.Addr, daemon.APIKey, stream.HostInfo{Hostname: hostname, MachineGUID: machineGUID}, caps)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

// settleAndVerify runs the corpus settle barrier and compares the read-back
// of every dimension of ch on host against the fixture oracle.
func settleAndVerify(t *testing.T, host string, ch fixture.Chart) {
	t.Helper()

	first, last := ch.FirstT(), ch.LastT()
	ue := int64(ch.UpdateEvery)
	if ue <= 0 {
		ue = 1
	}
	if _, err := td.WaitRetention(host, ch.Context, first, last, 15*time.Second); err != nil {
		t.Fatal(err)
	}

	doc, err := td.DataV3(host, daemon.DataParams(ch.Context, first-ue, last, (last-first)/ue+1))
	if err != nil {
		t.Fatal(err)
	}
	cols, err := canon.Columns(doc)
	if err != nil {
		t.Fatal(err)
	}

	for _, dim := range ch.Dimensions {
		col, ok := cols[dim.ID]
		if !ok {
			t.Fatalf("dimension %q missing from result (have %v)", dim.ID, keys(cols))
		}
		exp := dim.Expected()
		if len(col) != len(exp) {
			t.Fatalf("dimension %q: got %d points, want %d", dim.ID, len(col), len(exp))
		}
		for i, want := range exp {
			got := col[i]
			if got.T != want.T {
				t.Errorf("dim %q point %d: time %d, want %d", dim.ID, i, got.T, want.T)
				continue
			}
			switch {
			case want.Value == nil && got.Value != nil:
				t.Errorf("dim %q t0+%d: value %v, want gap (null)", dim.ID, want.T-fixture.T0, *got.Value)
			case want.Value != nil && got.Value == nil:
				t.Errorf("dim %q t0+%d: value null, want %v", dim.ID, want.T-fixture.T0, *want.Value)
			case want.Value != nil && !valuesMatch(*got.Value, *want.Value, ch.ValueTolerance):
				t.Errorf("dim %q t0+%d: value %v, want %v (tolerance %v)", dim.ID, want.T-fixture.T0, *got.Value, *want.Value, ch.ValueTolerance)
			}
			if got.ARP != want.ARP {
				t.Errorf("dim %q t0+%d: anomaly rate %v, want %v", dim.ID, want.T-fixture.T0, got.ARP, want.ARP)
			}
			if got.PA != want.PA {
				t.Errorf("dim %q t0+%d: annotations %d, want %d", dim.ID, want.T-fixture.T0, got.PA, want.PA)
			}
		}
	}
}

// valuesMatch compares a queried value to the oracle: exact when tol is
// zero, relative tolerance otherwise (quantization-probing fixtures).
func valuesMatch(got, want, tol float64) bool {
	if tol == 0 {
		return got == want
	}
	diff := got - want
	if diff < 0 {
		diff = -diff
	}
	limit := tol
	if aw := want; aw != 0 {
		if aw < 0 {
			aw = -aw
		}
		if aw*tol > limit {
			limit = aw * tol
		}
	}
	return diff <= limit
}

func keys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// pushLiveBurst sends chart metadata and the FULL point series in one
// buffered write — no settle discipline at all. Green requires the
// burst-retention fix (#23096) in the daemon under test.
func pushLiveBurst(t *testing.T, hostname, machineGUID string, ch fixture.Chart) {
	t.Helper()
	conn := connect(t, hostname, machineGUID, stream.CapsLive)
	ch.Define(conn)
	ch.PushLive(conn)
	if err := conn.Flush(); err != nil {
		t.Fatal(err)
	}
}

// pushLivePaced replays the historical spike-3 settle discipline: first
// point alone, wait until its retention stamp lands, then burst the rest.
// Kept as a control to isolate protocol failures from timing regressions.
func pushLivePaced(t *testing.T, hostname, machineGUID string, ch fixture.Chart) {
	t.Helper()
	conn := connect(t, hostname, machineGUID, stream.CapsLive)

	single := ch
	single.Dimensions = make([]fixture.Dimension, len(ch.Dimensions))
	for i, d := range ch.Dimensions {
		single.Dimensions[i] = d
		single.Dimensions[i].Points = d.Points[:1]
	}
	rest := ch
	rest.Dimensions = make([]fixture.Dimension, len(ch.Dimensions))
	for i, d := range ch.Dimensions {
		rest.Dimensions[i] = d
		rest.Dimensions[i].Points = d.Points[1:]
	}

	ch.Define(conn)
	single.PushLive(conn)
	if err := conn.Flush(); err != nil {
		t.Fatal(err)
	}
	if _, err := td.WaitRetention(hostname, ch.Context, ch.FirstT(), ch.FirstT(), 15*time.Second); err != nil {
		t.Fatal(err)
	}
	rest.PushLive(conn)
	if err := conn.Flush(); err != nil {
		t.Fatal(err)
	}
}

// pushReplication declares child retention and serves the parent's
// replication requests from the fixture. firstT is declared one step before
// the first point because the request window (after, before] is exclusive
// on the left.
func pushReplication(t *testing.T, hostname, machineGUID string, ch fixture.Chart) {
	t.Helper()
	conn := connect(t, hostname, machineGUID, stream.CapsReplication)

	firstT := ch.FirstT() - 1
	lastT := ch.LastT()
	childNow := lastT // fixture wall clock frozen at the last sample

	ch.Define(conn)
	conn.ChartDefinitionEnd(firstT, lastT, childNow)

	charts := map[string]struct{ FirstT, LastT int64 }{
		ch.ID: {FirstT: firstT, LastT: lastT},
	}
	served, err := conn.ServeReplication(charts, childNow, func(chart string, after, before int64) []stream.ReplayRow {
		return ch.ReplayWindow(after, before)
	}, 30*time.Second)
	if err != nil {
		t.Fatalf("replication dialogue: %v (served %v)", err, served)
	}
	if served[ch.ID] != len(ch.Dimensions[0].Points) {
		t.Fatalf("replication served %d rows, want %d", served[ch.ID], len(ch.Dimensions[0].Points))
	}
}

func TestLayer0RoundTrip(t *testing.T) {
	cases := map[string]struct {
		hostname string
		guid     string
		chart    fixture.Chart
		push     func(t *testing.T, hostname, machineGUID string, ch fixture.Chart)
	}{
		"live-burst-no-settle-discipline": {
			hostname: "l0-live",
			guid:     guid(1),
			chart:    fixture.FullPalette("fixture.l0live", "fixture.l0live", fixture.T0, 60),
			push:     pushLiveBurst,
		},
		"live-paced-legacy-discipline": {
			hostname: "l0-paced",
			guid:     guid(2),
			chart:    fixture.FullPalette("fixture.l0paced", "fixture.l0paced", fixture.T0, 60),
			push:     pushLivePaced,
		},
		"replication": {
			hostname: "l0-repl",
			guid:     guid(3),
			chart:    fixture.FullPalette("fixture.l0repl", "fixture.l0repl", fixture.T0, 60),
			push:     pushReplication,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			tc.push(t, tc.hostname, tc.guid, tc.chart)
			settleAndVerify(t, tc.hostname, tc.chart)
		})
	}
	roundTripOK = !t.Failed()
}

// TestLayer0TwoChildren pushes the same context from two children and
// verifies each host answers independently — the two-children palette seed.
func TestLayer0TwoChildren(t *testing.T) {
	hosts := []struct {
		hostname string
		guid     string
	}{
		{"l0-dual-a", guid(4)},
		{"l0-dual-b", guid(5)},
	}
	ch := fixture.FullPalette("fixture.l0dual", "fixture.l0dual", fixture.T0, 60)
	for _, h := range hosts {
		pushLiveBurst(t, h.hostname, h.guid, ch)
	}
	for _, h := range hosts {
		settleAndVerify(t, h.hostname, ch)
	}
}

// TestLayer0Labels verifies chart labels pushed via CLABEL are visible on
// the query path.
func TestLayer0Labels(t *testing.T) {
	ch := fixture.FullPalette("fixture.l0label", "fixture.l0label", fixture.T0, 60)
	ch.Labels = [][2]string{{"corpus_case", "layer0"}, {"corpus_kind", "labels"}}
	pushLiveBurst(t, "l0-label", guid(6), ch)
	settleAndVerify(t, "l0-label", ch)

	params := daemon.DataParams(ch.Context, fixture.T0, fixture.T0+60, 60)
	params.Set("group_by", "label")
	params.Set("group_by_label", "corpus_case")
	doc, err := td.DataV3("l0-label", params)
	if err != nil {
		t.Fatal(err)
	}
	cols, err := canon.Columns(doc)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := cols["layer0"]; !ok {
		t.Fatalf("group_by=label did not surface label value: have %v", keys(cols))
	}
}

// TestLayer0ZRestart re-verifies earlier fixtures byte-identical after a
// daemon restart, covering the journal-v2 read path. It MUST stay the last
// test in this file: it restarts the shared daemon and depends on the
// round-trip tests having pushed their data.
func TestLayer0ZRestart(t *testing.T) {
	if !roundTripOK {
		t.Skip("round-trip failures; skipping restart verification")
	}
	if err := td.Restart(); err != nil {
		t.Fatal(err)
	}
	settleAndVerify(t, "l0-live", fixture.FullPalette("fixture.l0live", "fixture.l0live", fixture.T0, 60))
	settleAndVerify(t, "l0-repl", fixture.FullPalette("fixture.l0repl", "fixture.l0repl", fixture.T0, 60))
}
