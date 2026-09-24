// SPDX-License-Identifier: GPL-3.0-or-later

package listen

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartemit"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/profilecatalog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testProfileDirs holds the profile named by the serialization fixtures.
var testProfileDirs = []profilecatalog.DirSpec{{Path: "testdata/profiles"}}

// testListeners is an explicit fixture endpoint. Init validates it; only Run binds.
var testListeners = []ListenerConfig{{
	Protocol: protocolUDP,
	Address:  "127.0.0.1:18125",
}}

// coreFixture drives a prepared collector through real metric cycles, template
// capture, the native engine and protocol emission, with a controllable clock.
type coreFixture struct {
	c      *Collector
	cycle  metrix.CycleController
	source *collectorapi.ChartTemplateSource
	engine *chartengine.Engine
	time   time.Time
}

func newCoreFixture(t testing.TB, capacity int, idle time.Duration, opts ...metrix.CollectorStoreOption) *coreFixture {
	t.Helper()
	c := New()
	c.Listeners = testListeners
	c.MaxSeries = capacity
	c.MetricIdleTimeout = confopt.Duration(idle)
	if len(opts) > 0 {
		c.store = metrix.NewCollectorStore(opts...)
	}
	f := prepareFixture(t, c)
	f.time = time.Unix(1000, 0)
	c.now = func() time.Time { return f.time }
	// Record-level tests drive the receiver directly. Run activates it after
	// binding; socket framing and lifecycle are covered by the runtime tests.
	c.receiver.start()
	return f
}

// prepareFixture runs Init/Check and binds the real template capture, metric
// cycle and native engine used by the framework's publication path.
func prepareFixture(t testing.TB, c *Collector) *coreFixture {
	t.Helper()
	f := &coreFixture{
		c: c,
	}
	require.NoError(t, c.Init(context.Background()))
	require.NoError(t, c.Check(context.Background()))
	managed, ok := metrix.AsCycleManagedStore(c.MetricStore())
	require.True(t, ok)
	f.cycle = managed.CycleController()
	var err error
	f.source, err = collectorapi.NewChartTemplateSource(c)
	require.NoError(t, err)
	_, err = f.source.Capture()
	require.NoError(t, err)
	f.engine, err = chartengine.New(chartengine.WithRuntimeStore(nil))
	require.NoError(t, err)
	t.Cleanup(func() { c.Cleanup(context.Background()) })
	return f
}

func (f *coreFixture) ingest(t testing.TB, lines ...string) {
	t.Helper()
	for _, line := range lines {
		require.NoError(t, f.c.receiver.ingest(line, f.time), line)
	}
}

func (f *coreFixture) collect(t testing.TB, metricAbort, outputAbort bool) string {
	t.Helper()
	f.cycle.BeginCycle()
	require.NoError(t, f.c.Collect(context.Background()))
	return f.finish(t, metricAbort, outputAbort)
}

func (f *coreFixture) finish(t testing.TB, metricAbort, outputAbort bool) string {
	t.Helper()
	captured, err := f.source.Capture()
	require.NoError(t, err)
	if metricAbort {
		f.cycle.AbortCycle()
		return ""
	}
	require.NoError(t, f.cycle.CommitCycleSuccess())
	attempt, err := f.engine.PreparePlanWithOptions(
		f.c.store.Read(metrix.ReadRaw(), metrix.ReadFlatten()),
		chartengine.PlanOptions{
			TemplateSet: captured,
		},
	)
	require.NoError(t, err)
	var buf bytes.Buffer
	require.NoError(
		t,
		chartemit.ApplyPlan(
			netdataapi.New(&buf),
			attempt.Plan(),
			chartemit.EmitEnv{
				TypeID:      "statsd.test",
				UpdateEvery: 1,
				StoreFirst:  true,
			},
		),
	)
	if outputAbort {
		attempt.Abort()
		return ""
	}
	require.NoError(t, attempt.Commit())
	return buf.String()
}

// applicationCharts counts CHART definitions outside the fixed receiver diagnostics.
func applicationCharts(wire string) int {
	n := 0
	for line := range strings.Lines(wire) {
		if strings.HasPrefix(line, "CHART ") && !strings.Contains(line, "'statsd.receiver.") {
			n++
		}
	}
	return n
}

