// SPDX-License-Identifier: GPL-3.0-or-later

// Package artifacts owns optional DEM captures independently of history.
package artifacts

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/gofrs/flock"
)

const FetchMaxBytes int64 = 5 << 20

var (
	ErrClosed         = os.ErrClosed
	ErrNotFound       = errors.New("artifact not found")
	ErrTooLarge       = errors.New("artifact exceeds the 5 MiB fetch limit")
	ErrInvalidCapture = errors.New("invalid artifact capture")
	ErrInvalidID      = errors.New("invalid artifact or run ID")
	ErrOwnerMismatch  = errors.New("artifact owner does not match")
	ErrExists         = errors.New("artifact run already exists")
	ErrLocked         = errors.New("artifact store is already owned")
	runIDPattern      = regexp.MustCompile(`^[0-9a-f]{32}$`)
	artifactIDPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,127}$`)
)

type manifest struct {
	Version int `json:"version"`
	Manifest
}

func validJobID(id string) bool {
	kind, name, ok := strings.Cut(id, ":")
	return ok && (kind == "journey" || kind == "lighthouse") && strings.TrimSpace(name) != ""
}

// Store serializes publication, fetch, expiry and close. Work may only be
// finalized after its caller has joined the complete supervised process tree.
// Same-UID/root tampering with this private store is outside its ownership model.
type Store struct {
	mu        sync.Mutex
	root      *os.Root
	path      string
	temporary bool
	lock      *flock.Flock
	closed    bool
	active    map[string]string
	uncertain map[string]bool
	policy    *Policy
	cleanup   Cleanup
	eviction  evictionIO
	// Inventory uses the same rooted filesystem; tests can fail enumeration alone.
	inventoryFS fs.FS
}

// Open takes one lifetime filesystem lock. Empty path creates a private store.
func Open(name string) (_ *Store, retErr error) {
	temporary := name == ""
	if temporary {
		var err error
		name, err = os.MkdirTemp("", "netdata-dem-artifacts-")
		if err != nil {
			return nil, err
		}
		temporaryRoot := name
		defer func() {
			if retErr != nil {
				retErr = errors.Join(retErr, os.RemoveAll(temporaryRoot))
			}
		}()
	}
	name, err := filepath.Abs(name)
	if err != nil {
		return nil, err
	}
	if err = makeStoreDir(name); err != nil {
		return nil, err
	}
	info, err := os.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("artifact root must be a real directory")
	}
	if err = os.Chmod(name, 0700); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(name)
	if err != nil {
		return nil, err
	}
	s := &Store{
		root:        root,
		path:        name,
		temporary:   temporary,
		active:      make(map[string]string),
		uncertain:   make(map[string]bool),
		inventoryFS: root.FS(),
	}
	defer func() {
		if retErr != nil {
			_ = root.Close()
			if s.lock != nil {
				_ = s.lock.Close()
			}
		}
	}()
	s.lock = flock.New(filepath.Join(name, ".lock"))
	locked, err := s.lock.TryLock()
	if err != nil {
		return nil, err
	}
	if !locked {
		return nil, ErrLocked
	}
	for _, dir := range []string{"work", "staging", "runs", "drained", "deleting"} {
		if err = root.MkdirAll(dir, 0700); err != nil {
			return nil, err
		}
		info, err = root.Lstat(dir)
		if err != nil {
			return nil, err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("artifact %s is not a real directory", dir)
		}
		if err = syncDir(root, dir); err != nil {
			return nil, err
		}
	}
	if err = syncDir(root, "."); err != nil {
		return nil, err
	}
	// Sync the store's parent too: first-use directory creation is durable.
	parent, err := os.Open(filepath.Dir(name))
	if err != nil {
		return nil, err
	}
	err = errors.Join(parent.Sync(), parent.Close())
	if err != nil {
		return nil, err
	}
	s.eviction = evictionIO{
		rename:    root.Rename,
		removeAll: root.RemoveAll,
		sync:      func(dir string) error { return syncDir(root, dir) },
	}
	return s, nil
}

func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	var err error
	if s.temporary {
		// Only an empty work/staging/drained set permits deleting a temporary root.
		// A previous owner may have left an unverified live process behind.
		removable := true
		for _, dir := range []string{"work", "staging", "drained", "deleting"} {
			entries, e := fs.ReadDir(s.root.FS(), dir)
			err = errors.Join(err, e)
			if e != nil || len(entries) != 0 {
				removable = false
			}
		}
		if removable {
			entries, e := fs.ReadDir(s.root.FS(), "runs")
			err = errors.Join(err, e)
			if e != nil {
				removable = false
			}
			for _, entry := range entries {
				if _, e := s.readManifest(entry.Name()); e != nil {
					removable = false
					break
				}
			}
		}
		if removable {
			err = errors.Join(err, s.root.RemoveAll("runs"))
		}
		if removable && err == nil {
			err = errors.Join(err, os.RemoveAll(s.path))
		}
	}
	s.closed = true
	return errors.Join(err, s.root.Close(), s.lock.Close())
}
