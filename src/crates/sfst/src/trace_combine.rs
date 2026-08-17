//! The shared trace combiner — ONE implementation of span dedup, canonical
//! ordering, capping, and forest building, used by every path that
//! assembles a trace: the single-file [`IndexReader::trace_by_id`]
//! (`index_reader`), the cross-source engine (`sfsq::traces`), and any
//! later consumer (compaction, cross-node fan-out).
//!
//! Contracts (pinned in the phase-4 design record):
//!
//! - Dedup key: `(span_id, kind)` — kind included because Zipkin-style
//!   shared client/server span ids are real. Valid within ONE trace: the
//!   combiner assembles exactly one trace per call. UNSET span ids NEVER
//!   collapse — those are distinct spans lacking a valid id, not resends.
//! - Total order and canonical-copy rule:
//!   `(start_ns, span_id, kind, content-hash, canonical-bytes)`. The
//!   canonical copy of unequal resends is the FIRST in that order —
//!   chronological-first, content-tiebroken — among SUCCESSFULLY
//!   materialized copies: if the true canonical copy's source fails, the
//!   next copy in order substitutes and the failure is reported
//!   (`failures`), so a consumer can treat `SourceFailure` as "the
//!   canonical copy may have been substituted". NO physical provenance
//!   participates, so results are independent of how spans scatter
//!   across sources.
//! - Canonical encoding: bincode standard encoding of the full
//!   [`TraceSpan`] in declared field order; hash = xxhash64, seed 0.
//!   Pre-release the encoding may change freely (nothing is released);
//!   it freezes at release.
//! - The merge consumes lightweight [`SpanRef`]s. What the cap bounds,
//!   precisely: RETAINED spans, and HELD span payloads. Every copy inside
//!   an equal-cheap-key group must MATERIALIZE once (the content
//!   comparison requires its bytes) — for a set id the selection streams,
//!   holding one candidate at a time (O(1) peak memory per group however
//!   many resends); an UNSET group is held whole, because ordering its
//!   distinct members content-deterministically needs them all — the one
//!   place peak memory is a group. Beyond-cap groups never materialize at
//!   all (the pre-group cap check). Examined *refs* are bounded by the
//!   trace's total row count across sources.
//! - Capping stops at the first unique span BEYOND the cap, which
//!   distinguishes an exactly-cap `Complete` trace from a truncated one.
//!
//! [`IndexReader::trace_by_id`]: crate::IndexReader::trace_by_id

use std::collections::{BinaryHeap, HashMap, HashSet};
use std::hash::Hasher;

use crate::index_reader::{Trace, TraceSpan};
use crate::{SpanId, TraceId};

/// The raw OTLP `SpanKind::SPAN_KIND_SERVER` value — the preferred parent
/// among a shared-id client/server pair (Zipkin shared-span semantics:
/// in-process children hang off the receiving side).
const SPAN_KIND_SERVER_RAW: i32 = 2;

/// A lightweight span candidate: everything the merge needs to order and
/// deduplicate WITHOUT materializing the span payload. `position` is
/// source-local (a file row position, or a tail scan's span index).
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct SpanRef {
    pub position: u32,
    pub start_ns: i64,
    pub span_id: SpanId,
    /// Raw OTLP span kind int (0 = UNSPECIFIED/absent) — part of the
    /// dedup key.
    pub kind: i32,
}

impl SpanRef {
    /// The cheap merge key — the comparator's prefix that needs no
    /// payload.
    fn cheap_key(&self) -> (i64, SpanId, i32) {
        (self.start_ns, self.span_id, self.kind)
    }
}

/// A source of one trace's spans for the combiner: cheap refs up front,
/// payloads on demand. Implemented by the SFST file session
/// ([`TraceFileSession`](crate::TraceFileSession)) and by the traces WAL
/// tail scan (in `sfsq`).
pub trait SpanSource {
    /// This source's candidates for `trace_id` (any order; the combiner
    /// sorts). An id absent from the source yields an empty vec.
    fn span_refs(&mut self, trace_id: TraceId) -> Result<Vec<SpanRef>, crate::Error>;

    /// Materialize the full span at a `position` previously returned by
    /// [`span_refs`](Self::span_refs).
    fn materialize(&mut self, position: u32) -> Result<TraceSpan, crate::Error>;
}

/// What [`combine`] produced.
pub struct CombineOutcome {
    pub trace: Trace,
    /// The span cap was reached AND at least one more unique span exists.
    pub truncated: bool,
    /// The merge was cancelled mid-way: the trace is the deterministic
    /// merged prefix assembled so far.
    pub cancelled: bool,
    /// Sources that failed (`span_refs` or `materialize`), by input index,
    /// with the error. A failed source's remaining candidates are dropped
    /// — the caller reports the failure (partial result), it is never
    /// silent.
    pub failures: Vec<(usize, crate::Error)>,
    /// Input indices of the sources whose spans were RETAINED in the
    /// trace (sorted, deduplicated) — the domain of any result metadata
    /// derived per source (e.g. the schema-kind map: it describes the
    /// returned data, not files that merely might contain the trace).
    pub contributing_sources: Vec<usize>,
}

