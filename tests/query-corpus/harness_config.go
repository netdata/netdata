// SPDX-License-Identifier: GPL-3.0-or-later

package corpus

import (
	"fmt"
	"os"
	"path/filepath"
)

type corpusPaths struct {
	Binary string
	Source string
}

var daemonFreeTests = map[string]struct{}{
	"TestC019FormatShapeGuards":                                   {},
	"TestC016PhaseBudgetGuards":                                   {},
	"TestC023ResolutionWindowRecordsUseFixtureCadence":            {},
	"TestC023TierNumberTimesPartialGapOverlap":                    {},
	"TestC4CFlapFixtureGuard":                                     {},
	"TestC4DAlignedGridOracleGuardsOffByOne":                      {},
	"TestCase025OverlapOracle":                                    {},
	"TestContractLedgerDeduplicatesAndKeepsFailuresSticky":        {},
	"TestContractLedgerDoesNotCountSkippedScopeAsEvaluated":       {},
	"TestContractLedgerKeepsFailureBeforeSkip":                    {},
	"TestContractLedgerRegistrationRemainsIncompleteUntilVerdict": {},
	"TestContractLedgerRequiresEveryComponent":                    {},
	"TestDaemonRunRequiredFailsClosed":                            {},
	"TestDecodeV1ContextsWeightsRejectsMalformedDimensions":       {},
	"TestDirectBraceEntriesRejectsUnclosedEntry":                  {},
	"TestGroupingRosterParserHandlesMultilineDeclarations":        {},
	"TestGroupingRosterParserRejectsDrift":                        {},
	"TestIncompleteContractSummaryNeverClaimsAllHold":             {},
	"TestInfrastructureAttribution":                               {},
	"TestL10QueryResultGuards":                                    {},
	"TestL11SlicingOracleGuards":                                  {},
	"TestL5ExactGridGuard":                                        {},
	"TestL6ContributorWeightedMetadataOracle":                     {},
	"TestL6RawCountSchemaGuard":                                   {},
	"TestL7StructuredResponseGuards":                              {},
	"TestL9DefaultWindowShapeGuards":                              {},
	"TestManifestComponentsAreValid":                              {},
	"TestManifestDocumentGuardDetectsDrift":                       {},
	"TestManifestDocumentMatchesContracts":                        {},
	"TestManifestLoaderPreservesFields":                           {},
	"TestManifestLoaderRejectsInvalidData":                        {},
	"TestOptionsAllDimensionsMetadataGuards":                      {},
	"TestQueryAssertionGuardsDetectMutations":                     {},
	"TestQueryStructuredResponseGuards":                           {},
	"TestResolveCorpusPathsRejectsUnusablePair":                   {},
	"TestResolveCorpusPathsRequiresPairedOverrides":               {},
	"TestResolveCorpusPathsValidatesOneDeclaredEngine":            {},
	"TestStrictDimensionStatsGuards":                              {},
	"TestWeightsExpectedIDsExactlyOnce":                           {},
	"TestWeightsTimeframeStatsRequireFiniteNumbers":               {},
	"TestWeightsValueMultiNodeGuards":                             {},
}

// daemonRunRequired recognizes only deliberately named, simple pure-test
// selections. Unknown or compound regular expressions fail closed and boot
// the daemon, so adding a runtime test can never accidentally take the fast
// path.
func daemonRunRequired(runPattern, listPattern string) bool {
	if listPattern != "" || runPattern == "^$" || runPattern == "$^" {
		return false
	}
	if runPattern == "" {
		return true
	}

	if len(runPattern) < 3 || runPattern[0] != '^' ||
		runPattern[len(runPattern)-1] != '$' {
		return true
	}

	_, pure := daemonFreeTests[runPattern[1:len(runPattern)-1]]
	return !pure
}

// resolveCorpusPaths binds an alternate binary to the source tree from which
// the grouping roster is read. The pair is operator-declared provenance: it
// prevents accidental mixed-worktree runs, but does not inspect build metadata.
func resolveCorpusPaths(binaryOverride, sourceOverride, workingDir string) (corpusPaths, error) {
	if (binaryOverride == "") != (sourceOverride == "") {
		return corpusPaths{}, fmt.Errorf(
			"QUERY_CORPUS_NETDATA and QUERY_CORPUS_SRC must be set together")
	}

	binary, source := binaryOverride, sourceOverride
	if binary == "" {
		binary = "../../build/netdata"
		source = "../../src"
	}
	absolute := func(path string) (string, error) {
		if !filepath.IsAbs(path) {
			path = filepath.Join(workingDir, path)
		}
		return filepath.Abs(path)
	}

	binary, err := absolute(binary)
	if err != nil {
		return corpusPaths{}, fmt.Errorf("resolve netdata binary: %w", err)
	}
	source, err = absolute(source)
	if err != nil {
		return corpusPaths{}, fmt.Errorf("resolve engine source: %w", err)
	}

	info, err := os.Stat(binary)
	if err != nil {
		return corpusPaths{}, fmt.Errorf("netdata binary not usable at %s: %w", binary, err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
		return corpusPaths{}, fmt.Errorf("netdata binary is not an executable regular file: %s", binary)
	}
	for _, relative := range []string{
		"web/api/queries/query.h",
		"web/api/queries/query-group-over-time.c",
	} {
		path := filepath.Join(source, relative)
		if info, err := os.Stat(path); err != nil || !info.Mode().IsRegular() {
			if err == nil {
				err = fmt.Errorf("not a regular file")
			}
			return corpusPaths{}, fmt.Errorf("engine source not usable at %s: %w", path, err)
		}
	}

	return corpusPaths{Binary: binary, Source: source}, nil
}
