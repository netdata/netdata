// SPDX-License-Identifier: GPL-3.0-or-later

package corpus

import (
	"fmt"
	"html"
	"io"
	"math"
	"mime"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/tests/query-corpus/canon"
	"github.com/netdata/netdata/tests/query-corpus/daemon"
	"github.com/netdata/netdata/tests/query-corpus/fixture"
	"github.com/netdata/netdata/tests/query-corpus/stream"
)

const c020Samples = 4

var c020BadgeValueRE = regexp.MustCompile(`<text class="bdge-lbl-val"[^>]*>([^<]*)</text>`)

type c020QueryCase struct {
	name        string
	host        string
	context     string
	dimension   string
	sourceUnits string
	storedValue float64
	rate        bool
}

type c020MixedObservation struct {
	ViewUnits      string
	DimensionUnits string
	ResultLabel    string
}

func c020Chart(id, context, units, algorithm string, value int, first int64) fixture.Chart {
	ch := fixture.Chart{
		ID: id, Title: "CASE-020 units", Units: units, Family: "fixture",
		Context: context, UpdateEvery: 1,
		Dimensions: []fixture.Dimension{{ID: "value", Algorithm: algorithm}},
	}
	for i := 1; i <= c020Samples; i++ {
		ch.Dimensions[0].Points = append(ch.Dimensions[0].Points, fixture.Point{
			T: first + int64(i), Collected: strconv.Itoa(value), Flags: stream.FlagNotAnomalous,
		})
	}
	return ch
}

func c020PushCharts(t *testing.T, host, machineGUID string, charts ...fixture.Chart) {
	t.Helper()

	conn := connect(t, host, machineGUID, stream.CapsLive)
	for _, ch := range charts {
		ch.Define(conn)
		ch.PushLive(conn)
	}
	if err := conn.Flush(); err != nil {
		t.Fatal(err)
	}

	seen := make(map[string]bool)
	for _, ch := range charts {
		if seen[ch.Context] {
			continue
		}
		seen[ch.Context] = true
		if _, err := td.WaitRetention(host, ch.Context, ch.FirstT(), ch.LastT(), 15*time.Second); err != nil {
			t.Fatal(err)
		}
	}
}

func c020OnlyColumn(t *testing.T, doc map[string]any, timestamp int64) (string, canon.Pt) {
	t.Helper()

	cols, err := canon.Columns(doc)
	if err != nil {
		t.Fatal(err)
	}
	if len(cols) != 1 {
		t.Fatalf("result has %d columns, want exactly one: %v", len(cols), keys(cols))
	}
	for label, points := range cols {
		if label == "" {
			t.Fatal("result column label is empty")
		}
		if len(points) != 1 {
			t.Fatalf("result column %q has %d rows, want exactly one", label, len(points))
		}
		if points[0].Value == nil {
			t.Fatalf("result column %q is null, want a number", label)
		}
		if points[0].T != timestamp {
			t.Fatalf("result column %q ends at %d, want %d", label, points[0].T, timestamp)
		}
		if points[0].PA&canon.AnnotationEmpty != 0 {
			t.Fatalf("result column %q is numeric but marked EMPTY", label)
		}
		if math.IsNaN(*points[0].Value) || math.IsInf(*points[0].Value, 0) {
			t.Fatalf("result column %q is non-finite: %v", label, *points[0].Value)
		}
		return label, points[0]
	}
	panic("unreachable")
}