/// The canonical encoding of a span — the content-derived identity the
/// total order's tiebreak uses. Bincode standard config over the full
/// struct in declared field order (deterministic: length-prefixed
/// strings/vectors, fields in order, no maps).
pub fn canonical_bytes(span: &TraceSpan) -> Vec<u8> {
    // Encoding plain owned data with bincode cannot fail (no I/O, no
    // maps, no floats); treat a failure as the impossibility it is.
    bincode::serde::encode_to_vec(span, bincode::config::standard())
        .expect("bincode encoding of plain span data cannot fail")
}

/// The content order two same-key copies resolve by: hash first (the
/// cheap comparator), full canonical bytes on a hash tie — so a genuine
/// xxhash64 collision cannot make unequal copies compare equal.
fn content_order(a: (u64, &[u8]), b: (u64, &[u8])) -> std::cmp::Ordering {
    (a.0, a.1).cmp(&(b.0, b.1))
}

/// xxhash64 (seed 0) of the canonical encoding.
pub fn content_hash(canonical: &[u8]) -> u64 {
    let mut h = twox_hash::XxHash64::with_seed(0);
    h.write(canonical);
    h.finish()
}

/// One heap entry: a ref plus where it came from. Ordered so the heap
/// pops the SMALLEST cheap key first (`Reverse` semantics inlined via
/// manual `Ord`), with `(source, index)` as an arbitrary-but-total
/// tiebreak for heap stability — group members are re-ordered
/// content-deterministically afterwards, so this physical tiebreak never
/// reaches the output.
#[derive(PartialEq, Eq)]
struct HeapEntry {
    key: (i64, SpanId, i32),
    source: usize,
    index: usize,
}

impl Ord for HeapEntry {
    fn cmp(&self, other: &Self) -> std::cmp::Ordering {
        // Reversed: BinaryHeap is a max-heap, we want the smallest key out
        // first.
        (other.key, other.source, other.index).cmp(&(self.key, self.source, self.index))
    }
}

impl PartialOrd for HeapEntry {
    fn partial_cmp(&self, other: &Self) -> Option<std::cmp::Ordering> {
        Some(self.cmp(other))
    }
}

