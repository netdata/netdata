// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
)

func main() {
	os.Exit(runCLI(os.Args[1:], os.Stdout, os.Stderr))
}

func runCLI(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("prometheus-profile-validation", flag.ContinueOnError)
	fs.SetOutput(stderr)

	var opts validationOptions
	var output string
	fs.StringVar(&opts.profilePath, "profile", "", "candidate Prometheus profile YAML")
	fs.StringVar(&opts.dumpPath, "dump", "", "Prometheus exposition dump")
	fs.StringVar(&opts.jobPath, "job", "", "optional validation job policy YAML")
	fs.StringVar(&output, "output", "text", "report format: text or json")
	fs.Usage = func() {
		fmt.Fprintln(stderr, "Validate a Prometheus chart profile through Netdata's real collector and chartengine.")
		fmt.Fprintln(stderr)
		fmt.Fprintf(stderr, "Usage: %s --profile PROFILE --dump METRICS [--job JOB] [--output text|json]\n", fs.Name())
		fmt.Fprintln(stderr)
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintf(stderr, "unexpected positional arguments: %v\n", fs.Args())
		fs.Usage()
		return 2
	}
	if opts.profilePath == "" || opts.dumpPath == "" {
		fmt.Fprintln(stderr, "--profile and --dump are required")
		fs.Usage()
		return 2
	}
	if output != "text" && output != "json" {
		fmt.Fprintln(stderr, "--output must be either text or json")
		return 2
	}

	report := validateProfile(opts)
	var err error
	if output == "json" {
		err = writeJSONReport(stdout, report)
	} else {
		err = writeTextReport(stdout, report)
	}
	if err != nil {
		fmt.Fprintf(stderr, "writing report: %v\n", err)
		return 2
	}
	if report.Verdict == verdictPass {
		return 0
	}
	return 1
}
