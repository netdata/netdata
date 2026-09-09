// SPDX-License-Identifier: GPL-3.0-or-later

package promvalidation

import (
	"maps"
	"slices"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
)

// chartOwnershipIndex joins compiler and rendered chart identities back to the
// authored profile-local path. It is intentionally report-only: production
// planning continues to use its compact internal IDs.
type chartOwnershipIndex struct {
	composed           bool
	pathsByTemplate    map[string]string
	profilesByTemplate map[string]string
	pathsByChart       map[string]map[string]struct{}
}

func newChartOwnershipIndex(
	refs []chartRef,
	plan chartengine.Plan,
	routes *planRouteSummary,
	composed bool,
) *chartOwnershipIndex {
	index := &chartOwnershipIndex{
		composed:           composed,
		pathsByTemplate:    make(map[string]string),
		profilesByTemplate: make(map[string]string),
		pathsByChart:       make(map[string]map[string]struct{}),
	}
	for _, ref := range refs {
		templateID, ok := chartengine.ChartTemplateIDAt(ref.groupPath, ref.chartIndex)
		if !ok || ref.profile == "" {
			continue
		}
		index.pathsByTemplate[templateID] = ref.path
		index.profilesByTemplate[templateID] = ref.profile
	}
	if routes != nil {
		for chartID, templateIDs := range routes.ownersByChart {
			for templateID := range templateIDs {
				index.addChartPath(chartID, index.pathsByTemplate[templateID])
			}
		}
	}
	for _, action := range plan.Actions {
		created, ok := action.(chartengine.CreateChartAction)
		if !ok {
			continue
		}
		path := index.pathsByTemplate[created.ChartTemplateID]
		if path == "" {
			continue
		}
		index.addChartPath(created.ChartID, path)
	}
	return index
}

func (i *chartOwnershipIndex) addChartPath(chartID, path string) {
	if i == nil || chartID == "" || path == "" {
		return
	}
	paths := i.pathsByChart[chartID]
	if paths == nil {
		paths = make(map[string]struct{})
		i.pathsByChart[chartID] = paths
	}
	paths[path] = struct{}{}
}

func (i *chartOwnershipIndex) templatePath(templateID, fallback string) string {
	if i != nil && i.composed {
		if path := i.pathsByTemplate[templateID]; path != "" {
			return path
		}
	}
	return fallback
}

func (i *chartOwnershipIndex) templateProfile(templateID string) string {
	if i == nil || !i.composed {
		return ""
	}
	return i.profilesByTemplate[templateID]
}

func (i *chartOwnershipIndex) chartPaths(chartID string) []string {
	if i == nil {
		return nil
	}
	return slices.Sorted(maps.Keys(i.pathsByChart[chartID]))
}

func (i *chartOwnershipIndex) chartPath(chartID, fallback string) string {
	if i != nil && i.composed {
		if paths := i.chartPaths(chartID); len(paths) > 0 {
			return strings.Join(paths, ", ")
		}
	}
	return fallback
}

func joinedOwnerPaths(paths ...[]string) []string {
	set := make(map[string]struct{})
	for _, items := range paths {
		for _, path := range items {
			if path != "" {
				set[path] = struct{}{}
			}
		}
	}
	return slices.Sorted(maps.Keys(set))
}

func collisionFindingPath(paths []string, fallback string) string {
	if len(paths) == 0 {
		return fallback
	}
	return strings.Join(paths, ", ")
}

func joinedCollisionPaths(collisions []wireChartCollisionReport) string {
	var paths [][]string
	for _, collision := range collisions {
		paths = append(paths, collision.Paths)
	}
	return collisionFindingPath(joinedOwnerPaths(paths...), "")
}

func joinedContextCollisionPaths(collisions []wireContextCollisionReport) string {
	var paths [][]string
	for _, collision := range collisions {
		paths = append(paths, collision.Paths)
	}
	return collisionFindingPath(joinedOwnerPaths(paths...), "")
}
