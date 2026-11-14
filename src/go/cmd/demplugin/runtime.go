// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/buildinfo"
	"github.com/netdata/netdata/go/plugins/pkg/pluginconfig"
	"github.com/netdata/netdata/go/plugins/plugin/dem"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/runner"
	runtime "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/runtime"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/runtimehost"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
)

// runRuntimeCommand runs `dem.plugin runtime <command> [flags]`; args start
// after "runtime". main routes these commands before cli.Parse, so they never
// enter the plugin protocol. --cancel-on-stdin-close borrows input as a private
// control channel; the caller retains ownership of input.
func runRuntimeCommand(ctx context.Context, args []string, input *os.File, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: dem.plugin runtime {ensure|install|update|repair|uninstall|preload|status|qualify|setup-plan} [flags]")
	}
	command, flags := args[0], args[1:]
	switch {
	case command == "setup-plan":
		return runSetupPlan(flags, stdout, stderr)
	case strings.HasPrefix(command, "native-"):
		return runNativeCommand(ctx, command, flags, stdout, stderr)
	}
	switch command {
	case "ensure", "install", "update", "repair", "uninstall", "preload", "status", "qualify":
		return runAdminCommand(ctx, command, flags, input, stdout, stderr)
	default:
		return fmt.Errorf("unknown runtime command %q", command)
	}
}

// runAdminCommand runs an administrator command. Once configuration loads, it
// prints exactly one JSON report, failures included.
func runAdminCommand(ctx context.Context, command string, args []string, input *os.File, stdout, stderr io.Writer) error {
	f, err := parseAdminFlags(command, args, stderr)
	if errors.Is(err, flag.ErrHelp) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := f.validateEnsureOnly(command); err != nil {
		return err
	}
	if f.cancelOnInputClose {
		if input == nil {
			return errors.New("--cancel-on-stdin-close requires a private stdin control channel")
		}
		var finish func()
		ctx, finish, err = runtimeInputContext(ctx, input)
		if err != nil {
			return err
		}
		defer finish()
	}
	if err := f.validate(command); err != nil {
		return err
	}
	var confDirs []string
	if f.configDir != "" {
		confDirs = []string{f.configDir}
	}
	config, err := dem.LoadConfig(pluginconfig.ResolveConfigDirs(confDirs))
	if err != nil {
		return err
	}
	cfg := config.Runtime
	if f.cache != "" {
		cfg.CachePath = f.cache
	}
	report := runtimeCommandReport{Command: command, Mode: cfg.Mode, State: "unavailable", DownloadAllowed: f.allowDownload, ReleasePath: f.release}
	if command == "repair" {
		report.State = "unknown"
	}
	// preload always prepares a fresh root through managed storage, whatever the
	// configured mode.
	if (cfg.Mode == runtimehost.ModePreloaded || cfg.Mode == runtimehost.ModeNative) && command != "preload" {
		err = runFixedRuntimeCommand(ctx, command, cfg, &report)
	} else {
		err = runManagedCommand(ctx, command, cfg, f, &report, stderr)
	}
	return writeReport(stdout, report, err)
}

type adminFlags struct {
	configDir          string
	expectedRoot       string
	cancelOnInputClose bool
	release            string
	manualUpdates      bool
	cache              string
	root               string
	archive            string
	allowDownload      bool
}

func parseAdminFlags(command string, args []string, stderr io.Writer) (adminFlags, error) {
	var f adminFlags
	flags := flag.NewFlagSet("runtime "+command, flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&f.configDir, "config-dir", "", "directory containing administrator dem.conf")
	flags.StringVar(&f.expectedRoot, "expected-root", "", "require this canonical managed root before ensure mutates storage")
	flags.BoolVar(&f.cancelOnInputClose, "cancel-on-stdin-close", false, "cancel ensure cooperatively when its private stdin control channel closes")
	flags.StringVar(&f.release, "release", "", "local signed release descriptor; otherwise use configured latest URL")
	flags.BoolVar(&f.manualUpdates, "manual-updates", false, "disable automatic daily update preparation (install/repair); activation always requires restart")
	flags.StringVar(&f.cache, "cache", "", "managed installation directory")
	flags.StringVar(&f.root, "root", "", "preload final root (must be new or empty; never move it)")
	flags.StringVar(&f.archive, "archive", "", "offline archive checked against the trusted release descriptor")
	flags.BoolVar(&f.allowDownload, "allow-download", false, "explicitly download canonical package-signing keys, signed release metadata and runtime bytes; install/repair enables automatic preparation unless --manual-updates")
	if err := flags.Parse(args); err != nil {
		return f, err
	}
	if flags.NArg() != 0 {
		return f, errors.New("unexpected positional runtime arguments")
	}
	return f, nil
}

// validateEnsureOnly checks the ensure-only flags, which precede every other
// check because ensure arms its stdin control channel next.
func (f adminFlags) validateEnsureOnly(command string) error {
	if command != "ensure" && (f.expectedRoot != "" || f.cancelOnInputClose) {
		return errors.New("--expected-root and --cancel-on-stdin-close are only valid for ensure")
	}
	if f.expectedRoot != "" {
		return validateSetupPlanPath(f.expectedRoot)
	}
	return nil
}

