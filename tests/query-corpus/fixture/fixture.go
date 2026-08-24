// SPDX-License-Identifier: GPL-3.0-or-later

// Package fixture models the corpus fixtures: deterministic data shapes at a
// fixed 2023 epoch, pushed through the streaming protocol and used to compute
// expected query results. Expectations are always derived from these
// definitions — never from the engine under test.
package fixture

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/netdata/netdata/tests/query-corpus/stream"
)

// T0 is the fixed fixture epoch: 2023-11-14 22:13:20 UTC. All fixture
// timestamps are offsets from it, making every case deterministic.
const T0 = 1700000000

// Point is one collected sample: T is the exact sample timestamp, Collected
// the wire text of the collected value (kept as a string for byte-exact
// control), Flags the SN flags text.
type Point struct {
	T         int64
	Collected string
	Flags     string
}

// Dimension is one metric of a chart with its full point series.
type Dimension struct {
	ID        string
	Algorithm string // defaults to absolute
	Mul, Div  int    // default to 1
	Points    []Point
}

// Chart is one instance with its dimensions and labels. ValueTolerance,
// when non-zero, is the relative tolerance for value comparison (used by
// quantization-probing fixtures; zero means exact).
type Chart struct {
	ID             string
	Title          string
	Units          string
	Family         string
	Context        string
	UpdateEvery    int
	ValueTolerance float64
	Labels         [][2]string
	Dimensions     []Dimension
}

// FirstT returns the earliest point timestamp across the chart's dimensions.
func (c Chart) FirstT() int64 {
	var first int64
	found := false
	for _, d := range c.Dimensions {
		for _, p := range d.Points {
			if !found || p.T < first {
				first = p.T
				found = true
			}
		}
	}
	return first
}

// LastT returns the latest point timestamp across the chart's dimensions.
func (c Chart) LastT() int64 {
	var last int64
	found := false
	for _, d := range c.Dimensions {
		for _, p := range d.Points {
			if !found || p.T > last {
				last = p.T
				found = true
			}
		}
	}
	return last
}

// CollectedValue returns one finite numeric fixture value. An E flag is the
// only way a fixture can represent a gap; every other malformed or non-finite
// wire value is a harness defect and panics before it can be mistaken for
// missing data.
func (p Point) CollectedValue(dimensionID string) (float64, bool) {
	if strings.ContainsRune(p.Flags, 'E') {
		return 0, false
	}

	value, err := parseFiniteDecimal(p.Collected)
	if err != nil {
		panic(
			"fixture: collected value " + strconv.Quote(p.Collected) +
				" is not finite for dimension " + strconv.Quote(dimensionID) +
				" at timestamp " + strconv.FormatInt(p.T, 10))
	}
	return value, true
}

// parseFiniteDecimal accepts only the corpus fixture's canonical decimal
// number syntax. Wire encodings such as hexadecimal storage numbers are
// deliberately excluded: fixtures model collected values, not protocol
// shorthand, so the oracle cannot parse a different value than ingestion.
func parseFiniteDecimal(text string) (float64, error) {
	if text == "" {
		return 0, fmt.Errorf("empty decimal")
	}

	i := 0
	if text[i] == '+' || text[i] == '-' {
		i++
	}
	mantissaDigits := 0
	for i < len(text) && text[i] >= '0' && text[i] <= '9' {
		i++
		mantissaDigits++
	}
	if i < len(text) && text[i] == '.' {
		i++
		for i < len(text) && text[i] >= '0' && text[i] <= '9' {
			i++
			mantissaDigits++
		}
	}
	if mantissaDigits == 0 {
		return 0, fmt.Errorf("decimal has no mantissa digits")
	}
	if i < len(text) && (text[i] == 'e' || text[i] == 'E') {
		i++
		if i < len(text) && (text[i] == '+' || text[i] == '-') {
			i++
		}
		exponentStart := i
		for i < len(text) && text[i] >= '0' && text[i] <= '9' {
			i++
		}
		if i == exponentStart {
			return 0, fmt.Errorf("decimal has no exponent digits")
		}
	}
	if i != len(text) {
		return 0, fmt.Errorf("decimal has trailing syntax")
	}

	value, err := strconv.ParseFloat(text, 64)
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, fmt.Errorf("decimal is not finite")
	}
	return value, nil
}