/// Assemble ONE trace from many sources: k-way merge of [`SpanRef`]s in
/// the combiner's total order, `(span_id, kind)` dedup with the
/// chronological-first / content-tiebroken canonical rule, an optional
/// span cap, and the forest build. See the module docs for the pinned
/// contracts.
///
/// `cap = None` means unbounded; `cap = Some(0)` is a caller error the
/// engine rejects at its request boundary (asserted here in debug builds).
///
/// `cancel` is polled once per merge group; when it fires, the merge
/// stops and the outcome carries the deterministic prefix assembled so
/// far with [`cancelled`](CombineOutcome::cancelled) set (callers that
/// don't cancel pass `&|| false`).
pub fn combine(
    sources: &mut [&mut dyn SpanSource],
    trace_id: TraceId,
    cap: Option<usize>,
    cancel: &dyn Fn() -> bool,
) -> CombineOutcome {
    debug_assert!(cap != Some(0), "a zero span cap is rejected at the request boundary");

    let mut failures: Vec<(usize, crate::Error)> = Vec::new();

    // Cancellation during head resolution (this loop) is the "before all
    // source heads are known" phase: no deterministic prefix exists yet,
    // so the outcome is EMPTY + cancelled — polled up front (covers the
    // zero-source call) and before each source's refs.
    let cancelled_empty = |failures: Vec<(usize, crate::Error)>| CombineOutcome {
        trace: Trace {
            spans: Vec::new(),
            roots: Vec::new(),
            children: Vec::new(),
        },
        truncated: false,
        cancelled: true,
        failures,
        contributing_sources: Vec::new(),
    };
    if cancel() {
        return cancelled_empty(failures);
    }

    // Per-source candidate lists, sorted by the cheap key (+ position for
    // an in-source total order; same-position duplicates cannot exist).
    let mut refs: Vec<Vec<SpanRef>> = Vec::with_capacity(sources.len());
    for (i, source) in sources.iter_mut().enumerate() {
        if cancel() {
            return cancelled_empty(failures);
        }
        match source.span_refs(trace_id) {
            Ok(mut r) => {
                r.sort_unstable_by_key(|s| (s.cheap_key(), s.position));
                refs.push(r);
            }
            Err(e) => {
                failures.push((i, e));
                refs.push(Vec::new());
            }
        }
    }

    let mut heap: BinaryHeap<HeapEntry> = BinaryHeap::new();
    for (source, list) in refs.iter().enumerate() {
        if let Some(first) = list.first() {
            heap.push(HeapEntry {
                key: first.cheap_key(),
                source,
                index: 0,
            });
        }
    }
    let advance = |heap: &mut BinaryHeap<HeapEntry>, source: usize, index: usize| {
        if let Some(next) = refs_get(&refs, source, index + 1) {
            heap.push(HeapEntry {
                key: next.cheap_key(),
                source,
                index: index + 1,
            });
        }
    };

    let mut failed: Vec<bool> = vec![false; sources.len()];
    let mut seen: HashSet<(SpanId, i32)> = HashSet::new();
    let mut spans: Vec<TraceSpan> = Vec::new();
    let mut contributors: Vec<usize> = Vec::new();
    let mut truncated = false;
    let mut cancelled = false;

    'merge: while let Some(head) = heap.pop() {
        if cancel() {
            cancelled = true;
            break 'merge;
        }
        // Collect the whole equal-cheap-key group (same start, span_id,
        // kind — for a set span_id that is one dedup key's copies; for
        // UNSET it is a set of distinct id-less spans). A source can hold
        // SEVERAL consecutive equal-key rows (same-file resends): drain
        // them ALL into the group, so canonical selection compares every
        // copy wherever it lives — a later same-source copy skipping via
        // `seen` without a content comparison would make the choice
        // depend on physical layout.
        let mut pending: Vec<(usize, usize)> = vec![(head.source, head.index)];
        while let Some(peek) = heap.peek() {
            if peek.key == head.key {
                let e = heap.pop().expect("peeked entry exists");
                pending.push((e.source, e.index));
            } else {
                break;
            }
        }
        let mut group: Vec<(usize, usize)> = Vec::with_capacity(pending.len());
        for (source, first) in pending {
            let mut index = first;
            group.push((source, index));
            while let Some(next) = refs_get(&refs, source, index + 1) {
                if next.cheap_key() == head.key {
                    index += 1;
                    group.push((source, index));
                } else {
                    break;
                }
            }
            advance(&mut heap, source, index);
        }
        group.retain(|&(source, _)| !failed[source]);
        if group.is_empty() {
            continue;
        }

        let (_, span_id, kind) = head.key;
        let is_unset = span_id.is_unset();

        // Dedup check BEFORE any materialization: a later chronological
        // copy of an already-canonical key skips as pure ref work.
        if !is_unset && seen.contains(&(span_id, kind)) {
            continue;
        }

        // Cap check BEFORE materialization: accepting anything from this
        // group would exceed the cap, so we have proven a unique span
        // beyond it — the result is truncated, and the beyond-cap payload
        // is never built.
        if let Some(n) = cap
            && spans.len() >= n
        {
            truncated = true;
            break 'merge;
        }

        // Materialize the group. A single set-id member needs no encoding
        // at all — it IS the canonical copy.
        if group.len() == 1 && !is_unset {
            let (source, index) = group[0];
            let r = refs[source][index];
            match sources[source].materialize(r.position) {
                Ok(span) => {
                    seen.insert((span_id, kind));
                    spans.push(span);
                    contributors.push(source);
                }
                Err(e) => {
                    failed[source] = true;
                    failures.push((source, e));
                }
            }
            continue;
        }

        if !is_unset {
            // Copies of one dedup key: STREAM the canonical selection —
            // each copy materializes once (the content comparison
            // requires it) but only the current minimum is HELD, so a
            // large resend group costs O(1) peak span memory, not
            // O(group). Every copy BYTE-EQUAL to the canonical
            // contributed the same span, so all of their sources count as
            // contributors — otherwise result metadata derived per source
            // (the schema-kind map) would depend on which tied source
            // happened to be visited first (typed values can render
            // identically, e.g. Int 1 and Str "1", while their schema
            // kinds differ).
            let mut best: Option<(u64, Vec<u8>, TraceSpan)> = None;
            let mut tied_sources: Vec<usize> = Vec::new();
            for (source, index) in group {
                if failed[source] {
                    continue;
                }
                let r = refs[source][index];
                match sources[source].materialize(r.position) {
                    Ok(span) => {
                        let bytes = canonical_bytes(&span);
                        let hash = content_hash(&bytes);
                        match &best {
                            None => {
                                best = Some((hash, bytes, span));
                                tied_sources = vec![source];
                            }
                            Some((best_hash, best_bytes, _)) => {
                                match content_order((hash, &bytes), (*best_hash, best_bytes)) {
                                    std::cmp::Ordering::Less => {
                                        best = Some((hash, bytes, span));
                                        tied_sources = vec![source];
                                    }
                                    std::cmp::Ordering::Equal => tied_sources.push(source),
                                    std::cmp::Ordering::Greater => {}
                                }
                            }
                        }
                    }
                    Err(e) => {
                        failed[source] = true;
                        failures.push((source, e));
                    }
                }
            }
            if let Some((_, _, span)) = best {
                seen.insert((span_id, kind));
                spans.push(span);
                contributors.append(&mut tied_sources);
            }
            continue;
        }

        // UNSET ids never collapse: every member is a DISTINCT span, and
        // ordering them content-deterministically requires holding the
        // whole group — the one place peak memory is a group, not a span.
        let mut members: Vec<(u64, Vec<u8>, TraceSpan, usize)> = Vec::with_capacity(group.len());
        for (source, index) in group {
            if failed[source] {
                continue;
            }
            let r = refs[source][index];
            match sources[source].materialize(r.position) {
                Ok(span) => {
                    let bytes = canonical_bytes(&span);
                    members.push((content_hash(&bytes), bytes, span, source));
                }
                Err(e) => {
                    failed[source] = true;
                    failures.push((source, e));
                }
            }
        }
        members.sort_by(|a, b| content_order((a.0, &a.1), (b.0, &b.1)));
        for (_, _, span, source) in members {
            if let Some(n) = cap
                && spans.len() >= n
            {
                truncated = true;
                break 'merge;
            }
            spans.push(span);
            contributors.push(source);
        }
    }

    contributors.sort_unstable();
    contributors.dedup();
    let (roots, children) = build_forest(&spans);
    CombineOutcome {
        trace: Trace {
            spans,
            roots,
            children,
        },
        truncated,
        cancelled,
        failures,
        contributing_sources: contributors,
    }
}

