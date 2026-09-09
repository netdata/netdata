// SPDX-License-Identifier: GPL-3.0-or-later

package topologymodel

import (
	"sort"
	"strings"
	"unicode"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyutil"
)

type localActorMatchPositions struct {
	first int
	rest  []int
}

// LocalActorMatchIndex resolves cache-local device identity without repeatedly scanning actor aliases.
type LocalActorMatchIndex struct {
	byChassisID map[string]localActorMatchPositions
	bySysName   map[string]localActorMatchPositions
	byIP        map[string]localActorMatchPositions
	actorIDs    map[string]struct{}
}

// NewLocalActorMatchIndex returns an empty local-identity index.
func NewLocalActorMatchIndex() *LocalActorMatchIndex {
	return &LocalActorMatchIndex{
		byChassisID: make(map[string]localActorMatchPositions),
		bySysName:   make(map[string]localActorMatchPositions),
		byIP:        make(map[string]localActorMatchPositions),
		actorIDs:    make(map[string]struct{}),
	}
}

// AddActorID records an exact, trimmed actor ID for fallback existence checks.
func (idx *LocalActorMatchIndex) AddActorID(actorID string) {
	if idx == nil {
		return
	}
	if actorID = strings.TrimSpace(actorID); actorID != "" {
		idx.actorIDs[actorID] = struct{}{}
	}
}

// ContainsActorID reports whether an exact, trimmed actor ID was recorded.
func (idx *LocalActorMatchIndex) ContainsActorID(actorID string) bool {
	if idx == nil {
		return false
	}
	_, ok := idx.actorIDs[strings.TrimSpace(actorID)]
	return ok
}

// AddMatch records the local-identity subset of one actor match at its stable generation-local position.
func (idx *LocalActorMatchIndex) AddMatch(position int, match Match) {
	if idx == nil || position < 0 {
		return
	}
	for _, chassisID := range match.ChassisIDs {
		addLocalActorMatchPosition(idx.byChassisID, normalizeFoldedLocalActorKey(chassisID), position)
	}
	addLocalActorMatchPosition(idx.bySysName, normalizeFoldedLocalActorKey(match.SysName), position)
	for _, value := range match.IPAddresses {
		addLocalActorMatchPosition(idx.byIP, topologyutil.NormalizeIPAddress(value), position)
	}
}

// FirstMatch returns the earliest recorded actor matching chassis ID, system name, or selected management IP.
func (idx *LocalActorMatchIndex) FirstMatch(local Device) (int, bool) {
	if idx == nil {
		return 0, false
	}
	keys := localActorMatchKeys(local)
	best := 0
	found := false
	for i, values := range [...]map[string]localActorMatchPositions{idx.byChassisID, idx.bySysName, idx.byIP} {
		if key := keys[i]; key != "" {
			if positions, ok := values[key]; ok && (!found || positions.first < best) {
				best = positions.first
				found = true
			}
		}
	}
	return best, found
}

// MatchIndexes appends every unique matching actor position in actor order.
func (idx *LocalActorMatchIndex) MatchIndexes(dst []int, local Device) []int {
	if idx == nil {
		return dst
	}
	keys := localActorMatchKeys(local)
	values := [...]map[string]localActorMatchPositions{idx.byChassisID, idx.bySysName, idx.byIP}
	var positions [3]localActorMatchPositions
	var present [3]bool
	for i, key := range keys {
		if key != "" {
			positions[i], present[i] = values[i][key]
		}
	}

	var offsets [3]int
	last := -1
	for {
		best := -1
		for i := range positions {
			if !present[i] || offsets[i] >= localActorMatchPositionCount(positions[i]) {
				continue
			}
			value := localActorMatchPositionAt(positions[i], offsets[i])
			if best == -1 || value < best {
				best = value
			}
		}
		if best == -1 {
			return dst
		}
		if best != last {
			dst = append(dst, best)
			last = best
		}
		for i := range positions {
			for present[i] && offsets[i] < localActorMatchPositionCount(positions[i]) &&
				localActorMatchPositionAt(positions[i], offsets[i]) == best {
				offsets[i]++
			}
		}
	}
}

func localActorMatchKeys(local Device) [3]string {
	return [3]string{
		normalizeFoldedLocalActorKey(local.ChassisID),
		normalizeFoldedLocalActorKey(local.SysName),
		topologyutil.NormalizeIPAddress(local.ManagementIP),
	}
}

func normalizeFoldedLocalActorKey(value string) string {
	return strings.Map(canonicalSimpleFoldRune, strings.TrimSpace(value))
}

func canonicalSimpleFoldRune(value rune) rune {
	canonical := value
	for folded := unicode.SimpleFold(value); folded != value; folded = unicode.SimpleFold(folded) {
		if folded < canonical {
			canonical = folded
		}
	}
	return canonical
}

func addLocalActorMatchPosition(index map[string]localActorMatchPositions, key string, position int) {
	if key == "" {
		return
	}
	positions, ok := index[key]
	if !ok {
		index[key] = localActorMatchPositions{first: position}
		return
	}
	if position == positions.first {
		return
	}
	if position < positions.first {
		positions.rest = insertLocalActorMatchPosition(positions.rest, positions.first)
		positions.first = position
	} else {
		positions.rest = insertLocalActorMatchPosition(positions.rest, position)
	}
	index[key] = positions
}

func insertLocalActorMatchPosition(values []int, position int) []int {
	i := sort.SearchInts(values, position)
	if i < len(values) && values[i] == position {
		return values
	}
	values = append(values, 0)
	copy(values[i+1:], values[i:])
	values[i] = position
	return values
}

func localActorMatchPositionCount(positions localActorMatchPositions) int {
	return 1 + len(positions.rest)
}

func localActorMatchPositionAt(positions localActorMatchPositions, index int) int {
	if index == 0 {
		return positions.first
	}
	return positions.rest[index-1]
}