// Define buffers the chart metadata (CHART, DIMENSION, CLABEL) on conn.
func (c Chart) Define(conn *stream.Conn) {
	conn.DefineChart(stream.Chart{
		ID:          c.ID,
		Title:       c.Title,
		Units:       c.Units,
		Family:      c.Family,
		Context:     c.Context,
		UpdateEvery: c.UpdateEvery,
	})
	for _, d := range c.Dimensions {
		conn.Dimension(d.ID, d.Algorithm, d.Mul, d.Div)
	}
	if len(c.Labels) > 0 {
		for _, kv := range c.Labels {
			conn.CLabel(kv[0], kv[1])
		}
		conn.CLabelCommit()
	}
}

// PushLive buffers the chart's full point series as BEGIN2/SET2/END2
// samples, row by row.
//
// Dimensions are matched by TIMESTAMP, not by position, so a dimension may
// carry fewer points than its siblings: a dimension that stops being
// collected while its chart keeps going simply gets no SET2 in the later
// rows, which is how a removed disk or a departed container looks on the
// wire. That is different from pushing an empty slot - there is no stored
// point at all, so the dimension's storage genuinely runs out.
func (c Chart) PushLive(conn *stream.Conn) {
	ue := c.UpdateEvery
	if ue <= 0 {
		ue = 1
	}

	byTime := c.pointsByTime()

	for _, p := range c.rowTimes() {
		conn.Begin2(c.ID, ue, p)
		for di, d := range c.Dimensions {
			if dp, has := byTime[di][p]; has {
				conn.Set2(d.ID, dp.Collected, dp.Flags)
			}
		}
		conn.End2()
	}
}

