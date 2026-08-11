// SPDX-License-Identifier: GPL-3.0-or-later

package promproof

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"path/filepath"
	"strings"

	"github.com/netdata/netdata/go/plugins/internal/promprofile/input"
	"github.com/netdata/netdata/go/plugins/internal/promprofile/replay"
	"github.com/netdata/netdata/go/plugins/internal/promprofile/semantics"
)

// ReplayFunc runs one case and returns exactly one result per supplied
// fixture, in order. Multi-step cases retain production lifecycle state.
type ReplayFunc func(context.Context, prominput.ReplayCase) ([]promreplay.Result, error)

// VerifyCompiledCatalog consumes cases sequentially and discards each full
// replay after reconciliation. Only the compiled catalog and current
// persistent target states survive between calls.
func VerifyCompiledCatalog(
	ctx context.Context,
	repoRoot, testdataRoot string,
	catalog *CompiledCatalog,
	replay ReplayFunc,
) error {
	return VerifyCompiledProfiles(ctx, repoRoot, testdataRoot, catalog, nil, replay)
}

// VerifyCompiledProfiles verifies only the named candidate profiles while
// retaining the complete compiled catalog needed to resolve their supports.
// An empty profile list verifies the whole catalog.
func VerifyCompiledProfiles(
	ctx context.Context,
	repoRoot, testdataRoot string,
	catalog *CompiledCatalog,
	profiles []string,
	replay ReplayFunc,
) error {
	if ctx == nil {
		return fmt.Errorf("verify proof: context is nil")
	}
	if catalog == nil || len(catalog.Bundles) == 0 {
		return fmt.Errorf("verify proof: compiled catalog is empty")
	}
	if replay == nil {
		return fmt.Errorf("verify proof: replay function is nil")
	}
	targets := sortedMapKeys(catalog.Bundles)
	if len(profiles) != 0 {
		requested := make(map[string]struct{}, len(profiles))
		for _, profile := range profiles {
			if catalog.Bundles[profile] == nil {
				return fmt.Errorf("verify proof: requested profile %q is absent from the compiled catalog", profile)
			}
			requested[profile] = struct{}{}
		}
		targets = sortedMapKeys(requested)
	}
	for _, profile := range targets {
		bundle := catalog.Bundles[profile]
		if bundle == nil {
			return fmt.Errorf("verify proof: profile %q has a nil compiled bundle", profile)
		}
		coverage, err := promsemantics.NewProductionCoverage(bundle.Program)
		if err != nil {
			return fmt.Errorf("verify proof profile %q coverage: %w", profile, err)
		}
		for _, caseName := range sortedMapKeys(bundle.Cases) {
			if err := ctx.Err(); err != nil {
				return fmt.Errorf("verify proof before %s/%s: %w", profile, caseName, err)
			}
			compiledCase := bundle.Cases[caseName]
			input, expected, observations, err := replayInput(
				repoRoot, testdataRoot, catalog, bundle, caseName, compiledCase,
			)
			if err != nil {
				return err
			}
			results, err := replay(ctx, input)
			if err != nil {
				return fmt.Errorf("proof %s/%s replay: %w", profile, caseName, err)
			}
			if len(results) != len(expected) {
				return fmt.Errorf("proof %s/%s replay returned %d steps, want %d",
					profile, caseName, len(results), len(expected))
			}
			previous := make(map[string]*promsemantics.ProductionObservationState)
			for stepIndex := range results {
				actual := cloneReplayResult(results[stepIndex])
				if len(actual.Errors) != 0 {
					if err := verifyReplayResult(expected[stepIndex], actual); err != nil {
						return fmt.Errorf("proof %s/%s step %d: %w", profile, caseName, stepIndex, err)
					}
					continue
				}
				snapshot := actual.Semantics
				if snapshot == nil {
					return fmt.Errorf("proof %s/%s step %d: semantic snapshot is nil\nreplay report:\n%s",
						profile, caseName, stepIndex, actual.Details)
				}
				reconciled, err := reconcileProduction(ctx, compiledCase.Semantics, snapshot)
				if err != nil {
					var finding *semanticFinding
					if !errors.As(err, &finding) {
						return fmt.Errorf("proof %s/%s step %d: %w", profile, caseName, stepIndex, err)
					}
					addSemanticFinding(&actual, finding.code, finding.err)
				} else {
					for _, target := range sortedMapKeys(observations[stepIndex]) {
						observation := observations[stepIndex][target]
						state, err := compiledCase.Semantics.ReconcileProductionObservation(
							ctx,
							target,
							promsemantics.ProductionObservationExpectation{
								State:      observation.State,
								Membership: observation.Predicates.Membership,
								Aggregate:  observation.Predicates.Aggregate,
								Identity:   observation.Predicates.Identity,
							},
							snapshot,
							reconciled,
							previous[target],
						)
						if err != nil {
							if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
								return fmt.Errorf("proof %s/%s step %d: %w", profile, caseName, stepIndex, err)
							}
							addSemanticFinding(&actual, "semantic_observation_mismatch", err)
							break
						}
						previous[target] = state
						if *compiledCase.Definition.Coverage && observation.Limitation != "" {
							coverage.ObserveLimitation(
								caseName,
								target,
								observation.Predicates.Membership,
								observation.Predicates.Aggregate,
							)
						}
					}
					if *compiledCase.Definition.Coverage {
						if err := coverage.ObserveCase(ctx, compiledCase.Semantics, snapshot, reconciled); err != nil {
							return fmt.Errorf("proof %s/%s step %d coverage: %w", profile, caseName, stepIndex, err)
						}
					}
				}
				if err := verifyReplayResult(expected[stepIndex], actual); err != nil {
					return fmt.Errorf("proof %s/%s step %d: %w", profile, caseName, stepIndex, err)
				}
			}
		}
		if err := coverage.Verify(ctx); err != nil {
			return fmt.Errorf("proof profile %q: %w", profile, err)
		}
	}
	return nil
}

