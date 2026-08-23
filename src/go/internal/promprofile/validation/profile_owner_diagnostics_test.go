// SPDX-License-Identifier: GPL-3.0-or-later

package promvalidation

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestValidateComposedProfilePropagatesSupportPriority(t *testing.T) {
	candidate := simpleOwnedProfile("app", "app_value", "")
	support := strings.Replace(
		simpleOwnedProfile("runtime", "runtime_value", ""),
		"      context: runtime_value\n",
		"      priority: 100\n      context: runtime_value\n",
		1,
	)
	report := validateComposedProfiles(t, candidate, support, simpleOwnedDump())
	for _, item := range report.Findings {
		if item.Code == "priority_forbidden" {
			t.Fatalf("explicit support priority must not be rejected: %#v", item)
		}
	}

	var found bool
	for _, item := range report.AuthoredMapping {
		if item.Path == "profiles[runtime].template.charts[0]" {
			found = true
			if item.Priority != 100 {
				t.Fatalf("support priority was not propagated: %#v", item)
			}
		}
	}
	if !found {
		t.Fatalf("support authored mapping was not found: %#v", report.AuthoredMapping)
	}
}

func TestValidateComposedProfileMatchFindingIdentifiesSupportOwner(t *testing.T) {
	candidate := simpleOwnedProfile("app", "app_value", "")
	support := strings.Replace(
		simpleOwnedProfile("runtime", "runtime_value", ""),
		"match: runtime_*",
		"match: 'runtime_* process_*'",
		1,
	)
	report := validateComposedProfiles(t, candidate, support, simpleOwnedDump())

	finding := findFinding(t, report, "generic_profile_match")
	if finding.Path != "profiles[runtime].match" {
		t.Fatalf("support match path: got %q", finding.Path)
	}
}

func TestValidateComposedProfileContributorPolicyChecksSupportOwner(t *testing.T) {
	candidate := simpleOwnedProfile("app", "app_value", "")
	support := strings.Replace(
		simpleOwnedProfile("runtime", "runtime_value", ""),
		"template:\n",
		"autogen:\n  selector:\n    allow: [runtime_*]\ntemplate:\n",
		1,
	)
	report := validateComposedProfiles(t, candidate, support, simpleOwnedDump())

	finding := findFinding(t, report, "closed_profile_fallback")
	if finding.Path != "profiles[runtime].autogen.selector.allow" {
		t.Fatalf("support contributor-policy path: got %q", finding.Path)
	}
}

func TestValidateComposedProfileFutureRunReportsEachOwnerProbe(t *testing.T) {
	report := validateComposedProfiles(
		t,
		simpleOwnedProfile("app", "app_value", ""),
		simpleOwnedProfile("runtime", "runtime_value", ""),
		simpleOwnedDump(),
	)

	if !strings.HasPrefix(report.Profiles.Candidate.FutureRawProbe, "app_") {
		t.Fatalf("candidate future probe: got %q", report.Profiles.Candidate.FutureRawProbe)
	}
	if len(report.Profiles.Supports) != 1 ||
		!strings.HasPrefix(report.Profiles.Supports[0].FutureRawProbe, "runtime_") {
		t.Fatalf("support future probe: got %#v", report.Profiles.Supports)
	}
	var text bytes.Buffer
	if err := writeTextReport(&text, report); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text.String(), "First future raw probe for supporting profile runtime: runtime_") {
		t.Fatalf("text report lost support future-probe ownership:\n%s", text.String())
	}
}

func TestValidateComposedProfileFutureRunAttributesExplicitProbesByCoverage(t *testing.T) {
	profile := func(name string) string {
		return fmt.Sprintf(`
match: %[1]s_*
relabeling:
  - match: %[1]s_worker_*
    metric_relabel_configs:
      - source_labels: [__name__]
        regex: %[1]s_worker_(.+)_(temperature)
        target_label: worker
        replacement: ${1}
      - source_labels: [__name__]
        regex: %[1]s_worker_(.+)_(temperature)
        target_label: __name__
        replacement: %[1]s_${2}
template:
  family: %[1]s
  metrics: [%[1]s_temperature]
  charts:
    - title: Temperature
      context: %[1]s_temperature
      units: Cel
      dimensions:
        - selector: %[1]s_temperature
          name: temperature
`, name)
	}
	dump := `# TYPE app_worker_current_temperature gauge
app_worker_current_temperature 1
# TYPE runtime_worker_current_temperature gauge
runtime_worker_current_temperature 2
`
	job := `future_inputs:
  - name: runtime_worker_future_temperature
  - name: app_worker_future_temperature
`
	report := validateComposedProfilesWithJob(t, profile("app"), profile("runtime"), dump, job)
	if !report.Passed() {
		t.Fatalf("explicit composed future proof failed: %#v", report.Findings)
	}
	if got, want := report.Profiles.Candidate.FutureRawProbe, "app_worker_future_temperature"; got != want {
		t.Fatalf("candidate future probe: got %q, want %q", got, want)
	}
	if got, want := report.Profiles.Supports[0].FutureRawProbe, "runtime_worker_future_temperature"; got != want {
		t.Fatalf("support future probe: got %q, want %q", got, want)
	}
}

