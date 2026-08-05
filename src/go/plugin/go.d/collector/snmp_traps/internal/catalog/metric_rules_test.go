// SPDX-License-Identifier: GPL-3.0-or-later

package catalog

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/model"
	"github.com/stretchr/testify/require"
)

const (
	profileMetricTypeCounter = MetricTypeCounter
	profileMetricTypeSample  = MetricTypeSample
	profileMetricTypeState   = MetricTypeState
)

type profileMetricCatalog struct {
	rulesByName map[string]*MetricRule
	chartsByID  map[string]*MetricChart
}

func newTestMetricEpoch() *Epoch { return newEpoch() }

func (idx *Epoch) addTestMetricDefinitions(rules []MetricRule, charts []MetricChart) error {
	return idx.addProfileMetrics(rules, charts, true)
}

const (
	testCiscoConfigTrapOID        = "1.3.6.1.4.1.9.9.43.2.0.1"
	testCiscoCommandSourceOID     = "1.3.6.1.4.1.9.9.43.1.1.1.1"
	testCiscoTerminalTypeOID      = "1.3.6.1.4.1.9.9.43.1.1.1.2"
	testCiscoTerminalTypeVarbind  = "ccmHistoryEventTerminalType"
	testCiscoCommandSourceVarbind = "ccmHistoryEventCommandSource"
	testIfIndexOID                = "1.3.6.1.2.1.2.2.1.1"
	testPortSecurityTrapOID       = "1.3.6.1.4.1.9.9.46.2.0.1"
)

func newPopulatedTestMetricEpoch(t *testing.T) *Epoch {
	t.Helper()
	idx := newTestMetricEpoch()
	traps := []*TrapDef{
		{
			OID:      testCiscoConfigTrapOID,
			Name:     "CISCO-CONFIG-MAN-MIB::ccmCLIRunningConfigChanged",
			Category: "config_change",
			Severity: "notice",
			VarbindRefs: []any{
				testCiscoCommandSourceVarbind,
				testCiscoTerminalTypeVarbind,
				"sysUpTime.0",
			},
			SharedVarbinds: map[string]*VarbindDef{
				testCiscoCommandSourceOID: {
					OID:         testCiscoCommandSourceOID,
					Type:        "INTEGER",
					RawName:     testCiscoCommandSourceVarbind,
					Constraints: "(1..4)",
				},
				testCiscoTerminalTypeOID: {
					OID:     testCiscoTerminalTypeOID,
					Type:    "INTEGER",
					RawName: testCiscoTerminalTypeVarbind,
					Enum: map[string]string{
						"1": "none",
						"2": "console",
						"3": "virtual",
						"4": "aux",
					},
				},
				model.SysUpTimeOID: {
					OID:     model.SysUpTimeOID,
					Type:    "TimeTicks",
					RawName: "sysUpTime.0",
				},
			},
		},
		{
			OID:      testPortSecurityTrapOID,
			Name:     "CISCO-PORT-SECURITY-MIB::cpsSecureMacAddrViolation",
			Category: "security",
			Severity: "warning",
			VarbindRefs: []any{
				"ifIndex",
			},
			SharedVarbinds: map[string]*VarbindDef{
				testIfIndexOID: {
					OID:         testIfIndexOID,
					Type:        "INTEGER",
					RawName:     "ifIndex",
					Constraints: "(1..48)",
				},
			},
		},
	}
	if err := idx.addTraps(traps); err != nil {
		t.Fatalf("addTraps failed: %v", err)
	}
	charts := []profileMetricChart{
		{
			ID:         "cisco_config_changes",
			Title:      "Cisco config changes",
			Context:    "snmp.trap.cisco.config.changes",
			Units:      "events/s",
			Algorithm:  "incremental",
			Type:       "line",
			SourceFile: "test-profile.yaml",
		},
		{
			ID:         "cisco_terminal_type",
			Title:      "Cisco terminal type",
			Context:    "snmp.trap.cisco.terminal.type",
			Units:      "type",
			Algorithm:  "absolute",
			Type:       "line",
			SourceFile: "test-profile.yaml",
		},
		{
			ID:         "port_security_violations",
			Title:      "Port security violations",
			Context:    "snmp.trap.cisco.port.security.violations",
			Units:      "events/s",
			Algorithm:  "incremental",
			Type:       "line",
			SourceFile: "test-profile.yaml",
		},
	}
	rules := []profileMetricRule{
		{
			Name:   "cisco.config.changed",
			Type:   profileMetricTypeCounter,
			OnTrap: "CISCO-CONFIG-MAN-MIB::ccmCLIRunningConfigChanged",
			Output: profileMetricOutput{
				Metric:    "snmp_trap_cisco_config_events",
				Dimension: "events",
				Chart:     "cisco_config_changes",
			},
			SourceFile: "test-profile.yaml",
		},
		{
			Name:             "cisco.config.terminal_type",
			Type:             profileMetricTypeSample,
			OnTrap:           testCiscoConfigTrapOID,
			ValueFromVarbind: testCiscoTerminalTypeVarbind,
			Output: profileMetricOutput{
				Metric:    "snmp_trap_cisco_terminal_type",
				Dimension: "terminal_type",
				Chart:     "cisco_terminal_type",
			},
			SourceFile: "test-profile.yaml",
		},
		{
			Name:     "cisco.port_security.ifindex",
			Type:     profileMetricTypeCounter,
			OnTrap:   testPortSecurityTrapOID,
			Identity: profileMetricIdentity{Resource: &profileMetricResource{Class: "interface", KeyFromVarbind: "ifIndex", MaxPerSource: 48}},
			Output: profileMetricOutput{
				Metric:    "snmp_trap_cisco_port_security_violations",
				Dimension: "violations",
				Chart:     "port_security_violations",
			},
			SourceFile: "test-profile.yaml",
		},
	}
	if err := idx.addTestMetricDefinitions(rules, charts); err != nil {
		t.Fatalf("addProfileMetrics failed: %v", err)
	}
	return idx
}