func c020Query(t *testing.T, host, context, timeGroup, groupBy, aggregation string, after, before int64) map[string]any {
	t.Helper()

	params := daemon.DataParams(context, after, before, 1)
	params.Set("time_group", timeGroup)
	params.Set("group_by", groupBy)
	params.Set("aggregation", aggregation)
	params.Set("options", "jsonwrap,unaligned")
	doc, err := td.DataV3(host, params)
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func TestCase020UnitsAcrossQuerySurfaces(t *testing.T) {
	trackContract(t, "CASE-020/units-across-query-surfaces")

	const (
		host          = "c020-units"
		rateContext   = "fixture.c020_units_rate"
		gaugeContext  = "fixture.c020_units_gauge"
		rateChartID   = "fixture.c020_units_rate"
		gaugeChartID  = "fixture.c020_units_gauge"
		fixtureFirst  = fixture.T0
		fixtureBefore = fixture.T0 + c020Samples
	)

	rateChart := c020Chart(rateChartID, rateContext, "requests/s", "incremental", 5, fixtureFirst)
	gaugeChart := c020Chart(gaugeChartID, gaugeContext, "requests", "absolute", 7, fixtureFirst)
	c020PushCharts(t, host, guid(330), rateChart, gaugeChart)

	metrics := []c020QueryCase{
		{
			name: "rate", host: host, context: rateContext, dimension: "value",
			sourceUnits: "requests/s", storedValue: 5, rate: true,
		},
		{
			name: "gauge", host: host, context: gaugeContext, dimension: "value",
			sourceUnits: "requests", storedValue: 7,
		},
	}
	timeGroups := []string{"average", "sum"}
	groupBys := []string{"dimension", "units", "dimension,units"}

	for _, metric := range metrics {
		for _, timeGroup := range timeGroups {
			for _, groupBy := range groupBys {
				name := metric.name + "/" + timeGroup + "/" + strings.ReplaceAll(groupBy, ",", "+")
				t.Run(name, func(t *testing.T) {
					resultUnits := metric.sourceUnits
					wantValue := metric.storedValue
					if timeGroup == "sum" {
						wantValue *= c020Samples
						if metric.rate {
							resultUnits = "requests"
						}
					}

					wantLabel := metric.dimension
					switch groupBy {
					case "units":
						wantLabel = resultUnits
					case "dimension,units":
						wantLabel = metric.dimension + "," + resultUnits
					}

					doc := c020Query(
						t, metric.host, metric.context, timeGroup, groupBy, "average",
						fixtureFirst, fixtureBefore,
					)
					db := queryObject(t, doc, "db", "db")
					view := queryObject(t, doc, "view", "view")

					if got := queryStrictOneUnit(t, db["units"], "db.units"); got != metric.sourceUnits {
						t.Errorf("db.units = %q, want stored units %q", got, metric.sourceUnits)
					}
					if got := queryStrictDimensionUnit(t, db, "db"); got != metric.sourceUnits {
						t.Errorf("db.dimensions.units[0] = %q, want stored units %q", got, metric.sourceUnits)
					}
					if got := queryStrictOneUnit(t, view["units"], "view.units"); got != resultUnits {
						t.Errorf("view.units = %q, want result units %q", got, resultUnits)
					}
					if got := queryStrictDimensionUnit(t, view, "view"); got != resultUnits {
						t.Errorf("view.dimensions.units[0] = %q, want result units %q", got, resultUnits)
					}

					label, point := c020OnlyColumn(t, doc, fixtureBefore)
					if label != wantLabel {
						t.Errorf("result label = %q, want %q", label, wantLabel)
					}
					if got := *point.Value; got != wantValue {
						t.Errorf("result value = %v, want exact fixture value %v", got, wantValue)
					}
				})
			}
		}
	}
}

func c020PushMixedOrder(t *testing.T, host, machineGUID, context string, rateFirst bool) {
	t.Helper()

	rateID := context + "_a"
	gaugeID := context + "_z"
	if !rateFirst {
		rateID, gaugeID = gaugeID, rateID
	}
	rate := c020Chart(rateID, context, "requests/s", "incremental", 5, fixture.T0)
	gauge := c020Chart(gaugeID, context, "requests", "absolute", 7, fixture.T0)
	if rateFirst {
		c020PushCharts(t, host, machineGUID, rate, gauge)
	} else {
		c020PushCharts(t, host, machineGUID, gauge, rate)
	}
}

func c020MixedResult(t *testing.T, host, context, groupBy string) c020MixedObservation {
	t.Helper()

	doc := c020Query(
		t, host, context, "sum", groupBy, "sum",
		fixture.T0, fixture.T0+c020Samples,
	)
	view := queryObject(t, doc, "view", "view")
	dimensions := queryObject(t, view, "dimensions", "view.dimensions")
	aggregated, ok := dimensions["aggregated"].([]any)
	if !ok || len(aggregated) != 1 || aggregated[0] != float64(2) {
		t.Fatalf("view.dimensions.aggregated = %v, want exactly [2] for the rate+gauge collision", dimensions["aggregated"])
	}
	label, point := c020OnlyColumn(t, doc, fixture.T0+c020Samples)
	if got, want := *point.Value, float64((5+7)*c020Samples); got != want {
		t.Errorf("mixed result value = %v, want exact combined volume %v", got, want)
	}
	return c020MixedObservation{
		ViewUnits:      queryStrictOneUnit(t, view["units"], "view.units"),
		DimensionUnits: queryStrictOneUnit(t, dimensions["units"], "view.dimensions.units"),
		ResultLabel:    label,
	}
}

func TestCase020MixedRateGaugeUnitsAreOrderIndependent(t *testing.T) {
	trackContract(t, "CASE-020/mixed-rate-gauge-units")

	const (
		rateFirstHost     = "c020-mixed-rate-first"
		gaugeFirstHost    = "c020-mixed-gauge-first"
		rateFirstContext  = "fixture.c020_mixed_rate_first"
		gaugeFirstContext = "fixture.c020_mixed_gauge_first"
	)

	c020PushMixedOrder(t, rateFirstHost, guid(331), rateFirstContext, true)
	c020PushMixedOrder(t, gaugeFirstHost, guid(332), gaugeFirstContext, false)

	for _, groupBy := range []string{"units", "dimension,units"} {
		t.Run(strings.ReplaceAll(groupBy, ",", "+"), func(t *testing.T) {
			rateFirst := c020MixedResult(t, rateFirstHost, rateFirstContext, groupBy)
			gaugeFirst := c020MixedResult(t, gaugeFirstHost, gaugeFirstContext, groupBy)
			wantLabel := "requests"
			if groupBy == "dimension,units" {
				wantLabel = "value,requests"
			}
			for name, observation := range map[string]c020MixedObservation{
				"rate-first":  rateFirst,
				"gauge-first": gaugeFirst,
			} {
				if observation.ViewUnits != "requests" {
					t.Errorf("%s view units = %q, want exact volume units %q", name, observation.ViewUnits, "requests")
				}
				if observation.DimensionUnits != "requests" {
					t.Errorf(
						"%s dimension units = %q, want exact volume units %q",
						name, observation.DimensionUnits, "requests",
					)
				}
				if observation.ResultLabel != wantLabel {
					t.Errorf("%s result label = %q, want %q", name, observation.ResultLabel, wantLabel)
				}
			}
			if rateFirst != gaugeFirst {
				t.Errorf(
					"mixed rate+gauge sum depends on encounter order for group_by=%s:\nrate-first:  %+v\ngauge-first: %+v",
					groupBy, rateFirst, gaugeFirst,
				)
			}
		})
	}
}

func c020Badge(
	t *testing.T,
	baseURL, host, chart, dimension, group string,
	after, before int64,
) string {
	t.Helper()

	params := url.Values{
		"chart":           {chart},
		"dimensions":      {dimension},
		"after":           {strconv.FormatInt(after, 10)},
		"before":          {strconv.FormatInt(before, 10)},
		"points":          {"1"},
		"group":           {group},
		"options":         {"unaligned"},
		"label":           {"CASE-020"},
		"precision":       {"0"},
		"fixed_width_lbl": {"80"},
		"fixed_width_val": {"120"},
	}
	endpoint := fmt.Sprintf(
		"%s/host/%s/api/v1/badge.svg?%s",
		baseURL, url.PathEscape(host), params.Encode(),
	)
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("badge HTTP status = %d, want 200; body=%q", resp.StatusCode, body)
	}
	if len(body) == 0 {
		t.Fatal("badge returned an empty body")
	}
	mediaType, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil {
		t.Fatalf("badge Content-Type is invalid: %q: %v", resp.Header.Get("Content-Type"), err)
	}
	if mediaType != "image/svg+xml" {
		t.Fatalf("badge Content-Type = %q, want image/svg+xml", mediaType)
	}
	if !strings.HasPrefix(string(body), "<svg ") || !strings.HasSuffix(string(body), "</svg>") {
		t.Fatalf("badge body is not a complete SVG: %q", body)
	}
	return string(body)
}

