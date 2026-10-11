// SPDX-License-Identifier: GPL-3.0-or-later

package journal

import (
	"context"
	"os"
	"time"
)

// RetentionPolicy describes the effective shared history allowance.
type RetentionPolicy struct {
	Days     int
	MaxBytes int64
}

// Inventory counts directory-visible journal files, including preallocation.
// Query-pinned files already unlinked from the directory are not included.
type Inventory struct {
	Bytes uint64
	Files int
}

type CleanupStatus struct {
	AttemptedAt      time.Time
	LastSuccessfulAt time.Time
	Error            string
}

// Status distinguishes an unknown inventory from an empty one and keeps cleanup
// failures separate from a failed writer. It makes no retained-coverage claim.
type Status struct {
	Policy         *RetentionPolicy
	Inventory      *Inventory
	ObservedAt     time.Time
	InventoryError string
	Cleanup        CleanupStatus
	WriterError    string
}

// Status samples metadata under writer exclusion, including after writer failure.
// It returns cancellation/closed errors; storage errors are facts in the result.
func (s *Store) Status(ctx context.Context) (Status, error) {
	if err := s.acquire(ctx); err != nil {
		return Status{}, err
	}
	defer s.release()
	if s.closed {
		return Status{}, os.ErrClosed
	}
	var status Status
	policy := s.log.RootRetentionPolicy()
	if policy.MaxAge != nil && policy.MaxBytes != nil {
		status.Policy = &RetentionPolicy{
			Days:     int(*policy.MaxAge / (24 * time.Hour)),
			MaxBytes: int64(*policy.MaxBytes),
		}
	}
	if result, attempted := s.log.LastRootRetentionResult(); attempted {
		status.Cleanup.AttemptedAt = result.AttemptedAt
		status.Cleanup.LastSuccessfulAt = result.LastSuccessfulAt
		if result.Err != nil {
			status.Cleanup.Error = result.Err.Error()
		}
	}
	if s.failure != nil {
		status.WriterError = s.failure.Error()
	}
	status.ObservedAt = time.Now()
	inventory, err := s.inventory(ctx)
	if ctx.Err() != nil {
		return Status{}, ctx.Err()
	}
	if err != nil {
		status.InventoryError = err.Error()
	} else {
		status.Inventory = &Inventory{Bytes: inventory.Bytes, Files: len(inventory.Files)}
	}
	return status, nil
}
