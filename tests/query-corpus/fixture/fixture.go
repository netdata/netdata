// SPDX-License-Identifier: GPL-3.0-or-later

// Package fixture models the corpus fixtures: deterministic data shapes at a
// fixed 2023 epoch, pushed through the streaming protocol and used to compute
// expected query results. Expectations are always derived from these
// definitions — never from the engine under test.
package fixture

import (
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

// Host is one child host: pushed over its own streaming connection.
type Host struct {
	Hostname    string
	MachineGUID string
	Charts      []Chart
}

// FirstT returns the earliest point timestamp across the chart's dimensions.
func (c Chart) FirstT() int64 {
	var first int64
	for _, d := range c.Dimensions {
		if len(d.Points) > 0 && (first == 0 || d.Points[0].T < first) {
			first = d.Points[0].T
		}
	}
	return first
}

// LastT returns the latest point timestamp across the chart's dimensions.
func (c Chart) LastT() int64 {
	var last int64
	for _, d := range c.Dimensions {
		if n := len(d.Points); n > 0 && d.Points[n-1].T > last {
			last = d.Points[n-1].T
		}
	}
	return last
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
// samples, row by row across dimensions (dimensions must share timestamps).
func (c Chart) PushLive(conn *stream.Conn) {
	ue := c.UpdateEvery
	if ue <= 0 {
		ue = 1
	}
	for i, p := range c.Dimensions[0].Points {
		conn.Begin2(c.ID, ue, p.T)
		for _, d := range c.Dimensions {
			conn.Set2(d.ID, d.Points[i].Collected, d.Points[i].Flags)
		}
		conn.End2()
	}
}

// ReplayWindow returns the chart's rows inside (after, before] in the
// stream.ReplayHandler contract.
func (c Chart) ReplayWindow(after, before int64) []stream.ReplayRow {
	var rows []stream.ReplayRow
	for i, p := range c.Dimensions[0].Points {
		if p.T <= after || p.T > before {
			continue
		}
		row := stream.ReplayRow{T: p.T}
		for _, d := range c.Dimensions {
			row.Dims = append(row.Dims, stream.ReplayValue{
				ID:        d.ID,
				Collected: d.Points[i].Collected,
				Flags:     d.Points[i].Flags,
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

// Expected computes the tier0 read-back oracle for the dimension. SN flags
// text semantics: 'E' = empty slot (gap); 'R' = reset annotation; 'A' =
// explicitly NOT anomalous — a sample without 'A' (and not empty) is
// anomalous (ARP 100). Values pass through the storage_number quantization
// (SNRoundTrip).
func (d Dimension) Expected() []ExpectedPoint {
	out := make([]ExpectedPoint, 0, len(d.Points))
	for _, p := range d.Points {
		ep := ExpectedPoint{T: p.T}
		flags := string(p.Flags)
		switch {
		case strings.ContainsRune(flags, 'E'):
			ep.PA = PAEmpty
		default:
			v, err := strconv.ParseFloat(p.Collected, 64)
			if err != nil {
				// a malformed fixture must fail loudly, not read as a gap
				panic("fixture: unparsable collected value " + strconv.Quote(p.Collected) + " for dimension " + d.ID)
			}
			q := SNRoundTrip(v)
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
