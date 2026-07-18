package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/netdata/netdata/go/plugins/internal/jobmgrtest/buildidentity"
	"github.com/netdata/netdata/go/plugins/internal/jobmgrtest/contract"
	"github.com/netdata/netdata/go/plugins/internal/jobmgrtest/runner"
	"github.com/netdata/netdata/go/plugins/internal/jobmgrtest/wireeval"
	"gopkg.in/yaml.v2"
)

const requiredComparisonGates = 171

const phaseModulePath = "github.com/netdata/netdata/go/plugins"

type options struct {
	goRoot              string
	baselineBundle      string
	evidenceDirectory   string
	rootConfigDirectory string
}

type phaseArtifacts struct {
	production       string
	godplugin        string
	ibmdplugin       string
	scriptsdplugin   string
	goRoot           string
	goExecutable     string
	productionSHA256 string
	source           buildidentity.Source
}

type phaseBuildTarget struct {
	path       string
	importPath string
	tags       string
	cgo        string
}

type packageGates struct {
	tests      map[string]struct{}
	benchmarks map[string]struct{}
}

type phaseSummary struct {
	Cases                int    `json:"cases"`
	ComponentTests       int    `json:"component_tests"`
	HotpathOwners        int    `json:"hotpath_owners"`
	DiagnosticBenchmarks int    `json:"diagnostic_benchmarks"`
	RuntimeSuites        int    `json:"runtime_suites"`
	ComparisonGates      int    `json:"comparison_gates"`
	EnvironmentSHA256    string `json:"environment_sha256"`
	SourceRevision       string `json:"source_revision"`
	SourceTree           string `json:"source_tree"`
	CandidateSHA256      string `json:"candidate_sha256"`
	Pass                 bool   `json:"pass"`
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(arguments []string) error {
	opts, err := parseOptions(arguments)
	if err != nil {
		return err
	}
	if err := validateInputs(opts); err != nil {
		return err
	}
	runtime.GOMAXPROCS(4)
	baseline, err := wireeval.VerifyBaselineBundle(opts.baselineBundle)
	if err != nil {
		return err
	}
	cases, err := contract.BMM002Cases()
	if err != nil {
		return err
	}
	if err := contract.ValidateEvidenceContract(); err != nil {
		return err
	}
	gates, err := collectPackageGates(cases)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(
		context.Background(),
		syscall.SIGINT,
		syscall.SIGTERM,
	)
	defer stop()
	artifacts, cleanup, err := buildPhaseArtifacts(
		ctx,
		opts,
		baseline.Executable,
	)
	if err != nil {
		return err
	}
	defer cleanup()

	if err := verifyNamedGates(
		ctx,
		artifacts.goExecutable,
		artifacts.goRoot,
		gates,
	); err != nil {
		return err
	}
	if err := runDeterministicGates(
		ctx,
		artifacts.goExecutable,
		artifacts.goRoot,
		gates,
	); err != nil {
		return err
	}
	if err := runHotpathBenchmarks(
		ctx,
		artifacts.goExecutable,
		artifacts.goRoot,
		gates,
	); err != nil {
		return err
	}
	if err := runProductionSuites(ctx, opts, artifacts); err != nil {
		return err
	}

	childArguments := []string{
		"--mode=wire/agent",
		"--fixture-config-dir=" + baseline.FixtureDir,
	}
	candidate := wireeval.CandidateProvenance{
		SourceRevision:   artifacts.source.Revision,
		SourceTree:       artifacts.source.GoTree,
		GoModSHA256:      artifacts.source.GoModSHA256,
		GoSumSHA256:      artifacts.source.GoSumSHA256,
		ExecutableSHA256: artifacts.productionSHA256,
		GoVersion:        runtime.Version(),
		Package: phaseModulePath +
			"/internal/jobmgrtest/cmd/agent",
		CGO: "0",
	}
	experiment, err := wireeval.RunExperiment(
		ctx,
		wireeval.ExperimentSpec{
			Baseline: wireeval.ChildSpec{
				Executable: baseline.Executable,
				Arguments:  append([]string(nil), childArguments...),
			},
			Production: wireeval.ChildSpec{
				Executable: artifacts.production,
				Arguments:  append([]string(nil), childArguments...),
			},
			EvidenceDirectory: opts.evidenceDirectory,
			Candidate:         &candidate,
			Progress: func(completed, total int) {
				if completed%10 == 0 || completed == total {
					_, _ = fmt.Fprintf(
						os.Stderr,
						"jobmgr phase: comparison completed %d/%d runs\n",
						completed,
						total,
					)
				}
			},
		},
	)
	if err != nil {
		return err
	}
	replayed, err := wireeval.VerifyEvidenceBundle(opts.evidenceDirectory)
	if err != nil {
		return err
	}
	if len(replayed.Gates) != requiredComparisonGates {
		return fmt.Errorf(
			"jobmgr phase: comparison produced %d gates, want %d",
			len(replayed.Gates),
			requiredComparisonGates,
		)
	}
	if !replayed.Pass {
		return errors.New(
			"jobmgr phase: one or more comparison gates failed",
		)
	}
	recordedCandidate, ok, err := wireeval.EvidenceCandidateProvenance(
		opts.evidenceDirectory,
	)
	if err != nil {
		return err
	}
	if !ok || recordedCandidate != candidate {
		return errors.New(
			"jobmgr phase: replayed Candidate provenance differs",
		)
	}

	summary := phaseSummary{
		Cases:                len(cases),
		ComponentTests:       countTests(gates),
		HotpathOwners:        len(contract.BMM002HotpathGates()),
		DiagnosticBenchmarks: countBenchmarks(gates),
		RuntimeSuites:        len(productionSuiteTests()),
		ComparisonGates:      len(replayed.Gates),
		EnvironmentSHA256:    experiment.EnvironmentSHA256,
		SourceRevision:       artifacts.source.Revision,
		SourceTree:           artifacts.source.GoTree,
		CandidateSHA256:      artifacts.productionSHA256,
		Pass:                 true,
	}
	return json.NewEncoder(os.Stdout).Encode(summary)
}

func parseOptions(arguments []string) (options, error) {
	var opts options
	flags := flag.NewFlagSet("jobmgrtest-phase", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	flags.StringVar(&opts.goRoot, "go-root", "", "absolute Go module root")
	flags.StringVar(
		&opts.baselineBundle,
		"baseline-bundle",
		"",
		"absolute sealed B-M-001 baseline bundle",
	)
	flags.StringVar(
		&opts.evidenceDirectory,
		"evidence-directory",
		"",
		"new absolute raw-evidence bundle directory",
	)
	flags.StringVar(
		&opts.rootConfigDirectory,
		"root-config-dir",
		"",
		"absolute isolated root config directory with empty jobs",
	)
	if err := flags.Parse(arguments); err != nil {
		return options{}, err
	}
	if flags.NArg() != 0 {
		return options{}, errors.New(
			"jobmgr phase: positional arguments are forbidden",
		)
	}
	paths := map[string]string{
		"Go root":               opts.goRoot,
		"baseline bundle":       opts.baselineBundle,
		"evidence directory":    opts.evidenceDirectory,
		"root config directory": opts.rootConfigDirectory,
	}
	for name, value := range paths {
		if !filepath.IsAbs(value) {
			return options{}, fmt.Errorf(
				"jobmgr phase: %s must be absolute",
				name,
			)
		}
	}
	return opts, nil
}

func validateInputs(opts options) error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf(
			"jobmgr phase: full B-M-002 validation requires Linux, got %s",
			runtime.GOOS,
		)
	}
	if info, err := os.Stat(filepath.Join(opts.goRoot, "go.mod")); err != nil ||
		!info.Mode().IsRegular() {
		return errors.New("jobmgr phase: Go module root is unavailable")
	}
	if _, err := os.Stat(opts.evidenceDirectory); !os.IsNotExist(err) {
		return errors.New(
			"jobmgr phase: evidence directory must not already exist",
		)
	}
	if err := validateRootConfigs(opts.rootConfigDirectory); err != nil {
		return err
	}
	return nil
}

