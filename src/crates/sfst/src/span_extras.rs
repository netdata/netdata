//! Span events and links: the `EVNB` / `LNKB` chunks and their build-side
//! accumulators.
//!
//! Rows hold their event/link *values* as regular interned tokens (searchable
//! through the normal field tiers, listed in the row's stream-batch entry
//! list). What the token list cannot express is **structure** — which tokens
//! form event #1 vs event #2, in what order, at what time. These chunks store
//! exactly that structure: per-row prefix-sum offsets into per-event/per-link
//! parallel arrays whose token references point at the SAME interned `KvId`s
//! the row carries. No value is stored twice; the skeleton is a few integers
//! per event/link.
//!
//! Both chunks are optional and additive (the `TIDX` pattern): absent when the
//! file has no events/links (and no dropped-count to preserve), detected via
//! the TOC, no format version bump. Row positions are **chronological**, like
//! every per-row column — the build reorders the insertion-order accumulators
//! through the same time permutation.
//!
//! Span-level `dropped_events_count` / `dropped_links_count` ride here as a
//! per-row array (`row_dropped`) rather than as standalone scalar columns: they
//! belong to the events/links data and are almost always zero (they compress
//! to ~nothing), and a file with no chunk trivially means "no drops".

use serde::{Deserialize, Serialize};

use crate::kv_interner::KvSlot;
use crate::{Error, KvId, SpanId, SpanIds, TraceId, TraceIds};

// ---------------------------------------------------------------------------
// On-disk payloads
// ---------------------------------------------------------------------------

/// Body of the `EVNB` chunk: the per-row event structure. All arrays are
/// parallel; `row_offsets`/`attr_offsets` are prefix sums (`row r`'s events are
/// `row_offsets[r]..row_offsets[r+1]`; event `e`'s attribute refs are
/// `attr_offsets[e]..attr_offsets[e+1]`).
#[derive(Debug, Clone, Default, PartialEq, Eq, Serialize, Deserialize)]
pub struct EventIndex {
    /// `record_count + 1` prefix sums into the per-event arrays.
    row_offsets: Vec<u32>,
    /// `record_count` per-row OTLP `Span.dropped_events_count` values.
    row_dropped: Vec<u32>,
    /// Per-event `Event.time_unix_nano`, stored **raw** (no normalization).
    times: Vec<u64>,
    /// Per-event `Event.dropped_attributes_count`.
    dropped: Vec<u32>,
    /// Per-event name token (`events.name=<name>`); always present — the
    /// flattener emits a name entry even for a (malformed) empty name.
    names: Vec<KvId>,
    /// `total_events + 1` prefix sums into `attr_refs`.
    attr_offsets: Vec<u32>,
    /// Per-attribute token refs (`events.attributes.*`), ordered within each
    /// event. Bare `KvId`s: values type at field level via the schema tree —
    /// the platform-wide fidelity bar (no per-occurrence type refs).
    attr_refs: Vec<KvId>,
}

/// One event of one row, resolved from an [`EventIndex`] — token refs, not
/// strings (the reader resolves refs in bulk per page/trace).
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct EventRef<'a> {
    pub time_unix_nano: u64,
    pub dropped_attributes_count: u32,
    pub name: KvId,
    pub attr_refs: &'a [KvId],
}

impl EventIndex {
    /// Total events in the file.
    pub fn total_events(&self) -> usize {
        self.times.len()
    }

    /// The span-level `dropped_events_count` of row `pos`.
    pub fn row_dropped_count(&self, pos: u32) -> u32 {
        self.row_dropped[pos as usize]
    }