func profileMetricCatalogForTest(t *testing.T, idx *Epoch) profileMetricCatalog {
	t.Helper()
	return profileMetricCatalog{rulesByName: idx.metricRulesByName, chartsByID: idx.metricChartsByID}
}

func profileMetricChartFromIndex(t *testing.T, idx *Epoch, id string) *profileMetricChart {
	t.Helper()
	chart := profileMetricCatalogForTest(t, idx).chartsByID[id]
	require.NotNil(t, chart)
	return chart
}

func TestResolveProfileMetricTrapTrimsReferences(t *testing.T) {
	idx := newTestMetricEpoch()
	trap := &TrapDef{
		OID:      "1.3.6.1.4.1.99999.1",
		Name:     "TEST-MIB::candidate",
		Category: "diagnostic",
		Severity: "info",
	}
	require.NoError(t, idx.addTraps([]*TrapDef{trap}))

	for _, ref := range []string{" 1.3.6.1.4.1.99999.1 ", " TEST-MIB::candidate "} {
		resolved, err := idx.ResolveTrap(ref)
		require.NoError(t, err)
		require.Same(t, trap, resolved)
	}
}

func TestValidateProfileDefinitionUniquenessRejectsDuplicateMetricDefinitions(t *testing.T) {
	tests := map[string]struct {
		definition ProfileDefinition
		want       string
	}{
		"rule name": {
			definition: ProfileDefinition{Metrics: []profileMetricRule{{Name: "duplicate.rule"}, {Name: "duplicate.rule"}}},
			want:       "duplicate metric rule duplicate.rule in profile",
		},
		"chart ID": {
			definition: ProfileDefinition{Charts: []profileMetricChart{{ID: "duplicate_chart"}, {ID: "duplicate_chart"}}},
			want:       "duplicate metric chart duplicate_chart in profile",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			err := validateProfileDefinitionUniqueness("test-profile.yaml", &tc.definition)
			require.ErrorContains(t, err, tc.want)
		})
	}
}

func TestParseProfileBundleRunsMetricDefinitionUniquenessValidation(t *testing.T) {
	tests := map[string]struct {
		profile string
		want    string
	}{
		"rule name": {
			profile: `
metrics:
  - name: duplicate.rule
  - name: duplicate.rule
`,
			want: "duplicate metric rule duplicate.rule in profile",
		},
		"chart ID": {
			profile: `
charts:
  - id: duplicate_chart
  - id: duplicate_chart
`,
			want: "duplicate metric chart duplicate_chart in profile",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := parseProfileBundle("test-profile.yaml", []byte(tc.profile))
			require.ErrorContains(t, err, tc.want)
		})
	}
}

func TestNormalizeProfileMetricDefinitionsAppliesAllDefaults(t *testing.T) {
	rule := profileMetricRule{Name: "test.sample", Type: " SAMPLE "}
	require.NoError(t, normalizeProfileMetricRule(&rule))
	require.Equal(t, profileMetricTypeSample, rule.Type)
	require.Equal(t, MetricIdentitySource, rule.Identity.Device)
	require.Equal(t, "snmp_trap_test_sample", rule.Output.Metric)
	require.Equal(t, "value", rule.Output.Dimension)
	require.Equal(t, "test_sample", rule.Output.Chart)
	require.Equal(t, MetricMissingDrop, rule.Missing)
	require.Equal(t, profileMetricScale{Multiplier: 1, Divisor: 1}, rule.Scale)

	chart := profileMetricChart{ID: "test_sample", Title: "Test sample", Units: "value"}
	require.NoError(t, normalizeProfileMetricChart(&chart))
	require.Equal(t, "snmp.trap.test_sample", chart.Context)
	require.Equal(t, "incremental", chart.Algorithm)
	require.Equal(t, "line", chart.Type)
	require.Equal(t, DefaultMetricChartMaxInstances, chart.Lifecycle.MaxInstances)
	require.Equal(t, DefaultMetricExpireAfterCycles, chart.Lifecycle.ExpireAfterCycles)
}

