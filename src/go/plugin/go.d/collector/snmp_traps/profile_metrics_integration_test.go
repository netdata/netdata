// SPDX-License-Identifier: GPL-3.0-or-later

package snmp_traps

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/attribution"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/catalog"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/profilemetrics"
	"github.com/stretchr/testify/require"
)

func TestProfileMetricsUpdateAfterCommittedTrapOnly(t *testing.T) {
	paths := rootTestProfileCatalogPaths(t)
	writeProfileYAML(t, paths.UserDirs[0], "profile.yaml", `
traps:
  - oid: 1.3.6.1.6.3.1.1.5.1
    name: SNMPv2-MIB::coldStart
    category: state_change
    severity: warning
charts:
  - id: cold_start
    title: SNMP cold start
    context: snmp.trap.cold.start
    units: events/s
    algorithm: incremental
metrics:
  - name: snmp.cold_start
    type: counter
    on_trap: SNMPv2-MIB::coldStart
    output:
      metric: snmp_trap_cold_start_events
      dimension: events
      chart: cold_start
`)

	lease, err := catalog.NewManager(paths).Acquire()
	require.NoError(t, err)
	t.Cleanup(lease.Close)
	policy, err := profilemetrics.Normalize(true, []string{"snmp.cold_start"})
	require.NoError(t, err)
	rt, err := profilemetrics.New(policy, lease.Epoch(), profilemetrics.Options{
		BaseChartTemplateYAML: chartTemplateYAML,
		SourceHashSalt:        "test",
	})
	require.NoError(t, err)
	require.NotNil(t, rt)

	packet := readColdStartUDPPacket(t)
	failedWriter := &mockTrapWriter{err: errors.New("write failed")}
	c := newDefaultTestV2Collector(failedWriter)
	c.profileIndex = lease.Epoch()
	c.profileMetrics = rt
	c.handlePacket(packet.Payload, packet.Peer, nil, nil)

	store := metrix.NewCollectorStore()
	collectProfileMetricsForTest(t, rt, store, "test")
	if _, ok := store.Read().Family("snmp_trap_cold_start_events"); ok {
		t.Fatal("profile metric was updated after a failed authoritative write")
	}

	successWriter := &mockTrapWriter{}
	c.trapWriter = successWriter
	c.handlePacket(packet.Payload, packet.Peer, nil, nil)
	require.Len(t, successWriter.entries, 1)

	collectProfileMetricsForTest(t, rt, store, "test")
	labels := rootTestProfileMetricSourceLabels(successWriter.entries[0], "test")
	if value, ok := store.Read().Value("snmp_trap_cold_start_events", labels); !ok || value != 1 {
		t.Fatalf("snmp_trap_cold_start_events = %v/%v, want 1/true", value, ok)
	}

	dedupWriter := &mockTrapWriter{}
	dedupCollector, _ := newDedupTestV2Collector(t, "test", dedupWriter)
	dedupCollector.profileIndex = lease.Epoch()
	dedupCollector.profileMetrics = rt
	dedupCollector.deduper.start()
	t.Cleanup(dedupCollector.deduper.Close)

	dedupCollector.handlePacket(packet.Payload, packet.Peer, nil, nil)
	dedupCollector.handlePacket(packet.Payload, packet.Peer, nil, nil)
	require.Len(t, dedupWriter.entries, 1)

	collectProfileMetricsForTest(t, rt, store, "test")
	if value, ok := store.Read().Value("snmp_trap_cold_start_events", labels); !ok || value != 2 {
		t.Fatalf("snmp_trap_cold_start_events after duplicate = %v/%v, want 2/true", value, ok)
	}
}

func collectProfileMetricsForTest(t *testing.T, rt *profilemetrics.Runtime, store metrix.CollectorStore, jobName string) {
	t.Helper()
	managed, ok := metrix.AsCycleManagedStore(store)
	require.True(t, ok)
	managed.CycleController().BeginCycle()
	rt.Collect(store, jobName)
	require.NoError(t, managed.CycleController().CommitCycleSuccess())
}

