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
	DefaultIdleRetention     = 30 * 24 * time.Hour
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
	IdleRetention     time.Duration
}

func DefaultConfig(path string) Config {
	return Config{Path: path, MaxBytes: DefaultMaxBytes, PreviousRetention: DefaultPreviousRetention, IdleRetention: DefaultIdleRetention}
}

// Store coordinates local processes. Owners must reconcile accepted demand
// before the first Sweep, and must not launch from generation paths except via Use.
// The directory must remain on one host and may not be copied or network-shared.
type Store struct {
	completedUses map[string]bool
	config        Config
	path          string
	root          *os.Root
	mu            sync.Mutex
	lock          *flock.Flock
	now           func() time.Time
	boot          bootIdentity
	cleanupError  error
}

type generation struct {
	Manifest Manifest  `json:"manifest"`
	LastUsed time.Time `json:"last_used"`
	Unneeded time.Time `json:"unneeded,omitempty"`
}

type Paths struct {
	Node         string
	Dependencies string
	Browser      string
}

func Open(config Config) (*Store, error) {
	if goruntime.GOOS != "linux" || (goruntime.GOARCH != "amd64" && goruntime.GOARCH != "arm64") {
		return nil, errors.New("managed browser runtimes require Linux amd64 or arm64")
	}
	if config.MaxBytes <= 0 || config.PreviousRetention < 0 || config.IdleRetention < 0 {
		return nil, errors.New("invalid runtime storage policy")
	}
	if !filepath.IsAbs(config.Path) {
		return nil, errors.New("runtime store requires an absolute path")
	}
	if err := os.MkdirAll(config.Path, 0700); err != nil {
		return nil, err
	}
	path, err := filepath.EvalSymlinks(config.Path)
	if err != nil {
		return nil, err
	}
	if err := privateDirectory(path); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		return nil, err
	}
	s := &Store{completedUses: make(map[string]bool), config: config, path: path, root: root, now: time.Now, boot: hostBoot()}
	if err := ensureLock(root, "coordination.lock"); err != nil {
		root.Close()
		return nil, err
	}
	s.lock = flock.New(filepath.Join(path, "coordination.lock"))
	err = s.withLock(func() error {
		for _, dir := range []string{"generations", "claims", "uses", "staging", "trash", "initializing"} {
			if err := root.Mkdir(dir, 0700); err != nil && !errors.Is(err, os.ErrExist) {
				return err
			}
			info, err := root.Lstat(dir)
			if err != nil || !info.IsDir() {
				return fmt.Errorf("invalid runtime store directory: %s", dir)
			}
		}
		return syncDirectory(root, ".")
	})
	if err != nil {
		s.Close()
		return nil, err
	}
	return s, nil
}

// Close requires the caller to have joined preparation and released its claims
// and use handles. It does not manufacture completion for retained pins.
func (s *Store) Close() error {
	return errors.Join(s.lock.Close(), s.root.Close())
}

func (s *Store) withLock(fn func() error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.lock.Lock(); err != nil {
		return err
	}
	err := fn()
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
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return errors.New("trailing runtime metadata")
	}
	return nil
}

func saveJSON(root *os.Root, name string, value any) error {
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
	return syncDirectory(root, filepath.Dir(name))
}

func (s *Store) loadGeneration(id string) (generation, error) {
	var g generation
	if !validID(id) {
		return g, errors.New("invalid generation identity")
	}
	name := filepath.Join("generations", id)
	info, err := s.root.Lstat(name)
	if errors.Is(err, os.ErrNotExist) {
		return g, ErrUnavailable
	}
	if err != nil || !info.IsDir() {
		return g, errors.New("invalid generation directory")
	}
	if err := readJSON(s.root, filepath.Join(name, "prepared.json"), &g); err != nil {
		return g, err
	}
	if err := g.Manifest.Validate(); err != nil || g.Manifest.Archive.SHA256 != id || g.LastUsed.IsZero() {
		return g, errors.New("invalid prepared generation metadata")
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
	g, err := s.loadGeneration(m.Archive.SHA256)
	if err != nil {
		return Paths{}, err
	}
	if !sameManifest(g.Manifest, m) {
		return Paths{}, errors.New("prepared runtime manifest differs from installed requirement")
	}
	base := filepath.Join(s.path, "generations", m.Archive.SHA256)
	return Paths{Node: filepath.Join(base, m.Layout.Node), Dependencies: filepath.Join(base, m.Layout.Dependencies), Browser: filepath.Join(base, "chrome")}, nil
}

func checkContext(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	return ctx.Err()
}
