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

func c020OneColumn(t *testing.T, doc map[string]any) (string, []canon.Pt) {
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
		return label, points
	}
	panic("unreachable")
}

func c020OnlyColumn(t *testing.T, doc map[string]any, timestamp int64) (string, canon.Pt) {
	t.Helper()

	label, points := c020OneColumn(t, doc)
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

type c020QuerySpec struct {
	host        string
	context     string
	timeGroup   string
	groupBy     string
	aggregation string
	after       int64
	before      int64
}

func c020Query(t *testing.T, spec c020QuerySpec) map[string]any {
	t.Helper()

	params := daemon.DataParams(spec.context, spec.after, spec.before, 1)
	params.Set("time_group", spec.timeGroup)
	params.Set("group_by", spec.groupBy)
	params.Set("aggregation", spec.aggregation)
	params.Set("options", "jsonwrap,unaligned")
	doc, err := td.DataV3(spec.host, params)
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func TestCase020UnitsAcrossQuerySurfaces(t *testing.T) {
	for _, contract := range []string{
		"CASE-020/units-across-query-surfaces",
		"CASE-020/query-surface-values",
	} {
		registerContract(t, contract)
	}

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

	run := func(t *testing.T, checkValues bool) {
		t.Helper()
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

						doc := c020Query(t, c020QuerySpec{
							host: metric.host, context: metric.context,
							timeGroup: timeGroup, groupBy: groupBy, aggregation: "average",
							after: fixtureFirst, before: fixtureBefore,
						})
						if checkValues {
							_, point := c020OnlyColumn(t, doc, fixtureBefore)
							if got := *point.Value; got != wantValue {
								t.Errorf("result value = %v, want exact fixture value %v", got, wantValue)
							}
							return
						}

						label, _ := c020OneColumn(t, doc)
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
						if label != wantLabel {
							t.Errorf("result label = %q, want %q", label, wantLabel)
						}
					})
				}
			}
		}
	}

	t.Run("units", func(t *testing.T) {
		trackContract(t, "CASE-020/units-across-query-surfaces")
		run(t, false)
	})
	t.Run("values", func(t *testing.T) {
		trackContract(t, "CASE-020/query-surface-values")
		run(t, true)
	})
}

type c020BadgeSpec struct {
	baseURL   string
	host      string
	chart     string
	dimension string
	group     string
	units     string
	after     int64
	before    int64
}

