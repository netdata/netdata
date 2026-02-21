// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/csv"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/topology/engine"
	"github.com/netdata/netdata/go/plugins/pkg/topology/engine/parity"
)

const (
	defaultEnlinkdRoot        = "/tmp/topology-library-repos/enlinkd"
	defaultFixtureSourceRel   = "features/enlinkd/tests/src/test/resources/linkd"
	defaultFixtureMirrorRel   = "pkg/topology/engine/testdata/enlinkd/upstream/linkd"
	defaultManifestRootRel    = "pkg/topology/engine/testdata/enlinkd"
	defaultEvidenceRel        = "pkg/topology/engine/parity/evidence"
	defaultScopedTestsRelEn   = "features/enlinkd/tests/src/test/java/org/opennms/netmgt/enlinkd"
	defaultScopedTestsRelNB   = "features/enlinkd/tests/src/test/java/org/opennms/netmgt/nb"
	defaultFixtureInventory   = "enlinkd-fixture-inventory.csv"
	defaultMethodInventory    = "enlinkd-test-method-inventory.csv"
	defaultAssertionInventory = "enlinkd-assertion-inventory.csv"
	defaultAssertionMapping   = "assertion-mapping.csv"
	defaultSummaryFile        = "parity-summary.json"
	defaultPhase2ReportFile   = "phase2-parity-report.json"
	defaultPhase2GapFile      = "phase2-gap-report.md"
	defaultOracleDiffJSONFile = "behavior-oracle-diff.json"
	defaultOracleDiffMDFile   = "behavior-oracle-diff.md"
)

var (
	testAnnotationRE  = regexp.MustCompile(`^\s*@Test\b`)
	packageRE         = regexp.MustCompile(`^\s*package\s+([A-Za-z0-9_.]+)\s*;`)
	classRE           = regexp.MustCompile(`\bclass\s+([A-Za-z_][A-Za-z0-9_]*)\b`)
	identifierRE      = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	assertionCallRE   = regexp.MustCompile(`\b(assert[A-Za-z0-9_]*)\s*\(`)
	testFileNameITRE  = regexp.MustCompile(`IT\.java$`)
	testFileNameTestR = regexp.MustCompile(`Test\.java$`)
)

type options struct {
	mode           string
	enlinkdRoot    string
	fixtureSrcRel  string
	fixtureDstPath string
	manifestRoot   string
	evidencePath   string
	summaryPath    string
	phase2Report   string
	phase2Gap      string
	oracleDiffJSON string
	oracleDiffMD   string
}

type fixtureRow struct {
	Scenario     string
	File         string
	RelativePath string
	SHA256       string
	SizeBytes    int64
	UpstreamPath string
}

type methodRow struct {
	Class         string
	Method        string
	SourceFile    string
	ProtocolScope string
}

type assertionRow struct {
	Class         string
	Method        string
	AssertionID   string
	SourceFile    string
	Line          int
	AssertCall    string
	ProtocolScope string
}

type methodRange struct {
	Name  string
	Start int
	End   int
	Scope string
}

type assertionCandidate struct {
	Line int
	Call string
}

type mappingStats struct {
	MappedAssertions int
	TotalAssertions  int
	MappedMethods    int
	TotalMethods     int
	MappedTestFiles  int
	TotalTestFiles   int
}

type protocolSummary struct {
	Protocol string `json:"protocol"`
	Total    int    `json:"total"`
	Passed   int    `json:"passed"`
	Failed   int    `json:"failed"`
}

type scenarioSummary struct {
	ID        string   `json:"id"`
	Manifest  string   `json:"manifest"`
	Protocols []string `json:"protocols"`
	Passed    bool     `json:"passed"`
	Failures  []string `json:"failures,omitempty"`
}

type goTestSummary struct {
	Package string `json:"package"`
	Passed  bool   `json:"passed"`
	Error   string `json:"error,omitempty"`
}

type paritySummary struct {
	Version               string            `json:"version"`
	FixtureScenarios      int               `json:"fixture_scenarios"`
	FixtureFiles          int               `json:"fixture_files"`
	TotalScenarios        int               `json:"total_scenarios"`
	ScenariosPassed       int               `json:"scenarios_passed"`
	ScenariosFailed       int               `json:"scenarios_failed"`
	TotalTestsMapped      int               `json:"total_tests_mapped"`
	TotalTestsInventory   int               `json:"total_tests_inventory"`
	TotalAssertionsMapped int               `json:"total_assertions_mapped"`
	TotalAssertionsTotal  int               `json:"total_assertions_inventory"`
	ProtocolCounts        []protocolSummary `json:"protocol_counts"`
	ScenarioResults       []scenarioSummary `json:"scenario_results"`
	GoTests               []goTestSummary   `json:"go_tests"`
	Determinism           struct {
		Runs          int  `json:"runs"`
		ByteIdentical bool `json:"byte_identical"`
	} `json:"determinism"`
}

type phase2SuiteSummary struct {
	FixtureScenarios      int               `json:"fixture_scenarios"`
	FixtureFiles          int               `json:"fixture_files"`
	TotalScenarios        int               `json:"total_scenarios"`
	ScenariosPassed       int               `json:"scenarios_passed"`
	ScenariosFailed       int               `json:"scenarios_failed"`
	TotalTestsMapped      int               `json:"total_tests_mapped"`
	TotalTestsInventory   int               `json:"total_tests_inventory"`
	TotalAssertionsMapped int               `json:"total_assertions_mapped"`
	TotalAssertionsTotal  int               `json:"total_assertions_inventory"`
	ProtocolCounts        []protocolSummary `json:"protocol_counts"`
}

type phase2CheckStatus struct {
	Name         string   `json:"name"`
	Status       string   `json:"status"`
	ChecksPassed int      `json:"checks_passed"`
	ChecksTotal  int      `json:"checks_total"`
	Commands     []string `json:"commands"`
	Failed       []string `json:"failed,omitempty"`
	Missing      []string `json:"missing,omitempty"`
	Errors       []string `json:"errors,omitempty"`
}

type phase2AssertionCoverage struct {
	Status                  string `json:"status"`
	InScopeTotal            int    `json:"in_scope_total"`
	InScopePorted           int    `json:"in_scope_ported"`
	InScopeNotApplicable    int    `json:"in_scope_not_applicable_approved"`
	InScopeUnmapped         int    `json:"in_scope_unmapped"`
	OutOfScopePorted        int    `json:"out_of_scope_ported"`
	OutOfScopeNotApplicable int    `json:"out_of_scope_not_applicable_approved"`
}

type phase2DeferredGap struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	Reason      string `json:"reason"`
	Evidence    string `json:"evidence"`
}

type phase2Report struct {
	Version              string                  `json:"version"`
	GeneratedAtUTC       string                  `json:"generated_at_utc"`
	Status               string                  `json:"status"`
	Suite                phase2SuiteSummary      `json:"suite"`
	ModuleParity         []phase2CheckStatus     `json:"module_parity"`
	ReversePairQuality   phase2CheckStatus       `json:"reverse_pair_quality"`
	IdentityMergeQuality phase2CheckStatus       `json:"identity_merge_quality"`
	AssertionCoverage    phase2AssertionCoverage `json:"assertion_coverage"`
	DeferredGaps         []phase2DeferredGap     `json:"deferred_gaps"`
}

type behaviorOracleReport struct {
	Version        string                         `json:"version"`
	GeneratedAtUTC string                         `json:"generated_at_utc"`
	Status         string                         `json:"status"`
	Scope          behaviorOracleScope            `json:"scope"`
	Totals         behaviorOracleTotals           `json:"totals"`
	Scenarios      []behaviorOracleScenarioReport `json:"scenarios"`
}

type behaviorOracleScope struct {
	Protocols []string `json:"protocols"`
}

type behaviorOracleTotals struct {
	ScenariosTotal        int `json:"scenarios_total"`
	ScenariosInScope      int `json:"scenarios_in_scope"`
	ScenariosSkipped      int `json:"scenarios_skipped"`
	ScenariosZeroDiff     int `json:"scenarios_zero_diff"`
	ScenariosWithDiffs    int `json:"scenarios_with_diffs"`
	ScenariosWithFailures int `json:"scenarios_with_failures"`
}

type behaviorOracleScenarioReport struct {
	ID            string                  `json:"id"`
	Manifest      string                  `json:"manifest"`
	Protocols     []string                `json:"protocols"`
	InScope       bool                    `json:"in_scope"`
	FixtureInputs []behaviorOracleFixture `json:"fixture_inputs,omitempty"`
	Expected      behaviorOracleSnapshot  `json:"expected"`
	Actual        behaviorOracleSnapshot  `json:"actual"`
	Diff          behaviorOracleDiff      `json:"diff"`
	Status        string                  `json:"status"`
	Errors        []string                `json:"errors,omitempty"`
}

type behaviorOracleFixture struct {
	DeviceID  string `json:"device_id"`
	Hostname  string `json:"hostname,omitempty"`
	Address   string `json:"address,omitempty"`
	WalkFile  string `json:"walk_file"`
	SHA256    string `json:"sha256"`
	SizeBytes int64  `json:"size_bytes"`
}

type behaviorOracleSnapshot struct {
	Devices     []parity.GoldenDevice    `json:"devices"`
	Adjacencies []parity.GoldenAdjacency `json:"adjacencies"`
	Metadata    behaviorOracleMetadata   `json:"metadata"`
}

type behaviorOracleMetadata struct {
	Devices                int `json:"devices"`
	DirectionalAdjacencies int `json:"directional_adjacencies"`
}

type behaviorOracleDiff struct {
	ZeroDiff              bool                        `json:"zero_diff"`
	MissingDevices        []parity.GoldenDevice       `json:"missing_devices,omitempty"`
	UnexpectedDevices     []parity.GoldenDevice       `json:"unexpected_devices,omitempty"`
	HostnameMismatches    []behaviorOracleDeviceDelta `json:"hostname_mismatches,omitempty"`
	MissingAdjacencies    []parity.GoldenAdjacency    `json:"missing_adjacencies,omitempty"`
	UnexpectedAdjacencies []parity.GoldenAdjacency    `json:"unexpected_adjacencies,omitempty"`
	MetadataMismatches    []behaviorOracleCountDelta  `json:"metadata_mismatches,omitempty"`
}

type behaviorOracleDeviceDelta struct {
	DeviceID string `json:"device_id"`
	Expected string `json:"expected"`
	Actual   string `json:"actual"`
}

type behaviorOracleCountDelta struct {
	Field    string `json:"field"`
	Expected int    `json:"expected"`
	Actual   int    `json:"actual"`
}

func main() {
	opts := parseOptions()

	switch opts.mode {
	case "sync":
		if err := runSync(opts); err != nil {
			fmt.Fprintf(os.Stderr, "topology-parity-evidence sync failed: %v\n", err)
			os.Exit(1)
		}
	case "verify":
		if err := runVerify(opts); err != nil {
			fmt.Fprintf(os.Stderr, "topology-parity-evidence verify failed: %v\n", err)
			os.Exit(1)
		}
	case "suite":
		if err := runSuite(opts); err != nil {
			fmt.Fprintf(os.Stderr, "topology-parity-evidence suite failed: %v\n", err)
			os.Exit(1)
		}
	case "phase2":
		if err := runPhase2(opts); err != nil {
			fmt.Fprintf(os.Stderr, "topology-parity-evidence phase2 failed: %v\n", err)
			os.Exit(1)
		}
	case "oracle-diff":
		if err := runOracleDiff(opts); err != nil {
			fmt.Fprintf(os.Stderr, "topology-parity-evidence oracle-diff failed: %v\n", err)
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "unsupported mode %q (want sync|verify|suite|phase2|oracle-diff)\n", opts.mode)
		os.Exit(1)
	}
}