func c020AssertBadgeValue(t *testing.T, svg, want string) {
	t.Helper()

	matches := c020BadgeValueRE.FindAllStringSubmatch(svg, -1)
	if len(matches) == 0 {
		t.Fatalf("badge has no rendered value node: %q", svg)
	}
	for i, match := range matches {
		got := html.UnescapeString(match[1])
		if got == "" {
			t.Fatalf("badge value node %d is empty", i)
		}
		if got != want {
			t.Errorf("badge value node %d = %q, want exact %q", i, got, want)
		}
	}
}

func TestCase020BadgeUnitsAndValues(t *testing.T) {
	trackContract(t, "CASE-020/badge-sum-units")

	const (
		host         = "c020-badge"
		rateChartID  = "fixture.c020_badge_rate"
		gaugeChartID = "fixture.c020_badge_gauge"
	)
	first := time.Now().Unix() - c020Samples
	before := first + c020Samples
	rate := c020Chart(rateChartID, rateChartID, "requests/s", "incremental", 5, first)
	gauge := c020Chart(gaugeChartID, gaugeChartID, "requests", "absolute", 7, first)
	c020PushCharts(t, host, guid(333), rate, gauge)

	cases := []struct {
		name, chart, group, want string
	}{
		{name: "rate/average", chart: rateChartID, group: "average", want: "5 requests/s"},
		{name: "rate/sum", chart: rateChartID, group: "sum", want: "20 requests"},
		{name: "gauge/average", chart: gaugeChartID, group: "average", want: "7 requests"},
		{name: "gauge/sum", chart: gaugeChartID, group: "sum", want: "28 requests"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svg := c020Badge(t, td.BaseURL, host, tc.chart, "value", tc.group, first, before)
			c020AssertBadgeValue(t, svg, tc.want)
		})
	}
}

