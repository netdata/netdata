// SPDX-License-Identifier: GPL-3.0-or-later

// The mixed-cadence fixture supports CASE-035 rate volume and the CASE-023
// availability/gap contracts. It changes both interval and value so
// cadence-blind arithmetic is observable.
package corpus

import (
	"strconv"
	"testing"
	"time"

	"github.com/netdata/netdata/tests/query-corpus/canon"
	"github.com/netdata/netdata/tests/query-corpus/daemon"
	"github.com/netdata/netdata/tests/query-corpus/fixture"
	"github.com/netdata/netdata/tests/query-corpus/stream"
)

const (
	c035FirstRate  = 10
	c035SecondRate = 100

	c035RateDim         = "load"
	c035AvailabilityDim = "availability"
	c035GapDim          = "gaps"
)

type c035Case struct {
	contract              string
	firstEvery, thenEvery int
	machineGUID           int
}

type c035Record struct {
	start, end                 int64
	matchedWeight, totalWeight float64
}

type c035Sample struct {
	t, interval           int64
	rate, availability    float64
	gapDimensionCollected bool
}

var c035Cases = []struct {
	name string
	spec c035Case
}{
	{
		name: "slows-down",
		spec: c035Case{
			contract:    "CASE-035/transition-volume-slowing-down",
			firstEvery:  1,
			thenEvery:   10,
			machineGUID: 296,
		},
	},
	{
		name: "speeds-up",
		spec: c035Case{
			contract:    "CASE-035/transition-volume-speeding-up",
			firstEvery:  10,
			thenEvery:   1,
			machineGUID: 297,
		},
	},
}

func c035TransitionOffset(tc c035Case) int64 {
	// Four and a half complete old-cadence tier2 records precede the
	// transition. The extra 90 seconds is divisible by both fixture
	// cadences but off the tier1 and tier2 grids in both directions, making
	// sample weighting materially different from elapsed-time weighting.
	granularity := int64(tc.firstEvery) * tier2Gran
	return 4*granularity + granularity/2 + 90
}

func c035Measured(samples []c035Sample, after, before int64) float64 {
	total := 0.0
	for _, sample := range samples {
		if sample.t > after && sample.t <= before {
			total += sample.rate * float64(sample.interval)
		}
	}
	return total
}

// c035Phase builds one definition of the chart. The second definition changes
// update_every on the same chart, exactly as a collector changing cadence
// does. Availability moves from 0 to 1 at the same boundary.
func c035Phase(
	context string,
	base, start int64,
	sampleCount, every int,
	second bool,
	tc c035Case,
) (fixture.Chart, []c035Sample) {
	ch := fixture.Chart{
		ID: context, Title: context, Units: "units/s", Family: "fixture",
		Context: context, UpdateEvery: every,
		Dimensions: []fixture.Dimension{
			{ID: c035RateDim, Algorithm: "incremental"},
			{ID: c035AvailabilityDim},
			{ID: c035GapDim},
		},
	}

	availability := "0"
	availabilityValue := 0.0
	if second {
		availability = "1"
		availabilityValue = 1
	}
	ledger := make([]c035Sample, 0, sampleCount)
	for i := 1; i <= sampleCount; i++ {
		ts := start + int64(i*every)
		point := func(value string) fixture.Point {
			return fixture.Point{T: ts, Collected: value, Flags: stream.FlagNotAnomalous}
		}

		rate := c035FirstRate
		if second {
			rate = c035SecondRate
		}
		gapCollected := second || c035GapCollected(base, ts, tc.firstEvery)
		ledger = append(ledger, c035Sample{
			t: ts, interval: int64(every),
			rate: float64(rate), availability: availabilityValue,
			gapDimensionCollected: gapCollected,
		})
		ch.Dimensions[0].Points = append(ch.Dimensions[0].Points, point(strconv.Itoa(rate)))
		ch.Dimensions[1].Points = append(ch.Dimensions[1].Points, point(availability))

		if gapCollected {
			ch.Dimensions[2].Points = append(ch.Dimensions[2].Points, point("1"))
		}
	}
	return ch, ledger
}

// The first four old-cadence records contain alternating numeric and missing
// records. At tier2, record zero remains numeric but contains the same two
// missing tier1 records used by the tier1 query. Records one and three are
// wholly absent. Other dimensions keep the chart alive, so these are genuine
// per-dimension holes rather than missing chart rows.
func c035GapCollected(base, ts int64, every int) bool {
	slot := (ts - base - 1) / int64(every)
	tier2Record := slot / tier2Gran
	if tier2Record == 1 || tier2Record == 3 {
		return false
	}
	tier1Record := (slot % tier2Gran) / tier1Gran
	return tier2Record != 0 || (tier1Record != 1 && tier1Record != 3)
}

