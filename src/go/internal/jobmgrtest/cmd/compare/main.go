package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"

	"github.com/netdata/netdata/go/plugins/internal/jobmgrtest/buildidentity"
	"github.com/netdata/netdata/go/plugins/internal/jobmgrtest/wireeval"
)

const compareModulePath = "github.com/netdata/netdata/go/plugins"

type options struct {
	goRoot            string
	baselineBundle    string
	evidenceDirectory string
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
	runtime.GOMAXPROCS(4)
	ctx, stop := signal.NotifyContext(
		context.Background(),
		syscall.SIGINT,
		syscall.SIGTERM,
	)
	defer stop()
	baseline, err := wireeval.VerifyBaselineBundle(opts.baselineBundle)
	if err != nil {
		return err
	}
	source, err := buildidentity.CurrentSource(ctx, opts.goRoot)
	if err != nil {
		return err
	}
	goTool, err := buildidentity.CurrentGoTool(ctx)
	if err != nil {
		return err
	}
	buildDirectory, err := os.MkdirTemp("", "jobmgrtest-compare-build-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(buildDirectory) }()
	exportedGoRoot := filepath.Join(buildDirectory, "source")
	if err := buildidentity.ExportCommittedGoTree(
		ctx,
		opts.goRoot,
		exportedGoRoot,
	); err != nil {
		return err
	}
	productionExecutable := filepath.Join(
		buildDirectory,
		"jobmgrtest-agent",
	)
	productionSHA256, err := buildidentity.BuildExecutable(
		ctx,
		goTool,
		exportedGoRoot,
		productionExecutable,
		buildidentity.BuildTarget{
			ImportPath: "./internal/jobmgrtest/cmd/agent",
			Expectation: buildidentity.ExecutableExpectation{
				Package: compareModulePath +
					"/internal/jobmgrtest/cmd/agent",
				CGO: "0",
			},
		},
	)
	if err != nil {
		return err
	}
	baselineSHA256, err := buildidentity.ArtifactSHA256(
		baseline.Executable,
	)
	if err != nil {
		return err
	}
	if baselineSHA256 == productionSHA256 {
		return errors.New(
			"jobmgr comparison: baseline and Candidate executable identities are equal",
		)
	}
	candidate := wireeval.CandidateProvenance{
		SourceRevision:   source.Revision,
		SourceTree:       source.GoTree,
		GoModSHA256:      source.GoModSHA256,
		GoSumSHA256:      source.GoSumSHA256,
		ExecutableSHA256: productionSHA256,
		GoVersion:        goTool.Version,
		Package: compareModulePath +
			"/internal/jobmgrtest/cmd/agent",
		CGO: "0",
	}
	childArguments := []string{
		"--mode=wire/agent",
		"--fixture-config-dir=" + baseline.FixtureDir,
	}
	experiment, err := wireeval.RunExperiment(ctx, wireeval.ExperimentSpec{
		Baseline: wireeval.ChildSpec{
			Executable: baseline.Executable,
			Arguments:  append([]string(nil), childArguments...),
		},
		Production: wireeval.ChildSpec{
			Executable: productionExecutable,
			Arguments:  append([]string(nil), childArguments...),
		},
		EvidenceDirectory: opts.evidenceDirectory,
		Candidate:         &candidate,
		Progress: func(completed, total int) {
			if completed%10 == 0 || completed == total {
				_, _ = fmt.Fprintf(
					os.Stderr,
					"jobmgr comparison: completed %d/%d runs\n",
					completed,
					total,
				)
			}
		},
	})
	if err != nil {
		return err
	}
	replayed, err := wireeval.VerifyEvidenceBundle(opts.evidenceDirectory)
	if err != nil {
		return err
	}
	recordedCandidate, ok, err := wireeval.EvidenceCandidateProvenance(
		opts.evidenceDirectory,
	)
	if err != nil {
		return err
	}
	if !ok || recordedCandidate != candidate {
		return errors.New(
			"jobmgr comparison: replayed Candidate provenance differs",
		)
	}
	report := struct {
		EnvironmentSHA256 string `json:"environment_sha256"`
		SourceRevision    string `json:"source_revision"`
		SourceTree        string `json:"source_tree"`
		CandidateSHA256   string `json:"candidate_sha256"`
		Pass              bool   `json:"pass"`
		Gates             any    `json:"gates"`
	}{
		EnvironmentSHA256: experiment.EnvironmentSHA256,
		SourceRevision:    source.Revision,
		SourceTree:        source.GoTree,
		CandidateSHA256:   productionSHA256,
		Pass:              replayed.Pass,
		Gates:             replayed.Gates,
	}
	if err := json.NewEncoder(os.Stdout).Encode(report); err != nil {
		return err
	}
	if !replayed.Pass {
		return errors.New("jobmgr comparison: one or more performance gates failed")
	}
	return nil
}

func parseOptions(arguments []string) (options, error) {
	var opts options
	flags := flag.NewFlagSet("jobmgrtest-compare", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	flags.StringVar(
		&opts.goRoot,
		"go-root",
		"",
		"absolute clean Go module root whose committed tree is built",
	)
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
	if err := flags.Parse(arguments); err != nil {
		return options{}, err
	}
	if flags.NArg() != 0 {
		return options{}, errors.New("jobmgr comparison: positional arguments are forbidden")
	}
	for name, value := range map[string]string{
		"Go root":            opts.goRoot,
		"baseline bundle":    opts.baselineBundle,
		"evidence directory": opts.evidenceDirectory,
	} {
		if !filepath.IsAbs(value) {
			return options{}, fmt.Errorf("jobmgr comparison: %s must be absolute", name)
		}
	}
	return opts, nil
}
