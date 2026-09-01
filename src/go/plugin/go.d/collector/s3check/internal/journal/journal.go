// SPDX-License-Identifier: GPL-3.0-or-later

package journal

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/netdata/netdata/go/plugins/plugin/framework/filelock"
)

const (
	stateVersion  = 1
	maxStateBytes = 64 * 1024
)

var (
	ErrNotLocked           = errors.New("s3check journal is not locked")
	ErrFingerprintMismatch = errors.New("s3check journal fingerprint mismatch")
	ErrStateTooLarge       = errors.New("s3check journal state is too large")
)

type envelope struct {
	Version     int             `json:"version"`
	OwnerID     string          `json:"owner_id"`
	Fingerprint string          `json:"fingerprint"`
	State       json.RawMessage `json:"state"`
}

// Journal persists one job owner's recovery state and owns its lifetime lock.
// Loading is read-only; Save and Clear require the caller to hold the lock.
type Journal struct {
	path        string
	ownerID     string
	fingerprint string
	locker      *filelock.Locker

	mu     sync.Mutex
	locked bool
}

func New(root, agentID, jobName, fingerprint string) (*Journal, error) {
	switch {
	case root == "":
		return nil, errors.New("journal root is required")
	case strings.TrimSpace(agentID) == "":
		return nil, errors.New("persisted Agent identity is required")
	case strings.TrimSpace(agentID) != agentID:
		return nil, errors.New("persisted Agent identity must not contain surrounding whitespace")
	case strings.TrimSpace(jobName) == "":
		return nil, errors.New("job name is required")
	case strings.TrimSpace(jobName) != jobName:
		return nil, errors.New("job name must not contain surrounding whitespace")
	case strings.TrimSpace(fingerprint) != fingerprint || !isSHA256Hex(fingerprint):
		return nil, errors.New("config fingerprint must be a lowercase SHA-256 hash")
	}

	ownerID := digest("s3check-owner-v1", agentID, jobName)
	return &Journal{
		path:        filepath.Join(root, ownerID+".json"),
		ownerID:     ownerID,
		fingerprint: fingerprint,
		locker:      filelock.New(root),
	}, nil
}

func Fingerprint(parts ...string) string {
	values := make([]string, 0, len(parts)+1)
	values = append(values, "s3check-config-v1")
	values = append(values, parts...)
	return digest(values...)
}

func digest(parts ...string) string {
	hash := sha256.New()
	for _, part := range parts {
		_, _ = fmt.Fprintf(hash, "%d:", len(part))
		_, _ = hash.Write([]byte(part))
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func isSHA256Hex(value string) bool {
	if len(value) != sha256.Size*2 || strings.ToLower(value) != value {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func (j *Journal) OwnerID() string { return j.ownerID }

func (j *Journal) Path() string { return j.path }

// Load reads a complete atomically published state without creating files or directories.
func (j *Journal) Load(dst any) (bool, error) {
	if dst == nil {
		return false, errors.New("journal load destination is nil")
	}
	file, err := os.Open(j.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("open s3check journal: %w", err)
	}
	defer file.Close() //nolint:errcheck // The bounded read leaves no recovery action for Close.

	raw, err := io.ReadAll(io.LimitReader(file, maxStateBytes+1))
	if err != nil {
		return false, fmt.Errorf("read s3check journal: %w", err)
	}
	if len(raw) > maxStateBytes {
		return false, ErrStateTooLarge
	}

	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return false, fmt.Errorf("decode s3check journal: %w", err)
	}
	switch {
	case env.Version != stateVersion:
		return false, fmt.Errorf("unsupported s3check journal version %d", env.Version)
	case env.OwnerID != j.ownerID:
		return false, errors.New("s3check journal owner mismatch")
	case env.Fingerprint != j.fingerprint:
		return false, ErrFingerprintMismatch
	case len(env.State) == 0 || string(env.State) == "null":
		return false, errors.New("s3check journal has no state")
	}
	if err := json.Unmarshal(env.State, dst); err != nil {
		return false, fmt.Errorf("decode s3check journal state: %w", err)
	}
	return true, nil
}

// TryLock acquires the one lifetime lock for this durable owner.
func (j *Journal) TryLock() (bool, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.locked {
		return true, nil
	}
	if err := ensurePrivateDir(filepath.Dir(j.path)); err != nil {
		return false, err
	}
	locked, err := j.locker.Lock(j.ownerID)
	if err != nil {
		return false, fmt.Errorf("lock s3check journal: %w", err)
	}
	j.locked = locked
	return locked, nil
}

func (j *Journal) Unlock() {
	j.mu.Lock()
	defer j.mu.Unlock()
	if !j.locked {
		return
	}
	j.locker.Unlock(j.ownerID)
	j.locked = false
}

func (j *Journal) Save(state any) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	if !j.locked {
		return ErrNotLocked
	}
	payload, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("encode s3check journal state: %w", err)
	}
	if string(payload) == "null" {
		return errors.New("s3check journal state is nil")
	}
	raw, err := json.MarshalIndent(envelope{
		Version:     stateVersion,
		OwnerID:     j.ownerID,
		Fingerprint: j.fingerprint,
		State:       payload,
	}, "", "  ")
	if err != nil {
		return fmt.Errorf("encode s3check journal: %w", err)
	}
	raw = append(raw, '\n')
	if len(raw) > maxStateBytes {
		return ErrStateTooLarge
	}
	return writeAtomic(j.path, raw)
}

func (j *Journal) Clear() error {
	j.mu.Lock()
	defer j.mu.Unlock()
	if !j.locked {
		return ErrNotLocked
	}
	if err := os.Remove(j.path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove s3check journal: %w", err)
	}
	if err := os.Remove(j.path + ".tmp"); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove s3check temporary journal: %w", err)
	}
	dir := filepath.Dir(j.path)
	if _, err := os.Stat(dir); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return syncDir(dir)
}

func writeAtomic(path string, raw []byte) (retErr error) {
	dir := filepath.Dir(path)
	if err := ensurePrivateDir(dir); err != nil {
		return err
	}
	tmpPath := path + ".tmp"
	file, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("create s3check temporary journal: %w", err)
	}
	defer func() {
		if retErr != nil {
			_ = os.Remove(tmpPath)
		}
	}()
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return fmt.Errorf("restrict s3check temporary journal: %w", err)
	}
	if _, err := file.Write(raw); err != nil {
		_ = file.Close()
		return fmt.Errorf("write s3check temporary journal: %w", err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return fmt.Errorf("sync s3check temporary journal: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close s3check temporary journal: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("publish s3check journal: %w", err)
	}
	return syncDir(dir)
}

func ensurePrivateDir(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create s3check journal directory: %w", err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return fmt.Errorf("restrict s3check journal directory: %w", err)
	}
	return nil
}

func syncDir(dir string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	handle, err := os.Open(dir)
	if err != nil {
		return fmt.Errorf("open s3check journal directory: %w", err)
	}
	syncErr := handle.Sync()
	closeErr := handle.Close()
	if syncErr != nil {
		return fmt.Errorf("sync s3check journal directory: %w", syncErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close s3check journal directory: %w", closeErr)
	}
	return nil
}
