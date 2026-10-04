// SPDX-License-Identifier: GPL-3.0-or-later

// Package artifacts owns optional DEM captures independently of history.
package artifacts

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gofrs/flock"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
)

const FetchMaxBytes int64 = 5 << 20

var (
	ErrClosed         = os.ErrClosed
	ErrNotFound       = errors.New("artifact not found")
	ErrTooLarge       = errors.New("artifact exceeds the 5 MiB fetch limit")
	ErrInvalidCapture = errors.New("invalid artifact capture")
	ErrInvalidID      = errors.New("invalid artifact or run ID")
	ErrExists         = errors.New("artifact run already exists")
	ErrLocked         = errors.New("artifact store is already owned")
	runIDPattern      = regexp.MustCompile(`^[0-9a-f]{32}$`)
	artifactIDPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,127}$`)
)

type Stats struct {
	RetainedBytes  int64  `json:"retained_bytes"`
	ProtectedBytes int64  `json:"protected_bytes"`
	CleanupErrors  uint64 `json:"cleanup_errors"`
}

type manifest struct {
	Version   int                  `json:"version"`
	RunID     string               `json:"run_id"`
	CreatedUS int64                `json:"created_us"`
	Artifacts []synthetic.Artifact `json:"artifacts"`
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
	active    map[string]bool
	uncertain map[string]bool
	stats     Stats
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
	s := &Store{root: root, path: name, temporary: temporary, active: make(map[string]bool), uncertain: make(map[string]bool)}
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
	for _, dir := range []string{"work", "staging", "runs", "drained"} {
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
	if err = s.refresh(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) Begin(runID string) (_ string, retErr error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return "", ErrClosed
	}
	if !runIDPattern.MatchString(runID) {
		return "", ErrInvalidID
	}
	for _, prefix := range []string{"work", "staging", "runs", "drained"} {
		_, err := s.root.Lstat(prefix + "/" + runID)
		if err == nil {
			return "", ErrExists
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return "", err
		}
	}
	if err := s.root.Mkdir("work/"+runID, 0700); err != nil {
		return "", err
	}
	aliasCreated := false
	defer func() {
		if retErr == nil {
			return
		}
		// Ownership was never handed to a child; only this setup's new resources
		// can be rolled back without a drainage marker.
		if aliasCreated {
			retErr = errors.Join(retErr, s.removeWorkAlias(runID))
		}
		retErr = errors.Join(retErr, s.root.Remove("work/"+runID), syncDir(s.root, "work"))
	}()
	if err := syncDir(s.root, "work"); err != nil {
		return "", err
	}
	// Chromium's Unix socket path must fit sockaddr_un (108 bytes on Linux).
	// The short alias contains no data; every file remains under the owned root.
	if err := os.Symlink(filepath.Join(s.path, "work", runID), workAlias(runID)); err != nil {
		return "", fmt.Errorf("create private browser work alias: %w", err)
	}
	aliasCreated = true
	if err := syncExternalDir("/tmp"); err != nil {
		return "", err
	}
	s.active[runID] = true
	return workAlias(runID), nil
}

// Finalize's caller MUST already have verified process-tree drainage. A marker
// outside the child's work folder durably records that permission. Failures before
// publication clean drained work; an attempted publication stays protected until
// both parent directories are synced or a later store owner reconciles disk.
func (s *Store) Finalize(ctx context.Context, runID string, captures []synthetic.Capture) (_ []synthetic.Artifact, retErr error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, ErrClosed
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !runIDPattern.MatchString(runID) {
		return nil, ErrInvalidID
	}
	if !s.active[runID] {
		return nil, ErrNotFound
	}
	delete(s.active, runID)
	s.uncertain[runID] = true
	defer func() { _ = s.refresh() }()
	if err := writeSync(s.root, "drained/"+runID, []byte("1\n")); err != nil {
		return nil, err
	}
	if err := syncDir(s.root, "drained"); err != nil {
		return nil, err
	}
	delete(s.uncertain, runID)
	defer func() {
		if retErr != nil && !s.uncertain[runID] {
			if err := s.cleanupDrained(runID); err != nil {
				s.stats.CleanupErrors++
				retErr = errors.Join(retErr, err)
			}
		}
	}()
	stage := "staging/" + runID
	if err := s.root.Mkdir(stage, 0700); err != nil {
		return nil, err
	}
	if err := syncDir(s.root, "staging"); err != nil {
		return nil, err
	}
	work, err := s.root.OpenRoot("work/" + runID)
	if err != nil {
		return nil, err
	}
	defer work.Close()
	m := manifest{Version: 1, RunID: runID, CreatedUS: time.Now().UnixMicro(), Artifacts: []synthetic.Artifact{}}
	ids := make(map[string]bool)
	for _, capture := range captures {
		if err = ctx.Err(); err != nil {
			return nil, err
		}
		if !artifactIDPattern.MatchString(capture.ID) || ids[capture.ID] {
			return nil, fmt.Errorf("%w: duplicate or invalid ID", ErrInvalidCapture)
		}
		ids[capture.ID] = true
		if !((capture.Kind == "screenshot" && capture.MIME == "image/png") || (capture.Kind == "report" && capture.MIME == "text/html")) {
			return nil, fmt.Errorf("%w: unsupported kind or MIME", ErrInvalidCapture)
		}
		if !strings.HasPrefix(capture.Path, "output/") || path.Clean(capture.Path) != capture.Path || strings.Contains(capture.Path, "\\") {
			return nil, fmt.Errorf("%w: capture must be below output/", ErrInvalidCapture)
		}
		source, err := openRegular(work, capture.Path)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrInvalidCapture, err)
		}
		info, err := source.Stat()
		if err != nil {
			return nil, errors.Join(err, source.Close())
		}
		if info.Size() > FetchMaxBytes {
			return nil, errors.Join(ErrTooLarge, source.Close())
		}
		if capture.Kind == "screenshot" {
			var header [8]byte
			_, err = io.ReadFull(source, header[:])
			if err == nil && string(header[:]) != "\x89PNG\r\n\x1a\n" {
				err = ErrInvalidCapture
			}
			if err == nil {
				_, err = source.Seek(0, io.SeekStart)
			}
			if err != nil {
				_ = source.Close()
				return nil, fmt.Errorf("%w: invalid PNG", ErrInvalidCapture)
			}
		}
		dest, err := s.root.OpenFile(stage+"/"+capture.ID, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			_ = source.Close()
			return nil, err
		}
		hash := sha256.New()
		// Published captures must be fetchable. Bound the copy too, even if the
		// source grows after Stat, rather than trusting the initial size alone.
		count, copyErr := io.Copy(io.MultiWriter(dest, hash), io.LimitReader(&contextReader{ctx: ctx, r: source}, FetchMaxBytes+1))
		if count > FetchMaxBytes {
			copyErr = errors.Join(copyErr, ErrTooLarge)
		}
		closeErr := errors.Join(dest.Sync(), dest.Close(), source.Close())
		if err = errors.Join(copyErr, closeErr); err != nil {
			return nil, err
		}
		m.Artifacts = append(m.Artifacts, synthetic.Artifact{ID: capture.ID, Kind: capture.Kind, MIME: capture.MIME, Bytes: count, SHA256: hex.EncodeToString(hash.Sum(nil))})
	}
	raw, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	if err = writeSync(s.root, stage+"/manifest.json", raw); err != nil {
		return nil, err
	}
	if err = syncDir(s.root, stage); err != nil {
		return nil, err
	}
	s.uncertain[runID] = true
	if err = s.root.Rename(stage, "runs/"+runID); err != nil {
		return nil, err
	}
	if err = errors.Join(syncDir(s.root, "runs"), syncDir(s.root, "staging")); err != nil {
		return nil, err
	}
	delete(s.uncertain, runID)
	// Work includes scripts, browser profiles and any unselected attachments.
	if err = s.cleanupDrained(runID); err != nil {
		s.stats.CleanupErrors++
	}
	return m.Artifacts, nil
}

func (s *Store) Fetch(ctx context.Context, runID, artifactID string) (synthetic.Artifact, []byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return synthetic.Artifact{}, nil, ErrClosed
	}
	if err := ctx.Err(); err != nil {
		return synthetic.Artifact{}, nil, err
	}
	if !runIDPattern.MatchString(runID) || !artifactIDPattern.MatchString(artifactID) {
		return synthetic.Artifact{}, nil, ErrInvalidID
	}
	m, err := s.readManifest(runID)
	if err != nil {
		return synthetic.Artifact{}, nil, err
	}
	var found *synthetic.Artifact
	for i := range m.Artifacts {
		if m.Artifacts[i].ID == artifactID {
			found = &m.Artifacts[i]
			break
		}
	}
	if found == nil {
		return synthetic.Artifact{}, nil, ErrNotFound
	}
	if found.Bytes > FetchMaxBytes {
		return *found, nil, ErrTooLarge
	}
	file, err := openRegular(s.root, "runs/"+runID+"/"+artifactID)
	if errors.Is(err, fs.ErrNotExist) {
		return *found, nil, ErrNotFound
	}
	if err != nil {
		return *found, nil, err
	}
	raw, readErr := io.ReadAll(io.LimitReader(&contextReader{ctx: ctx, r: file}, FetchMaxBytes+1))
	err = errors.Join(readErr, file.Close())
	if err != nil {
		return *found, nil, err
	}
	if int64(len(raw)) > FetchMaxBytes {
		return *found, nil, ErrTooLarge
	}
	sum := sha256.Sum256(raw)
	if int64(len(raw)) != found.Bytes || hex.EncodeToString(sum[:]) != found.SHA256 {
		return *found, nil, fmt.Errorf("artifact content does not match its manifest")
	}
	return *found, raw, nil
}

// Manifest returns immutable metadata without reading capture contents.
func (s *Store) Manifest(ctx context.Context, runID string) ([]synthetic.Artifact, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, ErrClosed
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !runIDPattern.MatchString(runID) {
		return nil, ErrInvalidID
	}
	m, err := s.readManifest(runID)
	if err != nil {
		return nil, err
	}
	return m.Artifacts, nil
}

// Enforce applies a shared age/byte budget. Protected work can exceed it;
// this is deliberately not a hard filesystem ceiling. Missing retained files
// do not prove expiry as distinct from unavailable or manually removed data.
func (s *Store) Enforce(ctx context.Context, days int, maxBytes int64) (Stats, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return s.stats, ErrClosed
	}
	if err := ctx.Err(); err != nil {
		return s.stats, err
	}
	const maxDays = int64(^uint64(0)>>1) / int64(24*time.Hour)
	if days < 0 || int64(days) > maxDays || maxBytes < 0 {
		return s.stats, fmt.Errorf("invalid artifact retention policy")
	}
	var result error
	markers, err := fs.ReadDir(s.root.FS(), "drained")
	if err != nil {
		return s.stats, err
	}
	for _, entry := range markers {
		id := entry.Name()
		if !runIDPattern.MatchString(id) || s.active[id] || s.uncertain[id] {
			continue
		}
		if err := ctx.Err(); err != nil {
			return s.stats, errors.Join(result, err)
		}
		if err := s.cleanupDrained(id); err != nil {
			s.stats.CleanupErrors++
			result = errors.Join(result, err)
		}
	}
	entries, err := fs.ReadDir(s.root.FS(), "runs")
	if err != nil {
		return s.stats, errors.Join(result, err)
	}
	var manifests []manifest
	for _, entry := range entries {
		if !runIDPattern.MatchString(entry.Name()) || s.uncertain[entry.Name()] {
			continue
		}
		m, err := s.readManifest(entry.Name())
		if err != nil {
			result = errors.Join(result, err)
			continue
		}
		manifests = append(manifests, m)
	}
	sort.Slice(manifests, func(i, j int) bool { return manifests[i].CreatedUS < manifests[j].CreatedUS })
	if err := s.refresh(); err != nil {
		return s.stats, errors.Join(result, err)
	}
	cutoff := time.Now().Add(-time.Duration(days) * 24 * time.Hour).UnixMicro()
	for _, m := range manifests {
		if err := ctx.Err(); err != nil {
			return s.stats, errors.Join(result, err)
		}
		if !(days > 0 && m.CreatedUS < cutoff) && !(maxBytes > 0 && s.stats.RetainedBytes+s.stats.ProtectedBytes > maxBytes) {
			continue
		}
		before, err := treeBytes(s.root, "runs/"+m.RunID)
		if err != nil {
			result = errors.Join(result, err)
			continue
		}
		if err = s.expire(m); err != nil {
			s.stats.CleanupErrors++
			result = errors.Join(result, err)
			after, scanErr := treeBytes(s.root, "runs/"+m.RunID)
			if errors.Is(scanErr, fs.ErrNotExist) {
				after, scanErr = 0, nil
			}
			if scanErr != nil {
				result = errors.Join(result, scanErr)
				break
			}
			s.stats.RetainedBytes -= before - after
		} else {
			s.stats.RetainedBytes -= before
		}

	}
	return s.stats, result
}

func (s *Store) Stats() Stats {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.closed {
		if err := s.refresh(); err != nil {
			s.stats.CleanupErrors++
		}
	}
	return s.stats
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
		for _, dir := range []string{"work", "staging", "drained"} {
			entries, e := fs.ReadDir(s.root.FS(), dir)
			err = errors.Join(err, e)
			if e != nil || len(entries) != 0 {
				removable = false
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

func (s *Store) readManifest(id string) (manifest, error) {
	if s.uncertain[id] {
		return manifest{}, fmt.Errorf("artifact publication is uncertain")
	}
	file, err := openRegular(s.root, "runs/"+id+"/manifest.json")
	if errors.Is(err, fs.ErrNotExist) {
		return manifest{}, ErrNotFound
	}
	if err != nil {
		return manifest{}, err
	}
	var m manifest
	err = errors.Join(json.NewDecoder(file).Decode(&m), file.Close())
	if err != nil {
		return manifest{}, err
	}
	if m.Version != 1 || m.RunID != id || m.CreatedUS <= 0 {
		return manifest{}, fmt.Errorf("invalid artifact manifest")
	}
	ids := make(map[string]bool)
	for _, a := range m.Artifacts {
		if !artifactIDPattern.MatchString(a.ID) || ids[a.ID] || a.Bytes < 0 || len(a.SHA256) != 64 || !((a.Kind == "screenshot" && a.MIME == "image/png") || (a.Kind == "report" && a.MIME == "text/html")) {
			return manifest{}, fmt.Errorf("invalid artifact manifest entry")
		}
		ids[a.ID] = true
	}
	return m, nil
}

func (s *Store) expire(m manifest) error {
	if err := s.root.RemoveAll("runs/" + m.RunID); err != nil {
		return err
	}
	return syncDir(s.root, "runs")
}

func (s *Store) cleanupDrained(id string) error {
	marker, err := openRegular(s.root, "drained/"+id)
	if err != nil {
		return err
	}
	raw, err := io.ReadAll(io.LimitReader(marker, 3))
	err = errors.Join(err, marker.Close())
	if err != nil {
		return err
	}
	if string(raw) != "1\n" {
		return fmt.Errorf("invalid drained marker")
	}
	if err := s.removeWorkAlias(id); err != nil {
		return err
	}
	for _, dir := range []string{"work", "staging"} {
		if err := s.root.RemoveAll(dir + "/" + id); err != nil {
			return err
		}
		if err := syncDir(s.root, dir); err != nil {
			return err
		}
	}
	if err := s.root.Remove("drained/" + id); err != nil {
		return err
	}
	return syncDir(s.root, "drained")
}

func (s *Store) removeWorkAlias(id string) error {
	alias := workAlias(id)
	target, linkErr := os.Readlink(alias)
	if linkErr == nil {
		if target != filepath.Join(s.path, "work", id) {
			return fmt.Errorf("browser work alias ownership does not match")
		}
		if err := os.Remove(alias); err != nil {
			return err
		}
		if err := syncExternalDir("/tmp"); err != nil {
			return err
		}
	} else if !errors.Is(linkErr, fs.ErrNotExist) {
		return linkErr
	}
	return nil
}

func (s *Store) refresh() error {
	retained, protected := int64(0), int64(0)
	for _, dir := range []string{"runs", "work", "staging", "drained"} {
		err := fs.WalkDir(s.root.FS(), dir, func(name string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				return nil
			}
			info, err := entry.Info()
			if err != nil {
				return err
			}
			if dir == "runs" && !s.uncertain[strings.Split(name, "/")[1]] {
				retained += info.Size()
			} else {
				protected += info.Size()
			}
			return nil
		})
		if err != nil {
			return err
		}
	}
	s.stats.RetainedBytes = retained
	s.stats.ProtectedBytes = protected
	return nil
}

func workAlias(id string) string { return filepath.Join("/tmp", "nd-dem-"+id) }
func syncExternalDir(name string) error {
	file, err := os.Open(name)
	if err != nil {
		return err
	}
	return errors.Join(file.Sync(), file.Close())
}

func treeBytes(root *os.Root, dir string) (int64, error) {
	var bytes int64
	err := fs.WalkDir(root.FS(), dir, func(_ string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		bytes += info.Size()
		return nil
	})
	return bytes, err
}

// Each newly created ancestor needs its parent entry synced on first use.
func makeStoreDir(name string) error {
	var dirs []string
	for current := name; ; current = filepath.Dir(current) {
		_, err := os.Stat(current)
		if err == nil {
			dirs = append(dirs, current)
			break
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		dirs = append(dirs, current)
		if filepath.Dir(current) == current {
			return err
		}
	}
	if err := os.MkdirAll(name, 0700); err != nil {
		return err
	}
	for _, dir := range dirs {
		f, err := os.Open(dir)
		if err != nil {
			return err
		}
		if err = errors.Join(f.Sync(), f.Close()); err != nil {
			return err
		}
	}
	return nil
}

func openRegular(root *os.Root, name string) (*os.File, error) {
	if !fs.ValidPath(name) {
		return nil, ErrInvalidCapture
	}
	parts := strings.Split(name, "/")
	for i := range parts {
		info, err := root.Lstat(strings.Join(parts[:i+1], "/"))
		if err != nil {
			return nil, err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("symbolic links are not artifacts")
		}
		if i < len(parts)-1 && !info.IsDir() {
			return nil, fmt.Errorf("artifact parent is not a directory")
		}
		if i == len(parts)-1 && !info.Mode().IsRegular() {
			return nil, fmt.Errorf("artifact is not a regular file")
		}
	}
	file, err := root.Open(name)
	if err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		_ = file.Close()
		return nil, fmt.Errorf("artifact is not a regular file: %v", err)
	}
	return file, nil
}
func writeSync(root *os.Root, name string, raw []byte) error {
	file, err := root.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_, writeErr := file.Write(raw)
	return errors.Join(writeErr, file.Sync(), file.Close())
}
func syncDir(root *os.Root, name string) error {
	file, err := root.Open(name)
	if err != nil {
		return err
	}
	return errors.Join(file.Sync(), file.Close())
}

type contextReader struct {
	ctx context.Context
	r   io.Reader
}

func (r *contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.r.Read(p)
}
