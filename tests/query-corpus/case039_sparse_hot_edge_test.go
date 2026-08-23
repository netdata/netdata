// SPDX-License-Identifier: GPL-3.0-or-later

// CASE-039 — active gauges collected less often than the requested hot-edge
// window. Certificate expiry, cloud inventory and capacity metrics may update
// hourly or daily, while a dashboard commonly asks for the last minute or the
// last 15 minutes. The newest sample can therefore predate the requested
// window even though the next collection is not due yet.
//
// Keep the table and chart contracts independent. A one-point LATEST request
// has an explicit last-known-value meaning; an ordinary AVERAGE time series
// follows the general bucket/retention path. If only one fails, one broken
// contract must not hide the other result.
package corpus

import (
	"fmt"
	"testing"
	"time"

	"github.com/netdata/netdata/tests/query-corpus/canon"
	"github.com/netdata/netdata/tests/query-corpus/daemon"
	"github.com/netdata/netdata/tests/query-corpus/fixture"
	"github.com/netdata/netdata/tests/query-corpus/stream"
)

const (
	c039LatestContract = "CASE-039/sparse-active-latest-table"
	c039ChartContract  = "CASE-039/sparse-active-chart"
)

func c039ExpectedValue(t *testing.T, doc map[string]any, dimension string, want float64) (int, bool) {
	t.Helper()

	cols, err := canon.Columns(doc)
	if err != nil {
		t.Logf("decode columns: %v", err)
		return 0, false
	}
	column, found := cols[dimension]
	if !found {
		t.Logf("response has columns %v, want %q", keys(cols), dimension)
		return 0, false
	}

	numeric := 0
	ok := true
	for _, point := range column {
		if point.Value == nil {
			continue
		}
		numeric++
		if *point.Value != want {
			t.Logf("point at %d = %v, want latest value %v", point.T, *point.Value, want)
			ok = false
		}
	}
	return numeric, ok
}

func c039LogEmptyResponse(t *testing.T, label string, doc map[string]any) {
	t.Helper()

	retention, _ := daemon.QueryRetention(doc)
	view, _ := doc["view"].(map[string]any)
	tiers, _ := strictTierPoints(t, doc)
	t.Logf("%s returned no numeric value: retention=[%d,%d] view.after=%v view.before=%v "+
		"view.update_every=%v tier-points=%v",
		label, retention.FirstEntry, retention.LastEntry,
		view["after"], view["before"], view["update_every"], tiers)
}

