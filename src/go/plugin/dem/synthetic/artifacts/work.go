// SPDX-License-Identifier: GPL-3.0-or-later

package artifacts

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

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

func workAlias(id string) string { return filepath.Join("/tmp", "nd-dem-"+id) }
