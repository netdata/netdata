// SPDX-License-Identifier: GPL-3.0-or-later

package processes

import (
	"context"
	"errors"
	"fmt"

	"github.com/netdata/netdata/go/plugins/plugin/apps/internal/model"
)

func (c *Collector) Collect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.scanner == nil {
		return errors.New("collector not initialized")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	snapshot, err := c.scanner.Scan(ctx)
	if err != nil {
		return fmt.Errorf("scan procfs: %w", err)
	}
	groups, assignments, err := c.grouping.Aggregate(&snapshot)
	if err != nil {
		return fmt.Errorf("group processes: %w", err)
	}
	files, err := c.scanner.Finalize(snapshot.Generation, assignments)
	if err != nil {
		return fmt.Errorf("finalize process scan: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	byID := make(map[uint32]model.GroupFD, len(files))
	for _, f := range files {
		byID[f.ID] = f
	}
	for i := range groups {
		if f, ok := byID[groups[i].ID]; ok {
			groups[i].FDCounts, groups[i].FDValid = f.Counts, f.Valid
		}
	}
	c.writeMetrics(groups, snapshot)
	// Readers retain a completed, owned snapshot; the next native scan cannot
	// mutate it. Publication and Function serialization share no C pointers.
	c.snapshot.Store(&snapshot)
	return nil
}
