// SPDX-License-Identifier: GPL-3.0-or-later

package runtime

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const (
	DefaultMaxBytes          = int64(4 << 30)
	DefaultPreviousRetention = 7 * 24 * time.Hour
)

var (
	ErrUnavailable = errors.New("matching browser runtime is not prepared")
	ErrPreparing   = errors.New("matching browser runtime is being prepared")
	ErrAllowance   = errors.New("managed runtime allowance cannot accommodate preparation")
)

// AllowanceError reports the coordinated inventory at the failed reservation.
// ReclaimableBytes may include trash whose deletion failed; it is not free space.
type AllowanceError struct {
	NeededBytes int64
	Status      Status
}

func (e *AllowanceError) Error() string {
	return fmt.Sprintf("%s: need %d bytes, limit %d, used %d, reserved %d, protected %d, reclaimable %d", ErrAllowance, e.NeededBytes, e.Status.LimitBytes, e.Status.UsedBytes, e.Status.ReservedBytes, e.Status.ProtectedBytes, e.Status.ReclaimableBytes)
}

func (e *AllowanceError) Unwrap() error { return ErrAllowance }

type Config struct {
	Path              string
	MaxBytes          int64
	PreviousRetention time.Duration
}

func DefaultConfig(path string) Config {
	return Config{
		Path:              path,
		MaxBytes:          DefaultMaxBytes,
		PreviousRetention: DefaultPreviousRetention,
	}
}

// Store coordinates durable installation, process consumers and execution pins.
// Executable access requires an acquired Consumer or Candidate.
// The directory must remain on one host and may not be copied or network-shared.
type Store struct {
	// path and root name the managed store. The ledger shares root and closes it;
	// Store methods add the managed checks, so ledger calls stay explicit.
	path          string
	root          *os.Root
	ledger        *ownershipLedger
	config        Config
	now           func() time.Time
	syncSelection func() error // Directory durability barrier before selection-dependent operations.
}

type generation struct {
	Manifest Manifest  `json:"manifest"`
	LastUsed time.Time `json:"last_used"`
	Unneeded time.Time `json:"unneeded,omitempty"`
}

func Open(config Config) (*Store, error) {
	if !SupportedHost() {
		return nil, errors.New("managed browser runtimes require Linux amd64 or arm64")
	}
	if config.MaxBytes <= 0 || config.PreviousRetention < 0 {
		return nil, errors.New("invalid runtime storage policy")
	}
	if !filepath.IsAbs(config.Path) {
		return nil, errors.New("runtime store requires an absolute path")
	}
	path, root, err := openRuntimeRoot(config.Path, managedRootLayout)
	if err != nil {
		return nil, err
	}
	s := &Store{
		path:   path,
		root:   root,
		ledger: newOwnershipLedger(path, root),
		config: config,
		now:    time.Now,
	}
	if err := requireManaged(root); err != nil {
		root.Close()
		return nil, err
	}
	s.syncSelection = func() error { return syncDirectory(root, ".") }
	err = s.withLock(func() error { return managedRootLayout.validate(root) })
	if err != nil {
		s.Close()
		return nil, err
	}
	return s, nil
}

// Close requires the caller to have joined preparation and released its claims
// and use handles. It does not manufacture completion for retained pins.
func (s *Store) Close() error { return s.ledger.Close() }

func (s *Store) withLock(fn func() error) error {
	return s.ledger.withLock(func() error {
		if err := requireManaged(s.root); err != nil {
			return err
		}
		return fn()
	})
}

func requireManaged(root *os.Root) error {
	if _, err := root.Lstat("preload.json"); !errors.Is(err, os.ErrNotExist) {
		return errors.Join(errors.New("protected preload cannot be opened as a managed cache"), err)
	}
	return nil
}

func (s *Store) loadGeneration(id string) (generation, error) {
	return loadGeneration(s.root, id)
}

func loadGeneration(root *os.Root, id string) (generation, error) {
	var g generation
	if !validID(id) {
		return g, errors.New("invalid generation identity")
	}
	name := filepath.Join("generations", id)
	info, err := root.Lstat(name)
	if errors.Is(err, os.ErrNotExist) {
		return g, ErrUnavailable
	}
	if err != nil {
		return g, err
	}
	if !info.IsDir() {
		return g, errors.New("invalid generation directory")
	}
	metadata := filepath.Join(name, "prepared.json")
	if err := readJSON(root, metadata, &g); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			err = damagedMetadata(metadata, err)
		}
		return g, err
	}
	if err := g.Manifest.Validate(); err != nil || g.Manifest.Archive.SHA256 != id || g.LastUsed.IsZero() {
		return g, damagedMetadata(metadata, errors.New("invalid prepared generation metadata"))
	}
	return g, nil
}

func (s *Store) prepared(m Manifest) (Paths, error) {
	return preparedPaths(s.root, s.path, m)
}

func preparedPaths(root *os.Root, path string, m Manifest) (Paths, error) {
	g, err := loadGeneration(root, m.Archive.SHA256)
	if err != nil {
		return Paths{}, err
	}
	if !sameExecutable(g.Manifest, m) {
		return Paths{}, damagedMetadata("selection.json", errors.New("prepared runtime manifest differs from installed requirement"))
	}
	return boundPaths(filepath.Join(path, "generations", m.Archive.SHA256), m.Layout), nil
}

// Use protects every access that may execute the managed runtime, including
// version probes. Pass Completion to ndexec, then Finish only after joining it.
// If launch never started, the owning caller may Finish(true). Lost callers
// cannot make that assertion; recovery requires a receipt or verified host reboot.
type Use struct {
	*executionLease
	Paths Paths
}

func (s *Store) beginUse(m Manifest) (*Use, error) {
	if err := m.Validate(); err != nil {
		return nil, err
	}
	var use *Use
	err := s.withLock(func() (result error) {
		paths, err := s.prepared(m)
		if err != nil {
			return err
		}
		lease, err := s.ledger.newUseLocked(m.Archive.SHA256)
		if err != nil {
			return err
		}
		defer func() {
			if result != nil {
				result = errors.Join(result, lease.abortLocked())
			}
		}()
		g, err := s.loadGeneration(m.Archive.SHA256)
		if err == nil {
			g.LastUsed = s.now().UTC()
			err = saveJSON(s.root, filepath.Join("generations", m.Archive.SHA256, "prepared.json"), g)
		}
		if err != nil {
			return err
		}
		if err := lease.publishLocked(); err != nil {
			return err
		}
		use = &Use{Paths: paths, executionLease: lease}
		return nil
	})
	if err != nil && use != nil {
		// No executable path escaped to the caller.
		return nil, errors.Join(err, use.Finish(true))
	}
	return use, err
}

// closeClaim preserves managed reconciliation after releasing common proof.
func (s *Store) closeClaim(c *claim) error {
	retired, err := c.close()
	if retired && err == nil {
		err = s.withLock(func() error { _, err := s.inventoryLocked(true); return err })
		s.ledger.recordCleanupError(err)
	}
	return err
}

// Managed payloads share trash with proof records, but their publication
// barriers remain the responsibility of Store.
func (s *Store) removeOwned(name string) error {
	switch filepath.Dir(name) {
	case "claims", "uses", "initializing":
		return s.ledger.removeOwned(name)
	case "staging":
		if _, err := s.root.Lstat(name); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		} else if err == nil {
			if err := errors.Join(syncDirectory(s.root, name), syncDirectory(s.root, "generations")); err != nil {
				return err
			}
		}
	case "generations":
	default:
		return errors.New("invalid managed retirement namespace")
	}
	return retireToTrash(s.root, name)
}