func TestLoadProfileAcceptsCanonicalMetricSyntax(t *testing.T) {
	dir := t.TempDir()
	profile := `
varbinds:
  ccmHistoryEventTerminalType:
    oid: 1.3.6.1.4.1.9.9.43.1.1.1.2
    type: INTEGER
    enum:
      "2": console
      "3": virtual
  ifIndex:
    oid: 1.3.6.1.2.1.2.2.1.1
    type: INTEGER
traps:
  - oid: 1.3.6.1.4.1.9.9.43.2.0.1
    name: CISCO-CONFIG-MAN-MIB::ccmCLIRunningConfigChanged
    category: config_change
    severity: notice
    varbinds:
      - ccmHistoryEventTerminalType
  - oid: 1.3.6.1.4.1.9.9.46.2.0.1
    name: CISCO-PORT-SECURITY-MIB::cpsSecureMacAddrViolation
    category: security
    severity: warning
    varbinds:
      - ifIndex
metrics:
  - name: cisco.config.changed
    type: counter
    on_trap: CISCO-CONFIG-MAN-MIB::ccmCLIRunningConfigChanged
    where:
      - varbind: ccmHistoryEventTerminalType
        equals: console
    output:
      metric: snmp_trap_cisco_config_events
      dimension: events
      chart: cisco_config_changes
  - name: cisco.config.changed.by_terminal
    type: counter
    on_trap: CISCO-CONFIG-MAN-MIB::ccmCLIRunningConfigChanged
    where:
      - varbind: ccmHistoryEventTerminalType
        in:
          - console
          - virtual
    output:
      metric: snmp_trap_cisco_config_terminal_events
      dimension: terminal_events
      chart: cisco_config_changes
  - name: cisco.config.console_state
    type: state
    on_trap: CISCO-CONFIG-MAN-MIB::ccmCLIRunningConfigChanged
    state:
      set_when:
        varbind: ccmHistoryEventTerminalType
        equals: console
      clear_when:
        varbind: ccmHistoryEventTerminalType
        equals: virtual
    output:
      metric: snmp_trap_cisco_console_state
      dimension: active
      chart: cisco_console_state
  - name: cisco.port_security.ifindex
    type: counter
    on_trap: CISCO-PORT-SECURITY-MIB::cpsSecureMacAddrViolation
    identity:
      resource:
        class: interface
        key_from_varbind: ifIndex
        max_per_source: 48
    output:
      metric: snmp_trap_cisco_port_security_violations
      dimension: violations
      chart: port_security_violations
charts:
  - id: cisco_config_changes
    title: Cisco configuration changes
    context: snmp.trap.cisco.config.changes
    units: events/s
    algorithm: incremental
  - id: cisco_console_state
    title: Cisco console state
    context: snmp.trap.cisco.console.state
    units: state
    algorithm: absolute
  - id: port_security_violations
    title: Port security violations
    context: snmp.trap.cisco.port.security.violations
    units: events/s
    algorithm: incremental
`
	writeProfileYAML(t, dir, "profile.yaml", profile)

	bundle, err := loadProfileBundle(filepath.Join(dir, "profile.yaml"))
	if err != nil {
		t.Fatalf("loadProfileBundle failed: %v", err)
	}
	idx := newTestMetricEpoch()
	if err := idx.addTraps(bundle.traps); err != nil {
		t.Fatalf("addTraps failed: %v", err)
	}
	if err := idx.addTestMetricDefinitions(bundle.metrics, bundle.charts); err != nil {
		t.Fatalf("addProfileMetrics failed: %v", err)
	}
	cat := profileMetricCatalogForTest(t, idx)
	if cat.rulesByName["cisco.config.changed"].Output.Dimension != "events" {
		t.Fatalf("canonical counter dimension = %q, want events", cat.rulesByName["cisco.config.changed"].Output.Dimension)
	}
	if got := cat.rulesByName["cisco.config.changed"].Where; len(got) != 1 || got[0].Varbind != "ccmHistoryEventTerminalType" || got[0].Equals != "console" {
		t.Fatalf("canonical where list = %#v, want ccmHistoryEventTerminalType equals console", got)
	}
	if got := cat.rulesByName["cisco.config.changed.by_terminal"].Where; len(got) != 1 || got[0].Varbind != "ccmHistoryEventTerminalType" || !reflect.DeepEqual(got[0].In, []any{"console", "virtual"}) {
		t.Fatalf("canonical where in list = %#v, want ccmHistoryEventTerminalType in console,virtual", got)
	}
	if cat.rulesByName["cisco.port_security.ifindex"].Identity.Resource == nil {
		t.Fatalf("canonical identity.resource syntax did not populate identity.resource")
	}
	if profileMetricChartFromIndex(t, idx, "cisco_config_changes") == nil {
		t.Fatalf("canonical chart did not create cisco_config_changes chart")
	}
	if got := profileMetricChartFromIndex(t, idx, "cisco_config_changes").Type; got != "line" {
		t.Fatalf("canonical chart default type = %q, want line", got)
	}
	if got := profileMetricChartFromIndex(t, idx, "port_security_violations").Type; got != "line" {
		t.Fatalf("canonical chart default type = %q, want line", got)
	}
}