/// `refs[source].get(index)` without borrowing pain in the closure.
fn refs_get(refs: &[Vec<SpanRef>], source: usize, index: usize) -> Option<&SpanRef> {
    refs.get(source).and_then(|list| list.get(index))
}

/// Build the trace graph over combiner-ordered spans: node-index
/// adjacency, the pinned shared-id parent rule, and the reachability
/// guarantee.
///
/// - A span whose `parent_span_id` is unset, self-referential, or absent
///   from the set is a GRAPH root — a partial trace forms a forest, never
///   dropped spans.
/// - A parent id matching several spans (a shared client/server-id pair)
///   resolves to the SERVER-kind candidate, else the earliest (the spans
///   are already in the combiner's total order, so "earliest" = lowest
///   index).
/// - Every span is reachable from some root even for pathological input:
///   a parent cycle with no external entry has no natural root, so the
///   earliest still-unreached span is promoted until all are reachable
///   (`children` still contains the cycle's edges — walkers must guard
///   against revisits).
pub fn build_forest(spans: &[TraceSpan]) -> (Vec<usize>, Vec<Vec<usize>>) {
    // Parent-candidate index: span_id → indices carrying it, in span
    // (total) order. UNSET ids are never parent candidates.
    let mut candidates: HashMap<SpanId, Vec<usize>> = HashMap::new();
    for (i, s) in spans.iter().enumerate() {
        if !s.span_id.is_unset() {
            candidates.entry(s.span_id).or_default().push(i);
        }
    }

    let mut children: Vec<Vec<usize>> = vec![Vec::new(); spans.len()];
    let mut roots: Vec<usize> = Vec::new();
    for (i, s) in spans.iter().enumerate() {
        let parent = if s.parent_span_id.is_unset() || s.parent_span_id == s.span_id {
            None
        } else {
            candidates.get(&s.parent_span_id).map(|cands| {
                // SERVER-kind preferred; else earliest in total order.
                cands
                    .iter()
                    .copied()
                    .find(|&c| spans[c].kind == SPAN_KIND_SERVER_RAW)
                    .unwrap_or(cands[0])
            })
        };
        match parent {
            Some(p) => children[p].push(i),
            None => roots.push(i),
        }
    }

    // Reachability guard: promote the earliest unreached span to a root
    // until a root-first walk covers everything. O(V+E) — each node marked
    // once, each edge followed once, `cursor` scans monotonically.
    let mut reachable = vec![false; spans.len()];
    let mut stack: Vec<usize> = roots.clone();
    let mut cursor = 0usize;
    loop {
        while let Some(i) = stack.pop() {
            if reachable[i] {
                continue;
            }
            reachable[i] = true;
            stack.extend(children[i].iter().copied().filter(|&c| !reachable[c]));
        }
        while cursor < spans.len() && reachable[cursor] {
            cursor += 1;
        }
        if cursor == spans.len() {
            break;
        }
        roots.push(cursor); // earliest (total-order) unreached span
        stack.push(cursor);
    }

    (roots, children)
}

#[cfg(test)]
mod tests {
    use super::*;

    fn span(span_id: [u8; 8], parent: [u8; 8], start: i64, kind: i32, name: &str) -> TraceSpan {
        TraceSpan {
            span_id: SpanId::from(span_id),
            parent_span_id: SpanId::from(parent),
            start_ns: start,
            duration_ns: 10,
            kind,
            flags: 0,
            dropped_attributes_count: 0,
            dropped_events_count: 0,
            dropped_links_count: 0,
            fields: vec![("name".into(), name.into())],
            events: Vec::new(),
            links: Vec::new(),
        }
    }

    /// An in-memory SpanSource over a plain span list (positions are
    /// indices), with an optional injected failure.
    struct VecSource {
        spans: Vec<TraceSpan>,
        fail_at: Option<u32>,
    }