func buildPhaseArtifacts(
	ctx context.Context,
	opts options,
	baselineExecutable string,
) (phaseArtifacts, func(), error) {
	if ctx == nil {
		return phaseArtifacts{}, func() {}, errors.New(
			"jobmgr phase: build context is nil",
		)
	}
	source, err := buildidentity.CurrentSource(ctx, opts.goRoot)
	if err != nil {
		return phaseArtifacts{}, func() {}, err
	}
	goTool, err := buildidentity.CurrentGoTool(ctx)
	if err != nil {
		return phaseArtifacts{}, func() {}, err
	}
	directory, err := os.MkdirTemp("", "jobmgrtest-phase-build-")
	if err != nil {
		return phaseArtifacts{}, func() {}, err
	}
	cleanup := func() { _ = os.RemoveAll(directory) }
	artifacts := phaseArtifacts{
		production:     filepath.Join(directory, "jobmgrtest-agent"),
		godplugin:      filepath.Join(directory, "go.d.plugin"),
		ibmdplugin:     filepath.Join(directory, "ibm.d.plugin"),
		scriptsdplugin: filepath.Join(directory, "scripts.d.plugin"),
		goRoot:         filepath.Join(directory, "source"),
		goExecutable:   goTool.Path,
		source:         source,
	}
	if err := buildidentity.ExportCommittedGoTree(
		ctx,
		opts.goRoot,
		artifacts.goRoot,
	); err != nil {
		cleanup()
		return phaseArtifacts{}, func() {}, err
	}
	targets := phaseBuildTargets(artifacts)
	names := make([]string, 0, len(targets))
	for name := range targets {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		target := targets[name]
		checksum, err := buildidentity.BuildExecutable(
			ctx,
			goTool,
			artifacts.goRoot,
			target.path,
			buildidentity.BuildTarget{
				ImportPath: target.importPath,
				Expectation: buildidentity.ExecutableExpectation{
					Package: phaseModulePath + "/" +
						strings.TrimPrefix(target.importPath, "./"),
					CGO:  target.cgo,
					Tags: target.tags,
				},
			},
		)
		if err != nil {
			cleanup()
			return phaseArtifacts{}, func() {}, fmt.Errorf(
				"jobmgr phase: build %s: %w",
				name,
				err,
			)
		}
		if name == "production Agent driver" {
			artifacts.productionSHA256 = checksum
		}
	}
	if artifacts.productionSHA256 == "" {
		cleanup()
		return phaseArtifacts{}, func() {}, errors.New(
			"jobmgr phase: production executable identity is absent",
		)
	}
	same, err := sameExecutableContent(
		baselineExecutable,
		artifacts.production,
	)
	if err != nil {
		cleanup()
		return phaseArtifacts{}, func() {}, err
	}
	if same {
		cleanup()
		return phaseArtifacts{}, func() {}, errors.New(
			"jobmgr phase: baseline and production executable identities are equal",
		)
	}
	versionCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	command := exec.CommandContext(
		versionCtx,
		artifacts.ibmdplugin,
		"--version",
	)
	command.Env = runner.ChildEnvironment()
	command.Stdout = os.Stderr
	command.Stderr = os.Stderr
	if err := command.Run(); err != nil {
		cleanup()
		return phaseArtifacts{}, func() {}, errors.New(
			"jobmgr phase: ibm.d.plugin is not a runnable IBM-enabled binary",
		)
	}
	return artifacts, cleanup, nil
}

