// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux && realhost

// This file is gated behind the `realhost` build tag and is NOT part of the default unit-test
// path. It exercises the collector against the machine it runs on, executing the real ras-mc-ctl
// and reading the real EDAC sysfs tree.
//
//	go test -tags realhost -v -run TestRealHost ./plugin/go.d/collector/rasdaemon/
//
// It is the only place the collector touches live system state, which is why it is separated:
// unit tests must stay hermetic.
package rasdaemon

import (
	"context"
	"os"
	"testing"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
	"github.com/stretchr/testify/require"
)

// ndRunCandidates are locations of netdata's nd-run helper. RunUnprivileged executes through it,
// and its path is baked in at build time, so a `go test` binary built outside a netdata install
// cannot find it. Production is unaffected: nd-run ships with the agent.
var ndRunCandidates = []string{
	"/usr/sbin/nd-run",
	"/opt/sw/netdata/usr/sbin/nd-run",
	"/opt/netdata/usr/sbin/nd-run",
	"/usr/local/netdata/usr/sbin/nd-run",
}

func useRealNdRun(t *testing.T) {
	t.Helper()
	for _, p := range ndRunCandidates {
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			restore := ndexec.SetRunnerPathsForTests(p, "")
			t.Cleanup(restore)
			t.Logf("using nd-run at %s", p)
			return
		}
	}
	t.Skipf("nd-run not found in %v; cannot exercise the production RunUnprivileged path", ndRunCandidates)
}

func TestRealHost_RasdaemonSource(t *testing.T) {
	useRealNdRun(t)

	collr := New()
	collr.Logger = logger.New()

	if err := collr.Init(context.Background()); err != nil {
		t.Skipf("ras-mc-ctl not available on this host: %v", err)
	}

	require.NoError(t, collr.Check(context.Background()), "Check against the real ras-mc-ctl")

	cc := mustCycleController(t, collr.MetricStore())
	cc.BeginCycle()
	require.NoError(t, collr.Collect(context.Background()))
	require.NoError(t, cc.CommitCycleSuccess())

	r := collr.MetricStore().Read(metrix.ReadRaw())

	// The always-present summary series must exist even on a machine that has never recorded a
	// RAS error. This is the property that makes "healthy" distinguishable from "collector broken".
	for _, sev := range memorySeverities {
		v, ok := r.Value(metricMemoryErrors, metrix.Labels{"severity": sev})
		require.Truef(t, ok, "missing %s{severity=%s} on a real host", metricMemoryErrors, sev)
		t.Logf("%s{severity=%s} = %v", metricMemoryErrors, sev, v)
	}
	for _, cls := range allClasses {
		v, ok := r.Value(metricClassEvents, metrix.Labels{"class": cls})
		require.Truef(t, ok, "missing %s{class=%s}", metricClassEvents, cls)
		t.Logf("%s{class=%s} = %v", metricClassEvents, cls, v)
	}
	v, ok := r.Value(metricMCERecords, nil)
	require.True(t, ok)
	t.Logf("%s = %v", metricMCERecords, v)
}
