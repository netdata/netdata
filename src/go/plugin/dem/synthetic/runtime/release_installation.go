// SPDX-License-Identifier: GPL-3.0-or-later

package runtime

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// ReleaseOperation distinguishes an explicit installation/repair decision from
// checks of an existing installation. Repair may deliberately select an older
// authenticated release; normal checks may never regress publication order.
type ReleaseOperation string

const (
	InstallRelease         ReleaseOperation = "install"
	ensureRelease          ReleaseOperation = "ensure"
	UpdateRelease          ReleaseOperation = "update"
	AutomaticReleaseUpdate ReleaseOperation = "automatic-update"
	RepairRelease          ReleaseOperation = "repair"
)

type UpdatePolicy string

const (
	AutomaticUpdates UpdatePolicy = "automatic"
	ManualUpdates    UpdatePolicy = "manual"
)

type ReleaseInstallOptions struct {
	Operation ReleaseOperation
	// Required for install and repair. Update operations preserve consent.
	Updates UpdatePolicy
}

func (s selection) validate() error {
	if s.Release == nil {
		if s.AutomaticUpdates {
			return errors.New("automatic runtime updates require an authenticated release")
		}
		return nil
	}
	return s.Release.Validate()
}

// BeginPublishedInstall takes installer exclusion BEFORE reading eligibility or
// calling load. The caller must authenticate load's result with VerifyRelease.
// Discovery runs outside inventory exclusion, so active monitoring continues.
// The returned candidate owns installer exclusion through qualification and
// publication. A nil candidate is a successful no-op: disabled automatic updates
// or an unchanged release. No download can resurrect an uninstalled selection.
func (s *Store) BeginPublishedInstall(ctx context.Context, options ReleaseInstallOptions, load func(context.Context) (Release, error)) (*Candidate, error) {
	candidate, _, err := s.beginPublishedInstall(ctx, options, load)
	return candidate, err
}

// BeginPublishedEnsure enrolls only a proven absent selection. A usable selected
// generation is checked under installer exclusion without discovery, inventory,
// qualification or changing its update policy. The returned manifest is a
// snapshot; exclusion is released before return. Damage requires explicit repair.
// A new candidate enables automatic preparation only when successfully selected.
func (s *Store) BeginPublishedEnsure(ctx context.Context, load func(context.Context) (Release, error)) (*Candidate, *Manifest, error) {
	return s.beginPublishedInstall(ctx, ReleaseInstallOptions{Operation: ensureRelease, Updates: AutomaticUpdates}, load)
}

func (s *Store) beginPublishedInstall(ctx context.Context, options ReleaseInstallOptions, load func(context.Context) (Release, error)) (_ *Candidate, _ *Manifest, result error) {
	switch options.Operation {
	case InstallRelease, RepairRelease, ensureRelease:
		if options.Updates != AutomaticUpdates && options.Updates != ManualUpdates {
			return nil, nil, errors.New("runtime installation requires an explicit update policy")
		}
	case UpdateRelease, AutomaticReleaseUpdate:
		if options.Updates != "" {
			return nil, nil, errors.New("runtime update preserves the installed update policy")
		}
	default:
		return nil, nil, errors.New("invalid published runtime operation")
	}
	if load == nil {
		return nil, nil, errors.New("runtime release loader is unavailable")
	}
	operation, err := s.lockInstallation(ctx)
	if err != nil {
		return nil, nil, err
	}
	transferred := false
	defer func() {
		if !transferred {
			result = errors.Join(result, s.releaseOwner(operation))
		}
	}()
	var current selection
	err = s.withLock(func() error {
		// Repair is an explicit drained operation and can recover damaged
		// selection metadata. Its caller supplies the new consent policy.
		if options.Operation == RepairRelease {
			return nil
		}
		if _, err := s.selectedLocked(true); err != nil {
			return err
		}
		var err error
		current, err = s.readSelectionRecord()
		if m := current.manifest(); err == nil && options.Operation == ensureRelease && m != nil {
			err = s.validateEnsureSelection(*m)
		}
		return err
	})
	if err != nil {
		return nil, nil, err
	}
	if m := current.manifest(); options.Operation == ensureRelease && m != nil {
		return nil, m, nil
	}
	if options.Operation == AutomaticReleaseUpdate && !current.AutomaticUpdates {
		return nil, nil, nil
	}
	if options.Operation == UpdateRelease && current.Release == nil {
		return nil, nil, errors.New("install an authenticated runtime release before checking updates")
	}
	release, err := load(ctx)
	if err != nil {
		return nil, nil, err
	}
	if err := release.Validate(); err != nil {
		return nil, nil, err
	}
	next := selection{Release: &release, AutomaticUpdates: current.AutomaticUpdates}
	if options.Operation == InstallRelease || options.Operation == RepairRelease || options.Operation == ensureRelease {
		next.AutomaticUpdates = options.Updates == AutomaticUpdates
	}
	if current.Release != nil {
		changed, err := releaseUpdate(*current.Release, release)
		if err != nil {
			return nil, nil, err
		}
		if !changed {
			// Reinstalling the exact release can explicitly change consent
			// without acquiring or executing unchanged payload bytes.
			if current.AutomaticUpdates != next.AutomaticUpdates {
				err = s.withLock(func() error { return s.selectLocked(next) })
			}
			return nil, nil, err
		}
	}
	// candidateWithOperation owns releasing operation even on failure.
	transferred = true
	candidate, err := s.candidateWithOperation(operation, next, options.Operation == RepairRelease)
	if err != nil {
		return nil, nil, err
	}
	return candidate, nil, nil
}

func (s *Store) validateEnsureSelection(m Manifest) (result error) {
	root, err := s.root.OpenRoot(filepath.Join("generations", m.Archive.SHA256))
	if err != nil {
		return selectedPayloadError(err)
	}
	defer func() { result = errors.Join(result, root.Close()) }()
	if err := verifyBoundPayload(root, m, ownerExecute); err != nil {
		return selectedPayloadError(err)
	}
	return nil
}

func selectedPayloadError(err error) error {
	// Only confirmed absence or structural violations prove damage. Access, I/O
	// and other inconclusive failures retain their cause for diagnosis or retry.
	var invalid payloadLayoutError
	if errors.Is(err, os.ErrNotExist) || errors.Is(err, syscall.ENOTDIR) || errors.As(err, &invalid) {
		return damaged(fmt.Errorf("selected runtime payload: %w", err))
	}
	return fmt.Errorf("selected runtime payload: %w", err)
}
