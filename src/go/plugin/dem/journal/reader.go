// SPDX-License-Identifier: GPL-3.0-or-later

package journal

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/netdata/systemd-journal-sdk/go/journal"
)

// OpenReader takes a stable snapshot before an append/rotation/sweep can change
// the file set. SDK snapshot construction stores O(retained entries) offsets;
// reducers stream rows and do not retain another complete event copy.
func (s *Store) OpenReader(ctx context.Context) (*Snapshot, func(), error) {
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
	reader := &Snapshot{
		ctx: ctx,
	}
	for _, path := range paths {
		if err := ctx.Err(); err != nil {
			_ = reader.close()
			return nil, nil, err
		}
		// SDK OpenFiles leaks earlier readers when opening a later file fails.
		// Own each single-file reader so all successful opens can be closed.
		file, err := journal.OpenFilesWithOptions([]string{path}, journal.DefaultReaderOptions().WithSnapshot(true))
		if err != nil {
			_ = reader.close()
			return nil, nil, err
		}
		reader.files = append(reader.files, file)
	}
	s.readers.Add(1)
	return reader, func() { _ = reader.close(); s.readers.Done() }, nil
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

// Snapshot owns stable readers until the release function returned by OpenReader runs.
// Reducers order their final output themselves; no merged row copy is needed.
// Keeping one reader per file also permits cancellation between snapshot opens.
type Snapshot struct {
	ctx   context.Context
	files []*journal.DirectoryReader
	index int
}

func (r *Snapshot) close() error {
	var err error
	for _, file := range r.files {
		err = errors.Join(err, file.Close())
	}
	return err
}
func (r *Snapshot) Step() (bool, error) {
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
func (r *Snapshot) SeekRealtimeUsec(usec uint64) error {
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
func (r *Snapshot) GetRealtimeUsec() (uint64, error) {
	return r.files[r.index].GetRealtimeUsec()
}
func (r *Snapshot) VisitEntryPayloads(visit func([]byte) error) error {
	return r.files[r.index].VisitEntryPayloads(visit)
}