func parseOptions() options {
	var opts options
	flag.StringVar(&opts.mode, "mode", "sync", "sync|verify|suite|phase2|oracle-diff")
	flag.StringVar(&opts.enlinkdRoot, "enlinkd-root", defaultEnlinkdRoot, "path to enlinkd checkout root")
	flag.StringVar(&opts.fixtureSrcRel, "fixture-source-rel", defaultFixtureSourceRel, "fixture source path relative to enlinkd root")
	flag.StringVar(&opts.fixtureDstPath, "fixture-dst", defaultFixtureMirrorRel, "fixture mirror destination path")
	flag.StringVar(&opts.manifestRoot, "manifest-root", defaultManifestRootRel, "local manifest root path")
	flag.StringVar(&opts.evidencePath, "evidence-dir", defaultEvidenceRel, "evidence output directory")
	flag.StringVar(&opts.summaryPath, "summary-file", filepath.Join(defaultEvidenceRel, defaultSummaryFile), "parity summary output file")
	flag.StringVar(&opts.phase2Report, "phase2-report-file", filepath.Join(defaultEvidenceRel, defaultPhase2ReportFile), "phase2 parity report output file")
	flag.StringVar(&opts.phase2Gap, "phase2-gap-file", filepath.Join(defaultEvidenceRel, defaultPhase2GapFile), "phase2 gap report output file")
	flag.StringVar(&opts.oracleDiffJSON, "oracle-diff-json", filepath.Join(defaultEvidenceRel, defaultOracleDiffJSONFile), "behavior oracle diff report (machine-readable JSON)")
	flag.StringVar(&opts.oracleDiffMD, "oracle-diff-md", filepath.Join(defaultEvidenceRel, defaultOracleDiffMDFile), "behavior oracle diff report (human-readable Markdown)")
	flag.Parse()
	return opts
}

func runSync(opts options) error {
	srcRoot := filepath.Join(opts.enlinkdRoot, opts.fixtureSrcRel)
	if err := requireDir(srcRoot); err != nil {
		return fmt.Errorf("fixture source root: %w", err)
	}

	if err := syncFixtureMirror(srcRoot, opts.fixtureDstPath); err != nil {
		return fmt.Errorf("sync fixture mirror: %w", err)
	}

	fixtureRows, err := collectFixtureInventory(opts.fixtureDstPath, opts.fixtureSrcRel)
	if err != nil {
		return fmt.Errorf("collect fixture inventory: %w", err)
	}

	testFiles, err := listScopedTestFiles(opts.enlinkdRoot)
	if err != nil {
		return fmt.Errorf("list scoped tests: %w", err)
	}
	assertionFiles, err := listScopedJavaFiles(opts.enlinkdRoot)
	if err != nil {
		return fmt.Errorf("list scoped java files: %w", err)
	}
	methodRows, assertionRows, err := collectTestAndAssertionInventories(opts.enlinkdRoot, testFiles, assertionFiles)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(opts.evidencePath, 0o755); err != nil {
		return fmt.Errorf("mkdir evidence dir: %w", err)
	}
	if err := writeFixtureInventoryCSV(filepath.Join(opts.evidencePath, defaultFixtureInventory), fixtureRows); err != nil {
		return err
	}
	if err := writeMethodInventoryCSV(filepath.Join(opts.evidencePath, defaultMethodInventory), methodRows); err != nil {
		return err
	}
	if err := writeAssertionInventoryCSV(filepath.Join(opts.evidencePath, defaultAssertionInventory), assertionRows); err != nil {
		return err
	}

	fmt.Printf("sync complete\n")
	fmt.Printf("fixture scenarios: %d\n", countDistinctScenarios(fixtureRows))
	fmt.Printf("fixture files: %d\n", len(fixtureRows))
	fmt.Printf("test files: %d\n", countDistinctFiles(methodRows))
	fmt.Printf("test methods: %d\n", len(methodRows))
	fmt.Printf("assertions: %d\n", len(assertionRows))
	return nil
}

func runVerify(opts options) error {
	localRows, err := verifyFixtureInventory(opts)
	if err != nil {
		return err
	}

	fmt.Printf("verify complete\n")
	fmt.Printf("fixture scenarios: %d\n", countDistinctScenarios(localRows))
	fmt.Printf("fixture files: %d\n", len(localRows))
	return nil
}

func verifyFixtureInventory(opts options) ([]fixtureRow, error) {
	srcRoot := filepath.Join(opts.enlinkdRoot, opts.fixtureSrcRel)
	if err := requireDir(srcRoot); err != nil {
		return nil, fmt.Errorf("fixture source root: %w", err)
	}
	if err := requireDir(opts.fixtureDstPath); err != nil {
		return nil, fmt.Errorf("fixture destination root: %w", err)
	}

	upstreamRows, err := collectFixtureInventory(srcRoot, opts.fixtureSrcRel)
	if err != nil {
		return nil, fmt.Errorf("collect upstream inventory: %w", err)
	}
	localRows, err := collectFixtureInventory(opts.fixtureDstPath, opts.fixtureSrcRel)
	if err != nil {
		return nil, fmt.Errorf("collect local inventory: %w", err)
	}
	if err := compareFixtureInventories(upstreamRows, localRows); err != nil {
		return nil, err
	}

	inventoryPath := filepath.Join(opts.evidencePath, defaultFixtureInventory)
	fileRows, err := readFixtureInventoryCSV(inventoryPath)
	if err != nil {
		return nil, err
	}
	if err := compareFixtureInventories(localRows, fileRows); err != nil {
		return nil, fmt.Errorf("inventory file mismatch (%s): %w", inventoryPath, err)
	}

	return localRows, nil
}

func runSuite(opts options) error {
	summaryA, err := buildSuiteSummary(opts)
	if err != nil {
		return err
	}
	summaryB, err := buildSuiteSummary(opts)
	if err != nil {
		return err
	}

	baseA, err := marshalSummaryJSON(summaryA)
	if err != nil {
		return err
	}
	baseB, err := marshalSummaryJSON(summaryB)
	if err != nil {
		return err
	}
	byteIdentical := bytes.Equal(baseA, baseB)

	summaryA.Determinism.Runs = 2
	summaryA.Determinism.ByteIdentical = byteIdentical

	outBytes, err := marshalSummaryJSON(summaryA)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(opts.summaryPath), 0o755); err != nil {
		return fmt.Errorf("create summary directory: %w", err)
	}
	if err := os.WriteFile(opts.summaryPath, outBytes, 0o644); err != nil {
		return fmt.Errorf("write summary %q: %w", opts.summaryPath, err)
	}

	fmt.Printf("suite complete\n")
	fmt.Printf("summary file: %s\n", opts.summaryPath)
	fmt.Printf("total scenarios: %d (passed=%d failed=%d)\n", summaryA.TotalScenarios, summaryA.ScenariosPassed, summaryA.ScenariosFailed)
	fmt.Printf("mapped tests/assertions: %d/%d tests, %d/%d assertions\n",
		summaryA.TotalTestsMapped, summaryA.TotalTestsInventory,
		summaryA.TotalAssertionsMapped, summaryA.TotalAssertionsTotal)

	if !byteIdentical {
		return fmt.Errorf("determinism check failed: canonical summary JSON differs across repeated runs")
	}
	for _, testResult := range summaryA.GoTests {
		if !testResult.Passed {
			return fmt.Errorf("go test failed for %s: %s", testResult.Package, testResult.Error)
		}
	}
	if summaryA.ScenariosFailed > 0 {
		return fmt.Errorf("%d scenario(s) failed parity validation", summaryA.ScenariosFailed)
	}
	return nil
}

func runOracleDiff(opts options) error {
	if _, err := verifyFixtureInventory(opts); err != nil {
		return err
	}

	report, err := buildBehaviorOracleDiffReport(opts.manifestRoot)
	if err != nil {
		return err
	}

	payload, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal behavior oracle report: %w", err)
	}
	payload = append(payload, '\n')

	if err := os.MkdirAll(filepath.Dir(opts.oracleDiffJSON), 0o755); err != nil {
		return fmt.Errorf("create oracle diff json directory: %w", err)
	}
	if err := os.WriteFile(opts.oracleDiffJSON, payload, 0o644); err != nil {
		return fmt.Errorf("write oracle diff json %q: %w", opts.oracleDiffJSON, err)
	}

	markdown := buildBehaviorOracleDiffMarkdown(report)
	if err := os.MkdirAll(filepath.Dir(opts.oracleDiffMD), 0o755); err != nil {
		return fmt.Errorf("create oracle diff markdown directory: %w", err)
	}
	if err := os.WriteFile(opts.oracleDiffMD, []byte(markdown), 0o644); err != nil {
		return fmt.Errorf("write oracle diff markdown %q: %w", opts.oracleDiffMD, err)
	}

	fmt.Printf("oracle diff complete\n")
	fmt.Printf("json report: %s\n", opts.oracleDiffJSON)
	fmt.Printf("markdown report: %s\n", opts.oracleDiffMD)
	fmt.Printf("status: %s\n", report.Status)
	fmt.Printf("in-scope scenarios: %d (zero-diff=%d diffs=%d failures=%d)\n",
		report.Totals.ScenariosInScope,
		report.Totals.ScenariosZeroDiff,
		report.Totals.ScenariosWithDiffs,
		report.Totals.ScenariosWithFailures)

	if report.Status != "pass" {
		return fmt.Errorf("behavior oracle diff contains in-scope mismatches")
	}
	return nil
}

func buildBehaviorOracleDiffReport(manifestRoot string) (behaviorOracleReport, error) {
	pattern := filepath.Join(manifestRoot, "*/manifest.yaml")
	manifestPaths, err := filepath.Glob(pattern)
	if err != nil {
		return behaviorOracleReport{}, fmt.Errorf("glob manifests %q: %w", pattern, err)
	}
	sort.Strings(manifestPaths)
	if len(manifestPaths) == 0 {
		return behaviorOracleReport{}, fmt.Errorf("no manifests found under %q", manifestRoot)
	}

	report := behaviorOracleReport{
		Version:        "v1",
		GeneratedAtUTC: time.Now().UTC().Format(time.RFC3339),
		Status:         "pass",
		Scope: behaviorOracleScope{
			Protocols: []string{"lldp", "cdp", "bridge_fdb", "arp_nd"},
		},
		Scenarios: make([]behaviorOracleScenarioReport, 0, 64),
	}

	for _, manifestPath := range manifestPaths {
		manifest, err := parity.LoadManifest(manifestPath)
		if err != nil {
			return behaviorOracleReport{}, err
		}

		scenarios := append([]parity.ManifestScenario(nil), manifest.Scenarios...)
		sort.Slice(scenarios, func(i, j int) bool {
			return scenarios[i].ID < scenarios[j].ID
		})

		for _, scenario := range scenarios {
			scenarioReport := evaluateBehaviorOracleScenario(manifestPath, scenario)
			report.Scenarios = append(report.Scenarios, scenarioReport)
		}
	}

	report.Totals.ScenariosTotal = len(report.Scenarios)
	for _, scenario := range report.Scenarios {
		if !scenario.InScope {
			report.Totals.ScenariosSkipped++
			continue
		}

		report.Totals.ScenariosInScope++
		switch scenario.Status {
		case "zero-diff":
			report.Totals.ScenariosZeroDiff++
		case "diff":
			report.Totals.ScenariosWithDiffs++
		default:
			report.Totals.ScenariosWithFailures++
		}
	}

	if report.Totals.ScenariosWithDiffs > 0 || report.Totals.ScenariosWithFailures > 0 {
		report.Status = "fail"
	}

	return report, nil
}