    /// Row `pos`'s events, in original OTLP order.
    pub fn events_for_row(&self, pos: u32) -> impl Iterator<Item = EventRef<'_>> {
        let lo = self.row_offsets[pos as usize] as usize;
        let hi = self.row_offsets[pos as usize + 1] as usize;
        (lo..hi).map(move |e| EventRef {
            time_unix_nano: self.times[e],
            dropped_attributes_count: self.dropped[e],
            name: self.names[e],
            attr_refs: &self.attr_refs
                [self.attr_offsets[e] as usize..self.attr_offsets[e + 1] as usize],
        })
    }

    /// Every token ref the index holds (names + attribute refs) — the reader
    /// collects these per trace to resolve strings in one pass.
    pub fn all_refs_for_row(&self, pos: u32) -> impl Iterator<Item = KvId> + '_ {
        self.events_for_row(pos)
            .flat_map(|e| std::iter::once(e.name).chain(e.attr_refs.iter().copied()))
    }

    /// Panic-safety validation at the trust boundary (the `TIDX` contract):
    /// array parallelism, prefix-sum monotonicity/termination, ref ranges.
    pub(crate) fn validate(&self, record_count: usize, kv_total: u32) -> Result<(), Error> {
        validate_skeleton(
            "event index",
            &self.row_offsets,
            &self.row_dropped,
            record_count,
            self.times.len(),
        )?;
        if self.dropped.len() != self.times.len() || self.names.len() != self.times.len() {
            return Err(Error::CorruptIndex(format!(
                "event index parallel arrays disagree: times {}, dropped {}, names {}",
                self.times.len(),
                self.dropped.len(),
                self.names.len()
            )));
        }
        validate_offsets(
            "event index attr_offsets",
            &self.attr_offsets,
            self.times.len(),
            self.attr_refs.len(),
        )?;
        if self
            .names
            .iter()
            .chain(self.attr_refs.iter())
            .any(|id| id.0 >= kv_total)
        {
            return Err(Error::CorruptIndex(format!(
                "event index token ref out of range (kv total {kv_total})"
            )));
        }
        Ok(())
    }
}

/// Body of the `LNKB` chunk: the per-row link structure. Same skeleton shape as
/// [`EventIndex`]; the per-link scalars differ (linked ids, flags, verbatim
/// `trace_state` in a small byte arena instead of a timestamp).
#[derive(Debug, Clone, Default, PartialEq, Eq, Serialize, Deserialize)]
pub struct LinkIndex {
    /// `record_count + 1` prefix sums into the per-link arrays.
    row_offsets: Vec<u32>,
    /// `record_count` per-row OTLP `Span.dropped_links_count` values.
    row_dropped: Vec<u32>,
    /// Per-link linked-to `trace_id` (16-byte arena; all-zero = unset).
    trace_ids: TraceIds,
    /// Per-link linked-to `span_id` (8-byte arena; all-zero = unset).
    span_ids: SpanIds,
    /// Per-link W3C `flags`.
    flags: Vec<u32>,
    /// Per-link `Link.dropped_attributes_count`.
    dropped: Vec<u32>,
    /// `total_links + 1` prefix sums into `ts_bytes` — each link's verbatim
    /// `trace_state` string (usually empty → zero bytes).
    ts_offsets: Vec<u32>,
    #[serde(with = "serde_bytes")]
    ts_bytes: Vec<u8>,
    /// `total_links + 1` prefix sums into `attr_refs`.
    attr_offsets: Vec<u32>,
    /// Per-attribute token refs (`links.attributes.*`), ordered within each link.
    attr_refs: Vec<KvId>,
}

/// One link of one row, resolved from a [`LinkIndex`].
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct LinkRef<'a> {
    pub trace_id: TraceId,
    pub span_id: SpanId,
    pub flags: u32,
    pub dropped_attributes_count: u32,
    /// Verbatim W3C `trace_state` bytes (written from a UTF-8 `String`).
    pub trace_state: &'a [u8],
    pub attr_refs: &'a [KvId],
}

impl LinkIndex {
    /// Total links in the file.
    pub fn total_links(&self) -> usize {
        self.flags.len()
    }

