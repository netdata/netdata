// SPDX-License-Identifier: GPL-3.0-or-later

package listen

import (
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReceiverDiagnostics(t *testing.T) {
	f := startRuntime(t, nil)
	udp := "a:1|c\nbad\n"
	f.sendUDP(t, udp)
	conn := f.dialTCP(t)
	tcp := "b:1|g\nb:1|c\nlatency:10|h\n"
	_, err := conn.Write([]byte(tcp))
	require.NoError(t, err)
	f.waitCounts(t, receiverCounts{
		accepted: 3,
		rejected: map[rejection]uint64{rejectSyntax: 1, rejectType: 1},
	})
	tcpBytes := func() bool { return f.c.diagnostics.tcpBytes.Load() == uint64(len(tcp)) }
	require.Eventually(t, tcpBytes, waitFor, time.Millisecond)
	f.collect(t, false, false)
	value(t, f.c, "receiver.bytes", float64(len(udp)), metrix.Labels{
		"transport": protocolUDP,
	})
	value(t, f.c, "receiver.bytes", float64(len(tcp)), metrix.Labels{
		"transport": protocolTCP,
	})
	value(t, f.c, "receiver.updates", 3, nil)
	for _, reason := range rejectReasons {
		want := map[rejection]float64{rejectSyntax: 1, rejectType: 1}[reason]
		value(t, f.c, "receiver.rejections", want, metrix.Labels{
			"reason": string(reason),
		})
	}
	value(t, f.c, "receiver.series", 3, nil)
	value(t, f.c, "receiver.series_limit", 1000, nil)
	value(t, f.c, "receiver.tcp_connections", 1, nil)

	// A quiet interval is zero activity, not failure; totals repeat.
	f.collect(t, false, false)
	value(t, f.c, "receiver.updates", 3, nil)
}

func TestWithheldPercentileDiagnostics(t *testing.T) {
	f := newCoreFixture(t, 3, time.Minute)
	f.ingest(t, "wide:1|h", "wide:1e20|h", "tiny:1|ms|@1e-40", "fine:5|ms")
	f.collect(t, false, false)
	for _, reason := range withheldReasons {
		want := map[string]float64{withheldSpan: 1, withheldNumericDomain: 1}[reason]
		value(t, f.c, "receiver.percentiles_withheld", want, metrix.Labels{
			"reason": reason,
		})
	}
	// Empty windows have no percentiles to withhold.
	f.collect(t, false, false)
	value(t, f.c, "receiver.percentiles_withheld", 1, metrix.Labels{
		"reason": withheldSpan,
	})
	// TCP-only diagnostics are not written for a UDP-only job.
	_, ok := f.c.store.Read().Value("receiver.tcp_connections", nil)
	assert.False(t, ok)
	_, ok = f.c.store.Read().Value("receiver.bytes", metrix.Labels{
		"transport": protocolTCP,
	})
	assert.False(t, ok)
}

func TestDiagnosticsChartTemplate(t *testing.T) {
	collecttest.AssertChartTemplateSchema(t, string(chartsYAML))
	entry, err := diagnosticsTemplate()
	require.NoError(t, err)
	assert.Equal(t, contextNamespace, entry.ContextNamespace)
	// Every diagnostic the collector writes is charted: a TCP job materializes all of them.
	f := startRuntime(t, nil)
	f.collect(t, false, false)
	collecttest.AssertChartCoverage(t, f.c, collecttest.ChartCoverageExpectation{})
}