func phaseBuildTargets(
	artifacts phaseArtifacts,
) map[string]phaseBuildTarget {
	return map[string]phaseBuildTarget{
		"production Agent driver": {
			path:       artifacts.production,
			importPath: "./internal/jobmgrtest/cmd/agent",
			cgo:        "0",
		},
		"go.d.plugin": {
			path:       artifacts.godplugin,
			importPath: "./cmd/godplugin",
			cgo:        "0",
		},
		"ibm.d.plugin": {
			path:       artifacts.ibmdplugin,
			importPath: "./cmd/ibmdplugin",
			tags:       "ibm_mq",
			cgo:        "1",
		},
		"scripts.d.plugin": {
			path:       artifacts.scriptsdplugin,
			importPath: "./cmd/scriptsdplugin",
			cgo:        "0",
		},
	}
}

func sameExecutableContent(left, right string) (bool, error) {
	digest := func(path string) ([sha256.Size]byte, error) {
		file, err := os.Open(path)
		if err != nil {
			return [sha256.Size]byte{}, err
		}
		defer file.Close()
		hash := sha256.New()
		if _, err := io.Copy(hash, file); err != nil {
			return [sha256.Size]byte{}, err
		}
		var result [sha256.Size]byte
		copy(result[:], hash.Sum(nil))
		return result, nil
	}
	leftDigest, err := digest(left)
	if err != nil {
		return false, err
	}
	rightDigest, err := digest(right)
	if err != nil {
		return false, err
	}
	return leftDigest == rightDigest, nil
}

