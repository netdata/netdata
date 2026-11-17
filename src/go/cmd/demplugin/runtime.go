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
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/browserruntime"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/runner"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/runtimehost"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
)

// runRuntimeCommand runs `dem.plugin runtime <command> [flags]`; args start
// after "runtime". main routes these commands before cli.Parse, so they never
// enter the plugin protocol. --cancel-on-stdin-close borrows input as a private
// control channel; the caller retains ownership of input.
func runRuntimeCommand(ctx context.Context, args []string, input *os.File, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return errors.New(
			"usage: dem.plugin runtime {ensure|install|update|fetch|prepare|activate|rollback|repair|uninstall|preload|status|qualify|setup-plan} [flags]",
		)
	}
	command, flags := args[0], args[1:]
	switch {
	case command == "setup-plan":
		return runSetupPlan(flags, stdout, stderr)
	case command == "pair-plan":
		return runPairPlan(flags, stdout, stderr)
	case strings.HasPrefix(command, "native-"):
		return runNativeCommand(ctx, command, flags, stdout, stderr)
	}
	switch command {
	case "ensure", "install", "update", "fetch", "prepare", "activate", "rollback", "repair", "uninstall", "preload", "preload-validate", "status", "qualify":
		return runAdminCommand(ctx, command, flags, input, stdout, stderr)
	default:
		return fmt.Errorf("unknown runtime command %q", command)
	}
}