    /// The span-level `dropped_links_count` of row `pos`.
    pub fn row_dropped_count(&self, pos: u32) -> u32 {
        self.row_dropped[pos as usize]
    }

    /// Row `pos`'s links, in original OTLP order.
    pub fn links_for_row(&self, pos: u32) -> impl Iterator<Item = LinkRef<'_>> {
        let lo = self.row_offsets[pos as usize] as usize;
        let hi = self.row_offsets[pos as usize + 1] as usize;
        (lo..hi).map(move |l| LinkRef {
            trace_id: self.trace_ids.get(l),
            span_id: self.span_ids.get(l),
            flags: self.flags[l],
            dropped_attributes_count: self.dropped[l],
            trace_state: &self.ts_bytes
                [self.ts_offsets[l] as usize..self.ts_offsets[l + 1] as usize],
            attr_refs: &self.attr_refs
                [self.attr_offsets[l] as usize..self.attr_offsets[l + 1] as usize],
        })
    }

    /// Every attribute token ref of row `pos`'s links.
    pub fn all_refs_for_row(&self, pos: u32) -> impl Iterator<Item = KvId> + '_ {
        self.links_for_row(pos)
            .flat_map(|l| l.attr_refs.iter().copied())
    }

    /// Panic-safety validation at the trust boundary (see [`EventIndex::validate`]).
    pub(crate) fn validate(&self, record_count: usize, kv_total: u32) -> Result<(), Error> {
        validate_skeleton(
            "link index",
            &self.row_offsets,
            &self.row_dropped,
            record_count,
            self.flags.len(),
        )?;
        let n = self.flags.len();
        // `len()` floors (`bytes / WIDTH`), so a trailing-bytes arena would pass
        // the count check alone — reject non-whole arenas explicitly, like the
        // per-row column readers do.
        if !self.trace_ids.well_formed() || !self.span_ids.well_formed() {
            return Err(Error::CorruptIndex(
                "link index id arena is not a whole number of ids".into(),
            ));
        }
        if self.trace_ids.len() != n || self.span_ids.len() != n || self.dropped.len() != n {
            return Err(Error::CorruptIndex(format!(
                "link index parallel arrays disagree: flags {n}, trace_ids {}, span_ids {}, dropped {}",
                self.trace_ids.len(),
                self.span_ids.len(),
                self.dropped.len()
            )));
        }
        validate_offsets(
            "link index ts_offsets",
            &self.ts_offsets,
            n,
            self.ts_bytes.len(),
        )?;
        validate_offsets(
            "link index attr_offsets",
            &self.attr_offsets,
            n,
            self.attr_refs.len(),
        )?;
        if self.attr_refs.iter().any(|id| id.0 >= kv_total) {
            return Err(Error::CorruptIndex(format!(
                "link index token ref out of range (kv total {kv_total})"
            )));
        }
        Ok(())
    }
}

/// Shared skeleton checks: `row_offsets` is a `record_count + 1` monotonic
/// prefix-sum array terminating at `total`, and `row_dropped` is per-row.
fn validate_skeleton(
    what: &str,
    row_offsets: &[u32],
    row_dropped: &[u32],
    record_count: usize,
    total: usize,
) -> Result<(), Error> {
    if row_dropped.len() != record_count {
        return Err(Error::CorruptIndex(format!(
            "{what} row_dropped has {} entries, expected record_count {record_count}",
            row_dropped.len()
        )));
    }
    validate_offsets(what, row_offsets, record_count, total)
}

