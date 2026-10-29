// SPDX-License-Identifier: GPL-3.0-or-later

package runtime

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

var storeDirectories = []string{"generations", "claims", "uses", "staging", "trash", "initializing"}
var storeLocks = []string{"coordination.lock", "installation.lock"}

// Publish only complete new roots. Reopening must never turn lost ownership
// namespaces or lock identities into apparent evidence that nothing is running.
func openStoreRoot(path string) (string, *os.Root, error) {
	parent := filepath.Dir(path)
	if err := os.MkdirAll(parent, 0700); err != nil {
		return "", nil, err
	}
	canonical, err := filepath.EvalSymlinks(path)
	if errors.Is(err, os.ErrNotExist) {
		// Do not replace a dangling symlink with a new installation.
		info, statErr := os.Lstat(path)
		if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
			return "", nil, statErr
		}
		if statErr == nil && info.Mode()&os.ModeSymlink != 0 {
			return "", nil, fmt.Errorf("invalid runtime root: %s", path)
		}
		parent, err = filepath.EvalSymlinks(parent)
		if err != nil {
			return "", nil, err
		}
		canonical = filepath.Join(parent, filepath.Base(path))
	} else if err != nil {
		return "", nil, err
	}
	entries, err := os.ReadDir(canonical)
	if err == nil {
		if err := privateDirectory(canonical); err != nil {
			return "", nil, err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", nil, err
	}
	if len(entries) == 0 {
		if err := initializeStore(canonical); err != nil {
			return "", nil, err
		}
	}
	if err := privateDirectory(canonical); err != nil {
		return "", nil, err
	}
	root, err := os.OpenRoot(canonical)
	if err != nil {
		return "", nil, err
	}
	// Reconcile a previous initializer's visible rename, including ancestors
	// created by MkdirAll. Never remove a published root on a sync failure.
	if err := errors.Join(validateStoreLayout(root), syncAncestors(filepath.Dir(canonical))); err != nil {
		return "", nil, errors.Join(err, root.Close())
	}
	return canonical, root, nil
}

func initializeStore(path string) error {
	temporary, err := os.MkdirTemp(filepath.Dir(path), ".dem-runtime-init-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(temporary)
	root, err := os.OpenRoot(temporary)
	if err != nil {
		return err
	}
	defer root.Close()
	for _, name := range storeLocks {
		if err := ensureLock(root, name); err != nil {
			return err
		}
	}
	for _, name := range storeDirectories {
		if err := root.Mkdir(name, 0700); err != nil {
			return err
		}
		if err := syncDirectory(root, name); err != nil {
			return err
		}
	}
	if err := saveJSON(root, "selection.json", selection{}); err != nil {
		return err
	}
	if err := syncDirectory(root, "."); err != nil {
		return err
	}
	if err := os.Rename(temporary, path); err != nil && !errors.Is(err, syscall.EEXIST) && !errors.Is(err, syscall.ENOTEMPTY) {
		return err
	}
	// A concurrent initializer may have published first. The caller validates
	// that authoritative root and reconciles its parent-directory durability.
	return nil
}

func validateStoreLayout(root *os.Root) error {
	for _, name := range storeLocks {
		info, err := root.Lstat(name)
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() || info.Mode().Perm() != 0600 || info.Size() != 0 {
			return fmt.Errorf("invalid runtime lock: %s", name)
		}
	}
	for _, name := range storeDirectories {
		info, err := root.Lstat(name)
		if err != nil {
			return err
		}
		if !info.IsDir() {
			return fmt.Errorf("invalid runtime ownership directory: %s", name)
		}
	}
	return nil
}

func syncAncestors(path string) error {
	for {
		dir, err := os.Open(path)
		if err != nil {
			return err
		}
		if err := errors.Join(dir.Sync(), dir.Close()); err != nil {
			return err
		}
		parent := filepath.Dir(path)
		if parent == path {
			return nil
		}
		path = parent
	}
}
