// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/netdata/netdata/go/plugins/internal/promprofile/validation"
)

func main() {
	os.Exit(runCLI(os.Args[1:], os.Stdout, os.Stderr))
}

func runCLI(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("prometheus-profile-validation", flag.ContinueOnError)
	fs.SetOutput(stderr)

	var opts promvalidation.Options
	var output string
	fs.StringVar(&opts.ProfilePath, "profile", "", "candidate Prometheus profile YAML")
	fs.Func("support-profile", "supporting Prometheus profile YAML (repeatable)", func(path string) error {
		opts.SupportingProfilePaths = append(opts.SupportingProfilePaths, path)
		return nil
	})
	fs.StringVar(&opts.DumpPath, "dump", "", "Prometheus exposition dump")
	fs.StringVar(&opts.JobPath, "job", "", "optional validation job policy YAML")
	fs.StringVar(&output, "output", "text", "report format: text or json")
	fs.Usage = func() {
		fmt.Fprintln(stderr, "Validate a Prometheus chart profile through Netdata's real collector and chartengine.")
		fmt.Fprintln(stderr)
		fmt.Fprintf(stderr, "Usage: %s --profile PROFILE [--support-profile PROFILE] --dump METRICS [--job JOB] [--output text|json]\n", fs.Name())
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
	if opts.ProfilePath == "" || opts.DumpPath == "" {
		fmt.Fprintln(stderr, "--profile and --dump are required")
		fs.Usage()
		return 2
	}
	if output != "text" && output != "json" {
		fmt.Fprintln(stderr, "--output must be either text or json")
		return 2
	}

	report := promvalidation.Validate(context.Background(), opts)
	var err error
	if output == "json" {
		err = promvalidation.WriteJSON(stdout, report)
	} else {
		err = promvalidation.WriteText(stdout, report)
	}
	if err != nil {
		fmt.Fprintf(stderr, "writing report: %v\n", err)
		return 2
	}
	if report.Passed() {
		return 0
	}
	return 1
}
