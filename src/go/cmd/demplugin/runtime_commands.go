// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/runner"
	runtime "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/runtime"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/runtimehost"
)

const unchangedReleaseNotice = "Selected release is unchanged. Already running processes keep their startup release until restart."

// releaseLoader returns publisher-authenticated release metadata; executable
// candidates are never begun without it.
type releaseLoader = func(context.Context) (runtime.VerifiedRelease, error)

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
	return qualifyInstalled(ctx, handle.Source, report)
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
	if !preparesCandidate(command) {
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
	load := cfg.ReleaseLoader(f.release, f.allowDownload)
	switch command {
	case "status":
		err = reportStatus(store, report)
	case "uninstall":
		err = uninstallRuntime(ctx, store, report)
	case "qualify":
		err = qualifySelected(ctx, store, report)
	case "ensure":
		err = ensureRuntime(ctx, store, load, f.archive, report, stderr)
	case "preload":
		err = preloadRuntime(ctx, store, storeConfig.Path, load, f.archive, report)
	default:
		err = installRuntime(ctx, command, store, load, f.archive, f.manualUpdates, report, stderr)
	}
	return closeUnlessUndrained(err, store)
}

func reportStatus(store *runtime.Store, report *runtimeCommandReport) error {
	status, err := report.inventory(store)
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
	err := store.Uninstall(ctx)
	if err == nil {
		report.State = "uninstalled"
		report.Notice = "Runtime files removed. This command does not change administrator-owned host policy; netdata-dem-runtime.sh handles high-level policy cleanup."
	}
	_, _ = report.inventory(store)
	return err
}

// qualifySelected qualifies the selected generation, then reports the store
// inventory after releasing it.
func qualifySelected(ctx context.Context, store *runtime.Store, report *runtimeCommandReport) error {
	consumer, err := store.AcquireSelected()
	if err == nil {
		report.setRuntime(consumer.Manifest().Identity())
		report.State = "installed"
		err = closeUnlessUndrained(qualifyInstalled(ctx, runtimehost.Borrow(consumer), report), consumer)
	}
	_, _ = report.inventory(store)
	return err
}

// qualifyInstalled qualifies an installed runtime in a standalone execution
// scope and reports it qualified when every check passes.
func qualifyInstalled(ctx context.Context, source runner.Source, report *runtimeCommandReport) error {
	qualification, err := runner.Qualify(ctx, source)
	report.Qualification = qualification
	if err != nil {
		return err
	}
	report.State = "qualified"
	return nil
}

// ensureRuntime enrolls a runtime only when none is selected; a usable selection
// is reported without download or qualification. ensure never reports store
// inventory.
func ensureRuntime(ctx context.Context, store *runtime.Store, load releaseLoader, archive string, report *runtimeCommandReport, stderr io.Writer) error {
	candidate, selected, err := store.BeginPublishedEnsure(ctx, func(ctx context.Context) (runtime.VerifiedRelease, error) {
		fmt.Fprintln(stderr, "First runtime enrollment acquires a signed release and enables daily automatic update preparation. Running DEM processes require a restart to activate the selection.")
		return load(ctx)
	})
	if err != nil {
		return err
	}
	if selected != nil {
		report.setRuntime(selected.Identity())
		report.State = "installed"
		report.Outcome = "already-selected"
		report.Notice = "A usable runtime is already selected; no download, qualification or update policy change was performed. A new DEM process uses this selection. Running processes may require a restart; their active release was not inspected."
		return nil
	}
	if candidate == nil {
		// A recorded release always comes back as the selection, so ensure never
		// sees the unchanged-release result.
		return errors.New("runtime ensure began neither a candidate nor a selection")
	}
	err = selectCandidate(ctx, candidate, archive, report)
	if err == nil {
		report.Outcome = "newly-selected"
		report.setRuntime(candidate.Manifest().Identity())
	}
	return closeUnlessUndrained(err, candidate)
}

