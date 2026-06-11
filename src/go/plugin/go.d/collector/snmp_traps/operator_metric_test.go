// SPDX-License-Identifier: GPL-3.0-or-later

package snmp_traps

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
)

const (
	ciscoConfigTrapOID        = "1.3.6.1.4.1.9.9.43.2.0.1"
	ciscoCommandSourceOID     = "1.3.6.1.4.1.9.9.43.1.1.1.1"
	ciscoTerminalTypeOID      = "1.3.6.1.4.1.9.9.43.1.1.1.2"
	ciscoTerminalUserOID      = "1.3.6.1.4.1.9.9.43.1.1.1.3"
	ciscoEventEnabledOID      = "1.3.6.1.4.1.9.9.43.1.1.1.4"
	ciscoLargeEnumOID         = "1.3.6.1.4.1.9.9.43.1.1.1.5"
	ciscoTerminalTypeVarbind  = "ccmHistoryEventTerminalType"
	ciscoCommandSourceVarbind = "ccmHistoryEventCommandSource"
)

func needCycleManagedStore(t *testing.T, store metrix.CollectorStore) metrix.CycleManagedStore {
	t.Helper()
	ms, ok := metrix.AsCycleManagedStore(store)
	if !ok {
		t.Fatalf("AsCycleManagedStore returned false")
	}
	return ms
}

func makeTestProfileIndex(t *testing.T) *ProfileIndex {
	t.Helper()
	return &ProfileIndex{
		trapsByOID: map[string]*TrapDef{
			ciscoConfigTrapOID: {
				OID:      ciscoConfigTrapOID,
				Name:     "CISCO-CONFIG-MAN-MIB::ccmCLIRunningConfigChanged",
				Category: "config_change",
				Severity: "notice",
				VarbindRefs: []any{
					ciscoCommandSourceVarbind,
					ciscoTerminalTypeVarbind,
					"ccmHistoryEventTerminalUser",
				},
				sharedVarbinds: map[string]*VarbindDef{
					ciscoCommandSourceOID: {
						OID:         ciscoCommandSourceOID,
						Type:        "INTEGER",
						rawName:     ciscoCommandSourceVarbind,
						Constraints: "(1..4)",
					},
					ciscoTerminalTypeOID: {
						OID:     ciscoTerminalTypeOID,
						Type:    "INTEGER",
						rawName: ciscoTerminalTypeVarbind,
						Enum: map[string]string{
							"1": "none",
							"2": "console",
							"3": "virtual",
							"4": "aux",
						},
					},
					ciscoTerminalUserOID: {
						OID:     ciscoTerminalUserOID,
						Type:    "OctetString",
						rawName: "ccmHistoryEventTerminalUser",
					},
					ciscoEventEnabledOID: {
						OID:     ciscoEventEnabledOID,
						Type:    "TruthValue",
						rawName: "ccmHistoryEventEnabled",
					},
					ciscoLargeEnumOID: {
						OID:     ciscoLargeEnumOID,
						Type:    "INTEGER",
						rawName: "ccmHistoryEventLargeEnum",
						Enum:    testEnum(maxBoundedVarbindValues + 1),
					},
				},
			},
			"1.3.6.1.4.1.9.9.46.2.0.1": {
				OID:      "1.3.6.1.4.1.9.9.46.2.0.1",
				Name:     "CISCO-PORT-SECURITY-MIB::cpsSecureMacAddrViolation",
				Category: "security",
				Severity: "warning",
				VarbindRefs: []any{
					"ifIndex",
					"cpsIfViolationMacAddress",
				},
				sharedVarbinds: map[string]*VarbindDef{
					"1.3.6.1.2.1.2.2.1.1": {
						OID:         "1.3.6.1.2.1.2.2.1.1",
						Type:        "INTEGER",
						rawName:     "ifIndex",
						Constraints: "(1..48)",
					},
					"1.3.6.1.4.1.9.9.315.1.2.1.1.1": {
						OID:     "1.3.6.1.4.1.9.9.315.1.2.1.1.1",
						Type:    "OctetString",
						rawName: "cpsIfViolationMacAddress",
					},
				},
			},
			"1.3.6.1.6.3.1.1.5.1": {
				OID:      "1.3.6.1.6.3.1.1.5.1",
				Name:     "SNMPv2-MIB::coldStart",
				Category: "state_change",
				Severity: "warning",
				VarbindRefs: []any{
					"snmpTrapOID",
				},
				sharedVarbinds: map[string]*VarbindDef{
					"1.3.6.1.6.3.1.1.4.1": {
						OID:     "1.3.6.1.6.3.1.1.4.1",
						Type:    "OBJECTID",
						rawName: "snmpTrapOID",
					},
				},
			},
		},
	}
}

func testEnum(count int) map[string]string {
	enum := make(map[string]string, count)
	for i := 1; i <= count; i++ {
		enum[strconv.Itoa(i)] = "value_" + strconv.Itoa(i)
	}
	return enum
}

func newTestOperatorMetrics(t *testing.T, cfg []MetricConfig) *operatorMetrics {
	t.Helper()
	return newOperatorMetrics(cfg, makeTestProfileIndex(t))
}

