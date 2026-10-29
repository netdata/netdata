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
const publicationName = ".dem-runtime-publication.json"

type storeLayout struct {
	Format int    `json:"format"`
	Phase  string `json:"phase"`
}

type storePublication struct {
	Parent string `json:"parent"`
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
	if err := reconcileStorePublication(canonical); err != nil {
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
	record, err := readStoreLayout(root)
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
		record, err = readStoreLayout(root)
	}
	if err != nil {
		return err
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
	err = finishStoreInitialization(root)
	return errors.Join(err, lock.Close())
}

func finishStoreInitialization(root *os.Root) error {
	record, err := readStoreLayout(root)
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

func readStoreLayout(root *os.Root) (storeLayout, error) {
	var record storeLayout
	if err := readJSON(root, layoutName, &record); err != nil {
		return record, err
	}
	if record.Format != 1 || (record.Phase != "initializing" && record.Phase != "ready") {
		return record, errors.New("invalid runtime initialization record")
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

// Only paths created by this protocol carry an origin. Resolving one proves all
// links in that staged suffix durable before another store can build beneath it.
func reconcileStorePublication(path string) error {
	root, err := os.OpenRoot(path)
	if err != nil {
		return err
	}
	var origin storePublication
	readErr := readJSON(root, publicationName, &origin)
	closeErr := root.Close()
	if errors.Is(readErr, os.ErrNotExist) {
		return closeErr
	}
	if err := errors.Join(readErr, closeErr); err != nil {
		return err
	}
	relative, err := filepath.Rel(origin.Parent, path)
	if !filepath.IsAbs(origin.Parent) || filepath.Clean(origin.Parent) != origin.Parent || err != nil ||
		relative == "." ||
		!filepath.IsLocal(relative) {
		return errors.New("invalid runtime initialization publication parent")
	}
	parent, err := os.OpenRoot(origin.Parent)
	if err != nil {
		return err
	}
	if err := errors.Join(syncDirectory(parent, "."), parent.Close()); err != nil {
		return err
	}
	// Retire receipts only after the original barrier. Clearing the whole known
	// suffix avoids stale parent-access requirements after successful bootstrap.
	current := origin.Parent
	for _, component := range strings.Split(relative, string(os.PathSeparator)) {
		current = filepath.Join(current, component)
		if err := retireStorePublication(current, origin); err != nil {
			return err
		}
	}
	return nil
}

func retireStorePublication(path string, origin storePublication) (result error) {
	root, err := os.OpenRoot(path)
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, root.Close()) }()
	var record storePublication
	if err := readJSON(root, publicationName, &record); errors.Is(err, os.ErrNotExist) {
		// Another reconciler only removes this after syncing the same parent.
		return nil
	} else if err != nil {
		return err
	}
	if record != origin {
		return errors.New("runtime initialization publication origins differ")
	}
	if err := root.Remove(publicationName); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return syncDirectory(root, ".")
}

// Existing roots, including mountpoints, need no operations on their parents.
// Every directory of a missing suffix advertises its publication debt, including
// to another store creating a different path beneath a shared new ancestor.
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
	if err := reconcileStorePublication(canonical); err != nil {
		return "", err
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
	origin := storePublication{
		Parent: canonical,
	}
	data, err := json.Marshal(origin)
	if err != nil {
		return "", err
	}
	dirPath := temporary
	for index := range missing {
		if index > 0 {
			dirPath = filepath.Join(dirPath, missing[index])
		}
		dir, err := os.OpenRoot(dirPath)
		if err != nil {
			return "", err
		}
		err = errors.Join(writeNewFile(dir, publicationName, append(data, '\n'), 0600), dir.Close())
		if err != nil {
			return "", err
		}
	}
	staged, err := os.OpenRoot(stagedPath)
	if err != nil {
		return "", err
	}
	err = createInitialLayout(staged, storeLayout{
		Format: 1,
		Phase:  "initializing",
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

func validateReadyStore(root *os.Root) error {
	record, err := readStoreLayout(root)
	if err != nil {
		return err
	}
	if record.Phase != "ready" {
		return errors.New("runtime initialization is incomplete")
	}
	return validateStoreLayout(root)
}
