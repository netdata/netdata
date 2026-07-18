package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
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

	"github.com/netdata/netdata/go/plugins/internal/jobmgrtest/contract"
	"github.com/netdata/netdata/go/plugins/internal/jobmgrtest/wireeval"
	"gopkg.in/yaml.v2"
)

const requiredComparisonGates = 171

type options struct {
	goRoot               string
	baselineBundle       string
	productionExecutable string
	evidenceDirectory    string
	rootConfigDirectory  string
	godplugin            string
	ibmdplugin           string
	scriptsdplugin       string
}

type packageGates struct {
	tests      map[string]struct{}
	benchmarks map[string]struct{}
}

type phaseSummary struct {
	Cases             int    `json:"cases"`
	ComponentTests    int    `json:"component_tests"`
	HotpathOwners     int    `json:"hotpath_owners"`
	HotpathBenchmarks int    `json:"hotpath_benchmarks"`
	RuntimeSuites     int    `json:"runtime_suites"`
	ComparisonGates   int    `json:"comparison_gates"`
	EnvironmentSHA256 string `json:"environment_sha256"`
	Pass              bool   `json:"pass"`
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

	if err := verifyNamedGates(ctx, opts.goRoot, gates); err != nil {
		return err
	}
	if err := runDeterministicGates(ctx, opts.goRoot, gates); err != nil {
		return err
	}
	if err := runHotpathBenchmarks(ctx, opts.goRoot, gates); err != nil {
		return err
	}
	if err := runProductionSuites(ctx, opts); err != nil {
		return err
	}

	childArguments := []string{
		"--mode=wire/agent",
		"--fixture-config-dir=" + baseline.FixtureDir,
	}
	experiment, err := wireeval.RunExperiment(
		ctx,
		wireeval.ExperimentSpec{
			Baseline: wireeval.ChildSpec{
				Executable: baseline.Executable,
				Arguments:  append([]string(nil), childArguments...),
			},
			Production: wireeval.ChildSpec{
				Executable: opts.productionExecutable,
				Arguments:  append([]string(nil), childArguments...),
			},
			EvidenceDirectory: opts.evidenceDirectory,
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

	summary := phaseSummary{
		Cases:             len(cases),
		ComponentTests:    countTests(gates),
		HotpathOwners:     len(contract.BMM002HotpathGates()),
		HotpathBenchmarks: countBenchmarks(gates),
		RuntimeSuites:     len(productionSuiteTests()),
		ComparisonGates:   len(replayed.Gates),
		EnvironmentSHA256: experiment.EnvironmentSHA256,
		Pass:              true,
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
		&opts.productionExecutable,
		"production-executable",
		"",
		"absolute permanent Agent-driver executable",
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
	flags.StringVar(
		&opts.godplugin,
		"godplugin",
		"",
		"absolute prebuilt go.d.plugin",
	)
	flags.StringVar(
		&opts.ibmdplugin,
		"ibmdplugin",
		"",
		"absolute prebuilt IBM-enabled ibm.d.plugin",
	)
	flags.StringVar(
		&opts.scriptsdplugin,
		"scriptsdplugin",
		"",
		"absolute prebuilt scripts.d.plugin",
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
		"production executable": opts.productionExecutable,
		"evidence directory":    opts.evidenceDirectory,
		"root config directory": opts.rootConfigDirectory,
		"go.d.plugin":           opts.godplugin,
		"ibm.d.plugin":          opts.ibmdplugin,
		"scripts.d.plugin":      opts.scriptsdplugin,
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
	for name, path := range map[string]string{
		"production executable": opts.productionExecutable,
		"go.d.plugin":           opts.godplugin,
		"ibm.d.plugin":          opts.ibmdplugin,
		"scripts.d.plugin":      opts.scriptsdplugin,
	} {
		if err := validateExecutable(name, path); err != nil {
			return err
		}
	}
	if _, err := os.Stat(opts.evidenceDirectory); !os.IsNotExist(err) {
		return errors.New(
			"jobmgr phase: evidence directory must not already exist",
		)
	}
	if err := validateRootConfigs(opts.rootConfigDirectory); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, opts.ibmdplugin, "--version")
	command.Stdout = os.Stderr
	command.Stderr = os.Stderr
	if err := command.Run(); err != nil {
		return errors.New(
			"jobmgr phase: ibm.d.plugin is not a genuine IBM-enabled binary",
		)
	}
	return nil
}

func validateExecutable(name, path string) error {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&0o111 == 0 {
		return fmt.Errorf("jobmgr phase: %s is unavailable", name)
	}
	return nil
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
	goRoot string,
	gates map[string]*packageGates,
) error {
	for _, packageName := range sortedPackages(gates) {
		command := exec.CommandContext(
			ctx,
			"go",
			"test",
			"-list",
			".",
			packageName,
		)
		command.Dir = goRoot
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
				"jobmgr phase: hotpath gates for %s: %w",
				packageName,
				err,
			)
		}
	}
	return nil
}

func runProductionSuites(ctx context.Context, opts options) error {
	environment := map[string]string{
		"JOBMGRTEST_ROOT_CONFIG_DIR":    opts.rootConfigDirectory,
		"JOBMGRTEST_GODPLUGIN_BIN":      opts.godplugin,
		"JOBMGRTEST_IBMDPLUGIN_BIN":     opts.ibmdplugin,
		"JOBMGRTEST_SCRIPTSDPLUGIN_BIN": opts.scriptsdplugin,
		"JOBMGRTEST_REQUIRE_ALL_ROOTS":  "1",
	}
	return runGoTest(
		ctx,
		opts.goRoot,
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
		"TestProductionCollectorBoundaryCases",
		"TestProductionResolverCases",
	}
}

func runGoTest(
	ctx context.Context,
	goRoot string,
	environment map[string]string,
	arguments ...string,
) error {
	command := exec.CommandContext(ctx, "go", append(
		[]string{"test"},
		arguments...,
	)...)
	command.Dir = goRoot
	command.Env = withEnvironment(os.Environ(), environment)
	command.Stdout = os.Stderr
	command.Stderr = os.Stderr
	return command.Run()
}

func withEnvironment(
	base []string,
	overrides map[string]string,
) []string {
	environment := make([]string, 0, len(base)+len(overrides))
	for _, entry := range base {
		name, _, ok := strings.Cut(entry, "=")
		if _, replace := overrides[name]; ok && replace {
			continue
		}
		environment = append(environment, entry)
	}
	names := make([]string, 0, len(overrides))
	for name := range overrides {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		environment = append(
			environment,
			name+"="+overrides[name],
		)
	}
	return environment
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
