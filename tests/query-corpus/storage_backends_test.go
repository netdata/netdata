// SPDX-License-Identifier: GPL-3.0-or-later

package corpus

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/netdata/netdata/tests/query-corpus/canon"
	"github.com/netdata/netdata/tests/query-corpus/daemon"
	"github.com/netdata/netdata/tests/query-corpus/fixture"
	"github.com/netdata/netdata/tests/query-corpus/stream"
)

const (
	storageBackendContext = "fixture.storage_backend_gap_state"
	storageBackendHost    = "storage-backend-gap-state"
	storageBackendDim     = "value"
)

func startDedicatedStorageDaemon(t *testing.T, options daemon.Options) *daemon.Daemon {
	t.Helper()

	options.Binary = netdataBinary
	options.RunDir = t.TempDir()
	dd, err := daemon.Start(options)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := dd.Stop(); err != nil {
			t.Errorf("dedicated daemon shutdown: %v", err)
		}
	})
	return dd
}

func pushDedicatedChart(
	t *testing.T, dd *daemon.Daemon, host, machineGUID string, ch fixture.Chart,
) func() error {
	t.Helper()

	conn, err := stream.Connect(dd.Addr, dd.StreamKey, stream.HostInfo{
		Hostname: host, MachineGUID: machineGUID,
	}, stream.CapsLive)
	if err != nil {
		t.Fatal(err)
	}
	var closeOnce sync.Once
	var closeErr error
	closeConn := func() error {
		closeOnce.Do(func() { closeErr = conn.Close() })
		return closeErr
	}
	t.Cleanup(func() { _ = closeConn() })

	ch.Define(conn)
	ch.PushLive(conn)
	if err := conn.Flush(); err != nil {
		t.Fatal(err)
	}
	return closeConn
}

func pushStorageBackendFixture(
	t *testing.T, dd *daemon.Daemon, machineGUID string, expectedFirst int64,
) func() error {
	t.Helper()

	ch := fixture.Series(storageBackendContext, storageBackendContext, fixture.T0, 6, 1,
		func(i int) string {
			return strconv.Itoa([]int{0, 10, 0, 20, 30, 0, 40}[i])
		},
		func(i int) string {
			if i == 2 || i == 5 {
				return stream.FlagEmpty
			}
			return stream.FlagNotAnomalous
		})
	ch.Dimensions[0].ID = storageBackendDim

	closeConn := pushDedicatedChart(t, dd, storageBackendHost, machineGUID, ch)
	if _, err := dd.WaitRetention(
		storageBackendHost, storageBackendContext, expectedFirst, ch.LastT(), 15*time.Second); err != nil {
		t.Fatal(err)
	}
	return closeConn
}