func (f adminFlags) validate(command string) error {
	if command == "ensure" && os.Geteuid() == 0 {
		return errors.New("runtime ensure must run as the nonroot DEM service account")
	}
	if f.configDir != "" && !filepath.IsAbs(f.configDir) {
		return errors.New("--config-dir must be absolute")
	}
	if preparesCandidate(command) {
		if f.release != "" && !filepath.IsAbs(f.release) {
			return errors.New("--release must be absolute")
		}
		if (f.archive != "") == f.allowDownload {
			return errors.New("provide either --archive or --allow-download for ensure/install/update/repair/preload")
		}
		if f.archive != "" && f.release == "" {
			return errors.New("offline archive requires an absolute --release path")
		}
	} else if f.release != "" || f.archive != "" || f.allowDownload {
		return errors.New("--release, --archive and --allow-download are only valid for ensure/install/update/repair/preload")
	}
	if f.manualUpdates && command != "install" && command != "repair" {
		return errors.New("--manual-updates is only valid for install/repair; update preserves the installed policy")
	}
	if f.archive != "" && !filepath.IsAbs(f.archive) {
		return errors.New("--archive must be absolute")
	}
	if command == "preload" {
		if !filepath.IsAbs(f.root) {
			return errors.New("preload requires an absolute --root final directory")
		}
		if f.cache != "" {
			return errors.New("preload uses --root, not --cache")
		}
	} else if f.root != "" {
		return errors.New("--root is only valid for preload")
	}
	return nil
}

// preparesCandidate reports whether the command acquires and qualifies a release.
func preparesCandidate(command string) bool {
	switch command {
	case "ensure", "install", "update", "repair", "preload":
		return true
	}
	return false
}

type runtimeCommandReport struct {
	Outcome         string                     `json:"outcome,omitempty"`
	DownloadAllowed bool                       `json:"download_allowed"`
	Command         string                     `json:"command"`
	Mode            runtimehost.Mode           `json:"mode"`
	State           string                     `json:"state"`
	ReleasePath     string                     `json:"release_path,omitempty"`
	CachePath       string                     `json:"cache_path,omitempty"`
	Generation      string                     `json:"generation,omitempty"`
	Runtime         *synthetic.RuntimeIdentity `json:"runtime,omitempty"`
	Candidate       *synthetic.RuntimeIdentity `json:"candidate,omitempty"`
	CandidateState  string                     `json:"candidate_state,omitempty"`
	SetupError      string                     `json:"setup_error,omitempty"`
	Storage         *runtime.Status            `json:"storage,omitempty"`
	StorageError    string                     `json:"storage_error,omitempty"`
	Native          *runtime.NativeStatus      `json:"native,omitempty"`
	Qualification   *runner.Qualification      `json:"qualification,omitempty"`
	PreloadedConfig map[string]any             `json:"preloaded_config,omitempty"`
	PreloadPath     string                     `json:"preload_path,omitempty"`
	Notice          string                     `json:"notice,omitempty"`
}

// setRuntime reports a runtime with its generation, the archive digest.
func (r *runtimeCommandReport) setRuntime(identity synthetic.RuntimeIdentity) {
	r.Runtime = &identity
	r.Generation = identity.SHA256
}

// inventory records the store inventory and its error in the report's storage
// fields. Only status treats the error as its own failure.
func (r *runtimeCommandReport) inventory(store *runtime.Store) (runtime.Status, error) {
	status, err := store.Inspect()
	r.Storage = &status
	r.StorageError = errorString(err)
	return status, err
}

// selectedInstallation reports executable selection independently of a candidate
// and of damage to unrelated retained generations.
func (r *runtimeCommandReport) selectedInstallation(status runtime.Status) {
	r.Runtime = nil
	r.Generation = ""
	r.State = "unavailable"
	if status.Selected != nil {
		r.setRuntime(status.Selected.Identity())
		r.State = "installed"
	}
	if status.SelectionError != "" {
		r.State = "damaged"
	}
	if status.InventoryError != "" {
		r.State = "unknown"
	}
}

// writeReport records a command failure in the report and prints it. It owns
// the failed ensure outcome, including a close failure after a selection.
func writeReport(stdout io.Writer, report runtimeCommandReport, err error) error {
	if err != nil {
		if report.Command == "ensure" {
			report.Outcome = "failed"
		}
		report.SetupError = err.Error()
	}
	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	return errors.Join(err, encoder.Encode(report))
}

func errorString(err error) string {
	if err != nil {
		return err.Error()
	}
	return ""
}

// closeUnlessUndrained joins the close error of a resource held by a command.
// An undrained process tree keeps its candidate, consumer or proof protected
// until process exit, so the resource stays open.
func closeUnlessUndrained(err error, resource io.Closer) error {
	if errors.Is(err, ndexec.ErrTreeNotDrained) {
		return err
	}
	return errors.Join(err, resource.Close())
}

// varLibDir returns the raw Agent state directory (environment, then build-time
// paths); plugin configuration is not initialized for runtime commands, and
// setup-plan validates this value before it is cleaned.
func varLibDir() string {
	if dir := os.Getenv("NETDATA_LIB_DIR"); dir != "" {
		return dir
	}
	if buildinfo.VarLibDir != "" {
		return buildinfo.VarLibDir
	}
	return buildinfo.DefaultVarLibDir
}
