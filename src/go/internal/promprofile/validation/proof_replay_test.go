// SPDX-License-Identifier: GPL-3.0-or-later

package promvalidation

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/netdata/netdata/go/plugins/internal/promprofile/input"
)

func TestReplayProofCaseUsesPersistentSemanticSnapshots(t *testing.T) {
	dir := t.TempDir()
	profilePath := filepath.Join(dir, "candidate.yaml")
	firstPath := filepath.Join(dir, "first.prom")
	secondPath := filepath.Join(dir, "second.prom")
	writeTestFile(t, profilePath, singleInstanceValueGaugeProfile)
	writeTestFile(t, firstPath, "# TYPE app_value gauge\napp_value{instance=\"node-a\"} 7\n")
	writeTestFile(t, secondPath, "# TYPE app_value gauge\napp_value{instance=\"node-a\"} 9\n")

	results, err := ReplayProofCase(context.Background(), prominput.ReplayCase{
		ProfilePath:    profilePath,
		FixturePaths:   []string{firstPath, secondPath},
		DefaultJobName: "candidate",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Fatalf("result count = %d, want 2", len(results))
	}
	for index, result := range results {
		if len(result.Snapshot.Errors) != 0 || result.Snapshot.Semantics == nil {
			t.Fatalf("step %d result = %#v", index, result)
		}
	}
	if countSemanticActions(results[0].Snapshot.Semantics, "create_chart") != 1 ||
		countSemanticActions(results[1].Snapshot.Semantics, "create_chart") != 0 {
		t.Fatalf("proof replay did not preserve chart state: first=%#v second=%#v",
			results[0].Snapshot.Semantics.PlanActions, results[1].Snapshot.Semantics.PlanActions)
	}
}

func TestReplayProofCaseReturnsStandaloneValidatorFailureWithoutSemantics(t *testing.T) {
	dir := t.TempDir()
	profilePath := filepath.Join(dir, "candidate.yaml")
	fixturePath := filepath.Join(dir, "duplicate.prom")
	writeTestFile(t, profilePath, singleInstanceValueGaugeProfile)
	writeTestFile(t, fixturePath, "# TYPE app_value gauge\napp_value{instance=\"node-a\"} 7\napp_value{instance=\"node-a\"} 9\n")

	results, err := ReplayProofCase(context.Background(), prominput.ReplayCase{
		ProfilePath:    profilePath,
		FixturePaths:   []string{fixturePath},
		DefaultJobName: "candidate",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("result count = %d, want 1", len(results))
	}
	if results[0].Snapshot.Errors["duplicate_source_sample"] != 1 {
		t.Fatalf("errors = %v, want duplicate_source_sample", results[0].Snapshot.Errors)
	}
	if results[0].Snapshot.Semantics != nil {
		t.Fatalf("semantic snapshot = %#v, want nil", results[0].Snapshot.Semantics)
	}
}

func TestReplayProofCaseRejectsTruncatedLifecycleFailure(t *testing.T) {
	dir := t.TempDir()
	profilePath := filepath.Join(dir, "candidate.yaml")
	invalidPath := filepath.Join(dir, "invalid.prom")
	validPath := filepath.Join(dir, "valid.prom")
	writeTestFile(t, profilePath, singleInstanceValueGaugeProfile)
	writeTestFile(t, invalidPath, "# TYPE app_value gauge\napp_value{instance=\"node-a\"} 7\napp_value{instance=\"node-a\"} 9\n")
	writeTestFile(t, validPath, "# TYPE app_value gauge\napp_value{instance=\"node-a\"} 7\n")

	results, err := ReplayProofCase(context.Background(), prominput.ReplayCase{
		ProfilePath:    profilePath,
		FixturePaths:   []string{invalidPath, validPath},
		DefaultJobName: "candidate",
	})
	if err == nil {
		t.Fatalf("ReplayProofCase() returned %d partial result(s) without an error", len(results))
	}
	if results != nil {
		t.Fatalf("results = %#v, want nil on incomplete lifecycle replay", results)
	}
}

func TestResolveMetadataJobSelectsExactMergedJob(t *testing.T) {
	path := filepath.Join(t.TempDir(), "metadata.yaml")
	content := `
modules:
  - meta:
      id: collector-prometheus-app
    setup:
      configuration:
        examples:
          list:
            - name: App
              config: |
                jobs:
                  - &base
                    name: primary
                    url: http://127.0.0.1:9090/metrics
                    expected_prefix: app_
                    max_time_series: 100
                  - <<: *base
                    name: secondary
                    url: http://127.0.0.1:9091/metrics
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	job, err := resolveMetadataJob(path, prominput.MetadataExample{
		IntegrationID: "collector-prometheus-app",
		ExampleName:   "App",
		JobName:       "secondary",
	})
	if err != nil {
		t.Fatal(err)
	}
	if job["name"] != "secondary" || job["expected_prefix"] != "app_" || job["max_time_series"] != 100 ||
		job["url"] != nil {
		t.Fatalf("resolved job = %#v", job)
	}
}

func TestStageProofJobRejectsProfileOwnedMetadataPolicy(t *testing.T) {
	path := filepath.Join(t.TempDir(), "metadata.yaml")
	content := `
modules:
  - meta: {id: collector-prometheus-app}
    setup:
      configuration:
        examples:
          list:
            - name: App
              config: |
                jobs:
                  - name: local
                    url: http://127.0.0.1:9090/metrics
                    app: app
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	_, cleanup, err := stageProofJob(prominput.ReplayCase{
		DefaultJobName: "app",
		MetadataPath:   path,
		MetadataExample: &prominput.MetadataExample{
			IntegrationID: "collector-prometheus-app", ExampleName: "App", JobName: "local",
		},
	})
	if cleanup != nil {
		cleanup()
	}
	if err == nil {
		t.Fatal("stageProofJob() accepted profile-owned metadata app")
	}
}