    impl VecSource {
        fn new(spans: Vec<TraceSpan>) -> Self {
            Self {
                spans,
                fail_at: None,
            }
        }
    }

    impl SpanSource for VecSource {
        fn span_refs(&mut self, _trace_id: TraceId) -> Result<Vec<SpanRef>, crate::Error> {
            Ok(self
                .spans
                .iter()
                .enumerate()
                .map(|(i, s)| SpanRef {
                    position: i as u32,
                    start_ns: s.start_ns,
                    span_id: s.span_id,
                    kind: s.kind,
                })
                .collect())
        }

        fn materialize(&mut self, position: u32) -> Result<TraceSpan, crate::Error> {
            if self.fail_at == Some(position) {
                return Err(crate::Error::CorruptIndex("injected failure".into()));
            }
            Ok(self.spans[position as usize].clone())
        }
    }

    fn tid() -> TraceId {
        TraceId::from([7u8; 16])
    }

    fn run(sources: &mut [&mut dyn SpanSource], cap: Option<usize>) -> CombineOutcome {
        combine(sources, tid(), cap, &|| false)
    }

    #[test]
    fn resends_collapse_to_the_chronological_first_copy_across_sources() {
        // The same (span_id, kind) at start 100 (source B) and start 5
        // (source A, a conflicting resend with a different name). The
        // chronological-first copy wins regardless of source order.
        let early = span([1; 8], [0; 8], 5, 3, "early-copy");
        let late = span([1; 8], [0; 8], 100, 3, "late-copy");
        for flip in [false, true] {
            let mut a = VecSource::new(vec![early.clone()]);
            let mut b = VecSource::new(vec![late.clone()]);
            let mut set: Vec<&mut dyn SpanSource> = if flip {
                vec![&mut b, &mut a]
            } else {
                vec![&mut a, &mut b]
            };
            let out = run(&mut set, None);
            assert!(out.failures.is_empty());
            assert_eq!(out.trace.spans.len(), 1);
            assert_eq!(out.trace.spans[0].fields[0].1, "early-copy");
        }
    }

    #[test]
    fn equal_start_conflicting_resends_pick_content_deterministically() {
        // Same key, same start, different payloads: the (hash, bytes)
        // order decides, so both source orders agree.
        let one = span([2; 8], [0; 8], 50, 1, "alpha");
        let two = span([2; 8], [0; 8], 50, 1, "beta");
        let pick = |first: &TraceSpan, second: &TraceSpan| {
            let mut a = VecSource::new(vec![first.clone()]);
            let mut b = VecSource::new(vec![second.clone()]);
            let mut set: Vec<&mut dyn SpanSource> = vec![&mut a, &mut b];
            run(&mut set, None).trace.spans[0].clone()
        };
        assert_eq!(pick(&one, &two), pick(&two, &one));
    }

    #[test]
    fn shared_client_server_pair_survives_and_parents_via_server() {
        // CLIENT and SERVER share span id X; a child references X as its
        // parent → both pair members survive dedup (kind is in the key)
        // and the child attaches to the SERVER-kind node.
        let client = span([3; 8], [0; 8], 10, 3, "client");
        let server = span([3; 8], [0; 8], 12, 2, "server");
        let child = span([4; 8], [3; 8], 15, 1, "child");
        let mut a = VecSource::new(vec![client, server, child]);
        let mut set: Vec<&mut dyn SpanSource> = vec![&mut a];
        let out = run(&mut set, None);
        assert_eq!(out.trace.spans.len(), 3);
        let server_idx = out
            .trace
            .spans
            .iter()
            .position(|s| s.kind == 2)
            .expect("server span present");
        let child_idx = out
            .trace
            .spans
            .iter()
            .position(|s| s.fields[0].1 == "child")
            .unwrap();
        assert_eq!(out.trace.children[server_idx], vec![child_idx]);
    }

    #[test]
    fn unset_span_ids_never_collapse_even_when_byte_equal() {
        let ghost = span([0; 8], [0; 8], 20, 1, "ghost");
        let mut a = VecSource::new(vec![ghost.clone()]);
        let mut b = VecSource::new(vec![ghost.clone()]);
        let mut set: Vec<&mut dyn SpanSource> = vec![&mut a, &mut b];
        let out = run(&mut set, None);
        // Distinct spans that merely lack ids — kept, both roots.
        assert_eq!(out.trace.spans.len(), 2);
        assert_eq!(out.trace.roots.len(), 2);
    }

