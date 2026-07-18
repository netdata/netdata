package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/netdata/netdata/go/plugins/internal/jobmgrtest/wireeval"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(arguments []string) error {
	flags := flag.NewFlagSet("jobmgrtest-verify", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	var evidenceDirectory string
	flags.StringVar(
		&evidenceDirectory,
		"evidence-directory",
		"",
		"absolute raw-evidence bundle directory",
	)
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("jobmgr verification: positional arguments are forbidden")
	}
	if !filepath.IsAbs(evidenceDirectory) {
		return errors.New("jobmgr verification: evidence directory must be absolute")
	}
	result, err := wireeval.VerifyEvidenceBundle(evidenceDirectory)
	if err != nil {
		return err
	}
	candidate, hasCandidate, err := wireeval.EvidenceCandidateProvenance(
		evidenceDirectory,
	)
	if err != nil {
		return err
	}
	report := struct {
		Gates     any                           `json:"Gates"`
		Pass      bool                          `json:"Pass"`
		Candidate *wireeval.CandidateProvenance `json:"Candidate,omitempty"`
	}{
		Gates: result.Gates,
		Pass:  result.Pass,
	}
	if hasCandidate {
		report.Candidate = &candidate
	}
	if err := json.NewEncoder(os.Stdout).Encode(report); err != nil {
		return err
	}
	if !result.Pass {
		return errors.New("jobmgr verification: one or more performance gates failed")
	}
	return nil
}
