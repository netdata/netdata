// SPDX-License-Identifier: GPL-3.0-or-later

// Package store owns DEM investigation journals. The command owns its lifetime;
// jobs hand it immutable, already redacted records.
package store

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/netdata/systemd-journal-sdk/go/journal"
	"github.com/netdata/systemd-journal-sdk/go/journalhost"
)

// Store serializes the SDK writer, including reader snapshot construction.
// Independent readers scan outside admission, so a history scan does not hold
// the writer for its entire duration. Snapshot file handles remain open until
// the query completes, including when retention unlinks an archive.
type Store struct {
	gate            chan struct{}
	root, temporary string
	host            *journalhost.Provider
	config          journal.LogConfig
	log             *journal.Log
	closed          bool
	readers         sync.WaitGroup
}

// Open opens one command-owned journal chain. An empty path uses private
// temporary journal and identity state, removed by Close after readers finish.
// Host identity files are read through journalhost; fallback state is confined
// to this store's identity directory.
func Open(ctx context.Context, path string) (*Store, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	temporary := ""
	if path == "" {
		var err error
		path, err = os.MkdirTemp("", "netdata-dem-journal-")
		if err != nil {
			return nil, err
		}
		temporary = path
	}
	st := &Store{
		gate:      make(chan struct{}, 1),
		root:      path,
		temporary: temporary,
	}
	ok := false
	defer func() {
		if !ok && temporary != "" {
			_ = os.RemoveAll(temporary)
		}
	}()
	if err := os.MkdirAll(path, 0750); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Join(path, "identity"), 0700); err != nil {
		return nil, err
	}
	host, err := journalhost.Load(journalhost.LoadOptions{
		StateDir: filepath.Join(path, "identity"),
	})
	if err != nil {
		return nil, fmt.Errorf("load journal identity: %w", err)
	}
	st.host = host
	st.config = journal.LogConfig{
		Source:       "dem",
		IdentityMode: journal.LogIdentityStrict,
		OpenMode:     journal.LogOpenEager,
		Options: journal.Options{
			MachineID: host.MachineID(),
			BootID:    host.BootID(),
			Compact:   true,
		},
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	st.log, err = journal.NewLog(path, st.config)
	if err != nil {
		return nil, fmt.Errorf("open history journal: %w", err)
	}
	ok = true
	return st, nil
}