func validateRootConfigs(directory string) error {
	info, err := os.Stat(directory)
	if err != nil || !info.IsDir() {
		return errors.New(
			"jobmgr phase: root config directory is unavailable",
		)
	}
	required := []string{
		"go.d/testrandom.conf",
		"ibm.d/websphere_mp.conf",
		"scripts.d/nagios.conf",
	}
	for _, name := range required {
		content, err := os.ReadFile(filepath.Join(directory, name))
		if err != nil {
			return fmt.Errorf(
				"jobmgr phase: root config %s is unavailable",
				name,
			)
		}
		var config struct {
			Jobs []any `yaml:"jobs"`
		}
		if err := yaml.UnmarshalStrict(content, &config); err != nil {
			return fmt.Errorf(
				"jobmgr phase: root config %s is invalid: %w",
				name,
				err,
			)
		}
		if config.Jobs == nil || len(config.Jobs) != 0 {
			return fmt.Errorf(
				"jobmgr phase: root config %s must contain jobs: []",
				name,
			)
		}
	}
	return nil
}

func collectPackageGates(
	cases []contract.ProductionCase,
) (map[string]*packageGates, error) {
	gates := make(map[string]*packageGates)
	addTest := func(proof contract.ComponentProof) {
		entry := gates[proof.Package]
		if entry == nil {
			entry = &packageGates{
				tests:      make(map[string]struct{}),
				benchmarks: make(map[string]struct{}),
			}
			gates[proof.Package] = entry
		}
		entry.tests[proof.Test] = struct{}{}
	}
	for _, proof := range contract.BMM001StructuralProofs() {
		addTest(proof)
	}
	for _, productionCase := range cases {
		evidence, err := contract.EvidenceFor(productionCase)
		if err != nil {
			return nil, err
		}
		for _, proof := range evidence.Components {
			addTest(proof)
		}
	}
	for _, hotpath := range contract.BMM002HotpathGates() {
		for _, test := range hotpath.Tests {
			addTest(contract.ComponentProof{
				Package: hotpath.Package,
				Test:    test,
			})
		}
		entry := gates[hotpath.Package]
		entry.benchmarks[hotpath.Benchmark] = struct{}{}
	}
	return gates, nil
}

func verifyNamedGates(
	ctx context.Context,
	goExecutable string,
	goRoot string,
	gates map[string]*packageGates,
) error {
	for _, packageName := range sortedPackages(gates) {
		command := exec.CommandContext(
			ctx,
			goExecutable,
			"test",
			"-list",
			".",
			packageName,
		)
		command.Dir = goRoot
		command.Env = phaseGoEnvironment(nil)
		var stdout bytes.Buffer
		command.Stdout = &stdout
		command.Stderr = os.Stderr
		if err := command.Run(); err != nil {
			return fmt.Errorf(
				"jobmgr phase: list gates for %s: %w",
				packageName,
				err,
			)
		}
		listed := parseGoTestList(stdout.String())
		for name := range gates[packageName].tests {
			if _, ok := listed[name]; !ok {
				return fmt.Errorf(
					"jobmgr phase: %s lacks test %s",
					packageName,
					name,
				)
			}
		}
		for name := range gates[packageName].benchmarks {
			if _, ok := listed[name]; !ok {
				return fmt.Errorf(
					"jobmgr phase: %s lacks benchmark %s",
					packageName,
					name,
				)
			}
		}
	}
	return nil
}

func parseGoTestList(output string) map[string]struct{} {
	listed := make(map[string]struct{})
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		if strings.HasPrefix(fields[0], "Test") ||
			strings.HasPrefix(fields[0], "Benchmark") ||
			strings.HasPrefix(fields[0], "Example") {
			listed[fields[0]] = struct{}{}
		}
	}
	return listed
}