func evaluateBehaviorOracleScenario(manifestPath string, scenario parity.ManifestScenario) behaviorOracleScenarioReport {
	out := behaviorOracleScenarioReport{
		ID:        scenario.ID,
		Manifest:  filepath.ToSlash(manifestPath),
		Protocols: enabledProtocols(scenario.Protocols),
		InScope:   scenarioInScope(scenario),
		Status:    "error",
		Expected: behaviorOracleSnapshot{
			Devices:     []parity.GoldenDevice{},
			Adjacencies: []parity.GoldenAdjacency{},
		},
		Actual: behaviorOracleSnapshot{
			Devices:     []parity.GoldenDevice{},
			Adjacencies: []parity.GoldenAdjacency{},
		},
	}

	resolved, err := parity.ResolveScenario(manifestPath, scenario)
	if err != nil {
		out.Errors = []string{err.Error()}
		return out
	}

	fixtures, err := collectBehaviorOracleFixtureInputs(resolved)
	if err != nil {
		out.Errors = []string{err.Error()}
		return out
	}
	out.FixtureInputs = fixtures

	if err := parity.ValidateCache(resolved.GoldenYAML, resolved.GoldenJSON); err != nil {
		out.Errors = append(out.Errors, err.Error())
	}

	golden, err := parity.LoadGoldenYAML(resolved.GoldenYAML)
	if err != nil {
		out.Errors = append(out.Errors, err.Error())
		return out
	}
	out.Expected = expectedBehaviorSnapshot(golden)

	walks, err := parity.LoadScenarioWalks(resolved)
	if err != nil {
		out.Errors = append(out.Errors, err.Error())
		return out
	}

	result, err := parity.BuildL2ResultFromWalks(walks, parity.BuildOptions{
		EnableLLDP:   scenario.Protocols.LLDP,
		EnableCDP:    scenario.Protocols.CDP,
		EnableBridge: scenario.Protocols.Bridge,
		EnableARP:    scenario.Protocols.ARPND,
	})
	if err != nil {
		out.Errors = append(out.Errors, err.Error())
		return out
	}
	out.Actual = actualBehaviorSnapshot(result)
	out.Diff = diffBehaviorSnapshots(out.Expected, out.Actual)

	if !out.InScope {
		out.Status = "skipped"
		return out
	}
	if len(out.Errors) > 0 {
		out.Status = "error"
		return out
	}
	if out.Diff.ZeroDiff {
		out.Status = "zero-diff"
	} else {
		out.Status = "diff"
	}
	return out
}

func collectBehaviorOracleFixtureInputs(scenario parity.ResolvedScenario) ([]behaviorOracleFixture, error) {
	out := make([]behaviorOracleFixture, 0, len(scenario.Fixtures))
	for _, fixture := range scenario.Fixtures {
		info, err := os.Stat(fixture.WalkFile)
		if err != nil {
			return nil, fmt.Errorf("stat walk file %q: %w", fixture.WalkFile, err)
		}
		sha256Value, err := sha256File(fixture.WalkFile)
		if err != nil {
			return nil, fmt.Errorf("sha256 walk file %q: %w", fixture.WalkFile, err)
		}

		out = append(out, behaviorOracleFixture{
			DeviceID:  fixture.DeviceID,
			Hostname:  fixture.Hostname,
			Address:   fixture.Address,
			WalkFile:  filepath.ToSlash(fixture.WalkFile),
			SHA256:    sha256Value,
			SizeBytes: info.Size(),
		})
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].DeviceID != out[j].DeviceID {
			return out[i].DeviceID < out[j].DeviceID
		}
		return out[i].WalkFile < out[j].WalkFile
	})
	return out, nil
}

func expectedBehaviorSnapshot(golden parity.GoldenDocument) behaviorOracleSnapshot {
	canonical := golden.Canonical()
	devices := append([]parity.GoldenDevice(nil), canonical.Devices...)
	adjacencies := append([]parity.GoldenAdjacency(nil), canonical.Adjacencies...)
	return behaviorOracleSnapshot{
		Devices:     devices,
		Adjacencies: adjacencies,
		Metadata: behaviorOracleMetadata{
			Devices:                canonical.Expectations.Devices,
			DirectionalAdjacencies: canonical.Expectations.DirectionalAdjacencies,
		},
	}
}

func actualBehaviorSnapshot(result engine.Result) behaviorOracleSnapshot {
	devices := make([]parity.GoldenDevice, 0, len(result.Devices))
	for _, dev := range result.Devices {
		devices = append(devices, parity.GoldenDevice{
			ID:       dev.ID,
			Hostname: dev.Hostname,
		})
	}
	sort.Slice(devices, func(i, j int) bool {
		if devices[i].ID != devices[j].ID {
			return devices[i].ID < devices[j].ID
		}
		return devices[i].Hostname < devices[j].Hostname
	})

	adjacencies := make([]parity.GoldenAdjacency, 0, len(result.Adjacencies))
	for _, adj := range result.Adjacencies {
		adjacencies = append(adjacencies, parity.GoldenAdjacency{
			Protocol:     adj.Protocol,
			SourceDevice: adj.SourceID,
			SourcePort:   adj.SourcePort,
			TargetDevice: adj.TargetID,
			TargetPort:   adj.TargetPort,
		})
	}
	sort.Slice(adjacencies, func(i, j int) bool {
		ai := adjacencies[i]
		aj := adjacencies[j]
		if ai.Protocol != aj.Protocol {
			return ai.Protocol < aj.Protocol
		}
		if ai.SourceDevice != aj.SourceDevice {
			return ai.SourceDevice < aj.SourceDevice
		}
		if ai.SourcePort != aj.SourcePort {
			return ai.SourcePort < aj.SourcePort
		}
		if ai.TargetDevice != aj.TargetDevice {
			return ai.TargetDevice < aj.TargetDevice
		}
		return ai.TargetPort < aj.TargetPort
	})

	return behaviorOracleSnapshot{
		Devices:     devices,
		Adjacencies: adjacencies,
		Metadata: behaviorOracleMetadata{
			Devices:                len(devices),
			DirectionalAdjacencies: len(adjacencies),
		},
	}
}

func diffBehaviorSnapshots(expected, actual behaviorOracleSnapshot) behaviorOracleDiff {
	diff := behaviorOracleDiff{
		ZeroDiff: true,
	}

	expectedByID := make(map[string]parity.GoldenDevice, len(expected.Devices))
	for _, dev := range expected.Devices {
		expectedByID[dev.ID] = dev
	}
	actualByID := make(map[string]parity.GoldenDevice, len(actual.Devices))
	for _, dev := range actual.Devices {
		actualByID[dev.ID] = dev
	}

	for _, dev := range expected.Devices {
		actualDev, ok := actualByID[dev.ID]
		if !ok {
			diff.MissingDevices = append(diff.MissingDevices, dev)
			continue
		}
		if dev.Hostname != actualDev.Hostname {
			diff.HostnameMismatches = append(diff.HostnameMismatches, behaviorOracleDeviceDelta{
				DeviceID: dev.ID,
				Expected: dev.Hostname,
				Actual:   actualDev.Hostname,
			})
		}
	}
	for _, dev := range actual.Devices {
		if _, ok := expectedByID[dev.ID]; !ok {
			diff.UnexpectedDevices = append(diff.UnexpectedDevices, dev)
		}
	}

	expectedAdjByKey := make(map[string]parity.GoldenAdjacency, len(expected.Adjacencies))
	for _, adj := range expected.Adjacencies {
		expectedAdjByKey[goldenAdjacencyKey(adj)] = adj
	}
	actualAdjByKey := make(map[string]parity.GoldenAdjacency, len(actual.Adjacencies))
	for _, adj := range actual.Adjacencies {
		actualAdjByKey[goldenAdjacencyKey(adj)] = adj
	}

	for _, adj := range expected.Adjacencies {
		if _, ok := actualAdjByKey[goldenAdjacencyKey(adj)]; !ok {
			diff.MissingAdjacencies = append(diff.MissingAdjacencies, adj)
		}
	}
	for _, adj := range actual.Adjacencies {
		if _, ok := expectedAdjByKey[goldenAdjacencyKey(adj)]; !ok {
			diff.UnexpectedAdjacencies = append(diff.UnexpectedAdjacencies, adj)
		}
	}

	diff.MetadataMismatches = append(diff.MetadataMismatches,
		buildCountDelta("devices", expected.Metadata.Devices, actual.Metadata.Devices),
		buildCountDelta("directional_adjacencies", expected.Metadata.DirectionalAdjacencies, actual.Metadata.DirectionalAdjacencies),
	)
	filteredCountDeltas := make([]behaviorOracleCountDelta, 0, len(diff.MetadataMismatches))
	for _, delta := range diff.MetadataMismatches {
		if delta.Field == "" {
			continue
		}
		filteredCountDeltas = append(filteredCountDeltas, delta)
	}
	diff.MetadataMismatches = filteredCountDeltas

	sort.Slice(diff.MissingDevices, func(i, j int) bool { return diff.MissingDevices[i].ID < diff.MissingDevices[j].ID })
	sort.Slice(diff.UnexpectedDevices, func(i, j int) bool { return diff.UnexpectedDevices[i].ID < diff.UnexpectedDevices[j].ID })
	sort.Slice(diff.HostnameMismatches, func(i, j int) bool { return diff.HostnameMismatches[i].DeviceID < diff.HostnameMismatches[j].DeviceID })
	sort.Slice(diff.MissingAdjacencies, func(i, j int) bool {
		return goldenAdjacencyKey(diff.MissingAdjacencies[i]) < goldenAdjacencyKey(diff.MissingAdjacencies[j])
	})
	sort.Slice(diff.UnexpectedAdjacencies, func(i, j int) bool {
		return goldenAdjacencyKey(diff.UnexpectedAdjacencies[i]) < goldenAdjacencyKey(diff.UnexpectedAdjacencies[j])
	})
	sort.Slice(diff.MetadataMismatches, func(i, j int) bool { return diff.MetadataMismatches[i].Field < diff.MetadataMismatches[j].Field })

	if len(diff.MissingDevices) > 0 ||
		len(diff.UnexpectedDevices) > 0 ||
		len(diff.HostnameMismatches) > 0 ||
		len(diff.MissingAdjacencies) > 0 ||
		len(diff.UnexpectedAdjacencies) > 0 ||
		len(diff.MetadataMismatches) > 0 {
		diff.ZeroDiff = false
	}
	return diff
}

func buildCountDelta(field string, expected, actual int) behaviorOracleCountDelta {
	if expected == actual {
		return behaviorOracleCountDelta{}
	}
	return behaviorOracleCountDelta{
		Field:    field,
		Expected: expected,
		Actual:   actual,
	}
}

func goldenAdjacencyKey(adj parity.GoldenAdjacency) string {
	return fmt.Sprintf("%s|%s|%s|%s|%s", adj.Protocol, adj.SourceDevice, adj.SourcePort, adj.TargetDevice, adj.TargetPort)
}

func scenarioInScope(scenario parity.ManifestScenario) bool {
	for _, protocol := range enabledProtocols(scenario.Protocols) {
		switch protocol {
		case "lldp", "cdp", "bridge_fdb", "arp_nd":
			continue
		default:
			return false
		}
	}
	return true
}