func storageBackendStateHeld(t *testing.T, dd *daemon.Daemon) bool {
	t.Helper()

	after, before := int64(fixture.T0), int64(fixture.T0+6)

	identityParams := daemon.DataParams(storageBackendContext, after, before, 6)
	identityParams.Set("scope_dimensions", storageBackendDim)
	identityParams.Set("options", "jsonwrap|unaligned")
	identityDoc, err := dd.DataV3(storageBackendHost, identityParams)
	if err != nil {
		t.Fatal(err)
	}
	identityCols, err := canon.Columns(identityDoc)
	if err != nil {
		t.Fatal(err)
	}
	identityWant := []expectedColumnPoint{
		wantNumberWithPAAt(after+1, 10, 0),
		wantEmptyWithMetadataAt(after+2, 0, canon.AnnotationEmpty),
		wantNumberWithPAAt(after+3, 20, 0),
		wantNumberWithPAAt(after+4, 30, 0),
		wantEmptyWithMetadataAt(after+5, 0, canon.AnnotationEmpty),
		wantNumberWithPAAt(after+6, 40, 0),
	}
	gridOK := queryTimestampGridExact(t, identityDoc, queryExpectedVirtualGrid(t, after, before, 6, false))
	tierOK := assertTierPresence(t, identityDoc, []bool{true})
	columnOK := assertOnlyColumn(t, identityCols, storageBackendDim)
	identityOK := assertExactColumn(t, identityCols, storageBackendDim, identityWant, 0)
	stats, statsOK := strictDimensionStats(
		t, identityDoc, "db", []string{storageBackendDim}, []string{"min", "avg", "max"})
	if got := stats[storageBackendDim]; got["min"] != 10 || got["avg"] != 25 || got["max"] != 40 {
		t.Logf("db statistics = %v, want min/avg/max exactly 10/25/40", got)
		statsOK = false
	}

	sumParams := daemon.DataParams(storageBackendContext, after, before, 3)
	sumParams.Set("scope_dimensions", storageBackendDim)
	sumParams.Set("time_group", "sum")
	sumParams.Set("options", "jsonwrap|unaligned")
	sumDoc, err := dd.DataV3(storageBackendHost, sumParams)
	if err != nil {
		t.Fatal(err)
	}
	sumCols, err := canon.Columns(sumDoc)
	if err != nil {
		t.Fatal(err)
	}
	sumGridOK := queryTimestampGridExact(t, sumDoc, queryExpectedVirtualGrid(t, after, before, 3, false))
	sumTierOK := assertTierPresence(t, sumDoc, []bool{true})
	sumColumnOK := assertOnlyColumn(t, sumCols, storageBackendDim)
	sumValuesOK := assertExactColumn(t, sumCols, storageBackendDim, []expectedColumnPoint{
		wantNumberWithPAAt(after+2, 10, canon.AnnotationPartial),
		wantNumberWithPAAt(after+4, 50, 0),
		wantNumberWithPAAt(after+6, 40, canon.AnnotationPartial),
	}, 0)

	rawParams := daemon.DataParams(storageBackendContext, after, before, 6)
	rawParams.Set("scope_dimensions", storageBackendDim)
	rawParams.Set("options", "jsonwrap|unaligned|raw")
	rawDoc, err := dd.DataV3(storageBackendHost, rawParams)
	if err != nil {
		t.Fatal(err)
	}
	rawStats, rawStatsOK := strictDimensionStats(
		t, rawDoc, "db", []string{storageBackendDim}, []string{"sum", "cnt"})
	if got := rawStats[storageBackendDim]; got["sum"] != 100 || got["cnt"] != 4 {
		t.Logf("raw db statistics = %v, want sum/cnt exactly 100/4", got)
		rawStatsOK = false
	}
	rawTierOK := assertTierPresence(t, rawDoc, []bool{true})

	return gridOK && tierOK && columnOK && identityOK && statsOK &&
		sumGridOK && sumTierOK && sumColumnOK && sumValuesOK &&
		rawStatsOK && rawTierOK
}

func TestStorageBackendGapState(t *testing.T) {
	const contract = "L1/storage-backend-gap-state"

	cases := map[string]struct {
		pageType string
		memory   string
		restart  bool
		guidIdx  int
	}{
		"dbengine-gorilla-hot":     {memory: "dbengine", guidIdx: 410},
		"dbengine-gorilla-restart": {memory: "dbengine", restart: true, guidIdx: 411},
		"dbengine-raw-hot":         {pageType: "raw", memory: "dbengine", guidIdx: 412},
		"dbengine-raw-restart":     {pageType: "raw", memory: "dbengine", restart: true, guidIdx: 413},
		"ram":                      {memory: "ram", guidIdx: 414},
		"alloc":                    {memory: "alloc", guidIdx: 415},
	}
	for component, tc := range cases {
		t.Run(component, func(t *testing.T) {
			trackContractComponent(t, contract, component)
			dd := startDedicatedStorageDaemon(t, daemon.Options{
				StorageTiers: 1, DBEnginePageType: tc.pageType, StreamMemoryMode: tc.memory,
			})
			expectedFirst := int64(fixture.T0 + 1)
			if tc.memory == "ram" || tc.memory == "alloc" {
				// The legacy ring reports the interval preceding its first stored sample.
				expectedFirst = fixture.T0
			}
			closeFixture := pushStorageBackendFixture(t, dd, guid(tc.guidIdx), expectedFirst)
			if tc.restart {
				if err := closeFixture(); err != nil {
					t.Fatal(err)
				}
				if err := dd.Restart(); err != nil {
					t.Fatal(err)
				}
				if _, err := dd.WaitRetention(
					storageBackendHost, storageBackendContext, fixture.T0+1, fixture.T0+6, 15*time.Second); err != nil {
					t.Fatal(err)
				}
			}
			if !storageBackendStateHeld(t, dd) {
				t.Errorf("BROKEN %s (%s): %s", contract, component, manifest[contract].Proves)
			}
		})
	}
}