func TestLoadProfileRejectsRemovedProfileMetricSyntax(t *testing.T) {
	tests := map[string]string{
		"auto_safe": `    auto_safe: true
`,
		"compact_metric": `    metric: snmp_trap_test_events
`,
		"compact_dimension": `    dimension: events
`,
		"compact_chart_id": `    chart_id: test_events
`,
		"compact_value": `    value: testValue
`,
		"top_level_resource": `    resource:
      class: interface
      key: ifIndex
`,
		"resource_key": `    identity:
      resource:
        class: interface
        key: ifIndex
`,
		"resource_max": `    identity:
      resource:
        class: interface
        key_from_varbind: ifIndex
        max: 48
`,
		"state_varbind": `    state:
      varbind: testValue
`,
		"state_set": `    state:
      set: 1
`,
		"state_clear": `    state:
      clear: 0
`,
		"state_ttl_behavior": `    state:
      ttl_behavior: clear_and_keep
`,
		"chart_meta": `    chart_meta:
      title: Test events
`,
		"map_where": `    where:
      testValue: 1
`,
	}

	for name, removed := range tests {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			profile := "metrics:\n  - name: test.removed\n    type: counter\n" + removed
			writeProfileYAML(t, dir, "profile.yaml", profile)
			if _, err := loadProfileBundle(filepath.Join(dir, "profile.yaml")); err == nil {
				t.Fatalf("loadProfileBundle accepted removed metric syntax %s", name)
			}
		})
	}
}

func TestLoadProfileRejectsUnknownProfileMetricChartKeys(t *testing.T) {
	tests := map[string]string{
		"dimensions": `
charts:
  - id: bad_chart
    title: Bad chart
    context: snmp.trap.bad.chart
    units: events/s
    dimensions:
      - selector: snmp_trap_bad
        name: bad
`,
		"lifecycle": `
charts:
  - id: bad_chart
    title: Bad chart
    context: snmp.trap.bad.chart
    units: events/s
    lifecycle:
      typo: bad
`,
	}

	for name, profile := range tests {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			writeProfileYAML(t, dir, "profile.yaml", profile)
			if _, err := loadProfileBundle(filepath.Join(dir, "profile.yaml")); err == nil {
				t.Fatalf("loadProfileBundle accepted unknown chart %s key", name)
			}
		})
	}
}

func TestProfileMetricValidationResourceClassPolicy(t *testing.T) {
	idx := newPopulatedTestMetricEpoch(t)
	err := idx.addTestMetricDefinitions([]profileMetricRule{{
		Name:     "cisco.port_security.custom_resource_class",
		Type:     profileMetricTypeCounter,
		OnTrap:   testPortSecurityTrapOID,
		Identity: profileMetricIdentity{Resource: &profileMetricResource{Class: "custom", KeyFromVarbind: "ifIndex", MaxPerSource: 48}},
		Output: profileMetricOutput{
			Metric:    "snmp_trap_cisco_custom_resource_class",
			Dimension: "violations",
			Chart:     "custom_resource_class",
		},
		SourceFile: "test-profile.yaml",
	}}, []profileMetricChart{{
		ID:         "custom_resource_class",
		Title:      "Custom resource class",
		Context:    "snmp.trap.cisco.custom.resource.class",
		Units:      "events/s",
		Algorithm:  "incremental",
		SourceFile: "test-profile.yaml",
	}})
	if err == nil {
		t.Fatalf("addProfileMetrics accepted non-stock resource class without site_ prefix")
	}

	idx = newPopulatedTestMetricEpoch(t)
	err = idx.addTestMetricDefinitions([]profileMetricRule{{
		Name:     "cisco.port_security.site_resource_class",
		Type:     profileMetricTypeCounter,
		OnTrap:   testPortSecurityTrapOID,
		Identity: profileMetricIdentity{Resource: &profileMetricResource{Class: "site_lab_port", KeyFromVarbind: "ifIndex", MaxPerSource: 48}},
		Output: profileMetricOutput{
			Metric:    "snmp_trap_cisco_site_resource_class",
			Dimension: "violations",
			Chart:     "site_resource_class",
		},
		SourceFile: "test-profile.yaml",
	}}, []profileMetricChart{{
		ID:         "site_resource_class",
		Title:      "Site resource class",
		Context:    "snmp.trap.cisco.site.resource.class",
		Units:      "events/s",
		Algorithm:  "incremental",
		SourceFile: "test-profile.yaml",
	}})
	if err != nil {
		t.Fatalf("addProfileMetrics rejected site-prefixed resource class: %v", err)
	}
}

