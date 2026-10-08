// SPDX-License-Identifier: GPL-3.0-or-later

package journal

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/netdata/systemd-journal-sdk/go/journal"
)

// OpenReader captures the retained file set and native index bounds while
// appends, rotation and retention are excluded. Traversal releases admission and
// keeps only bounded SDK buffers, not an offset for every retained entry.
func (s *Store) OpenReader(ctx context.Context) (*Snapshot, func(), error) {
	if err := s.acquire(ctx); err != nil {
		return nil, nil, err
	}
	defer s.release()
	if s.closed {
		return nil, nil, os.ErrClosed
	}
	if s.failure != nil {
		return nil, nil, s.failure
	}
	paths, err := s.journalPaths(ctx, false)
	if err != nil {
		return nil, nil, err
	}
	reader := &Snapshot{ctx: ctx}
	for _, path := range paths {
		file, err := openSnapshot(ctx, path)
		if err != nil {
			_ = reader.close()
			return nil, nil, err
		}
		reader.files = append(reader.files, file)
	}
	s.readers.Add(1)
	var once sync.Once
	return reader, func() { once.Do(func() { _ = reader.close(); s.readers.Done() }) }, nil
}

func openSnapshot(ctx context.Context, path string) (*journal.IndexedSnapshot, error) {
	file, err := journal.OpenIndexedSnapshot(ctx, path, journal.IndexedSnapshotOptions{
		CaptureFields: [][]byte{[]byte(rumBucket), []byte(syntheticBucket)},
		CaptureValues: []journal.Field{journal.StringField(schemaField, schemaVersion)},
	})
	if err != nil {
		return nil, fmt.Errorf("open history snapshot %q: %w", path, err)
	}
	coverage, err := file.CapturedValue([]byte(schemaField), []byte(schemaVersion))
	if err == nil && coverage.EntryCount != file.EntryCount() {
		err = fmt.Errorf("incompatible DEM history schema: current schema covers %d of %d records", coverage.EntryCount, file.EntryCount())
	}
	if err == nil && filepath.Base(path) != "dem.journal" && !file.IsArchived() {
		err = fmt.Errorf("history archive is not finalized")
	}
	if err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("history %q: %w", path, err)
	}
	return file, nil
}

// Explicit enumeration preserves directory I/O failures. Glob and SDK directory
// readers can silently skip unavailable files and produce incomplete aggregates.
func (s *Store) journalPaths(ctx context.Context, allowMissingCurrent bool) ([]string, error) {
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return nil, fmt.Errorf("read history root: %w", err)
	}
	current := s.host.MachineID().String()
	machines := []string{current}
	for _, entry := range entries {
		if entry.Name() == current {
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
		if allowMissingCurrent && machine == current && errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("read history machine directory: %w", err)
		}
		for _, file := range files {
			name := file.Name()
			if name != "dem.journal" && name != "dem.journal~" && !strings.HasPrefix(name, "dem@") {
				continue
			}
			path := filepath.Join(dir, name)
			if strings.HasSuffix(name, ".journal~") {
				return nil, fmt.Errorf("history contains damaged journal %q; operator recovery is required", path)
			}
			if strings.HasSuffix(name, ".journal") {
				paths = append(paths, path)
			}
		}
	}
	return paths, nil
}

// Snapshot owns independent single-consumer SDK readers until release. Callers
// copy domain values in callbacks and discard partial results on any error.
type Snapshot struct {
	ctx   context.Context
	files []*journal.IndexedSnapshot
}

func (r *Snapshot) close() error {
	var err error
	for _, file := range r.files {
		err = errors.Join(err, file.Close())
	}
	return err
}
func (r *Snapshot) VisitEntries(visit func(*journal.SnapshotEntry) error) error {
	for _, file := range r.files {
		if err := file.VisitEntries(r.ctx, visit); err != nil {
			return err
		}
	}
	return r.ctx.Err()
}
func (r *Snapshot) VisitMatch(name, value string, visit func(*journal.SnapshotEntry) error) error {
	for _, file := range r.files {
		if err := file.VisitMatch(r.ctx, []byte(name), []byte(value), visit); err != nil {
			return err
		}
	}
	return r.ctx.Err()
}

// Narrow ranges probe occupied minute candidates cheaply; broad/sparse ranges
// walk existing bucket values. This crossover affects cost only, never coverage.
// Mixed retained-history measurements keep exact probes ahead through 256
// minutes; sparse multi-day ranges favor FIELD enumeration.
const exactBucketLimit = int64(256)

// VisitRange selects candidates by the domain clock's minute. Domain decoders
// validate exact timestamps against the inclusive whole-second bounds afterward.
func (r *Snapshot) VisitRange(kind string, after, before int64, visit func(*journal.SnapshotEntry) error) error {
	if after < 0 || before < after {
		return fmt.Errorf("invalid history observation/start range")
	}
	field := ""
	switch kind {
	case "rum":
		field = rumBucket
	case "synthetic":
		field = syntheticBucket
	default:
		return fmt.Errorf("invalid history kind %q", kind)
	}
	first, last := after/60, before/60
	for _, file := range r.files {
		if last-first < exactBucketLimit {
			for minute := first; minute <= last; minute++ {
				if err := file.VisitMatch(r.ctx, []byte(field), []byte(strconv.FormatInt(minute, 10)), visit); err != nil {
					return err
				}
			}
		} else {
			err := file.VisitField(r.ctx, []byte(field), func(value []byte) (bool, error) {
				minute, err := decimal(value)
				if err != nil {
					return false, fmt.Errorf("invalid history bucket: %w", err)
				}
				return minute >= first && minute <= last, nil
			}, visit)
			if err != nil {
				return err
			}
		}
	}
	return r.ctx.Err()
}
