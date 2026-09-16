// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/redfish/internal/registry"
)

func TestGeneratedArtifactsAreCurrent(t *testing.T) {
	root, err := packageRoot()
	if err != nil {
		t.Fatal(err)
	}
	repository, err := repositoryRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	contract, err := registry.Compile()
	if err != nil {
		t.Fatal(err)
	}

	for _, target := range artifactTargets(root, repository, contract) {
		t.Run(filepath.Base(filepath.Dir(target.path))+"_"+filepath.Base(target.path), func(t *testing.T) {
			want, err := target.render()
			if err != nil {
				t.Fatal(err)
			}
			got, err := os.ReadFile(target.path)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, want) {
				t.Fatalf("%s is stale; run go generate ./plugin/go.d/collector/redfish", target.path)
			}
		})
	}
}

func TestRenderChartsUsesLintCompliantSequenceIndentationAndRoundTrips(t *testing.T) {
	raw, err := renderCharts(registry.MustCompile(), "redfish")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte("groups:\n- ")) {
		t.Fatal("top-level groups sequence is not indented")
	}
	if !bytes.Contains(raw, []byte("groups:\n  - family:")) {
		t.Fatal("rendered chart template does not contain the expected indented groups sequence")
	}
	if _, err := charttpl.DecodeYAML(raw); err != nil {
		t.Fatalf("rendered chart template does not round-trip through the runtime decoder: %v", err)
	}
}

func TestRenderHealth(t *testing.T) {
	contract, err := registry.Compile()
	if err != nil {
		t.Fatal(err)
	}

	raw, err := renderHealth(contract)
	if err != nil {
		t.Fatal(err)
	}
	value := string(raw)
	for _, expected := range []string{
		"on: system.hw.sensor.temperature.alarm",
		"chart labels: _collect_module=redfish",
		"on: redfish.collection.status",
		"on: redfish.log_backend.state",
		"on: redfish.log_service.ingestion_state",
		"delay: up 30m down 5m multiplier 1.5 max 1h",
	} {
		if !strings.Contains(value, expected) {
			t.Errorf("generated health manifest does not contain %q", expected)
		}
	}
}

func TestReadingAlarmRulesCoverEveryNonClearState(t *testing.T) {
	contract, err := registry.Compile()
	if err != nil {
		t.Fatal(err)
	}
	rules, err := compileHealthRules(contract)
	if err != nil {
		t.Fatal(err)
	}

	const (
		calculation = "$warning + $cap + $alarm + $critical + $emergency + $fault"
		warning     = "$warning > 0 OR $cap > 0 OR $alarm > 0"
		critical    = "$critical > 0 OR $emergency > 0 OR $fault > 0"
	)
	alarmContexts := make(map[string]struct{})
	for _, chart := range contract.Charts {
		if chart.Class == registry.ClassReadingAlarm ||
			(chart.Class == registry.ClassCategoricalParent &&
				(chart.Context == "redfish.aggregate.reading_alarm" ||
					strings.HasSuffix(chart.Context, ".alarm"))) {
			alarmContexts[chart.Context] = struct{}{}
		}
	}
	if len(alarmContexts) == 0 {
		t.Fatal("registry has no reading alarm contexts")
	}

	for _, rule := range rules {
		if _, ok := alarmContexts[rule.Context]; !ok {
			continue
		}
		delete(alarmContexts, rule.Context)
		if rule.Calculation != calculation {
			t.Errorf("%s calculation = %q, want %q", rule.Context, rule.Calculation, calculation)
		}
		if rule.Warning != warning {
			t.Errorf("%s warning expression = %q, want %q", rule.Context, rule.Warning, warning)
		}
		if rule.Critical != critical {
			t.Errorf("%s critical expression = %q, want %q", rule.Context, rule.Critical, critical)
		}
		if strings.HasPrefix(rule.Context, "system.hw.sensor.") &&
			rule.ChartLabels != "_collect_module=redfish" {
			t.Errorf("%s chart label filter = %q", rule.Context, rule.ChartLabels)
		}
	}
	for context := range alarmContexts {
		t.Errorf("reading alarm context %q has no health rule", context)
	}
}

func TestReadingAlarmRulesDoNotAlertForClearUnknownOrMissingState(t *testing.T) {
	contract := registry.MustCompile()
	rules, err := compileHealthRules(contract)
	if err != nil {
		t.Fatal(err)
	}

	want := map[string]string{
		"":          "clear",
		"clear":     "clear",
		"unknown":   "clear",
		"warning":   "warning",
		"cap":       "warning",
		"alarm":     "warning",
		"critical":  "critical",
		"emergency": "critical",
		"fault":     "critical",
	}
	checked := 0
	for _, rule := range rules {
		chart, ok := chartByContext(contract.Charts, rule.Context)
		if !ok || (chart.Class != registry.ClassReadingAlarm &&
			!(chart.Class == registry.ClassCategoricalParent &&
				(chart.Context == "redfish.aggregate.reading_alarm" || strings.HasSuffix(chart.Context, ".alarm")))) {
			continue
		}
		checked++
		for state, severity := range want {
			got := "clear"
			if state != "" && strings.Contains(rule.Critical, "$"+state+" > 0") {
				got = "critical"
			} else if state != "" && strings.Contains(rule.Warning, "$"+state+" > 0") {
				got = "warning"
			}
			if got != severity {
				t.Errorf("%s state %q evaluates as %q, want %q", rule.Context, state, got, severity)
			}
		}
	}
	if checked == 0 {
		t.Fatal("no reading alarm rules were checked")
	}
}

func chartByContext(charts []registry.ChartSpec, context string) (registry.ChartSpec, bool) {
	for _, chart := range charts {
		if chart.Context == context {
			return chart, true
		}
	}
	return registry.ChartSpec{}, false
}

func TestCompileHealthRulesReferenceExactChartContexts(t *testing.T) {
	contract, err := registry.Compile()
	if err != nil {
		t.Fatal(err)
	}
	rules, err := compileHealthRules(contract)
	if err != nil {
		t.Fatal(err)
	}

	contexts := make(map[string]struct{}, len(contract.Charts))
	for _, chart := range contract.Charts {
		contexts[chart.Context] = struct{}{}
	}
	for _, rule := range rules {
		if _, ok := contexts[rule.Context]; !ok {
			t.Errorf("health rule %q references unknown context %q", rule.Name, rule.Context)
		}
	}
}

func TestRenderChartsRejectsUnsupportedContext(t *testing.T) {
	contract := registry.Contract{Charts: []registry.ChartSpec{{
		Module: "redfish", Context: "unsupported.context",
	}}}
	_, err := renderCharts(contract, "redfish")
	if err == nil || !strings.Contains(err.Error(), `unsupported chart context "unsupported.context"`) {
		t.Fatalf("renderCharts() error = %v, want unsupported-context error", err)
	}
}
