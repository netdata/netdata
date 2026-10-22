// SPDX-License-Identifier: GPL-3.0-or-later

package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	goruntime "runtime"
	"time"

	"github.com/gofrs/flock"
	"github.com/netdata/netdata/go/plugins/pkg/safefile"
)

// Import uses the same trusted digest/format as network preparation. The archive
// is an offline input, not a replacement for the installed trust manifest.
func (s *Store) Import(ctx context.Context, m Manifest, archive string) error {
	return s.prepare(ctx, m, func() (io.ReadCloser, error) {
		file, err := safefile.Open(archive)
		if err != nil {
			return nil, fmt.Errorf("offline runtime archive: %w", err)
		}
		return file, nil
	})
}

func (s *Store) prepare(ctx context.Context, m Manifest, open func() (io.ReadCloser, error)) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := m.Validate(); err != nil {
		return err
	}
	if m.Platform != "linux/"+goruntime.GOARCH {
		return errors.New("runtime archive does not match this architecture")
	}
	if err := checkContext(ctx); err != nil {
		return err
	}
	claim, err := s.Demand(m.Archive.SHA256)
	if err != nil {
		return err
	}
	defer claim.Close() // Close records cleanup health independently of preparation.
	// Cleanup errors are exposed independently; a matching prepared runtime
	// remains usable. New reservations still require a complete inventory.
	_ = s.Sweep()
	name, owner, err := s.reserve(m)
	if owner != nil {
		// Installed before checking err: coordination unlock may fail after reserve.
		defer s.finishPreparation(name, owner)
	}
	if err != nil || owner == nil {
		return err
	}
	source, err := open()
	if err != nil {
		return err
	}
	defer source.Close()
	operation, err := s.root.OpenRoot(name)
	if err != nil {
		return err
	}
	defer operation.Close()
	archive, err := operation.OpenFile("archive.tar.gz", os.O_RDWR|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer archive.Close()
	if err := saveArchive(contextReader{ctx: ctx, source: source}, archive, m.Archive); err != nil {
		return err
	}
	if _, err := archive.Seek(0, io.SeekStart); err != nil {
		return err
	}
	if err := operation.Mkdir("payload", 0700); err != nil {
		return err
	}
	payload, err := operation.OpenRoot("payload")
	if err != nil {
		return err
	}
	defer payload.Close()
	if err := extractArchive(contextReader{ctx: ctx, source: archive}, payload, m.Archive); err != nil {
		return err
	}
	final := filepath.Join(s.path, "generations", m.Archive.SHA256)
	if err := bindPrefix(payload, final, m); err != nil {
		return err
	}
	g := generation{Manifest: m, LastUsed: s.now().UTC()}
	if err := saveJSON(payload, "prepared.json", g); err != nil {
		return err
	}
	if err := syncTree(payload); err != nil {
		return err
	}
	if err := checkContext(ctx); err != nil {
		return err
	}
	return s.withLock(func() error {
		if _, err := s.loadGeneration(m.Archive.SHA256); err == nil {
			return errors.New("runtime generation appeared during preparation")
		} else if !errors.Is(err, ErrUnavailable) {
			return err
		}
		if err := s.root.Rename(filepath.Join(name, "payload"), filepath.Join("generations", m.Archive.SHA256)); err != nil {
			return err
		}
		if err := errors.Join(syncDirectory(s.root, name), syncDirectory(s.root, "generations")); err != nil {
			return err
		}
		// Published bytes are now counted in generations. Only archive/owner
		// records remain reserved until the in-process writer releases them.
		remaining, err := s.diskBytes(name)
		if err != nil {
			return err
		}
		return saveJSON(s.root, filepath.Join(name, "owner.json"), reservation{Generation: m.Archive.SHA256, Bytes: remaining})
	})
}

func (s *Store) reserve(m Manifest) (string, *flock.Flock, error) {
	needed, err := s.preparationBytes(m)
	if err != nil {
		return "", nil, err
	}
	for {
		// Deletion occurs outside coordination exclusion; failed bytes stay counted.
		cleanupErr := s.flushTrash()
		var name string
		var owner *flock.Flock
		progress := false
		err := s.withLock(func() error {
			// Publication and reservation share one decision. A concurrent
			// preparer may have finished since this caller established demand.
			if _, err := s.prepared(m); err == nil {
				g, err := s.loadGeneration(m.Archive.SHA256)
				if err != nil {
					return err
				}
				g.LastUsed = s.now().UTC()
				return saveJSON(s.root, filepath.Join("generations", m.Archive.SHA256, "prepared.json"), g)
			} else if !errors.Is(err, ErrUnavailable) {
				return err
			}
			inv, err := s.inventoryLocked(true)
			if err != nil {
				return err
			}
			for _, r := range inv.reservations {
				if r.Generation == m.Archive.SHA256 {
					return ErrPreparing
				}
			}
			status, err := s.statusLocked(inv)
			if err != nil {
				return err
			}
			if status.UsedBytes <= s.config.MaxBytes && status.ReservedBytes <= s.config.MaxBytes-status.UsedBytes && needed <= s.config.MaxBytes-status.UsedBytes-status.ReservedBytes {
				name, owner, err = s.newOwner("staging", reservation{Generation: m.Archive.SHA256, Bytes: needed})
				return err
			}
			ids := s.candidates(inv, true)
			if len(ids) == 0 {
				return errors.Join(&AllowanceError{NeededBytes: needed, Status: status}, cleanupErr)
			}
			if err := s.removeOwned(filepath.Join("generations", ids[0])); err != nil {
				return err
			}
			progress = true
			return nil
		})
		if err != nil || !progress {
			return name, owner, err
		}
	}
}

func (s *Store) preparationBytes(m Manifest) (int64, error) {
	final := filepath.Join(s.path, "generations", m.Archive.SHA256)
	fonts, launcher, err := boundConfiguration(final, m.Layout)
	if err != nil {
		return 0, err
	}
	// Reserve the longest timestamp representation and temporary metadata copy.
	stamp := time.Date(9999, 12, 31, 23, 59, 59, 999999999, time.UTC)
	g, err := json.Marshal(generation{Manifest: m, LastUsed: stamp, Unneeded: stamp})
	if err != nil {
		return 0, err
	}
	r, err := json.Marshal(reservation{Generation: m.Archive.SHA256, Bytes: math.MaxInt64})
	if err != nil {
		return 0, err
	}
	var total int64
	for _, amount := range []int64{m.Archive.Bytes, m.Archive.ExpandedBytes, int64(len(fonts) + len(launcher)), 2 * int64(len(g)+1), 2 * int64(len(r)+1)} {
		if amount > math.MaxInt64-total {
			return 0, errors.New("runtime reservation size overflows")
		}
		total += amount
	}
	return total, nil
}

func syncTree(root *os.Root) error {
	return fs.WalkDir(root.FS(), ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return syncDirectory(root, name)
		}
		return nil
	})
}

type contextReader struct {
	ctx    context.Context
	source io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := checkContext(r.ctx); err != nil {
		return 0, err
	}
	return r.source.Read(p)
}
