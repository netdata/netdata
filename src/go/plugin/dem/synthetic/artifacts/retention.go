// SPDX-License-Identifier: GPL-3.0-or-later

package artifacts

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"time"
)

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
		if !(days > 0 && m.CreatedUS < cutoff) &&
			!(maxBytes > 0 && s.stats.RetainedBytes+s.stats.ProtectedBytes > maxBytes) {
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

func (s *Store) expire(m manifest) error {
	if err := s.root.RemoveAll("runs/" + m.RunID); err != nil {
		return err
	}
	return syncDir(s.root, "runs")
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