func TestValidateComposedJobPolicyUsesProfileNamespaceUnion(t *testing.T) {
	candidate := strings.Replace(
		simpleOwnedProfile("app", "app_value", ""),
		"match: app_*",
		"match: '!runtime_* app_*'",
		1,
	)
	job := `relabeling:
  - match: runtime_*
    metric_relabel_configs:
      - source_labels: [private]
        regex: "1"
        action: drop
`
	report := validateComposedProfilesWithJob(
		t,
		candidate,
		simpleOwnedProfile("runtime", "runtime_value", ""),
		simpleOwnedDump(),
		job,
	)
	finding := findFinding(t, report, "unbounded_relabel_discard")
	if got, want := finding.Path, "relabeling[0].metric_relabel_configs[0].action"; got != want {
		t.Fatalf("composition-wide job finding path: got %q, want %q", got, want)
	}
}

func TestValidateComposedProfileLoadFailureIdentifiesSupportInput(t *testing.T) {
	dir := t.TempDir()
	candidatePath := filepath.Join(dir, "app.yaml")
	supportPath := filepath.Join(dir, "runtime.yaml")
	dumpPath := filepath.Join(dir, "metrics.prom")
	for path, content := range map[string]string{
		candidatePath: simpleOwnedProfile("app", "app_value", ""),
		supportPath:   simpleOwnedProfile("runtime", "runtime_value", "") + "unknown: true\n",
		dumpPath:      simpleOwnedDump(),
	} {
		writeTestFile(t, path, content)
	}

	report := Validate(context.Background(), Options{
		ProfilePath:            candidatePath,
		SupportingProfilePaths: []string{supportPath},
		DumpPath:               dumpPath,
	})
	finding := findFinding(t, report, "profile_load")
	if finding.Path != supportPath {
		t.Fatalf("support load failure path: got %q, want %q", finding.Path, supportPath)
	}
}

func TestValidateComposedProfileRelabelDecodeFailureIdentifiesSupportInput(t *testing.T) {
	dir := t.TempDir()
	candidatePath := filepath.Join(dir, "app.yaml")
	supportPath := filepath.Join(dir, "runtime.yaml")
	dumpPath := filepath.Join(dir, "metrics.prom")
	support := simpleOwnedProfile("runtime", "runtime_value", `relabeling:
  - match: runtime_value
    metric_relabel_configs:
      - action: bogus
`)
	for path, content := range map[string]string{
		candidatePath: simpleOwnedProfile("app", "app_value", ""),
		supportPath:   support,
		dumpPath:      simpleOwnedDump(),
	} {
		writeTestFile(t, path, content)
	}

	report := Validate(context.Background(), Options{
		ProfilePath:            candidatePath,
		SupportingProfilePaths: []string{supportPath},
		DumpPath:               dumpPath,
	})
	finding := findFinding(t, report, "profile_relabeling")
	if finding.Path != supportPath {
		t.Fatalf("support relabel failure path: got %q, want %q", finding.Path, supportPath)
	}
}

