// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/runner"
	runtime "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/runtime"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/runtimehost"
)

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