    #[test]
    fn cap_keeps_globally_earliest_and_detects_truncation_exactly() {
        // Source A holds LATER spans, source B holds earlier ones; the
        // cap must keep the globally earliest regardless.
        let mut a = VecSource::new(vec![
            span([10; 8], [0; 8], 300, 1, "late-1"),
            span([11; 8], [0; 8], 400, 1, "late-2"),
        ]);
        let mut b = VecSource::new(vec![
            span([12; 8], [0; 8], 1, 1, "early-1"),
            span([13; 8], [0; 8], 2, 1, "early-2"),
        ]);
        let mut set: Vec<&mut dyn SpanSource> = vec![&mut a, &mut b];
        let out = run(&mut set, Some(2));
        assert!(out.truncated);
        let names: Vec<&str> = out
            .trace
            .spans
            .iter()
            .map(|s| s.fields[0].1.as_str())
            .collect();
        assert_eq!(names, ["early-1", "early-2"]);

        // Exactly-cap: 2 unique spans, cap 2 → Complete (not truncated) —
        // the cap+1 rule requires a unique span BEYOND the cap.
        let mut b2 = VecSource::new(vec![
            span([12; 8], [0; 8], 1, 1, "early-1"),
            span([13; 8], [0; 8], 2, 1, "early-2"),
        ]);
        let mut set2: Vec<&mut dyn SpanSource> = vec![&mut b2];
        let out2 = run(&mut set2, Some(2));
        assert!(!out2.truncated);
        assert_eq!(out2.trace.spans.len(), 2);

        // Duplicate-only rows beyond the boundary do NOT truncate: the
        // third candidate is a resend of an already-canonical key.
        let mut c = VecSource::new(vec![
            span([12; 8], [0; 8], 1, 1, "early-1"),
            span([13; 8], [0; 8], 2, 1, "early-2"),
            span([13; 8], [0; 8], 900, 1, "resend-of-early-2"),
        ]);
        let mut set3: Vec<&mut dyn SpanSource> = vec![&mut c];
        let out3 = run(&mut set3, Some(2));
        assert!(!out3.truncated);
        assert_eq!(out3.trace.spans.len(), 2);
    }

    #[test]
    fn same_source_equal_key_conflicts_compare_content_like_split_sources() {
        // Two UNEQUAL copies with the SAME cheap key inside ONE source
        // must pick the same canonical copy as when they are split across
        // two sources — the layout-independence contract. (Regression for
        // the group-drain fix: a same-source second copy used to skip via
        // `seen` without a content comparison.)
        let one = span([2; 8], [0; 8], 50, 1, "alpha");
        let two = span([2; 8], [0; 8], 50, 1, "beta");

        let mut same = VecSource::new(vec![one.clone(), two.clone()]);
        let mut set: Vec<&mut dyn SpanSource> = vec![&mut same];
        let from_same = run(&mut set, None).trace.spans[0].clone();

        let mut a = VecSource::new(vec![one.clone()]);
        let mut b = VecSource::new(vec![two.clone()]);
        let mut split: Vec<&mut dyn SpanSource> = vec![&mut a, &mut b];
        let from_split = run(&mut split, None).trace.spans[0].clone();

        assert_eq!(from_same, from_split);

        // And with the copies in the opposite same-source order.
        let mut flipped = VecSource::new(vec![two, one]);
        let mut set2: Vec<&mut dyn SpanSource> = vec![&mut flipped];
        assert_eq!(run(&mut set2, None).trace.spans[0].clone(), from_split);
    }

    #[test]
    fn byte_equal_copies_credit_every_tied_source_as_contributor() {
        // Two sources holding a byte-equal copy of one span: BOTH are
        // contributors (result metadata like the schema-kind map must not
        // depend on which tied source sorts first).
        let copy = span([6; 8], [0; 8], 30, 1, "same");
        let mut a = VecSource::new(vec![copy.clone()]);
        let mut b = VecSource::new(vec![copy.clone()]);
        let mut set: Vec<&mut dyn SpanSource> = vec![&mut a, &mut b];
        let out = run(&mut set, None);
        assert_eq!(out.trace.spans.len(), 1);
        assert_eq!(out.contributing_sources, vec![0, 1]);
    }

    #[test]
    fn content_order_falls_back_to_bytes_on_a_hash_tie() {
        // A fabricated hash collision: equal hashes, different bytes —
        // the full-bytes comparison decides, so unequal copies can never
        // compare equal (the collision-safety half of the comparator).
        let a = (42u64, vec![1u8, 2, 3]);
        let b = (42u64, vec![1u8, 2, 4]);
        assert_eq!(content_order((a.0, &a.1), (b.0, &b.1)), std::cmp::Ordering::Less);
        assert_eq!(content_order((b.0, &b.1), (a.0, &a.1)), std::cmp::Ordering::Greater);
        assert_eq!(content_order((a.0, &a.1), (a.0, &a.1)), std::cmp::Ordering::Equal);
    }

