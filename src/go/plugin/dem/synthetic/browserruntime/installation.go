// SPDX-License-Identifier: GPL-3.0-or-later

package browserruntime

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"syscall"
	"time"

	"github.com/gofrs/flock"
)

var ErrInUse = errors.New("runtime installation has consumers or unresolved executions")

var errInvalidOperation = errors.New("invalid published runtime operation")

// ReleaseOperation distinguishes enrollment, repair and owner-controlled
// pair preparation. Repair may select an older authenticated release; Agent owners
// decide paired release ordering.
type ReleaseOperation string

const (
	InstallRelease ReleaseOperation = "install"
	PrepareRelease ReleaseOperation = "prepare"
	RepairRelease  ReleaseOperation = "repair"
)

type UpdatePolicy string

const (
	AutomaticUpdates UpdatePolicy = "automatic"
	ManualUpdates    UpdatePolicy = "manual"
)

type ReleaseInstallOptions struct {
	Operation ReleaseOperation
	// Required for install and repair. Pair preparation preserves consent.
	Updates UpdatePolicy
}

// BeginPublishedInstall takes installer exclusion before reading eligibility or
// calling load, which returns a release authenticated by VerifyRelease (through
// FetchRelease or ReadRelease in production). Discovery runs outside inventory
// exclusion, so active monitoring continues.
// The returned candidate owns installer exclusion through preparation and
// publication. A nil candidate is an unchanged release. Pair preparation verifies
// the selected payload before reporting unchanged, without replacing recovery
// references. Preparation of an uninstalled selection is refused before discovery.
func (s *Store) BeginPublishedInstall(
	ctx context.Context,
	options ReleaseInstallOptions,
	load func(context.Context) (VerifiedRelease, error),
) (_ *Candidate, result error) {
	switch options.Operation {
	case InstallRelease, RepairRelease:
		if options.Updates != AutomaticUpdates && options.Updates != ManualUpdates {
			return nil, errors.New("runtime installation requires an explicit update policy")
		}
	case PrepareRelease:
		if options.Updates != "" {
			return nil, errors.New("runtime preparation preserves the installed update policy")
		}
	default:
		return nil, errInvalidOperation
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
			result = errors.Join(result, s.ledger.releaseOwner(operation))
		}
	}()
	var current selection
	var pending bool
	err = s.withLock(func() error {
		recovery, err := s.readPairRecovery()
		if err != nil {
			return err
		}
		pending = recovery.Pending != nil
		if pending && options.Operation != PrepareRelease {
			return ErrPairPending
		}
		// Repair is an explicit drained operation and can recover damaged
		// selection metadata. Its caller supplies the new consent policy.
		if options.Operation == RepairRelease {
			return nil
		}
		if _, err := s.selectedLocked(true); err != nil {
			return err
		}
		current, err = s.readSelectionRecord()
		return err
	})
	if err != nil {
		return nil, err
	}
	if options.Operation == PrepareRelease && current.Release == nil {
		return nil, errors.New("install an authenticated runtime release before paired preparation")
	}
	verified, err := load(ctx)
	if err != nil {
		return nil, err
	}
	// The zero VerifiedRelease, the only one not built by VerifyRelease, is invalid.
	release := verified.release
	if err := s.ValidateRelease(release); err != nil {
		return nil, err
	}
	if s.config.Agent != nil && current.Release != nil && !reflect.DeepEqual(current.Release.Agent, release.Agent) &&
		options.Operation != PrepareRelease && options.Operation != RepairRelease {
		return nil, errors.New("a different Agent/runtime pair requires preparation by its installation owner")
	}
	next := selection{
		Release:          &release,
		AutomaticUpdates: current.AutomaticUpdates,
	}
	if options.Operation == InstallRelease || options.Operation == RepairRelease {
		next.AutomaticUpdates = options.Updates == AutomaticUpdates
	}
	if current.Release != nil {
		changed, err := releaseChanged(*current.Release, release)
		if err != nil {
			return nil, err
		}
		if !changed && !pending {
			if options.Operation == PrepareRelease {
				err = s.withLock(func() error { return s.validateRetainedSelection(current) })
				return nil, publicDamage(err)
			}
			// Reinstalling the exact release can explicitly change consent
			// without acquiring or executing unchanged payload bytes.
			if current.AutomaticUpdates != next.AutomaticUpdates {
				err = s.withLock(func() error { return s.selectLocked(next) })
			}
			return nil, err
		}
	}
	// candidateWithOperation owns releasing operation even on failure.
	transferred = true
	candidate, err := s.candidateWithOperation(operation, next, options.Operation == RepairRelease)
	if err != nil {
		return nil, err
	}
	candidate.prepareOnly = options.Operation == PrepareRelease
	return candidate, nil
}

