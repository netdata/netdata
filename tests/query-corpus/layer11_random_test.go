// SPDX-License-Identifier: GPL-3.0-or-later

// Layer 11, part two — randomized slicing with reproducible, greedy shrinking.
//
// Generated cases vary window bounds, resolution, tier, offset, option and data
// shape. A failure is repeatedly simplified while it remains a failure, yielding
// a locally minimal case under shrinkCandidates. QUERY_CORPUS_SEED replays the
// same generation and shrinking sequence.
package corpus

import (
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"testing"
)

type materializedSliceRequest struct {
	Axes          sliceAxes
	After, Before int64
	Points        int64
}

// randomSliceCase stores material inputs. Every check and shrink materializes
// the actual request again, so changing an axis cannot leave stale bounds.
type randomSliceCase struct {
	Shape       string
	UE          int
	Tier        int
	Option      string
	StartSample int64
	Offset      int64
	Records     int64
	Buckets     int64
}

func (c randomSliceCase) materialize() materializedSliceRequest {
	a := sliceAxes{
		Shape: c.Shape, UE: c.UE, Tier: c.Tier, Option: c.Option,
		StartOffset: c.Offset,
	}
	if c.Records > 0 {
		a.PointsPer = int(c.Buckets / c.Records)
	}
	if a.PointsPer < 1 {
		a.PointsPer = 1
	}
	after := sliceBase(c.UE) + int64(c.UE)*c.StartSample + c.Offset
	before := after + c.Records*sliceRecordDuration(a)
	points := c.Buckets
	if points < 1 {
		points = 1
	}
	return materializedSliceRequest{Axes: a, After: after, Before: before, Points: points}
}

func (c randomSliceCase) String() string {
	r := c.materialize()
	return fmt.Sprintf("%s window=(t0%+d,t0%+d] points=%d records=%d",
		r.Axes, r.After-fixture_T0(), r.Before-fixture_T0(), r.Points, c.Records)
}

func sliceCaseTotal(
	t *testing.T,
	c randomSliceCase,
	after, before int64,
) sliceQueryResult {
	t.Helper()
	r := c.materialize()
	points := r.Points * (before - after) / (r.Before - r.After)
	if points < 1 {
		points = 1
	}
	return sliceQuery(t, r.Axes, after, before, points)
}

type sliceCaseCheck struct {
	ok                     bool
	detail                 string
	additive, conservation bool
}

func checkCase(t *testing.T, c randomSliceCase) sliceCaseCheck {
	t.Helper()
	r := c.materialize()
	a := r.Axes
	check := sliceCaseCheck{ok: true}
	requireSliceTierCovers(t, a, r.After, r.Before)
	var whole sliceQueryResult
	haveWhole := false

	if r.Before-r.After >= 2*sliceRecordDuration(a) {
		mid := r.After + (r.Before-r.After)/2
		whole = sliceCaseTotal(t, c, r.After, r.Before)
		haveWhole = true
		// The released API can include the split endpoint in both halves. The
		// exact content of that one shared stored record is the allowance.
		left := sliceCaseTotal(t, c, r.After, mid)
		right := sliceCaseTotal(t, c, mid, r.Before)
		if !whole.validGrid || !left.validGrid || !right.validGrid {
			return sliceCaseCheck{detail: "additivity: invalid response grid"}
		}
		if sliceExpected(a.Shape, a.UE, r.After, r.Before) > 0 {
			if whole.numericRows == 0 || left.numericRows+right.numericRows == 0 {
				return sliceCaseCheck{detail: fmt.Sprintf(
					"additivity: fixture has data but numeric coverage is %d/%d+%d",
					whole.numericRows, left.numericRows, right.numericRows)}
			}
			edgeAllowance, sharedRecords, overlapOK := sliceSharedRecords(a, left, right)
			if !overlapOK {
				return sliceCaseCheck{detail: "additivity: malformed subquery views"}
			}
			if sharedRecords > 1 {
				goto conservation
			}
			check.additive = true
			difference := left.total + right.total - whole.total
			numericRows := whole.numericRows + left.numericRows + right.numericRows
			if !sliceWithinTolerance(difference, edgeAllowance, numericRows) {
				return sliceCaseCheck{additive: true, detail: fmt.Sprintf(
					"additivity: whole=%.1f halves=%.1f (left=%.1f right=%.1f, allowance=%.1f)",
					whole.total, left.total+right.total, left.total, right.total, edgeAllowance)}
			}
		}
	}

conservation:
	bucket := (r.Before - r.After) / r.Points
	if bucket >= int64(a.UE) {
		if !haveWhole {
			whole = sliceCaseTotal(t, c, r.After, r.Before)
			haveWhole = true
		}
		if !whole.validGrid {
			return sliceCaseCheck{additive: check.additive, detail: "conservation: invalid response grid"}
		}
		want := sliceExpected(a.Shape, a.UE, r.After, r.Before)
		if want > 0 {
			if whole.numericRows == 0 {
				return sliceCaseCheck{additive: check.additive,
					detail: "conservation: positive fixture content but no numeric rows"}
			}
			check.conservation = true
			edgeAllowance := sliceEdgeAllowance(a, r.After, r.Before)
			if !sliceWithinTolerance(whole.total-want, edgeAllowance, whole.numericRows) {
				return sliceCaseCheck{additive: check.additive, conservation: true, detail: fmt.Sprintf(
					"conservation: got=%.1f want=%.1f allowance=%.1f",
					whole.total, want, edgeAllowance)}
			}
		}
	}
	return check
}

