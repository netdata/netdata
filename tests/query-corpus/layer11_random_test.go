// SPDX-License-Identifier: GPL-3.0-or-later

// Layer 11, part two — randomised slicing, with shrinking.
//
// The matrix in layer11_slicing_test.go covers every PAIR of axis values I
// thought to list. That is the right coverage target for combinations, and it
// is still a list I curated: it finds what I anticipated.
//
// Every slicing defect in this corpus so far escaped exactly that way. The
// aggregation sweep held the window alignment still; the alignment tests held
// the data shape still; the shape tests never turned an option on. Each time
// the bug sat in the axis I had pinned, and each time a person found it
// before the corpus did.
//
// So this generates configurations instead of listing them - window bounds,
// resolution, tier, offset and shape drawn at random - and checks the same two
// properties. When one fails it SHRINKS the case: repeatedly simplifies it and
// keeps whatever still fails, until nothing can be simplified further. What
// gets reported is not the random case that happened to break but the smallest
// case that breaks, which is the difference between a lead and a bug report.
//
// It is seeded and therefore reproducible: the seed is printed on every run
// and QUERY_CORPUS_SEED replays it exactly.
package corpus

import (
	"fmt"
	"math"
	"math/rand"
	"os"
	"strconv"
	"testing"
)

// sliceCase is one generated question: how to slice, and what to slice.
type sliceCase struct {
	Axes          sliceAxes
	After, Before int64
	Points        int64
}

func (c sliceCase) String() string {
	return fmt.Sprintf("%s window=(t0%+d,t0%+d] points=%d",
		c.Axes, c.After-int64(fixture_T0()), c.Before-int64(fixture_T0()), c.Points)
}

// sliceExpected is the fixture's own arithmetic: what it pushed into the
// window, in the metric's units.
func sliceExpected(shape string, ue int, after, before int64) float64 {
	want := 0.0
	base := int64(fixture_T0()) - int64(fixture_T0())%int64(ue)
	for i := 1; i <= sliceSamples; i++ {
		ts := base + int64(i*ue)
		if ts > after && ts <= before && sliceCollected(shape, i) {
			want += sliceValue
		}
	}
	return want
}

// sliceCaseTotal answers the case as asked.
func sliceCaseTotal(t *testing.T, c sliceCase, after, before int64) float64 {
	t.Helper()
	a := c.Axes
	// keep the resolution of the case when re-asking a sub-window
	points := c.Points * (before - after) / (c.Before - c.After)
	if points < 1 {
		points = 1
	}
	return sliceTotalPoints(t, a, after, before, points)
}

// checkCase runs both properties against one generated case.
func checkCase(t *testing.T, c sliceCase) (ok bool, detail string) {
	t.Helper()
	a := c.Axes

	recordEvery := int64(a.UE)
	if a.Tier > 0 {
		recordEvery = int64(a.UE) * tier1Gran
	}
	recordContent := float64(sliceValue) * float64(a.UE)
	if a.Tier > 0 {
		recordContent *= float64(tier1Gran)
	}

	// additivity: the halves total the whole. No oracle, no preconditions
	// beyond having something to split.
	if c.Before-c.After >= 2*recordEvery {
		mid := c.After + (c.Before-c.After)/2
		whole := sliceCaseTotal(t, c, c.After, c.Before)
		left := sliceCaseTotal(t, c, c.After, mid)
		right := sliceCaseTotal(t, c, mid, c.Before)
		tol := recordContent * 1.05
		if tol < 1e-6 {
			tol = 1e-6
		}
		if math.Abs((left+right)-whole) > tol {
			return false, fmt.Sprintf("additivity: whole=%.1f halves=%.1f (left=%.1f right=%.1f)",
				whole, left+right, left, right)
		}
	}

	// conservation: only where the chart points are at least as wide as the
	// collection interval, and only against a tier that covers the window
	bucket := (c.Before - c.After) / c.Points
	if bucket >= int64(a.UE) && sliceTierCovers(t, a, c.After, c.Before) {
		got := sliceTotalPoints(t, a, c.After, c.Before, c.Points)
		want := sliceExpected(a.Shape, a.UE, c.After, c.Before)
		if math.Abs(got-want) > recordContent*2.1 {
			return false, fmt.Sprintf("conservation: got=%.1f want=%.1f", got, want)
		}
	}

	return true, ""
}

