// SPDX-License-Identifier: GPL-3.0-or-later

package browserruntime

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// errPreloadDamaged reports damage in a sealed preload. Managed repair refuses
// preloads, so the remedy is a replacement root, not ErrDamaged's advice.
var errPreloadDamaged = errors.New(
	"preloaded runtime metadata is damaged; prepare a replacement preload at a new final root and point runtime.preload_path to it",
)

type preloadRecord struct {
	Manifest Manifest `json:"manifest"`
	Path     string   `json:"path"`
}

// SealPreload finishes a new administrator-owned root after preparation. The
// administrator must keep it immutable and stop every consumer before removal.
// Native package replacement coordination is a separate delivery contract.
func (c *Candidate) SealPreload() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return errors.New("runtime candidate is closed")
	}
	return publicDamage(c.store.withLock(func() error {
		inv, err := c.store.strictInventoryLocked(true)
		if err != nil {
			return err
		}
		if len(inv.generations) != 1 || len(inv.processes) > 0 || len(inv.uses) > 0 || len(inv.reservations) > 0 ||
			len(inv.candidates) != 1 ||
			inv.candidates[c.manifest.Archive.SHA256] != 1 {
			return ErrInUse
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
		if err := c.store.removeOwned(c.claim.name); err != nil {
			return err
		}
		// Publication can fail after rename. Close must not mutate a sealed root.
		c.closed = true
		c.sealed = true
		return saveJSON(c.store.root, "preload.json", preloadRecord{
			Manifest: c.manifest,
			Path:     c.store.path,
		})
	}))
}

// ReadPreload reads administrator-controlled metadata from the immutable root.
// It creates no state, so read-only mounts work. This service-account-owned
// preload does not claim native package lifecycle or same-account isolation.
// A nil agent checks execution compatibility and integrity; installation owners
// supply the target Agent to additionally require an exact release pair.
func ReadPreload(path string, agent *AgentRelease) (Paths, Manifest, error) {
	if !SupportedHost() {
		return Paths{}, Manifest{}, errors.New("preloaded browser runtime requires Linux amd64 or arm64")
	}
	if !filepath.IsAbs(path) {
		return Paths{}, Manifest{}, errors.New("runtime.preload_path must be absolute")
	}
	// Admission and the binding comparison use the same canonical final prefix,
	// including when runtime.preload_path names a symlink to the sealed root.
	path, err := filepath.EvalSymlinks(path)
	if err != nil {
		return Paths{}, Manifest{}, err
	}
	paths, m, err := readPreload(path)
	if isMetadataDamage(err) {
		// The cause is kept as text only, as for ErrDamaged.
		return Paths{}, Manifest{}, fmt.Errorf("%w: %v", errPreloadDamaged, err)
	}
	if err != nil {
		return Paths{}, Manifest{}, err
	}
	if err := m.Validate(); err != nil {
		return Paths{}, m, err
	}
	prefix := filepath.Join(path, "generations", m.Archive.SHA256)
	root, err := os.OpenRoot(prefix)
	if err == nil {
		err = verifyBoundPayload(root, prefix, m, ownerExecute)
		if err == nil {
			err = checkPayloadAgent(root, m, agent)
		}
		err = errors.Join(err, root.Close())
	}
	if err != nil {
		var invalid payloadLayoutError
		if errors.Is(err, os.ErrNotExist) || errors.Is(err, syscall.ENOTDIR) || errors.As(err, &invalid) {
			err = fmt.Errorf("%w: %v", errPreloadDamaged, err)
		}
		return Paths{}, m, err
	}
	return paths, m, nil
}

// path is the canonical absolute preload root checked by ReadPreload.
func readPreload(path string) (Paths, Manifest, error) {
	var empty Manifest
	if err := privatePreloadDirectory(path); err != nil {
		return Paths{}, empty, err
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		return Paths{}, empty, err
	}
	defer root.Close()
	if err := managedRootLayout.validateReady(root); err != nil {
		return Paths{}, empty, err
	}
	var record preloadRecord
	if err := readJSON(root, "preload.json", &record); err != nil {
		return Paths{}, empty, err
	}
	if err := record.Manifest.ValidateMetadata(); err != nil {
		return Paths{}, empty, damagedMetadata("preload.json", err)
	}
	if record.Path != path || !record.Manifest.MatchesHost() {
		return Paths{}, empty, errors.New("preload identity, platform or final path differs from its preparation")
	}
	paths, err := preparedPaths(root, path, record.Manifest)
	if errors.Is(err, ErrUnavailable) {
		// A sealed preload names a generation it no longer contains.
		err = damagedMetadata("preload.json", err)
	}
	return paths, record.Manifest, err
}