func TestProfileMetricValidationRejectsNonNumericPredicateBounds(t *testing.T) {
	tests := map[string]struct {
		pred      profileMetricPredicate
		wantError string
	}{
		"greater_than": {
			pred: profileMetricPredicate{
				Varbind:     testCiscoCommandSourceVarbind,
				GreaterThan: "not-a-number",
			},
			wantError: "must be a finite number",
		},
		"less_than": {
			pred: profileMetricPredicate{
				Varbind:  testCiscoCommandSourceVarbind,
				LessThan: "not-a-number",
			},
			wantError: "must be a finite number",
		},
		"range_lower": {
			pred: profileMetricPredicate{
				Varbind: testCiscoCommandSourceVarbind,
				Range:   []any{"not-a-number", 4},
			},
			wantError: "must be a finite number",
		},
		"range_upper": {
			pred: profileMetricPredicate{
				Varbind: testCiscoCommandSourceVarbind,
				Range:   []any{1, "not-a-number"},
			},
			wantError: "must be a finite number",
		},
		"range_reversed": {
			pred: profileMetricPredicate{
				Varbind: testCiscoCommandSourceVarbind,
				Range:   []any{4, 1},
			},
			wantError: "range[0] must be less than or equal to range[1]",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			idx := newPopulatedTestMetricEpoch(t)
			err := idx.addTestMetricDefinitions([]profileMetricRule{{
				Name:   "cisco.config.bad_" + name,
				Type:   profileMetricTypeCounter,
				OnTrap: testCiscoConfigTrapOID,
				Where:  profileMetricPredicates{tc.pred},
				Output: profileMetricOutput{
					Metric:    "snmp_trap_cisco_bad_" + name,
					Dimension: "bad_" + name,
					Chart:     "cisco_config_changes",
				},
				SourceFile: "test-profile.yaml",
			}}, nil)
			if err == nil || !strings.Contains(err.Error(), tc.wantError) {
				t.Fatalf("addProfileMetrics error = %v, want %q", err, tc.wantError)
			}
		})
	}
}

