// SPDX-License-Identifier: GPL-3.0-or-later
package artifacts

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
)

// Manifest is Go-authored capture membership, independent of run history.
type Manifest struct {
	JobID       string               `json:"job_id"`
	RunID       string               `json:"run_id"`
	PublishedUS int64                `json:"published_us"`
	Artifacts   []synthetic.Artifact `json:"artifacts"`
}

type Set struct {
	JobID       string `json:"job_id"`
	RunID       string `json:"run_id"`
	PublishedUS int64  `json:"published_us"`
	Bytes       int64  `json:"bytes"`
	Artifacts   int    `json:"artifact_count"`
}

type Cursor struct {
	PublishedUS int64  `json:"published_us"`
	RunID       string `json:"run_id"`
}

type Filter struct {
	JobID  string
	Before *Cursor
	Limit  int
}

type ByteObservation struct {
	Bytes *int64 `json:"bytes"`
	Error string `json:"error,omitempty"`
}

type Inventory struct {
	ObservedUS   int64           `json:"observed_us"`
	Retained     ByteObservation `json:"retained"`
	Workspace    ByteObservation `json:"workspace"`
	Unclassified ByteObservation `json:"unclassified"`
	Deleting     ByteObservation `json:"deleting"`
	KnownSets    int             `json:"known_sets"`
	KnownJobs    int             `json:"known_jobs"`
	Error        string          `json:"error,omitempty"`
}

type Policy struct {
	Days     int   `json:"days"`
	MaxBytes int64 `json:"max_bytes"`
}
type Cleanup struct {
	AttemptedUS  int64  `json:"attempted_us"`
	SuccessfulUS int64  `json:"successful_us"`
	Error        string `json:"error,omitempty"`
}

type Status struct {
	Policy    *Policy   `json:"policy"`
	Inventory Inventory `json:"inventory"`
	Cleanup   Cleanup   `json:"cleanup"`
}

type Page struct {
	Sets   []Set   `json:"sets"`
	Next   *Cursor `json:"next,omitempty"`
	Status Status  `json:"storage"`
}

// Catalogue reads current metadata/file sizes, never payload contents. Errors
// from individual inventory categories are explicit in Status; closed/cancelled
// requests fail as a whole. Filters never change the global inventory facts.
func (s *Store) Catalogue(ctx context.Context, filter Filter) (Page, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return Page{}, ErrClosed
	}
	if err := ctx.Err(); err != nil {
		return Page{}, err
	}
	if filter.Limit < 1 || (filter.JobID != "" && !validJobID(filter.JobID)) ||
		(filter.Before != nil && (filter.Before.PublishedUS <= 0 || !runIDPattern.MatchString(filter.Before.RunID))) {
		return Page{}, fmt.Errorf("%w: invalid artifact catalogue filter", ErrInvalidID)
	}
	entries, inv := s.inventory(ctx)
	if err := ctx.Err(); err != nil {
		return Page{}, err
	}
	sort.Slice(entries, func(i, j int) bool { return newer(entries[i], entries[j]) })
	page := Page{
		Sets:   []Set{},
		Status: s.status(inv),
	}
	for _, entry := range entries {
		if filter.JobID != "" && filter.JobID != entry.JobID {
			continue
		}
		if c := filter.Before; c != nil &&
			(entry.PublishedUS > c.PublishedUS || (entry.PublishedUS == c.PublishedUS && entry.RunID >= c.RunID)) {
			continue
		}
		if len(page.Sets) == filter.Limit {
			last := page.Sets[len(page.Sets)-1]
			page.Next = &Cursor{
				PublishedUS: last.PublishedUS,
				RunID:       last.RunID,
			}
			break
		}
		page.Sets = append(page.Sets, entry)
	}
	return page, nil
}

func newer(a, b Set) bool {
	if a.PublishedUS != b.PublishedUS {
		return a.PublishedUS > b.PublishedUS
	}
	return a.RunID > b.RunID
}
func (s *Store) status(inv Inventory) Status {
	var policy *Policy
	if s.policy != nil {
		copy := *s.policy
		policy = &copy
	}
	return Status{
		Policy:    policy,
		Inventory: inv,
		Cleanup:   s.cleanup,
	}
}
func observation(bytes int64, err error) ByteObservation {
	if err != nil {
		return ByteObservation{
			Error: err.Error(),
		}
	}
	return ByteObservation{
		Bytes: &bytes,
	}
}
func inventoryError(inv Inventory) error {
	var result error
	for _, e := range []string{inv.Error, inv.Retained.Error, inv.Workspace.Error, inv.Unclassified.Error, inv.Deleting.Error} {
		if e != "" {
			result = errors.Join(result, errors.New(e))
		}
	}
	return result
}
