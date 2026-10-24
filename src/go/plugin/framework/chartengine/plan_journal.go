// SPDX-License-Identifier: GPL-3.0-or-later

package chartengine

import "slices"

// planJournal lets a plan build mutate the committed materialized state in place.
//
// The first write of a committed object in a build records its committed value;
// objects created by the build carry the build token and need no record. Commit
// drops the journal; Abort replays it, so an aborted attempt leaves the committed
// state exactly as it was. Planning an unchanged chart therefore records only
// sequence stamps and scratch values instead of cloning the chart.
//
// A nil journal mutates without recording (direct state setup in tests).
type planJournal struct {
	token     uint64
	chartMaps []chartMapUndo
	dimMaps   []dimMapUndo
	entryMaps []entryMapUndo
	charts    []chartUndo
	dims      []dimUndo
	entries   []entryUndo
	seqs      []seqUndo

	// byChart indexes chart and dimension-map records by chart for committedChart. It
	// is built on first use and extended with the records appended since.
	byChart        map[*materializedChartState]*chartRecords
	indexedCharts  int
	indexedDimMaps int
}

// chartRecords locates one chart's records in the journal.
type chartRecords struct {
	header int   // index in charts of the chart's oldest header record, or -1
	dims   []int // indexes in dimMaps of the chart's records, oldest first
}

// Map records mark deletions (del) so a commit can count them per map.
type chartMapUndo struct {
	m   map[string]*materializedChartState
	id  string
	old *materializedChartState
	had bool
	del bool
}

// dimMapUndo keys on the owning chart: a chart's dimension map is never replaced
// during a build.
type dimMapUndo struct {
	chart *materializedChartState
	name  string
	old   *materializedDimensionState
	had   bool
	del   bool
}

type entryMapUndo struct {
	m    map[string]*dimBuildEntry
	name string
	old  *dimBuildEntry
	had  bool
	del  bool
	// owner is the chart whose scratch map m is, if any.
	owner *materializedChartState
}

type chartUndo struct {
	chart *materializedChartState
	old   materializedChartState
}

type dimUndo struct {
	dim *materializedDimensionState
	old materializedDimensionState
}

type entryUndo struct {
	entry *dimBuildEntry
	old   dimBuildEntry
}

type seqUndo struct {
	seq *uint64
	old uint64
}

// journalSizing carries the previous build's record counts so the next journal
// allocates each record slice once.
type journalSizing struct {
	entries int
	seqs    int
}

func newPlanJournal(token uint64, sizing journalSizing) planJournal {
	j := planJournal{
		token: token,
	}
	if sizing.entries > 0 {
		j.entries = make([]entryUndo, 0, sizing.entries)
	}
	if sizing.seqs > 0 {
		j.seqs = make([]seqUndo, 0, sizing.seqs)
	}
	return j
}

func (j *planJournal) sizing() journalSizing {
	if j == nil {
		return journalSizing{}
	}
	return journalSizing{entries: len(j.entries), seqs: len(j.seqs)}
}

// newChart returns a chart owned by this build.
func (j *planJournal) newChart(chart *materializedChartState) *materializedChartState {
	if j != nil {
		chart.journalToken = j.token
	}
	return chart
}

// touchChart records a committed chart before its first field write in this build.
func (j *planJournal) touchChart(chart *materializedChartState) {
	if j == nil || chart.journalToken == j.token {
		return
	}
	j.charts = append(j.charts, chartUndo{chart: chart, old: *chart})
	chart.journalToken = j.token
}

func (j *planJournal) touchDim(dim *materializedDimensionState) {
	if j == nil || dim.journalToken == j.token {
		return
	}
	j.dims = append(j.dims, dimUndo{dim: dim, old: *dim})
	dim.journalToken = j.token
}

func (j *planJournal) touchEntry(entry *dimBuildEntry) {
	if j == nil || entry.journalToken == j.token {
		return
	}
	j.entries = append(j.entries, entryUndo{entry: entry, old: *entry})
	entry.journalToken = j.token
}

// setChartSeen stamps a chart; a touched or new chart already has its committed value recorded.
func (j *planJournal) setChartSeen(chart *materializedChartState, seq uint64) {
	if chart.lastSeenSuccessSeq == seq {
		return
	}
	if j != nil && chart.journalToken != j.token {
		j.seqs = append(j.seqs, seqUndo{seq: &chart.lastSeenSuccessSeq, old: chart.lastSeenSuccessSeq})
	}
	chart.lastSeenSuccessSeq = seq
}

