// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"

	"github.com/netdata/netdata/go/plugins/internal/promprofile/input"
	"github.com/netdata/netdata/go/plugins/internal/promprofile/proof"
	"github.com/netdata/netdata/go/plugins/internal/promprofile/replay"
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
	if command != "evidence-dirs" && command != "verify" {
		fmt.Fprintf(stderr, "error: unknown command %q\n", command)
		usage(stderr)
		return 2
	}
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	flags.SetOutput(stderr)
	repoRoot := flags.String("repo-root", defaultRepoRoot(), "Netdata repository root")
	testdataRoot := flags.String("testdata-root", "", "netdata/testdata checkout root (defaults to <repo-root>/src/go/testdata)")
	profileName := flags.String("profile", "", "operate on only this profile")
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

	bundles, err := promproof.Discover(*repoRoot)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	var profiles []string
	if *profileName != "" {
		for _, bundle := range bundles {
			if bundle.Descriptor.Profile == *profileName {
				profiles = append(profiles, *profileName)
				break
			}
		}
		if len(profiles) == 0 {
			fmt.Fprintf(stderr, "error: profile proof %q was not found\n", *profileName)
			return 1
		}
	} else {
		for _, bundle := range bundles {
			profiles = append(profiles, bundle.Descriptor.Profile)
		}
	}
	switch command {
	case "evidence-dirs":
		directories := make([]string, 0, len(profiles))
		for _, profile := range profiles {
			for _, bundle := range bundles {
				if bundle.Descriptor.Profile == profile {
					directories = append(directories, bundle.ExternalRoot())
					break
				}
			}
		}
		slices.Sort(directories)
		for _, directory := range directories {
			fmt.Fprintln(stdout, directory)
		}
	case "verify":
		if err := verifyBundles(context.Background(), *repoRoot, *testdataRoot, bundles, profiles, stdout); err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 1
		}
	}
	return 0
}

func verifyBundles(
	ctx context.Context,
	repoRoot, testdataRoot string,
	bundles []promproof.Bundle,
	profiles []string,
	stdout io.Writer,
) error {
	if len(profiles) == 0 {
		return nil
	}
	catalog, err := promproof.LoadCompiledCatalog(ctx, repoRoot, testdataRoot, bundles)
	if err != nil {
		return err
	}
	metadataPath := filepath.Join(
		repoRoot,
		filepath.FromSlash("src/go/plugin/go.d/collector/prometheus/metadata.yaml"),
	)
	if err := promproof.VerifyCompiledProfiles(
		ctx,
		repoRoot,
		testdataRoot,
		catalog,
		profiles,
		func(ctx context.Context, input prominput.ReplayCase) ([]promreplay.Result, error) {
			return replayValidation(ctx, input, metadataPath)
		},
	); err != nil {
		return err
	}
	for _, profile := range profiles {
		fmt.Fprintf(stdout, "verified %s\n", catalog.Bundles[profile].Bundle.Path)
	}
	return nil
}

func defaultRepoRoot() string {
	directory, err := os.Getwd()
	if err != nil {
		return "."
	}
	for {
		proofRoot := filepath.Join(directory, filepath.FromSlash(promproof.ProofRoot))
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
	fmt.Fprintln(writer, "usage: prometheus-profile-proof <evidence-dirs|verify> [options]")
}
