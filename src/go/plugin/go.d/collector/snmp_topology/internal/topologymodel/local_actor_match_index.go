// SPDX-License-Identifier: GPL-3.0-or-later

package topologymodel

import (
	"sort"
	"strings"
	"unicode"

	"github.com/netdata/netdata/go/plugins/pkg/topology/worklimit"
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
	_ = idx.AddActorIDWithLimiter(actorID, nil)
}

func (idx *LocalActorMatchIndex) AddActorIDWithLimiter(actorID string, limiter worklimit.Limiter) error {
	if idx == nil {
		return nil
	}
	if err := worklimit.ChargeStrings(limiter, []string{actorID}); err != nil {
		return err
	}
	if actorID = strings.TrimSpace(actorID); actorID != "" {
		idx.actorIDs[actorID] = struct{}{}
	}
	return nil
}

// ContainsActorID reports whether an exact, trimmed actor ID was recorded.
func (idx *LocalActorMatchIndex) ContainsActorID(actorID string) bool {
	ok, _ := idx.ContainsActorIDWithLimiter(actorID, nil)
	return ok
}

func (idx *LocalActorMatchIndex) ContainsActorIDWithLimiter(actorID string, limiter worklimit.Limiter) (bool, error) {
	if idx == nil {
		return false, nil
	}
	if err := worklimit.ChargeStrings(limiter, []string{actorID}); err != nil {
		return false, err
	}
	_, ok := idx.actorIDs[strings.TrimSpace(actorID)]
	return ok, nil
}

// AddMatch records the local-identity subset of one actor match at its stable generation-local position.
func (idx *LocalActorMatchIndex) AddMatch(position int, match Match) {
	_ = idx.AddMatchWithLimiter(position, match, nil)
}

func (idx *LocalActorMatchIndex) AddMatchWithLimiter(position int, match Match, limiter worklimit.Limiter) error {
	if idx == nil || position < 0 {
		return nil
	}
	if err := ChargeMatch(limiter, match); err != nil {
		return err
	}
	for _, chassisID := range match.ChassisIDs {
		addLocalActorMatchPosition(idx.byChassisID, normalizeFoldedLocalActorKey(chassisID), position)
	}
	addLocalActorMatchPosition(idx.bySysName, normalizeFoldedLocalActorKey(match.SysName), position)
	for _, value := range match.IPAddresses {
		addLocalActorMatchPosition(idx.byIP, topologyutil.NormalizeIPAddress(value), position)
	}
	return nil
}

// FirstMatch returns the earliest recorded actor matching chassis ID, system name, or selected management IP.
func (idx *LocalActorMatchIndex) FirstMatch(local Device) (int, bool) {
	position, ok, _ := idx.FirstMatchWithLimiter(local, nil)
	return position, ok
}

func (idx *LocalActorMatchIndex) FirstMatchWithLimiter(local Device, limiter worklimit.Limiter) (int, bool, error) {
	if idx == nil {
		return 0, false, nil
	}
	if err := worklimit.ChargeStrings(limiter, []string{local.ChassisID, local.SysName, local.ManagementIP}); err != nil {
		return 0, false, err
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
	return best, found, nil
}

// MatchIndexes appends every unique matching actor position in actor order.
func (idx *LocalActorMatchIndex) MatchIndexes(dst []int, local Device) []int {
	result, _ := idx.MatchIndexesWithLimiter(dst, local, nil)
	return result
}

func (idx *LocalActorMatchIndex) MatchIndexesWithLimiter(dst []int, local Device, limiter worklimit.Limiter) ([]int, error) {
	if idx == nil {
		return dst, nil
	}
	if err := worklimit.ChargeStrings(limiter, []string{local.ChassisID, local.SysName, local.ManagementIP}); err != nil {
		return nil, err
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
			return dst, nil
		}
		if best != last {
			if err := limiter.Charge(1); err != nil {
				return nil, err
			}
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
