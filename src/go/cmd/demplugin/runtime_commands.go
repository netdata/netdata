// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/browserruntime"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/runner"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/runtimehost"
)

const unchangedReleaseNotice = "Selected release is unchanged. Already running processes keep their startup release until restart."

// releaseLoader returns publisher-authenticated release metadata; executable
// candidates are never begun without it.
type releaseLoader = func(context.Context) (browserruntime.VerifiedRelease, error)

// runFixedRuntimeCommand serves status and qualify for preloaded and native
// runtimes, whose files this CLI never changes.
func runFixedRuntimeCommand(
	ctx context.Context,
	command string,
	cfg runtimehost.Config,
	report *runtimeCommandReport,
) error {
	// Installation owners require an exact target pair, independently of the
	// compatibility checks used by ordinary status and execution.
	if command == "preload-validate" && cfg.Mode == runtimehost.ModePreloaded {
		_, manifest, err := browserruntime.ReadPreload(cfg.PreloadPath, new(runtimehost.AgentRelease()))
		if err == nil {
			report.setRuntime(manifest.Identity())
			report.PreloadPath = cfg.PreloadPath
			report.State = "installed"
		}
		return err
	}
	if command != "status" && command != "qualify" {
		if cfg.Mode == runtimehost.ModeNative {
			return errors.New(
				"native runtime installation, update, repair and removal belong to the package manager; install the optional netdata-dem-runtime package",
			)
		}
		return errors.New("install/update/repair/uninstall require runtime.mode: managed")
	}
	handle := runtimehost.Open(cfg, "")
	return closeUnlessUndrained(inspectFixedRuntime(ctx, command, cfg, handle, report), handle)
}