/// A prefix-sum offsets array over `count` items terminating at `total`.
fn validate_offsets(what: &str, offsets: &[u32], count: usize, total: usize) -> Result<(), Error> {
    if offsets.len() != count + 1 {
        return Err(Error::CorruptIndex(format!(
            "{what} has {} offsets, expected {}",
            offsets.len(),
            count + 1
        )));
    }
    if offsets.first() != Some(&0) || offsets.windows(2).any(|w| w[0] > w[1]) {
        return Err(Error::CorruptIndex(format!(
            "{what} offsets are not a monotonic prefix sum"
        )));
    }
    if offsets.last().copied().unwrap_or(0) as usize != total {
        return Err(Error::CorruptIndex(format!(
            "{what} offsets terminate at {}, expected total {total}",
            offsets.last().copied().unwrap_or(0)
        )));
    }
    Ok(())
}

// ---------------------------------------------------------------------------
// Build-side accumulators (insertion order, interner slots)
// ---------------------------------------------------------------------------

/// Insertion-order accumulator for [`EventIndex`]: the traces seal pushes each
/// span's events ([`push_event`](Self::push_event)) and seals the row
/// ([`end_row`](Self::end_row)) — exactly one `end_row` per
/// [`RowIndex::row`](crate::RowIndex::row) call, enforced at build by the
/// row-count check. Holds interner [`KvSlot`]s; the build remaps rows to
/// chronological order and translates slots to file [`KvId`]s.
#[derive(Debug, Clone)]
pub struct EventRows {
    row_offsets: Vec<u32>,
    row_dropped: Vec<u32>,
    times: Vec<u64>,
    dropped: Vec<u32>,
    names: Vec<KvSlot>,
    attr_offsets: Vec<u32>,
    attr_refs: Vec<KvSlot>,
}

impl Default for EventRows {
    fn default() -> Self {
        Self {
            row_offsets: vec![0],
            row_dropped: Vec::new(),
            times: Vec::new(),
            dropped: Vec::new(),
            names: Vec::new(),
            attr_offsets: vec![0],
            attr_refs: Vec::new(),
        }
    }
}

impl EventRows {
    pub fn new() -> Self {
        Self::default()
    }

    /// Append one event to the current (not yet ended) row.
    pub fn push_event(
        &mut self,
        time_unix_nano: u64,
        dropped: u32,
        name: KvSlot,
        attrs: &[KvSlot],
    ) {
        self.times.push(time_unix_nano);
        self.dropped.push(dropped);
        self.names.push(name);
        self.attr_refs.extend_from_slice(attrs);
        self.attr_offsets.push(self.attr_refs.len() as u32);
    }

    /// Seal the current row with its span-level `dropped_events_count`.
    pub fn end_row(&mut self, dropped_events_count: u32) {
        self.row_dropped.push(dropped_events_count);
        self.row_offsets.push(self.times.len() as u32);
    }

    /// Rows accumulated so far (must equal the row count at build).
    pub fn num_rows(&self) -> usize {
        self.row_dropped.len()
    }

    /// Whether the chunk would carry any information (an event, or a nonzero
    /// dropped count that must survive). The producer skips the chunk otherwise.
    pub fn is_meaningful(&self) -> bool {
        !self.times.is_empty() || self.row_dropped.iter().any(|&d| d != 0)
    }

    /// Chronological reorder + slot→file-id translation (build phase 2).
    /// `positions` yields insertion-order indices in chronological order —
    /// the same permutation every per-row column rides.
    pub(crate) fn reordered(
        &self,
        positions: impl Iterator<Item = usize>,
        kv_to_file: &[KvId],
    ) -> EventIndex {
        let mut out = EventIndex {
            row_offsets: Vec::with_capacity(self.row_dropped.len() + 1),
            row_dropped: Vec::with_capacity(self.row_dropped.len()),
            times: Vec::with_capacity(self.times.len()),
            dropped: Vec::with_capacity(self.dropped.len()),
            names: Vec::with_capacity(self.names.len()),
            attr_offsets: Vec::with_capacity(self.attr_offsets.len()),
            attr_refs: Vec::with_capacity(self.attr_refs.len()),
        };
        out.row_offsets.push(0);
        out.attr_offsets.push(0);
        for ins in positions {
            out.row_dropped.push(self.row_dropped[ins]);
            let lo = self.row_offsets[ins] as usize;
            let hi = self.row_offsets[ins + 1] as usize;
            for e in lo..hi {
                out.times.push(self.times[e]);
                out.dropped.push(self.dropped[e]);
                out.names.push(kv_to_file[self.names[e].idx()]);
                let alo = self.attr_offsets[e] as usize;
                let ahi = self.attr_offsets[e + 1] as usize;
                out.attr_refs
                    .extend(self.attr_refs[alo..ahi].iter().map(|s| kv_to_file[s.idx()]));
                out.attr_offsets.push(out.attr_refs.len() as u32);
            }
            out.row_offsets.push(out.times.len() as u32);
        }
        out
    }
}