func TestCase039SparseActiveMetricAtHotEdge(t *testing.T) {
	registerContract(t, c039LatestContract)
	registerContract(t, c039ChartContract)

	cases := []struct {
		name        string
		updateEvery int64
		age         int64
		machineGUID int
		value       float64
	}{
		{name: "ue60", updateEvery: 60, age: 5, machineGUID: 430, value: 42},
		{name: "ue600", updateEvery: 600, age: 300, machineGUID: 431, value: 43},
		{name: "ue3600", updateEvery: 3600, age: 1800, machineGUID: 432, value: 44},
		{name: "ue86400", updateEvery: 86400, age: 43200, machineGUID: 433, value: 45},
	}
	spans := []int64{60, 15 * 60}

	latestOK := true
	chartOK := true
	for _, tc := range cases {
		queryBefore := time.Now().Unix() - 1
		last := queryBefore - tc.age
		first := last - tc.updateEvery
		context := "fixture.c039_sparse_hot_edge." + tc.name
		host := "c039-" + tc.name
		ch := fixture.Chart{
			ID: context, Title: "sparse active gauge", Units: "units", Family: "fixture",
			Context: context, UpdateEvery: int(tc.updateEvery),
			Dimensions: []fixture.Dimension{{
				ID: "value", Algorithm: "absolute",
				Points: []fixture.Point{
					{T: first, Collected: fmt.Sprintf("%.0f", tc.value), Flags: stream.FlagNotAnomalous},
					{T: last, Collected: fmt.Sprintf("%.0f", tc.value), Flags: stream.FlagNotAnomalous},
				},
			}},
		}

		pushLiveBurst(t, host, guid(tc.machineGUID), ch)
		if _, err := td.WaitRetention(host, context, first, last, 20*time.Second); err != nil {
			t.Fatal(err)
		}
		if !(last < queryBefore && queryBefore < last+tc.updateEvery) {
			t.Fatalf("fixture is not between collections: last=%d query-before=%d next=%d",
				last, queryBefore, last+tc.updateEvery)
		}

		for _, span := range spans {
			after := queryBefore - span
			label := fmt.Sprintf("update_every=%ds age=%ds window=%ds", tc.updateEvery, tc.age, span)

			latestParams := daemon.DataParams(context, after, queryBefore, 1)
			latestParams.Set("time_group", "latest")
			latestParams.Set("options", "jsonwrap|unaligned|virtual-points")
			latestDoc, err := td.DataV3(host, latestParams)
			if err != nil {
				t.Fatal(err)
			}
			latestValues, valuesOK := c039ExpectedValue(t, latestDoc, "value", tc.value)
			if latestValues != 1 || !valuesOK {
				if latestValues == 0 {
					c039LogEmptyResponse(t, "latest "+label, latestDoc)
				} else {
					t.Logf("latest %s returned %d numeric rows, want exactly one", label, latestValues)
				}
				latestOK = false
			}
			if !queryTimestampGridExact(
				t, latestDoc, queryExpectedVirtualGrid(t, after, queryBefore, 1, false)) {
				t.Logf("latest %s changed the requested timestamp grid", label)
				latestOK = false
			}
			if !assertTierPresence(t, latestDoc, []bool{false, false, false}) {
				t.Logf("latest %s did not use the collector-cache fast path", label)
				latestOK = false
			}

			chartParams := daemon.DataParams(context, after, queryBefore, 60)
			chartParams.Set("options", "jsonwrap|unaligned|virtual-points")
			chartDoc, err := td.DataV3(host, chartParams)
			if err != nil {
				t.Fatal(err)
			}
			chartValues, valuesOK := c039ExpectedValue(t, chartDoc, "value", tc.value)
			if chartValues == 0 || !valuesOK {
				if chartValues == 0 {
					c039LogEmptyResponse(t, "average "+label, chartDoc)
				}
				chartOK = false
			}
		}
	}

	// The latest collection interval is inclusive at its endpoint and finite:
	// db_last + update_every == after is eligible; one second later is not.
	queryBefore := time.Now().Unix() - 2
	updateEvery := int64(600)
	last := queryBefore - 60 - updateEvery
	first := last - updateEvery
	context := "fixture.c039_sparse_hot_edge.boundary"
	host := "c039-boundary"
	ch := fixture.Chart{
		ID: context, Title: "sparse active gauge boundary", Units: "units", Family: "fixture",
		Context: context, UpdateEvery: int(updateEvery),
		Dimensions: []fixture.Dimension{{
			ID: "value", Algorithm: "absolute",
			Points: []fixture.Point{
				{T: first, Collected: "46", Flags: stream.FlagNotAnomalous},
				{T: last, Collected: "46", Flags: stream.FlagNotAnomalous},
			},
		}},
	}
	pushLiveBurst(t, host, guid(434), ch)
	if _, err := td.WaitRetention(host, context, first, last, 20*time.Second); err != nil {
		t.Fatal(err)
	}

	equalAfter := last + updateEvery
	equalBefore := queryBefore
	equalParams := daemon.DataParams(context, equalAfter, equalBefore, 1)
	equalParams.Set("time_group", "latest")
	equalParams.Set("options", "jsonwrap|unaligned|virtual-points")
	equalDoc, err := td.DataV3(host, equalParams)
	if err != nil {
		t.Fatal(err)
	}
	equalValues, valuesOK := c039ExpectedValue(t, equalDoc, "value", 46)
	if equalValues != 1 || !valuesOK {
		c039LogEmptyResponse(t, "latest inclusive interval boundary", equalDoc)
		latestOK = false
	}
	if !queryTimestampGridExact(
		t, equalDoc, queryExpectedVirtualGrid(t, equalAfter, equalBefore, 1, false)) {
		t.Logf("latest inclusive interval boundary changed the requested timestamp grid")
		latestOK = false
	}
	if !assertTierPresence(t, equalDoc, []bool{false, false, false}) {
		t.Logf("latest inclusive interval boundary did not use the collector-cache fast path")
		latestOK = false
	}

	expiredAfter := equalAfter + 1
	expiredBefore := equalBefore + 1
	expiredParams := daemon.DataParams(context, expiredAfter, expiredBefore, 1)
	expiredParams.Set("time_group", "latest")
	expiredParams.Set("options", "jsonwrap|unaligned|virtual-points")
	expiredDoc, err := td.DataV3(host, expiredParams)
	if err != nil {
		t.Fatal(err)
	}
	if !canon.EmptyResult(expiredDoc) {
		t.Logf("latest expired interval did not return the canonical empty result")
		latestOK = false
	}
	if !assertTierPresence(t, expiredDoc, []bool{false, false, false}) {
		t.Logf("latest expired interval unexpectedly read storage tiers")
		latestOK = false
	}

	assertContract(t, c039LatestContract, latestOK)
	assertContract(t, c039ChartContract, chartOK)
}