// applicationSeries counts retained store series outside the receiver diagnostics.
func applicationSeries(c *Collector) int {
	n := 0
	c.store.Read(metrix.ReadRaw(), metrix.ReadFlatten()).
		ForEachSeries(func(name string, _ metrix.LabelView, _ metrix.SampleValue) {
			if !strings.HasPrefix(name, "receiver.") {
				n++
			}
		})
	return n
}

// value asserts one scalar series in the store.
func value(t testing.TB, c *Collector, name string, want float64, ls metrix.Labels) {
	t.Helper()
	got, ok := c.store.Read().Value(name, ls)
	require.True(t, ok, name)
	assert.Equal(t, want, got, name)
}

// Profiles used by the receiver tests, in precedence order: pools owns
// svc.*.size names, shadow owns the remaining svc.* names, app renders from the
// shared final stream without preprocessing, and meta edits metadata and names.
var testProfiles = map[string]string{
	"pools": `
match: 'svc.*'
relabeling:
  - match: 'svc.*.size'
    metric_relabel_configs:
      - source_labels: [__name__]
        regex: 'svc\.([^.]+)\.size'
        target_label: pool
        replacement: '$1'
      - source_labels: [__name__]
        regex: 'svc\.[^.]+\.size'
        target_label: __name__
        replacement: svc.pool.size
template:
  family: pools
  metrics:
    - g.value.svc.pool.size
  charts:
    - title: Pool size
      context: pools.size
      units: items
      instances:
        by_labels: [pool]
      dimensions:
        - selector: g.value.svc.pool.size
          name: size
`,
	"shadow": `
match: 'svc.*'
relabeling:
  - match: '*'
    metric_relabel_configs:
      - target_label: shadow
        replacement: 'yes'
`,
	"app": `
match: 'svc.* app.*'
template:
  family: app
  metrics:
    - c.total.app.requests
    - g.value.svc.pool.size
  charts:
    - title: Application requests
      context: app.requests
      units: requests/s
      dimensions:
        - selector: c.total.app.requests
          name: requests
    - title: Total pool size
      context: app.pool_size
      units: items
      dimensions:
        - selector: g.value.svc.pool.size
          name: size
`,
	"meta": `
match: 'meta.*'
relabeling:
  - match: 'meta.*'
    metric_relabel_configs:
      - target_label: nd_unit
        replacement: bytes
      - target_label: measure_field
        replacement: ''
      - source_labels: [__name__]
        regex: 'meta\.alias\..+'
        target_label: __name__
        replacement: meta.alias
      - source_labels: [__name__]
        regex: 'meta\.empty'
        target_label: __name__
        replacement: ''
      - source_labels: [__name__]
        regex: 'meta\.typo'
        target_label: nd_units
        replacement: bytes
      - source_labels: [zone]
        regex: '(.+)'
        target_label: region
        replacement: 'r-$1'
`,
}

// writeProfiles writes profile files to a temporary user profile directory.
func writeProfiles(t testing.TB, files map[string]string) []profilecatalog.DirSpec {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name+".yaml"), []byte(content), 0o600))
	}
	return []profilecatalog.DirSpec{{Path: dir}}
}

func newProfileFixture(t testing.TB, files map[string]string, names ...string) *coreFixture {
	t.Helper()
	return newConfiguredProfileFixture(t, func(*Collector) {}, files, names...)
}

// newConfiguredProfileFixture lets configure adjust configuration before Init.
func newConfiguredProfileFixture(
	t testing.TB,
	configure func(*Collector),
	files map[string]string,
	names ...string,
) *coreFixture {
	t.Helper()
	c := New()
	c.Listeners = testListeners
	c.profileDirs = writeProfiles(t, files)
	c.Profiles = names
	configure(c)
	f := prepareFixture(t, c)
	f.time = time.Unix(1000, 0)
	c.now = func() time.Time { return f.time }
	c.receiver.start()
	return f
}

func entryIDs(c *Collector) []string {
	var ids []string
	for _, e := range c.ChartTemplateSet().Entries() {
		ids = append(ids, e.ID)
	}
	return ids
}

// waitFor bounds every wait for asynchronous socket input.
const waitFor = 5 * time.Second