// TestCase020BadgeArchivedFilteredRateUnits separates the metric set the
// badge pre-scan can see from the metric set its query can read:
//
//  1. store a rate, then restart so it exists only as archived metadata;
//  2. recreate the same chart with a live gauge, keeping the badge fresh;
//  3. explicitly select the archived rate over its old retention window.
//
// The real query walks the instance's RRDMETRICs and selects archived_rate.
// The badge pre-scan walks only the live chart's RRDDIMs, where only
// live_gauge exists. The value proves which metric the query actually read;
// its label must use that same metric set and report the integrated volume.
func TestCase020BadgeArchivedFilteredRateUnits(t *testing.T) {
	trackContract(t, "CASE-020/badge-filtered-metric-units")

	const (
		host     = "c020-badge-archived"
		chart    = "fixture.c020_badge_archived"
		rateDim  = "archived_rate"
		gaugeDim = "live_gauge"
	)
	machineGUID := guid(335)

	d, err := daemon.Start(daemon.Options{
		Binary: netdataBinary,
		RunDir: t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Stop() })

	rate := c020Chart(chart, chart, "requests/s", "incremental", 5, fixture.T0)
	rate.Dimensions[0].ID = rateDim
	first, last := rate.FirstT(), rate.LastT()

	rateConn, err := stream.Connect(
		d.Addr, daemon.StreamKey,
		stream.HostInfo{Hostname: host, MachineGUID: machineGUID},
		stream.CapsLive,
	)
	if err != nil {
		t.Fatal(err)
	}
	rate.Define(rateConn)
	rate.PushLive(rateConn)
	if err := rateConn.Flush(); err != nil {
		_ = rateConn.Close()
		t.Fatal(err)
	}
	if _, err := d.WaitRetention(host, chart, first, last, 15*time.Second); err != nil {
		_ = rateConn.Close()
		t.Fatal(err)
	}
	if err := rateConn.Close(); err != nil {
		t.Fatal(err)
	}

	if err := d.Restart(); err != nil {
		t.Fatal(err)
	}

	liveFirst := time.Now().Unix() - c020Samples
	gauge := c020Chart(chart, chart, "requests/s", "absolute", 7, liveFirst)
	gauge.Dimensions[0].ID = gaugeDim
	liveLast := gauge.LastT()

	gaugeConn, err := stream.Connect(
		d.Addr, daemon.StreamKey,
		stream.HostInfo{Hostname: host, MachineGUID: machineGUID},
		stream.CapsLive,
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = gaugeConn.Close() })
	gauge.Define(gaugeConn)
	gauge.PushLive(gaugeConn)
	if err := gaugeConn.Flush(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(15 * time.Second)
	for {
		doc, queryErr := d.DataV3(host, daemon.DataParams(chart, liveFirst, liveLast, c020Samples))
		if queryErr == nil {
			if retention, found := daemon.QueryRetention(doc); found &&
				retention.LastEntry == liveLast {
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("archived/live badge fixture did not settle over its recent %ds probe", c020Samples)
		}
		time.Sleep(200 * time.Millisecond)
	}

	svg := c020Badge(t, d.BaseURL, host, chart, rateDim, "sum", first-1, last)
	c020AssertBadgeValue(t, svg, "20 requests")
}
