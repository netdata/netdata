// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux

package rasdaemon

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	dataConfigJSON, _ = os.ReadFile("testdata/config.json")
	dataConfigYAML, _ = os.ReadFile("testdata/config.yaml")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON": dataConfigJSON,
		"dataConfigYAML": dataConfigYAML,
	} {
		require.NotNil(t, data, name)
	}
}

// mockRasMcCtl returns canned bytes instead of executing ras-mc-ctl.
type mockRasMcCtl struct {
	out   []byte
	err   error
	calls int
}

func (m *mockRasMcCtl) summary() ([]byte, error) {
	m.calls++
	if m.err != nil {
		return nil, m.err
	}
	return m.out, nil
}

func newTestCollector(cli rasMcCtlCli) *Collector {
	collr := New()
	collr.exec = cli
	return collr
}

func mustCycleController(t *testing.T, store metrix.CollectorStore) metrix.CycleController {
	t.Helper()
	managed, ok := metrix.AsCycleManagedStore(store)
	require.True(t, ok, "store does not expose cycle control")
	return managed.CycleController()
}

// TestCollector_ChartTemplateYAML validates the shipped chart template against the schema and
// compiles it through the same chartengine path the runtime uses.
func TestCollector_ChartTemplateYAML(t *testing.T) {
	templateYAML := New().ChartTemplateYAML()
	collecttest.AssertChartTemplateSchema(t, templateYAML)

	spec, err := charttpl.DecodeYAML([]byte(templateYAML))
	require.NoError(t, err)
	require.NoError(t, spec.Validate())

	_, err = chartengine.Compile(spec, 1)
	require.NoError(t, err)
}

func TestCollector_ConfigurationSerialize(t *testing.T) {
	collecttest.TestConfigurationSerialize(t, &Collector{}, dataConfigJSON, dataConfigYAML)
}

