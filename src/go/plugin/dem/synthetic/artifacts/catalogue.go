// SPDX-License-Identifier: GPL-3.0-or-later

package artifacts

import (
	"cmp"
	"context"
	"fmt"
	"slices"

	"github.com/netdata/netdata/go/plugins/plugin/dem/internal/vnodeguid"

	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
)

type Set struct {
	VnodeGUID   string
	JobID       string
	RunID       string
	PublishedUS int64
	Bytes       int64
	Artifacts   int
}

type Cursor struct {
	PublishedUS int64
	RunID       string
}

type Filter struct {
	JobID     string
	VnodeGUID string
	Before    *Cursor
	Limit     int
}

type Page struct {
	Sets   []Set
	Next   *Cursor
	Status Status
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
	if vnodeguid.Validate(filter.VnodeGUID) != nil || filter.Limit < 1 ||
		(filter.JobID != "" && !synthetic.ValidJobID(filter.JobID)) ||
		(filter.Before != nil && (filter.Before.PublishedUS <= 0 || !runIDPattern.MatchString(filter.Before.RunID))) {
		return Page{}, fmt.Errorf("%w: invalid artifact catalogue filter", ErrInvalidID)
	}
	entries, inv := s.inventory(ctx)
	if err := ctx.Err(); err != nil {
		return Page{}, err
	}
	slices.SortFunc(entries, newestFirst)
	page := Page{
		Sets:   []Set{},
		Status: s.status(inv),
	}
	for _, entry := range entries {
		if filter.VnodeGUID != "" && filter.VnodeGUID != entry.VnodeGUID {
			continue
		}
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

// newestFirst orders sets by publication time, then run ID, newest first.
func newestFirst(a, b Set) int {
	return cmp.Or(cmp.Compare(b.PublishedUS, a.PublishedUS), cmp.Compare(b.RunID, a.RunID))
}