// shrinkCase reduces a failing case to the smallest one that still fails.
// Each step is a simplification a human would try; whatever still fails is
// kept, and the walk repeats until nothing simplifies.
func shrinkCase(t *testing.T, c sliceCase) sliceCase {
	t.Helper()
	for progress := true; progress; {
		progress = false
		for _, simpler := range shrinkCandidates(c) {
			if bad, _ := checkCase(t, simpler); !bad {
				c = simpler
				progress = true
				break
			}
		}
	}
	return c
}

// shrinkCandidates lists one-step simplifications, simplest first.
func shrinkCandidates(c sliceCase) []sliceCase {
	var out []sliceCase
	add := func(f func(*sliceCase)) {
		s := c
		f(&s)
		if s != c && s.Before > s.After && s.Points >= 1 {
			out = append(out, s)
		}
	}

	add(func(s *sliceCase) { s.Axes.Shape = "dense" })     // the plainest data
	add(func(s *sliceCase) { s.Axes.Option = "" })         // no option flag
	add(func(s *sliceCase) { s.Axes.StartOffset = 0 })     // on the grid
	add(func(s *sliceCase) { s.Axes.Tier = 0 })            // the simplest tier
	add(func(s *sliceCase) { s.Axes.PointsPer = 1 })       // one point per record
	add(func(s *sliceCase) { s.Points = 1 })               // a single bucket
	add(func(s *sliceCase) { s.Points = s.Points / 2 })    // fewer buckets
	add(func(s *sliceCase) {                               // half the window
		s.Before = s.After + (s.Before-s.After)/2
		s.Points = s.Points / 2
	})
	return out
}

func TestLayer11RandomisedSlicing(t *testing.T) {
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
	ok := true
	seen := map[string]bool{}

	for i := 0; i < rounds; i++ {
		a := sliceAxes{
			Shape:       sliceShapes[rng.Intn(len(sliceShapes))],
			UE:          sliceUEs[rng.Intn(len(sliceUEs))],
			Tier:        sliceTiers[rng.Intn(len(sliceTiers))],
			PointsPer:   slicePoints[rng.Intn(len(slicePoints))],
			StartOffset: int64(rng.Intn(60)),
			Option:      sliceOptions[rng.Intn(len(sliceOptions))],
		}

		recordEvery := int64(a.UE)
		if a.Tier > 0 {
			recordEvery = int64(a.UE) * tier1Gran
		}

		base := int64(fixture_T0()) - int64(fixture_T0())%int64(a.UE)
		// a window well inside the fixture, of a random length in records
		records := int64(4 + rng.Intn(12))
		after := base + int64(a.UE)*int64(200+rng.Intn(400)) + a.StartOffset
		before := after + records*recordEvery
		points := records * int64(a.PointsPer)
		if points < 1 {
			points = 1
		}

		c := sliceCase{Axes: a, After: after, Before: before, Points: points}

		good, _ := checkCase(t, c)
		if good {
			continue
		}

		min := shrinkCase(t, c)
		_, detail := checkCase(t, min)
		key := min.String()
		if seen[key] {
			continue
		}
		seen[key] = true
		ok = false
		t.Logf("slicing property not met, shrunk to the smallest failing case:\n    %s\n    %s",
			key, detail)
	}

	t.Logf("%d randomised cases, %d distinct minimal failures", rounds, len(seen))
	expectAgentStatus(t, "L11/randomised-slicing", ok)
}