func TestCollector_Init(t *testing.T) {
	tests := map[string]struct {
		timeout    confopt.Duration
		binaryPath string
		wantFail   bool
	}{
		"explicit binary path is accepted as-is": {
			timeout:    confopt.Duration(time_second),
			binaryPath: "/nonexistent/ras-mc-ctl",
		},
		"non-positive timeout is rejected": {
			timeout:  0,
			wantFail: true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr := New()
			collr.Timeout = test.timeout
			collr.BinaryPath = test.binaryPath

			err := collr.Init(context.Background())
			if test.wantFail {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestCollector_InitFailsWithoutRasMcCtl pins that the collector does NOT autodetect when
// ras-mc-ctl is absent. It deliberately has no sysfs fallback: the Netdata Agent already
// collects EDAC sysfs counters internally (proc.plugin, context mem.edac_mc_dimm_errors,
// including a dimm_label chart label), so a fallback would double-collect them.
func TestCollector_InitFailsWithoutRasMcCtl(t *testing.T) {
	origNames, origPaths := rasMcCtlBinaryNames, rasMcCtlFallbackPaths
	rasMcCtlBinaryNames = []string{"ras-mc-ctl-does-not-exist-" + t.Name()}
	rasMcCtlFallbackPaths = []string{filepath.Join(t.TempDir(), "absent")}
	t.Cleanup(func() { rasMcCtlBinaryNames, rasMcCtlFallbackPaths = origNames, origPaths })

	assert.Error(t, New().Init(context.Background()))
}

func TestCollector_CollectKnownBad(t *testing.T) {
	out, err := os.ReadFile("testdata/summary-with-errors.txt")
	require.NoError(t, err)

	collr := newTestCollector(&mockRasMcCtl{out: out})
	cc := mustCycleController(t, collr.MetricStore())
	cc.BeginCycle()
	require.NoError(t, collr.Collect(context.Background()))
	require.NoError(t, cc.CommitCycleSuccess())

	r := collr.MetricStore().Read(metrix.ReadRaw())

	// The failing stick must appear ONCE with both csrows summed (2 + 1). Reporting 2 here
	// would under-count a pre-failure DIMM by a third.
	assertMetric(t, r, metricMCEvents, metrix.Labels{"dimm": "DDR4_A1", "severity": sevCorrected}, 3)
	assertMetric(t, r, metricMCEvents, metrix.Labels{"dimm": "DDR4_B1", "severity": sevCorrected}, 1)
	assertMetric(t, r, metricMCEvents, metrix.Labels{"dimm": "DDR4_G1", "severity": sevUncorrected}, 1)

	// Aggregate memory totals across all DIMMs.
	assertMetric(t, r, metricMemoryErrors, metrix.Labels{"severity": sevCorrected}, 4)
	assertMetric(t, r, metricMemoryErrors, metrix.Labels{"severity": sevUncorrected}, 1)

	assertMetric(t, r, metricAEREvents, metrix.Labels{"severity": sevCorrected}, 2)
	assertMetric(t, r, metricAEREvents, metrix.Labels{"severity": sevFatal}, 1)

	assertMetric(t, r, metricMCERecords, nil, 2)
	assertMetric(t, r, metricMemoryFailureEvents, nil, 1)

	assertMetric(t, r, metricClassEvents, metrix.Labels{"class": classDisk}, 1)
	assertMetric(t, r, metricClassEvents, metrix.Labels{"class": classDevlink}, 1)
	assertMetric(t, r, metricClassEvents, metrix.Labels{"class": classSignal}, 1)

	// A DIMM with no recorded errors must not materialize a series at all.
	_, ok := r.Value(metricMCEvents, metrix.Labels{"dimm": "DDR4_H1", "severity": sevCorrected})
	assert.False(t, ok, "healthy DIMM should not produce a per-DIMM series")
}

func TestCollector_CollectKnownGood(t *testing.T) {
	out, err := os.ReadFile("testdata/summary-no-errors.txt")
	require.NoError(t, err)

	collr := newTestCollector(&mockRasMcCtl{out: out})
	cc := mustCycleController(t, collr.MetricStore())
	cc.BeginCycle()
	// A healthy machine parses cleanly and simply produces no series.
	require.NoError(t, collr.Collect(context.Background()))
	require.NoError(t, cc.CommitCycleSuccess())

	r := collr.MetricStore().Read(metrix.ReadRaw())

	// The always-present summary series MUST exist and read zero, so a healthy machine is
	// distinguishable from a broken collector and alerts have a context to attach to.
	for _, sev := range memorySeverities {
		assertMetric(t, r, metricMemoryErrors, metrix.Labels{"severity": sev}, 0)
	}
	for _, sev := range aerSeverities {
		assertMetric(t, r, metricAEREvents, metrix.Labels{"severity": sev}, 0)
	}
	for _, cls := range allClasses {
		assertMetric(t, r, metricClassEvents, metrix.Labels{"class": cls}, 0)
	}
	assertMetric(t, r, metricMCERecords, nil, 0)
	assertMetric(t, r, metricMemoryFailureEvents, nil, 0)

	// No DIMM has errors, so no per-DIMM instance should exist.
	_, ok := r.Value(metricMCEvents, metrix.Labels{"dimm": "DDR4_A1", "severity": sevCorrected})
	assert.False(t, ok)
}

func TestCanonicalSeverity(t *testing.T) {
	tests := map[string]string{
		"Corrected":               sevCorrected,
		"corrected":               sevCorrected,
		"Correctable":             sevCorrected,
		"Uncorrected":             sevUncorrected,
		"Uncorrected (Non-Fatal)": sevUncorrected,
		"Uncorrectable":           sevUncorrected,
		"non-fatal":               sevUncorrected,
		"Fatal":                   sevFatal,
		"Uncorrected (Fatal)":     sevUncorrected, // "uncorrected" wins; it is the actionable fact
		"Deferred":                sevOther,
		"":                        sevOther,
	}
	for in, want := range tests {
		t.Run(in, func(t *testing.T) {
			assert.Equal(t, want, canonicalSeverity(in))
		})
	}
}

// TestCollector_CollectFailures asserts the collector reports an error rather than silently
// publishing zeros. A zero here reads as "no RAS errors" on a host whose RAS pipeline is
// actually broken — precisely the failure this collector exists to catch.
func TestCollector_CollectFailures(t *testing.T) {
	tests := map[string]struct {
		cli rasMcCtlCli
	}{
		"exec failure": {
			cli: &mockRasMcCtl{err: errors.New("ras-mc-ctl: command not found")},
		},
		"empty output (ras-mc-ctl died on a missing table)": {
			cli: &mockRasMcCtl{out: []byte("")},
		},
		"unparsable output": {
			cli: &mockRasMcCtl{out: []byte("something entirely unexpected\n")},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr := newTestCollector(test.cli)
			cc := mustCycleController(t, collr.MetricStore())
			cc.BeginCycle()
			assert.Error(t, collr.Collect(context.Background()))
			cc.AbortCycle()

			assert.Error(t, collr.Check(context.Background()))
		})
	}
}

func TestCollector_CollectContextCancelled(t *testing.T) {
	out, err := os.ReadFile("testdata/summary-with-errors.txt")
	require.NoError(t, err)

	collr := newTestCollector(&mockRasMcCtl{out: out})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	cc := mustCycleController(t, collr.MetricStore())
	cc.BeginCycle()
	assert.ErrorIs(t, collr.Collect(ctx), context.Canceled)
	cc.AbortCycle()
}

func TestCollector_CleanupIsIdempotent(t *testing.T) {
	collr := newTestCollector(&mockRasMcCtl{out: []byte("No Memory errors.\n")})
	assert.NotPanics(t, func() {
		collr.Cleanup(context.Background())
		collr.Cleanup(context.Background())
	})
}

func TestCollector_MetricStoreNotNil(t *testing.T) {
	assert.NotNil(t, New().MetricStore())
}

func assertMetric(t *testing.T, r metrix.Reader, name string, labels metrix.Labels, want float64) {
	t.Helper()
	got, ok := r.Value(name, labels)
	require.Truef(t, ok, "expected metric %s labels=%v", name, labels)
	assert.InDeltaf(t, want, got, 1e-9, "unexpected value for %s labels=%v", name, labels)
}

const time_second = 1_000_000_000