func TestValidateComposedProfileRelabelFindingsIdentifyEachOwner(t *testing.T) {
	candidate := simpleOwnedProfile("app", "app_value", `relabeling:
  - match: app_drop
    metric_relabel_configs:
      - source_labels: [__name__]
        regex: app_drop
        action: drop
`)
	support := simpleOwnedProfile("runtime", "runtime_value", `relabeling:
  - match: runtime_drop
    metric_relabel_configs:
      - source_labels: [__name__]
        regex: runtime_drop
        action: drop
`)
	report := validateComposedProfiles(t, candidate, support, simpleOwnedDump()+
		"# TYPE app_drop gauge\napp_drop 1\n# TYPE runtime_drop gauge\nruntime_drop 1\n")

	var paths []string
	for _, item := range report.Findings {
		if item.Code == "profile_relabel_discard_review" {
			paths = append(paths, item.Path)
		}
	}
	slices.Sort(paths)
	want := []string{
		"profiles[app].relabeling[0].metric_relabel_configs[0]",
		"profiles[runtime].relabeling[0].metric_relabel_configs[0]",
	}
	if !slices.Equal(paths, want) {
		t.Fatalf("profile discard paths: got %v, want %v", paths, want)
	}
	checkedExclusions := 0
	for _, item := range report.PipelineExcluded {
		if item.Name != "app_drop" && item.Name != "runtime_drop" {
			continue
		}
		checkedExclusions++
		wantPath := fmt.Sprintf("profiles[%s].relabeling[0].metric_relabel_configs[0]", strings.TrimSuffix(item.Name, "_drop"))
		if !slices.Equal(item.PolicyPaths, []string{wantPath}) {
			t.Fatalf("pipeline exclusion %q policy paths: got %v, want %q", item.Name, item.PolicyPaths, wantPath)
		}
	}
	if checkedExclusions != 2 {
		t.Fatalf("pipeline exclusions with profile provenance: got %d in %#v", checkedExclusions, report.PipelineExcluded)
	}
}

func TestValidateComposedPipelineRenameIdentifiesProfileOwner(t *testing.T) {
	profile := func(name string) string {
		return fmt.Sprintf(`
match: %[1]s_*
relabeling:
  - match: %[1]s_raw
    metric_relabel_configs:
      - source_labels: [__name__]
        regex: %[1]s_raw
        target_label: __name__
        replacement: %[1]s_value
        action: replace
template:
  family: %[1]s
  metrics: [%[1]s_value]
  charts:
    - title: Value
      context: %[1]s_value
      units: values
      dimensions:
        - selector: %[1]s_value
          name: value
`, name)
	}
	dump := "# TYPE app_raw gauge\napp_raw 1\n# TYPE runtime_raw gauge\nruntime_raw 2\n"
	report := validateComposedProfiles(t, profile("app"), profile("runtime"), dump)
	for _, name := range []string{"app", "runtime"} {
		var got *pipelineRenamedReport
		for index := range report.PipelineRenamed {
			if report.PipelineRenamed[index].RawName == name+"_raw" {
				got = &report.PipelineRenamed[index]
				break
			}
		}
		if got == nil {
			t.Fatalf("missing %s rename provenance: %#v", name, report.PipelineRenamed)
		}
		want := []string{fmt.Sprintf("profiles[%s].relabeling[0].metric_relabel_configs[0]", name)}
		if !slices.Equal(got.PolicyPaths, want) {
			t.Fatalf("%s rename policy paths: got %v, want %v", name, got.PolicyPaths, want)
		}
	}
}

func TestValidateComposedProfileInvalidNameDiscardIdentifiesEachOwner(t *testing.T) {
	invalidRename := func(prefix string) string {
		return fmt.Sprintf(`relabeling:
  - match: %[1]s_bad
    metric_relabel_configs:
      - source_labels: [missing]
        regex: (.*)
        target_label: __name__
        replacement: ${1}
        action: replace
`, prefix)
	}
	report := validateComposedProfiles(
		t,
		simpleOwnedProfile("app", "app_value", invalidRename("app")),
		simpleOwnedProfile("runtime", "runtime_value", invalidRename("runtime")),
		simpleOwnedDump()+"# TYPE app_bad gauge\napp_bad 1\n# TYPE runtime_bad gauge\nruntime_bad 1\n",
	)

	finding := findFinding(t, report, "invalid_relabel_metric_name_discard")
	wantPath := "profiles[app].relabeling[0], profiles[runtime].relabeling[0]"
	if finding.Path != wantPath {
		t.Fatalf("invalid-name finding path: got %q, want %q", finding.Path, wantPath)
	}
}

func TestValidateComposedProfileIdentityCollapseIdentifiesContributingOwners(t *testing.T) {
	rename := func(prefix string) string {
		return fmt.Sprintf(`relabeling:
  - match: %[1]s_value
    metric_relabel_configs:
      - source_labels: [__name__]
        regex: %[1]s_value
        target_label: __name__
        replacement: shared_value
        action: replace
`, prefix)
	}
	report := validateComposedProfiles(
		t,
		simpleOwnedProfile("app", "shared_value", rename("app")),
		simpleOwnedProfile("runtime", "shared_value", rename("runtime")),
		simpleOwnedDump(),
	)

	finding := findFinding(t, report, "observed_relabel_identity_collapse")
	wantPaths := []string{
		"profiles[app].relabeling[0].metric_relabel_configs[0]",
		"profiles[runtime].relabeling[0].metric_relabel_configs[0]",
	}
	if finding.Path != strings.Join(wantPaths, ", ") {
		t.Fatalf("identity-collapse finding path: got %q, want %q", finding.Path, strings.Join(wantPaths, ", "))
	}
}