// c035Records is the Class B fixture oracle at one selected storage tier.
// Tier0 records retain each sample's own collection interval. Higher-tier
// records use the old-cadence grid through the deliberately mixed record
// because collection-frequency changes flush storage pages but not the
// in-progress tier rollup. The rollup retains sum/min/max/count, not each
// sample's historical duration.
//
// Source: src/database/rrdset-collection.c:21-32,
// src/database/rrddim-collection.c:68-80 and
// src/database/engine/rrdengineapi.c:716-729.
func c035Records(
	base int64,
	tc c035Case,
	tier int,
	before int64,
	samples []c035Sample,
) []c035Record {
	grouping := int64(1)
	if tier == 1 {
		grouping = tier1Gran
	} else if tier == 2 {
		grouping = tier2Gran
	}
	granularity := int64(tc.firstEvery) * grouping

	var records []c035Record
	add := func(sample c035Sample) {
		if sample.t > before {
			return
		}
		if tier == 0 {
			records = append(records, c035Record{
				start:         sample.t - sample.interval,
				end:           sample.t,
				matchedWeight: sample.availability * float64(sample.interval),
				totalWeight:   float64(sample.interval),
			})
			return
		}

		end := base + ((sample.t-base+granularity-1)/granularity)*granularity
		if len(records) == 0 || records[len(records)-1].end != end {
			records = append(records, c035Record{start: end - granularity, end: end})
		}
		record := &records[len(records)-1]
		record.totalWeight++
		record.matchedWeight += sample.availability
	}

	for _, sample := range samples {
		add(sample)
	}
	return records
}

func c035Overlap(record c035Record, after, before int64) int64 {
	from := record.start
	if from < after {
		from = after
	}
	to := record.end
	if to > before {
		to = before
	}
	if to <= from {
		return 0
	}
	return to - from
}

func c035ExpectedAvailability(records []c035Record, after, before, step int64) []expectedColumnPoint {
	// The higher-tier fraction mirrors the approved two-point mass model.
	// For this 0/1 fixture, avg is exactly the sample-weighted share at 1.
	// Source: src/web/api/queries/tg-expression.h:366-438.
	points := int((before - after) / step)
	want := make([]expectedColumnPoint, points)
	for i := range want {
		rowStart := after + int64(i)*step
		rowEnd := rowStart + step
		up := 0.0
		for _, record := range records {
			overlap := c035Overlap(record, rowStart, rowEnd)
			if overlap == 0 {
				continue
			}
			if record.totalWeight <= 0 {
				continue
			}
			fraction := record.matchedWeight / record.totalWeight
			up += fraction * float64(overlap)
		}
		want[i] = wantNumberAt(rowEnd, 100*up/float64(step))
	}
	return want
}

func c035ExpectedVolume(samples []c035Sample, after, before, step int64) []expectedColumnPoint {
	points := int((before - after) / step)
	want := make([]expectedColumnPoint, points)
	for i := range want {
		rowStart := after + int64(i)*step
		rowEnd := rowStart + step
		want[i] = wantNumberAt(rowEnd, c035Measured(samples, rowStart, rowEnd))
	}
	return want
}

func c035ConditionQuery(
	t *testing.T,
	context, host, dimension string,
	tier int,
	after, before, step int64,
	group, expression string,
	want []expectedColumnPoint,
) bool {
	t.Helper()
	params := daemon.DataParamsTier(context, tier, after, before, (before-after)/step, group)
	params.Set("options", "jsonwrap|unaligned")
	params.Set("time_group_options", expression)
	params.Set("scope_dimensions", dimension)
	doc, err := td.DataV3(host, params)
	if err != nil {
		t.Fatal(err)
	}
	cols, err := canon.Columns(doc)
	if err != nil {
		t.Fatal(err)
	}

	ok := assertExactView(t, doc, after, before, step)
	if !assertSelectedTier(t, doc, tier) {
		ok = false
	}
	if !assertOnlyColumn(t, cols, dimension) {
		ok = false
	}
	tolerance := 0.0
	if group == "percentage-of-time" {
		tolerance = printTol
	}
	if !assertExactColumn(t, cols, dimension, want, tolerance) {
		ok = false
	}
	return ok
}