// shrinkCase greedily reaches a locally minimal failing case under the
// simplifications in shrinkCandidates.
func shrinkCase(t *testing.T, c randomSliceCase) randomSliceCase {
	t.Helper()
	for progress := true; progress; {
		progress = false
		for _, simpler := range shrinkCandidates(c) {
			if !checkCase(t, simpler).ok {
				c = simpler
				progress = true
				break
			}
		}
	}
	return c
}

func sliceCaseSimpler(candidate, current randomSliceCase) bool {
	rank := func(c randomSliceCase) [6]int64 {
		var shape, option, offset, tier int64
		if c.Shape != "dense" {
			shape = 1
		}
		if c.Option != "" {
			option = 1
		}
		if c.Offset != 0 {
			offset = 1
		}
		if c.Tier != 0 {
			tier = 1
		}
		return [6]int64{shape, option, offset, tier, c.Records, c.Buckets}
	}

	candidateRank, currentRank := rank(candidate), rank(current)
	for i := range candidateRank {
		if candidateRank[i] != currentRank[i] {
			return candidateRank[i] < currentRank[i]
		}
	}
	return false
}

func shrinkCandidates(c randomSliceCase) []randomSliceCase {
	var candidates []randomSliceCase
	seen := map[materializedSliceRequest]bool{c.materialize(): true}
	add := func(change func(*randomSliceCase)) {
		candidate := c
		change(&candidate)
		request := candidate.materialize()
		if candidate.Records < 1 || candidate.Buckets < 1 || request.Before <= request.After ||
			!sliceCaseSimpler(candidate, c) || seen[request] {
			return
		}
		seen[request] = true
		candidates = append(candidates, candidate)
	}

	add(func(candidate *randomSliceCase) { candidate.Shape = "dense" })
	add(func(candidate *randomSliceCase) { candidate.Option = "" })
	add(func(candidate *randomSliceCase) { candidate.Offset = 0 })
	add(func(candidate *randomSliceCase) { candidate.Tier = 0 })
	add(func(candidate *randomSliceCase) { candidate.Buckets = candidate.Records })
	add(func(candidate *randomSliceCase) { candidate.Buckets = 1 })
	add(func(candidate *randomSliceCase) { candidate.Buckets = max(int64(1), candidate.Buckets/2) })
	add(func(candidate *randomSliceCase) {
		oldRecords := candidate.Records
		candidate.Records = max(int64(1), candidate.Records/2)
		candidate.Buckets = max(int64(1), candidate.Buckets*candidate.Records/oldRecords)
	})
	return candidates
}

func TestLayer11RandomisedSlicing(t *testing.T) {
	trackContract(t, "L11/randomised-slicing")

	seed := int64(20231114)
	if s := os.Getenv("QUERY_CORPUS_SEED"); s != "" {
		if v, err := strconv.ParseInt(s, 10, 64); err == nil {
			seed = v
		}
	}
	t.Logf("seed=%d (replay with QUERY_CORPUS_SEED=%d)", seed, seed)
	rng := rand.New(rand.NewSource(seed))

	for _, shape := range sliceShapes {
		for _, ue := range sliceUEs {
			sliceFixture(t, shape, ue)
		}
	}

	const rounds = 60
	cases := make([]randomSliceCase, 0, rounds)
	// Mandatory eligible cases guarantee coverage per shape/cadence/tier.
	for _, shape := range sliceShapes {
		for _, ue := range sliceUEs {
			for _, tier := range sliceTiers {
				cases = append(cases, randomSliceCase{
					Shape: shape, UE: ue, Tier: tier,
					StartSample: int64(200 + rng.Intn(200)), Offset: int64(rng.Intn(60)),
					Records: 8, Buckets: 8,
				})
			}
		}
	}
	for len(cases) < rounds {
		records := int64(4 + rng.Intn(12))
		pointsPerRecord := int64(slicePoints[rng.Intn(len(slicePoints))])
		cases = append(cases, randomSliceCase{
			Shape:       sliceShapes[rng.Intn(len(sliceShapes))],
			UE:          sliceUEs[rng.Intn(len(sliceUEs))],
			Tier:        sliceTiers[rng.Intn(len(sliceTiers))],
			Option:      sliceOptions[rng.Intn(len(sliceOptions))],
			StartSample: int64(200 + rng.Intn(400)),
			Offset:      int64(rng.Intn(60)), Records: records,
			Buckets: records * pointsPerRecord,
		})
	}

	ok := true
	failures := map[string]bool{}
	additiveCoverage := map[string]bool{}
	conservationCoverage := map[string]bool{}
	for _, c := range cases {
		check := checkCase(t, c)
		key := sliceCoverageKey(sliceAxes{Shape: c.Shape, UE: c.UE, Tier: c.Tier})
		additiveCoverage[key] = additiveCoverage[key] || check.additive
		conservationCoverage[key] = conservationCoverage[key] || check.conservation
		if check.ok {
			continue
		}

		minimal := shrinkCase(t, c)
		minimalCheck := checkCase(t, minimal)
		failureKey := minimal.String()
		if failures[failureKey] {
			continue
		}
		failures[failureKey] = true
		ok = false
		t.Logf("slicing property not met, greedily reduced to a locally minimal failing case:\n    %s\n    %s",
			failureKey, minimalCheck.detail)
	}
	for _, key := range missingSliceCoverage(additiveCoverage) {
		t.Logf("random additivity coverage missing for %s", key)
		ok = false
	}
	for _, key := range missingSliceCoverage(conservationCoverage) {
		t.Logf("random conservation coverage missing for %s", key)
		ok = false
	}

	t.Logf("%d randomized cases, %d distinct locally minimal failures", len(cases), len(failures))
	assertContract(t, "L11/randomised-slicing", ok)
}