func (s *Store) validatePreparedPayload(m Manifest) (result error) {
	name := filepath.Join("generations", m.Archive.SHA256)
	root, err := s.root.OpenRoot(name)
	if err != nil {
		return selectedPayloadError(err)
	}
	defer func() { result = errors.Join(result, root.Close()) }()
	if err := verifyBoundPayload(root, filepath.Join(s.path, name), m, ownerExecute); err != nil {
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
// preparation and selection. Close aborts without changing the
// selected release unless Select has already published it.
type Candidate struct {
	*Consumer
	operation   *flock.Flock
	sealed      bool
	finished    bool
	publication selection
	prepareOnly bool
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
			if err := s.repairLocked(*publication.Release); err != nil {
				return err
			}
		} else if _, err := s.selectedLocked(true); err != nil {
			return err
		}
		owner, err := s.ledger.claimLocked(m.Archive.SHA256, candidateClaim)
		if err == nil {
			consumer = &Consumer{
				store:    s,
				manifest: m,
				agent:    publication.Release.Agent,
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
	return publicDamage(c.store.importArchive(ctx, c.manifest, archive))
}

func (c *Candidate) Download(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return errors.New("runtime candidate is closed")
	}
	return publicDamage(c.store.download(ctx, c.manifest))
}

// Select publishes a verified, prepared candidate. Optional browser diagnostics
// are separate. Existing consumers keep their acquired generation.
func (c *Candidate) Select() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return errors.New("runtime candidate is closed")
	}
	if c.prepareOnly {
		return errors.New("paired preparation cannot select a runtime; use PreparePair and drained activation")
	}
	if err := c.store.ValidateRelease(*c.publication.Release); err != nil {
		return err
	}
	return publicDamage(c.store.withLock(func() error {
		if err := c.store.noPendingPair(); err != nil {
			return err
		}
		if _, err := c.store.selectedLocked(true); err != nil {
			return err
		}
		if _, err := c.store.prepared(c.manifest); err != nil {
			return err
		}
		if err := c.store.validatePreparedPayload(c.manifest); err != nil {
			return err
		}
		if err := c.store.verifyPreparedAgent(c.manifest, c.agent); err != nil {
			return err
		}
		return c.store.selectLocked(c.publication)
	}))
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
		if err := s.noPendingPair(); err != nil {
			return err
		}
		inv, err := s.lenientInventoryLocked(true)
		if err != nil {
			return err
		}
		if inv.inUse() {
			return ErrInUse
		}
		if err := s.selectLocked(selection{}); err != nil {
			return err
		}
		if err := s.savePairRecovery(pairRecovery{Format: 1}); err != nil {
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

func (s *Store) repairLocked(release Release) error {
	m := release.Manifest
	inv, err := s.lenientInventoryLocked(true)
	if err != nil {
		return err
	}
	if inv.inUse() {
		return ErrInUse
	}
	// Rebuild exact retained bytes without discarding their recovery promise.
	// Check references even when an interrupted repair left no generation.
	for _, retained := range inv.recovery.selections() {
		if retained.Release.Manifest.Archive.SHA256 == m.Archive.SHA256 &&
			(!sameExecutable(retained.Release.Manifest, m) || !reflect.DeepEqual(retained.Release.Agent, release.Agent)) {
			return errors.New("runtime repair target differs from its retained pair")
		}
	}
	for id := range inv.generations {
		if id != m.Archive.SHA256 && inv.recovery.protects(id) && inv.generationErrors[id] != nil {
			return errors.New("runtime repair would discard a retained pair; use the installation owner's recovery procedure")
		}
	}
	// A same-release rebuild cannot leave its old selection admitting startup
	// consumers while those paths are retired. Preserve healthy other selections.
	if inv.selectionError != nil || inv.isSelected(m.Archive.SHA256) {
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