func ciscoConfigMetric(context, dimension string) MetricConfig {
	cfg := MetricConfig{OID: ciscoConfigTrapOID, Context: context}
	if dimension != "" {
		cfg.DimensionFromVarbind = dimension
	}
	return cfg
}

func ciscoConfigEntry(varbindOID string, value any, name string) *TrapEntry {
	return &TrapEntry{
		TrapOID: ciscoConfigTrapOID,
		Varbinds: []VarbindValue{
			{OID: varbindOID, Value: value, Name: name},
		},
	}
}

func ciscoConfigReloadedTrap(varbinds map[string]*VarbindDef) *TrapDef {
	return &TrapDef{
		OID:            ciscoConfigTrapOID,
		Name:           "CISCO-CONFIG-MAN-MIB::ccmCLIRunningConfigChanged",
		sharedVarbinds: varbinds,
	}
}

func assertMetricDimensionBucket(t *testing.T, om *operatorMetrics, rawValue, bucket, reason string) {
	t.Helper()

	om.metrics[0].dimMu.Lock()
	_, rawSeen := om.metrics[0].dimCounts[rawValue]
	ctr, bucketSeen := om.metrics[0].dimCounts[bucket]
	om.metrics[0].dimMu.Unlock()
	if rawSeen {
		t.Fatalf("%s leaked raw metric dimension %q", reason, rawValue)
	}
	if !bucketSeen || ctr.Load() != 1 {
		count := uint64(0)
		if ctr != nil {
			count = ctr.Load()
		}
		t.Fatalf("%s bucket %q = seen %v count %d, want seen true count 1", reason, bucket, bucketSeen, count)
	}
}

