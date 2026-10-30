// SPDX-License-Identifier: GPL-3.0-or-later

package runtime

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	goruntime "runtime"
	"sync"
	"time"

	"github.com/gofrs/flock"
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
	completedUses       map[string]bool
	releasingOwners     map[io.Closer]struct{} // Failed local lock closes only; dead records belong to inventory.
	config              Config
	path                string
	root                *os.Root
	mu                  sync.Mutex
	lock                *flock.Flock
	now                 func() time.Time
	boot                bootIdentity
	cleanupError        error
	cleanupErrorVersion uint64
	syncSelection       func() error // Directory durability barrier before selection-dependent operations.
}

type generation struct {
	Manifest Manifest  `json:"manifest"`
	LastUsed time.Time `json:"last_used"`
	Unneeded time.Time `json:"unneeded,omitempty"`
}

type Paths struct {
	EntryPoint   string
	Node         string
	Dependencies string
	Browser      string
}

func Open(config Config) (*Store, error) {
	if goruntime.GOOS != "linux" || (goruntime.GOARCH != "amd64" && goruntime.GOARCH != "arm64") {
		return nil, errors.New("managed browser runtimes require Linux amd64 or arm64")
	}
	if config.MaxBytes <= 0 || config.PreviousRetention < 0 {
		return nil, errors.New("invalid runtime storage policy")
	}
	if !filepath.IsAbs(config.Path) {
		return nil, errors.New("runtime store requires an absolute path")
	}
	path, root, err := openStoreRoot(config.Path)
	if err != nil {
		return nil, err
	}
	s := &Store{
		completedUses:   make(map[string]bool),
		releasingOwners: make(map[io.Closer]struct{}),
		config:          config,
		path:            path,
		root:            root,
		now:             time.Now,
		boot:            hostBoot(),
	}
	if err := requireManaged(root); err != nil {
		root.Close()
		return nil, err
	}
	s.syncSelection = func() error { return syncDirectory(root, ".") }
	s.lock = flock.New(filepath.Join(path, "coordination.lock"))
	err = s.withLock(func() error { return validateStoreLayout(root) })
	if err != nil {
		s.Close()
		return nil, err
	}
	return s, nil
}

// Close requires the caller to have joined preparation and released its claims
// and use handles. It does not manufacture completion for retained pins.
func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := errors.Join(s.retryOwnerReleasesLocked(), s.lock.Close()); err != nil {
		return err
	}
	return s.root.Close()
}

func (s *Store) withLock(fn func() error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.lock.Lock(); err != nil {
		return err
	}
	err := requireManaged(s.root)
	if err == nil {
		err = fn()
	}
	return errors.Join(err, s.lock.Unlock())
}

func randomID() (string, error) {
	var value [32]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(value[:]), nil
}

func ensureLock(root *os.Root, name string) error {
	f, err := root.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err == nil {
		err = errors.Join(f.Sync(), f.Close())
	} else if errors.Is(err, os.ErrExist) {
		info, statErr := root.Lstat(name)
		if statErr != nil {
			return statErr
		}
		if !info.Mode().IsRegular() || info.Mode().Perm() != 0600 || info.Size() != 0 {
			return errors.New("invalid runtime lock file")
		}
		return nil
	}
	return err
}

func syncDirectory(root *os.Root, name string) error {
	f, err := root.Open(name)
	if err != nil {
		return err
	}
	return errors.Join(f.Sync(), f.Close())
}

func readJSON(root *os.Root, name string, value any) error {
	info, err := root.Lstat(name)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return errors.New("runtime metadata is not a regular file")
	}
	f, err := root.Open(name)
	if err != nil {
		return err
	}
	defer f.Close()
	decoder := json.NewDecoder(f)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return metadataDecodeError(name, err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err != nil {
			return metadataDecodeError(name, err)
		}
		return damagedMetadata(name, errors.New("trailing runtime metadata"))
	}
	return nil
}

func saveJSON(root *os.Root, name string, value any) error {
	if err := publishJSON(root, name, value); err != nil {
		return err
	}
	return syncDirectory(root, filepath.Dir(name))
}

func publishJSON(root *os.Root, name string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	id, err := randomID()
	if err != nil {
		return err
	}
	temporary := name + "." + id
	if err := writeNewFile(root, temporary, append(data, '\n'), 0600); err != nil {
		return err
	}
	if err := root.Rename(temporary, name); err != nil {
		return err
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

func sameManifest(a, b Manifest) bool {
	// Mirrors may change transport without changing executable identity.
	a.Archive.URL, b.Archive.URL = "", ""
	left, errA := json.Marshal(a)
	right, errB := json.Marshal(b)
	return errA == nil && errB == nil && string(left) == string(right)
}

func (s *Store) prepared(m Manifest) (Paths, error) {
	return preparedPaths(s.root, s.path, m)
}

func preparedPaths(root *os.Root, path string, m Manifest) (Paths, error) {
	g, err := loadGeneration(root, m.Archive.SHA256)
	if err != nil {
		return Paths{}, err
	}
	if !sameManifest(g.Manifest, m) {
		return Paths{}, damagedMetadata("selection.json", errors.New("prepared runtime manifest differs from installed requirement"))
	}
	base := filepath.Join(path, "generations", m.Archive.SHA256)
	return Paths{
		EntryPoint:   filepath.Join(base, m.Layout.EntryPoint),
		Node:         filepath.Join(base, m.Layout.Node),
		Dependencies: filepath.Join(base, m.Layout.Dependencies),
		Browser:      filepath.Join(base, "chrome"),
	}, nil
}

func checkContext(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	return ctx.Err()
}

// Available verifies prepared identity without acquiring execution ownership or
// changing retention timestamps. Consumer.BeginUse must precede executable access.
func (s *Store) Available(m Manifest) error {
	if err := m.Validate(); err != nil {
		return err
	}
	return s.withLock(func() error {
		_, err := s.prepared(m)
		return err
	})
}

func requireManaged(root *os.Root) error {
	if _, err := root.Lstat("preload.json"); !errors.Is(err, os.ErrNotExist) {
		return errors.Join(errors.New("protected preload cannot be opened as a managed cache"), err)
	}
	return nil
}