/// Insertion-order accumulator for [`LinkIndex`] — the link analog of
/// [`EventRows`], same row protocol.
#[derive(Debug, Clone)]
pub struct LinkRows {
    row_offsets: Vec<u32>,
    row_dropped: Vec<u32>,
    trace_ids: Vec<TraceId>,
    span_ids: Vec<SpanId>,
    flags: Vec<u32>,
    dropped: Vec<u32>,
    trace_states: Vec<String>,
    attr_offsets: Vec<u32>,
    attr_refs: Vec<KvSlot>,
}

impl Default for LinkRows {
    fn default() -> Self {
        Self {
            row_offsets: vec![0],
            row_dropped: Vec::new(),
            trace_ids: Vec::new(),
            span_ids: Vec::new(),
            flags: Vec::new(),
            dropped: Vec::new(),
            trace_states: Vec::new(),
            attr_offsets: vec![0],
            attr_refs: Vec::new(),
        }
    }
}

impl LinkRows {
    pub fn new() -> Self {
        Self::default()
    }

    /// Append one link to the current (not yet ended) row.
    #[allow(clippy::too_many_arguments)]
    pub fn push_link(
        &mut self,
        trace_id: TraceId,
        span_id: SpanId,
        flags: u32,
        dropped: u32,
        trace_state: &str,
        attrs: &[KvSlot],
    ) {
        self.trace_ids.push(trace_id);
        self.span_ids.push(span_id);
        self.flags.push(flags);
        self.dropped.push(dropped);
        self.trace_states.push(trace_state.to_string());
        self.attr_refs.extend_from_slice(attrs);
        self.attr_offsets.push(self.attr_refs.len() as u32);
    }

    /// Seal the current row with its span-level `dropped_links_count`.
    pub fn end_row(&mut self, dropped_links_count: u32) {
        self.row_dropped.push(dropped_links_count);
        self.row_offsets.push(self.flags.len() as u32);
    }

    /// Rows accumulated so far (must equal the row count at build).
    pub fn num_rows(&self) -> usize {
        self.row_dropped.len()
    }

    /// Whether the chunk would carry any information (see [`EventRows::is_meaningful`]).
    pub fn is_meaningful(&self) -> bool {
        !self.flags.is_empty() || self.row_dropped.iter().any(|&d| d != 0)
    }