func buildBehaviorOracleDiffMarkdown(report behaviorOracleReport) string {
	var b strings.Builder
	b.WriteString("# Behavior Oracle Diff Report\n\n")
	b.WriteString(fmt.Sprintf("- Generated at (UTC): `%s`\n", report.GeneratedAtUTC))
	b.WriteString(fmt.Sprintf("- Status: `%s`\n", report.Status))
	b.WriteString(fmt.Sprintf("- In-scope protocols: `%s`\n", strings.Join(report.Scope.Protocols, ", ")))
	b.WriteString(fmt.Sprintf("- Scenarios: total `%d`, in-scope `%d`, skipped `%d`\n",
		report.Totals.ScenariosTotal, report.Totals.ScenariosInScope, report.Totals.ScenariosSkipped))
	b.WriteString(fmt.Sprintf("- Zero-diff `%d`, with diffs `%d`, failures `%d`\n\n",
		report.Totals.ScenariosZeroDiff, report.Totals.ScenariosWithDiffs, report.Totals.ScenariosWithFailures))

	b.WriteString("## Pass Criteria\n\n")
	b.WriteString("- No missing or unexpected device IDs.\n")
	b.WriteString("- No hostname mismatches for matched device IDs.\n")
	b.WriteString("- No missing or unexpected directed adjacency keys (`protocol|source_device|source_port|target_device|target_port`).\n")
	b.WriteString("- No metadata mismatches (`devices`, `directional_adjacencies`).\n\n")

	b.WriteString("## Per-Scenario Summary\n\n")
	for _, scenario := range report.Scenarios {
		b.WriteString(fmt.Sprintf("- `%s` (%s): status `%s`; missing_devices=%d unexpected_devices=%d hostname_mismatches=%d missing_adjacencies=%d unexpected_adjacencies=%d metadata_mismatches=%d\n",
			scenario.ID,
			strings.Join(scenario.Protocols, ","),
			scenario.Status,
			len(scenario.Diff.MissingDevices),
			len(scenario.Diff.UnexpectedDevices),
			len(scenario.Diff.HostnameMismatches),
			len(scenario.Diff.MissingAdjacencies),
			len(scenario.Diff.UnexpectedAdjacencies),
			len(scenario.Diff.MetadataMismatches)))
	}
	b.WriteString("\n")

	b.WriteString("## Command Evidence\n\n")
	b.WriteString("- `go run ./tools/topology-parity-evidence --mode oracle-diff`\n")
	return b.String()
}

type testSelection struct {
	packagePath string
	tests       []string
}

type goTestSelectionResult struct {
	command      string
	passed       []string
	failed       []string
	missing      []string
	commandError string
}

type goTestEvent struct {
	Action string `json:"Action"`
	Test   string `json:"Test"`
}

func runPhase2(opts options) error {
	summary, err := buildSuiteSummary(opts)
	if err != nil {
		return err
	}

	modules := []struct {
		name       string
		selections []testSelection
	}{
		{
			name: "lldp",
			selections: []testSelection{{
				packagePath: "./pkg/topology/engine",
				tests: []string{
					"TestMatchLLDPLinksEnlinkdPassOrder_Precedence",
					"TestMatchLLDPLinksEnlinkdPassOrder_FallbackPasses/port-description",
					"TestMatchLLDPLinksEnlinkdPassOrder_FallbackPasses/sysname",
					"TestMatchLLDPLinksEnlinkdPassOrder_FallbackPasses/chassis-port-subtype",
					"TestMatchLLDPLinksEnlinkdPassOrder_FallbackPasses/chassis-port-description",
					"TestMatchLLDPLinksEnlinkdPassOrder_FallbackPasses/chassis-only",
				},
			}},
		},
		{
			name: "cdp",
			selections: []testSelection{{
				packagePath: "./pkg/topology/engine",
				tests: []string{
					"TestMatchCDPLinksEnlinkdPassOrder_DefaultAndParsedTarget",
					"TestMatchCDPLinksEnlinkdPassOrder_SkipsSelfTarget",
				},
			}},
		},
		{
			name: "bridge_fdb_arp",
			selections: []testSelection{{
				packagePath: "./pkg/topology/engine",
				tests: []string{
					"TestBuildL2ResultFromObservations_FDBAttachments",
					"TestBuildL2ResultFromObservations_FDBDropsDuplicateMACAcrossPorts",
					"TestBuildL2ResultFromObservations_FDBSkipsSelfAndNonLearned",
					"TestBuildL2ResultFromObservations_FDBBridgeDomainFallbackToBridgePort",
				},
			}},
		},
		{
			name: "updater",
			selections: []testSelection{
				{
					packagePath: "./pkg/topology/engine",
					tests: []string{
						"TestBuildL2ResultFromObservations_AnnotatesPairMetadata",
					},
				},
				{
					packagePath: "./pkg/topology/engine",
					tests: []string{
						"TestToTopologyData_MergesPairedAdjacenciesIntoBidirectionalLink",
					},
				},
			},
		},
	}

	moduleStatus := make([]phase2CheckStatus, 0, len(modules))
	overallPass := summary.ScenariosFailed == 0
	for _, module := range modules {
		status, err := runSelectionGroup(module.name, module.selections)
		if err != nil {
			return err
		}
		moduleStatus = append(moduleStatus, status)
		if status.Status != "pass" {
			overallPass = false
		}
	}

	reversePairQuality, err := runSelectionGroup("reverse_pair_quality", []testSelection{{
		packagePath: "./plugin/go.d/collector/snmp",
		tests: []string{
			"TestTopologyCache_LldpSnapshot",
			"TestTopologyCache_CdpSnapshot",
			"TestTopologyCache_CdpSnapshotHexAddress",
			"TestTopologyCache_CdpSnapshotRawAddressWithoutIP",
			"TestTopologyCache_SnapshotBidirectionalPairMetadata",
		},
	}})
	if err != nil {
		return err
	}
	if reversePairQuality.Status != "pass" {
		overallPass = false
	}

	identityMergeQuality, err := runSelectionGroup("identity_merge_quality", []testSelection{{
		packagePath: "./plugin/go.d/collector/snmp",
		tests: []string{
			"TestTopologyCache_SnapshotMergesRemoteIdentityAcrossProtocols",
		},
	}})
	if err != nil {
		return err
	}
	if identityMergeQuality.Status != "pass" {
		overallPass = false
	}

	assertionCoverage, err := computePhase2AssertionCoverage(opts.evidencePath)
	if err != nil {
		return err
	}
	if assertionCoverage.Status != "pass" {
		overallPass = false
	}

	report := phase2Report{
		Version:        "v1",
		GeneratedAtUTC: time.Now().UTC().Format(time.RFC3339),
		Status:         "pass",
		Suite: phase2SuiteSummary{
			FixtureScenarios:      summary.FixtureScenarios,
			FixtureFiles:          summary.FixtureFiles,
			TotalScenarios:        summary.TotalScenarios,
			ScenariosPassed:       summary.ScenariosPassed,
			ScenariosFailed:       summary.ScenariosFailed,
			TotalTestsMapped:      summary.TotalTestsMapped,
			TotalTestsInventory:   summary.TotalTestsInventory,
			TotalAssertionsMapped: summary.TotalAssertionsMapped,
			TotalAssertionsTotal:  summary.TotalAssertionsTotal,
			ProtocolCounts:        append([]protocolSummary(nil), summary.ProtocolCounts...),
		},
		ModuleParity:         moduleStatus,
		ReversePairQuality:   reversePairQuality,
		IdentityMergeQuality: identityMergeQuality,
		AssertionCoverage:    assertionCoverage,
		DeferredGaps: []phase2DeferredGap{
			{
				ID:          "gap-live-office-validation",
				Description: "Live office `topology:snmp` sanity validation is still pending.",
				Reason:      "Repository test fixtures do not include the live office runtime environment.",
				Evidence:    "TODO-topology-library-phase2-direct-port.md G3 runtime gate",
			},
		},
	}

	if !overallPass {
		report.Status = "fail"
	}

	reportBytes, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal phase2 report json: %w", err)
	}
	reportBytes = append(reportBytes, '\n')
	if err := os.MkdirAll(filepath.Dir(opts.phase2Report), 0o755); err != nil {
		return fmt.Errorf("create phase2 report directory: %w", err)
	}
	if err := os.WriteFile(opts.phase2Report, reportBytes, 0o644); err != nil {
		return fmt.Errorf("write phase2 report %q: %w", opts.phase2Report, err)
	}

	gapReport := buildPhase2GapReportMarkdown(report)
	if err := os.MkdirAll(filepath.Dir(opts.phase2Gap), 0o755); err != nil {
		return fmt.Errorf("create phase2 gap report directory: %w", err)
	}
	if err := os.WriteFile(opts.phase2Gap, []byte(gapReport), 0o644); err != nil {
		return fmt.Errorf("write phase2 gap report %q: %w", opts.phase2Gap, err)
	}

	fmt.Printf("phase2 report complete\n")
	fmt.Printf("report file: %s\n", opts.phase2Report)
	fmt.Printf("gap report: %s\n", opts.phase2Gap)
	fmt.Printf("status: %s\n", report.Status)
	fmt.Printf("in-scope assertion coverage: %d/%d ported (not-applicable=%d unmapped=%d)\n",
		assertionCoverage.InScopePorted,
		assertionCoverage.InScopeTotal,
		assertionCoverage.InScopeNotApplicable,
		assertionCoverage.InScopeUnmapped)

	if report.Status != "pass" {
		return fmt.Errorf("phase2 report contains failing gates")
	}
	return nil
}

func runSelectionGroup(name string, selections []testSelection) (phase2CheckStatus, error) {
	status := phase2CheckStatus{
		Name:     name,
		Status:   "pass",
		Commands: make([]string, 0, len(selections)),
	}

	for _, selection := range selections {
		result, err := runGoTestSelection(selection)
		if err != nil {
			return phase2CheckStatus{}, err
		}
		status.Commands = append(status.Commands, result.command)
		status.ChecksTotal += len(selection.tests)
		status.ChecksPassed += len(result.passed)
		status.Failed = append(status.Failed, result.failed...)
		status.Missing = append(status.Missing, result.missing...)
		if result.commandError != "" {
			status.Errors = append(status.Errors, result.commandError)
		}
	}

	status.Failed = uniqueSortedStrings(status.Failed)
	status.Missing = uniqueSortedStrings(status.Missing)
	status.Errors = uniqueSortedStrings(status.Errors)

	if len(status.Failed) > 0 || len(status.Missing) > 0 || len(status.Errors) > 0 {
		status.Status = "fail"
	}
	return status, nil
}

