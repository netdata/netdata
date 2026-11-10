// SPDX-License-Identifier: GPL-3.0-or-later

// Run from the src/go module. The cleanup proof owns one runtime use for its
// entire process tree; Agent integration tests prove individual owner lifetimes.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	runtime "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/runtime"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() (err error) {
	storePath := flag.String("store", "", "installed managed runtime root")
	helper := flag.String("helper", "", "installed nd-run executable")
	script := flag.String("script", "", "browser-reuse proof.mjs path")
	scriptArg := flag.String("script-arg", "", "optional proof mode argument")
	work := flag.String("work", "", "new experiment output directory")
	limit := flag.Duration("timeout", 10*time.Minute, "whole experiment timeout")
	flag.Parse()
	for _, p := range []string{*storePath, *helper, *script, *work} {
		if !filepath.IsAbs(p) {
			return errors.New("all input paths must be absolute")
		}
	}
	if os.Geteuid() == 0 {
		return errors.New("run under the runtime service identity, not root")
	}
	if err = os.Mkdir(*work, 0700); err != nil {
		return err
	}
	out, err := os.Create(filepath.Join(*work, "results.jsonl"))
	if err != nil {
		return err
	}
	defer out.Close()
	diagnostic, err := os.Create(filepath.Join(*work, "stderr.log"))
	if err != nil {
		return err
	}
	defer diagnostic.Close()
	store, err := runtime.Open(runtime.DefaultConfig(*storePath))
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, store.Close()) }()
	consumer, err := store.AcquireSelected()
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, consumer.Close()) }()
	use, err := consumer.BeginUse()
	if err != nil {
		return err
	}
	restore := ndexec.SetRunnerPathsForTests(*helper, "")
	defer restore()
	ctx, cancel := context.WithTimeout(context.Background(), *limit)
	defer cancel()
	started := time.Now()
	args := []string{*script, use.Paths.EntryPoint, *work}
	if *scriptArg != "" {
		args = append(args, *scriptArg)
	}
	process, startErr := ndexec.StartUnprivilegedProcessTree(ctx,
		ndexec.TreeOptions{Stdout: out, Stderr: diagnostic, Completion: use.Completion},
		use.Paths.Node, args...)
	if startErr != nil {
		return errors.Join(startErr, use.Finish(true))
	}
	result := process.Wait()
	finishErr := use.Finish(result.Drained)
	manifest := consumer.Manifest()
	record := map[string]any{"drained": result.Drained, "elapsed_seconds": time.Since(started).Seconds(),
		"runtime_version": manifest.Version, "runtime_contract": manifest.Contract, "runtime_sha256": manifest.Archive.SHA256,
		"process_error": fmt.Sprint(result.Err), "finish_error": fmt.Sprint(finishErr)}
	encodeErr := json.NewEncoder(os.Stdout).Encode(record)
	return errors.Join(result.Err, finishErr, encodeErr)
}