// installRuntime installs, updates or repairs the selection. Its report shows
// the selected installation separately from candidate progress.
func installRuntime(ctx context.Context, command string, store *runtime.Store, load releaseLoader, archive string, manualUpdates bool, report *runtimeCommandReport, stderr io.Writer) error {
	err := installCandidate(ctx, command, store, load, archive, manualUpdates, report, stderr)
	status, _ := report.inventory(store)
	report.selectedInstallation(status)
	if err != nil && command == "repair" {
		report.Notice = "Repair failed; selected installation state is shown separately from candidate progress. A cleared selection is not restored. Stop and drain all consumers before retrying repair."
	}
	return err
}

func installCandidate(ctx context.Context, command string, store *runtime.Store, load releaseLoader, archive string, manualUpdates bool, report *runtimeCommandReport, stderr io.Writer) error {
	options := runtime.ReleaseInstallOptions{Operation: runtime.ReleaseOperation(command)}
	// install and repair set the update policy; update preserves the installed one.
	if command != "update" {
		options.Updates = runtime.AutomaticUpdates
		if manualUpdates {
			options.Updates = runtime.ManualUpdates
		} else {
			fmt.Fprintln(stderr, "Runtime installation enables daily automatic update preparation. Activation requires a DEM restart; use --manual-updates to disable automatic preparation.")
		}
	}
	candidate, err := store.BeginPublishedInstall(ctx, options, load)
	if err != nil {
		return err
	}
	if candidate == nil {
		report.Notice = unchangedReleaseNotice
		return nil
	}
	return closeUnlessUndrained(selectCandidate(ctx, candidate, archive, report), candidate)
}

// preloadRuntime prepares a candidate in a fresh final root and seals it there.
// A sealed root is no longer managed storage, so only an unsealed attempt
// reports the store inventory.
func preloadRuntime(ctx context.Context, store *runtime.Store, root string, load releaseLoader, archive string, report *runtimeCommandReport) error {
	sealed, err := sealPreload(ctx, store, root, load, archive, report)
	if !sealed {
		status, _ := report.inventory(store)
		report.selectedInstallation(status)
	}
	return err
}

// sealPreload begins a candidate without automatic updates, qualifies it and
// seals it as the preload root. sealed reports a completed seal even when
// closing the candidate fails afterwards.
func sealPreload(ctx context.Context, store *runtime.Store, root string, load releaseLoader, archive string, report *runtimeCommandReport) (sealed bool, err error) {
	options := runtime.ReleaseInstallOptions{Operation: runtime.InstallRelease, Updates: runtime.ManualUpdates}
	candidate, err := store.BeginPublishedInstall(ctx, options, load)
	if err != nil {
		return false, err
	}
	if candidate == nil {
		report.Notice = unchangedReleaseNotice
		return false, nil
	}
	err = qualifyCandidate(ctx, candidate, archive, report)
	if err == nil {
		err = candidate.SealPreload()
	}
	if err == nil {
		sealed = true
		report.setRuntime(candidate.Manifest().Identity())
		report.State = "installed"
		report.CachePath = ""
		report.PreloadPath = root
		report.PreloadedConfig = map[string]any{"runtime": map[string]any{"mode": runtimehost.ModePreloaded, "preload_path": root}}
		report.Notice = "Preload is qualified for this CLI user/environment and bound to this final root. Keep it immutable and private. Stop all consumers before replacement or removal; this is not a native package transaction. Restart DEM to activate it."
	}
	return sealed, closeUnlessUndrained(err, candidate)
}

// selectCandidate qualifies the candidate and selects it.
func selectCandidate(ctx context.Context, candidate *runtime.Candidate, archive string, report *runtimeCommandReport) error {
	if err := qualifyCandidate(ctx, candidate, archive, report); err != nil {
		return err
	}
	if err := candidate.Select(); err != nil {
		return err
	}
	report.CandidateState = "selected"
	report.State = "installed"
	report.Notice = "Runtime installed and selected. Running DEM processes keep their acquired release; restart DEM to activate this selection."
	return nil
}

// qualifyCandidate imports or downloads the candidate and qualifies it in a
// standalone execution scope. A cancellation after qualification stops the
// command before the candidate is published.
func qualifyCandidate(ctx context.Context, candidate *runtime.Candidate, archive string, report *runtimeCommandReport) error {
	report.Candidate = new(candidate.Manifest().Identity())
	report.CandidateState = "requested"
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
	return ctx.Err()
}