func runGoTestSelection(selection testSelection) (goTestSelectionResult, error) {
	if strings.TrimSpace(selection.packagePath) == "" {
		return goTestSelectionResult{}, fmt.Errorf("test selection package path is empty")
	}
	if len(selection.tests) == 0 {
		return goTestSelectionResult{}, fmt.Errorf("test selection for %s has no tests", selection.packagePath)
	}

	topLevelTests := topLevelTestNames(selection.tests)
	regex := buildGoTestNameRegex(topLevelTests)
	args := []string{"test", "-json", selection.packagePath, "-run", regex, "-count=1"}
	command := "go " + strings.Join(args, " ")

	cmd := exec.Command("go", args...)
	output, err := cmd.CombinedOutput()

	passedSet := make(map[string]struct{}, len(selection.tests))
	failedSet := make(map[string]struct{}, len(selection.tests))
	scanner := bufio.NewScanner(bytes.NewReader(output))
	scanner.Buffer(make([]byte, 0, 64*1024), 2*1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		var event goTestEvent
		if json.Unmarshal(line, &event) != nil {
			continue
		}
		if strings.TrimSpace(event.Test) == "" {
			continue
		}
		switch event.Action {
		case "pass":
			passedSet[event.Test] = struct{}{}
		case "fail":
			failedSet[event.Test] = struct{}{}
		}
	}
	if scanErr := scanner.Err(); scanErr != nil {
		return goTestSelectionResult{}, fmt.Errorf("scan go test json output: %w", scanErr)
	}

	result := goTestSelectionResult{
		command: command,
		passed:  make([]string, 0, len(selection.tests)),
		failed:  make([]string, 0, len(selection.tests)),
		missing: make([]string, 0, len(selection.tests)),
	}
	for _, testName := range selection.tests {
		if _, failed := failedSet[testName]; failed {
			result.failed = append(result.failed, testName)
			continue
		}
		if _, passed := passedSet[testName]; passed {
			result.passed = append(result.passed, testName)
			continue
		}
		result.missing = append(result.missing, testName)
	}

	sort.Strings(result.passed)
	sort.Strings(result.failed)
	sort.Strings(result.missing)

	if err != nil {
		result.commandError = truncateWhitespace(string(output), 2048)
	}
	return result, nil
}

func buildGoTestNameRegex(testNames []string) string {
	parts := make([]string, 0, len(testNames))
	for _, name := range testNames {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		parts = append(parts, regexp.QuoteMeta(name))
	}
	sort.Strings(parts)
	return "^(" + strings.Join(parts, "|") + ")$"
}

func topLevelTestNames(testNames []string) []string {
	names := make([]string, 0, len(testNames))
	for _, name := range testNames {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if idx := strings.IndexByte(name, '/'); idx > 0 {
			name = name[:idx]
		}
		names = append(names, name)
	}
	return uniqueSortedStrings(names)
}

type assertionScopeRow struct {
	AssertionID string
	Scope       string
}

func computePhase2AssertionCoverage(evidencePath string) (phase2AssertionCoverage, error) {
	inventoryPath := filepath.Join(evidencePath, defaultAssertionInventory)
	mappingPath := filepath.Join(evidencePath, defaultAssertionMapping)

	assertions, err := readAssertionScopeRows(inventoryPath)
	if err != nil {
		return phase2AssertionCoverage{}, err
	}
	statusByAssertion, err := readMappingStatusByAssertion(mappingPath)
	if err != nil {
		return phase2AssertionCoverage{}, err
	}

	inScopeProtocols := map[string]struct{}{
		"lldp":       {},
		"cdp":        {},
		"bridge_fdb": {},
		"arp_nd":     {},
	}

	coverage := phase2AssertionCoverage{}
	for _, assertion := range assertions {
		status, ok := statusByAssertion[assertion.AssertionID]
		if _, inScope := inScopeProtocols[assertion.Scope]; inScope {
			coverage.InScopeTotal++
			if !ok {
				coverage.InScopeUnmapped++
				continue
			}
			switch status {
			case "ported":
				coverage.InScopePorted++
			case "not-applicable-approved":
				coverage.InScopeNotApplicable++
			default:
				return phase2AssertionCoverage{}, fmt.Errorf("unsupported status %q for in-scope assertion %q", status, assertion.AssertionID)
			}
			continue
		}

		if !ok {
			continue
		}
		switch status {
		case "ported":
			coverage.OutOfScopePorted++
		case "not-applicable-approved":
			coverage.OutOfScopeNotApplicable++
		default:
			return phase2AssertionCoverage{}, fmt.Errorf("unsupported status %q for out-of-scope assertion %q", status, assertion.AssertionID)
		}
	}

	coverage.Status = "pass"
	if coverage.InScopeNotApplicable > 0 || coverage.InScopeUnmapped > 0 || coverage.InScopePorted != coverage.InScopeTotal {
		coverage.Status = "fail"
	}
	return coverage, nil
}

func readAssertionScopeRows(path string) ([]assertionScopeRow, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open assertion inventory %q: %w", path, err)
	}
	defer f.Close()

	r := csv.NewReader(f)
	records, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("read assertion inventory %q: %w", path, err)
	}
	if len(records) == 0 {
		return nil, fmt.Errorf("assertion inventory %q is empty", path)
	}

	header := strings.Join(records[0], ",")
	expectedHeader := "class,method,assertion_id,source_file,line,assert_call,protocol_scope"
	if header != expectedHeader {
		return nil, fmt.Errorf("unexpected assertion inventory header in %q: %q", path, header)
	}

	rows := make([]assertionScopeRow, 0, len(records)-1)
	for i := 1; i < len(records); i++ {
		rec := records[i]
		if len(rec) != 7 {
			return nil, fmt.Errorf("assertion inventory %q line %d: expected 7 columns, got %d", path, i+1, len(rec))
		}
		rows = append(rows, assertionScopeRow{
			AssertionID: strings.TrimSpace(rec[2]),
			Scope:       strings.TrimSpace(rec[6]),
		})
	}
	return rows, nil
}

func readMappingStatusByAssertion(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open mapping csv %q: %w", path, err)
	}
	defer f.Close()

	r := csv.NewReader(f)
	records, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("read mapping csv %q: %w", path, err)
	}
	if len(records) == 0 {
		return nil, fmt.Errorf("mapping csv %q is empty", path)
	}

	header := strings.Join(records[0], ",")
	expectedHeader := "upstream_class,upstream_method,upstream_assert_id,local_test,local_assert,status"
	if header != expectedHeader {
		return nil, fmt.Errorf("unexpected mapping header in %q: %q", path, header)
	}

	out := make(map[string]string, len(records)-1)
	for i := 1; i < len(records); i++ {
		rec := records[i]
		if len(rec) != 6 {
			return nil, fmt.Errorf("mapping csv %q line %d: expected 6 columns, got %d", path, i+1, len(rec))
		}
		assertionID := strings.TrimSpace(rec[2])
		status := strings.TrimSpace(rec[5])
		if status != "ported" && status != "not-applicable-approved" {
			return nil, fmt.Errorf("mapping csv %q line %d: unsupported status %q", path, i+1, status)
		}
		out[assertionID] = status
	}
	return out, nil
}

func buildPhase2GapReportMarkdown(report phase2Report) string {
	var b strings.Builder
	b.WriteString("# Topology Library Phase 2 Gap Report\n\n")
	b.WriteString(fmt.Sprintf("- Generated at (UTC): `%s`\n", report.GeneratedAtUTC))
	b.WriteString(fmt.Sprintf("- Overall status: `%s`\n", report.Status))
	b.WriteString(fmt.Sprintf("- Scenario parity: `%d/%d` passed\n", report.Suite.ScenariosPassed, report.Suite.TotalScenarios))
	b.WriteString(fmt.Sprintf("- Assertion parity: `%d/%d` mapped\n\n", report.Suite.TotalAssertionsMapped, report.Suite.TotalAssertionsTotal))

	b.WriteString("## What Matches Enlinkd (In Scope)\n\n")
	for _, module := range report.ModuleParity {
		b.WriteString(fmt.Sprintf("- `%s`: `%d/%d` checks passed (status: `%s`).\n",
			module.Name, module.ChecksPassed, module.ChecksTotal, module.Status))
	}
	b.WriteString(fmt.Sprintf("- In-scope assertion coverage: `%d/%d` ported, `%d` not-applicable-approved, `%d` unmapped.\n\n",
		report.AssertionCoverage.InScopePorted,
		report.AssertionCoverage.InScopeTotal,
		report.AssertionCoverage.InScopeNotApplicable,
		report.AssertionCoverage.InScopeUnmapped))

	b.WriteString("## Runtime Quality Checks\n\n")
	b.WriteString(fmt.Sprintf("- Reverse pair quality: `%d/%d` checks passed (status: `%s`).\n",
		report.ReversePairQuality.ChecksPassed,
		report.ReversePairQuality.ChecksTotal,
		report.ReversePairQuality.Status))
	b.WriteString(fmt.Sprintf("- Identity merge quality: `%d/%d` checks passed (status: `%s`).\n\n",
		report.IdentityMergeQuality.ChecksPassed,
		report.IdentityMergeQuality.ChecksTotal,
		report.IdentityMergeQuality.Status))

	b.WriteString("## Intentionally Deferred Gaps\n\n")
	for _, gap := range report.DeferredGaps {
		b.WriteString(fmt.Sprintf("- `%s`: %s\n", gap.ID, gap.Description))
		b.WriteString(fmt.Sprintf("  - Reason: %s\n", gap.Reason))
		b.WriteString(fmt.Sprintf("  - Evidence: %s\n", gap.Evidence))
	}
	b.WriteString("\n")

	b.WriteString("## Command Evidence\n\n")
	for _, module := range report.ModuleParity {
		for _, command := range module.Commands {
			b.WriteString(fmt.Sprintf("- `%s`\n", command))
		}
	}
	for _, command := range report.ReversePairQuality.Commands {
		b.WriteString(fmt.Sprintf("- `%s`\n", command))
	}
	for _, command := range report.IdentityMergeQuality.Commands {
		b.WriteString(fmt.Sprintf("- `%s`\n", command))
	}
	return b.String()
}

func uniqueSortedStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		set[value] = struct{}{}
	}
	out := make([]string, 0, len(set))
	for value := range set {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func buildSuiteSummary(opts options) (paritySummary, error) {
	localRows, err := verifyFixtureInventory(opts)
	if err != nil {
		return paritySummary{}, err
	}

	mapStats, err := collectMappingStats(opts.evidencePath)
	if err != nil {
		return paritySummary{}, err
	}

	scenarioResults, protocolCounts, err := collectScenarioSummaries(opts.manifestRoot)
	if err != nil {
		return paritySummary{}, err
	}

	passed := 0
	for _, result := range scenarioResults {
		if result.Passed {
			passed++
		}
	}

	return paritySummary{
		Version:               "v1",
		FixtureScenarios:      countDistinctScenarios(localRows),
		FixtureFiles:          len(localRows),
		TotalScenarios:        len(scenarioResults),
		ScenariosPassed:       passed,
		ScenariosFailed:       len(scenarioResults) - passed,
		TotalTestsMapped:      mapStats.MappedMethods,
		TotalTestsInventory:   mapStats.TotalMethods,
		TotalAssertionsMapped: mapStats.MappedAssertions,
		TotalAssertionsTotal:  mapStats.TotalAssertions,
		ProtocolCounts:        protocolCounts,
		ScenarioResults:       scenarioResults,
		GoTests:               runRequiredGoTests(),
	}, nil
}

func marshalSummaryJSON(summary paritySummary) ([]byte, error) {
	payload, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal summary json: %w", err)
	}
	return append(payload, '\n'), nil
}

func runRequiredGoTests() []goTestSummary {
	type testCmd struct {
		packageLabel string
		args         []string
	}

	commands := []testCmd{
		{
			packageLabel: "./pkg/topology/engine/parity",
			args:         []string{"test", "./pkg/topology/engine/parity"},
		},
		{
			packageLabel: "./pkg/topology/engine",
			args:         []string{"test", "./pkg/topology/engine"},
		},
		{
			packageLabel: "./tools/topology-parity-evidence",
			args:         []string{"test", "./tools/topology-parity-evidence"},
		},
		{
			packageLabel: "./plugin/go.d/collector/snmp -run ^TestTopology",
			args:         []string{"test", "./plugin/go.d/collector/snmp", "-run", "^TestTopology"},
		},
	}

	results := make([]goTestSummary, 0, len(commands))
	for _, tc := range commands {
		cmd := exec.Command("go", tc.args...)
		output, err := cmd.CombinedOutput()
		if err != nil {
			results = append(results, goTestSummary{
				Package: tc.packageLabel,
				Passed:  false,
				Error:   truncateWhitespace(string(output), 2048),
			})
			continue
		}
		results = append(results, goTestSummary{
			Package: tc.packageLabel,
			Passed:  true,
		})
	}
	return results
}

