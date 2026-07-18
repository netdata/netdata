package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/netdata/netdata/go/plugins/internal/jobmgrtest/buildidentity"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(arguments []string) error {
	flags := flag.NewFlagSet("jobmgrtest-ibmmanifest", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	var goRoot string
	var executable string
	var output string
	flags.StringVar(&goRoot, "go-root", "", "absolute Go module root")
	flags.StringVar(
		&executable,
		"executable",
		"",
		"absolute supported IBM-enabled ibm.d.plugin",
	)
	flags.StringVar(
		&output,
		"output",
		"",
		"new absolute manifest path beside the executable",
	)
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New(
			"jobmgr IBM manifest: positional arguments are forbidden",
		)
	}
	for name, path := range map[string]string{
		"Go root":    goRoot,
		"executable": executable,
		"output":     output,
	} {
		if !filepath.IsAbs(path) {
			return fmt.Errorf(
				"jobmgr IBM manifest: %s must be absolute",
				name,
			)
		}
	}
	return buildidentity.GenerateIBMManifest(
		context.Background(),
		goRoot,
		executable,
		output,
	)
}
