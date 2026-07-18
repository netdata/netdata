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

	"github.com/netdata/netdata/go/plugins/internal/jobmgrtest/wireeval"
)

type options struct {
	baselineBundle       string
	productionExecutable string
	evidenceDirectory    string
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
	baseline, err := wireeval.VerifyBaselineBundle(opts.baselineBundle)
	if err != nil {
		return err
	}
	info, err := os.Stat(opts.productionExecutable)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&0o111 == 0 {
		return errors.New("jobmgr comparison: production executable is unavailable")
	}
	childArguments := []string{
		"--mode=wire/agent",
		"--fixture-config-dir=" + baseline.FixtureDir,
	}
	ctx, stop := signal.NotifyContext(
		context.Background(),
		syscall.SIGINT,
		syscall.SIGTERM,
	)
	defer stop()
	experiment, err := wireeval.RunExperiment(ctx, wireeval.ExperimentSpec{
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
	report := struct {
		EnvironmentSHA256 string `json:"environment_sha256"`
		Pass              bool   `json:"pass"`
		Gates             any    `json:"gates"`
	}{
		EnvironmentSHA256: experiment.EnvironmentSHA256,
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
	if err := flags.Parse(arguments); err != nil {
		return options{}, err
	}
	if flags.NArg() != 0 {
		return options{}, errors.New("jobmgr comparison: positional arguments are forbidden")
	}
	for name, value := range map[string]string{
		"baseline bundle":       opts.baselineBundle,
		"production executable": opts.productionExecutable,
		"evidence directory":    opts.evidenceDirectory,
	} {
		if !filepath.IsAbs(value) {
			return options{}, fmt.Errorf("jobmgr comparison: %s must be absolute", name)
		}
	}
	return opts, nil
}
