// SPDX-License-Identifier: GPL-3.0-or-later
package artifacts

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"time"
)

// The narrow syscall boundary permits real transition/failure tests without
// substituting the store's ownership, selection or recovery logic.
type evictionIO struct {
	rename    func(string, string) error
	removeAll func(string) error
	sync      func(string) error
}

// Enforce applies age first, then prefers each job's newest eligible set.
// Workspace and unclassified/pending-deletion files are reported separately;
// they cannot be reclaimed to satisfy this retained-capture allowance.
func (s *Store) Enforce(ctx context.Context, days int, maxBytes int64) (Status, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return s.status(Inventory{}), ErrClosed
	}
	if err := ctx.Err(); err != nil {
		return s.status(Inventory{}), err
	}
	const maxDays = int64(^uint64(0)>>1) / int64(24*time.Hour)
	if days < 0 || int64(days) > maxDays || maxBytes < 0 {
		return s.status(Inventory{}), fmt.Errorf("invalid artifact retention policy")
	}
	s.policy = &Policy{
		Days:     days,
		MaxBytes: maxBytes,
	}
	s.cleanup.AttemptedUS = time.Now().UnixMicro()
	result := s.retryCleanup(ctx)
	entries, inv := s.inventory(ctx)
	result = errors.Join(result, inventoryError(inv))
	// Only a complete enumeration establishes which known sets are newest.
	if inv.Retained.Error == "" {
		for _, entry := range expiredSets(entries, time.Now(), days, maxBytes) {
			if err := ctx.Err(); err != nil {
				result = errors.Join(result, err)
				break
			}
			if err := s.expire(entry.RunID); err != nil {
				result = errors.Join(result, err)
			}
		}
	}
	// One final inventory reports partial failures accurately; never scan per victim.
	_, inv = s.inventory(ctx)
	result = errors.Join(result, inventoryError(inv), ctx.Err())
	if result != nil {
		s.cleanup.Error = result.Error()
	} else {
		s.cleanup.Error = ""
		s.cleanup.SuccessfulUS = time.Now().UnixMicro()
	}
	return s.status(inv), result
}

func expiredSets(entries []Set, now time.Time, days int, maxBytes int64) []Set {
	sort.Slice(entries, func(i, j int) bool { return newer(entries[j], entries[i]) })
	var victims, eligible []Set
	newest := map[string]string{}
	var total int64
	cutoff := now.Add(-time.Duration(days) * 24 * time.Hour).UnixMicro()
	for _, entry := range entries {
		if (days > 0 && entry.PublishedUS < cutoff) || (maxBytes > 0 && entry.Bytes > maxBytes) {
			victims = append(victims, entry)
			continue
		}
		eligible = append(eligible, entry)
		newest[entry.JobID] = entry.RunID
		total += entry.Bytes
	}
	if maxBytes > 0 {
		for _, preferred := range []bool{false, true} {
			for _, entry := range eligible {
				if total <= maxBytes {
					break
				}
				if (newest[entry.JobID] == entry.RunID) == preferred {
					victims = append(victims, entry)
					total -= entry.Bytes
				}
			}
		}
	}
	return victims
}

func (s *Store) retryCleanup(ctx context.Context) error {
	var result error
	markers, err := fs.ReadDir(s.root.FS(), "drained")
	result = errors.Join(result, err)
	for _, entry := range markers {
		if err := ctx.Err(); err != nil {
			return errors.Join(result, err)
		}
		id := entry.Name()
		if !runIDPattern.MatchString(id) {
			result = errors.Join(result, fmt.Errorf("unrecognized drainage entry %q", id))
			continue
		}
		if s.active[id] != "" || s.uncertain[id] {
			continue
		}
		result = errors.Join(result, s.cleanupDrained(id))
	}
	entries, err := fs.ReadDir(s.root.FS(), "deleting")
	result = errors.Join(result, err)
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return errors.Join(result, err)
		}
		if !runIDPattern.MatchString(entry.Name()) || !entry.IsDir() {
			result = errors.Join(result, fmt.Errorf("unrecognized deletion entry %q", entry.Name()))
			continue
		}
		result = errors.Join(result, s.finishDeletion(entry.Name()))
	}
	// Retry a failed final sync even when the preceding removal left no entries.
	return errors.Join(result, s.eviction.sync("deleting"))
}

func (s *Store) expire(id string) error {
	if _, err := s.root.Lstat("deleting/" + id); err == nil {
		return fmt.Errorf("deletion destination exists: %w", ErrExists)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if err := s.eviction.rename("runs/"+id, "deleting/"+id); err != nil {
		return err
	}
	return s.finishDeletion(id)
}

func (s *Store) finishDeletion(id string) error {
	// A conflicting source is not permission to destroy either directory.
	if _, err := s.root.Lstat("runs/" + id); err == nil {
		return fmt.Errorf("conflicting published and deleting run %s", id)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if s.active[id] != "" || s.uncertain[id] {
		return fmt.Errorf("deleting run has uncertain ownership")
	}
	info, err := s.root.Lstat("deleting/" + id)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("deletion entry is not a directory")
	}
	// A successor can observe an unsynced rename. Visibility is not durability.
	if err := errors.Join(s.eviction.sync("runs"), s.eviction.sync("deleting")); err != nil {
		return err
	}
	if err := s.eviction.removeAll("deleting/" + id); err != nil {
		return err
	}
	return s.eviction.sync("deleting")
}

func (s *Store) recordCleanupError(err error) {
	// A failed run-local cleanup cannot erase an earlier maintenance failure.
	if s.cleanup.Error == "" {
		s.cleanup.Error = err.Error()
	}
}