// c035AvailabilityMatrix isolates the cadence-changing record at each higher
// tier. One preceding row is the control and avoids the independent points=1
// window defect; zoom behavior is covered by the general CASE-023 matrices.
func c035AvailabilityMatrix(
	t *testing.T,
	context, host string,
	base int64,
	tc c035Case,
	samples []c035Sample,
) bool {
	t.Helper()
	ok := true
	for _, target := range []struct {
		tier     int
		grouping int64
	}{
		{tier: 1, grouping: tier1Gran},
		{tier: 2, grouping: tier2Gran},
	} {
		granularity := int64(tc.firstEvery) * target.grouping
		straddleStart := base + c035TransitionOffset(tc)/granularity*granularity
		after := straddleStart - granularity
		before := straddleStart + granularity

		records := c035Records(base, tc, target.tier, before, samples)
		if !c035ConditionQuery(t, context, host, c035AvailabilityDim,
			target.tier, after, before, granularity,
			"percentage-of-time", "==1",
			c035ExpectedAvailability(records, after, before, granularity)) {
			ok = false
		}

		// The same window on tier 0 proves the exact elapsed-time answer
		// independently of the higher-tier sample-weighted estimator.
		records = c035Records(base, tc, 0, before, samples)
		if !c035ConditionQuery(t, context, host, c035AvailabilityDim,
			0, after, before, granularity,
			"percentage-of-time", "==1",
			c035ExpectedAvailability(records, after, before, granularity)) {
			ok = false
		}
	}
	return ok
}

func c035ExpectedGapRows(
	tc c035Case,
	tier int,
	after, before, step int64,
	samples []c035Sample,
) []expectedColumnPoint {
	slot := int64(tc.firstEvery)
	if tier == 1 {
		slot *= tier1Gran
	} else if tier == 2 {
		slot *= tier2Gran
	}

	points := int((before - after) / step)
	want := make([]expectedColumnPoint, points)
	for i := range want {
		rowStart := after + int64(i)*step
		rowEnd := rowStart + step
		collected, gaps := 0, 0
		for slotEnd := rowStart + slot; slotEnd <= rowEnd; slotEnd += slot {
			slotStart := slotEnd - slot
			hasData := false
			for _, sample := range samples {
				if sample.t > slotStart && sample.t <= slotEnd && sample.gapDimensionCollected {
					hasData = true
					break
				}
			}
			if hasData {
				collected++
			} else {
				gaps++
			}
		}
		want[i] = wantNumberAt(rowEnd, 100*float64(gaps)/float64(collected+gaps))
	}
	return want
}

func c035GapMatrix(
	t *testing.T,
	context, host string,
	base int64,
	tc c035Case,
	samples []c035Sample,
) bool {
	t.Helper()
	ok := true
	for _, tier := range []int{0, 1, 2} {
		grouping := int64(tier1Gran)
		if tier == 2 {
			grouping = tier2Gran
		}
		granularity := int64(tc.firstEvery) * grouping
		after := base
		before := base + 4*granularity
		step := 2 * granularity
		want := c035ExpectedGapRows(tc, tier, after, before, step, samples)

		params := daemon.DataParamsTier(context, tier, after, before, 2, "percentage-of-samples")
		params.Set("options", "jsonwrap|unaligned")
		params.Set("time_group_options", "==gap")
		params.Set("scope_dimensions", c035GapDim)
		doc, err := td.DataV3(host, params)
		if err != nil {
			t.Fatal(err)
		}
		cols, err := canon.Columns(doc)
		if err != nil {
			t.Fatal(err)
		}
		if !assertExactView(t, doc, after, before, step) ||
			!assertSelectedTier(t, doc, tier) ||
			!assertOnlyColumn(t, cols, c035GapDim) ||
			!assertExactColumn(t, cols, c035GapDim, want, 0) {
			ok = false
		}
	}
	return ok
}

func c035QueryVolume(
	t *testing.T,
	context, host string,
	tier int,
	after, before, step int64,
	want []expectedColumnPoint,
) bool {
	t.Helper()

	params := daemon.DataParamsTier(context, tier, after, before, (before-after)/step, "sum")
	params.Set("options", "jsonwrap|unaligned")
	params.Set("scope_dimensions", c035RateDim)
	doc, err := td.DataV3(host, params)
	if err != nil {
		t.Fatal(err)
	}
	cols, err := canon.Columns(doc)
	if err != nil {
		t.Fatal(err)
	}

	ok := assertExactView(t, doc, after, before, step)
	if !assertSelectedTier(t, doc, tier) {
		ok = false
	}
	if !assertOnlyColumn(t, cols, c035RateDim) {
		ok = false
	}
	if !assertExactColumn(t, cols, c035RateDim, want, 0) {
		t.Logf("tier%d query over (%d,%d] did not preserve each row's exact "+
			"fixture-measured rate x collection interval volume", tier, after, before)
		ok = false
	}
	return ok
}