    #[test]
    fn unset_group_cap_waste_is_bounded_to_one_group() {
        // The documented exception: an UNSET equal-key group materializes
        // wholesale (content-deterministic ordering needs every member's
        // bytes) — but NOTHING beyond that group materializes once the
        // cap is hit. Group A (2 UNSET spans at ts=1) + group B (2 UNSET
        // at ts=2), cap 1: A's 2 members materialize, B's never do.
        struct Counting {
            inner: VecSource,
            materialized: std::rc::Rc<std::cell::Cell<usize>>,
        }
        impl SpanSource for Counting {
            fn span_refs(&mut self, id: TraceId) -> Result<Vec<SpanRef>, crate::Error> {
                self.inner.span_refs(id)
            }
            fn materialize(&mut self, position: u32) -> Result<TraceSpan, crate::Error> {
                self.materialized.set(self.materialized.get() + 1);
                self.inner.materialize(position)
            }
        }
        let count = std::rc::Rc::new(std::cell::Cell::new(0));
        let mut src = Counting {
            inner: VecSource::new(vec![
                span([0; 8], [0; 8], 1, 1, "a1"),
                span([0; 8], [0; 8], 1, 1, "a2"),
                span([0; 8], [0; 8], 2, 1, "b1"),
                span([0; 8], [0; 8], 2, 1, "b2"),
            ]),
            materialized: std::rc::Rc::clone(&count),
        };
        let mut set: Vec<&mut dyn SpanSource> = vec![&mut src];
        let out = run(&mut set, Some(1));
        assert!(out.truncated);
        assert_eq!(out.trace.spans.len(), 1);
        assert_eq!(count.get(), 2, "only the first UNSET group materialized");
    }

    #[test]
    fn beyond_cap_payloads_are_never_materialized() {
        // The cap bounds WORK, not just output: with cap 1 and three
        // unique candidates, exactly ONE payload materializes (the
        // accepted span); the beyond-cap candidate that proves truncation
        // is detected on its cheap ref alone.
        struct Counting {
            inner: VecSource,
            materialized: std::rc::Rc<std::cell::Cell<usize>>,
        }
        impl SpanSource for Counting {
            fn span_refs(&mut self, id: TraceId) -> Result<Vec<SpanRef>, crate::Error> {
                self.inner.span_refs(id)
            }
            fn materialize(&mut self, position: u32) -> Result<TraceSpan, crate::Error> {
                self.materialized.set(self.materialized.get() + 1);
                self.inner.materialize(position)
            }
        }
        let count = std::rc::Rc::new(std::cell::Cell::new(0));
        let mut src = Counting {
            inner: VecSource::new(vec![
                span([1; 8], [0; 8], 1, 1, "kept"),
                span([2; 8], [0; 8], 2, 1, "beyond-cap"),
                span([3; 8], [0; 8], 3, 1, "beyond-cap-2"),
            ]),
            materialized: std::rc::Rc::clone(&count),
        };
        let mut set: Vec<&mut dyn SpanSource> = vec![&mut src];
        let out = run(&mut set, Some(1));
        assert!(out.truncated);
        assert_eq!(out.trace.spans.len(), 1);
        assert_eq!(count.get(), 1, "only the accepted span materialized");
    }

    #[test]
    fn cancellation_semantics_split_by_phase() {
        let make = || {
            VecSource::new(vec![
                span([1; 8], [0; 8], 1, 1, "first"),
                span([2; 8], [0; 8], 2, 1, "second"),
            ])
        };

        // Cancel DURING head resolution (poll 2 = the per-source refs
        // poll, after the entry poll): no deterministic prefix exists —
        // EMPTY + cancelled.
        let mut src = make();
        let mut set: Vec<&mut dyn SpanSource> = vec![&mut src];
        let polls = std::cell::Cell::new(0usize);
        let cancel = || {
            polls.set(polls.get() + 1);
            polls.get() >= 2
        };
        let out = combine(&mut set, tid(), None, &cancel);
        assert!(out.cancelled);
        assert!(out.trace.spans.is_empty());

        // Cancel DURING the merge (after group 1: polls = entry, refs,
        // group 1, group 2): the deterministic PREFIX survives.
        let mut src = make();
        let mut set: Vec<&mut dyn SpanSource> = vec![&mut src];
        let polls = std::cell::Cell::new(0usize);
        let cancel = || {
            polls.set(polls.get() + 1);
            polls.get() >= 4
        };
        let out = combine(&mut set, tid(), None, &cancel);
        assert!(out.cancelled);
        assert_eq!(out.trace.spans.len(), 1);
        assert_eq!(out.trace.spans[0].fields[0].1, "first");

        // Cancelled with ZERO sources: still cancelled, never Complete.
        let mut empty: Vec<&mut dyn SpanSource> = vec![];
        let out = combine(&mut empty, tid(), None, &|| true);
        assert!(out.cancelled);
    }

    #[test]
    fn a_failing_source_is_reported_and_the_rest_still_combine() {
        let mut good = VecSource::new(vec![span([20; 8], [0; 8], 1, 1, "good")]);
        let mut bad = VecSource::new(vec![span([21; 8], [0; 8], 2, 1, "bad")]);
        bad.fail_at = Some(0);
        let mut set: Vec<&mut dyn SpanSource> = vec![&mut good, &mut bad];
        let out = run(&mut set, None);
        assert_eq!(out.trace.spans.len(), 1);
        assert_eq!(out.trace.spans[0].fields[0].1, "good");
        assert_eq!(out.failures.len(), 1);
        assert_eq!(out.failures[0].0, 1);
    }