func (j *planJournal) setDimSeen(dim *materializedDimensionState, seq uint64) {
	if dim.lastSeenSuccessSeq == seq {
		return
	}
	if j != nil && dim.journalToken != j.token {
		j.seqs = append(j.seqs, seqUndo{seq: &dim.lastSeenSuccessSeq, old: dim.lastSeenSuccessSeq})
	}
	dim.lastSeenSuccessSeq = seq
}

func (j *planJournal) putChart(m map[string]*materializedChartState, id string, chart *materializedChartState) {
	if j != nil {
		old, had := m[id]
		j.chartMaps = append(j.chartMaps, chartMapUndo{m: m, id: id, old: old, had: had})
	}
	m[id] = chart
}

func (j *planJournal) deleteChart(m map[string]*materializedChartState, id string) {
	old, had := m[id]
	if !had {
		return
	}
	if j != nil {
		j.chartMaps = append(j.chartMaps, chartMapUndo{
			m:   m,
			id:  id,
			old: old,
			had: true,
			del: true,
		})
	}
	delete(m, id)
}

func (j *planJournal) putDim(chart *materializedChartState, name string, dim *materializedDimensionState) {
	if j != nil {
		old, had := chart.dimensions[name]
		j.dimMaps = append(j.dimMaps, dimMapUndo{chart: chart, name: name, old: old, had: had})
		dim.journalToken = j.token
	}
	chart.dimensions[name] = dim
}

func (j *planJournal) deleteDim(chart *materializedChartState, name string) {
	old, had := chart.dimensions[name]
	if !had {
		return
	}
	if j != nil {
		j.dimMaps = append(j.dimMaps, dimMapUndo{
			chart: chart,
			name:  name,
			old:   old,
			had:   true,
			del:   true,
		})
	}
	delete(chart.dimensions, name)
}

func (j *planJournal) putEntry(m map[string]*dimBuildEntry, name string, entry *dimBuildEntry) {
	if j != nil {
		old, had := m[name]
		j.entryMaps = append(j.entryMaps, entryMapUndo{m: m, name: name, old: old, had: had})
		entry.journalToken = j.token
	}
	m[name] = entry
}

func (j *planJournal) deleteEntry(owner *materializedChartState, m map[string]*dimBuildEntry, name string) {
	old, had := m[name]
	if !had {
		return
	}
	if j != nil {
		j.entryMaps = append(j.entryMaps, entryMapUndo{
			m:     m,
			name:  name,
			old:   old,
			had:   true,
			del:   true,
			owner: owner,
		})
	}
	delete(m, name)
}

// compactMinDeletes is how many deletions a materialized map absorbs before it can be
// rebuilt, so small or slowly churning maps are left alone.
const compactMinDeletes = 64

// compactAfterCommit runs once this build is committed and can no longer roll back.
// Go maps keep their capacity after deletions and staging reuses the committed maps,
// so a map whose deletions since its last rebuild exceed its live size is rebuilt at
// its live size: capacity follows live cardinality within a constant factor. Work is
// O(journal) plus amortized O(1) per deletion.
func (j *planJournal) compactAfterCommit(state *materializedState) {
	for _, rec := range j.chartMaps {
		if rec.del {
			state.chartDeletes++
		}
	}
	if needsCompaction(len(state.charts), state.chartDeletes) {
		state.charts = rebuildMap(state.charts)
		state.chartDeletes = 0
	}
	for _, rec := range j.dimMaps {
		if rec.del {
			rec.chart.dimDeletes++
		}
	}
	for _, rec := range j.dimMaps {
		if c := rec.chart; rec.del && needsCompaction(len(c.dimensions), int(c.dimDeletes)) {
			c.dimensions = rebuildMap(c.dimensions)
			c.dimDeletes = 0
		}
	}
	for _, rec := range j.entryMaps {
		if rec.del && rec.owner != nil {
			rec.owner.scratchDeletes++
		}
	}
	for _, rec := range j.entryMaps {
		if c := rec.owner; rec.del && c != nil && needsCompaction(len(c.scratchEntries), int(c.scratchDeletes)) {
			c.scratchEntries = rebuildMap(c.scratchEntries)
			c.scratchDeletes = 0
		}
	}
}