func collectScenarioSummaries(manifestRoot string) ([]scenarioSummary, []protocolSummary, error) {
	pattern := filepath.Join(manifestRoot, "*/manifest.yaml")
	manifestPaths, err := filepath.Glob(pattern)
	if err != nil {
		return nil, nil, fmt.Errorf("glob manifests %q: %w", pattern, err)
	}
	sort.Strings(manifestPaths)
	if len(manifestPaths) == 0 {
		return nil, nil, fmt.Errorf("no manifests found under %q", manifestRoot)
	}

	results := make([]scenarioSummary, 0, 64)
	protocolCounts := map[string]protocolSummary{
		"lldp":       {Protocol: "lldp"},
		"cdp":        {Protocol: "cdp"},
		"bridge_fdb": {Protocol: "bridge_fdb"},
		"arp_nd":     {Protocol: "arp_nd"},
	}

	for _, manifestPath := range manifestPaths {
		manifest, err := parity.LoadManifest(manifestPath)
		if err != nil {
			return nil, nil, err
		}

		scenarios := append([]parity.ManifestScenario(nil), manifest.Scenarios...)
		sort.Slice(scenarios, func(i, j int) bool {
			return scenarios[i].ID < scenarios[j].ID
		})

		for _, scenario := range scenarios {
			result := evaluateScenario(manifestPath, scenario)
			results = append(results, result)

			for _, protocol := range result.Protocols {
				count := protocolCounts[protocol]
				count.Total++
				if result.Passed {
					count.Passed++
				} else {
					count.Failed++
				}
				protocolCounts[protocol] = count
			}
		}
	}

	orderedProtocols := []string{"lldp", "cdp", "bridge_fdb", "arp_nd"}
	summary := make([]protocolSummary, 0, len(orderedProtocols))
	for _, protocol := range orderedProtocols {
		summary = append(summary, protocolCounts[protocol])
	}
	return results, summary, nil
}

func evaluateScenario(manifestPath string, scenario parity.ManifestScenario) scenarioSummary {
	out := scenarioSummary{
		ID:        scenario.ID,
		Manifest:  filepath.ToSlash(manifestPath),
		Protocols: enabledProtocols(scenario.Protocols),
	}

	failures := make([]string, 0, 4)

	resolved, err := parity.ResolveScenario(manifestPath, scenario)
	if err != nil {
		out.Failures = []string{err.Error()}
		return out
	}

	if err := parity.ValidateCache(resolved.GoldenYAML, resolved.GoldenJSON); err != nil {
		failures = append(failures, err.Error())
	}

	golden, err := parity.LoadGoldenYAML(resolved.GoldenYAML)
	if err != nil {
		failures = append(failures, err.Error())
		out.Failures = failures
		return out
	}

	walks, err := parity.LoadScenarioWalks(resolved)
	if err != nil {
		failures = append(failures, err.Error())
		out.Failures = failures
		return out
	}

	result, err := parity.BuildL2ResultFromWalks(walks, parity.BuildOptions{
		EnableLLDP:   scenario.Protocols.LLDP,
		EnableCDP:    scenario.Protocols.CDP,
		EnableBridge: scenario.Protocols.Bridge,
		EnableARP:    scenario.Protocols.ARPND,
	})
	if err != nil {
		failures = append(failures, err.Error())
		out.Failures = failures
		return out
	}

	if len(result.Devices) != golden.Expectations.Devices {
		failures = append(failures, fmt.Sprintf("devices mismatch: expected %d got %d", golden.Expectations.Devices, len(result.Devices)))
	}
	if len(result.Adjacencies) != golden.Expectations.DirectionalAdjacencies {
		failures = append(failures, fmt.Sprintf("directional adjacencies mismatch: expected %d got %d", golden.Expectations.DirectionalAdjacencies, len(result.Adjacencies)))
	}

	expectedAdjacencies := goldenAdjacencyKeySet(golden.Adjacencies)
	actualAdjacencies := resultAdjacencyKeySet(result.Adjacencies)
	if !stringSetEqual(expectedAdjacencies, actualAdjacencies) {
		failures = append(failures, fmt.Sprintf("adjacency set mismatch: expected %d keys got %d", len(expectedAdjacencies), len(actualAdjacencies)))
	}

	out.Passed = len(failures) == 0
	out.Failures = failures
	return out
}

func enabledProtocols(protocols parity.ManifestProtocols) []string {
	out := make([]string, 0, 4)
	if protocols.LLDP {
		out = append(out, "lldp")
	}
	if protocols.CDP {
		out = append(out, "cdp")
	}
	if protocols.Bridge {
		out = append(out, "bridge_fdb")
	}
	if protocols.ARPND {
		out = append(out, "arp_nd")
	}
	return out
}

func resultAdjacencyKeySet(adjacencies []engine.Adjacency) map[string]struct{} {
	out := make(map[string]struct{}, len(adjacencies))
	for _, adj := range adjacencies {
		out[fmt.Sprintf("%s|%s|%s|%s|%s", adj.Protocol, adj.SourceID, adj.SourcePort, adj.TargetID, adj.TargetPort)] = struct{}{}
	}
	return out
}

func goldenAdjacencyKeySet(adjacencies []parity.GoldenAdjacency) map[string]struct{} {
	out := make(map[string]struct{}, len(adjacencies))
	for _, adj := range adjacencies {
		out[fmt.Sprintf("%s|%s|%s|%s|%s", adj.Protocol, adj.SourceDevice, adj.SourcePort, adj.TargetDevice, adj.TargetPort)] = struct{}{}
	}
	return out
}

func stringSetEqual(a, b map[string]struct{}) bool {
	if len(a) != len(b) {
		return false
	}
	for key := range a {
		if _, ok := b[key]; !ok {
			return false
		}
	}
	return true
}

func collectMappingStats(evidencePath string) (mappingStats, error) {
	mappingFile := filepath.Join(evidencePath, defaultAssertionMapping)
	methodInventoryFile := filepath.Join(evidencePath, defaultMethodInventory)
	assertionInventoryFile := filepath.Join(evidencePath, defaultAssertionInventory)

	mappedAssertions, mappedMethods, err := readMappingCoverage(mappingFile)
	if err != nil {
		return mappingStats{}, err
	}
	assertionMethods, err := readAssertionMethodSet(assertionInventoryFile)
	if err != nil {
		return mappingStats{}, err
	}
	methodCoverage, err := readMethodCoverage(methodInventoryFile, mappedMethods, assertionMethods)
	if err != nil {
		return mappingStats{}, err
	}
	totalAssertions, err := readAssertionInventoryCount(assertionInventoryFile)
	if err != nil {
		return mappingStats{}, err
	}

	return mappingStats{
		MappedAssertions: mappedAssertions,
		TotalAssertions:  totalAssertions,
		MappedMethods:    methodCoverage.mappedMethods,
		TotalMethods:     methodCoverage.totalMethods,
		MappedTestFiles:  len(methodCoverage.mappedFiles),
		TotalTestFiles:   methodCoverage.totalFiles,
	}, nil
}

type methodCoverageInfo struct {
	totalMethods  int
	mappedMethods int
	totalFiles    int
	mappedFiles   map[string]struct{}
}

func readMappingCoverage(path string) (int, map[string]struct{}, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, nil, fmt.Errorf("open mapping csv %q: %w", path, err)
	}
	defer f.Close()

	r := csv.NewReader(f)
	records, err := r.ReadAll()
	if err != nil {
		return 0, nil, fmt.Errorf("read mapping csv %q: %w", path, err)
	}
	if len(records) == 0 {
		return 0, nil, fmt.Errorf("mapping csv %q is empty", path)
	}

	header := strings.Join(records[0], ",")
	expectedHeader := "upstream_class,upstream_method,upstream_assert_id,local_test,local_assert,status"
	if header != expectedHeader {
		return 0, nil, fmt.Errorf("unexpected mapping header in %q: %q", path, header)
	}

	methods := make(map[string]struct{}, len(records))
	for i := 1; i < len(records); i++ {
		rec := records[i]
		if len(rec) != 6 {
			return 0, nil, fmt.Errorf("mapping csv %q line %d: expected 6 columns, got %d", path, i+1, len(rec))
		}
		status := strings.TrimSpace(rec[5])
		if status != "ported" && status != "not-applicable-approved" {
			return 0, nil, fmt.Errorf("mapping csv %q line %d: unsupported status %q", path, i+1, status)
		}
		methods[rec[0]+"#"+rec[1]] = struct{}{}
	}
	return len(records) - 1, methods, nil
}

func readMethodCoverage(path string, mappedMethods map[string]struct{}, assertionMethods map[string]struct{}) (methodCoverageInfo, error) {
	f, err := os.Open(path)
	if err != nil {
		return methodCoverageInfo{}, fmt.Errorf("open method inventory %q: %w", path, err)
	}
	defer f.Close()

	r := csv.NewReader(f)
	records, err := r.ReadAll()
	if err != nil {
		return methodCoverageInfo{}, fmt.Errorf("read method inventory %q: %w", path, err)
	}
	if len(records) == 0 {
		return methodCoverageInfo{}, fmt.Errorf("method inventory %q is empty", path)
	}

	header := strings.Join(records[0], ",")
	expectedHeader := "class,method,source_file,protocol_scope"
	if header != expectedHeader {
		return methodCoverageInfo{}, fmt.Errorf("unexpected method inventory header in %q: %q", path, header)
	}

	allFiles := make(map[string]struct{}, len(records))
	mappedFiles := make(map[string]struct{}, len(records))
	mappedMethodCount := 0
	for i := 1; i < len(records); i++ {
		rec := records[i]
		if len(rec) != 4 {
			return methodCoverageInfo{}, fmt.Errorf("method inventory %q line %d: expected 4 columns, got %d", path, i+1, len(rec))
		}
		methodKey := rec[0] + "#" + rec[1]
		allFiles[rec[2]] = struct{}{}
		_, methodHasAssertions := assertionMethods[methodKey]
		_, methodMappedByAssertion := mappedMethods[methodKey]
		if methodMappedByAssertion || !methodHasAssertions {
			mappedMethodCount++
			mappedFiles[rec[2]] = struct{}{}
		}
	}

	return methodCoverageInfo{
		totalMethods:  len(records) - 1,
		mappedMethods: mappedMethodCount,
		totalFiles:    len(allFiles),
		mappedFiles:   mappedFiles,
	}, nil
}

func readAssertionMethodSet(path string) (map[string]struct{}, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open assertion inventory %q: %w", path, err)
	}
	defer f.Close()

	r := csv.NewReader(f)
	records, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("read assertion inventory %q: %w", path, err)
	}
	if len(records) == 0 {
		return nil, fmt.Errorf("assertion inventory %q is empty", path)
	}
	header := strings.Join(records[0], ",")
	expectedHeader := "class,method,assertion_id,source_file,line,assert_call,protocol_scope"
	if header != expectedHeader {
		return nil, fmt.Errorf("unexpected assertion inventory header in %q: %q", path, header)
	}

	methods := make(map[string]struct{}, len(records))
	for i := 1; i < len(records); i++ {
		rec := records[i]
		if len(rec) != 7 {
			return nil, fmt.Errorf("assertion inventory %q line %d: expected 7 columns, got %d", path, i+1, len(rec))
		}
		methods[rec[0]+"#"+rec[1]] = struct{}{}
	}
	return methods, nil
}