    #[test]
    fn forest_reaches_disjoint_cycles_beside_a_true_root() {
        // R -> C rooted pair + X <-> Y parent cycle: the cycle members
        // must still be reachable (the graph-roots contract; summary-root
        // policy is separate).
        let r = span([1; 8], [0; 8], 100, 1, "root");
        let c = span([2; 8], [1; 8], 110, 1, "child");
        let x = span([3; 8], [4; 8], 120, 1, "x");
        let y = span([4; 8], [3; 8], 130, 1, "y");
        let mut a = VecSource::new(vec![r, c, x, y]);
        let mut set: Vec<&mut dyn SpanSource> = vec![&mut a];
        let out = run(&mut set, None);
        let t = &out.trace;
        assert_eq!(t.spans.len(), 4);
        // Roots: the true root + the promoted cycle entry.
        assert_eq!(t.roots.len(), 2);
        // Every span reachable from some root, revisit-guarded.
        let mut seen = vec![false; t.spans.len()];
        let mut stack = t.roots.clone();
        while let Some(i) = stack.pop() {
            if std::mem::replace(&mut seen[i], true) {
                continue;
            }
            stack.extend(t.children[i].iter().copied().filter(|&k| !seen[k]));
        }
        assert!(seen.iter().all(|&s| s));
    }

    #[test]
    fn summary_root_prefers_true_root_over_earlier_orphan() {
        // An orphan (missing parent) starts EARLIER than the true
        // (unset-parent) root: graph roots include both; the summary root
        // is the true root.
        let orphan = span([5; 8], [9; 8], 1, 1, "orphan"); // parent [9;8] absent
        let root = span([6; 8], [0; 8], 50, 1, "true-root");
        let mut a = VecSource::new(vec![orphan, root]);
        let mut set: Vec<&mut dyn SpanSource> = vec![&mut a];
        let out = run(&mut set, None);
        assert_eq!(out.trace.roots.len(), 2);
        let summary = out.trace.summary_root().expect("non-empty trace");
        assert_eq!(out.trace.spans[summary].fields[0].1, "true-root");

        // With no true root anywhere, the earliest graph root wins.
        let o1 = span([5; 8], [9; 8], 1, 1, "orphan-early");
        let o2 = span([6; 8], [9; 8], 50, 1, "orphan-late");
        let mut b = VecSource::new(vec![o2, o1]);
        let mut set2: Vec<&mut dyn SpanSource> = vec![&mut b];
        let out2 = run(&mut set2, None);
        let summary2 = out2.trace.summary_root().unwrap();
        assert_eq!(out2.trace.spans[summary2].fields[0].1, "orphan-early");
    }

    #[test]
    fn canonical_encoding_is_sensitive_to_every_field() {
        // Mutating any TraceSpan field must change the canonical bytes —
        // the encoding is the identity unequal resends are ordered by.
        let base = TraceSpan {
            span_id: SpanId::from([1; 8]),
            parent_span_id: SpanId::from([2; 8]),
            start_ns: 100,
            duration_ns: 10,
            kind: 2,
            flags: 1,
            dropped_attributes_count: 1,
            dropped_events_count: 1,
            dropped_links_count: 1,
            fields: vec![("name".into(), "n".into())],
            events: vec![crate::TraceEvent {
                time_unix_nano: 5,
                name: "e".into(),
                dropped_attributes_count: 0,
                attributes: vec![("k".into(), "v".into())],
            }],
            links: vec![crate::TraceLink {
                trace_id: TraceId::from([3; 16]),
                span_id: SpanId::from([4; 8]),
                trace_state: "ot=th:8".into(),
                flags: 0,
                dropped_attributes_count: 0,
                attributes: Vec::new(),
            }],
        };
        let reference = canonical_bytes(&base);
        let mutations: Vec<TraceSpan> = vec![
            TraceSpan { span_id: SpanId::from([9; 8]), ..base.clone() },
            TraceSpan { parent_span_id: SpanId::from([9; 8]), ..base.clone() },
            TraceSpan { start_ns: 101, ..base.clone() },
            TraceSpan { duration_ns: 11, ..base.clone() },
            TraceSpan { kind: 3, ..base.clone() },
            TraceSpan { flags: 2, ..base.clone() },
            TraceSpan { dropped_attributes_count: 2, ..base.clone() },
            TraceSpan { dropped_events_count: 2, ..base.clone() },
            TraceSpan { dropped_links_count: 2, ..base.clone() },
            TraceSpan { fields: vec![("name".into(), "m".into())], ..base.clone() },
            TraceSpan { events: Vec::new(), ..base.clone() },
            TraceSpan { links: Vec::new(), ..base.clone() },
        ];
        for (i, m) in mutations.iter().enumerate() {
            assert_ne!(
                canonical_bytes(m),
                reference,
                "mutation {i} did not change the canonical encoding"
            );
        }
        // And equal spans encode equal (the dedup fast path's premise).
        assert_eq!(canonical_bytes(&base.clone()), reference);
    }
}