func replaceDedicatedTierGrouping(t *testing.T, dd *daemon.Daemon, from, to int) {
	t.Helper()

	path := filepath.Join(dd.Opts.RunDir, "etc", "netdata.conf")
	conf, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	oldLine := fmt.Sprintf("    dbengine tier 1 update every iterations = %d\n", from)
	newLine := fmt.Sprintf("    dbengine tier 1 update every iterations = %d\n", to)
	if matches := strings.Count(string(conf), oldLine); matches != 1 {
		t.Fatalf("tier grouping line %q occurs %d times, want exactly once", oldLine, matches)
	}
	updated := strings.Replace(string(conf), oldLine, newLine, 1)
	if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
		t.Fatal(err)
	}
}

func waitDedicatedTierLastEntry(
	t *testing.T, dd *daemon.Daemon, host, context string, tier int, last int64,
) {
	t.Helper()

	deadline := time.Now().Add(30 * time.Second)
	var seen []daemon.Retention
	for {
		params := daemon.DataParams(context, last-1, last, 1)
		params.Set("options", "jsonwrap|unaligned")
		doc, err := dd.DataV3(host, params)
		if err == nil {
			seen = perTierRetention(t, doc)
			if tier < len(seen) && seen[tier].LastEntry >= last {
				return
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("tier %d retention did not reach %d: %v", tier, last, seen)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

type historicalGroupingQuery struct {
	host, context, dimension string
	after, before, points    int64
	value                    int64
	partial                  bool
}

func historicalGroupingAnswerHeld(t *testing.T, dd *daemon.Daemon, q historicalGroupingQuery) bool {
	t.Helper()

	params := daemon.DataParamsTier(q.context, 1, q.after, q.before, q.points, "sum")
	params.Set("scope_dimensions", q.dimension)
	params.Set("options", "jsonwrap|unaligned")
	doc, err := dd.DataV3(q.host, params)
	if err != nil {
		t.Fatal(err)
	}
	cols, err := canon.Columns(doc)
	if err != nil {
		t.Fatal(err)
	}
	wantPA := int64(0)
	if q.partial {
		wantPA = canon.AnnotationPartial
	}
	step := (q.before - q.after) / q.points
	want := make([]expectedColumnPoint, 0, q.points)
	for end := q.after + step; end <= q.before; end += step {
		want = append(want, wantNumberWithPAAt(end, float64(q.value), wantPA))
	}

	return queryTimestampGridExact(t, doc, queryExpectedVirtualGrid(t, q.after, q.before, q.points, false)) &&
		assertTierPresence(t, doc, []bool{false, true}) &&
		assertOnlyColumn(t, cols, q.dimension) &&
		assertExactColumn(t, cols, q.dimension, want, 0)
}

func TestHistoricalTierGrouping(t *testing.T) {
	const (
		contract = "L2/historical-tier-grouping"
		after    = int64(fixture.T0 + 8)
		before   = int64(fixture.T0 + 24)
	)

	t.Run("complete-4-to-8", func(t *testing.T) {
		const (
			component = "complete-4-to-8"
			host      = "historical-tier-grouping-complete"
			context   = "fixture.historical_tier_grouping_complete"
			dimension = "value"
		)
		trackContractComponent(t, contract, component)
		dd := startDedicatedStorageDaemon(t, daemon.Options{
			StorageTiers: 2,
			TierGrouping: [3]int{0, 4, 0},
		})

		ch := fixture.Series(context, context, fixture.T0, 40, 1,
			func(int) string { return "1" }, notAnom)
		ch.Dimensions[0].ID = dimension
		closeFixture := pushDedicatedChart(t, dd, host, guid(420), ch)
		waitDedicatedTierLastEntry(t, dd, host, context, 1, before)
		if !historicalGroupingAnswerHeld(t, dd, historicalGroupingQuery{
			host: host, context: context, dimension: dimension,
			after: after, before: before, points: 2, value: 8,
		}) {
			t.Fatal("grouping-4 control did not produce two complete eight-second sums")
		}
		if err := closeFixture(); err != nil {
			t.Fatal(err)
		}
		if err := dd.Stop(); err != nil {
			t.Fatal(err)
		}
		replaceDedicatedTierGrouping(t, dd, 4, 8)
		if err := dd.Restart(); err != nil {
			t.Fatal(err)
		}
		waitDedicatedTierLastEntry(t, dd, host, context, 1, before)
		if !historicalGroupingAnswerHeld(t, dd, historicalGroupingQuery{
			host: host, context: context, dimension: dimension,
			after: after, before: before, points: 2, value: 8,
		}) {
			t.Errorf("BROKEN %s (%s): %s", contract, component, manifest[contract].Proves)
		}
	})

	t.Run("partial-8-to-4", func(t *testing.T) {
		const (
			component = "partial-8-to-4"
			host      = "historical-tier-grouping-partial"
			context   = "fixture.historical_tier_grouping_partial"
			dimension = "value"
		)
		trackContractComponent(t, contract, component)
		dd := startDedicatedStorageDaemon(t, daemon.Options{
			StorageTiers: 2,
			TierGrouping: [3]int{0, 8, 0},
		})

		ch := fixture.Series(context, context, fixture.T0, 40, 1,
			func(int) string { return "1" },
			func(i int) string {
				if i%2 == 0 {
					return stream.FlagEmpty
				}
				return stream.FlagNotAnomalous
			})
		ch.Dimensions[0].ID = dimension
		closeFixture := pushDedicatedChart(t, dd, host, guid(421), ch)
		waitDedicatedTierLastEntry(t, dd, host, context, 1, before)
		if !historicalGroupingAnswerHeld(t, dd, historicalGroupingQuery{
			host: host, context: context, dimension: dimension,
			after: after, before: before, points: 2, value: 4, partial: true,
		}) {
			t.Fatal("grouping-8 control did not produce two partial eight-second sums")
		}
		if err := closeFixture(); err != nil {
			t.Fatal(err)
		}
		if err := dd.Stop(); err != nil {
			t.Fatal(err)
		}
		replaceDedicatedTierGrouping(t, dd, 8, 4)
		if err := dd.Restart(); err != nil {
			t.Fatal(err)
		}
		waitDedicatedTierLastEntry(t, dd, host, context, 1, before)
		if !historicalGroupingAnswerHeld(t, dd, historicalGroupingQuery{
			host: host, context: context, dimension: dimension,
			after: after, before: before, points: 2, value: 4, partial: true,
		}) {
			t.Errorf("BROKEN %s (%s): %s", contract, component, manifest[contract].Proves)
		}
	})
}

func TestV1RollupCount65536(t *testing.T) {
	const (
		contract  = "L2/v1-rollup-count-65536"
		grouping  = int64(65_536)
		rows      = int64(1)
		rowWidth  = grouping / rows
		host      = "v1-rollup-count-65536"
		context   = "fixture.v1_rollup_count_65536"
		dimension = "value"
	)
	trackContract(t, contract)

	base := int64(fixture.T0) - int64(fixture.T0)%grouping
	dd := startDedicatedStorageDaemon(t, daemon.Options{
		StorageTiers: 2,
		TierGrouping: [3]int{0, int(grouping), 0},
	})
	// Persist two complete rollups so the legacy page can derive its cadence
	// from consecutive timestamps. This contract isolates the wrapped count;
	// a singleton legacy page has a separate, unavoidable cadence ambiguity.
	ch := fixture.Series(context, context, base, int(2*grouping+1), 1,
		func(int) string { return "1" }, notAnom)
	ch.Dimensions[0].ID = dimension
	closeFixture := pushDedicatedChart(t, dd, host, guid(422), ch)
	if _, err := dd.WaitRetention(host, context, base+1, base+2*grouping+1, 60*time.Second); err != nil {
		t.Fatal(err)
	}
	if err := closeFixture(); err != nil {
		t.Fatal(err)
	}
	if err := dd.Restart(); err != nil {
		t.Fatal(err)
	}

	params := daemon.DataParamsTier(context, 1, base+grouping, base+2*grouping, rows, "average")
	params.Set("scope_dimensions", dimension)
	params.Set("options", "jsonwrap|unaligned")
	doc, err := dd.DataV3(host, params)
	if err != nil {
		t.Fatal(err)
	}
	cols, err := canon.Columns(doc)
	if err != nil {
		t.Fatal(err)
	}
	want := make([]expectedColumnPoint, 0, rows)
	for end := base + grouping + rowWidth; end <= base+2*grouping; end += rowWidth {
		want = append(want, wantNumberWithPAAt(end, 1, 0))
	}
	stats, statsOK := strictDimensionStats(t, doc, "db", []string{dimension}, []string{"min", "avg", "max"})
	if got := stats[dimension]; got["min"] != 1 || got["avg"] != 1 || got["max"] != 1 {
		t.Logf("db statistics = %v, want min/avg/max exactly 1/1/1", got)
		statsOK = false
	}
	held := queryTimestampGridExact(t, doc, queryExpectedVirtualGrid(t, base+grouping, base+2*grouping, rows, false)) &&
		assertTierPresence(t, doc, []bool{false, true}) &&
		assertOnlyColumn(t, cols, dimension) &&
		assertExactColumn(t, cols, dimension, want, 0) &&
		statsOK
	assertContract(t, contract, held)
}