func readAssertionInventoryCount(path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, fmt.Errorf("open assertion inventory %q: %w", path, err)
	}
	defer f.Close()

	r := csv.NewReader(f)
	records, err := r.ReadAll()
	if err != nil {
		return 0, fmt.Errorf("read assertion inventory %q: %w", path, err)
	}
	if len(records) == 0 {
		return 0, fmt.Errorf("assertion inventory %q is empty", path)
	}
	header := strings.Join(records[0], ",")
	expectedHeader := "class,method,assertion_id,source_file,line,assert_call,protocol_scope"
	if header != expectedHeader {
		return 0, fmt.Errorf("unexpected assertion inventory header in %q: %q", path, header)
	}
	return len(records) - 1, nil
}

func truncateWhitespace(s string, maxLen int) string {
	s = strings.TrimSpace(s)
	if len(s) <= maxLen {
		return s
	}
	return strings.TrimSpace(s[:maxLen]) + "...(truncated)"
}

func requireDir(p string) error {
	st, err := os.Stat(p)
	if err != nil {
		return err
	}
	if !st.IsDir() {
		return fmt.Errorf("%q is not a directory", p)
	}
	return nil
}

func syncFixtureMirror(srcRoot, dstRoot string) error {
	if err := os.MkdirAll(dstRoot, 0o755); err != nil {
		return fmt.Errorf("mkdir destination root: %w", err)
	}

	sourceFiles := make(map[string]struct{}, 256)
	sourceDirs := make(map[string]struct{}, 64)
	sourceDirs["."] = struct{}{}

	if err := filepath.WalkDir(srcRoot, func(srcPath string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(srcRoot, srcPath)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		rel = filepath.Clean(rel)
		dstPath := filepath.Join(dstRoot, rel)

		if d.IsDir() {
			sourceDirs[rel] = struct{}{}
			return os.MkdirAll(dstPath, 0o755)
		}

		info, err := d.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		sourceFiles[rel] = struct{}{}
		if err := copyFile(srcPath, dstPath, info.Mode()); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return err
	}

	if err := pruneMirror(dstRoot, sourceFiles, sourceDirs); err != nil {
		return err
	}
	return nil
}

func copyFile(srcPath, dstPath string, mode fs.FileMode) error {
	src, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("open source %q: %w", srcPath, err)
	}
	defer src.Close()

	if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
		return fmt.Errorf("mkdir parent for %q: %w", dstPath, err)
	}

	dst, err := os.OpenFile(dstPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode.Perm())
	if err != nil {
		return fmt.Errorf("open destination %q: %w", dstPath, err)
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return fmt.Errorf("copy %q -> %q: %w", srcPath, dstPath, err)
	}
	return nil
}

func pruneMirror(dstRoot string, sourceFiles, sourceDirs map[string]struct{}) error {
	var allPaths []string
	if err := filepath.WalkDir(dstRoot, func(dstPath string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(dstRoot, dstPath)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		allPaths = append(allPaths, filepath.Clean(rel))
		return nil
	}); err != nil {
		return err
	}

	// Remove files first, then directories deepest-first.
	sort.Slice(allPaths, func(i, j int) bool {
		di := strings.Count(allPaths[i], string(filepath.Separator))
		dj := strings.Count(allPaths[j], string(filepath.Separator))
		if di != dj {
			return di > dj
		}
		return allPaths[i] > allPaths[j]
	})

	for _, rel := range allPaths {
		dstPath := filepath.Join(dstRoot, rel)
		info, err := os.Lstat(dstPath)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return err
		}

		if info.IsDir() {
			if _, ok := sourceDirs[rel]; ok {
				continue
			}
			if err := os.Remove(dstPath); err != nil && !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("remove stale dir %q: %w", dstPath, err)
			}
			continue
		}

		if _, ok := sourceFiles[rel]; ok {
			continue
		}
		if err := os.Remove(dstPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove stale file %q: %w", dstPath, err)
		}
	}
	return nil
}

func collectFixtureInventory(root, upstreamRelPrefix string) ([]fixtureRow, error) {
	rows := make([]fixtureRow, 0, 256)
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}

		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(filepath.Clean(rel))
		parts := strings.Split(rel, "/")
		if len(parts) < 2 {
			return fmt.Errorf("unexpected fixture relative path %q", rel)
		}

		hashValue, err := sha256File(p)
		if err != nil {
			return err
		}

		rows = append(rows, fixtureRow{
			Scenario:     parts[0],
			File:         path.Base(rel),
			RelativePath: rel,
			SHA256:       hashValue,
			SizeBytes:    info.Size(),
			UpstreamPath: path.Join(filepath.ToSlash(upstreamRelPrefix), rel),
		})
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(rows, func(i, j int) bool {
		if rows[i].RelativePath != rows[j].RelativePath {
			return rows[i].RelativePath < rows[j].RelativePath
		}
		return rows[i].SHA256 < rows[j].SHA256
	})
	return rows, nil
}

