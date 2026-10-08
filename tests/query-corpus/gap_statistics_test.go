// SPDX-License-Identifier: GPL-3.0-or-later

package corpus

import (
	"testing"
	"time"

	"github.com/netdata/netdata/tests/query-corpus/canon"
	"github.com/netdata/netdata/tests/query-corpus/daemon"
	"github.com/netdata/netdata/tests/query-corpus/fixture"
	"github.com/netdata/netdata/tests/query-corpus/stream"
)

func TestGapOnlyDBAverageIsNull(t *testing.T) {
	trackContract(t, "L5/gap-only-db-average-is-null")

	const (
		context = "fixture.gap_statistics"
		host    = "gap-statistics"
	)
	ch := fixture.Chart{
		ID: context, Title: "gap statistics", Units: "units",
		Family: "fixture", Context: context, UpdateEvery: 1,
		Dimensions: []fixture.Dimension{{ID: "gap"}, {ID: "control"}},
	}
	for i := int64(1); i <= 11; i++ {
		gap := fixture.Point{T: fixture.T0 + i, Flags: stream.FlagEmpty}
		if i == 1 {
			gap.Collected = "1"
			gap.Flags = stream.FlagNotAnomalous
		}
		ch.Dimensions[0].Points = append(ch.Dimensions[0].Points, gap)
		ch.Dimensions[1].Points = append(ch.Dimensions[1].Points, fixture.Point{
			T: fixture.T0 + i, Collected: "1", Flags: stream.FlagNotAnomalous,
		})
	}

	pushLiveBurst(t, host, guid(390), ch)
	if _, err := td.WaitRetention(host, context, ch.FirstT(), ch.LastT(), 15*time.Second); err != nil {
		t.Fatal(err)
	}

	after, before := int64(fixture.T0+1), int64(fixture.T0+11)
	params := daemon.DataParams(context, after, before, before-after)
	params.Set("scope_dimensions", "gap")
	params.Set("options", "jsonwrap|unaligned")
	doc, err := td.DataV3(host, params)
	if err != nil {
		t.Fatal(err)
	}
	if !assertExactView(t, doc, after, before, 1) {
		t.Fail()
	}
	cols, err := canon.Columns(doc)
	if err != nil {
		t.Fatal(err)
	}
	if !assertExactColumnSet(t, cols, []string{"gap"}) {
		t.Fail()
	}
	want := make([]expectedColumnPoint, 0, before-after)
	for ts := after + 1; ts <= before; ts++ {
		want = append(want, wantEmptyWithMetadataAt(ts, 0, canon.AnnotationEmpty))
	}
	if !assertExactColumn(t, cols, "gap", want, 0) {
		t.Fail()
	}

	db := queryObject(t, doc, "db", "db")
	dimensions := queryObject(t, db, "dimensions", "db.dimensions")
	ids, ok := dimensions["ids"].([]any)
	if !ok || len(ids) != 1 || ids[0] != "gap" {
		t.Fatalf("db.dimensions.ids = %v, want exactly [gap]", dimensions["ids"])
	}
	statistics := queryObject(t, dimensions, "sts", "db.dimensions.sts")
	for _, field := range []string{"min", "avg", "max"} {
		values, ok := statistics[field].([]any)
		if !ok || len(values) != 1 || values[0] != nil {
			t.Errorf("db.dimensions.sts.%s = %v, want exactly [null] for a gap-only query",
				field, statistics[field])
		}
	}
}