func TestProfileMetricValidationRejectsUnsupportedPublicConfig(t *testing.T) {
	idx := newPopulatedTestMetricEpoch(t)
	err := idx.addTestMetricDefinitions([]profileMetricRule{{
		Name:             "cisco.config.bad_missing",
		Type:             profileMetricTypeSample,
		OnTrap:           testCiscoConfigTrapOID,
		ValueFromVarbind: testCiscoTerminalTypeVarbind,
		Missing:          "unknown_dimension",
		Output: profileMetricOutput{
			Metric:    "snmp_trap_cisco_bad_missing",
			Dimension: "value",
			Chart:     "cisco_terminal_type",
		},
		SourceFile: "test-profile.yaml",
	}}, nil)
	if err == nil {
		t.Fatalf("addProfileMetrics accepted sample missing=unknown_dimension without resource identity")
	}

	err = idx.addTestMetricDefinitions(nil, []profileMetricChart{{
		ID:         "profile_metric_diagnostics",
		Title:      "Reserved diagnostics",
		Context:    "snmp.trap.site.diagnostics",
		Units:      "events/s",
		Algorithm:  "incremental",
		SourceFile: "test-profile.yaml",
	}})
	if err == nil {
		t.Fatalf("addProfileMetrics accepted reserved diagnostics chart id")
	}

	err = idx.addTestMetricDefinitions(nil, []profileMetricChart{{
		ID:         "site_events",
		Title:      "Reserved events context",
		Context:    "snmp.trap.events",
		Units:      "events/s",
		Algorithm:  "incremental",
		SourceFile: "test-profile.yaml",
	}})
	if err == nil {
		t.Fatalf("addProfileMetrics accepted reserved built-in chart context")
	}

	err = idx.addTestMetricDefinitions(nil, []profileMetricChart{{
		ID:         "site_profile_metric_diagnostics",
		Title:      "Reserved profile metric diagnostics context",
		Context:    "snmp.trap.profile_metric_diagnostics",
		Units:      "events/s",
		Algorithm:  "incremental",
		SourceFile: "test-profile.yaml",
	}})
	if err == nil {
		t.Fatalf("addProfileMetrics accepted reserved profile metric diagnostics chart context")
	}

	err = idx.addTestMetricDefinitions([]profileMetricRule{{
		Name:   "cisco.config.bad_diagnostic_prefix",
		Type:   profileMetricTypeCounter,
		OnTrap: testCiscoConfigTrapOID,
		Output: profileMetricOutput{
			Metric:    "snmp_trap_profile_metrics_custom",
			Dimension: "events",
			Chart:     "cisco_config_changes",
		},
		SourceFile: "test-profile.yaml",
	}}, nil)
	if err == nil {
		t.Fatalf("addProfileMetrics accepted reserved profile metric diagnostics prefix")
	}

	err = idx.addTestMetricDefinitions([]profileMetricRule{{
		Name:   "cisco.config.bad_not_absent",
		Type:   profileMetricTypeCounter,
		OnTrap: testCiscoConfigTrapOID,
		Where: profileMetricPredicates{{
			Varbind: testCiscoTerminalTypeVarbind,
			Absent:  new(true),
			Not:     true,
		}},
		Output: profileMetricOutput{
			Metric:    "snmp_trap_cisco_bad_not_absent",
			Dimension: "events",
			Chart:     "cisco_config_changes",
		},
		SourceFile: "test-profile.yaml",
	}}, nil)
	if err == nil {
		t.Fatalf("addProfileMetrics accepted not with absent predicate")
	}

	err = idx.addTestMetricDefinitions([]profileMetricRule{{
		Name:   "cisco.config.bad_range",
		Type:   profileMetricTypeCounter,
		OnTrap: testCiscoConfigTrapOID,
		Where: profileMetricPredicates{{
			Varbind: testCiscoCommandSourceVarbind,
			Range:   []any{1},
		}},
		Output: profileMetricOutput{
			Metric:    "snmp_trap_cisco_bad_range",
			Dimension: "events",
			Chart:     "cisco_config_changes",
		},
		SourceFile: "test-profile.yaml",
	}}, nil)
	if err == nil {
		t.Fatalf("addProfileMetrics accepted one-sided range predicate")
	}

	err = idx.addTestMetricDefinitions([]profileMetricRule{{
		Name:   "cisco.config.bad_empty_predicate",
		Type:   profileMetricTypeCounter,
		OnTrap: testCiscoConfigTrapOID,
		Where: profileMetricPredicates{{
			Varbind: testCiscoTerminalTypeVarbind,
		}},
		Output: profileMetricOutput{
			Metric:    "snmp_trap_cisco_bad_empty_predicate",
			Dimension: "events",
			Chart:     "cisco_config_changes",
		},
		SourceFile: "test-profile.yaml",
	}}, nil)
	if err == nil {
		t.Fatalf("addProfileMetrics accepted predicate without condition")
	}

	err = idx.addTestMetricDefinitions([]profileMetricRule{{
		Name:   "cisco.config.bad_where_varbind",
		Type:   profileMetricTypeCounter,
		OnTrap: testCiscoConfigTrapOID,
		Where: profileMetricPredicates{{
			Varbind: "missingVarbind",
			Equals:  1,
		}},
		Output: profileMetricOutput{
			Metric:    "snmp_trap_cisco_bad_where_varbind",
			Dimension: "events",
			Chart:     "cisco_config_changes",
		},
		SourceFile: "test-profile.yaml",
	}}, nil)
	if err == nil {
		t.Fatalf("addProfileMetrics accepted where predicate with unknown varbind")
	}

	err = idx.addTestMetricDefinitions([]profileMetricRule{{
		Name:   "cisco.config.bad_where_field",
		Type:   profileMetricTypeCounter,
		OnTrap: testCiscoConfigTrapOID,
		Where: profileMetricPredicates{{
			Field:  "missing_field",
			Equals: "value",
		}},
		Output: profileMetricOutput{
			Metric:    "snmp_trap_cisco_bad_where_field",
			Dimension: "events",
			Chart:     "cisco_config_changes",
		},
		SourceFile: "test-profile.yaml",
	}}, nil)
	if err == nil {
		t.Fatalf("addProfileMetrics accepted where predicate with unknown synthetic field")
	}

	err = idx.addTestMetricDefinitions([]profileMetricRule{{
		Name:   "cisco.config.bad_state_set_varbind",
		Type:   profileMetricTypeState,
		OnTrap: testCiscoConfigTrapOID,
		State: profileMetricState{
			SetWhen:   &profileMetricPredicate{Varbind: "missingVarbind", Equals: 1},
			ClearWhen: &profileMetricPredicate{Varbind: testCiscoCommandSourceVarbind, Equals: 2},
		},
		Output: profileMetricOutput{
			Metric:    "snmp_trap_cisco_bad_state_set_varbind",
			Dimension: "state",
			Chart:     "cisco_terminal_type",
		},
		SourceFile: "test-profile.yaml",
	}}, nil)
	if err == nil {
		t.Fatalf("addProfileMetrics accepted state.set_when predicate with unknown varbind")
	}

	err = idx.addTestMetricDefinitions([]profileMetricRule{{
		Name:   "cisco.config.bad_ttl",
		Type:   profileMetricTypeState,
		OnTrap: testCiscoConfigTrapOID,
		State: profileMetricState{
			SetWhen:   &profileMetricPredicate{Varbind: testCiscoCommandSourceVarbind, Equals: 2},
			ClearWhen: &profileMetricPredicate{Varbind: testCiscoCommandSourceVarbind, Equals: 3},
			TTL:       "not-a-duration",
		},
		Output: profileMetricOutput{
			Metric:    "snmp_trap_cisco_bad_ttl",
			Dimension: "state",
			Chart:     "cisco_terminal_type",
		},
		SourceFile: "test-profile.yaml",
	}}, nil)
	if err == nil {
		t.Fatalf("addProfileMetrics accepted invalid state.ttl")
	}

	for name, ttl := range map[string]string{
		"negative_ttl": "-1h",
		"zero_ttl":     "0s",
	} {
		t.Run(name, func(t *testing.T) {
			idx := newPopulatedTestMetricEpoch(t)
			err := idx.addTestMetricDefinitions([]profileMetricRule{{
				Name:   "cisco.config." + name,
				Type:   profileMetricTypeState,
				OnTrap: testCiscoConfigTrapOID,
				State: profileMetricState{
					SetWhen:   &profileMetricPredicate{Varbind: testCiscoCommandSourceVarbind, Equals: 2},
					ClearWhen: &profileMetricPredicate{Varbind: testCiscoCommandSourceVarbind, Equals: 3},
					TTL:       ttl,
				},
				Output: profileMetricOutput{
					Metric:    "snmp_trap_cisco_" + name,
					Dimension: "state",
					Chart:     "cisco_terminal_type",
				},
				SourceFile: "test-profile.yaml",
			}}, nil)
			if err == nil || !strings.Contains(err.Error(), "must be greater than zero") {
				t.Fatalf("addProfileMetrics error = %v, want positive state.ttl validation error", err)
			}
		})
	}

	err = idx.addTestMetricDefinitions([]profileMetricRule{{
		Name:             "cisco.config.bad_multiplier",
		Type:             profileMetricTypeSample,
		OnTrap:           testCiscoConfigTrapOID,
		ValueFromVarbind: testCiscoTerminalTypeVarbind,
		Scale:            profileMetricScale{Multiplier: -1, Divisor: 1},
		Output: profileMetricOutput{
			Metric:    "snmp_trap_cisco_bad_multiplier",
			Dimension: "value",
			Chart:     "cisco_terminal_type",
		},
		SourceFile: "test-profile.yaml",
	}}, nil)
	if err == nil {
		t.Fatalf("addProfileMetrics accepted negative scale multiplier")
	}

	err = idx.addTestMetricDefinitions([]profileMetricRule{{
		Name:   "cisco.config.bad_chart_algorithm",
		Type:   profileMetricTypeCounter,
		OnTrap: testCiscoConfigTrapOID,
		Output: profileMetricOutput{
			Metric:    "snmp_trap_cisco_bad_chart_algorithm",
			Dimension: "events",
			Chart:     "bad_chart_algorithm",
		},
		SourceFile: "test-profile.yaml",
	}}, []profileMetricChart{{
		ID:         "bad_chart_algorithm",
		Title:      "Bad chart algorithm",
		Context:    "snmp.trap.cisco.bad.chart.algorithm",
		Units:      "events/s",
		Algorithm:  "percentage-of-incremental-row",
		SourceFile: "test-profile.yaml",
	}})
	if err == nil {
		t.Fatalf("addProfileMetrics accepted framework-unsupported chart algorithm")
	}

	err = idx.addTestMetricDefinitions([]profileMetricRule{{
		Name:   "cisco.config.bad_chart_type",
		Type:   profileMetricTypeCounter,
		OnTrap: testCiscoConfigTrapOID,
		Output: profileMetricOutput{
			Metric:    "snmp_trap_cisco_bad_chart_type",
			Dimension: "events",
			Chart:     "bad_chart_type",
		},
		SourceFile: "test-profile.yaml",
	}}, []profileMetricChart{{
		ID:         "bad_chart_type",
		Title:      "Bad chart type",
		Context:    "snmp.trap.cisco.bad.chart.type",
		Units:      "events/s",
		Algorithm:  "incremental",
		Type:       "pie",
		SourceFile: "test-profile.yaml",
	}})
	if err == nil {
		t.Fatalf("addProfileMetrics accepted framework-unsupported chart type")
	}

	err = idx.addTestMetricDefinitions([]profileMetricRule{{
		Name:   "cisco.config.duplicate_output_metric",
		Type:   profileMetricTypeCounter,
		OnTrap: testCiscoConfigTrapOID,
		Output: profileMetricOutput{
			Metric:    "snmp_trap_cisco_config_events",
			Dimension: "duplicate",
			Chart:     "cisco_config_changes",
		},
		SourceFile: "test-profile.yaml",
	}}, nil)
	if err == nil {
		t.Fatalf("addProfileMetrics accepted duplicate output.metric")
	}

	err = idx.addTestMetricDefinitions([]profileMetricRule{{
		Name:     "cisco.port_security.bad_negative_resource_cap",
		Type:     profileMetricTypeCounter,
		OnTrap:   testPortSecurityTrapOID,
		Identity: profileMetricIdentity{Resource: &profileMetricResource{Class: "interface", KeyFromVarbind: "ifIndex", MaxPerSource: -1}},
		Output: profileMetricOutput{
			Metric:    "snmp_trap_cisco_bad_negative_resource_cap",
			Dimension: "violations",
			Chart:     "port_security_violations",
		},
		SourceFile: "test-profile.yaml",
	}}, nil)
	if err == nil {
		t.Fatalf("addProfileMetrics accepted negative resource max_per_source")
	}

	err = idx.addTestMetricDefinitions([]profileMetricRule{{
		Name:     "cisco.config.resource_on_non_resource_chart",
		Type:     profileMetricTypeCounter,
		OnTrap:   testPortSecurityTrapOID,
		Identity: profileMetricIdentity{Resource: &profileMetricResource{Class: "interface", KeyFromVarbind: "ifIndex", MaxPerSource: 48}},
		Output: profileMetricOutput{
			Metric:    "snmp_trap_cisco_bad_resource_shape",
			Dimension: "events",
			Chart:     "cisco_config_changes",
		},
		SourceFile: "test-profile.yaml",
	}}, nil)
	if err == nil {
		t.Fatalf("addProfileMetrics accepted mixed resource and non-resource rules on one chart")
	}

	err = idx.addTestMetricDefinitions([]profileMetricRule{{
		Name:     "cisco.port_security.bad_resource_class",
		Type:     profileMetricTypeCounter,
		OnTrap:   testPortSecurityTrapOID,
		Identity: profileMetricIdentity{Resource: &profileMetricResource{Class: "peer", KeyFromVarbind: "ifIndex", MaxPerSource: 48}},
		Output: profileMetricOutput{
			Metric:    "snmp_trap_cisco_bad_resource_class",
			Dimension: "events",
			Chart:     "port_security_violations",
		},
		SourceFile: "test-profile.yaml",
	}}, nil)
	if err == nil {
		t.Fatalf("addProfileMetrics accepted mixed resource classes on one chart")
	}

	const testCiscoUsernameOID = "1.3.6.1.4.1.9.9.43.1.1.1.99"
	configTrap, err := idx.ResolveTrap("CISCO-CONFIG-MAN-MIB::ccmCLIRunningConfigChanged")
	require.NoError(t, err)
	configTrap.SharedVarbinds[testCiscoUsernameOID] = &VarbindDef{
		OID:     testCiscoUsernameOID,
		Type:    "DisplayString",
		RawName: "ccmHistoryEventUser",
	}
	err = idx.addTestMetricDefinitions([]profileMetricRule{{
		Name:     "cisco.config.bad_string_resource",
		Type:     profileMetricTypeCounter,
		OnTrap:   testCiscoConfigTrapOID,
		Identity: profileMetricIdentity{Resource: &profileMetricResource{Class: "user", KeyFromVarbind: "ccmHistoryEventUser", MaxPerSource: 48}},
		Output: profileMetricOutput{
			Metric:    "snmp_trap_cisco_bad_string_resource",
			Dimension: "events",
			Chart:     "bad_string_resource",
		},
		SourceFile: "test-profile.yaml",
	}}, []profileMetricChart{{
		ID:         "bad_string_resource",
		Title:      "Bad string resource",
		Context:    "snmp.trap.cisco.bad.string.resource",
		Units:      "events/s",
		Algorithm:  "incremental",
		SourceFile: "test-profile.yaml",
	}})
	if err == nil {
		t.Fatalf("addProfileMetrics accepted non-integer resource key varbind")
	}
}

