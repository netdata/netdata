// SPDX-License-Identifier: GPL-3.0-or-later

// Package journal owns the command-wide DEM journal, snapshots and retention.
package journal

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

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
