// SPDX-License-Identifier: GPL-3.0-or-later

package profilemetrics

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/catalog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProfileMetricRuntimeHydratesOnlySelectedStockRule(t *testing.T) {
	root := t.TempDir()
	stockDir := filepath.Join(root, "default")
	require.NoError(t, os.MkdirAll(stockDir, 0o755))
	writeProfileYAML(t, stockDir, "stock.yaml", `
traps:
  - oid: 1.3.6.1.6.3.1.1.5.1
    name: SNMPv2-MIB::coldStart
    category: state_change
    severity: warning
charts:
  - id: stock_cold_start
    title: Stock cold start
    context: snmp.trap.stock.cold.start
    units: events/s
    algorithm: incremental
metrics:
  - name: stock.cold_start
    type: counter
    on_trap: SNMPv2-MIB::coldStart
    output:
      metric: snmp_trap_stock_cold_start_events
      dimension: events
      chart: stock_cold_start
`)
	writeProfileYAML(t, stockDir, "unselected.yaml", `not: [valid`)
	writeProfileCatalogue(t, stockDir, map[string]any{
		"stock": map[string]any{
			"file":              "stock.yaml",
			"mibs":              []string{"SNMPv2-MIB"},
			"metric_rule_names": []string{"stock.cold_start"},
			"trap_oids":         []string{"1.3.6.1.6.3.1.1.5.1"},
		},
		"unselected": map[string]any{
			"file":      "unselected.yaml",
			"mibs":      []string{"UNSELECTED-MIB"},
			"trap_oids": []string{"1.3.6.1.4.1.99997.1"},
		},
	})

	lease, err := catalog.NewManager(catalog.Paths{StockDir: stockDir}).Acquire()
	require.NoError(t, err, "stock profile bodies must remain lazy at acquisition")
	t.Cleanup(lease.Close)
	idx := lease.Epoch()
	defs, err := idx.Definitions(nil)
	require.NoError(t, err)
	assert.Empty(t, defs.RulesByName)

	rt := newTestProfileMetricRuntimeWithConfig(t, idx, testRuntimeConfig{
		Enabled: true,
		Include: []string{"stock.cold_start"},
	})
	require.Len(t, rt.rules, 1)
	defs, err = idx.Definitions([]string{"stock.cold_start"})
	require.NoError(t, err)
	assert.NotNil(t, defs.RulesByName["stock.cold_start"])
}

func TestProfileMetricUserRuleCanReferenceStockTrapName(t *testing.T) {
	root := t.TempDir()
	stockDir := filepath.Join(root, "default")
	userDir := t.TempDir()
	require.NoError(t, os.MkdirAll(stockDir, 0o755))
	writeProfileYAML(t, stockDir, "stock.yaml", `
traps:
  - oid: 1.3.6.1.6.3.1.1.5.1
    name: SNMPv2-MIB::coldStart
    category: state_change
    severity: warning
`)
	writeProfileCatalogue(t, stockDir, map[string]any{
		"stock": map[string]any{
			"file":      "stock.yaml",
			"mibs":      []string{"SNMPv2-MIB"},
			"trap_oids": []string{"1.3.6.1.6.3.1.1.5.1"},
		},
	})
	writeProfileYAML(t, userDir, "site.yaml", `
charts:
  - id: site_cold_start
    title: Site cold start
    context: snmp.trap.site.cold_start
    units: events/s
    algorithm: incremental
metrics:
  - name: site.cold_start
    type: counter
    on_trap: SNMPv2-MIB::coldStart
    output:
      metric: snmp_trap_site_cold_start_events
      dimension: events
      chart: site_cold_start
`)

	lease, err := catalog.NewManager(catalog.Paths{UserDirs: []string{userDir}, StockDir: stockDir}).Acquire()
	require.NoError(t, err)
	t.Cleanup(lease.Close)
	idx := lease.Epoch()
	rt := newTestProfileMetricRuntimeWithConfig(t, idx, testRuntimeConfig{
		Enabled: true,
		Include: []string{"site.cold_start"},
	})
	entry := &testTrapEntry{
		JobName:       "profile-job",
		TrapOID:       "1.3.6.1.6.3.1.1.5.1",
		TrapName:      "SNMPv2-MIB::coldStart",
		SourceIP:      "192.0.2.30",
		SourceUDPPeer: "192.0.2.30",
		Enrichment: &testTrapEnrichmentAudit{Source: &testTrapSourceAudit{
			Selected: "192.0.2.30",
			Method:   "udp_peer",
		}},
	}

	rt.Update(entry)
	store := metrix.NewCollectorStore()
	collectProfileMetricsOnce(t, rt, store, "profile-job")
	labels := profileMetricSourceLabels("192.0.2.30")
	if v, ok := store.Read().Value("snmp_trap_site_cold_start_events", labels); !ok || v != 1 {
		t.Fatalf("snmp_trap_site_cold_start_events = %v/%v, want 1/true", v, ok)
	}
}
