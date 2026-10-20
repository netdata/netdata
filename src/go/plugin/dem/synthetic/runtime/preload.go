// SPDX-License-Identifier: GPL-3.0-or-later

package runtime

import (
	"errors"
	"os"
	"path/filepath"
	goruntime "runtime"
)

type preloadRecord struct {
	Generation string `json:"generation"`
	Path       string `json:"path"`
}

// SealPreload finishes a freshly prepared administrator-owned root. It must
// never be moved or replaced while consumers run. Further managed operations,
// including those through already open stores, refuse this protected root.
func (s *Store) SealPreload(m Manifest) error {
	return s.withLock(func() error {
		if _, err := s.prepared(m); err != nil {
			return err
		}
		return saveJSON(s.root, "preload.json", preloadRecord{Generation: m.Archive.SHA256, Path: s.path})
	})
}

// ReadPreload resolves only a prepared bundle matching installed trust. It
// creates no locks, claims or cache state, so a read-only mount is supported.
// The administrator owns immutability; this is not a hostile same-account boundary.
func ReadPreload(path string, m Manifest) (Paths, error) {
	if goruntime.GOOS != "linux" || (goruntime.GOARCH != "amd64" && goruntime.GOARCH != "arm64") {
		return Paths{}, errors.New("preloaded browser runtime requires Linux amd64 or arm64")
	}
	if err := m.Validate(); err != nil {
		return Paths{}, err
	}
	if m.Platform != "linux/"+goruntime.GOARCH {
		return Paths{}, errors.New("preload does not match this architecture")
	}
	if !filepath.IsAbs(path) {
		return Paths{}, errors.New("runtime.preload_path must be absolute")
	}
	path, err := filepath.EvalSymlinks(path)
	if err != nil {
		return Paths{}, err
	}
	if err := privatePreloadDirectory(path); err != nil {
		return Paths{}, err
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		return Paths{}, err
	}
	defer root.Close()
	var record preloadRecord
	if err := readJSON(root, "preload.json", &record); err != nil {
		return Paths{}, err
	}
	if record.Path != path || record.Generation != m.Archive.SHA256 {
		return Paths{}, errors.New("preload identity or final path differs from installed requirement")
	}
	return preparedPaths(root, path, m)
}