func TestProfileMetricValidationAllowsRetiredSourceMetricIdentifiers(t *testing.T) {
	tests := []struct {
		metric  string
		chartID string
		context string
	}{
		{metric: "snmp_trap_source_custom_events", chartID: "sources", context: "snmp.trap.sources"},
		{metric: "snmp_trap_sources_custom_events", chartID: "source_attribution", context: "snmp.trap.source_attribution"},
		{metric: "snmp_trap_site_source_pipeline_events", chartID: "source_pipeline", context: "snmp.trap.source_pipeline"},
		{metric: "snmp_trap_site_source_error_events", chartID: "source_errors", context: "snmp.trap.source_errors"},
		{metric: "snmp_trap_site_source_seen_events", chartID: "source_last_seen", context: "snmp.trap.source_last_seen"},
	}
	for _, tc := range tests {
		t.Run(tc.chartID, func(t *testing.T) {
			idx := newPopulatedTestMetricEpoch(t)
			err := idx.addTestMetricDefinitions([]profileMetricRule{{
				Name:   "site.source." + tc.chartID,
				Type:   profileMetricTypeCounter,
				OnTrap: testCiscoConfigTrapOID,
				Output: profileMetricOutput{
					Metric:    tc.metric,
					Dimension: "events",
					Chart:     tc.chartID,
				},
				SourceFile: "test-profile.yaml",
			}}, []profileMetricChart{{
				ID:         tc.chartID,
				Title:      "Site source events",
				Context:    tc.context,
				Units:      "events/s",
				Algorithm:  "incremental",
				SourceFile: "test-profile.yaml",
			}})
			if err != nil {
				t.Fatalf("addProfileMetrics rejected identifiers released with retired built-in source metrics: %v", err)
			}
		})
	}
}