func TestValidateConvergedJobOutputPreservesLaterProfileOwner(t *testing.T) {
	candidate := strings.Replace(simpleOwnedProfile("app", "app_value", `relabeling:
  - match: raw_value
    metric_relabel_configs:
      - source_labels: [__name__]
        regex: raw_value
        target_label: __name__
        replacement: app_value
        action: replace
`), "match: app_*", "match: raw_*", 1)
	support := simpleOwnedProfile("runtime", "runtime_value", "")
	dump := `# TYPE raw_value gauge
raw_value{source="one"} 1
raw_value{source="two"} 2
# TYPE runtime_value gauge
runtime_value 1
`
	job := `relabeling:
  - match: raw_value
    metric_relabel_configs:
      - regex: source
        action: labeldrop
`

	dir := t.TempDir()
	candidatePath := filepath.Join(dir, "app.yaml")
	supportPath := filepath.Join(dir, "runtime.yaml")
	dumpPath := filepath.Join(dir, "metrics.prom")
	jobPath := filepath.Join(dir, "job.yaml")
	for path, content := range map[string]string{
		candidatePath: candidate,
		supportPath:   support,
		dumpPath:      dump,
		jobPath:       job,
	} {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	report := Validate(context.Background(), Options{
		ProfilePath:            candidatePath,
		SupportingProfilePaths: []string{supportPath},
		DumpPath:               dumpPath,
		JobPath:                jobPath,
	})

	finding := findFinding(t, report, "observed_relabel_identity_collapse")
	wantPath := "profiles[app].relabeling[0].metric_relabel_configs[0], relabeling[0].metric_relabel_configs[0]"
	if finding.Path != wantPath {
		t.Fatalf("converged identity-collapse path: got %q, want %q", finding.Path, wantPath)
	}
}

func TestValidateComposedProfileTypedFamilyRejectionIdentifiesContributingOwners(t *testing.T) {
	rename := func(prefix string) string {
		return fmt.Sprintf(`relabeling:
  - match: %[1]s_latency_seconds*
    metric_relabel_configs:
      - source_labels: [__name__]
        regex: %[1]s_latency_seconds(.*)
        target_label: __name__
        replacement: shared_latency_seconds${1}
        action: replace
`, prefix)
	}
	profile := func(prefix string) string {
		return fmt.Sprintf(`
match: %[1]s_*
%[2]s
template:
  family: %[1]s
  metrics: [shared_latency_seconds_bucket]
  charts:
    - title: Latency
      context: latency
      units: observations/s
      type: heatmap
      dimensions:
        - selector: shared_latency_seconds_bucket
`, prefix, rename(prefix))
	}
	dump := func(prefix string) string {
		return fmt.Sprintf(`# TYPE %[1]s_latency_seconds histogram
%[1]s_latency_seconds_bucket{le="1"} 1
%[1]s_latency_seconds_bucket{le="+Inf"} 1
%[1]s_latency_seconds_sum 0.5
%[1]s_latency_seconds_count 1
`, prefix)
	}
	report := validateComposedProfiles(t, profile("app"), profile("runtime"), dump("app")+dump("runtime"))

	finding := findFinding(t, report, "typed_family_relabel_rejected")
	wantPath := "profiles[app].relabeling[0].metric_relabel_configs[0], profiles[runtime].relabeling[0].metric_relabel_configs[0]"
	if finding.Path != wantPath {
		t.Fatalf("typed-family finding path: got %q, want %q", finding.Path, wantPath)
	}
	if !slices.Equal(report.Profiles.Selected, []string{"app", "runtime"}) {
		t.Fatalf("selected profiles on collector failure: got %v", report.Profiles.Selected)
	}
}

func TestValidateComposedProfileTypedFamilyRejectionExcludesUnrelatedIdentityOwner(t *testing.T) {
	rename := func(prefix string, corrupt bool) string {
		policy := fmt.Sprintf(`relabeling:
  - match: %[1]s_latency_seconds*
    metric_relabel_configs:
      - source_labels: [__name__]
        regex: %[1]s_latency_seconds(.*)
        target_label: __name__
        replacement: shared_latency_seconds${1}
        action: replace
`, prefix)
		if corrupt {
			policy += `      - source_labels: [__name__]
        regex: shared_latency_seconds_sum
        action: drop
`
		}
		return policy
	}
	profile := func(prefix string, corrupt bool) string {
		return fmt.Sprintf(`
match: %[1]s_*
%[2]s
template:
  family: %[1]s
  metrics: [shared_latency_seconds_bucket]
  charts:
    - title: Latency
      context: latency
      units: observations/s
      type: heatmap
      instances:
        by_labels: [instance]
      dimensions:
        - selector: shared_latency_seconds_bucket
`, prefix, rename(prefix, corrupt))
	}
	dump := func(prefix, instance string) string {
		return fmt.Sprintf(`# TYPE %[1]s_latency_seconds histogram
%[1]s_latency_seconds_bucket{instance=%[2]q,le="1"} 1
%[1]s_latency_seconds_bucket{instance=%[2]q,le="+Inf"} 1
%[1]s_latency_seconds_sum{instance=%[2]q} 0.5
%[1]s_latency_seconds_count{instance=%[2]q} 1
`, prefix, instance)
	}
	report := validateComposedProfiles(
		t,
		profile("app", true),
		profile("runtime", false),
		dump("app", "a")+dump("runtime", "b"),
	)

	finding := findFinding(t, report, "typed_family_relabel_rejected")
	wantPath := "profiles[app].relabeling[0].metric_relabel_configs[0], profiles[app].relabeling[0].metric_relabel_configs[1]"
	if finding.Path != wantPath {
		t.Fatalf("typed-family finding path: got %q, want %q", finding.Path, wantPath)
	}
}

func TestValidateComposedReportUsesProfileLocalChartPaths(t *testing.T) {
	report := validateComposedProfiles(t, simpleOwnedProfile("app", "app_value", ""), simpleOwnedProfile("runtime", "runtime_value", ""), simpleOwnedDump())
	if !report.Passed() {
		for _, finding := range report.Findings {
			if finding.Code != "rendered_id_collision" {
				t.Fatalf("composed validation failed: %#v", report.Findings)
			}
		}
	}
	want := []string{
		"profiles[app].template.charts[0]",
		"profiles[runtime].template.charts[0]",
	}
	got := make([]string, 0, len(report.AuthoredMapping))
	for _, item := range report.AuthoredMapping {
		got = append(got, item.Path)
	}
	if !slices.Equal(got, want) {
		t.Fatalf("composed authored paths: got %v, want %v", got, want)
	}
	for _, chart := range report.Charts {
		if chart.Autogen {
			continue
		}
		if chart.Profile == "" || !strings.HasPrefix(chart.Path, "profiles["+chart.Profile+"].template") {
			t.Fatalf("materialized chart lost profile ownership: %#v", chart)
		}
	}

	var encoded bytes.Buffer
	if err := WriteJSON(&encoded, report); err != nil {
		t.Fatal(err)
	}
	for _, token := range []string{`"candidate"`, `"supports"`, `"app"`, `"runtime"`} {
		if !strings.Contains(encoded.String(), token) {
			t.Fatalf("composed report does not explicitly model candidate/support profiles; missing %s:\n%s", token, encoded.String())
		}
	}
}

func TestValidateComposedDuplicateRootFamilyIdentifiesBothProfiles(t *testing.T) {
	candidate := strings.Replace(simpleOwnedProfile("app", "app_value", ""), "family: app", "family: Shared", 1)
	support := strings.Replace(simpleOwnedProfile("runtime", "runtime_value", ""), "family: runtime", "family: Shared", 1)
	report := validateComposedProfiles(t, candidate, support, simpleOwnedDump())
	finding := findFinding(t, report, "duplicate_sibling_family")
	want := "profiles[app].template, profiles[runtime].template"
	if finding.Path != want {
		t.Fatalf("duplicate root-family path: got %q, want %q", finding.Path, want)
	}
}

func TestValidateComposedObservedLabelAggregationIdentifiesBothCharts(t *testing.T) {
	dump := "# TYPE app_value gauge\napp_value{region=\"eu\"} 1\n# TYPE runtime_value gauge\nruntime_value{region=\"us\"} 2\n"
	report := validateComposedProfiles(t, simpleOwnedProfile("app", "app_value", ""), simpleOwnedProfile("runtime", "runtime_value", ""), dump)
	finding := findFinding(t, report, "observed_label_aggregation")
	want := "profiles[app].template.charts[0], profiles[runtime].template.charts[0]"
	if finding.Path != want {
		t.Fatalf("observed label aggregation path: got %q, want %q", finding.Path, want)
	}
}

func TestValidateComposedIncrementalUnitsIdentifiesSupportChart(t *testing.T) {
	support := strings.Replace(simpleOwnedProfile("runtime", "runtime_value", ""), "units: values", "units: operations", 1)
	dump := "# TYPE app_value gauge\napp_value 1\n# TYPE runtime_value counter\nruntime_value 2\n"
	report := validateComposedProfiles(t, simpleOwnedProfile("app", "app_value", ""), support, dump)
	finding := findFinding(t, report, "incremental_units_review")
	if finding.Path != "profiles[runtime].template.charts[0]" {
		t.Fatalf("incremental units path: got %q", finding.Path)
	}
}

func TestValidateComposedScaleGapIdentifiesSupportChart(t *testing.T) {
	support := `
match: runtime_*
template:
  family: runtime
  metrics: [runtime_current, runtime_capacity]
  charts:
    - title: Current and Capacity
      context: runtime_capacity
      units: items
      dimensions:
        - selector: runtime_current
          name: current
        - selector: runtime_capacity
          name: capacity
`
	dump := "# TYPE app_value gauge\napp_value 1\n# TYPE runtime_current gauge\nruntime_current 1\n# TYPE runtime_capacity gauge\nruntime_capacity 100\n"
	report := validateComposedProfiles(t, simpleOwnedProfile("app", "app_value", ""), support, dump)
	finding := findFinding(t, report, "observed_scale_gap")
	if finding.Path != "profiles[runtime].template.charts[0]" {
		t.Fatalf("observed scale path: got %q", finding.Path)
	}
}

func TestValidateComposedWireCollisionIdentifiesBothCharts(t *testing.T) {
	profile := func(name, selector, id string) string {
		return fmt.Sprintf(`
match: %[1]s_*
template:
  family: %[1]s
  metrics: [%[2]s]
  charts:
    - id: %q
      title: Value
      context: %[1]s_value
      units: values
      dimensions:
        - selector: %[2]s
          name: value
`, name, selector, id)
	}
	report := validateComposedProfiles(t, profile("app", "app_value", "one"), profile("runtime", "runtime_value", "'one"), simpleOwnedDump())
	finding := findFinding(t, report, "chart_wire_id_collision_observed")
	want := "profiles[app].template.charts[0], profiles[runtime].template.charts[0]"
	if finding.Path != want {
		t.Fatalf("wire collision path: got %q, want %q", finding.Path, want)
	}
	if len(report.ChartWireCollisions) != 1 || !slices.Equal(report.ChartWireCollisions[0].Paths, strings.Split(want, ", ")) {
		t.Fatalf("wire collision report ownership: %#v", report.ChartWireCollisions)
	}
}

func TestValidateComposedAutogenSuppressionIdentifiesEachSelectorOwner(t *testing.T) {
	candidate := simpleOwnedProfile("app", "app_value", "autogen:\n  selector:\n    deny: [app_hidden]\n")
	support := simpleOwnedProfile("runtime", "runtime_value", "autogen:\n  selector:\n    deny: [runtime_hidden]\n")
	dump := simpleOwnedDump() + "# TYPE app_hidden gauge\napp_hidden 1\n# TYPE runtime_hidden gauge\nruntime_hidden 1\n"
	report := validateComposedProfiles(t, candidate, support, dump)
	finding := findFinding(t, report, "profile_suppressed_series")
	want := "profiles[app].autogen.selector, profiles[runtime].autogen.selector"
	if finding.Path != want {
		t.Fatalf("profile suppression path: got %q, want %q", finding.Path, want)
	}
}

func TestValidateComposedSupportDeadChartUsesProfileLocalPath(t *testing.T) {
	support := simpleOwnedProfile("runtime", "runtime_absent", "")
	report := validateComposedProfiles(t, simpleOwnedProfile("app", "app_value", ""), support, simpleOwnedDump())
	finding := findFinding(t, report, "dead_chart")
	if finding.Path != "profiles[runtime].template.charts[0]" {
		t.Fatalf("support dead-chart path: got %q", finding.Path)
	}
}

func TestValidateComposedSupportAuthoredChecksUseProfileLocalPath(t *testing.T) {
	support := strings.Replace(
		simpleOwnedProfile("runtime", "runtime_value", ""),
		"          name: value\n",
		"          name: value\n          options:\n            hidden: true\n",
		1,
	)
	report := validateComposedProfiles(t, simpleOwnedProfile("app", "app_value", ""), support, simpleOwnedDump())
	finding := findFinding(t, report, "all_dimensions_hidden")
	if got, want := finding.Path, "profiles[runtime].template.charts[0]"; got != want {
		t.Fatalf("support authored-check path: got %q, want %q", got, want)
	}
}

func TestValidateComposedSupportDistributionChecksUseProfileLocalPath(t *testing.T) {
	support := `
match: runtime_*
template:
  family: runtime
  metrics: [runtime_latency_seconds_bucket]
  charts:
    - title: Latency
      context: runtime_latency
      units: requests/s
      dimensions:
        - selector: runtime_latency_seconds_bucket
          name_from_label: le
`
	dump := simpleOwnedDump() + `# TYPE runtime_latency_seconds histogram
runtime_latency_seconds_bucket{le="1"} 1
runtime_latency_seconds_bucket{le="+Inf"} 1
runtime_latency_seconds_sum 0.5
runtime_latency_seconds_count 1
`
	report := validateComposedProfiles(t, simpleOwnedProfile("app", "app_value", ""), support, dump)
	finding := findFinding(t, report, "histogram_bucket_units")
	if got, want := finding.Path, "profiles[runtime].template.charts[0]"; got != want {
		t.Fatalf("support distribution-check path: got %q, want %q", got, want)
	}
}

func TestValidateComposedUnexpectedAutogenIdentifiesMatchingSupport(t *testing.T) {
	dump := simpleOwnedDump() + "# TYPE runtime_extra gauge\nruntime_extra 3\n"
	report := validateComposedProfiles(
		t,
		simpleOwnedProfile("app", "app_value", ""),
		simpleOwnedProfile("runtime", "runtime_value", ""),
		dump,
	)
	finding := findFinding(t, report, "unexpected_autogen")
	if got, want := finding.Path, "profiles[runtime].template"; got != want {
		t.Fatalf("unexpected-autogen path: got %q, want %q", got, want)
	}
}

func TestValidateComposedUnexpectedTypedAutogenUsesSourceFamilyOwner(t *testing.T) {
	support := strings.Replace(
		simpleOwnedProfile("runtime", "runtime_value", ""),
		"match: runtime_*",
		"match: runtime_latency_seconds",
		1,
	)
	dump := simpleOwnedDump() + `# TYPE runtime_latency_seconds histogram
runtime_latency_seconds_bucket{le="1"} 1
runtime_latency_seconds_bucket{le="+Inf"} 1
runtime_latency_seconds_sum 0.5
runtime_latency_seconds_count 1
`
	report := validateComposedProfiles(t, simpleOwnedProfile("app", "app_value", ""), support, dump)
	finding := findFinding(t, report, "unexpected_autogen")
	if got, want := finding.Path, "profiles[runtime].template"; got != want {
		t.Fatalf("typed unexpected-autogen path: got %q, want %q", got, want)
	}
}

func TestValidateComposedDuplicateAutogenPoliciesPreserveRuleOwner(t *testing.T) {
	profile := func(name, metric string) string {
		return fmt.Sprintf(`
match: shared_*
autogen:
  selector:
    deny: [shared_hidden]
template:
  family: %[1]s
  metrics: [%[2]s]
  charts:
    - title: Value
      context: %[1]s_value
      units: values
      dimensions: [{selector: %[2]s, name: value}]
`, name, metric)
	}
	report := validateComposedProfiles(
		t,
		profile("app", "shared_app_value"),
		profile("runtime", "shared_runtime_value"),
		"# TYPE shared_app_value gauge\nshared_app_value 1\n"+
			"# TYPE shared_runtime_value gauge\nshared_runtime_value 2\n"+
			"# TYPE shared_hidden gauge\nshared_hidden 3\n",
	)
	if hasFinding(report, "unmatched_series", "error") {
		t.Fatalf("duplicate equivalent autogen policies lost rule ownership: %#v", report.Findings)
	}
	finding := findFinding(t, report, "profile_suppressed_series")
	if got, want := finding.Path, "profiles[app].autogen.selector"; got != want {
		t.Fatalf("duplicate-policy owner path: got %q, want %q", got, want)
	}
}

func TestValidateComposedWireDimensionLossIdentifiesOnlyLosingProfile(t *testing.T) {
	dump := "# TYPE app_value gauge\napp_value{state=\"'\"} 1\n# TYPE runtime_value gauge\nruntime_value 2\n"
	report := validateComposedProfiles(
		t,
		singleDynamicValueGaugeProfile,
		simpleOwnedProfile("runtime", "runtime_value", ""),
		dump,
	)
	finding := findFinding(t, report, "dimension_wire_emission_loss")
	if got, want := finding.Path, "profiles[app].template.charts[0]"; got != want {
		t.Fatalf("wire dimension-loss path: got %q, want %q", got, want)
	}
}

func TestValidateCandidateOnlyChartPathsRemainStable(t *testing.T) {
	result := runValidation(t, simpleOwnedProfile("app", "app_value", ""), "# TYPE app_value gauge\napp_value 1\n", "")
	if len(result.report.AuthoredMapping) != 1 {
		t.Fatalf("authored mapping: %#v", result.report.AuthoredMapping)
	}
	if got, want := result.report.AuthoredMapping[0].Path, "groups[1](app).charts[0]"; got != want {
		t.Fatalf("candidate-only authored path: got %q, want %q", got, want)
	}
}

func TestValidateSequenceResolvesExistingChartOwnerFromAcceptedRoute(t *testing.T) {
	dir := t.TempDir()
	candidatePath := filepath.Join(dir, "app.yaml")
	supportPath := filepath.Join(dir, "runtime.yaml")
	firstDumpPath := filepath.Join(dir, "first.prom")
	secondDumpPath := filepath.Join(dir, "second.prom")
	for path, content := range map[string]string{
		candidatePath:  singleDynamicValueGaugeProfile,
		supportPath:    simpleOwnedProfile("runtime", "runtime_value", ""),
		firstDumpPath:  "# TYPE app_value gauge\napp_value{state=\"ok\"} 1\n# TYPE runtime_value gauge\nruntime_value 2\n",
		secondDumpPath: "# TYPE app_value gauge\napp_value{state=\"'\"} 1\n# TYPE runtime_value gauge\nruntime_value 2\n",
	} {
		writeTestFile(t, path, content)
	}

	reports, err := validateSequence(context.Background(), Options{
		ProfilePath:            candidatePath,
		SupportingProfilePaths: []string{supportPath},
	}, []string{firstDumpPath, secondDumpPath}, validationMode{})
	if err != nil {
		t.Fatal(err)
	}
	if len(reports) != 2 || !reports[0].Passed() {
		t.Fatalf("sequence did not establish the existing chart: %#v", reports)
	}
	finding := findFinding(t, reports[1], "dimension_wire_emission_loss")
	if got, want := finding.Path, "profiles[app].template.charts[0]"; got != want {
		t.Fatalf("existing chart route path: got %q, want %q", got, want)
	}
}

func validateComposedProfiles(t *testing.T, candidate, support, dump string) Report {
	return validateComposedProfilesWithJob(t, candidate, support, dump, "")
}

func validateComposedProfilesWithJob(t *testing.T, candidate, support, dump, job string) Report {
	t.Helper()
	dir := t.TempDir()
	candidatePath := filepath.Join(dir, "app.yaml")
	supportPath := filepath.Join(dir, "runtime.yaml")
	dumpPath := filepath.Join(dir, "metrics.prom")
	jobPath := ""
	for path, content := range map[string]string{
		candidatePath: candidate,
		supportPath:   support,
		dumpPath:      dump,
	} {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if job != "" {
		jobPath = filepath.Join(dir, "job.yaml")
		if err := os.WriteFile(jobPath, []byte(job), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return Validate(context.Background(), Options{
		ProfilePath:            candidatePath,
		SupportingProfilePaths: []string{supportPath},
		DumpPath:               dumpPath,
		JobPath:                jobPath,
	})
}

func simpleOwnedProfile(name, selector, extra string) string {
	return fmt.Sprintf(`
match: %[1]s_*
%[3]s
template:
  family: %[1]s
  metrics: [%[2]s]
  charts:
    - title: Value
      context: %[1]s_value
      units: values
      dimensions:
        - selector: %[2]s
          name: value
`, name, selector, extra)
}

func simpleOwnedDump() string {
	return "# TYPE app_value gauge\napp_value 1\n# TYPE runtime_value gauge\nruntime_value 2\n"
}

func findFinding(t *testing.T, report Report, code string) finding {
	t.Helper()
	for _, item := range report.Findings {
		if item.Code == code {
			return item
		}
	}
	t.Fatalf("missing finding %q in %#v", code, report.Findings)
	return finding{}
}
