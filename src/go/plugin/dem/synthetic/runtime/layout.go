// SPDX-License-Identifier: GPL-3.0-or-later

package runtime

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gofrs/flock"
)

var storeDirectories = []string{"generations", "claims", "uses", "staging", "trash", "initializing"}
var storeLocks = []string{"coordination.lock", "installation.lock"}

const layoutName = "layout.json"
const layoutTemporaryPrefix = ".dem-layout-"

type storeLayout struct {
	Format            int    `json:"format"`
	Phase             string `json:"phase"`
	PublicationParent string `json:"publication_parent,omitempty"`
}

// The initialization record distinguishes interrupted bootstrap from lost safety
// state. Ready stores never recreate ownership namespaces or lock identities.
func openStoreRoot(path string) (string, *os.Root, error) {
	canonical, err := ensureStoreRoot(filepath.Clean(path))
	if err != nil {
		return "", nil, err
	}
	if err := privateDirectory(canonical); err != nil {
		return "", nil, err
	}
	root, err := os.OpenRoot(canonical)
	if err != nil {
		return "", nil, err
	}
	if err := openStoreLayout(root, canonical); err != nil {
		return "", nil, errors.Join(err, root.Close())
	}
	return canonical, root, nil
}

func openStoreLayout(root *os.Root, path string) error {
	record, err := readStoreLayout(root, path)
	if errors.Is(err, os.ErrNotExist) {
		// Only unpublished bootstrap temporaries may precede the first record.
		// Existing state with a missing record is never a fresh installation.
		dir, openErr := root.Open(".")
		if openErr != nil {
			return openErr
		}
		entries, readErr := dir.ReadDir(-1)
		err = errors.Join(readErr, dir.Close())
		if err != nil {
			return err
		}
		for _, entry := range entries {
			suffix, ok := strings.CutPrefix(entry.Name(), layoutTemporaryPrefix)
			if !ok || !validID(suffix) || !entry.Type().IsRegular() {
				// Another initializer may have published after our first read.
				if _, err := root.Lstat(layoutName); err == nil {
					break
				}
				return errors.New("runtime initialization record is missing from a nonempty store")
			}
		}
		if err := createInitialLayout(root, storeLayout{
			Format: 1,
			Phase:  "initializing",
		}); err != nil {
			return err
		}
		record, err = readStoreLayout(root, path)
	}
	if err != nil {
		return err
	}
	if record.PublicationParent != "" {
		parent, err := os.OpenRoot(record.PublicationParent)
		if err != nil {
			return err
		}
		err = errors.Join(syncDirectory(parent, "."), parent.Close())
		if err != nil {
			return err
		}
	}
	if err := syncDirectory(root, "."); err != nil {
		return err
	}
	if record.Phase == "ready" {
		return validateStoreLayout(root)
	}
	// No executable access is allowed during initializing. Concurrent openers
	// share this lock and reread the winning phase before filling missing state.
	if err := ensureLock(root, "coordination.lock"); err != nil {
		return err
	}
	lock := flock.New(filepath.Join(path, "coordination.lock"))
	if err := lock.Lock(); err != nil {
		return errors.Join(err, lock.Close())
	}
	err = finishStoreInitialization(root, path)
	return errors.Join(err, lock.Close())
}

func finishStoreInitialization(root *os.Root, path string) error {
	record, err := readStoreLayout(root, path)
	if err != nil {
		return err
	}
	if record.Phase == "ready" {
		return errors.Join(validateStoreLayout(root), syncDirectory(root, "."))
	}
	for _, name := range storeLocks {
		if err := ensureLock(root, name); err != nil {
			return err
		}
		// A competing initializer may have created this file but not synced it.
		if err := syncDirectory(root, name); err != nil {
			return err
		}
	}
	for _, name := range storeDirectories {
		if err := root.Mkdir(name, 0700); err != nil && !errors.Is(err, os.ErrExist) {
			return err
		}
		if err := syncDirectory(root, name); err != nil {
			return err
		}
	}
	if err := validateStoreLayout(root); err != nil {
		return err
	}
	if err := saveJSON(root, "selection.json", selection{}); err != nil {
		return err
	}
	return saveJSON(root, layoutName, storeLayout{
		Format: 1,
		Phase:  "ready",
	})
}

