// SPDX-License-Identifier: GPL-3.0-or-later

package runtime

import (
	"context"
	"errors"
)

// ReleaseOperation distinguishes an explicit installation/repair decision from
// checks of an existing installation. Repair may deliberately select an older
// authenticated release; normal checks may never regress publication order.
type ReleaseOperation string

const (
	InstallRelease         ReleaseOperation = "install"
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

func (s selection) validateRelease() error {
	if s.Release == nil {
		if s.AutomaticUpdates {
			return errors.New("automatic runtime updates require an authenticated release")
		}
		return nil
	}
	if s.Manifest == nil {
		return errors.New("uninstalled runtime retains release metadata")
	}
	if err := s.Release.Validate(); err != nil {
		return err
	}
	if s.Manifest.Archive.URL != s.Release.Manifest.Archive.URL || !sameManifest(*s.Manifest, s.Release.Manifest) {
		return errors.New("selected runtime differs from its authenticated release")
	}
	return nil
}

// BeginPublishedInstall takes installer exclusion BEFORE reading eligibility or
// calling load. The caller must authenticate load's result with VerifyRelease.
// Discovery runs outside inventory exclusion, so active monitoring continues.
// The returned candidate owns installer exclusion through qualification and
// publication. A nil candidate is a successful no-op: disabled automatic updates
// or an unchanged release. No download can resurrect an uninstalled selection.
func (s *Store) BeginPublishedInstall(ctx context.Context, options ReleaseInstallOptions, load func(context.Context) (Release, error)) (_ *Candidate, result error) {
	switch options.Operation {
	case InstallRelease, RepairRelease:
		if options.Updates != AutomaticUpdates && options.Updates != ManualUpdates {
			return nil, errors.New("runtime installation requires an explicit update policy")
		}
	case UpdateRelease, AutomaticReleaseUpdate:
		if options.Updates != "" {
			return nil, errors.New("runtime update preserves the installed update policy")
		}
	default:
		return nil, errors.New("invalid published runtime operation")
	}
	if load == nil {
		return nil, errors.New("runtime release loader is unavailable")
	}
	operation, err := s.lockInstallation(ctx)
	if err != nil {
		return nil, err
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
		return err
	})
	if err != nil {
		return nil, err
	}
	if options.Operation == AutomaticReleaseUpdate && !current.AutomaticUpdates {
		return nil, nil
	}
	if options.Operation == UpdateRelease && current.Release == nil {
		return nil, errors.New("install an authenticated runtime release before checking updates")
	}
	release, err := load(ctx)
	if err != nil {
		return nil, err
	}
	if err := release.Validate(); err != nil {
		return nil, err
	}
	next := selection{Manifest: &release.Manifest, Release: &release, AutomaticUpdates: current.AutomaticUpdates}
	if options.Operation == InstallRelease || options.Operation == RepairRelease {
		next.AutomaticUpdates = options.Updates == AutomaticUpdates
	}
	if current.Release != nil {
		changed, err := ReleaseUpdate(*current.Release, release)
		if err != nil {
			return nil, err
		}
		if !changed {
			// Reinstalling the exact release can explicitly change consent
			// without acquiring or executing unchanged payload bytes.
			if current.AutomaticUpdates != next.AutomaticUpdates {
				err = s.withLock(func() error { return s.selectRecordLocked(next) })
			}
			return nil, err
		}
	}
	// candidateWithOperation owns releasing operation even on failure.
	transferred = true
	candidate, err := s.candidateWithOperation(operation, release.Manifest, options.Operation == RepairRelease)
	if err != nil {
		return nil, err
	}
	candidate.publication = &next
	return candidate, nil
}
