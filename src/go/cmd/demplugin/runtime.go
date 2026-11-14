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

// runRuntimeCommand handles `dem.plugin runtime ...` before cli.Parse; these
// commands never enter the plugin protocol. Executable candidates require
// publisher-authenticated release metadata. --cancel-on-stdin-close borrows
// input as a private control channel; the caller retains ownership of input.
func runRuntimeCommand(ctx context.Context, args []string, input *os.File, stdout, stderr io.Writer) (bool, error) {
	if len(args) == 0 || args[0] != "runtime" {
		return false, nil
	}
	if len(args) < 2 {
		return true, errors.New("usage: dem.plugin runtime {ensure|install|update|repair|uninstall|preload|status|qualify|setup-plan} [flags]")
	}
	command, flags := args[1], args[2:]
	switch {
	case command == "setup-plan":
		return true, runSetupPlan(flags, stdout, stderr)
	case strings.HasPrefix(command, "native-"):
		return true, runNativeCommand(ctx, command, flags, stdout, stderr)
	}
	switch command {
	case "ensure", "install", "update", "repair", "uninstall", "preload", "status", "qualify":
		return true, runAdminCommand(ctx, command, flags, input, stdout, stderr)
	default:
		return true, fmt.Errorf("unknown runtime command %q", command)
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
	if command == "ensure" {
		report.Outcome = "failed"
	}
	if command == "repair" {
		report.State = "unknown"
	}
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

// runFixedRuntimeCommand serves status and qualify for preloaded and native
// runtimes, whose files this CLI never changes.
func runFixedRuntimeCommand(ctx context.Context, command string, cfg runtimehost.Config, report *runtimeCommandReport) error {
	if command != "status" && command != "qualify" {
		if cfg.Mode == runtimehost.ModeNative {
			return errors.New("native runtime installation, update, repair and removal belong to the package manager; install the optional netdata-dem-runtime package")
		}
		return errors.New("install/update/repair/uninstall require runtime.mode: managed")
	}
	handle := runtimehost.Open(cfg, "")
	return closeUnlessUndrained(inspectFixedRuntime(ctx, command, cfg, handle, report), handle)
}

func inspectFixedRuntime(ctx context.Context, command string, cfg runtimehost.Config, handle *runtimehost.Handle, report *runtimeCommandReport) error {
	if handle.SetupError != nil {
		return handle.SetupError
	}
	// Without a setup error the handle is ready: native hosts know their running
	// identity and preloaded handles their manifest.
	if cfg.Mode == runtimehost.ModeNative {
		status := handle.Host.Status()
		report.setRuntime(*status.Running)
		report.Native = status.Native
		report.Notice = "Native package files are owned by the package manager. Stop standalone consumers before package replacement or removal."
	} else {
		report.setRuntime(handle.Manifest.Identity())
		report.PreloadPath = cfg.PreloadPath
	}
	report.State = "installed"
	if command != "qualify" {
		return nil
	}
	qualification, err := runner.Qualify(ctx, handle.Source)
	report.Qualification = qualification
	if err != nil {
		return err
	}
	report.State = "qualified"
	return nil
}

// runManagedCommand runs a command against managed runtime storage; preload
// prepares a fresh final root through the same store.
func runManagedCommand(ctx context.Context, command string, cfg runtimehost.Config, f adminFlags, report *runtimeCommandReport, stderr io.Writer) error {
	if command == "preload" {
		entries, err := os.ReadDir(f.root)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if len(entries) != 0 {
			return errors.New("preload root must be empty; use a new final root and stop all consumers before replacement")
		}
		cfg.CachePath = f.root
	}
	storeConfig, err := cfg.StoreConfig(varLibDir())
	if err != nil {
		return err
	}
	if command == "ensure" {
		canonical, err := runtime.CanonicalRoot(storeConfig.Path)
		if err != nil {
			return err
		}
		if f.expectedRoot != "" && canonical != filepath.Clean(f.expectedRoot) {
			return fmt.Errorf("configured canonical runtime root %q differs from --expected-root %q; no runtime storage changed", canonical, f.expectedRoot)
		}
		storeConfig.Path = canonical
	}
	report.CachePath = storeConfig.Path
	if err := ctx.Err(); err != nil {
		return err
	}
	preparing := preparesCandidate(command)
	if !preparing {
		if _, err := os.Stat(storeConfig.Path); err != nil {
			if command == "status" && !errors.Is(err, os.ErrNotExist) {
				report.State = "unknown"
			}
			return fmt.Errorf("runtime installation: %w", err)
		}
	}
	store, err := runtime.Open(storeConfig)
	if err != nil {
		if command == "status" {
			report.State = "unknown"
		}
		return err
	}
	sealed := false
	switch command {
	case "status":
		err = reportStatus(store, report)
	case "uninstall":
		err = uninstallRuntime(ctx, store, report)
	case "qualify":
		err = qualifySelected(ctx, store, report)
	default:
		sealed, err = prepareRuntime(ctx, command, store, storeConfig.Path, cfg, f, report, stderr)
	}
	// Status reports its own inspection, ensure reports no inventory, and a sealed
	// preload root is no longer managed storage. The rest report storage after
	// their candidate or consumer closed.
	if command != "status" && command != "ensure" && !sealed {
		status, inspectErr := store.Inspect()
		report.Storage = &status
		report.StorageError = errorString(inspectErr)
		if preparing {
			report.selectedInstallation(status)
			if err != nil && command == "repair" {
				report.Notice = "Repair failed; selected installation state is shown separately from candidate progress. A cleared selection is not restored. Stop and drain all consumers before retrying repair."
			}
		}
	}
	return closeUnlessUndrained(err, store)
}

// prepareRuntime runs one candidate transaction: begin, acquire, qualify, then
// select the candidate or seal it as the preload root at path. sealed reports
// a completed seal, even when closing the candidate fails afterwards.
func prepareRuntime(ctx context.Context, command string, store *runtime.Store, path string, cfg runtimehost.Config, f adminFlags, report *runtimeCommandReport, stderr io.Writer) (sealed bool, err error) {
	candidate, selected, err := beginCandidate(ctx, command, store, cfg, f, stderr)
	if err != nil {
		return false, err
	}
	if selected != nil {
		report.setRuntime(selected.Identity())
		report.State = "installed"
		report.Outcome = "already-selected"
		report.Notice = "A usable runtime is already selected; no download, qualification or update policy change was performed. A new DEM process uses this selection. Running processes may require a restart; their active release was not inspected."
		return false, nil
	}
	if candidate == nil {
		report.Notice = "Selected release is unchanged. Already running processes keep their startup release until restart."
		return false, nil
	}
	identity := candidate.Manifest().Identity()
	report.Candidate = &identity
	report.CandidateState = "requested"
	err = finishCandidate(ctx, command, candidate, identity, path, f.archive, report)
	sealed = command == "preload" && err == nil
	return sealed, closeUnlessUndrained(err, candidate)
}

// beginCandidate starts the transaction. ensure returns the selected manifest
// instead when a usable selection exists; no candidate and no selection means
// the selected release already satisfies the command.
func beginCandidate(ctx context.Context, command string, store *runtime.Store, cfg runtimehost.Config, f adminFlags, stderr io.Writer) (*runtime.Candidate, *runtime.Manifest, error) {
	load := cfg.ReleaseLoader(f.release, f.allowDownload)
	if command == "ensure" {
		return store.BeginPublishedEnsure(ctx, func(ctx context.Context) (runtime.VerifiedRelease, error) {
			fmt.Fprintln(stderr, "First runtime enrollment acquires a signed release and enables daily automatic update preparation. Running DEM processes require a restart to activate the selection.")
			return load(ctx)
		})
	}
	options := runtime.ReleaseInstallOptions{Operation: runtime.ReleaseOperation(command)}
	if command == "preload" {
		options.Operation = runtime.InstallRelease
	}
	if options.Operation == runtime.InstallRelease || options.Operation == runtime.RepairRelease {
		options.Updates = runtime.AutomaticUpdates
		if f.manualUpdates || command == "preload" {
			options.Updates = runtime.ManualUpdates
		}
		if options.Updates == runtime.AutomaticUpdates {
			fmt.Fprintln(stderr, "Runtime installation enables daily automatic update preparation. Activation requires a DEM restart; use --manual-updates to disable automatic preparation.")
		}
	}
	candidate, err := store.BeginPublishedInstall(ctx, options, load)
	return candidate, nil, err
}

// finishCandidate imports or downloads the candidate, qualifies it in a
// standalone execution scope, and then selects it or seals it as a preload root.
func finishCandidate(ctx context.Context, command string, candidate *runtime.Candidate, identity synthetic.RuntimeIdentity, path, archive string, report *runtimeCommandReport) error {
	var err error
	if archive != "" {
		err = candidate.Import(ctx, archive)
	} else {
		err = candidate.Download(ctx)
	}
	if err != nil {
		return err
	}
	report.CandidateState = "prepared"
	report.Qualification, err = runner.Qualify(ctx, runtimehost.Borrow(candidate))
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if command == "preload" {
		if err := candidate.SealPreload(); err != nil {
			return err
		}
		report.setRuntime(identity)
		report.CachePath = ""
		report.PreloadPath = path
		report.PreloadedConfig = map[string]any{"runtime": map[string]any{"mode": runtimehost.ModePreloaded, "preload_path": path}}
		report.Notice = "Preload is qualified for this CLI user/environment and bound to this final root. Keep it immutable and private. Stop all consumers before replacement or removal; this is not a native package transaction. Restart DEM to activate it."
	} else {
		if err := candidate.Select(); err != nil {
			return err
		}
		report.CandidateState = "selected"
		if command == "ensure" {
			report.Outcome = "newly-selected"
			report.setRuntime(identity)
		}
		report.Notice = "Runtime installed and selected. Running DEM processes keep their acquired release; restart DEM to activate this selection."
	}
	report.State = "installed"
	return nil
}

func reportStatus(store *runtime.Store, report *runtimeCommandReport) error {
	status, err := store.Inspect()
	report.Storage = &status
	report.StorageError = errorString(err)
	report.selectedInstallation(status)
	report.Notice = "Selected installation is shown; already running DEM processes may retain an older release until restart. Metadata damage requires explicit repair after stopping and draining consumers."
	if err != nil {
		return err
	}
	if status.Selected == nil {
		return runtime.ErrUnavailable
	}
	return nil
}

func uninstallRuntime(ctx context.Context, store *runtime.Store, report *runtimeCommandReport) error {
	if err := store.Uninstall(ctx); err != nil {
		return err
	}
	report.State = "uninstalled"
	report.Notice = "Runtime files removed. This command does not change administrator-owned host policy; netdata-dem-runtime.sh handles high-level policy cleanup."
	return nil
}

// qualifySelected qualifies the selected generation in a standalone execution scope.
func qualifySelected(ctx context.Context, store *runtime.Store, report *runtimeCommandReport) error {
	consumer, err := store.AcquireSelected()
	if err != nil {
		return err
	}
	report.setRuntime(consumer.Manifest().Identity())
	report.State = "installed"
	report.Qualification, err = runner.Qualify(ctx, runtimehost.Borrow(consumer))
	if err == nil {
		report.State = "qualified"
	}
	return closeUnlessUndrained(err, consumer)
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

// writeReport records a command failure in the report and prints it.
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

// varLibDir resolves the Agent state directory the same way before plugin
// configuration is initialized: environment, then build-time paths.
func varLibDir() string {
	if dir := os.Getenv("NETDATA_LIB_DIR"); dir != "" {
		return dir
	}
	if buildinfo.VarLibDir != "" {
		return buildinfo.VarLibDir
	}
	return buildinfo.DefaultVarLibDir
}