type semanticFinding struct {
	code string
	err  error
}

func (e *semanticFinding) Error() string { return e.err.Error() }
func (e *semanticFinding) Unwrap() error { return e.err }

func cloneReplayResult(input promreplay.Result) promreplay.Result {
	result := input
	result.Errors = cloneFindingCounts(input.Errors)
	return result
}

func cloneFindingCounts(input map[string]int) map[string]int {
	if input == nil {
		return make(map[string]int)
	}
	result := make(map[string]int, len(input))
	maps.Copy(result, input)
	return result
}

func addSemanticFinding(actual *promreplay.Result, code string, err error) {
	actual.Errors[code]++
	line := code + ": " + err.Error()
	if actual.Details == "" {
		actual.Details = line
	} else {
		actual.Details = strings.TrimRight(actual.Details, "\n") + "\n" + line
	}
}

func replayInput(
	repoRoot, testdataRoot string,
	catalog *CompiledCatalog,
	bundle *CompiledBundle,
	caseName string,
	compiledCase CompiledCase,
) (prominput.ReplayCase, []ExpectedResult, []map[string]ProofObservation, error) {
	definition := compiledCase.Definition
	profile := bundle.Bundle.Descriptor.Profile
	input := prominput.ReplayCase{
		ProfilePath:    filepath.Join(repoRoot, filepath.FromSlash(bundle.Bundle.ProfilePath())),
		DefaultJobName: profile,
		FutureInputs:   effectiveFutureInputs(bundle.Bundle.Descriptor.FutureInputs, definition.FutureInputs),
	}
	for _, active := range compiledCase.Semantics.ActiveProfiles() {
		if active == profile {
			continue
		}
		support := catalog.Bundles[active]
		if support == nil {
			return prominput.ReplayCase{}, nil, nil, fmt.Errorf(
				"proof %s/%s: active support %q has no compiled bundle", profile, caseName, active)
		}
		input.SupportingProfilePaths = append(input.SupportingProfilePaths,
			filepath.Join(repoRoot, filepath.FromSlash(support.Bundle.ProfilePath())))
	}
	metadata := bundle.Bundle.Descriptor.MetadataExample
	if definition.Job.Minimal {
		metadata = nil
	}
	if definition.Job.MetadataExample != nil {
		metadata = definition.Job.MetadataExample
	}
	if metadata != nil {
		copy := *metadata
		input.MetadataExample = &copy
	}

	if definition.Fixture != "" {
		input.FixturePaths = []string{filepath.Join(
			testdataRoot,
			filepath.FromSlash(bundle.Bundle.FixturePath(definition.Fixture)),
		)}
		return input,
			[]ExpectedResult{*definition.Expected},
			[]map[string]ProofObservation{nil},
			nil
	}
	expected := make([]ExpectedResult, 0, len(definition.Steps))
	observations := make([]map[string]ProofObservation, 0, len(definition.Steps))
	for _, step := range definition.Steps {
		input.FixturePaths = append(input.FixturePaths, filepath.Join(
			testdataRoot,
			filepath.FromSlash(bundle.Bundle.FixturePath(step.Fixture)),
		))
		expected = append(expected, step.Expected)
		observations = append(observations, step.Observations)
	}
	return input, expected, observations, nil
}