func (s *Store) acquire(ctx context.Context) error {
	select {
	case s.gate <- struct{}{}:
		if err := ctx.Err(); err != nil {
			s.release()
			return err
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (s *Store) release() { <-s.gate }

func (s *Store) Close() error {
	_ = s.acquire(context.Background())
	defer s.release()
	if s.closed {
		return nil
	}
	s.closed = true
	s.readers.Wait()
	var err error
	if s.log != nil {
		err = s.log.Close()
	}
	if s.temporary != "" {
		err = errors.Join(err, os.RemoveAll(s.temporary))
	}
	return err
}

// EnforceHistoryRetention applies SDK whole-file policies. Closing and
// lazily reopening archives idle activity, so age expiry also runs without
// new events. The active file remains protected, so maxBytes is not a hard cap.
// MaxBytes measures committed journal bytes, excluding filesystem preallocation.
// Age is measured from each file's saved-time head, not from each event.
func (s *Store) EnforceHistoryRetention(ctx context.Context, days int, maxBytes int64) error {
	if err := s.acquire(ctx); err != nil {
		return err
	}
	defer s.release()
	if s.closed {
		return os.ErrClosed
	}
	policy := journal.RetentionPolicy{}
	if days > 0 {
		const maxDays = int64(^uint64(0)>>1) / int64(24*time.Hour)
		if int64(days) > maxDays {
			return fmt.Errorf("history retention days overflow: %d", days)
		}
		policy = policy.WithMaxAge(time.Duration(days) * 24 * time.Hour)
	}
	if maxBytes > 0 {
		policy = policy.WithMaxBytes(uint64(maxBytes))
	}
	// Reopening applies the new policy; closing must not run stale limits.
	if s.log != nil {
		if err := s.log.CloseWithoutRetention(); err != nil {
			return fmt.Errorf("archive history journal: %w", err)
		}
		s.log = nil
	}
	s.config.RetentionPolicy = policy
	// NewLog derives rotation thresholds from the retention budget. Keep its
	// writer lazy, and own the log before retention can fail. Idle sweeps need
	// no new active file; the next append creates one with a fresh time head.
	config := s.config
	config.OpenMode = journal.LogOpenLazy
	log, err := journal.NewLog(s.root, config)
	if err != nil {
		return fmt.Errorf("reopen history journal: %w", err)
	}
	s.log = log
	if err := s.log.EnforceRetention(); err != nil {
		return fmt.Errorf("enforce history retention: %w", err)
	}
	return nil
}

// openReader takes a stable snapshot before an append/rotation/sweep can change
// the file set. SDK snapshot construction stores O(retained entries) offsets;
// reducers stream rows and do not retain another complete event copy.
func (s *Store) openReader(ctx context.Context) (*journalSnapshot, func(), error) {
	if err := s.acquire(ctx); err != nil {
		return nil, nil, err
	}
	defer s.release()
	if s.closed {
		return nil, nil, os.ErrClosed
	}
	paths, err := s.journalPaths(ctx)
	if err != nil {
		return nil, nil, err
	}
	if len(paths) == 0 {
		return nil, func() {}, nil
	}
	// OpenFiles reports unreadable/corrupt owned files; OpenDirectory silently
	// skips them and could label an incomplete aggregate as exact.
	reader := &journalSnapshot{
		ctx: ctx,
	}
	for _, path := range paths {
		if err := ctx.Err(); err != nil {
			_ = reader.Close()
			return nil, nil, err
		}
		// SDK OpenFiles leaks earlier readers when opening a later file fails.
		// Own each single-file reader so all successful opens can be closed.
		file, err := journal.OpenFilesWithOptions([]string{path}, journal.DefaultReaderOptions().WithSnapshot(true))
		if err != nil {
			_ = reader.Close()
			return nil, nil, err
		}
		reader.files = append(reader.files, file)
	}
	s.readers.Add(1)
	return reader, func() { _ = reader.Close(); s.readers.Done() }, nil
}

// Glob suppresses directory I/O errors. Explicit enumeration keeps an
// unavailable history directory from masquerading as an empty exact result.
func (s *Store) journalPaths(ctx context.Context) ([]string, error) {
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return nil, fmt.Errorf("read history root: %w", err)
	}
	currentMachine := s.host.MachineID().String()
	machines := []string{currentMachine}
	for _, entry := range entries {
		if entry.Name() == currentMachine {
			continue
		}
		if _, err := journal.ParseUUID(entry.Name()); err == nil {
			machines = append(machines, entry.Name())
		}
	}
	var paths []string
	for _, machine := range machines {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		dir := filepath.Join(s.root, machine)
		files, err := os.ReadDir(dir)
		if err != nil {
			return nil, fmt.Errorf("read history machine directory: %w", err)
		}
		for _, file := range files {
			if !strings.HasPrefix(file.Name(), "dem@") {
				continue
			}
			path := filepath.Join(dir, file.Name())
			// SDK quarantines damaged active journals as .journal~ and leaves their
			// recovery/removal to the operator. Never silently omit owned history.
			if strings.HasSuffix(file.Name(), ".journal~") {
				return nil, fmt.Errorf("history contains damaged journal %q; operator recovery is required", path)
			}
			if strings.HasSuffix(file.Name(), ".journal") {
				paths = append(paths, path)
			}
		}
	}
	return paths, nil
}

// Reducers order their final output themselves; no merged row copy is needed.
// Keeping one reader per file also permits cancellation between snapshot opens.
type journalSnapshot struct {
	ctx   context.Context
	files []*journal.DirectoryReader
	index int
}

func (r *journalSnapshot) Close() error {
	var err error
	for _, file := range r.files {
		err = errors.Join(err, file.Close())
	}
	return err
}
func (r *journalSnapshot) Step() (bool, error) {
	for r.index < len(r.files) {
		if err := r.ctx.Err(); err != nil {
			return false, err
		}
		more, err := r.files[r.index].Step()
		if err != nil || more {
			return more, err
		}
		r.index++
	}
	return false, nil
}
func (r *journalSnapshot) SeekRealtimeUsec(usec uint64) error {
	for _, file := range r.files {
		if err := r.ctx.Err(); err != nil {
			return err
		}
		if err := file.SeekRealtimeUsec(usec); err != nil {
			return err
		}
	}
	return nil
}
func (r *journalSnapshot) GetRealtimeUsec() (uint64, error) {
	return r.files[r.index].GetRealtimeUsec()
}
func (r *journalSnapshot) VisitEntryPayloads(visit func([]byte) error) error {
	return r.files[r.index].VisitEntryPayloads(visit)
}