func c020Badge(t *testing.T, spec c020BadgeSpec) string {
	t.Helper()

	params := url.Values{
		"chart":           {spec.chart},
		"dimensions":      {spec.dimension},
		"after":           {strconv.FormatInt(spec.after, 10)},
		"before":          {strconv.FormatInt(spec.before, 10)},
		"points":          {"1"},
		"group":           {spec.group},
		"options":         {"unaligned"},
		"label":           {"CASE-020"},
		"precision":       {"0"},
		"fixed_width_lbl": {"80"},
		"fixed_width_val": {"120"},
	}
	if spec.units != "" {
		params.Set("units", spec.units)
	}
	endpoint := fmt.Sprintf(
		"%s/host/%s/api/v1/badge.svg?%s",
		spec.baseURL, url.PathEscape(spec.host), params.Encode(),
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

func c020BadgeRenderedValue(t *testing.T, svg string) string {
	t.Helper()

	matches := c020BadgeValueRE.FindAllStringSubmatch(svg, -1)
	if len(matches) == 0 {
		t.Fatalf("badge has no rendered value node: %q", svg)
	}
	var rendered string
	for i, match := range matches {
		got := html.UnescapeString(match[1])
		if got == "" {
			t.Fatalf("badge value node %d is empty", i)
		}
		if i == 0 {
			rendered = got
		} else if got != rendered {
			t.Fatalf("badge value nodes disagree: node 0 = %q, node %d = %q", rendered, i, got)
		}
	}
	return rendered
}

func c020AssertBadgeNumber(t *testing.T, svg string, want float64) {
	t.Helper()

	rendered := c020BadgeRenderedValue(t, svg)
	number, _, _ := strings.Cut(rendered, " ")
	got, err := strconv.ParseFloat(number, 64)
	if err != nil {
		t.Fatalf("badge rendered value %q has no numeric prefix: %v", rendered, err)
	}
	if got != want {
		t.Errorf("badge number = %v, want exact fixture value %v (rendered %q)", got, want, rendered)
	}
}

func c020AssertBadgeUnits(t *testing.T, svg, want string) {
	t.Helper()

	rendered := c020BadgeRenderedValue(t, svg)
	_, got, found := strings.Cut(rendered, " ")
	if !found {
		got = ""
	}
	if got != want {
		t.Errorf("badge units = %q, want exact %q (rendered %q)", got, want, rendered)
	}
}

func TestCase020BadgeSumValues(t *testing.T) {

	const (
		host         = "c020-badge-values"
		rateChartID  = "fixture.c020_badge_value_rate"
		gaugeChartID = "fixture.c020_badge_value_gauge"
	)
	now := time.Now().Unix()
	first := now - 30
	before := first + c020Samples
	rate := c020Chart(rateChartID, rateChartID, "requests/s", "incremental", 5, first)
	gauge := c020Chart(gaugeChartID, gaugeChartID, "requests", "absolute", 7, first)
	for i, value := range []int{2, 3, 5, 7} {
		rate.Dimensions[0].Points[i].Collected = strconv.Itoa(value)
	}
	for i, value := range []int{11, 13, 17, 19} {
		gauge.Dimensions[0].Points[i].Collected = strconv.Itoa(value)
	}
	rate.Dimensions[0].Points = append(rate.Dimensions[0].Points, fixture.Point{
		T: now, Collected: "101", Flags: stream.FlagNotAnomalous,
	})
	gauge.Dimensions[0].Points = append(gauge.Dimensions[0].Points, fixture.Point{
		T: now, Collected: "103", Flags: stream.FlagNotAnomalous,
	})
	c020PushCharts(t, host, guid(333), rate, gauge)

	cases := []struct {
		name, contract, chart string
		want                  float64
	}{
		{name: "rate", contract: "CASE-020/badge-rate-sum-value", chart: rateChartID, want: 17},
		{name: "gauge", contract: "CASE-020/badge-gauge-sum-value", chart: gaugeChartID, want: 60},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			trackContract(t, tc.contract)
			svg := c020Badge(t, c020BadgeSpec{
				baseURL: td.BaseURL, host: host, chart: tc.chart,
				dimension: "value", group: "sum", units: "fixture-unit",
				after: first, before: before,
			})
			c020AssertBadgeNumber(t, svg, tc.want)
		})
	}
}

func TestCase020BadgeSumUnits(t *testing.T) {
	const (
		host         = "c020-badge-units"
		rateChartID  = "fixture.c020_badge_units_rate"
		gaugeChartID = "fixture.c020_badge_units_gauge"
		mixedChartID = "fixture.c020_badge_units_mixed"
	)
	now := time.Now().Unix()
	first := now - 30
	before := first + c020Samples
	rate := c020Chart(rateChartID, rateChartID, "requests/s", "incremental", 5, first)
	gauge := c020Chart(gaugeChartID, gaugeChartID, "requests", "absolute", 7, first)
	mixed := c020Chart(mixedChartID, mixedChartID, "requests/s", "incremental", 5, first)
	mixed.Dimensions[0].ID = "rate"
	mixed.Dimensions = append(mixed.Dimensions, c020Chart(
		mixedChartID, mixedChartID, "requests/s", "absolute", 7, first,
	).Dimensions[0])
	mixed.Dimensions[1].ID = "gauge"
	for i, ch := range []*fixture.Chart{&rate, &gauge, &mixed} {
		for d := range ch.Dimensions {
			ch.Dimensions[d].Points = append(ch.Dimensions[d].Points, fixture.Point{
				T: now, Collected: strconv.Itoa(101 + i + d), Flags: stream.FlagNotAnomalous,
			})
		}
	}
	c020PushCharts(t, host, guid(334), rate, gauge, mixed)

	cases := []struct {
		name, contract, chart, dimension, want string
	}{
		{
			name: "rate", contract: "CASE-020/badge-rate-sum-units",
			chart: rateChartID, dimension: "value", want: "requests",
		},
		{
			name: "gauge", contract: "CASE-020/badge-gauge-sum-units",
			chart: gaugeChartID, dimension: "value", want: "requests",
		},
		{
			name: "mixed-chart-rate-selection", contract: "CASE-020/badge-mixed-algorithm-sum-units",
			chart: mixedChartID, dimension: "rate", want: "requests/s",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			trackContract(t, tc.contract)
			svg := c020Badge(t, c020BadgeSpec{
				baseURL: td.BaseURL, host: host, chart: tc.chart,
				dimension: tc.dimension, group: "sum", after: first, before: before,
			})
			c020AssertBadgeUnits(t, svg, tc.want)
		})
	}
}