func TestValidateMetricsSuccess(t *testing.T) {
	idx := makeTestProfileIndex(t)

	tests := map[string][]MetricConfig{
		"single-dim metric": {
			{OID: "1.3.6.1.4.1.9.9.43.2.0.1", Context: "snmp.trap.cisco_config_changes"},
		},
		"dimension_from_varbind enum-backed": {
			{OID: "1.3.6.1.4.1.9.9.43.2.0.1", Context: "snmp.trap.cisco_config", DimensionFromVarbind: "ccmHistoryEventTerminalType"},
		},
		"dimension_from_varbind numeric-range": {
			{OID: "1.3.6.1.4.1.9.9.43.2.0.1", Context: "snmp.trap.cisco_config", DimensionFromVarbind: "ccmHistoryEventCommandSource"},
		},
		"dimension_from_varbind integer-range on port-security ifIndex": {
			{OID: "1.3.6.1.4.1.9.9.46.2.0.1", Context: "snmp.trap.port_security", DimensionFromVarbind: "ifIndex"},
		},
		"dimension_from_varbind boolean truthvalue": {
			{OID: "1.3.6.1.4.1.9.9.43.2.0.1", Context: "snmp.trap.cisco_config_enabled", DimensionFromVarbind: "ccmHistoryEventEnabled"},
		},
		"multiple metrics": {
			{OID: "1.3.6.1.4.1.9.9.43.2.0.1", Context: "snmp.trap.config"},
			{OID: "1.3.6.1.6.3.1.1.5.1", Context: "snmp.trap.cold_start"},
		},
		"SMIv1 and SMIv2 alternate OID spelling": {
			{OID: "1.3.6.1.6.3.1.1.5.0.1", Context: "snmp.trap.cold_start"},
		},
	}

	for name, cfg := range tests {
		t.Run(name, func(t *testing.T) {
			if err := validateMetrics(cfg, idx); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestValidateMetricsFailures(t *testing.T) {
	idx := makeTestProfileIndex(t)

	tests := map[string]struct {
		cfg     []MetricConfig
		errText string
	}{
		"missing oid": {
			cfg:     []MetricConfig{{Context: "snmp.trap.test"}},
			errText: "oid is required",
		},
		"invalid oid": {
			cfg:     []MetricConfig{{OID: "not.an.oid", Context: "snmp.trap.test"}},
			errText: "invalid oid",
		},
		"oid not in profiles": {
			cfg:     []MetricConfig{{OID: "9.9.9.9.9.9.9.0.1", Context: "snmp.trap.test"}},
			errText: "not found in any loaded profile",
		},
		"missing context": {
			cfg:     []MetricConfig{{OID: "1.3.6.1.4.1.9.9.43.2.0.1"}},
			errText: "context is required",
		},
		"context prefix wrong": {
			cfg:     []MetricConfig{{OID: "1.3.6.1.4.1.9.9.43.2.0.1", Context: "my.custom.context"}},
			errText: "must start with",
		},
		"context suffix uppercase": {
			cfg:     []MetricConfig{{OID: "1.3.6.1.4.1.9.9.43.2.0.1", Context: "snmp.trap.Cisco_Config"}},
			errText: "invalid suffix",
		},
		"context suffix empty": {
			cfg:     []MetricConfig{{OID: "1.3.6.1.4.1.9.9.43.2.0.1", Context: "snmp.trap."}},
			errText: "has no suffix after",
		},
		"context suffix trailing dot": {
			cfg:     []MetricConfig{{OID: "1.3.6.1.4.1.9.9.43.2.0.1", Context: "snmp.trap.cisco."}},
			errText: "dot separators must not be empty",
		},
		"context suffix empty segment": {
			cfg:     []MetricConfig{{OID: "1.3.6.1.4.1.9.9.43.2.0.1", Context: "snmp.trap.cisco..config"}},
			errText: "dot separators must not be empty",
		},
		"dimension_from_varbind raw numeric OID": {
			cfg:     []MetricConfig{{OID: "1.3.6.1.4.1.9.9.43.2.0.1", Context: "snmp.trap.test", DimensionFromVarbind: "1.3.6.1.4.1.9.9.43.1.1.1.2"}},
			errText: "raw numeric OID",
		},
		"dimension_from_varbind unknown name": {
			cfg:     []MetricConfig{{OID: "1.3.6.1.4.1.9.9.43.2.0.1", Context: "snmp.trap.test", DimensionFromVarbind: "nonexistentVarbind"}},
			errText: "not found in trap definition",
		},
		"dimension_from_varbind unbounded octet string": {
			cfg:     []MetricConfig{{OID: "1.3.6.1.4.1.9.9.43.2.0.1", Context: "snmp.trap.test", DimensionFromVarbind: "ccmHistoryEventTerminalUser"}},
			errText: "unbounded varbind",
		},
		"dimension_from_varbind unbounded mac address": {
			cfg:     []MetricConfig{{OID: "1.3.6.1.4.1.9.9.46.2.0.1", Context: "snmp.trap.test", DimensionFromVarbind: "cpsIfViolationMacAddress"}},
			errText: "unbounded varbind",
		},
		"dimension_from_varbind enum over limit": {
			cfg:     []MetricConfig{{OID: "1.3.6.1.4.1.9.9.43.2.0.1", Context: "snmp.trap.test", DimensionFromVarbind: "ccmHistoryEventLargeEnum"}},
			errText: "unbounded varbind",
		},
		"duplicate oid": {
			cfg: []MetricConfig{
				{OID: "1.3.6.1.6.3.1.1.5.1", Context: "snmp.trap.cold_start_a"},
				{OID: "1.3.6.1.6.3.1.1.5.1", Context: "snmp.trap.cold_start_b"},
			},
			errText: "duplicate oid",
		},
		"duplicate alternate oid": {
			cfg: []MetricConfig{
				{OID: "1.3.6.1.6.3.1.1.5.1", Context: "snmp.trap.cold_start_a"},
				{OID: "1.3.6.1.6.3.1.1.5.0.1", Context: "snmp.trap.cold_start_b"},
			},
			errText: "already configured trap oid",
		},
		"duplicate context": {
			cfg: []MetricConfig{
				{OID: "1.3.6.1.6.3.1.1.5.1", Context: "snmp.trap.same"},
				{OID: "1.3.6.1.4.1.9.9.43.2.0.1", Context: "snmp.trap.same"},
			},
			errText: "duplicate context",
		},
		"duplicate generated selector": {
			cfg: []MetricConfig{
				{OID: "1.3.6.1.6.3.1.1.5.1", Context: "snmp.trap.foo.bar"},
				{OID: "1.3.6.1.4.1.9.9.43.2.0.1", Context: "snmp.trap.foo_bar"},
			},
			errText: "duplicate metric selector",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			err := validateMetrics(tc.cfg, idx)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tc.errText) {
				t.Fatalf("error %q does not contain %q", err.Error(), tc.errText)
			}
		})
	}
}

func TestValidateMetricsErrorsIncludeIndex(t *testing.T) {
	idx := makeTestProfileIndex(t)

	cfg := []MetricConfig{
		{OID: "1.3.6.1.4.1.9.9.43.2.0.1", Context: "snmp.trap.ok"},
		{OID: "not.an.oid", Context: "snmp.trap.fail"},
		{OID: "1.3.6.1.6.3.1.1.5.1", Context: "snmp.trap.also_ok"},
	}
	err := validateMetrics(cfg, idx)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "metrics[1]") {
		t.Fatalf("error does not include index: %v", err)
	}
}

func TestValidateMetricsNilIndex(t *testing.T) {
	err := validateMetrics([]MetricConfig{{OID: "1.2.3.0.1", Context: "snmp.trap.test"}}, nil)
	if err == nil {
		t.Fatal("expected error for nil index")
	}
}

func TestBuildChartTemplateYAMLNoMetrics(t *testing.T) {
	yaml, err := buildChartTemplateYAML(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if yaml == "" {
		t.Fatal("expected non-empty template")
	}
	collecttest.AssertChartTemplateSchema(t, yaml)
}

func TestBuildChartTemplateYAMLSingleDimension(t *testing.T) {
	cfg := []MetricConfig{
		{OID: "1.3.6.1.4.1.9.9.43.2.0.1", Context: "snmp.trap.cisco_config_changes"},
	}
	yaml, err := buildChartTemplateYAML(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	collecttest.AssertChartTemplateSchema(t, yaml)
}

func TestBuildChartTemplateYAMLDimensionFromVarbind(t *testing.T) {
	cfg := []MetricConfig{
		{OID: "1.3.6.1.4.1.9.9.43.2.0.1", Context: "snmp.trap.cisco_config", DimensionFromVarbind: "ccmHistoryEventTerminalType"},
	}
	yaml, err := buildChartTemplateYAML(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	collecttest.AssertChartTemplateSchema(t, yaml)
}

func TestBuildChartTemplateYAMLMultipleMetrics(t *testing.T) {
	cfg := []MetricConfig{
		{OID: "1.3.6.1.4.1.9.9.43.2.0.1", Context: "snmp.trap.config"},
		{OID: "1.3.6.1.4.1.9.9.46.2.0.1", Context: "snmp.trap.security"},
		{OID: "1.3.6.1.6.3.1.1.5.1", Context: "snmp.trap.cold_start"},
	}
	yaml, err := buildChartTemplateYAML(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	collecttest.AssertChartTemplateSchema(t, yaml)
}

func TestOperatorMetricsIncSingleDimension(t *testing.T) {
	cfg := []MetricConfig{
		{OID: "1.3.6.1.4.1.9.9.43.2.0.1", Context: "snmp.trap.cisco_config"},
	}
	om := newTestOperatorMetrics(t, cfg)
	td := makeTestProfileIndex(t).Lookup(cfg[0].OID)
	entry := &TrapEntry{TrapOID: cfg[0].OID}

	om.inc("1.3.6.1.4.1.9.9.43.2.0.1", entry, td)

	if om.metrics[0].singleCount.Load() != 1 {
		t.Fatalf("singleCount = %d, want 1", om.metrics[0].singleCount.Load())
	}

	om.inc("1.3.6.1.4.1.9.9.43.2.0.1", entry, td)
	om.inc("1.3.6.1.4.1.9.9.43.2.0.1", entry, td)

	if om.metrics[0].singleCount.Load() != 3 {
		t.Fatalf("singleCount = %d, want 3", om.metrics[0].singleCount.Load())
	}
}

func TestOperatorMetricsIncNoCounterForUnconfiguredOID(t *testing.T) {
	cfg := []MetricConfig{
		{OID: "1.3.6.1.4.1.9.9.43.2.0.1", Context: "snmp.trap.cisco_config"},
	}
	om := newTestOperatorMetrics(t, cfg)
	td := makeTestProfileIndex(t).Lookup(cfg[0].OID)

	om.inc("9.9.9.9.0.1", &TrapEntry{TrapOID: "9.9.9.9.0.1"}, td)

	if om.metrics[0].singleCount.Load() != 0 {
		t.Fatalf("singleCount = %d, want 0", om.metrics[0].singleCount.Load())
	}
}

func TestOperatorMetricsIncMatchesAlternateOIDSpelling(t *testing.T) {
	cfg := []MetricConfig{
		{OID: "1.3.6.1.6.3.1.1.5.0.1", Context: "snmp.trap.cold_start"},
	}
	om := newTestOperatorMetrics(t, cfg)
	td := makeTestProfileIndex(t).Lookup("1.3.6.1.6.3.1.1.5.1")

	om.inc("1.3.6.1.6.3.1.1.5.1", &TrapEntry{TrapOID: "1.3.6.1.6.3.1.1.5.1"}, td)

	if om.metrics[0].singleCount.Load() != 1 {
		t.Fatalf("singleCount = %d, want 1", om.metrics[0].singleCount.Load())
	}
}

func TestOperatorMetricsIncRequiresCurrentTrapDefinition(t *testing.T) {
	cfg := []MetricConfig{
		{OID: "1.3.6.1.6.3.1.1.5.1", Context: "snmp.trap.cold_start"},
	}
	om := newTestOperatorMetrics(t, cfg)

	om.inc("1.3.6.1.6.3.1.1.5.1", &TrapEntry{TrapOID: "1.3.6.1.6.3.1.1.5.1"}, nil)

	if om.metrics[0].singleCount.Load() != 0 {
		t.Fatalf("singleCount = %d, want 0 when current profile lookup is missing", om.metrics[0].singleCount.Load())
	}
}

func TestOperatorMetricsIncDimensionFromVarbind(t *testing.T) {
	idx := makeTestProfileIndex(t)
	td := idx.Lookup(ciscoConfigTrapOID)

	cfg := []MetricConfig{
		ciscoConfigMetric("snmp.trap.cisco_config", ciscoTerminalTypeVarbind),
	}
	om := newTestOperatorMetrics(t, cfg)

	entry := ciscoConfigEntry(ciscoTerminalTypeOID, int64(2), ciscoTerminalTypeVarbind)

	om.inc(ciscoConfigTrapOID, entry, td)

	om.metrics[0].dimMu.Lock()
	count := len(om.metrics[0].dimCounts)
	ctr, ok := om.metrics[0].dimCounts["console"]
	om.metrics[0].dimMu.Unlock()

	if count != 1 {
		t.Fatalf("dimCounts length = %d, want 1", count)
	}
	if !ok {
		t.Fatalf("dimCounts does not contain 'console', got %v", om.metrics[0].dimCounts)
	}
	if ctr.Load() != 1 {
		t.Fatalf("dim count = %d, want 1", ctr.Load())
	}

	om.inc(ciscoConfigTrapOID, entry, td)
	if ctr.Load() != 2 {
		t.Fatalf("dim count after second inc = %d, want 2", ctr.Load())
	}
}

func TestOperatorMetricsIncDimensionFromTabularVarbindInstance(t *testing.T) {
	idx := makeTestProfileIndex(t)
	td := idx.Lookup(ciscoConfigTrapOID)

	cfg := []MetricConfig{
		ciscoConfigMetric("snmp.trap.cisco_config", ciscoTerminalTypeVarbind),
	}
	om := newTestOperatorMetrics(t, cfg)

	entry := ciscoConfigEntry(ciscoTerminalTypeOID+".99", int64(2), "")

	om.inc(ciscoConfigTrapOID, entry, td)

	om.metrics[0].dimMu.Lock()
	ctr, ok := om.metrics[0].dimCounts["console"]
	_, missingSeen := om.metrics[0].dimCounts["<missing>"]
	om.metrics[0].dimMu.Unlock()
	if !ok {
		t.Fatalf("dimCounts does not contain 'console', got %v", om.metrics[0].dimCounts)
	}
	if missingSeen {
		t.Fatal("tabular varbind instance was counted as <missing>")
	}
	if ctr.Load() != 1 {
		t.Fatalf("console count = %d, want 1", ctr.Load())
	}
}

func TestOperatorMetricsDimensionValuesStayBoundedAtRuntime(t *testing.T) {
	idx := makeTestProfileIndex(t)

	t.Run("unknown enum value", func(t *testing.T) {
		cfg := []MetricConfig{
			ciscoConfigMetric("snmp.trap.cisco_config", ciscoTerminalTypeVarbind),
		}
		td := idx.Lookup(cfg[0].OID)
		om := newOperatorMetrics(cfg, idx)
		entry := ciscoConfigEntry(ciscoTerminalTypeOID, int64(99), ciscoTerminalTypeVarbind)

		om.inc(cfg[0].OID, entry, td)

		assertMetricDimensionBucket(t, om, "99", "unknown", "unknown enum")
	})

	t.Run("out of range numeric value", func(t *testing.T) {
		cfg := []MetricConfig{
			ciscoConfigMetric("snmp.trap.cisco_config_range", ciscoCommandSourceVarbind),
		}
		td := idx.Lookup(cfg[0].OID)
		om := newOperatorMetrics(cfg, idx)
		entry := ciscoConfigEntry(ciscoCommandSourceOID, int64(99), ciscoCommandSourceVarbind)

		om.inc(cfg[0].OID, entry, td)

		assertMetricDimensionBucket(t, om, "99", "out_of_range", "out-of-range numeric")
	})

	t.Run("expanded enum after reload stays bounded", func(t *testing.T) {
		cfg := []MetricConfig{
			ciscoConfigMetric("snmp.trap.cisco_config_enum_reload", ciscoTerminalTypeVarbind),
		}
		om := newOperatorMetrics(cfg, idx)
		reloadedTrap := ciscoConfigReloadedTrap(map[string]*VarbindDef{
			ciscoTerminalTypeOID: {
				OID:     ciscoTerminalTypeOID,
				Type:    "INTEGER",
				rawName: ciscoTerminalTypeVarbind,
				Enum:    testEnum(maxBoundedVarbindValues + 1),
			},
		})
		entry := ciscoConfigEntry(ciscoTerminalTypeOID, int64(maxBoundedVarbindValues+1), ciscoTerminalTypeVarbind)

		om.inc(cfg[0].OID, entry, reloadedTrap)

		assertMetricDimensionBucket(t, om, "value_"+strconv.Itoa(maxBoundedVarbindValues+1), "unknown", "expanded enum")
	})

	t.Run("enum label rename after reload keeps job labels stable", func(t *testing.T) {
		cfg := []MetricConfig{
			ciscoConfigMetric("snmp.trap.cisco_config_enum_rename", ciscoTerminalTypeVarbind),
		}
		td := idx.Lookup(cfg[0].OID)
		om := newOperatorMetrics(cfg, idx)
		entry := ciscoConfigEntry(ciscoTerminalTypeOID, int64(2), ciscoTerminalTypeVarbind)
		reloadedTrap := ciscoConfigReloadedTrap(map[string]*VarbindDef{
			ciscoTerminalTypeOID: {
				OID:     ciscoTerminalTypeOID,
				Type:    "INTEGER",
				rawName: ciscoTerminalTypeVarbind,
				Enum: map[string]string{
					"1": "none",
					"2": "terminal",
					"3": "virtual",
					"4": "aux",
				},
			},
		})

		om.inc(cfg[0].OID, entry, td)
		om.inc(cfg[0].OID, entry, reloadedTrap)

		om.metrics[0].dimMu.Lock()
		consoleCtr, consoleSeen := om.metrics[0].dimCounts["console"]
		_, terminalSeen := om.metrics[0].dimCounts["terminal"]
		om.metrics[0].dimMu.Unlock()
		if terminalSeen {
			t.Fatal("renamed enum label created a new metric dimension")
		}
		if !consoleSeen || consoleCtr.Load() != 2 {
			t.Fatalf("console bucket = seen %v count %d, want seen true count 2", consoleSeen, func() uint64 {
				if consoleCtr == nil {
					return 0
				}
				return consoleCtr.Load()
			}())
		}
	})

	t.Run("expanded numeric range after reload stays bounded", func(t *testing.T) {
		cfg := []MetricConfig{
			ciscoConfigMetric("snmp.trap.cisco_config_range_reload", ciscoCommandSourceVarbind),
		}
		om := newOperatorMetrics(cfg, idx)
		reloadedTrap := ciscoConfigReloadedTrap(map[string]*VarbindDef{
			ciscoCommandSourceOID: {
				OID:         ciscoCommandSourceOID,
				Type:        "INTEGER",
				rawName:     ciscoCommandSourceVarbind,
				Constraints: "(1..128)",
			},
		})
		entry := ciscoConfigEntry(ciscoCommandSourceOID, int64(99), ciscoCommandSourceVarbind)

		om.inc(cfg[0].OID, entry, reloadedTrap)

		assertMetricDimensionBucket(t, om, "99", "out_of_range", "expanded numeric range")
	})

	t.Run("missing varbind definition after reload", func(t *testing.T) {
		cfg := []MetricConfig{
			ciscoConfigMetric("snmp.trap.cisco_config_missing", ciscoTerminalTypeVarbind),
		}
		om := newOperatorMetrics(cfg, idx)
		reloadedTrap := ciscoConfigReloadedTrap(map[string]*VarbindDef{})
		entry := ciscoConfigEntry(ciscoTerminalTypeOID, int64(2), ciscoTerminalTypeVarbind)

		om.inc(cfg[0].OID, entry, reloadedTrap)

		assertMetricDimensionBucket(t, om, "ccmHistoryEventTerminalType", "<missing>", "missing varbind")
	})

	t.Run("unbounded varbind metadata after reload", func(t *testing.T) {
		cfg := []MetricConfig{
			ciscoConfigMetric("snmp.trap.cisco_config_reloaded", ciscoTerminalTypeVarbind),
		}
		om := newOperatorMetrics(cfg, idx)
		reloadedTrap := ciscoConfigReloadedTrap(map[string]*VarbindDef{
			ciscoTerminalTypeOID: {
				OID:     ciscoTerminalTypeOID,
				Type:    "OctetString",
				rawName: ciscoTerminalTypeVarbind,
			},
		})
		entry := ciscoConfigEntry(ciscoTerminalTypeOID, "operator-controlled-value", ciscoTerminalTypeVarbind)

		om.inc(cfg[0].OID, entry, reloadedTrap)

		assertMetricDimensionBucket(t, om, "operator-controlled-value", "unknown", "unbounded reloaded varbind")
	})
}

func TestMetricBoolValue(t *testing.T) {
	tests := map[string]struct {
		value any
		want  string
		ok    bool
	}{
		"bool true":          {value: true, want: "true", ok: true},
		"bool false":         {value: false, want: "false", ok: true},
		"truthvalue true":    {value: int64(1), want: "true", ok: true},
		"truthvalue false":   {value: uint64(2), want: "false", ok: true},
		"string true":        {value: "true", want: "true", ok: true},
		"string false":       {value: "2", want: "false", ok: true},
		"unknown truthvalue": {value: int64(3), ok: false},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got, ok := metricBoolValue(tc.value)
			if ok != tc.ok {
				t.Fatalf("ok = %v, want %v", ok, tc.ok)
			}
			if got != tc.want {
				t.Fatalf("value = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestMetricIntValueRejectsUint64Overflow(t *testing.T) {
	if _, ok := metricIntValue(uint64(1 << 63)); ok {
		t.Fatal("expected uint64 larger than math.MaxInt64 to be rejected")
	}
}

func TestOperatorMetricsCollectSingleDimension(t *testing.T) {
	cfg := []MetricConfig{
		{OID: "1.3.6.1.4.1.9.9.43.2.0.1", Context: "snmp.trap.cisco_config"},
	}
	om := newTestOperatorMetrics(t, cfg)
	om.metrics[0].singleCount.Store(42)

	store := metrix.NewCollectorStore()
	ms, ok := metrix.AsCycleManagedStore(store)
	if !ok {
		t.Fatal("AsCycleManagedStore failed")
	}
	cc := ms.CycleController()
	cc.BeginCycle()
	om.collect(store, "test-job")
	cc.CommitCycleSuccess()

	labels := metrix.Labels{"job_name": "test-job"}
	val, ok := store.Read().Value(metricSelector("snmp.trap.cisco_config"), labels)
	if !ok {
		t.Fatal("metric not found in store")
	}
	if val != 42 {
		t.Fatalf("value = %v, want 42", val)
	}
}

func TestOperatorMetricsCollectDimensionFromVarbind(t *testing.T) {
	cfg := []MetricConfig{
		{OID: "1.3.6.1.4.1.9.9.43.2.0.1", Context: "snmp.trap.cisco_config", DimensionFromVarbind: "ccmHistoryEventTerminalType"},
	}
	om := newTestOperatorMetrics(t, cfg)
	om.metrics[0].dimCounts = map[string]*atomic.Uint64{
		"console": func() *atomic.Uint64 { v := atomic.Uint64{}; v.Store(5); return &v }(),
		"virtual": func() *atomic.Uint64 { v := atomic.Uint64{}; v.Store(3); return &v }(),
	}

	store := metrix.NewCollectorStore()
	cc := needCycleManagedStore(t, store).CycleController()
	cc.BeginCycle()
	om.collect(store, "test-job")
	cc.CommitCycleSuccess()

	consoleLabels := metrix.Labels{"job_name": "test-job", "varbind_value": "console"}
	val, ok := store.Read().Value(metricSelector("snmp.trap.cisco_config"), consoleLabels)
	if !ok {
		t.Fatal("console metric not found in store")
	}
	if val != 5 {
		t.Fatalf("console value = %v, want 5", val)
	}

	virtualLabels := metrix.Labels{"job_name": "test-job", "varbind_value": "virtual"}
	val, ok = store.Read().Value(metricSelector("snmp.trap.cisco_config"), virtualLabels)
	if !ok {
		t.Fatal("virtual metric not found in store")
	}
	if val != 3 {
		t.Fatalf("virtual value = %v, want 3", val)
	}
}

func TestOperatorMetricsNilSafe(t *testing.T) {
	var om *operatorMetrics
	om.inc("1.2.3.0.1", nil, nil)

	store := metrix.NewCollectorStore()
	cc := needCycleManagedStore(t, store).CycleController()
	cc.BeginCycle()
	om.collect(store, "test-job")
	cc.CommitCycleSuccess()
}

func TestCollectorHandlePacketIncrementsOperatorMetric(t *testing.T) {
	packet := readColdStartUDPPacket(t)
	trap := testColdStartTrap("state_change", "warning", "coldStart from {TRAP_SOURCE_IP}")
	trap.Name = "SNMPv2-MIB::coldStart"
	setSingleTestTrap(t, trap)
	writer := &mockTrapWriter{}
	c := newDefaultTestV2Collector(writer)
	c.operatorMetrics = newTestOperatorMetrics(t, []MetricConfig{{OID: trap.OID, Context: "snmp.trap.cold_start"}})

	c.handlePacket(packet.payload, packet.peer, nil, nil)

	if c.operatorMetrics.metrics[0].singleCount.Load() != 1 {
		t.Fatalf("operator metric singleCount = %d, want 1", c.operatorMetrics.metrics[0].singleCount.Load())
	}
}

func TestCollectorHandlePacketNoOperatorMetricForDroppedTraps(t *testing.T) {
	packet := readColdStartUDPPacket(t)
	trap := testColdStartTrap("state_change", "warning", "coldStart from {TRAP_SOURCE_IP}")
	trap.Name = "SNMPv2-MIB::coldStart"
	setSingleTestTrap(t, trap)

	c := newDefaultTestV2Collector(&mockTrapWriter{err: errors.New("write failed")})
	c.operatorMetrics = newTestOperatorMetrics(t, []MetricConfig{{OID: trap.OID, Context: "snmp.trap.cold_start"}})

	c.handlePacket(packet.payload, packet.peer, nil, nil)

	if c.operatorMetrics.metrics[0].singleCount.Load() != 0 {
		t.Fatalf("operator metric singleCount = %d, want 0 (write failed)", c.operatorMetrics.metrics[0].singleCount.Load())
	}
}

func TestCollectorHandlePacketNoOperatorMetricForUnconfiguredOID(t *testing.T) {
	packet := readColdStartUDPPacket(t)
	trap := testColdStartTrap("state_change", "warning", "coldStart from {TRAP_SOURCE_IP}")
	trap.Name = "SNMPv2-MIB::coldStart"
	setSingleTestTrap(t, trap)
	writer := &mockTrapWriter{}
	c := newDefaultTestV2Collector(writer)
	c.operatorMetrics = newTestOperatorMetrics(t, []MetricConfig{{OID: "9.9.9.9.9.0.1", Context: "snmp.trap.unrelated"}})

	c.handlePacket(packet.payload, packet.peer, nil, nil)

	if c.operatorMetrics.metrics[0].singleCount.Load() != 0 {
		t.Fatalf("operator metric for unrelated OID should not increment, got %d", c.operatorMetrics.metrics[0].singleCount.Load())
	}
}

func TestCollectorHandlePacketNoOperatorMetricWhenNil(t *testing.T) {
	packet := readColdStartUDPPacket(t)
	trap := testColdStartTrap("state_change", "warning", "coldStart from {TRAP_SOURCE_IP}")
	trap.Name = "SNMPv2-MIB::coldStart"
	setSingleTestTrap(t, trap)
	writer := &mockTrapWriter{}
	c := newDefaultTestV2Collector(writer)

	c.handlePacket(packet.payload, packet.peer, nil, nil)

	if len(writer.entries) != 1 {
		t.Fatalf("trap not written when no operator metrics: %v", writer.entries)
	}
}

func TestCollectorCollectsOperatorMetrics(t *testing.T) {
	cfg := []MetricConfig{
		{OID: "1.3.6.1.4.1.9.9.43.2.0.1", Context: "snmp.trap.cisco_config"},
	}
	store := metrix.NewCollectorStore()
	c := &Collector{
		jobName:         "test-job",
		store:           store,
		listener:        &Listener{},
		operatorMetrics: newTestOperatorMetrics(t, cfg),
	}
	c.operatorMetrics.metrics[0].singleCount.Store(7)

	cc := needCycleManagedStore(t, store).CycleController()
	cc.BeginCycle()
	c.collect(context.Background())
	cc.CommitCycleSuccess()

	labels := metrix.Labels{"job_name": "test-job"}
	val, ok := store.Read().Value(metricSelector("snmp.trap.cisco_config"), labels)
	if !ok {
		t.Fatal("operator metric not found in store after collect")
	}
	if val != 7 {
		t.Fatalf("value = %v, want 7", val)
	}
}

func TestCollectorHandlePacketOperatorMetricWithDimFromVarbind(t *testing.T) {
	trap := &TrapDef{
		OID:      "1.3.6.1.4.1.9.9.43.2.0.1",
		Name:     "CISCO-CONFIG-MAN-MIB::ccmCLIRunningConfigChanged",
		Category: "config_change",
		Severity: "notice",
		VarbindRefs: []any{
			"ccmHistoryEventTerminalType",
		},
		sharedVarbinds: map[string]*VarbindDef{
			"1.3.6.1.4.1.9.9.43.1.1.1.2": {
				OID:     "1.3.6.1.4.1.9.9.43.1.1.1.2",
				Type:    "INTEGER",
				rawName: "ccmHistoryEventTerminalType",
				Enum: map[string]string{
					"1": "none",
					"2": "console",
					"3": "virtual",
					"4": "aux",
				},
			},
		},
	}
	setTestProfileIndex(t, map[string]*TrapDef{trap.OID: trap})

	pdu := &TrapPDU{
		OID:     "1.3.6.1.4.1.9.9.43.2.0.1",
		Version: SnmpVersionV2c,
		Varbinds: []VarbindValue{
			{OID: "1.3.6.1.4.1.9.9.43.1.1.1.2", Value: int64(2)},
		},
		SourceIP: "198.51.100.10",
	}

	c := &Collector{
		jobName:    "test",
		trapWriter: &mockTrapWriter{},
		versions:   map[SnmpVersion]struct{}{SnmpVersionV2c: {}},
		operatorMetrics: newTestOperatorMetrics(t, []MetricConfig{
			{OID: "1.3.6.1.4.1.9.9.43.2.0.1", Context: "snmp.trap.config", DimensionFromVarbind: "ccmHistoryEventTerminalType"},
		}),
	}

	entry := trapEntryFromPDU(c.jobName, pdu, trap, time.Now().UnixMicro(), monotonicUsec())
	renderTrapEntryTemplates(entry, trap)
	c.operatorMetrics.inc(entry.TrapOID, entry, trap)

	om := c.operatorMetrics
	om.metrics[0].dimMu.Lock()
	ctr, ok := om.metrics[0].dimCounts["console"]
	om.metrics[0].dimMu.Unlock()
	if !ok {
		t.Fatal("dimCounts does not contain 'console'")
	}
	if ctr.Load() != 1 {
		t.Fatalf("console count = %d, want 1", ctr.Load())
	}
}

func TestCollectorNoOperatorMetricOnDedupSuppression(t *testing.T) {
	packet := readColdStartUDPPacket(t)
	trap := testColdStartTrap("state_change", "warning", "coldStart from {TRAP_SOURCE_IP}")
	trap.Name = "SNMPv2-MIB::coldStart"
	setSingleTestTrap(t, trap)

	om := newTestOperatorMetrics(t, []MetricConfig{
		{OID: trap.OID, Context: "snmp.trap.cold_start"},
	})

	deduper := newTrapDeduper("test", DedupConfig{Enabled: true, WindowSec: 3600, CacheMaxEntries: 1000}, &mockTrapWriter{}, nil, "")
	if deduper != nil {
		deduper.start()
		defer deduper.Close()
	}

	c := &Collector{
		jobName:         "test",
		trapWriter:      &mockTrapWriter{},
		versions:        map[SnmpVersion]struct{}{SnmpVersionV2c: {}},
		allowlist:       NewAllowlist(nil, []string{"public"}),
		deduper:         deduper,
		operatorMetrics: om,
	}

	addr, _ := netip.ParseAddr("198.51.100.10")
	peer := &net.UDPAddr{IP: addr.AsSlice(), Port: 16234}

	c.handlePacket(packet.payload, packet.peer, nil, peer)
	c.handlePacket(packet.payload, packet.peer, nil, peer)

	if om.metrics[0].singleCount.Load() != 1 {
		t.Fatalf("operator metric singleCount = %d, want 1 (second trap should be dedup-suppressed)", om.metrics[0].singleCount.Load())
	}
}