// rowTimes is every timestamp any dimension carries, in order.
func (c Chart) rowTimes() []int64 {
	seen := make(map[int64]struct{})
	var out []int64
	for _, d := range c.Dimensions {
		for _, p := range d.Points {
			if _, has := seen[p.T]; has {
				continue
			}
			seen[p.T] = struct{}{}
			out = append(out, p.T)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func (d Dimension) pointsByTime() map[int64]Point {
	byTime := make(map[int64]Point, len(d.Points))
	for _, p := range d.Points {
		if _, duplicate := byTime[p.T]; duplicate {
			panic(fmt.Sprintf("fixture: dimension %q repeats timestamp %d", d.ID, p.T))
		}
		byTime[p.T] = p
	}
	return byTime
}

// pointsInTimeOrder returns the dimension in the same timestamp order used by
// live and replication ingestion.
func (d Dimension) pointsInTimeOrder() []Point {
	byTime := d.pointsByTime()
	times := make([]int64, 0, len(byTime))
	for ts := range byTime {
		times = append(times, ts)
	}
	sort.Slice(times, func(i, j int) bool { return times[i] < times[j] })

	points := make([]Point, 0, len(times))
	for _, ts := range times {
		points = append(points, byTime[ts])
	}
	return points
}

func (c Chart) pointsByTime() []map[int64]Point {
	byTime := make([]map[int64]Point, len(c.Dimensions))
	for i, d := range c.Dimensions {
		byTime[i] = d.pointsByTime()
	}
	return byTime
}

// ReplayWindow returns the chart's rows inside (after, before] in the
// stream.ReplayHandler contract.
func (c Chart) ReplayWindow(after, before int64) []stream.ReplayRow {
	// Matched by TIMESTAMP, like PushLive. Pairing by position assumes every
	// dimension carries the same number of points at the same moments, which
	// a chart whose dimensions stop at different times does not - and that
	// shape is deliberately used (a dimension whose storage runs out while
	// its chart keeps going). Indexing the shorter dimension by the longer
	// one's position reads the wrong sample, or runs off the end.
	byTime := c.pointsByTime()

	var rows []stream.ReplayRow
	for _, ts := range c.rowTimes() {
		if ts <= after || ts > before {
			continue
		}
		row := stream.ReplayRow{T: ts}
		for di, d := range c.Dimensions {
			dp, has := byTime[di][ts]
			if !has {
				// this dimension has nothing at this moment - the same
				// thing PushLive does, which is to say nothing at all
				continue
			}
			row.Dims = append(row.Dims, stream.ReplayValue{
				ID:        d.ID,
				Collected: dp.Collected,
				Flags:     dp.Flags,
			})
		}
		rows = append(rows, row)
	}
	return rows
}

// Point annotation bits as exposed in json2 (RRDR_VALUE_* in
// src/web/api/queries/rrdr.h).
const (
	PAEmpty = 1 << 0
	PAReset = 1 << 1
)

// ExpectedPoint is the oracle's view of one queried second of one dimension:
// Value is nil for gaps; ARP is the expected anomaly rate percentage; PA
// the expected annotation bits.
type ExpectedPoint struct {
	T     int64
	Value *float64
	ARP   float64
	PA    int64
}

// Expected computes the tier0 read-back oracle for the dimension. Live v2
// SET2 and replicated RSET records carry the calculated value explicitly, so
// Algorithm/Mul/Div metadata does not transform this value on the parent; v1
// raw-counter fixtures use their own oracle. SN flags
// text semantics: 'E' = empty slot (gap); 'R' = reset annotation; 'A' =
// explicitly NOT anomalous — a sample without 'A' (and not empty) is
// anomalous (ARP 100). Values pass through the storage_number quantization
// (SNRoundTrip).
func (d Dimension) Expected() []ExpectedPoint {
	out := make([]ExpectedPoint, 0, len(d.Points))
	for _, p := range d.pointsInTimeOrder() {
		ep := ExpectedPoint{T: p.T}
		flags := string(p.Flags)
		if value, collected := p.CollectedValue(d.ID); !collected {
			ep.PA = PAEmpty
		} else {
			q := SNRoundTrip(value)
			ep.Value = &q
			if strings.ContainsRune(flags, 'R') {
				ep.PA |= PAReset
			}
			if !strings.ContainsRune(flags, 'A') {
				ep.ARP = 100
			}
		}
		out = append(out, ep)
	}
	return out
}

// Series builds a single-dimension chart from per-index generators:
// i runs 1..n, timestamps t0 + i*ue.
func Series(chartID, context string, t0 int64, n, ue int, collected func(i int) string, flags func(i int) string) Chart {
	if ue <= 0 {
		ue = 1
	}
	points := make([]Point, 0, n)
	for i := 1; i <= n; i++ {
		points = append(points, Point{
			T:         t0 + int64(i*ue),
			Collected: collected(i),
			Flags:     flags(i),
		})
	}
	return Chart{
		ID:          chartID,
		Title:       "Corpus series",
		Units:       "units",
		Family:      "fixture",
		Context:     context,
		UpdateEvery: ue,
		Dimensions: []Dimension{
			{ID: "load", Points: points},
		},
	}
}

// FullPalette is the layer-0 wire-fidelity shape: n per-second points
// starting at t0+1, values i%10, an empty slot (gap) at t0+31 and an
// anomalous sample at t0+41. It exercises the complete/interior-gap/
// anomalous/all-value-digits palette entries in one chart.
func FullPalette(chartID, context string, t0 int64, n int) Chart {
	points := make([]Point, 0, n)
	for i := 1; i <= n; i++ {
		p := Point{T: t0 + int64(i), Collected: strconv.Itoa(i % 10), Flags: stream.FlagNotAnomalous}
		switch i {
		case 31:
			p.Flags = stream.FlagEmpty
		case 41:
			p.Flags = stream.FlagAnomalous
		}
		points = append(points, p)
	}
	return Chart{
		ID:      chartID,
		Title:   "Corpus full palette",
		Units:   "units",
		Family:  "fixture",
		Context: context,
		Dimensions: []Dimension{
			{ID: "load", Points: points},
		},
	}
}