// chartRecords returns the records of chart, indexing records appended since the
// last call, so a build that looks up many charts stays linear in its journal.
func (j *planJournal) chartRecords(chart *materializedChartState) *chartRecords {
	if j.byChart == nil {
		j.byChart = make(map[*materializedChartState]*chartRecords)
	}
	of := func(c *materializedChartState) *chartRecords {
		recs := j.byChart[c]
		if recs == nil {
			recs = &chartRecords{
				header: -1,
			}
			j.byChart[c] = recs
		}
		return recs
	}
	for ; j.indexedCharts < len(j.charts); j.indexedCharts++ {
		if recs := of(j.charts[j.indexedCharts].chart); recs.header < 0 {
			recs.header = j.indexedCharts
		}
	}
	for ; j.indexedDimMaps < len(j.dimMaps); j.indexedDimMaps++ {
		recs := of(j.dimMaps[j.indexedDimMaps].chart)
		recs.dims = append(recs.dims, j.indexedDimMaps)
	}
	return j.byChart[chart]
}

func needsCompaction(live, deletes int) bool {
	return deletes >= compactMinDeletes && deletes > live
}

func rebuildMap[V any](m map[string]V) map[string]V {
	out := make(map[string]V, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// committedChart returns the committed definition of a chart this build may already
// have modified: its committed header and committed dimension membership. Dimension
// definition fields other than ordering and stamps never change for an existing
// dimension, so the live dimension values serve as committed definitions.
func (j *planJournal) committedChart(chart *materializedChartState) *materializedChartState {
	if j == nil || chart == nil {
		return chart
	}
	out := *chart
	recs := j.chartRecords(chart)
	if recs == nil {
		return &out
	}
	if recs.header >= 0 {
		out = j.charts[recs.header].old
	}
	var membership map[string]*materializedDimensionState
	for _, i := range slices.Backward(recs.dims) {
		rec := j.dimMaps[i]
		if membership == nil {
			membership = make(map[string]*materializedDimensionState)
		}
		// Reverse order leaves the oldest (committed) membership for each name.
		membership[rec.name] = rec.old
		if !rec.had {
			membership[rec.name] = nil
		}
	}
	if membership == nil {
		out.dimensions = chart.dimensions
		return &out
	}
	dims := make(map[string]*materializedDimensionState, len(chart.dimensions)+len(membership))
	for name, dim := range chart.dimensions {
		dims[name] = dim
	}
	for name, dim := range membership {
		if dim == nil {
			delete(dims, name)
			continue
		}
		dims[name] = dim
	}
	out.dimensions = dims
	return &out
}

// rollback restores every recorded committed value, newest first within each kind.
// Map membership and whole-object records are independent of sequence stamps, and a
// stamp record always holds the committed value, so stamps are restored last.
func (j *planJournal) rollback() {
	if j == nil {
		return
	}
	for i := len(j.chartMaps) - 1; i >= 0; i-- {
		rec := j.chartMaps[i]
		if rec.had {
			rec.m[rec.id] = rec.old
		} else {
			delete(rec.m, rec.id)
		}
	}
	for i := len(j.dimMaps) - 1; i >= 0; i-- {
		rec := j.dimMaps[i]
		if rec.had {
			rec.chart.dimensions[rec.name] = rec.old
		} else {
			delete(rec.chart.dimensions, rec.name)
		}
	}
	for i := len(j.entryMaps) - 1; i >= 0; i-- {
		rec := j.entryMaps[i]
		if rec.had {
			rec.m[rec.name] = rec.old
		} else {
			delete(rec.m, rec.name)
		}
	}
	for i := len(j.charts) - 1; i >= 0; i-- {
		*j.charts[i].chart = j.charts[i].old
	}
	for i := len(j.dims) - 1; i >= 0; i-- {
		*j.dims[i].dim = j.dims[i].old
	}
	for i := len(j.entries) - 1; i >= 0; i-- {
		*j.entries[i].entry = j.entries[i].old
	}
	for i := len(j.seqs) - 1; i >= 0; i-- {
		*j.seqs[i].seq = j.seqs[i].old
	}
	*j = planJournal{token: j.token}
}
