// SPDX-License-Identifier: GPL-3.0-or-later

package promsemantics

import (
	"context"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/internal/promprofile/replay"
)

func TestCompileSemanticContractAcceptsExactlyOneIdentityAlternatives(t *testing.T) {
	contract := loadTestSemanticContract(t, identityAlternativesDesignV1(), identityAlternativesSourceV1(), "", "")
	if got := contract.Source.Signals["requests"].LabelPresenceConstraints["daemon_identity"].Alternatives; !sameNestedStrings(got, [][]string{{"ceph_daemon"}, {"instance_id"}}) {
		t.Fatalf("source alternatives = %#v", got)
	}
	if got := contract.Design.Entities["service"].Identity.Alternatives; !sameNestedStrings(got, [][]string{{"ceph_daemon"}, {"instance_id"}}) {
		t.Fatalf("entity alternatives = %#v", got)
	}
	if _, err := CompileSemanticContract(context.Background(), SemanticCompileInput{Contract: contract}); err != nil {
		t.Fatalf("CompileSemanticContract() error = %v", err)
	}
}

func TestLoadSemanticContractRejectsInvalidIdentityAlternatives(t *testing.T) {
	tests := map[string]struct {
		design string
		source string
		want   string
	}{
		"entity alternative overlaps required identity": {
			design: strings.Replace(identityAlternativesDesignV1(), "required: []", "required: [ceph_daemon]", 1),
			source: identityAlternativesSourceV1(),
			want:   "both required and alternative",
		},
		"entity alternatives overlap": {
			design: strings.Replace(identityAlternativesDesignV1(), "- [instance_id]", "- [ceph_daemon]", 1),
			source: identityAlternativesSourceV1(),
			want:   "multiple alternatives",
		},
		"constraint has one alternative": {
			design: identityAlternativesDesignV1(),
			source: strings.Replace(identityAlternativesSourceV1(), "          - [instance_id]\n", "", 1),
			want:   "at least two alternatives",
		},
		"constraint references unknown label": {
			design: identityAlternativesDesignV1(),
			source: strings.Replace(identityAlternativesSourceV1(), "- [instance_id]", "- [unknown]", 1),
			want:   `references unknown label "unknown"`,
		},
		"constraint alternatives overlap": {
			design: identityAlternativesDesignV1(),
			source: strings.Replace(identityAlternativesSourceV1(), "- [instance_id]", "- [ceph_daemon]", 1),
			want:   `label "ceph_daemon" is already owned by constraint "daemon_identity"`,
		},
		"constraint label is not globally optional": {
			design: identityAlternativesDesignV1(),
			source: strings.Replace(identityAlternativesSourceV1(), "presence: optional", "presence: required", 1),
			want:   `label "ceph_daemon" must have optional presence`,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			err := loadTestSemanticContractError(t, tc.design, tc.source)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("LoadSemanticContract() error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestCompileSemanticContractRejectsIncompatibleIdentityConstraint(t *testing.T) {
	source := strings.Replace(identityAlternativesSourceV1(), "- [instance_id]", "- [cluster, instance_id]", 1)
	source = strings.Replace(source, "      ceph_daemon:\n", `      cluster:
        meaning: Ceph cluster identity.
        presence: optional
        domain: {kind: open}
        endpoint_cardinality: {kind: singleton}
        stability: stable
        evidence: [daemon_label]
      ceph_daemon:
`, 1)
	contract := loadTestSemanticContract(t, identityAlternativesDesignV1(), source, "", "")
	_, err := CompileSemanticContract(context.Background(), SemanticCompileInput{Contract: contract})
	if err == nil || !strings.Contains(err.Error(), "neither statically guaranteed nor constrained") {
		t.Fatalf("CompileSemanticContract() error = %v, want incompatible-constraint failure", err)
	}
}

func TestCompileSemanticContractAcceptsStaticallyGuaranteedIdentityAlternatives(t *testing.T) {
	design := strings.Replace(identityAlternativesDesignV1(), `      requests:
        signal: requests
        components: [total]`, `      requests:
        signal: requests
        components: [total]
      rgw_requests:
        signal: rgw_requests
        components: [total]`, 1)
	source := strings.Replace(validSourceSemanticsV1, "evidence:\n", `evidence:
  daemon_label:
    kind: label
    upstream: exporter
    locations: [metrics.go:8]
    claim: Each source statically declares its one daemon identity.
`, 1)
	source = strings.Replace(source, "    labels: {}", `    labels:
      ceph_daemon:
        meaning: Non-RGW Ceph daemon identity.
        presence: required
        domain: {kind: open}
        endpoint_cardinality: {kind: singleton}
        stability: stable
        evidence: [daemon_label]`, 1)
	source = strings.Replace(source, "relationships: {}", `  rgw_requests:
    source:
      inline:
        registrations:
          canonical:
            family: {exact: example_rgw_requests_total}
            prometheus: {type: counter, shape: scalar}
            evidence: [request_registration]
    population:
      id: completed_requests
      meaning: Completed application requests.
      evidence: [request_population]
`+validScalarComponentV1+`
    labels:
      instance_id:
        meaning: RGW daemon identity.
        presence: required
        domain: {kind: open}
        endpoint_cardinality: {kind: singleton}
        stability: stable
        evidence: [daemon_label]
    functional_dependencies: {}
relationships: {}`, 1)
	compileTestSemanticContract(t, design, source)

	contract := loadTestSemanticContract(
		t, design, strings.Replace(source, "presence: required", "presence: present", 1), "", "",
	)
	_, err := CompileSemanticContract(context.Background(), SemanticCompileInput{Contract: contract})
	if err == nil || !strings.Contains(err.Error(), "neither statically guaranteed nor constrained") {
		t.Fatalf("CompileSemanticContract(present identity) error = %v, want nonblank-guarantee failure", err)
	}
}

func TestReconcileProductionSourcesEnforcesExactlyOneLabelAlternative(t *testing.T) {
	program := compileTestSemanticContract(t, identityAlternativesDesignV1(), identityAlternativesSourceV1())
	semanticCase, err := program.EvaluateCaseEnvironment(
		context.Background(), map[string]map[string]AxisValue{"example": {}},
	)
	if err != nil {
		t.Fatal(err)
	}
	tests := map[string]struct {
		labels  []promreplay.SemanticLabel
		wantErr string
	}{
		"daemon identity": {labels: semanticTestLabels("ceph_daemon", "osd.0")},
		"RGW identity":    {labels: semanticTestLabels("instance_id", "rgw.a")},
		"neither":         {wantErr: "exactly one complete label alternative"},
		"both": {
			labels:  semanticTestLabels("ceph_daemon", "osd.0", "instance_id", "rgw.a"),
			wantErr: "exactly one complete label alternative",
		},
		"blank is absent": {
			labels:  semanticTestLabels("ceph_daemon", "", "instance_id", ""),
			wantErr: "exactly one complete label alternative",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			snapshot := validProductionSourceSnapshot()
			snapshot.Sources[0].Labels = tc.labels
			snapshot.Sources[0].FinalLabels = tc.labels
			_, err := semanticCase.ReconcileProductionSources(context.Background(), snapshot)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("ReconcileProductionSources() error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("ReconcileProductionSources() error = %v, want %q", err, tc.wantErr)
			}
		})
	}
}

func TestReconcileProductionSourcesRejectsPartialIdentityAlternative(t *testing.T) {
	design := strings.Replace(identityAlternativesDesignV1(), "- [ceph_daemon]", "- [cluster, ceph_daemon]", 1)
	source := strings.Replace(identityAlternativesSourceV1(), "- [ceph_daemon]", "- [cluster, ceph_daemon]", 1)
	source = strings.Replace(source, "      ceph_daemon:\n", `      cluster:
        meaning: Ceph cluster identity.
        presence: optional
        domain: {kind: open}
        endpoint_cardinality: {kind: singleton}
        stability: stable
        evidence: [daemon_label]
      ceph_daemon:
`, 1)
	program := compileTestSemanticContract(t, design, source)
	semanticCase, err := program.EvaluateCaseEnvironment(
		context.Background(), map[string]map[string]AxisValue{"example": {}},
	)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := validProductionSourceSnapshot()
	snapshot.Sources[0].Labels = semanticTestLabels("ceph_daemon", "osd.0")
	snapshot.Sources[0].FinalLabels = snapshot.Sources[0].Labels
	_, err = semanticCase.ReconcileProductionSources(context.Background(), snapshot)
	if err == nil || !strings.Contains(err.Error(), "exactly one complete label alternative") {
		t.Fatalf("ReconcileProductionSources() error = %v, want partial-alternative failure", err)
	}
}

func TestReconcileProductionRoutesUsesSelectedIdentityAlternative(t *testing.T) {
	program := compileTestSemanticContract(t, identityAlternativesDesignV1(), identityAlternativesSourceV1())
	semanticCase, err := program.EvaluateCaseEnvironment(
		context.Background(), map[string]map[string]AxisValue{"example": {}},
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, identity := range []string{"ceph_daemon", "instance_id"} {
		t.Run(identity, func(t *testing.T) {
			snapshot := validProductionRouteSnapshot()
			snapshot.Sources[0].Labels = semanticTestLabels(identity, "daemon-a")
			snapshot.Sources[0].FinalLabels = snapshot.Sources[0].Labels
			route := &snapshot.Sources[0].Routes[0]
			route.IdentityLabels = []string{identity}
			route.ChartLabels = []string{identity}
			route.ChartLabelValues = semanticTestLabels(identity, "daemon-a")
			reconciled := reconcileTestProductionCase(t, semanticCase, snapshot)
			if err := semanticCase.ReconcileProductionRoutes(context.Background(), snapshot, reconciled); err != nil {
				t.Fatalf("ReconcileProductionRoutes() error = %v", err)
			}
		})
	}
}

func TestReconcileProductionClaimsAllowsDifferentIdentityAlternativesInOneCase(t *testing.T) {
	program := compileTestSemanticContract(t, identityAlternativesDesignV1(), identityAlternativesSourceV1())
	semanticCase, err := program.EvaluateCaseEnvironment(
		context.Background(), map[string]map[string]AxisValue{"example": {}},
	)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := validProductionSourceSnapshot()
	snapshot.Sources = []promreplay.SemanticSource{
		{FinalLabels: semanticTestLabels("ceph_daemon", "osd.0")},
		{FinalLabels: semanticTestLabels("instance_id", "rgw.a")},
	}
	reconciled := &ReconciledSemanticCase{Edges: []ReconciledSemanticEdge{
		{SourceIndex: 0, DestinationProfile: "example", View: "requests"},
		{SourceIndex: 1, DestinationProfile: "example", View: "requests"},
	}}
	if err := semanticCase.ReconcileProductionClaims(context.Background(), snapshot, reconciled); err != nil {
		t.Fatalf("ReconcileProductionClaims() error = %v", err)
	}
}

func TestProductionCoverageRequiresEveryLabelPresenceAlternative(t *testing.T) {
	program := compileTestSemanticContract(t, identityAlternativesDesignV1(), identityAlternativesSourceV1())
	semanticCase, err := program.EvaluateCaseEnvironment(
		context.Background(), map[string]map[string]AxisValue{"example": {}},
	)
	if err != nil {
		t.Fatal(err)
	}
	coverage, err := NewProductionCoverage(program)
	if err != nil {
		t.Fatal(err)
	}
	keys := []string{
		"label presence constraint requests/daemon_identity alternative 0",
		"label presence constraint requests/daemon_identity alternative 1",
	}
	for _, key := range keys {
		if _, ok := coverage.required[key]; !ok {
			t.Fatalf("coverage does not require %q", key)
		}
	}
	for index, identity := range []string{"ceph_daemon", "instance_id"} {
		snapshot := validProductionSourceSnapshot()
		snapshot.Sources[0].Labels = semanticTestLabels(identity, "daemon-a")
		snapshot.Sources[0].FinalLabels = snapshot.Sources[0].Labels
		reconciled, err := semanticCase.ReconcileProductionSources(context.Background(), snapshot)
		if err != nil {
			t.Fatal(err)
		}
		if err := coverage.ObserveCase(context.Background(), semanticCase, snapshot, reconciled); err != nil {
			t.Fatal(err)
		}
		if _, ok := coverage.seen[keys[index]]; !ok {
			t.Fatalf("coverage did not observe %q", keys[index])
		}
		if index == 0 {
			if _, ok := coverage.seen[keys[1]]; ok {
				t.Fatalf("coverage observed unexercised %q", keys[1])
			}
		}
	}
}

func loadTestSemanticContractError(t *testing.T, design, source string) error {
	t.Helper()
	directory := t.TempDir()
	paths := SemanticContractPaths{
		ProfileDesign:   filepath.Join(directory, ProfileDesignFilename),
		SourceSemantics: filepath.Join(directory, SourceFilename),
	}
	writeTextFile(t, paths.ProfileDesign, design)
	writeTextFile(t, paths.SourceSemantics, source)
	_, err := LoadSemanticContract(context.Background(), paths)
	return err
}

func identityAlternativesDesignV1() string {
	return strings.Replace(validProfileDesignV1, `    identity:
      required: []
      optional: []`, `    identity:
      required: []
      alternatives:
        - [ceph_daemon]
        - [instance_id]
      optional: []`, 1)
}

func identityAlternativesSourceV1() string {
	source := strings.Replace(validSourceSemanticsV1, "evidence:\n", `evidence:
  daemon_label:
    kind: label
    upstream: exporter
    locations: [metrics.go:8]
    claim: The source identifies the daemon with one role-specific label.
  daemon_identity:
    kind: relationship
    upstream: exporter
    locations: [metrics.go:9]
    claim: Every series carries exactly one complete daemon identity alternative.
`, 1)
	return strings.Replace(source, `    labels: {}
    functional_dependencies: {}`, `    labels:
      ceph_daemon:
        meaning: Non-RGW Ceph daemon identity.
        presence: optional
        domain: {kind: open}
        endpoint_cardinality: {kind: singleton}
        stability: stable
        evidence: [daemon_label]
      instance_id:
        meaning: RGW daemon identity.
        presence: optional
        domain: {kind: open}
        endpoint_cardinality: {kind: singleton}
        stability: stable
        evidence: [daemon_label]
    label_presence_constraints:
      daemon_identity:
        kind: exactly_one
        alternatives:
          - [ceph_daemon]
          - [instance_id]
        evidence: [daemon_identity]
    functional_dependencies: {}`, 1)
}

func sameNestedStrings(left, right [][]string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if !slices.Equal(left[index], right[index]) {
			return false
		}
	}
	return true
}
