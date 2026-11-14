// SPDX-License-Identifier: GPL-3.0-or-later

package runtime

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/gofrs/flock"
)

var ErrInUse = errors.New("runtime installation has consumers or unresolved executions")

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

// BeginPublishedInstall takes installer exclusion before reading eligibility or
// calling load, which returns a release authenticated by VerifyRelease (through
// FetchRelease or ReadRelease in production). Discovery runs outside inventory
// exclusion, so active monitoring continues.
// The returned candidate owns installer exclusion through qualification and
// publication. A nil candidate is a successful no-op: disabled automatic updates
// or an unchanged release. No download can resurrect an uninstalled selection.
func (s *Store) BeginPublishedInstall(ctx context.Context, options ReleaseInstallOptions, load func(context.Context) (VerifiedRelease, error)) (*Candidate, error) {
	candidate, _, err := s.beginPublishedInstall(ctx, options, load)
	return candidate, err
}

// BeginPublishedEnsure enrolls only a proven absent selection. A usable selected
// generation is checked under installer exclusion without discovery, inventory,
// qualification or changing its update policy. The returned manifest is a
// snapshot; exclusion is released before return. Damage requires explicit repair.
// A new candidate enables automatic preparation only when successfully selected.
func (s *Store) BeginPublishedEnsure(ctx context.Context, load func(context.Context) (VerifiedRelease, error)) (*Candidate, *Manifest, error) {
	return s.beginPublishedInstall(ctx, ReleaseInstallOptions{Operation: ensureRelease, Updates: AutomaticUpdates}, load)
}

func (s *Store) beginPublishedInstall(ctx context.Context, options ReleaseInstallOptions, load func(context.Context) (VerifiedRelease, error)) (_ *Candidate, _ *Manifest, result error) {
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
			result = errors.Join(result, s.ledger.releaseOwner(operation))
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
	verified, err := load(ctx)
	if err != nil {
		return nil, nil, err
	}
	// The zero VerifiedRelease, the only one not built by VerifyRelease, is invalid.
	release := verified.release
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

// Candidate holds the installation transaction and generation protection through
// preparation, qualification and selection. Close aborts without changing the
// selected release unless Select has already published it.
type Candidate struct {
	*Consumer
	operation   *flock.Flock
	sealed      bool
	finished    bool
	publication selection
}

func (s *Store) lockInstallation(ctx context.Context) (*flock.Flock, error) {
	lock := flock.New(filepath.Join(s.path, "installation.lock"), flock.SetFlag(os.O_RDWR))
	locked, err := lock.TryLockContext(ctx, 50*time.Millisecond)
	if !locked || err != nil {
		return nil, errors.Join(err, ctx.Err(), lock.Close())
	}
	return lock, nil
}

// Ownership of operation transfers to the candidate, or is released on error.
// Repair rebuilds the release and retires damaged generations. It requires every
// consumer and execution to be drained and never resets ownership proof.
func (s *Store) candidateWithOperation(operation *flock.Flock, publication selection, repair bool) (*Candidate, error) {
	m := publication.Release.Manifest
	var consumer *Consumer
	err := s.withLock(func() error {
		if repair {
			if err := s.repairLocked(m); err != nil {
				return err
			}
		} else if _, err := s.selectedLocked(true); err != nil {
			return err
		}
		owner, err := s.ledger.claimLocked(m.Archive.SHA256, "candidate")
		if err == nil {
			consumer = &Consumer{
				store:    s,
				manifest: m,
				claim:    owner,
			}
		}
		return err
	})
	if err != nil {
		if consumer != nil {
			err = errors.Join(err, consumer.Close())
		}
		return nil, errors.Join(err, s.ledger.releaseOwner(operation))
	}
	return &Candidate{
		Consumer:    consumer,
		operation:   operation,
		publication: publication,
	}, nil
}

func (c *Candidate) Import(ctx context.Context, archive string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return errors.New("runtime candidate is closed")
	}
	return c.store.importArchive(ctx, c.manifest, archive)
}

func (c *Candidate) Download(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return errors.New("runtime candidate is closed")
	}
	return c.store.download(ctx, c.manifest)
}

// Select publishes a prepared candidate after the caller has qualified both
// supported workload kinds. Existing consumers keep their acquired generation.
func (c *Candidate) Select() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return errors.New("runtime candidate is closed")
	}
	return c.store.withLock(func() error {
		if _, err := c.store.selectedLocked(true); err != nil {
			return err
		}
		if _, err := c.store.prepared(c.manifest); err != nil {
			return err
		}
		return c.store.selectLocked(c.publication)
	})
}

func (c *Candidate) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.finished {
		return nil
	}
	c.closed = true
	var err error
	if c.sealed {
		err = c.store.ledger.releaseOwner(c.claim.lock)
	} else {
		err = c.store.closeClaim(c.claim)
	}
	// Failed releases remain in Store's retry list, independently of the caller.
	err = errors.Join(err, c.store.ledger.releaseOwner(c.operation))
	c.finished = true
	return err
}

// Uninstall is serialized with installers. A refusal leaves selection unchanged.
// A successful clear prevents future process admission before bytes are retired.
func (s *Store) Uninstall(ctx context.Context) (result error) {
	operation, err := s.lockInstallation(ctx)
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, s.ledger.releaseOwner(operation)) }()
	err = s.withLock(func() error {
		inv, err := s.inspectInventoryLocked(true)
		if err != nil {
			return err
		}
		if inv.inUse() {
			return ErrInUse
		}
		if err := s.selectLocked(selection{}); err != nil {
			return err
		}
		for id := range inv.generations {
			if err := s.removeOwned(filepath.Join("generations", id)); err != nil {
				return fmt.Errorf("runtime uninstalled, cleanup pending: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	return s.flushTrash()
}

func (s *Store) repairLocked(m Manifest) error {
	inv, err := s.inspectInventoryLocked(true)
	if err != nil {
		return err
	}
	if inv.inUse() {
		return ErrInUse
	}
	// A same-release rebuild cannot leave its old selection admitting startup
	// consumers while those paths are retired. Preserve healthy other selections.
	if inv.selectionError != nil || (inv.selected != nil && inv.selected.Archive.SHA256 == m.Archive.SHA256) {
		if err := s.selectLocked(selection{}); err != nil {
			return err
		}
	}
	for id := range inv.generations {
		if id != m.Archive.SHA256 && inv.generationErrors[id] == nil {
			continue
		}
		if err := s.removeOwned(filepath.Join("generations", id)); err != nil {
			return fmt.Errorf("runtime repair cleanup pending: %w", err)
		}
	}
	return nil
}