// runAdminCommand runs an administrator command. Once configuration loads, it
// prints exactly one JSON report, failures included.
func runAdminCommand(
	ctx context.Context,
	command string,
	args []string,
	input *os.File,
	stdout, stderr io.Writer,
) error {
	f, err := parseAdminFlags(command, args, stderr)
	if errors.Is(err, flag.ErrHelp) {
		return nil
	}
	if err != nil {
		return err
	}
	if command == "update" {
		return errors.New(runtimeUpdateGuidance)
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
	if f.keyring != "" {
		cfg.OfflineKeyring = f.keyring
	}
	if f.cache != "" {
		cfg.CachePath = f.cache
	}
	report := runtimeCommandReport{
		Command:         command,
		Mode:            cfg.Mode,
		State:           "unavailable",
		DownloadAllowed: f.allowDownload,
		ReleasePath:     f.release,
	}
	if command == "repair" {
		report.State = "unknown"
	}
	// preload always prepares a fresh root through managed storage, whatever the
	// configured mode.
	if command == "fetch" {
		var verified browserruntime.VerifiedRelease
		verified, err = cfg.ReleaseLoader(f.release, f.allowDownload)(ctx)
		if err == nil {
			var inputs browserruntime.ReleaseInputs
			inputs, err = verified.SaveInputs(ctx, runtimehost.AgentRelease(), f.output, f.archive)
			if err == nil {
				report.Inputs = &inputs
				report.State = "inputs-verified"
				report.Notice = "Authenticated runtime inputs retained. Selection is unchanged; the installation owner controls preparation and activation."
			}
		}
	} else if (cfg.Mode == runtimehost.ModePreloaded || cfg.Mode == runtimehost.ModeNative) && command != "preload" {
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
	output             string
	keyring            string
	preparedOnly       bool
}

const runtimeUpdateGuidance = "Runtime updates belong to the Agent installation owner. Update or reinstall the Agent through its installer, package manager or deployment workflow to acquire its matching runtime; runtime update does not change runtime storage."

func parseAdminFlags(command string, args []string, stderr io.Writer) (adminFlags, error) {
	var f adminFlags
	flags := flag.NewFlagSet("runtime "+command, flag.ContinueOnError)
	flags.SetOutput(stderr)
	if command == "update" {
		flags.Usage = func() { fmt.Fprintln(stderr, runtimeUpdateGuidance) }
	}
	flags.StringVar(&f.configDir, "config-dir", "", "directory containing administrator dem.conf")
	flags.StringVar(
		&f.expectedRoot,
		"expected-root",
		"",
		"require this canonical managed root before ensure mutates storage",
	)
	flags.BoolVar(
		&f.cancelOnInputClose,
		"cancel-on-stdin-close",
		false,
		"cancel ensure cooperatively when its private stdin control channel closes",
	)
	flags.StringVar(&f.release, "release", "", "local signed release descriptor; otherwise use this Agent's configured release URL")
	flags.BoolVar(
		&f.manualUpdates,
		"manual-updates",
		false,
		"require explicit runtime acquisition during Agent updates (install/repair)",
	)
	flags.StringVar(&f.cache, "cache", "", "managed installation directory")
	flags.StringVar(&f.root, "root", "", "preload final root (must be new or empty; never move it)")
	flags.StringVar(&f.archive, "archive", "", "offline archive checked against the trusted release descriptor")
	flags.StringVar(&f.output, "output", "", "new installer-owned directory for authenticated inputs (fetch only)")
	flags.StringVar(&f.keyring, "keyring", "", "explicit retained public keyring for offline fetch/prepare")
	flags.BoolVar(&f.preparedOnly, "prepared-only", false, "cancel only a pending preparation (rollback only); otherwise do nothing")
	flags.BoolVar(
		&f.allowDownload,
		"allow-download",
		false,
		"explicitly download canonical keys, signed metadata and runtime bytes; install/repair permits paired downloads unless --manual-updates",
	)
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
	if f.preparedOnly && command != "rollback" {
		return errors.New("--prepared-only is only valid for rollback")
	}
	if command == "ensure" && os.Geteuid() == 0 {
		return errors.New("runtime ensure must run as the nonroot DEM service account")
	}
	if f.configDir != "" && !filepath.IsAbs(f.configDir) {
		return errors.New("--config-dir must be absolute")
	}
	if f.keyring != "" && (!filepath.IsAbs(f.keyring) || f.allowDownload || (command != "fetch" && command != "prepare")) {
		return errors.New("--keyring requires an absolute path and offline fetch/prepare")
	}
	if preparesCandidate(command) {
		if f.release != "" && !filepath.IsAbs(f.release) {
			return errors.New("--release must be absolute")
		}
		if (f.archive != "") == f.allowDownload {
			return errors.New("provide either --archive or --allow-download for ensure/install/fetch/prepare/repair/preload")
		}
		if f.archive != "" && f.release == "" {
			return errors.New("offline archive requires an absolute --release path")
		}
	} else if f.release != "" || f.archive != "" || f.allowDownload {
		return errors.New("--release, --archive and --allow-download are only valid for ensure/install/fetch/prepare/repair/preload")
	}
	if command == "fetch" {
		if !filepath.IsAbs(f.output) || f.cache != "" {
			return errors.New("fetch requires an absolute new --output directory and does not use --cache")
		}
	} else if f.output != "" {
		return errors.New("--output is only valid for fetch")
	}
	if f.manualUpdates && command != "install" && command != "repair" {
		return errors.New("--manual-updates is only valid for install/repair")
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

// preparesCandidate reports whether the command acquires and verifies a release.
func preparesCandidate(command string) bool {
	switch command {
	case "ensure", "install", "fetch", "prepare", "repair", "preload":
		return true
	}
	return false
}

type runtimeCommandReport struct {
	Outcome         string                        `json:"outcome,omitempty"`
	DownloadAllowed bool                          `json:"download_allowed"`
	Command         string                        `json:"command"`
	Mode            runtimehost.Mode              `json:"mode"`
	State           string                        `json:"state"`
	ReleasePath     string                        `json:"release_path,omitempty"`
	CachePath       string                        `json:"cache_path,omitempty"`
	Generation      string                        `json:"generation,omitempty"`
	Runtime         *synthetic.RuntimeIdentity    `json:"runtime,omitempty"`
	Candidate       *synthetic.RuntimeIdentity    `json:"candidate,omitempty"`
	CandidateState  string                        `json:"candidate_state,omitempty"`
	SetupError      string                        `json:"setup_error,omitempty"`
	Storage         *browserruntime.Status        `json:"storage,omitempty"`
	StorageError    string                        `json:"storage_error,omitempty"`
	Inputs          *browserruntime.ReleaseInputs `json:"inputs,omitempty"`
	Native          *browserruntime.NativeStatus  `json:"native,omitempty"`
	Qualification   *runner.Qualification         `json:"qualification,omitempty"`
	PreloadedConfig map[string]any                `json:"preloaded_config,omitempty"`
	PreloadPath     string                        `json:"preload_path,omitempty"`
	Notice          string                        `json:"notice,omitempty"`
}

// setRuntime reports a runtime with its generation, the archive digest.
func (r *runtimeCommandReport) setRuntime(identity synthetic.RuntimeIdentity) {
	r.Runtime = &identity
	r.Generation = identity.SHA256
}

// inventory records the store inventory and its error in the report's storage
// fields. Only status treats the error as its own failure.
func (r *runtimeCommandReport) inventory(store *browserruntime.Store) (browserruntime.Status, error) {
	status, err := store.Inspect()
	r.Storage = &status
	r.StorageError = errorString(err)
	return status, err
}

// selectedInstallation reports executable selection independently of a candidate
// and of damage to unrelated retained generations.
func (r *runtimeCommandReport) selectedInstallation(status browserruntime.Status) {
	r.Runtime = nil
	r.Generation = ""
	r.State = "unavailable"
	if status.Release != nil {
		r.setRuntime(status.Release.Manifest.Identity())
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