func runDeterministicGates(
	ctx context.Context,
	goExecutable string,
	goRoot string,
	gates map[string]*packageGates,
) error {
	for _, packageName := range sortedPackages(gates) {
		names := sortedNames(gates[packageName].tests)
		if len(names) == 0 {
			continue
		}
		if err := runGoTest(
			ctx,
			goExecutable,
			goRoot,
			nil,
			"-count=1",
			packageName,
			"-run",
			exactNamePattern(names),
		); err != nil {
			return fmt.Errorf(
				"jobmgr phase: deterministic gates for %s: %w",
				packageName,
				err,
			)
		}
	}
	return nil
}

func runHotpathBenchmarks(
	ctx context.Context,
	goExecutable string,
	goRoot string,
	gates map[string]*packageGates,
) error {
	for _, packageName := range sortedPackages(gates) {
		names := sortedNames(gates[packageName].benchmarks)
		if len(names) == 0 {
			continue
		}
		if err := runGoTest(
			ctx,
			goExecutable,
			goRoot,
			nil,
			"-count=1",
			packageName,
			"-run",
			"^$",
			"-bench",
			exactNamePattern(names),
			"-benchtime=100x",
			"-benchmem",
		); err != nil {
			return fmt.Errorf(
				"jobmgr phase: diagnostic hotpath benchmarks for %s: %w",
				packageName,
				err,
			)
		}
	}
	return nil
}

func runProductionSuites(
	ctx context.Context,
	opts options,
	artifacts phaseArtifacts,
) error {
	environment := map[string]string{
		"JOBMGRTEST_ROOT_CONFIG_DIR":    opts.rootConfigDirectory,
		"JOBMGRTEST_GODPLUGIN_BIN":      artifacts.godplugin,
		"JOBMGRTEST_IBMDPLUGIN_BIN":     artifacts.ibmdplugin,
		"JOBMGRTEST_SCRIPTSDPLUGIN_BIN": artifacts.scriptsdplugin,
		"JOBMGRTEST_REQUIRE_ALL_ROOTS":  "1",
	}
	return runGoTest(
		ctx,
		artifacts.goExecutable,
		artifacts.goRoot,
		environment,
		"-count=1",
		"./plugin/agent/jobmgr",
		"-run",
		exactNamePattern(productionSuiteTests()),
		"-v",
	)
}

func productionSuiteTests() []string {
	return []string{
		"TestProductionAgentCases",
		"TestProductionProcessCases",
		"TestProductionShippedRootCases",
		"TestProductionShippedRootScenarioMatrix",
		"TestProductionCollectorBoundaryCases",
		"TestProductionResolverCases",
	}
}

func runGoTest(
	ctx context.Context,
	goExecutable string,
	goRoot string,
	environment map[string]string,
	arguments ...string,
) error {
	command := exec.CommandContext(ctx, goExecutable, append(
		[]string{"test"},
		arguments...,
	)...)
	command.Dir = goRoot
	command.Env = phaseGoEnvironment(environment)
	command.Stdout = os.Stderr
	command.Stderr = os.Stderr
	return command.Run()
}

func phaseGoEnvironment(
	overrides map[string]string,
) []string {
	return buildidentity.GoEnvironment(overrides)
}

func exactNamePattern(names []string) string {
	escaped := make([]string, len(names))
	for index, name := range names {
		escaped[index] = regexp.QuoteMeta(name)
	}
	return "^(?:" + strings.Join(escaped, "|") + ")$"
}

func sortedPackages(gates map[string]*packageGates) []string {
	packages := make([]string, 0, len(gates))
	for packageName := range gates {
		packages = append(packages, packageName)
	}
	sort.Strings(packages)
	return packages
}

func sortedNames(names map[string]struct{}) []string {
	sorted := make([]string, 0, len(names))
	for name := range names {
		sorted = append(sorted, name)
	}
	sort.Strings(sorted)
	return sorted
}

func countTests(gates map[string]*packageGates) int {
	var count int
	for _, gate := range gates {
		count += len(gate.tests)
	}
	return count
}

func countBenchmarks(gates map[string]*packageGates) int {
	var count int
	for _, gate := range gates {
		count += len(gate.benchmarks)
	}
	return count
}