func sha256File(p string) (string, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

func listScopedTestFiles(enlinkdRoot string) ([]string, error) {
	scopedRoots := []string{
		filepath.Join(enlinkdRoot, defaultScopedTestsRelEn),
		filepath.Join(enlinkdRoot, defaultScopedTestsRelNB),
	}

	files := make([]string, 0, 32)
	for _, root := range scopedRoots {
		if _, err := os.Stat(root); errors.Is(err, os.ErrNotExist) {
			continue
		}
		err := filepath.WalkDir(root, func(p string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if d.IsDir() {
				return nil
			}
			name := d.Name()
			if !(testFileNameITRE.MatchString(name) || testFileNameTestR.MatchString(name)) {
				return nil
			}
			rel, err := filepath.Rel(enlinkdRoot, p)
			if err != nil {
				return err
			}
			files = append(files, filepath.ToSlash(rel))
			return nil
		})
		if err != nil {
			return nil, err
		}
	}

	sort.Strings(files)
	return files, nil
}

func collectTestAndAssertionInventories(enlinkdRoot string, methodFiles, assertionFiles []string) ([]methodRow, []assertionRow, error) {
	methods := make([]methodRow, 0, 256)
	assertions := make([]assertionRow, 0, 5000)

	for _, rel := range methodFiles {
		abs := filepath.Join(enlinkdRoot, filepath.FromSlash(rel))
		fileMethods, _, err := parseJavaTestFile(abs, rel)
		if err != nil {
			return nil, nil, fmt.Errorf("parse %q: %w", rel, err)
		}
		methods = append(methods, fileMethods...)
	}

	for _, rel := range assertionFiles {
		abs := filepath.Join(enlinkdRoot, filepath.FromSlash(rel))
		_, fileAssertions, err := parseJavaTestFile(abs, rel)
		if err != nil {
			return nil, nil, fmt.Errorf("parse assertions in %q: %w", rel, err)
		}
		assertions = append(assertions, fileAssertions...)
	}

	sort.Slice(methods, func(i, j int) bool {
		if methods[i].Class != methods[j].Class {
			return methods[i].Class < methods[j].Class
		}
		if methods[i].Method != methods[j].Method {
			return methods[i].Method < methods[j].Method
		}
		return methods[i].SourceFile < methods[j].SourceFile
	})

	sort.Slice(assertions, func(i, j int) bool {
		if assertions[i].Class != assertions[j].Class {
			return assertions[i].Class < assertions[j].Class
		}
		if assertions[i].Method != assertions[j].Method {
			return assertions[i].Method < assertions[j].Method
		}
		if assertions[i].Line != assertions[j].Line {
			return assertions[i].Line < assertions[j].Line
		}
		return assertions[i].AssertionID < assertions[j].AssertionID
	})

	return methods, assertions, nil
}

func listScopedJavaFiles(enlinkdRoot string) ([]string, error) {
	scopedRoots := []string{
		filepath.Join(enlinkdRoot, defaultScopedTestsRelEn),
		filepath.Join(enlinkdRoot, defaultScopedTestsRelNB),
	}

	files := make([]string, 0, 64)
	for _, root := range scopedRoots {
		if _, err := os.Stat(root); errors.Is(err, os.ErrNotExist) {
			continue
		}
		err := filepath.WalkDir(root, func(p string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if d.IsDir() {
				return nil
			}
			if filepath.Ext(d.Name()) != ".java" {
				return nil
			}
			rel, err := filepath.Rel(enlinkdRoot, p)
			if err != nil {
				return err
			}
			files = append(files, filepath.ToSlash(rel))
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	sort.Strings(files)
	return files, nil
}

func parseJavaTestFile(absPath, relPath string) ([]methodRow, []assertionRow, error) {
	data, err := os.ReadFile(absPath)
	if err != nil {
		return nil, nil, err
	}

	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	if len(lines) == 0 {
		return nil, nil, nil
	}

	packageName := ""
	className := ""
	methods := make([]methodRow, 0, 16)
	ranges := make([]methodRange, 0, 16)
	candidates := make([]assertionCandidate, 0, 128)

	inBlockComment := false
	pendingTest := false
	gatheringSignature := false
	signatureStartLine := 0
	signatureBuilder := strings.Builder{}
	inMethod := false
	methodName := ""
	methodStartLine := 0
	methodScope := ""
	braceDepth := 0
	var methodLines []string
	var methodLineNumbers []int

	for i, rawLine := range lines {
		lineNo := i + 1
		cleanLine, nextInBlockComment := stripJavaLine(rawLine, inBlockComment)
		inBlockComment = nextInBlockComment
		trimmed := strings.TrimSpace(cleanLine)

		if matches := assertionCallRE.FindAllStringSubmatchIndex(rawLine, -1); len(matches) > 0 {
			for _, m := range matches {
				if len(m) < 4 {
					continue
				}
				candidates = append(candidates, assertionCandidate{
					Line: lineNo,
					Call: rawLine[m[2]:m[3]],
				})
			}
		}

		if packageName == "" {
			if m := packageRE.FindStringSubmatch(trimmed); len(m) == 2 {
				packageName = m[1]
			}
		}
		if className == "" {
			if m := classRE.FindStringSubmatch(trimmed); len(m) == 2 {
				className = m[1]
			}
		}

		if inMethod {
			methodLines = append(methodLines, rawLine)
			methodLineNumbers = append(methodLineNumbers, lineNo)
			braceDepth += countBraces(cleanLine)

			if braceDepth <= 0 {
				fqcn := buildClassName(packageName, className)
				methods = append(methods, methodRow{
					Class:         fqcn,
					Method:        methodName,
					SourceFile:    relPath,
					ProtocolScope: methodScope,
				})
				ranges = append(ranges, methodRange{
					Name:  methodName,
					Start: methodStartLine,
					End:   lineNo,
					Scope: methodScope,
				})

				inMethod = false
				methodName = ""
				methodStartLine = 0
				methodScope = ""
				braceDepth = 0
				methodLines = nil
				methodLineNumbers = nil
			}
			continue
		}

		if testAnnotationRE.MatchString(trimmed) {
			pendingTest = true
			gatheringSignature = false
			signatureBuilder.Reset()
			signatureStartLine = 0
			continue
		}

		if !pendingTest {
			continue
		}

		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "@") {
			// Additional annotations between @Test and method signature.
			continue
		}

		if !gatheringSignature {
			if !looksLikeMethodDeclarationStart(trimmed) {
				// Skip annotation trailers like "})" that can appear after @Test annotations.
				continue
			}
			gatheringSignature = true
			signatureStartLine = lineNo
		}
		if signatureBuilder.Len() > 0 {
			signatureBuilder.WriteByte(' ')
		}
		signatureBuilder.WriteString(trimmed)

		if !strings.Contains(cleanLine, "{") {
			continue
		}

		name := extractMethodName(signatureBuilder.String())
		if name == "" {
			return nil, nil, fmt.Errorf("unable to parse test method name in %s near line %d", relPath, signatureStartLine)
		}

		methodName = name
		methodStartLine = signatureStartLine
		methodScope = detectProtocolScope(methodName, relPath, signatureBuilder.String())
		braceDepth = countBraces(signatureBuilder.String())
		methodLines = []string{rawLine}
		methodLineNumbers = []int{lineNo}
		inMethod = true
		pendingTest = false
		gatheringSignature = false
		signatureBuilder.Reset()
		signatureStartLine = 0

		if braceDepth <= 0 {
			// Single-line method body.
			fqcn := buildClassName(packageName, className)
			methods = append(methods, methodRow{
				Class:         fqcn,
				Method:        methodName,
				SourceFile:    relPath,
				ProtocolScope: methodScope,
			})
			ranges = append(ranges, methodRange{
				Name:  methodName,
				Start: methodStartLine,
				End:   lineNo,
				Scope: methodScope,
			})
			inMethod = false
			methodName = ""
			methodStartLine = 0
			methodScope = ""
			braceDepth = 0
			methodLines = nil
			methodLineNumbers = nil
		}

		_ = methodStartLine
	}

	if inMethod {
		return nil, nil, fmt.Errorf("unterminated method %q in %s near line %d", methodName, relPath, methodStartLine)
	}
	fqcn := buildClassName(packageName, className)
	assertions := collectAssertionsForFile(fqcn, relPath, candidates, ranges)
	return methods, assertions, nil
}

func looksLikeMethodDeclarationStart(trimmed string) bool {
	if strings.HasPrefix(trimmed, "public ") || strings.HasPrefix(trimmed, "protected ") || strings.HasPrefix(trimmed, "private ") {
		return true
	}
	// Some files may use package-private visibility for tests.
	return strings.Contains(trimmed, "(") && !strings.HasPrefix(trimmed, "}") && !strings.HasPrefix(trimmed, ")")
}

func stripJavaLine(line string, inBlockComment bool) (string, bool) {
	var out strings.Builder
	escaped := false
	inString := false
	inChar := false

	for i := 0; i < len(line); i++ {
		ch := line[i]

		if inBlockComment {
			if ch == '*' && i+1 < len(line) && line[i+1] == '/' {
				inBlockComment = false
				i++
			}
			continue
		}

		if inString {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == '"' {
				inString = false
			}
			continue
		}

		if inChar {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == '\'' {
				inChar = false
			}
			continue
		}

		if ch == '/' && i+1 < len(line) {
			next := line[i+1]
			if next == '/' {
				break
			}
			if next == '*' {
				inBlockComment = true
				i++
				continue
			}
		}

		if ch == '"' {
			inString = true
			continue
		}
		if ch == '\'' {
			inChar = true
			continue
		}
		out.WriteByte(ch)
	}

	return out.String(), inBlockComment
}

func countBraces(s string) int {
	delta := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '{':
			delta++
		case '}':
			delta--
		}
	}
	return delta
}

func extractMethodName(signature string) string {
	idx := strings.Index(signature, "(")
	if idx <= 0 {
		return ""
	}
	before := strings.TrimSpace(signature[:idx])
	fields := strings.Fields(before)
	if len(fields) == 0 {
		return ""
	}
	name := fields[len(fields)-1]
	if !identifierRE.MatchString(name) {
		return ""
	}
	switch name {
	case "if", "for", "while", "switch", "catch", "new", "return", "try":
		return ""
	}
	return name
}

func buildClassName(packageName, className string) string {
	if packageName == "" {
		return className
	}
	if className == "" {
		return packageName
	}
	return packageName + "." + className
}

func detectProtocolScope(methodName, sourceFile, material string) string {
	s := strings.ToLower(methodName + " " + sourceFile + " " + material)

	scopes := make([]string, 0, 4)
	add := func(scope string) {
		for _, existing := range scopes {
			if existing == scope {
				return
			}
		}
		scopes = append(scopes, scope)
	}

	if hasAny(s, "lldp", "chassis", "portid", "remport", "lldpre") {
		add("lldp")
	}
	if hasAny(s, "cdp", "cisco") {
		add("cdp")
	}
	if hasAny(s, "bridge", "fdb", "dot1d", "sharedsegment", "broadcastdomain", "stp", "vlan", "bridgemac") {
		add("bridge_fdb")
	}
	if hasAny(s, "arp", "ipnettomedia", "neighbor", "neighbour", "ndp", "iproute") {
		add("arp_nd")
	}

	if len(scopes) == 0 {
		return "other"
	}
	sort.Strings(scopes)
	return strings.Join(scopes, "|")
}

func hasAny(s string, tokens ...string) bool {
	for _, token := range tokens {
		if strings.Contains(s, token) {
			return true
		}
	}
	return false
}

func collectAssertionsForFile(className, sourceFile string, candidates []assertionCandidate, ranges []methodRange) []assertionRow {
	rows := make([]assertionRow, 0, len(candidates))
	counters := make(map[string]int, len(ranges))
	for _, c := range candidates {
		methodName := ""
		scope := ""
		for _, r := range ranges {
			if c.Line >= r.Start && c.Line <= r.End {
				methodName = r.Name
				scope = r.Scope
				break
			}
		}

		// Keep assertion inventory scoped to discovered @Test methods only.
		// Assertions in helpers/non-test code are not directly mappable to
		// method inventory and create false parity gaps.
		if methodName == "" {
			continue
		}

		counters[methodName]++
		rows = append(rows, assertionRow{
			Class:         className,
			Method:        methodName,
			AssertionID:   fmt.Sprintf("%s#%s#A%04d", className, methodName, counters[methodName]),
			SourceFile:    sourceFile,
			Line:          c.Line,
			AssertCall:    c.Call,
			ProtocolScope: scope,
		})
	}
	return rows
}

func writeFixtureInventoryCSV(outPath string, rows []fixtureRow) error {
	f, err := os.Create(outPath)
	if err != nil {
		return fmt.Errorf("create %q: %w", outPath, err)
	}
	defer f.Close()

	w := csv.NewWriter(f)
	if err := w.Write([]string{"scenario", "file", "relative_path", "sha256", "size_bytes", "upstream_path"}); err != nil {
		return err
	}
	for _, row := range rows {
		record := []string{
			row.Scenario,
			row.File,
			row.RelativePath,
			row.SHA256,
			strconv.FormatInt(row.SizeBytes, 10),
			row.UpstreamPath,
		}
		if err := w.Write(record); err != nil {
			return err
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return fmt.Errorf("write %q: %w", outPath, err)
	}
	return nil
}

func writeMethodInventoryCSV(outPath string, rows []methodRow) error {
	f, err := os.Create(outPath)
	if err != nil {
		return fmt.Errorf("create %q: %w", outPath, err)
	}
	defer f.Close()

	w := csv.NewWriter(f)
	if err := w.Write([]string{"class", "method", "source_file", "protocol_scope"}); err != nil {
		return err
	}
	for _, row := range rows {
		record := []string{row.Class, row.Method, row.SourceFile, row.ProtocolScope}
		if err := w.Write(record); err != nil {
			return err
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return fmt.Errorf("write %q: %w", outPath, err)
	}
	return nil
}

func writeAssertionInventoryCSV(outPath string, rows []assertionRow) error {
	f, err := os.Create(outPath)
	if err != nil {
		return fmt.Errorf("create %q: %w", outPath, err)
	}
	defer f.Close()

	w := csv.NewWriter(f)
	if err := w.Write([]string{"class", "method", "assertion_id", "source_file", "line", "assert_call", "protocol_scope"}); err != nil {
		return err
	}
	for _, row := range rows {
		record := []string{
			row.Class,
			row.Method,
			row.AssertionID,
			row.SourceFile,
			strconv.Itoa(row.Line),
			row.AssertCall,
			row.ProtocolScope,
		}
		if err := w.Write(record); err != nil {
			return err
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return fmt.Errorf("write %q: %w", outPath, err)
	}
	return nil
}

func readFixtureInventoryCSV(inPath string) ([]fixtureRow, error) {
	f, err := os.Open(inPath)
	if err != nil {
		return nil, fmt.Errorf("open %q: %w", inPath, err)
	}
	defer f.Close()

	r := csv.NewReader(f)
	records, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("read %q: %w", inPath, err)
	}
	if len(records) == 0 {
		return nil, fmt.Errorf("%q is empty", inPath)
	}
	header := strings.Join(records[0], ",")
	expectedHeader := "scenario,file,relative_path,sha256,size_bytes,upstream_path"
	if header != expectedHeader {
		return nil, fmt.Errorf("unexpected header in %q: %q", inPath, header)
	}

	rows := make([]fixtureRow, 0, len(records)-1)
	for i := 1; i < len(records); i++ {
		rec := records[i]
		if len(rec) != 6 {
			return nil, fmt.Errorf("%q line %d: expected 6 columns, got %d", inPath, i+1, len(rec))
		}
		size, err := strconv.ParseInt(rec[4], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("%q line %d: invalid size_bytes %q: %w", inPath, i+1, rec[4], err)
		}
		rows = append(rows, fixtureRow{
			Scenario:     rec[0],
			File:         rec[1],
			RelativePath: rec[2],
			SHA256:       rec[3],
			SizeBytes:    size,
			UpstreamPath: rec[5],
		})
	}

	sort.Slice(rows, func(i, j int) bool {
		if rows[i].RelativePath != rows[j].RelativePath {
			return rows[i].RelativePath < rows[j].RelativePath
		}
		return rows[i].SHA256 < rows[j].SHA256
	})
	return rows, nil
}

func compareFixtureInventories(expected, actual []fixtureRow) error {
	if len(expected) != len(actual) {
		return fmt.Errorf("row count mismatch: expected %d, got %d", len(expected), len(actual))
	}

	for i := range expected {
		e := expected[i]
		a := actual[i]
		if e.RelativePath != a.RelativePath {
			return fmt.Errorf("relative_path mismatch at row %d: expected %q, got %q", i+1, e.RelativePath, a.RelativePath)
		}
		if e.SHA256 != a.SHA256 {
			return fmt.Errorf("sha256 mismatch for %q: expected %s, got %s", e.RelativePath, e.SHA256, a.SHA256)
		}
		if e.SizeBytes != a.SizeBytes {
			return fmt.Errorf("size mismatch for %q: expected %d, got %d", e.RelativePath, e.SizeBytes, a.SizeBytes)
		}
		if e.UpstreamPath != a.UpstreamPath {
			return fmt.Errorf("upstream_path mismatch for %q: expected %q, got %q", e.RelativePath, e.UpstreamPath, a.UpstreamPath)
		}
	}
	return nil
}

func countDistinctScenarios(rows []fixtureRow) int {
	set := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		set[row.Scenario] = struct{}{}
	}
	return len(set)
}

func countDistinctFiles(rows []methodRow) int {
	set := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		set[row.SourceFile] = struct{}{}
	}
	return len(set)
}