func effectiveFutureInputs(
	profile, proofCase map[string]prominput.FutureInput,
) map[string]prominput.FutureInput {
	if len(profile) == 0 && len(proofCase) == 0 {
		return nil
	}
	result := make(map[string]prominput.FutureInput, len(profile)+len(proofCase))
	maps.Copy(result, profile)
	maps.Copy(result, proofCase)
	return result
}

func verifyReplayResult(expected ExpectedResult, actual promreplay.Result) error {
	if len(actual.UnsupportedFindingSeverities) != 0 {
		return fmt.Errorf("unsupported finding severities %v", actual.UnsupportedFindingSeverities)
	}
	verdict := "PASS"
	if len(actual.Errors) != 0 {
		verdict = "FAIL"
	}
	if verdict != expected.Verdict {
		return replayResultMismatch(actual, "verdict got %q, want %q", verdict, expected.Verdict)
	}
	if expected.Verdict == "PASS" {
		if len(actual.Errors) != 0 {
			return replayResultMismatch(actual, "PASS emitted errors=%v", actual.Errors)
		}
		return nil
	}
	for _, code := range expected.Findings {
		if actual.Errors[code] == 0 {
			return replayResultMismatch(actual, "required error finding %q is absent; errors=%v", code, actual.Errors)
		}
	}
	return nil
}

func replayResultMismatch(actual promreplay.Result, format string, args ...any) error {
	message := fmt.Sprintf(format, args...)
	if actual.Details != "" {
		message += "\nreport:\n" + actual.Details
	}
	return fmt.Errorf("%s", message)
}

func reconcileProduction(
	ctx context.Context,
	semanticCase *promsemantics.CompiledSemanticCase,
	snapshot *promreplay.SemanticSnapshot,
) (*promsemantics.ReconciledSemanticCase, error) {
	reconciled, err := semanticCase.ReconcileProductionSources(ctx, snapshot)
	if err != nil {
		return nil, semanticReconciliationFinding(ctx, "semantic_source_mismatch", "source reconciliation", err)
	}
	if err := semanticCase.ReconcileProductionNormalizations(ctx, snapshot, reconciled); err != nil {
		return nil, semanticReconciliationFinding(ctx, "semantic_normalization_mismatch", "normalization reconciliation", err)
	}
	if err := semanticCase.ReconcileProductionRoutes(ctx, snapshot, reconciled); err != nil {
		return nil, semanticReconciliationFinding(ctx, "semantic_route_mismatch", "route reconciliation", err)
	}
	if err := semanticCase.ReconcileProductionClaims(ctx, snapshot, reconciled); err != nil {
		return nil, semanticReconciliationFinding(ctx, "semantic_claim_mismatch", "claim reconciliation", err)
	}
	if err := semanticCase.ReconcileProductionChartPolicies(ctx, snapshot, reconciled); err != nil {
		return nil, semanticReconciliationFinding(ctx, "semantic_chart_policy_mismatch", "chart-policy reconciliation", err)
	}
	if err := semanticCase.ReconcileProductionPlan(ctx, snapshot, reconciled); err != nil {
		return nil, semanticReconciliationFinding(ctx, "semantic_plan_mismatch", "plan reconciliation", err)
	}
	return reconciled, nil
}

func semanticReconciliationFinding(ctx context.Context, code, stage string, err error) error {
	wrapped := fmt.Errorf("%s: %w", stage, err)
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || ctx.Err() != nil {
		return wrapped
	}
	return &semanticFinding{code: code, err: wrapped}
}
