// SPDX-License-Identifier: GPL-3.0-or-later

package profilemetrics

import (
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProfileMetricSelection(t *testing.T) {
	idx := testProfileMetricIndex(t)
	require.NoError(t, idx.AddMetricDefinitions([]profileMetricRule{{
		Name:       "disabled.rule",
		Type:       profileMetricTypeCounter,
		Enabled:    new(false),
		OnTrap:     testCiscoConfigTrapOID,
		Output:     profileMetricOutput{Metric: "snmp_trap_disabled_events", Dimension: "events", Chart: "cisco_config_changes"},
		SourceFile: "test-profile.yaml",
	}}, nil))
	cat := profileMetricCatalogForTest(t, idx)

	tests := map[string]struct {
		cfg   ProfileMetricsConfig
		want  []string
		error bool
	}{
		"disabled": {
			cfg: ProfileMetricsConfig{},
		},
		"explicit include": {
			cfg:  ProfileMetricsConfig{Enabled: true, Include: []string{"cisco.config.terminal_type", "cisco.config.changed"}},
			want: []string{"cisco.config.changed", "cisco.config.terminal_type"},
		},
		"missing include": {
			cfg:   ProfileMetricsConfig{Enabled: true, Include: []string{"missing.rule"}},
			error: true,
		},
		"disabled include": {
			cfg:   ProfileMetricsConfig{Enabled: true, Include: []string{"disabled.rule"}},
			error: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			cfg, err := normalizeProfileMetricsConfig(tc.cfg)
			if err != nil {
				t.Fatalf("normalizeProfileMetricsConfig failed: %v", err)
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
	idx := testProfileMetricIndex(t)
	cat := profileMetricCatalogForTest(t, idx)
	cfg, err := normalizeProfileMetricsConfig(ProfileMetricsConfig{
		Enabled: true,
		Include: []string{"cisco.config.changed", "cisco.config.terminal_type"},
	})
	if err != nil {
		t.Fatalf("normalizeProfileMetricsConfig failed: %v", err)
	}

	cfg.limits.MaxRules = 1
	if _, err := selectProfileMetricRules(cfg, cat); err == nil {
		t.Fatalf("selectProfileMetricRules accepted more selected rules than max_rules")
	}
}

func TestNewProfileMetricRuntimeRejectsNilProfileIndex(t *testing.T) {
	cfg, err := normalizeProfileMetricsConfig(ProfileMetricsConfig{
		Enabled: true,
		Include: []string{"cisco.config.changed"},
	})
	if err != nil {
		t.Fatalf("normalizeProfileMetricsConfig failed: %v", err)
	}

	if _, _, err := newProfileMetricRuntime(cfg, nil, "test"); err == nil || !strings.Contains(err.Error(), "profile index not available") {
		t.Fatalf("newProfileMetricRuntime nil index error = %v, want profile index not available", err)
	}
}

func TestProfileMetricValidationRejectsDuplicateChartDimensions(t *testing.T) {
	idx := testProfileMetricIndex(t)
	err := idx.AddMetricDefinitions([]profileMetricRule{{
		Name:   "cisco.config.duplicate_dimension",
		Type:   profileMetricTypeCounter,
		OnTrap: testCiscoConfigTrapOID,
		Output: profileMetricOutput{
			Metric:    "snmp_trap_cisco_config_duplicate_dimension_events",
			Dimension: "events",
			Chart:     "cisco_config_changes",
		},
		SourceFile: "site-profile.yaml",
	}}, nil)
	if err != nil {
		t.Fatalf("addProfileMetrics rejected alternate same-dimension rule before selection: %v", err)
	}
	cfg, err := normalizeProfileMetricsConfig(ProfileMetricsConfig{
		Enabled: true,
		Include: []string{"cisco.config.changed", "cisco.config.duplicate_dimension"},
	})
	if err != nil {
		t.Fatalf("normalizeProfileMetricsConfig failed: %v", err)
	}
	_, _, err = newProfileMetricRuntime(cfg, idx, "test")
	if err == nil ||
		!strings.Contains(err.Error(), "reuses output.dimension") ||
		!strings.Contains(err.Error(), "cisco.config.changed") {
		t.Fatalf("newProfileMetricRuntime duplicate dimension error = %v, want rule-specific duplicate dimension error", err)
	}
}