func inspectFixedRuntime(
	ctx context.Context,
	command string,
	cfg runtimehost.Config,
	handle *runtimehost.Handle,
	report *runtimeCommandReport,
) error {
	if handle.SetupError != nil {
		if errors.Is(handle.SetupError, browserruntime.ErrIncompatible) {
			report.State = "incompatible"
			report.Notice = "The installed runtime is incompatible with this plugin's execution contract or lacks required provenance. Install or restore a compatible runtime through the installation owner."
		}
		return handle.SetupError
	}
	// Without a setup error the handle knows the release it runs.
	report.setRuntime(handle.Identity())
	if cfg.Mode == runtimehost.ModeNative {
		report.Native = handle.NativeStatus()
		report.Notice = "Native package files are owned by the package manager. Stop standalone consumers before package replacement or removal."
	} else {
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
func runManagedCommand(
	ctx context.Context,
	command string,
	cfg runtimehost.Config,
	f adminFlags,
	report *runtimeCommandReport,
	stderr io.Writer,
) error {
	if command == "preload-validate" {
		return errors.New("preload-validate requires runtime.mode: preloaded")
	}
	if command == "preload" {
		entries, err := os.ReadDir(f.root)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if len(entries) != 0 {
			return errors.New(
				"preload root must be empty; use a new final root and stop all consumers before replacement",
			)
		}
		cfg.CachePath = f.root
	}
	storeConfig, err := cfg.StoreConfig(varLibDir())
	if err != nil {
		return err
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
	store, err := browserruntime.Open(storeConfig)
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
	case "activate", "rollback":
		if command == "activate" {
			err = store.ActivatePair(ctx)
			report.Notice = "Selected runtime pair verified and any pending activation completed. The installation owner must start the matching Agent."
		} else if f.preparedOnly {
			err = store.CancelPreparation(ctx)
			report.Notice = "Pending preparation canceled, if present. The installation owner must ensure the selected runtime matches the Agent before restarting."
		} else {
			err = store.RollbackPair(ctx)
			report.Notice = "Previous runtime selected. Restore its matching Agent through its installation owner before restarting."
		}
		status, _ := report.inventory(store)
		report.selectedInstallation(status)
	case "qualify":
		err = qualifySelected(ctx, store, report)
	case "preload":
		err = preloadRuntime(ctx, store, storeConfig.Path, load, f.archive, report)
	default:
		err = installRuntime(ctx, command, store, load, f.archive, f.manualUpdates, report, stderr)
	}
	return closeUnlessUndrained(err, store)
}

func reportStatus(store *browserruntime.Store, report *runtimeCommandReport) error {
	status, err := report.inventory(store)
	report.selectedInstallation(status)
	report.Notice = "Selected installation is shown; already running DEM processes may retain an older release until restart. Metadata damage requires explicit repair after stopping and draining consumers."
	if err != nil {
		return err
	}
	if status.Release == nil {
		return browserruntime.ErrUnavailable
	}
	consumer, err := store.AcquireSelected()
	if err != nil {
		if errors.Is(err, browserruntime.ErrIncompatible) {
			report.State = "incompatible"
			report.Notice = "The selected runtime is incompatible with this plugin's execution contract. Install or restore a compatible runtime through the installation owner."
		}
		return err
	}
	return consumer.Close()
}

func uninstallRuntime(ctx context.Context, store *browserruntime.Store, report *runtimeCommandReport) error {
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
func qualifySelected(ctx context.Context, store *browserruntime.Store, report *runtimeCommandReport) error {
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

// installRuntime installs, updates or repairs the selection. Its report shows
// the selected installation separately from candidate progress.
func installRuntime(
	ctx context.Context,
	command string,
	store *browserruntime.Store,
	load releaseLoader,
	archive string,
	manualUpdates bool,
	report *runtimeCommandReport,
	stderr io.Writer,
) error {
	err := installCandidate(ctx, command, store, load, archive, manualUpdates, report, stderr)
	status, _ := report.inventory(store)
	report.selectedInstallation(status)
	if err != nil && command == "repair" {
		report.Notice = "Repair failed; selected installation state is shown separately from candidate progress. A cleared selection is not restored. Stop and drain all consumers before retrying repair."
	}
	return err
}

func installCandidate(
	ctx context.Context,
	command string,
	store *browserruntime.Store,
	load releaseLoader,
	archive string,
	manualUpdates bool,
	report *runtimeCommandReport,
	stderr io.Writer,
) error {
	operation, err := releaseOperation(command)
	if err != nil {
		return err
	}
	options := browserruntime.ReleaseInstallOptions{
		Operation: operation,
	}
	// Install and repair set the update policy; preparation preserves it.
	if operation != browserruntime.PrepareRelease {
		options.Updates = browserruntime.AutomaticUpdates
		if manualUpdates {
			options.Updates = browserruntime.ManualUpdates
		} else {
			fmt.Fprintln(stderr, "Runtime installation permits companion downloads during Agent updates. First activation requires a DEM restart; use --manual-updates to require explicit acquisition.")
		}
	}
	candidate, err := store.BeginPublishedInstall(ctx, options, load)
	if err != nil {
		return err
	}
	if candidate == nil {
		report.Notice = unchangedReleaseNotice
		report.Outcome = "unchanged"
		return nil
	}
	if operation == browserruntime.PrepareRelease {
		err = prepareCandidate(ctx, candidate, archive, report)
		if err == nil {
			err = candidate.PreparePair()
		}
		if err == nil {
			report.Outcome = "prepared"
			report.Notice = "Matching target is retained without changing selection. The installation owner must stop consumers, install the matching Agent and activate the pair."
		}
		return closeUnlessUndrained(err, candidate)
	}
	return closeUnlessUndrained(selectCandidate(ctx, candidate, archive, report), candidate)
}

// releaseOperation names the store operation of the commands that install a
// published release; preload has its own entry point.
func releaseOperation(command string) (browserruntime.ReleaseOperation, error) {
	switch command {
	case "install":
		return browserruntime.InstallRelease, nil
	case "prepare":
		return browserruntime.PrepareRelease, nil
	case "repair":
		return browserruntime.RepairRelease, nil
	}
	return "", fmt.Errorf("runtime command %q does not install a published release", command)
}

// preloadRuntime prepares a candidate in a fresh final root and seals it there.
// A sealed root is no longer managed storage, so only an unsealed attempt
// reports the store inventory.
func preloadRuntime(
	ctx context.Context,
	store *browserruntime.Store,
	root string,
	load releaseLoader,
	archive string,
	report *runtimeCommandReport,
) error {
	sealed, err := sealPreload(ctx, store, root, load, archive, report)
	if !sealed {
		status, _ := report.inventory(store)
		report.selectedInstallation(status)
	}
	return err
}

// sealPreload begins a candidate without automatic updates, verifies it and
// seals it as the preload root. sealed reports a completed seal even when
// closing the candidate fails afterwards.
func sealPreload(
	ctx context.Context,
	store *browserruntime.Store,
	root string,
	load releaseLoader,
	archive string,
	report *runtimeCommandReport,
) (sealed bool, err error) {
	options := browserruntime.ReleaseInstallOptions{
		Operation: browserruntime.InstallRelease,
		Updates:   browserruntime.ManualUpdates,
	}
	candidate, err := store.BeginPublishedInstall(ctx, options, load)
	if err != nil {
		return false, err
	}
	if candidate == nil {
		report.Notice = unchangedReleaseNotice
		return false, nil
	}
	err = prepareCandidate(ctx, candidate, archive, report)
	if err == nil {
		err = candidate.SealPreload()
	}
	if err == nil {
		sealed = true
		report.setRuntime(candidate.Manifest().Identity())
		report.State = "installed"
		report.CachePath = ""
		report.PreloadPath = root
		report.PreloadedConfig = map[string]any{
			"runtime": map[string]any{"mode": runtimehost.ModePreloaded, "preload_path": root},
		}
		report.Notice = "Preload integrity is verified and bound to this final root. Browser execution has not been tested; use runtime qualify for an optional local diagnostic. Keep it immutable and private. Stop all consumers before replacement or removal; this is not a native package transaction. Restart DEM to activate it."
	}
	return sealed, closeUnlessUndrained(err, candidate)
}

// selectCandidate verifies the candidate and selects it.
func selectCandidate(
	ctx context.Context,
	candidate *browserruntime.Candidate,
	archive string,
	report *runtimeCommandReport,
) error {
	if err := prepareCandidate(ctx, candidate, archive, report); err != nil {
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

// prepareCandidate imports or downloads authenticated bytes and verifies their
// bounds, layout and final-path binding without executing the browser. A
// cancellation stops the command before the candidate is published.
func prepareCandidate(
	ctx context.Context,
	candidate *browserruntime.Candidate,
	archive string,
	report *runtimeCommandReport,
) error {
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
	return ctx.Err()
}
