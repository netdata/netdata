// SPDX-License-Identifier: GPL-3.0-or-later

package runtime

import (
	"errors"
	"os"
	"path/filepath"
	goruntime "runtime"
)

type preloadRecord struct {
	Manifest Manifest `json:"manifest"`
	Path     string   `json:"path"`
}

// SealPreload finishes a new administrator-owned root after qualification. The
// administrator must keep it immutable and stop every consumer before removal.
// Native package replacement coordination is a separate delivery contract.
func (c *Candidate) SealPreload() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return errors.New("runtime candidate is closed")
	}
	return c.store.withLock(func() error {
		inv, err := c.store.inventoryLocked(true)
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
	})
}

// ReadPreload reads administrator-controlled metadata from the immutable root.
// It creates no state, so read-only mounts work. This service-account-owned
// preload does not claim native package lifecycle or same-account isolation.
func ReadPreload(path string) (Paths, Manifest, error) {
	var empty Manifest
	if goruntime.GOOS != "linux" || (goruntime.GOARCH != "amd64" && goruntime.GOARCH != "arm64") {
		return Paths{}, empty, errors.New("preloaded browser runtime requires Linux amd64 or arm64")
	}
	if !filepath.IsAbs(path) {
		return Paths{}, empty, errors.New("runtime.preload_path must be absolute")
	}
	path, err := filepath.EvalSymlinks(path)
	if err != nil {
		return Paths{}, empty, err
	}
	if err := privatePreloadDirectory(path); err != nil {
		return Paths{}, empty, err
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		return Paths{}, empty, err
	}
	defer root.Close()
	if err := validateReadyStore(root); err != nil {
		return Paths{}, empty, err
	}
	var record preloadRecord
	if err := readJSON(root, "preload.json", &record); err != nil {
		return Paths{}, empty, err
	}
	if err := record.Manifest.Validate(); err != nil {
		return Paths{}, empty, err
	}
	if record.Path != path || record.Manifest.Platform != "linux/"+goruntime.GOARCH {
		return Paths{}, empty, errors.New("preload identity, platform or final path differs from its preparation")
	}
	paths, err := preparedPaths(root, path, record.Manifest)
	return paths, record.Manifest, err
}