type c035FixtureState struct {
	base          int64
	context, host string
	samples       []c035Sample
}

var c035Fixtures = map[string]c035FixtureState{}

func c035Fixture(t *testing.T, name string, tc c035Case) c035FixtureState {
	t.Helper()
	if state, ok := c035Fixtures[name]; ok {
		return state
	}

	base := int64(fixture.T0) - int64(fixture.T0)%36000
	context := "fixture.c035_" + name
	host := "c035-" + name
	transitionOffset := c035TransitionOffset(tc)
	firstSamples := int(transitionOffset) / tc.firstEvery
	ch1, samples1 := c035Phase(context, base, base, firstSamples, tc.firstEvery, false, tc)

	conn := connect(t, host, guid(tc.machineGUID), stream.CapsLive)
	ch1.Define(conn)
	ch1.PushLive(conn)
	if err := conn.Flush(); err != nil {
		t.Fatal(err)
	}
	if _, err := td.WaitRetention(host, context, ch1.FirstT(), ch1.LastT(), 30*time.Second); err != nil {
		t.Fatal(err)
	}

	transition := base + transitionOffset
	tier2Every := int64(tc.firstEvery) * tier2Gran
	tier2End := base + (transitionOffset/tier2Every+1)*tier2Every
	secondSamples := int((tier2End-transition)/int64(tc.thenEvery)) + 4000
	ch2, samples2 := c035Phase(context, base, transition, secondSamples, tc.thenEvery, true, tc)
	ch2.Define(conn)
	ch2.PushLive(conn)
	if err := conn.Flush(); err != nil {
		t.Fatal(err)
	}
	if _, err := td.WaitRetention(host, context, ch1.FirstT(), ch2.LastT(), 60*time.Second); err != nil {
		t.Fatal(err)
	}

	state := c035FixtureState{
		base: base, context: context, host: host,
		samples: append(samples1, samples2...),
	}
	c035Fixtures[name] = state
	return state
}

func c035TransitionWindow(base int64, tc c035Case, grouping int64) (after, before, step int64) {
	step = int64(tc.firstEvery) * grouping
	start := base + c035TransitionOffset(tc)/step*step
	before = start + step
	after = before - 2*step
	return after, before, step
}

func TestCase035RateVolumeAcrossIntervalChange(t *testing.T) {
	for _, item := range c035Cases {
		item := item
		t.Run(item.name, func(t *testing.T) {
			tc := item.spec
			trackContract(t, tc.contract)
			state := c035Fixture(t, item.name, tc)

			ok := true
			for _, target := range []struct {
				tier     int
				grouping int64
			}{
				{tier: 1, grouping: tier1Gran},
				{tier: 2, grouping: tier2Gran},
			} {
				after, before, step := c035TransitionWindow(state.base, tc, target.grouping)
				want := c035ExpectedVolume(state.samples, after, before, step)
				if !c035QueryVolume(t, state.context, state.host, target.tier,
					after, before, step, want) {
					ok = false
				}

				// The same window on tier 0 independently proves the
				// fixture-ledger interpretation at this row width.
				if !c035QueryVolume(t, state.context, state.host, 0,
					after, before, step, want) {
					ok = false
				}
			}
			assertContract(t, tc.contract, ok)
		})
	}
}

func TestCase023AvailabilityAcrossIntervalChange(t *testing.T) {
	const contract = "CASE-023/cadence-change-availability"
	for _, item := range c035Cases {
		item := item
		t.Run(item.name, func(t *testing.T) {
			trackContractComponent(t, contract, item.name)
			state := c035Fixture(t, item.name, item.spec)
			if !c035AvailabilityMatrix(t, state.context, state.host,
				state.base, item.spec, state.samples) {
				t.Errorf("BROKEN %s (%s): %s", contract, item.name, manifest[contract].Proves)
			}
		})
	}
}

func TestCase023HistoricalGapSlotsSurviveCadenceChange(t *testing.T) {
	const contract = "CASE-023/historical-gap-slots-after-cadence-change"
	trackContract(t, contract)

	// Speeding up is the discriminating direction: using the new 1-second
	// cadence makes each old 10-second gap count ten times. Slowing down
	// clamps both the correct and wrong one-slot answers to one.
	item := c035Cases[1]
	state := c035Fixture(t, item.name, item.spec)
	assertContract(t, contract,
		c035GapMatrix(t, state.context, state.host, state.base, item.spec, state.samples))
}