// freeAddress returns a loopback address currently free for both UDP and TCP.
func freeAddress(t testing.TB) string {
	t.Helper()
	for range 20 {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		addr := ln.Addr().String()
		pc, err := net.ListenPacket("udp", addr)
		_ = ln.Close()
		if err == nil {
			_ = pc.Close()
			return addr
		}
	}
	t.Fatal("no loopback port free for both UDP and TCP")
	return ""
}

// requireFree proves an address is not held by binding and releasing it.
func requireFree(t testing.TB, protocol, addr string) {
	t.Helper()
	var closer io.Closer
	var err error
	if protocol == protocolUDP {
		closer, err = net.ListenPacket(protocolUDP, addr)
	} else {
		closer, err = net.Listen(protocolTCP, addr)
	}
	require.NoError(t, err, "%s %s must be free", protocol, addr)
	require.NoError(t, closer.Close())
}

// runtimeFixture is a running collector with UDP and TCP listeners on one
// loopback port.
type runtimeFixture struct {
	*coreFixture
	addr   string
	cancel context.CancelFunc
	done   chan error
}

// startRuntime prepares a collector on one UDP and one TCP loopback listener
// and runs it until readiness. configure may adjust configuration before Init.
func startRuntime(t testing.TB, configure func(*Collector)) *runtimeFixture {
	t.Helper()
	c := New()
	addr := freeAddress(t)
	c.Listeners = []ListenerConfig{{Protocol: protocolUDP, Address: addr}, {Protocol: protocolTCP, Address: addr}}
	c.profileDirs = nil
	if configure != nil {
		configure(c)
	}
	f := &runtimeFixture{
		coreFixture: prepareFixture(t, c),
		addr:        addr,
		done:        make(chan error, 1),
	}
	ctx, cancel := context.WithCancel(context.Background())
	f.cancel = cancel
	ready := make(chan struct{})
	go func() { f.done <- c.Run(ctx, func() { close(ready) }) }()
	select {
	case <-ready:
	case err := <-f.done:
		t.Fatalf("Run returned before readiness: %v", err)
	case <-time.After(waitFor):
		t.Fatal("Run did not become ready")
	}
	t.Cleanup(func() { f.stop(t) })
	return f
}

// stop cancels Run and waits for it to join every reader.
func (f *runtimeFixture) stop(t testing.TB) error {
	t.Helper()
	f.cancel()
	select {
	case err := <-f.done:
		f.done <- err // Keep the result for idempotent stop calls.
		return err
	case <-time.After(waitFor):
		t.Fatal("Run did not return after cancellation")
		return nil
	}
}

func (f *runtimeFixture) sendUDP(t testing.TB, payload string) {
	t.Helper()
	conn, err := net.Dial(protocolUDP, f.addr)
	require.NoError(t, err)
	defer conn.Close()
	_, err = conn.Write([]byte(payload))
	require.NoError(t, err)
}

func (f *runtimeFixture) dialTCP(t testing.TB) net.Conn {
	t.Helper()
	conn, err := net.Dial(protocolTCP, f.addr)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

// receiverCounts are the receiver's accepted and nonzero rejected totals.
type receiverCounts struct {
	accepted uint64
	rejected map[rejection]uint64
}

func (f *coreFixture) counts() receiverCounts {
	r := f.c.receiver
	r.mu.Lock()
	defer r.mu.Unlock()
	out := receiverCounts{
		accepted: r.accepted,
		rejected: make(map[rejection]uint64),
	}
	for i, n := range r.rejects {
		if n != 0 {
			out.rejected[rejectReasons[i]] = n
		}
	}
	return out
}

// waitCounts waits until receiver totals reach exactly the wanted state.
func (f *coreFixture) waitCounts(t testing.TB, want receiverCounts) {
	t.Helper()
	if want.rejected == nil {
		want.rejected = map[rejection]uint64{}
	}
	require.Eventually(t, func() bool {
		got := f.counts()
		return got.accepted == want.accepted && fmt.Sprint(got.rejected) == fmt.Sprint(want.rejected)
	}, waitFor, time.Millisecond)
	// Settle: no further records arrive afterwards.
	time.Sleep(20 * time.Millisecond)
	assert.Equal(t, want, f.counts())
}
