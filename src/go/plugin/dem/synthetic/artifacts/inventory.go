// SPDX-License-Identifier: GPL-3.0-or-later
package artifacts

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"time"
)

// Inventory categories are independent: a live browser changing its workspace
// must not prevent retention of already published immutable capture sets.
func (s *Store) inventory(ctx context.Context) ([]Set, Inventory) {
	var sets []Set
	var retained, workspace, unclassified int64
	var retainedErr, workspaceErr, unclassifiedErr, classificationErr error
	entries, err := fs.ReadDir(s.inventoryFS, "runs")
	if err != nil {
		retainedErr = err
		unclassifiedErr = err
	}
	jobs := map[string]bool{}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			retainedErr = errors.Join(retainedErr, err)
			unclassifiedErr = errors.Join(unclassifiedErr, err)
			break
		}
		id := entry.Name()
		size, sizeErr := treeBytesContext(ctx, s.inventoryFS, "runs/"+id)
		if s.uncertain[id] {
			workspace += size
			workspaceErr = errors.Join(workspaceErr, sizeErr)
			continue
		}
		var m manifest
		var readErr error
		if !runIDPattern.MatchString(id) || !entry.IsDir() {
			readErr = fmt.Errorf("unrecognized published entry %q", id)
		} else {
			m, readErr = s.readManifest(id)
		}
		if readErr != nil {
			unclassified += size
			unclassifiedErr = errors.Join(unclassifiedErr, sizeErr)
			classificationErr = errors.Join(classificationErr, readErr, sizeErr)
			continue
		}
		if sizeErr != nil {
			// Failed measurement does not erase ownership. Selection needs the
			// complete retained population to preserve each job's newest set.
			retainedErr = errors.Join(retainedErr, sizeErr)
			continue
		}
		sets = append(
			sets,
			Set{
				JobID:       m.JobID,
				RunID:       id,
				PublishedUS: m.PublishedUS,
				Bytes:       size,
				Artifacts:   len(m.Artifacts),
			},
		)
		retained += size
		jobs[m.JobID] = true
	}
	for _, dir := range []string{"work", "staging", "drained"} {
		size, err := treeBytesContext(ctx, s.inventoryFS, dir)
		workspace += size
		workspaceErr = errors.Join(workspaceErr, err)
	}
	pending, pendingErr := treeBytesContext(ctx, s.inventoryFS, "deleting")
	inv := Inventory{
		ObservedUS:   time.Now().UnixMicro(),
		Retained:     observation(retained, retainedErr),
		Workspace:    observation(workspace, workspaceErr),
		Unclassified: observation(unclassified, unclassifiedErr),
		Deleting:     observation(pending, pendingErr),
		KnownSets:    len(sets),
		KnownJobs:    len(jobs),
	}
	if classificationErr != nil {
		inv.Error = classificationErr.Error()
	}
	return sets, inv
}
