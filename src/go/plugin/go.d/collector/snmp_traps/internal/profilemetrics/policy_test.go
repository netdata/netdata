// SPDX-License-Identifier: GPL-3.0-or-later

package profilemetrics

import (
	"reflect"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/catalog"
)

func TestProfileMetricSelection(t *testing.T) {
	idx := newPopulatedTestCatalog(t)
	idx = idx.withDefinitions([]catalog.MetricRule{{
		Name:    "disabled.rule",
		Type:    catalog.MetricTypeCounter,
		Enabled: new(false),
		OnTrap:  testCiscoConfigTrapOID,
		Identity: catalog.MetricIdentity{
			Device: catalog.MetricIdentitySource,
		},
		Output:  catalog.MetricOutput{Metric: "snmp_trap_disabled_events", Dimension: "events", Chart: "cisco_config_changes"},
		Missing: catalog.MetricMissingDrop,
		Scale:   catalog.MetricScale{Multiplier: 1, Divisor: 1},
	}}, nil)
	cat := profileMetricCatalogForTest(idx)

	tests := map[string]struct {
		cfg   testRuntimeConfig
		want  []string
		error bool
	}{
		"disabled": {
			cfg: testRuntimeConfig{},
		},
		"explicit include": {
			cfg:  testRuntimeConfig{Enabled: true, Include: []string{"cisco.config.terminal_type", "cisco.config.changed"}},
			want: []string{"cisco.config.changed", "cisco.config.terminal_type"},
		},
		"missing include": {
			cfg:   testRuntimeConfig{Enabled: true, Include: []string{"missing.rule"}},
			error: true,
		},
		"disabled include": {
			cfg:   testRuntimeConfig{Enabled: true, Include: []string{"disabled.rule"}},
			error: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			cfg, err := normalizeTestRuntimeConfig(tc.cfg)
			if err != nil {
				t.Fatalf("normalizeTestRuntimeConfig failed: %v", err)
			}
			rules, err := selectProfileMetricRules(cfg, cat)
			if tc.error {
				if err == nil {
					t.Fatalf("selectProfileMetricRules returned nil error")
				}
				return
			}
			if err != nil {
				t.Fatalf("selectProfileMetricRules failed: %v", err)
			}
			var got []string
			for _, rule := range rules {
				got = append(got, rule.Name)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("selected rules = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestProfileMetricSelectionRejectsMoreThanMaxRules(t *testing.T) {
	idx := newPopulatedTestCatalog(t)
	cat := profileMetricCatalogForTest(idx)
	cfg, err := normalizeTestRuntimeConfig(testRuntimeConfig{
		Enabled: true,
		Include: []string{"cisco.config.changed", "cisco.config.terminal_type"},
	})
	if err != nil {
		t.Fatalf("normalizeTestRuntimeConfig failed: %v", err)
	}

	cfg.limits.MaxRules = 1
	if _, err := selectProfileMetricRules(cfg, cat); err == nil {
		t.Fatalf("selectProfileMetricRules accepted more selected rules than max_rules")
	}
}

func TestNewProfileMetricRuntimeRejectsNilProfileIndex(t *testing.T) {
	cfg, err := normalizeTestRuntimeConfig(testRuntimeConfig{
		Enabled: true,
		Include: []string{"cisco.config.changed"},
	})
	if err != nil {
		t.Fatalf("normalizeTestRuntimeConfig failed: %v", err)
	}

	if _, _, err := newTestRuntime(cfg, nil, "test"); err == nil || !strings.Contains(err.Error(), "profile index not available") {
		t.Fatalf("newTestRuntime nil index error = %v, want profile index not available", err)
	}
}

func TestNewRejectsSelectedDuplicateChartDimensions(t *testing.T) {
	idx := newPopulatedTestCatalog(t)
	idx = idx.withDefinitions([]catalog.MetricRule{{
		Name:   "cisco.config.duplicate_dimension",
		Type:   catalog.MetricTypeCounter,
		OnTrap: testCiscoConfigTrapOID,
		Identity: catalog.MetricIdentity{
			Device: catalog.MetricIdentitySource,
		},
		Output: catalog.MetricOutput{
			Metric:    "snmp_trap_cisco_config_duplicate_dimension_events",
			Dimension: "events",
			Chart:     "cisco_config_changes",
		},
		Missing: catalog.MetricMissingDrop,
		Scale:   catalog.MetricScale{Multiplier: 1, Divisor: 1},
	}}, nil)
	cfg, err := normalizeTestRuntimeConfig(testRuntimeConfig{
		Enabled: true,
		Include: []string{"cisco.config.changed", "cisco.config.duplicate_dimension"},
	})
	if err != nil {
		t.Fatalf("normalizeTestRuntimeConfig failed: %v", err)
	}
	_, _, err = newTestRuntime(cfg, idx, "test")
	if err == nil ||
		!strings.Contains(err.Error(), "reuses output.dimension") ||
		!strings.Contains(err.Error(), "cisco.config.changed") {
		t.Fatalf("newTestRuntime duplicate dimension error = %v, want rule-specific duplicate dimension error", err)
	}
}