    /// Chronological reorder + slot→file-id translation (build phase 2).
    pub(crate) fn reordered(
        &self,
        positions: impl Iterator<Item = usize>,
        kv_to_file: &[KvId],
    ) -> LinkIndex {
        let mut out = LinkIndex {
            row_offsets: vec![0],
            row_dropped: Vec::with_capacity(self.row_dropped.len()),
            trace_ids: TraceIds::with_capacity(self.trace_ids.len()),
            span_ids: SpanIds::with_capacity(self.span_ids.len()),
            flags: Vec::with_capacity(self.flags.len()),
            dropped: Vec::with_capacity(self.dropped.len()),
            ts_offsets: vec![0],
            ts_bytes: Vec::new(),
            attr_offsets: vec![0],
            attr_refs: Vec::with_capacity(self.attr_refs.len()),
        };
        for ins in positions {
            out.row_dropped.push(self.row_dropped[ins]);
            let lo = self.row_offsets[ins] as usize;
            let hi = self.row_offsets[ins + 1] as usize;
            for l in lo..hi {
                out.trace_ids.push(self.trace_ids[l]);
                out.span_ids.push(self.span_ids[l]);
                out.flags.push(self.flags[l]);
                out.dropped.push(self.dropped[l]);
                out.ts_bytes
                    .extend_from_slice(self.trace_states[l].as_bytes());
                out.ts_offsets.push(out.ts_bytes.len() as u32);
                let alo = self.attr_offsets[l] as usize;
                let ahi = self.attr_offsets[l + 1] as usize;
                out.attr_refs
                    .extend(self.attr_refs[alo..ahi].iter().map(|s| kv_to_file[s.idx()]));
                out.attr_offsets.push(out.attr_refs.len() as u32);
            }
            out.row_offsets.push(out.flags.len() as u32);
        }
        out
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn tid(b: u8) -> TraceId {
        TraceId::from([b; 16])
    }
    fn sid(b: u8) -> SpanId {
        SpanId::from([b; 8])
    }

    /// identity translation big enough for the test slots.
    fn ident(n: u32) -> Vec<KvId> {
        (0..n).map(KvId).collect()
    }

    #[test]
    fn event_rows_round_trip_with_reorder() {
        // 3 rows in insertion order: row0 two events, row1 none (dropped=7),
        // row2 one event. Chronological order = [2, 0, 1].
        let mut rows = EventRows::new();
        rows.push_event(11, 1, KvSlot(0), &[KvSlot(1), KvSlot(2)]);
        rows.push_event(22, 0, KvSlot(3), &[]);
        rows.end_row(0);
        rows.end_row(7);
        rows.push_event(33, 0, KvSlot(4), &[KvSlot(5)]);
        rows.end_row(0);
        assert_eq!(rows.num_rows(), 3);
        assert!(rows.is_meaningful());

        let idx = rows.reordered([2usize, 0, 1].into_iter(), &ident(6));
        idx.validate(3, 6).unwrap();
        assert_eq!(idx.total_events(), 3);

        // New position 0 = old row 2.
        let evs: Vec<_> = idx.events_for_row(0).collect();
        assert_eq!(evs.len(), 1);
        assert_eq!(evs[0].time_unix_nano, 33);
        assert_eq!(evs[0].name, KvId(4));
        assert_eq!(evs[0].attr_refs, &[KvId(5)]);

        // New position 1 = old row 0: order + grouping preserved.
        let evs: Vec<_> = idx.events_for_row(1).collect();
        assert_eq!(evs.len(), 2);
        assert_eq!(
            (evs[0].time_unix_nano, evs[0].dropped_attributes_count),
            (11, 1)
        );
        assert_eq!(evs[0].attr_refs, &[KvId(1), KvId(2)]);
        assert_eq!(evs[1].time_unix_nano, 22);
        assert!(evs[1].attr_refs.is_empty());

        // New position 2 = old row 1: empty, span-level dropped preserved.
        assert_eq!(idx.events_for_row(2).count(), 0);
        assert_eq!(idx.row_dropped_count(2), 7);
    }

    #[test]
    fn empty_rows_are_not_meaningful() {
        let mut rows = EventRows::new();
        rows.end_row(0);
        rows.end_row(0);
        assert!(!rows.is_meaningful());

        let mut rows = LinkRows::new();
        rows.end_row(0);
        assert!(!rows.is_meaningful());
        // A dropped count alone IS meaningful (must survive without any link).
        let mut rows = LinkRows::new();
        rows.end_row(3);
        assert!(rows.is_meaningful());
    }

    #[test]
    fn link_rows_round_trip_with_trace_state_arena() {
        let mut rows = LinkRows::new();
        rows.push_link(tid(0xAA), sid(0xBB), 0x100, 2, "ot=th:8", &[KvSlot(0)]);
        rows.push_link(tid(0xCC), sid(0xDD), 0, 0, "", &[]);
        rows.end_row(1);
        rows.end_row(0);

        let idx = rows.reordered([0usize, 1].into_iter(), &ident(1));
        idx.validate(2, 1).unwrap();
        assert_eq!(idx.total_links(), 2);
        assert_eq!(idx.row_dropped_count(0), 1);

        let links: Vec<_> = idx.links_for_row(0).collect();
        assert_eq!(links.len(), 2);
        assert_eq!(links[0].trace_id, tid(0xAA));
        assert_eq!(links[0].span_id, sid(0xBB));
        assert_eq!(links[0].flags, 0x100);
        assert_eq!(links[0].dropped_attributes_count, 2);
        assert_eq!(links[0].trace_state, b"ot=th:8");
        assert_eq!(links[0].attr_refs, &[KvId(0)]);
        assert_eq!(links[1].trace_state, b"");
        assert_eq!(idx.links_for_row(1).count(), 0);
    }

    #[test]
    fn validate_rejects_broken_skeletons() {
        let mut rows = EventRows::new();
        rows.push_event(1, 0, KvSlot(0), &[]);
        rows.end_row(0);
        let idx = rows.reordered([0usize].into_iter(), &ident(1));

        // Wrong record_count.
        assert!(idx.validate(2, 1).is_err());
        // Token ref out of range.
        assert!(idx.validate(1, 0).is_err());
        // Good.
        assert!(idx.validate(1, 1).is_ok());
    }

    #[test]
    fn validate_rejects_corrupt_offsets_and_arenas() {
        // attr_offsets terminator disagrees with attr_refs.len().
        let mut rows = EventRows::new();
        rows.push_event(1, 0, KvSlot(0), &[KvSlot(1)]);
        rows.end_row(0);
        let mut idx = rows.reordered([0usize].into_iter(), &ident(2));
        assert!(idx.validate(1, 2).is_ok());
        *idx.attr_offsets.last_mut().unwrap() += 1;
        assert!(idx.validate(1, 2).is_err(), "bad attr terminator rejected");

        // Non-monotonic attr_offsets.
        let mut rows = EventRows::new();
        rows.push_event(1, 0, KvSlot(0), &[KvSlot(1)]);
        rows.push_event(2, 0, KvSlot(0), &[]);
        rows.end_row(0);
        let mut idx = rows.reordered([0usize].into_iter(), &ident(2));
        idx.attr_offsets = vec![0, 1, 0]; // drops below the prior offset
        assert!(
            idx.validate(1, 2).is_err(),
            "non-monotonic offsets rejected"
        );

        // A link id arena with trailing bytes: len() floors to the right count,
        // so only the well_formed() check catches it.
        let mut rows = LinkRows::new();
        rows.push_link(tid(1), sid(2), 0, 0, "", &[]);
        rows.end_row(0);
        let mut idx = rows.reordered([0usize].into_iter(), &ident(0));
        assert!(idx.validate(1, 0).is_ok());
        let raw = bincode::serde::encode_to_vec(
            serde_bytes::ByteBuf::from(vec![7u8; 17]), // 1 id + 1 trailing byte
            bincode::config::standard(),
        )
        .unwrap();
        idx.trace_ids = bincode::serde::decode_from_slice(&raw, bincode::config::standard())
            .unwrap()
            .0;
        assert!(
            idx.validate(1, 0).is_err(),
            "trailing arena bytes must be rejected"
        );
    }

    /// The reader-boundary wiring: a CRC-valid file whose EVNB payload fails
    /// validation must surface `CorruptIndex` from `ChunkReader::event_index`,
    /// and a valid payload must round-trip through the same path.
    #[test]
    fn reader_boundary_validates_event_index() {
        use std::io::Cursor;

        use crate::writer::{ChunkCounts, ChunkWriter, ColumnsPresent};
        use crate::{
            BitmapValue, ColumnsTable, Histogram, IdRanges, Metadata, SchemaTree, StreamBatch,
            Summary,
        };

        fn write_file(index: &EventIndex) -> Vec<u8> {
            let counts = ChunkCounts {
                columns: ColumnsPresent::default(),
                trace_id_index: false,
                trace_id_bloom: false,
                event_index: true,
                link_index: false,
                trace_rollup: false,
                mid_fields: 0,
                high_fields: 0,
                stream_batches: 1,
            };
            let mut w = ChunkWriter::new(Cursor::new(Vec::new()), counts).unwrap();
            w.summary(&Summary {
                min_timestamp_s: 1,
                max_timestamp_s: 1,
                record_count: 1,
                content_meta: Vec::new(),
            })
            .unwrap();
            w.metadata(&Metadata {
                histogram: Histogram {
                    timestamps: vec![1],
                    counts: vec![1],
                },
                id_ranges: IdRanges {
                    low_end: KvId(2),
                    mid_end: KvId(2),
                    high_end: KvId(2),
                },
                tree: SchemaTree::flat(&Vec::new().into()),
                columns: ColumnsTable::default(),
            })
            .unwrap();
            w.timestamps(&[1]).unwrap();
            w.primary(Vec::<(&str, BitmapValue)>::new()).unwrap();
            w.event_index(index).unwrap();
            w.add_stream_batch(&StreamBatch::for_write(&[])).unwrap();
            w.finish().unwrap().into_inner()
        }

        let mut rows = EventRows::new();
        rows.push_event(7, 0, KvSlot(0), &[KvSlot(1)]);
        rows.end_row(0);
        let good = rows.reordered([0usize].into_iter(), &ident(2));

        // Valid payload round-trips through the reader.
        let buf = write_file(&good);
        let reader = crate::reader::ChunkReader::open(&buf).unwrap();
        assert!(reader.has_event_index());
        assert_eq!(reader.event_index().unwrap(), good);

        // Corrupt payload (bad attr terminator) is rejected as CorruptIndex.
        let mut bad = good.clone();
        *bad.attr_offsets.last_mut().unwrap() += 1;
        let buf = write_file(&bad);
        let reader = crate::reader::ChunkReader::open(&buf).unwrap();
        assert!(matches!(reader.event_index(), Err(Error::CorruptIndex(_))));

        // Out-of-range token ref (>= META high_end) is rejected too.
        let mut bad = good.clone();
        bad.names[0] = KvId(99);
        let buf = write_file(&bad);
        let reader = crate::reader::ChunkReader::open(&buf).unwrap();
        assert!(matches!(reader.event_index(), Err(Error::CorruptIndex(_))));
    }

    #[test]
    fn serde_round_trip() {
        let mut rows = EventRows::new();
        rows.push_event(5, 0, KvSlot(0), &[KvSlot(1)]);
        rows.end_row(0);
        let idx = rows.reordered([0usize].into_iter(), &ident(2));
        let bytes = bincode::serde::encode_to_vec(&idx, bincode::config::standard()).unwrap();
        let (back, _): (EventIndex, _) =
            bincode::serde::decode_from_slice(&bytes, bincode::config::standard()).unwrap();
        assert_eq!(back, idx);

        let mut rows = LinkRows::new();
        rows.push_link(tid(1), sid(2), 0, 0, "x", &[]);
        rows.end_row(0);
        let idx = rows.reordered([0usize].into_iter(), &ident(0));
        let bytes = bincode::serde::encode_to_vec(&idx, bincode::config::standard()).unwrap();
        let (back, _): (LinkIndex, _) =
            bincode::serde::decode_from_slice(&bytes, bincode::config::standard()).unwrap();
        assert_eq!(back, idx);
    }
}