func newRootTestProfileMetricRuntime(t *testing.T) *profilemetrics.Runtime {
	t.Helper()
	paths := rootTestProfileCatalogPaths(t)
	writeRootTestProfileMetricProfile(t, paths.UserDirs[0])
	lease, err := catalog.NewManager(paths).Acquire()
	require.NoError(t, err)
	t.Cleanup(lease.Close)
	policy, err := profilemetrics.Normalize(true, []string{"cisco.config.changed"})
	require.NoError(t, err)
	rt, err := profilemetrics.New(policy, lease.Epoch(), profilemetrics.Options{
		BaseChartTemplateYAML: chartTemplateYAML,
		SourceHashSalt:        "test",
	})
	require.NoError(t, err)
	require.NotNil(t, rt)
	return rt
}

func writeRootTestProfileMetricProfile(t *testing.T, dir string) {
	t.Helper()
	writeProfileYAML(t, dir, "profile.yaml", `
varbinds:
  ccmHistoryEventTerminalType:
    oid: 1.3.6.1.4.1.9.9.43.1.1.1.2
    type: INTEGER
traps:
  - oid: 1.3.6.1.4.1.9.9.43.2.0.1
    name: CISCO-CONFIG-MAN-MIB::ccmCLIRunningConfigChanged
    category: config_change
    severity: notice
    varbinds:
      - ccmHistoryEventTerminalType
charts:
  - id: cisco_config_changes
    title: Cisco config changes
    context: snmp.trap.cisco.config.changes
    units: events/s
    algorithm: incremental
metrics:
  - name: cisco.config.changed
    type: counter
    on_trap: CISCO-CONFIG-MAN-MIB::ccmCLIRunningConfigChanged
    output:
      metric: snmp_trap_cisco_config_events
      dimension: events
      chart: cisco_config_changes
`)
}

func rootTestProfileCatalogPaths(t *testing.T) catalog.Paths {
	t.Helper()
	root := t.TempDir()
	userDir := filepath.Join(root, "user")
	stockDir := filepath.Join(root, "default")
	require.NoError(t, os.MkdirAll(userDir, 0o755))
	require.NoError(t, os.MkdirAll(stockDir, 0o755))
	writeProfileYAML(t, stockDir, "minimal.yaml", `
traps:
  - oid: 1.3.6.1.6.3.1.1.5.3
    name: IF-MIB::linkDown
    category: state_change
    severity: warning
`)
	writeProfileCatalogue(t, stockDir, map[string]any{
		"minimal": map[string]any{
			"file":      "minimal.yaml",
			"mibs":      []string{"IF-MIB"},
			"trap_oids": []string{"1.3.6.1.6.3.1.1.5.3"},
		},
	})
	return catalog.Paths{UserDirs: []string{userDir}, StockDir: stockDir}
}

func rootTestCiscoConfigTrapEntry(jobName string) *TrapEntry {
	return &TrapEntry{
		JobName:       jobName,
		TrapOID:       "1.3.6.1.4.1.9.9.43.2.0.1",
		TrapName:      "CISCO-CONFIG-MAN-MIB::ccmCLIRunningConfigChanged",
		SourceIP:      "192.0.2.10",
		SourceUDPPeer: "192.0.2.10",
		Enrichment: &TrapEnrichmentAudit{Source: &TrapSourceAudit{
			Selected: "192.0.2.10",
			Method:   "udp_peer",
		}},
		Varbinds: []VarbindValue{{
			OID:   "1.3.6.1.4.1.9.9.43.1.1.1.2",
			Type:  "INTEGER",
			Value: 2,
		}},
	}
}

func rootTestProfileMetricSourceLabels(entry *TrapEntry, sourceHashSalt string) metrix.Labels {
	source, ok := attribution.Resolve(entry, entry.JobName, attribution.DeviceSource, sourceHashSalt)
	if !ok {
		panic("test trap entry has no source identity")
	}
	return metrix.Labels{
		"job_name":    entry.JobName,
		"source_id":   source.Key.SourceID,
		"source_kind": source.Key.SourceKind,
	}
}
