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
	c035FirstRate          = 10
	c035SecondRate         = 100
	c035FirstIdentityRate  = 10000
	c035SecondIdentityRate = 20000

	c035RateDim         = "load"
	c035IdentityRateDim = "identity_rate"
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
	t, interval                      int64
	rate, identityRate, availability float64
	gapDimensionCollected            bool
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

func c035CaseNamed(t *testing.T, name string) c035Case {
	t.Helper()
	for _, item := range c035Cases {
		if item.name == name {
			return item.spec
		}
	}
	t.Fatalf("CASE-035 fixture %q is not defined", name)
	return c035Case{}
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

func c035MeasuredIdentityRate(samples []c035Sample, after, before int64) float64 {
	total := 0.0
	for _, sample := range samples {
		if sample.t > after && sample.t <= before {
			total += sample.identityRate * float64(sample.interval)
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
			{ID: c035IdentityRateDim, Algorithm: "incremental"},
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
		identityRate := c035FirstIdentityRate + i
		if second {
			rate = c035SecondRate
			identityRate = c035SecondIdentityRate + i
		}
		gapCollected := second || c035GapCollected(base, ts, tc.firstEvery)
		ledger = append(ledger, c035Sample{
			t: ts, interval: int64(every),
			rate: float64(rate), identityRate: float64(identityRate), availability: availabilityValue,
			gapDimensionCollected: gapCollected,
		})
		ch.Dimensions[0].Points = append(ch.Dimensions[0].Points, point(strconv.Itoa(rate)))
		ch.Dimensions[1].Points = append(ch.Dimensions[1].Points, point(strconv.Itoa(identityRate)))
		ch.Dimensions[2].Points = append(ch.Dimensions[2].Points, point(availability))

		if gapCollected {
			ch.Dimensions[3].Points = append(ch.Dimensions[3].Points, point("1"))
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
// Source: netdata/netdata @ 043f50ec075441010c1495250871d37a8ac69f8d
// src/database/rrdset-collection.c:21-32
// rrdset_set_update_every_s() callout
// src/database/rrddim-collection.c:9-12,68-80
// tier_next_point_time_s(), store_metric_at_tier()
// src/database/engine/rrdengineapi.c:716-729
// rrdeng_store_metric_change_collection_frequency()
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
	//
	// Source: netdata/netdata @ c8f9ce4d5622767ea752a2877bf1049a0bc85a46
	// src/web/api/queries/tg-expression.h:366-436
	// tg_expression_window_fraction()
	// src/web/api/queries/percentage_of_time/percentage_of_time.h:57-75,86-105
	// tg_percentage_of_time_add_point(), tg_percentage_of_time_flush()
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

func c035ExpectedVolumeRows(
	samples []c035Sample,
	after, before, step int64,
	withMetadata, identity bool,
) []expectedColumnPoint {
	points := int((before - after) / step)
	want := make([]expectedColumnPoint, points)
	for i := range want {
		rowStart := after + int64(i)*step
		rowEnd := rowStart + step
		value := c035Measured(samples, rowStart, rowEnd)
		if identity {
			value = c035MeasuredIdentityRate(samples, rowStart, rowEnd)
		}
		if withMetadata {
			want[i] = wantNumberWithMetadataAt(rowEnd, value, 0, 0)
		} else {
			want[i] = wantNumberAt(rowEnd, value)
		}
	}
	return want
}

func c035ExpectedVolume(samples []c035Sample, after, before, step int64) []expectedColumnPoint {
	return c035ExpectedVolumeRows(samples, after, before, step, false, false)
}

func c035ExpectedCompleteVolume(samples []c035Sample, after, before, step int64) []expectedColumnPoint {
	return c035ExpectedVolumeRows(samples, after, before, step, true, false)
}

func c035ExpectedIdentityVolume(samples []c035Sample, after, before, step int64) []expectedColumnPoint {
	return c035ExpectedVolumeRows(samples, after, before, step, false, true)
}

type c035QuerySpec struct {
	context    string
	host       string
	dimension  string
	tier       int
	after      int64
	before     int64
	step       int64
	group      string
	expression string
	want       []expectedColumnPoint
}

func c035QueryExact(t *testing.T, spec c035QuerySpec) bool {
	t.Helper()
	params := daemon.DataParamsTier(
		spec.context, spec.tier, spec.after, spec.before,
		(spec.before-spec.after)/spec.step, spec.group)
	params.Set("options", "jsonwrap|unaligned")
	if spec.expression != "" {
		params.Set("time_group_options", spec.expression)
	}
	params.Set("scope_dimensions", spec.dimension)
	doc, err := td.DataV3(spec.host, params)
	if err != nil {
		t.Fatal(err)
	}
	cols, err := canon.Columns(doc)
	if err != nil {
		t.Fatal(err)
	}

	ok := assertExactView(t, doc, spec.after, spec.before, spec.step)
	if !assertSelectedTier(t, doc, spec.tier) {
		ok = false
	}
	if !assertOnlyColumn(t, cols, spec.dimension) {
		ok = false
	}
	tolerance := 0.0
	if spec.group == "percentage-of-time" {
		tolerance = printTol
	}
	if !assertExactColumn(t, cols, spec.dimension, spec.want, tolerance) {
		ok = false
	}
	return ok
}

// c035AvailabilityMatrix runs the same cadence-changing windows either at
// tier 0 or at each higher tier. Keeping those verdicts separate prevents a
// legacy page-format limitation from hiding an exact tier-0 regression.
func c035AvailabilityMatrix(
	t *testing.T,
	context, host string,
	base int64,
	tc c035Case,
	samples []c035Sample,
	higherTiers bool,
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

		tier := 0
		if higherTiers {
			tier = target.tier
		}
		records := c035Records(base, tc, tier, before, samples)
		if !c035QueryExact(t, c035QuerySpec{
			context: context, host: host, dimension: c035AvailabilityDim,
			tier: tier, after: after, before: before, step: granularity,
			group: "percentage-of-time", expression: "==1",
			want: c035ExpectedAvailability(records, after, before, granularity),
		}) {
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

		if !c035QueryExact(t, c035QuerySpec{
			context: context, host: host, dimension: c035GapDim,
			tier: tier, after: after, before: before, step: step,
			group: "percentage-of-samples", expression: "==gap", want: want,
		}) {
			ok = false
		}
	}
	return ok
}

type c035FixtureState struct {
	base          int64
	context, host string
	samples       []c035Sample
}

var c035Fixtures = map[string]c035FixtureState{}
var c035CompletedFixtures = map[string]c035FixtureState{}

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

func c035CompletedFixture(
	t *testing.T,
	name, stateName string,
	tc c035Case,
	firstSamples, guidOffset int,
) c035FixtureState {
	t.Helper()
	key := stateName + "/" + name
	if state, ok := c035CompletedFixtures[key]; ok {
		return state
	}

	base := int64(fixture.T0) - int64(fixture.T0)%36000
	context := "fixture.c035_" + stateName + "_" + name
	host := "c035-" + stateName + "-" + name

	ch1, samples1 := c035Phase(context, base, base, firstSamples, tc.firstEvery, false, tc)
	conn := connect(t, host, guid(tc.machineGUID+guidOffset), stream.CapsLive)
	ch1.Define(conn)
	ch1.PushLive(conn)
	if err := conn.Flush(); err != nil {
		t.Fatal(err)
	}
	if _, err := td.WaitRetention(host, context, ch1.FirstT(), ch1.LastT(), 30*time.Second); err != nil {
		t.Fatal(err)
	}

	boundary := base + 4*int64(tc.firstEvery)*tier2Gran
	transition := base + int64(firstSamples*tc.firstEvery)
	remainingOldWindow := boundary + int64(tc.firstEvery)*tier2Gran - transition
	secondSamples := int((remainingOldWindow+int64(tc.thenEvery)-1)/int64(tc.thenEvery)) + 2
	newTier2Every := int64(tc.thenEvery) * tier2Gran
	firstNewSampleEnd := transition + int64(tc.thenEvery)
	firstNewTier2End := firstNewSampleEnd + newTier2Every -
		(firstNewSampleEnd+newTier2Every)%newTier2Every
	// Advance through one additional tier2 row so both queried rows have left
	// the collector's one-point delayed buffer.
	newRowsSamples := int((firstNewTier2End+3*newTier2Every-transition)/int64(tc.thenEvery)) + 2
	if secondSamples < newRowsSamples {
		secondSamples = newRowsSamples
	}
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
	c035CompletedFixtures[key] = state
	return state
}

func c035BufferedFixture(t *testing.T, name string, tc c035Case) c035FixtureState {
	t.Helper()

	// The final old-cadence sample starts on a tier2 boundary. It promotes the
	// completed tier1 and tier2 points, then collection changes cadence before
	// another old-cadence sample can trigger their modulo-based early flush.
	return c035CompletedFixture(t, name, "buffered", tc, 4*tier2Gran+1, 10)
}

func c035BoundaryFixture(t *testing.T, name string, tc c035Case) c035FixtureState {
	t.Helper()

	// The final old-cadence sample ends exactly on a tier2 boundary. The
	// completed tier1 and tier2 points remain in virtual_point until the first
	// new-cadence sample arrives.
	return c035CompletedFixture(t, name, "boundary", tc, 4*tier2Gran, 20)
}

func c035TransitionWindow(base int64, tc c035Case, grouping int64) (after, before, step int64) {
	step = int64(tc.firstEvery) * grouping
	start := base + c035TransitionOffset(tc)/step*step
	before = start + step
	after = before - 2*step
	return after, before, step
}

func TestCase035CompletedRollupKeepsOriginalCadence(t *testing.T) {
	const contract = "CASE-035/completed-rollup-keeps-original-cadence"
	for _, item := range c035Cases {
		item := item
		t.Run(item.name, func(t *testing.T) {
			states := []struct {
				name  string
				state c035FixtureState
			}{
				{name: "buffered", state: c035BufferedFixture(t, item.name, item.spec)},
				{name: "boundary", state: c035BoundaryFixture(t, item.name, item.spec)},
			}
			for _, stateCase := range states {
				stateCase := stateCase
				t.Run(stateCase.name, func(t *testing.T) {
					boundary := stateCase.state.base + 4*int64(item.spec.firstEvery)*tier2Gran
					for _, target := range []struct {
						tier     int
						grouping int64
					}{
						{tier: 1, grouping: tier1Gran},
						{tier: 2, grouping: tier2Gran},
					} {
						target := target
						t.Run("tier"+strconv.Itoa(target.tier), func(t *testing.T) {
							component := stateCase.name + "-" + item.name +
								"-tier" + strconv.Itoa(target.tier)
							trackContractComponent(t, contract, component)

							step := int64(item.spec.firstEvery) * target.grouping
							after, before := boundary-2*step, boundary
							if !c035QueryExact(t, c035QuerySpec{
								context: stateCase.state.context, host: stateCase.state.host,
								dimension: c035RateDim, tier: target.tier,
								after: after, before: before, step: step, group: "sum",
								want: c035ExpectedCompleteVolume(stateCase.state.samples, after, before, step),
							}) {
								t.Errorf("BROKEN %s (%s): %s",
									contract, component, manifest[contract].Proves)
							}

							// Exact-boundary handling resets the rollup grid. Two complete
							// new-cadence rows prove that no old boundary or empty point leaks
							// into subsequent storage.
							if stateCase.name == "boundary" {
								newStep := int64(item.spec.thenEvery) * target.grouping
								firstNewSampleEnd := boundary + int64(item.spec.thenEvery)
								newAfter := firstNewSampleEnd + newStep -
									(firstNewSampleEnd+newStep)%newStep
								newBefore := newAfter + 2*newStep
								// Repeat the identical crossing query. A false gap cached by the
								// first pass must not hide the first new-cadence page on the second.
								for attempt := 1; attempt <= 2; attempt++ {
									if !c035QueryExact(t, c035QuerySpec{
										context: stateCase.state.context, host: stateCase.state.host,
										dimension: c035RateDim, tier: target.tier,
										after: newAfter, before: newBefore, step: newStep, group: "sum",
										want: c035ExpectedCompleteVolume(
											stateCase.state.samples, newAfter, newBefore, newStep),
									}) {
										t.Errorf("BROKEN %s (%s-new-grid-attempt-%d): %s",
											contract, component, attempt, manifest[contract].Proves)
									}
								}
							}
						})
					}
				})
			}
		})
	}
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
				if !c035QueryExact(t, c035QuerySpec{
					context: state.context, host: state.host, dimension: c035RateDim,
					tier: target.tier, after: after, before: before, step: step,
					group: "sum", want: want,
				}) {
					t.Logf("tier%d query over (%d,%d] did not preserve each row's exact "+
						"fixture-measured rate x collection interval volume",
						target.tier, after, before)
					ok = false
				}

				// The dedicated page-boundary component below owns the tier1-width
				// tier0 control. Keep the distinct tier2-width control here.
				if target.tier != 2 {
					continue
				}
				if !c035QueryExact(t, c035QuerySpec{
					context: state.context, host: state.host, dimension: c035RateDim,
					tier: 0, after: after, before: before, step: step,
					group: "sum", want: want,
				}) {
					t.Logf("tier0 query over (%d,%d] did not preserve each row's exact "+
						"fixture-measured rate x collection interval volume", after, before)
					ok = false
				}
			}
			assertContract(t, tc.contract, ok)
		})
	}
}

func TestCase035Tier0PageBoundaryKeepsEverySample(t *testing.T) {
	const contract = "CASE-035/tier0-page-boundary-keeps-every-sample"
	for _, item := range c035Cases {
		item := item
		t.Run(item.name, func(t *testing.T) {
			trackContractComponent(t, contract, item.name)
			state := c035Fixture(t, item.name, item.spec)
			after, before, step := c035TransitionWindow(state.base, item.spec, tier1Gran)
			if !c035QueryExact(t, c035QuerySpec{
				context: state.context, host: state.host, dimension: c035IdentityRateDim,
				tier: 0, after: after, before: before, step: step,
				group: "sum", want: c035ExpectedIdentityVolume(state.samples, after, before, step),
			}) {
				t.Errorf("BROKEN %s (%s): %s", contract, item.name, manifest[contract].Proves)
			}
		})
	}
}

func TestCase023AvailabilityAcrossIntervalChange(t *testing.T) {
	for _, item := range c035Cases {
		item := item
		t.Run(item.name, func(t *testing.T) {
			state := c035Fixture(t, item.name, item.spec)
			for _, scope := range []struct {
				name, contract string
				higher         bool
			}{
				{name: "tier0", contract: "CASE-023/cadence-change-availability-tier0"},
				{name: "higher-tiers", contract: "CASE-023/cadence-change-availability-higher-tiers", higher: true},
			} {
				scope := scope
				t.Run(scope.name, func(t *testing.T) {
					trackContractComponent(t, scope.contract, item.name)
					if !c035AvailabilityMatrix(t, state.context, state.host,
						state.base, item.spec, state.samples, scope.higher) {
						t.Errorf("BROKEN %s (%s): %s",
							scope.contract, item.name, manifest[scope.contract].Proves)
					}
				})
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
	const name = "speeds-up"
	tc := c035CaseNamed(t, name)
	state := c035Fixture(t, name, tc)
	assertContract(t, contract,
		c035GapMatrix(t, state.context, state.host, state.base, tc, state.samples))
}