func readStoreLayout(root *os.Root, path string) (storeLayout, error) {
	var record storeLayout
	if err := readJSON(root, layoutName, &record); err != nil {
		return record, err
	}
	if record.Format != 1 || (record.Phase != "initializing" && record.Phase != "ready") {
		return record, errors.New("invalid runtime initialization record")
	}
	if record.PublicationParent != "" {
		parent := record.PublicationParent
		relative, err := filepath.Rel(parent, path)
		if record.Phase != "initializing" || !filepath.IsAbs(parent) || filepath.Clean(parent) != parent ||
			err != nil ||
			relative == "." ||
			!filepath.IsLocal(relative) {
			return record, errors.New("invalid runtime initialization publication parent")
		}
	}
	return record, nil
}

func createInitialLayout(root *os.Root, record storeLayout) error {
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	id, err := randomID()
	if err != nil {
		return err
	}
	name := layoutTemporaryPrefix + id
	if err := writeNewFile(root, name, append(data, '\n'), 0600); err != nil {
		return err
	}
	defer root.Remove(name)
	// Linking a synced record publishes it atomically without replacing a winner.
	if err := root.Link(name, layoutName); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	return syncDirectory(root, ".")
}

// Existing roots, including mountpoints, need no operations on their parents.
// A missing suffix appears atomically with its publication debt, so another
// opener of the same root cannot mistake it for durable administrator provisioning.
func ensureStoreRoot(path string) (string, error) {
	ancestor := path
	var missing []string
	for {
		_, err := os.Lstat(ancestor)
		if err == nil {
			break
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		missing = append([]string{filepath.Base(ancestor)}, missing...)
		ancestor = filepath.Dir(ancestor)
	}
	canonical, err := filepath.EvalSymlinks(ancestor)
	if err != nil {
		return "", err
	}
	if len(missing) == 0 {
		return canonical, nil
	}
	temporary, err := os.MkdirTemp(canonical, ".dem-runtime-init-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(temporary)
	stagedPath := filepath.Join(append([]string{temporary}, missing[1:]...)...)
	if err := os.MkdirAll(stagedPath, 0700); err != nil {
		return "", err
	}
	staged, err := os.OpenRoot(stagedPath)
	if err != nil {
		return "", err
	}
	err = createInitialLayout(staged, storeLayout{
		Format:            1,
		Phase:             "initializing",
		PublicationParent: canonical,
	})
	err = errors.Join(err, staged.Close())
	if err != nil {
		return "", err
	}
	tree, err := os.OpenRoot(temporary)
	if err != nil {
		return "", err
	}
	err = errors.Join(syncTree(tree), tree.Close())
	if err != nil {
		return "", err
	}
	err = renameDirectoryExclusive(canonical, filepath.Base(temporary), missing[0])
	if errors.Is(err, os.ErrExist) {
		// Reconcile the winning publication before retrying a shared missing suffix.
		parent, openErr := os.OpenRoot(canonical)
		if openErr != nil {
			return "", openErr
		}
		if err := errors.Join(syncDirectory(parent, "."), parent.Close()); err != nil {
			return "", err
		}
		return ensureStoreRoot(path)
	}
	if err != nil {
		return "", err
	}
	return filepath.Join(append([]string{canonical}, missing...)...), nil
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

func validateReadyStore(root *os.Root, path string) error {
	record, err := readStoreLayout(root, path)
	if err != nil {
		return err
	}
	if record.Phase != "ready" {
		return errors.New("runtime initialization is incomplete")
	}
	return validateStoreLayout(root)
}
