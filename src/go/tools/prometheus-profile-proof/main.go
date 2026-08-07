// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/netdata/netdata/go/plugins/internal/promprofileproof"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(arguments []string, stdout, stderr io.Writer) int {
	if len(arguments) == 0 {
		usage(stderr)
		return 2
	}
	command := arguments[0]
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	flags.SetOutput(stderr)
	repoRoot := flags.String("repo-root", defaultRepoRoot(), "Netdata repository root")
	testdataRoot := flags.String("testdata-root", "", "netdata/testdata checkout root (defaults to <repo-root>/src/go/testdata)")
	if err := flags.Parse(arguments[1:]); err != nil {
		return 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintf(stderr, "error: unexpected arguments: %v\n", flags.Args())
		return 2
	}
	if *testdataRoot == "" {
		*testdataRoot = filepath.Join(*repoRoot, "src", "go", "testdata")
	}

	bundles, err := promprofileproof.Discover(*repoRoot)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	switch command {
	case "evidence-dirs":
		for _, directory := range promprofileproof.EvidenceDirectories(bundles) {
			fmt.Fprintln(stdout, directory)
		}
	case "verify":
		for _, bundle := range bundles {
			if err := promprofileproof.Verify(*repoRoot, *testdataRoot, bundle); err != nil {
				fmt.Fprintf(stderr, "error: %s: %v\n", bundle.Path, err)
				return 1
			}
			fmt.Fprintf(stdout, "verified %s\n", bundle.Path)
		}
	case "refresh":
		refreshed := make([]promprofileproof.Bundle, 0, len(bundles))
		for _, bundle := range bundles {
			updated, err := promprofileproof.Refresh(*repoRoot, *testdataRoot, bundle)
			if err != nil {
				fmt.Fprintf(stderr, "error: %s: %v\n", bundle.Path, err)
				return 1
			}
			refreshed = append(refreshed, updated)
		}
		for _, bundle := range refreshed {
			if err := promprofileproof.Write(*repoRoot, bundle); err != nil {
				fmt.Fprintf(stderr, "error: %s: %v\n", bundle.Path, err)
				return 1
			}
			fmt.Fprintf(stdout, "refreshed %s\n", bundle.Path)
		}
	default:
		fmt.Fprintf(stderr, "error: unknown command %q\n", command)
		usage(stderr)
		return 2
	}
	return 0
}

func defaultRepoRoot() string {
	directory, err := os.Getwd()
	if err != nil {
		return "."
	}
	for {
		proofRoot := filepath.Join(directory, filepath.FromSlash(promprofileproof.ProofRoot))
		if info, err := os.Stat(proofRoot); err == nil && info.IsDir() {
			return directory
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			return "."
		}
		directory = parent
	}
}

func usage(writer io.Writer) {
	fmt.Fprintln(writer, "usage: prometheus-profile-proof <evidence-dirs|verify|refresh> [options]")
}
