// SPDX-License-Identifier: GPL-3.0-or-later

package promvalidation

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/netdata/netdata/go/plugins/internal/promprofile/replay"
)

func TestValidationSequenceRetainsProductionStateAcrossFixtures(t *testing.T) {
	dir := t.TempDir()
	profilePath := filepath.Join(dir, "candidate.yaml")
	firstPath := filepath.Join(dir, "first.prom")
	secondPath := filepath.Join(dir, "second.prom")
	profile := `
match: app_*
app: app
template:
  family: Example
  context_namespace: app
  metrics: [app_value]
  charts:
    - title: Value
      context: value
      units: values
      label_promotion: []
      instances:
        by_labels: [instance]
      dimensions:
        - selector: app_value
          name: value
`
	writeTestFile(t, profilePath, profile)
	writeTestFile(t, firstPath, "# TYPE app_value gauge\napp_value{instance=\"node-a\"} 7\n")
	writeTestFile(t, secondPath, "# TYPE app_value gauge\napp_value{instance=\"node-a\"} 9\n")

	reports, err := validateSequence(context.Background(), Options{ProfilePath: profilePath}, []string{firstPath, secondPath}, validationMode{
		automaticProfileSelection: true,
		defaultJobName:            "candidate",
		semanticFacts:             true,
		semanticCoverageReplay:    true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(reports) != 2 {
		t.Fatalf("report count = %d, want 2; reports=%#v", len(reports), reports)
	}
	snapshots := []*promreplay.SemanticSnapshot{
		reports[0].ResultSnapshot().Semantics,
		reports[1].ResultSnapshot().Semantics,
	}
	for index, snapshot := range snapshots {
		if !slices.Equal(snapshot.SelectedProfiles, []string{"candidate"}) {
			t.Fatalf("step %d selected profiles = %v", index, snapshot.SelectedProfiles)
		}
		if len(snapshot.Sources) != 1 || len(snapshot.Sources[0].Routes) != 1 {
			t.Fatalf("step %d sources = %#v", index, snapshot.Sources)
		}
		if got := snapshot.Sources[0].Routes[0].ChartLabelValues; !slices.Equal(got, []promreplay.SemanticLabel{{
			Name: "instance", Value: "node-a",
		}}) {
			t.Fatalf("step %d chart labels = %#v", index, got)
		}
	}
	if countSemanticActions(snapshots[0], "create_chart") != 1 ||
		countSemanticActions(snapshots[1], "create_chart") != 0 {
		t.Fatalf("chart creation was not persistent: first=%#v second=%#v",
			snapshots[0].PlanActions, snapshots[1].PlanActions)
	}
	if got := semanticUpdatedValue(t, snapshots[0]); got != 7 {
		t.Fatalf("first value = %v, want 7", got)
	}
	if got := semanticUpdatedValue(t, snapshots[1]); got != 9 {
		t.Fatalf("second value = %v, want 9", got)
	}
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func countSemanticActions(snapshot *promreplay.SemanticSnapshot, kind string) int {
	count := 0
	for _, action := range snapshot.PlanActions {
		if action.Kind == kind {
			count++
		}
	}
	return count
}

func semanticUpdatedValue(t *testing.T, snapshot *promreplay.SemanticSnapshot) float64 {
	t.Helper()
	for _, action := range snapshot.PlanActions {
		if action.Kind == "update_dimension" && !action.IsEmpty {
			if !action.Float {
				t.Fatalf("update is not a float action: %#v", action)
			}
			return action.Float64
		}
	}
	t.Fatalf("snapshot has no nonempty dimension update: %#v", snapshot.PlanActions)
	return 0
}
