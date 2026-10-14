//! Tests for [`IndexReader::trace_by_id`], the single-file entry into the
//! shared trace combiner (`trace_combine.rs`, driven through the per-file
//! session in session.rs) — exercised end to end over in-memory SFST files
//! this module builds with the crate's own writer (see `trace_file`).
//!
//! Pins:
//!
//! - a corrupt `TBLM` bloom degrades to the exact `TIDX` lookup instead of
//!   hiding a findable trace; the raw accessor still surfaces the error;
//! - an id absent from the file resolves to the empty trace, not an error;
//! - dedup: duplicate `(span_id, kind)` rows collapse to one span, UNSET
//!   span ids never do;
//! - graph: unset or absent-from-set parents are roots; `children` is
//!   node-index adjacency parallel to `spans`;
//! - a parent cycle with no external entry stays a walkable forest: a
//!   root is promoted, and walkers must guard revisits (`children` keeps
//!   the cycle edges).
//!
//! Not pinned here: attribute facets, events, and links materialization —
//! fixture files carry empty attribute fields on purpose.

use crate::writer::{ChunkCounts, ChunkWriter, ColumnsPresent};
use crate::{
    BitmapValue, ColumnEntry, ColumnsTable, DroppedAttributeCounts, Durations, Flags, Histogram,
    IdRanges, IndexReader, KvId, Metadata, ParentSpanIds, SpanId, SpanIds, StreamBatch, Summary,
    TraceId, TraceIdIndex, TraceIds,
};

const TRACE: [u8; 16] = [7u8; 16];

fn sid(b: u8) -> SpanId {
    SpanId::from([b; 8])
}

/// Build a minimal traces SFST whose rows all share one `trace_id`, from
/// `(span_id, parent_span_id)` pairs given in row order (ascending
/// timestamps are assigned, so row order == start order). Attribute
/// fields stay empty — the tests exercise ids, parents, and row scalars,
/// not facets.
fn trace_file(rows: &[(SpanId, SpanId)]) -> Vec<u8> {
    trace_file_with_bloom(rows, None)
}

/// Like [`trace_file`], optionally writing a caller-supplied `TBLM`
/// (trace-id bloom) payload — which may be deliberately malformed: the
/// writer packs it as-is, validation happens on read.
fn trace_file_with_bloom(
    rows: &[(SpanId, SpanId)],
    bloom: Option<&crate::TraceIdBloom>,
) -> Vec<u8> {
    let n = rows.len();
    let mut trace = TraceIds::with_capacity(n);
    let mut span = SpanIds::with_capacity(n);
    let mut parent = ParentSpanIds::with_capacity(n);
    for &(s, p) in rows {
        trace.push(TraceId::from(TRACE));
        span.push(s);
        parent.push(p);
    }
    let flags = Flags(vec![0u32; n]);
    let drac = DroppedAttributeCounts(vec![0u32; n]);
    let durations = Durations(vec![0i64; n]);
    let index = TraceIdIndex::build(&trace);

    let columns = ColumnsPresent {
        observed_ts: false,
        trace_id: true,
        span_id: true,
        flags: true,
        dropped_attributes_count: true,
        parent_span_id: true,
        duration: true,
    };
    let counts = ChunkCounts {
        columns,
        trace_id_index: true,
        trace_id_bloom: bloom.is_some(),
        event_index: false,
        link_index: false,
        trace_rollup: false,
        trace_rollup_stats: false,
        mid_fields: 0,
        high_fields: 0,
        stream_batches: 1,
    };
    let summary = Summary {
        min_timestamp_s: 0,
        max_timestamp_s: n as u32,
        record_count: n as u32,
        content_meta: Vec::new(),
    };
    let metadata = Metadata {
        histogram: Histogram {
            timestamps: vec![0],
            counts: vec![n as u32],
        },
        id_ranges: IdRanges {
            low_end: KvId(0),
            mid_end: KvId(0),
            high_end: KvId(0),
        },
        tree: Default::default(),
        columns: ColumnsTable(vec![
            ColumnEntry {
                name: TraceIds::NAME.into(),
                ty: TraceIds::COLUMN_TYPE,
            },
            ColumnEntry {
                name: SpanIds::NAME.into(),
                ty: SpanIds::COLUMN_TYPE,
            },
            ColumnEntry {
                name: Flags::NAME.into(),
                ty: Flags::COLUMN_TYPE,
            },
            ColumnEntry {
                name: DroppedAttributeCounts::NAME.into(),
                ty: DroppedAttributeCounts::COLUMN_TYPE,
            },
            ColumnEntry {
                name: ParentSpanIds::NAME.into(),
                ty: ParentSpanIds::COLUMN_TYPE,
            },
            ColumnEntry {
                name: Durations::NAME.into(),
                ty: Durations::COLUMN_TYPE,
            },
        ]),
    };
    // One ascending timestamp per row so start-sort order == row order.
    let timestamps: Vec<i64> = (0..n as i64).collect();

    let mut w = ChunkWriter::new(std::io::Cursor::new(Vec::new()), counts).unwrap();
    w.summary(&summary).unwrap();
    w.metadata(&metadata).unwrap();
    w.timestamps(&timestamps).unwrap();
    w.primary(std::iter::empty::<(&str, BitmapValue)>())
        .unwrap();
    w.trace_ids(&trace).unwrap();
    w.span_ids(&span).unwrap();
    w.flags(&flags).unwrap();
    w.dropped_attribute_counts(&drac).unwrap();
    w.parent_span_ids(&parent).unwrap();
    w.durations(&durations).unwrap();
    w.trace_id_index(&index).unwrap();
    if let Some(b) = bloom {
        w.trace_id_bloom(b).unwrap();
    }
    w.add_stream_batch(&StreamBatch::for_write(&vec![Vec::<KvId>::new(); n]))
        .unwrap();
    w.finish().unwrap().into_inner()
}

#[test]
fn corrupt_bloom_degrades_to_the_exact_lookup() {
    // The bloom is a skip hint: a TBLM that decodes but fails validation
    // (absurd hash count here) must NOT make a findable trace unfindable —
    // trace_by_id falls through to TIDX. The accessor itself still errors,
    // so cross-file callers can observe the corruption.
    let hostile = crate::TraceIdBloom::raw_for_tests(
        1,
        fastbloom::BloomFilter::from_vec(vec![0u64; 4])
            .seed(&1)
            .hashes(1_000),
    );
    let buf = trace_file_with_bloom(&[(sid(1), SpanId::from([0; 8]))], Some(&hostile));
    let reader = IndexReader::open(&buf).unwrap();

    assert!(reader.has_trace_id_bloom());
    assert!(
        reader.trace_id_bloom().is_err(),
        "accessor surfaces corruption"
    );
    let trace = reader.trace_by_id(TraceId::from(TRACE)).unwrap();
    assert_eq!(trace.spans.len(), 1, "lookup degraded to TIDX and resolved");
}

#[test]
fn absent_trace_yields_empty() {
    // An id the TIDX doesn't carry is not an error: trace_by_id resolves
    // it to the empty trace (no spans, no roots). No bloom in this
    // fixture, so the miss comes from the exact lookup.
    let buf = trace_file(&[(sid(1), SpanId::from([0; 8]))]);
    let reader = IndexReader::open(&buf).unwrap();
    let trace = reader.trace_by_id(TraceId::from([0xEE; 16])).unwrap();
    assert!(trace.spans.is_empty());
    assert!(trace.roots.is_empty());
}

#[test]
fn duplicate_span_id_is_collapsed_to_first() {
    let unset = SpanId::from([0; 8]);
    // span A sent twice (a resend), then span B — all roots (unset parents).
    let buf = trace_file(&[(sid(1), unset), (sid(1), unset), (sid(2), unset)]);
    let reader = IndexReader::open(&buf).unwrap();
    let trace = reader.trace_by_id(TraceId::from(TRACE)).unwrap();

    // Both A rows share the dedup key (span_id, kind) → one span; B is a
    // distinct key.
    assert_eq!(trace.spans.len(), 2);
    assert_eq!(trace.roots.len(), 2);
    // `children` is node-index adjacency parallel to `spans`.
    assert!(trace.children.iter().all(|kids| kids.is_empty()));
}

#[test]
fn unset_span_ids_are_not_collapsed() {
    let unset = SpanId::from([0; 8]);
    // Two rows both with an UNSET span_id are distinct spans, not one resend.
    let buf = trace_file(&[(unset, unset), (unset, unset)]);
    let reader = IndexReader::open(&buf).unwrap();
    let trace = reader.trace_by_id(TraceId::from(TRACE)).unwrap();
    assert_eq!(trace.spans.len(), 2);
    assert_eq!(trace.roots.len(), 2);
}

#[test]
fn parent_edges_and_missing_parent_root() {
    let unset = SpanId::from([0; 8]);
    // A (unset parent → root), B (child of A), C (parent X not in this
    // file's span set → root: a partial trace still forms a forest).
    let buf = trace_file(&[(sid(1), unset), (sid(2), sid(1)), (sid(3), sid(9))]);
    let reader = IndexReader::open(&buf).unwrap();
    let trace = reader.trace_by_id(TraceId::from(TRACE)).unwrap();

    assert_eq!(trace.spans.len(), 3);
    assert_eq!(trace.roots.len(), 2);
    let a_idx = trace
        .spans
        .iter()
        .position(|s| s.span_id == sid(1))
        .unwrap();
    let a_children = &trace.children[a_idx];
    assert_eq!(a_children.len(), 1);
    assert_eq!(trace.spans[a_children[0]].span_id, sid(2));
}

#[test]
fn parent_cycle_stays_a_forest_and_terminates() {
    // A's parent is B, B's parent is A — a 2-node cycle with no external entry.
    // The reachability guard must promote a root so every span is reachable,
    // and must not loop forever.
    let buf = trace_file(&[(sid(1), sid(2)), (sid(2), sid(1))]);
    let reader = IndexReader::open(&buf).unwrap();
    let trace = reader.trace_by_id(TraceId::from(TRACE)).unwrap();

    assert_eq!(trace.spans.len(), 2);
    // At least one span is promoted to a root so the forest is walkable.
    assert!(!trace.roots.is_empty());

    // The cycle's edges remain in `children`, so the walk guards against
    // revisits — and still reaches every span from the promoted root(s).
    let mut seen = vec![false; trace.spans.len()];
    let mut stack = trace.roots.clone();
    while let Some(i) = stack.pop() {
        if seen[i] {
            continue;
        }
        seen[i] = true;
        stack.extend(trace.children[i].iter().copied().filter(|&c| !seen[c]));
    }
    assert!(seen.iter().all(|&s| s), "every span reachable from a root");
}

// ── MetaReader ──────────────────────────────────────────────────────

/// Flip the first byte of chunk `id`, so its CRC no longer matches.
fn corrupt_chunk(buf: &mut [u8], id: chunk_file::ChunkId) {
    let offset = {
        let container =
            chunk_file::container::Container::open(buf, crate::MAGIC, crate::VERSION).unwrap();
        let meta = container.chunk_meta(id).expect("chunk present");
        usize::try_from(meta.offset).unwrap()
    };
    buf[offset] ^= 0xFF;
}

#[test]
fn meta_reader_matches_the_full_readers_summary_metadata_and_fields() {
    let buf = trace_file(&[(sid(1), SpanId::UNSET), (sid(2), sid(1))]);
    let full = IndexReader::open(&buf).unwrap();
    let meta = crate::MetaReader::open(&buf).unwrap();
    assert_eq!(meta.summary(), full.summary());
    assert_eq!(meta.metadata().unwrap(), full.metadata());
    assert_eq!(
        meta.field_table().unwrap().names().collect::<Vec<_>>(),
        full.field_table().names().collect::<Vec<_>>()
    );
}

#[test]
fn meta_reader_never_decodes_the_primary_fst() {
    // A corrupt PRIM fails the full open but not the metadata view: key
    // listing must not pay for (or fail on) the low-card dictionary.
    let mut buf = trace_file(&[(sid(1), SpanId::UNSET)]);
    corrupt_chunk(&mut buf, crate::CHUNK_PRIMARY);
    assert!(IndexReader::open(&buf).is_err());

    let meta = crate::MetaReader::open(&buf).unwrap();
    assert!(meta.field_table().is_ok());
    assert!(
        meta.into_index_reader().is_err(),
        "the upgrade decodes PRIM and surfaces the corruption"
    );
}

// ── Owned sessions: bloom first ─────────────────────────────────────

const OTHER: [u8; 16] = [8u8; 16];

/// A bloom over `ids` (the file it is written into may hold fewer).
fn bloom_over(ids: &[[u8; 16]]) -> crate::TraceIdBloom {
    let mut trace = TraceIds::with_capacity(ids.len());
    for id in ids {
        trace.push(TraceId::from(*id));
    }
    let index = TraceIdIndex::build(&trace);
    crate::TraceIdBloom::build(&index, &trace).expect("non-empty")
}

/// A loose bloom over `ids` (64 bits, one hash: ~3% false positives for
/// two ids), as files sealed before the tight target carry.
fn loose_bloom_over(ids: &[[u8; 16]]) -> crate::TraceIdBloom {
    let mut filter = fastbloom::BloomFilter::with_num_bits(64)
        .seed(&0u128)
        .hashes(1);
    for id in ids {
        filter.insert(id);
    }
    crate::TraceIdBloom::raw_for_tests(ids.len() as u32, filter)
}

fn owned(buf: &[u8]) -> crate::TraceFileSession<'_, '_> {
    crate::TraceFileSession::open_owned(crate::MetaReader::open(buf).unwrap())
}

#[test]
fn an_owned_session_answers_a_bloom_miss_from_the_bloom_alone() {
    use crate::trace_combine::SpanSource;
    let bloom = bloom_over(&[TRACE]);
    let miss = (9u8..=255)
        .map(|b| TraceId::from([b; 16]))
        .find(|id| !bloom.might_contain(*id))
        .expect("an id the bloom rules out");
    let mut buf = trace_file_with_bloom(&[(sid(1), SpanId::UNSET)], Some(&bloom));
    for id in [
        crate::CHUNK_TRACE_INDEX,
        crate::CHUNK_TRACE_IDS,
        crate::CHUNK_PRIMARY,
    ] {
        corrupt_chunk(&mut buf, id);
    }
    let mut session = owned(&buf);
    assert!(session.span_refs(miss).unwrap().is_empty());
}

#[test]
fn a_false_positive_reads_only_the_lookup_pair() {
    use crate::trace_combine::SpanSource;
    // The bloom admits OTHER; the file does not hold it. Two rows, so a
    // bloom over two ids is valid (not the unreadable-bloom fallback).
    let bloom = bloom_over(&[TRACE, OTHER]);
    let mut buf = trace_file_with_bloom(&[(sid(1), SpanId::UNSET), (sid(2), sid(1))], Some(&bloom));
    for id in [crate::CHUNK_SPAN_IDS, crate::CHUNK_PRIMARY] {
        corrupt_chunk(&mut buf, id);
    }
    let mut session = owned(&buf);
    assert!(!session.bloom_unreadable());
    assert!(session.might_contain(TraceId::from(OTHER)));
    assert!(session.span_refs(TraceId::from(OTHER)).unwrap().is_empty());
    // Finding the held trace needs the row columns and the full reader:
    // their corruption surfaces there.
    assert!(session.span_refs(TraceId::from(TRACE)).is_err());
}

#[test]
fn an_owned_session_finds_what_a_borrowed_one_finds() {
    use crate::trace_combine::SpanSource;
    let bloom = bloom_over(&[TRACE]);
    let buf = trace_file_with_bloom(&[(sid(1), SpanId::UNSET), (sid(2), sid(1))], Some(&bloom));
    let reader = IndexReader::open(&buf).unwrap();
    let mut borrowed = crate::TraceFileSession::open(&reader);
    let mut owned = owned(&buf);
    let id = TraceId::from(TRACE);
    let refs = borrowed.span_refs(id).unwrap();
    assert_eq!(refs.len(), 2);
    assert_eq!(owned.span_refs(id).unwrap(), refs);
    for r in &refs {
        assert_eq!(
            owned.materialize(r.position).unwrap(),
            borrowed.materialize(r.position).unwrap()
        );
    }
}

#[test]
fn a_screened_session_answers_false_positives_without_a_lookup() {
    use crate::trace_combine::SpanSource;
    use std::cell::Cell;
    // A loose sealed bloom admits OTHER; the file does not hold it.
    let bloom = loose_bloom_over(&[TRACE, OTHER]);
    assert!(!bloom.is_tight());
    let buf = trace_file_with_bloom(&[(sid(1), SpanId::UNSET), (sid(2), sid(1))], Some(&bloom));
    let other = TraceId::from(OTHER);
    // A lookup that finds nothing drops what it decoded and reports it.
    let drops = Cell::new(0);

    // Unscreened: the false positive decodes the lookup structures.
    let mut plain = owned(&buf).on_release(|| drops.set(drops.get() + 1));
    assert!(!plain.bloom_unreadable());
    assert!(plain.might_contain(other), "the loose bloom admits it");
    assert!(plain.span_refs(other).unwrap().is_empty());
    assert_eq!(drops.get(), 1, "the lookup ran and was dropped");
    assert!(!plain.is_loaded());

    // Screened: the first lookup builds the screen, which then rules
    // OTHER out without a lookup.
    drops.set(0);
    let mut screened = owned(&buf)
        .screened()
        .on_release(|| drops.set(drops.get() + 1));
    assert!(
        screened.might_contain(other),
        "the sealed bloom alone admits it"
    );
    assert_eq!(screened.span_refs(TraceId::from(TRACE)).unwrap().len(), 2);
    assert!(!screened.might_contain(other));
    assert!(screened.span_refs(other).unwrap().is_empty());
    assert_eq!(drops.get(), 0, "no lookup ran");
    assert!(screened.is_loaded(), "the hit keeps its layer");
}

#[test]
fn a_tight_sealed_bloom_needs_no_screen() {
    use crate::trace_combine::SpanSource;
    use std::cell::Cell;
    // A tight bloom that (by construction) admits OTHER: a screened
    // session builds nothing more, so OTHER takes the lookup like an
    // unscreened one — at the tight rate that is the rare case.
    let bloom = bloom_over(&[TRACE, OTHER]);
    assert!(bloom.is_tight());
    let buf = trace_file_with_bloom(&[(sid(1), SpanId::UNSET), (sid(2), sid(1))], Some(&bloom));
    let drops = Cell::new(0);
    let mut screened = owned(&buf)
        .screened()
        .on_release(|| drops.set(drops.get() + 1));
    assert!(!screened.bloom_unreadable());
    assert_eq!(screened.span_refs(TraceId::from(TRACE)).unwrap().len(), 2);
    assert!(
        screened.might_contain(TraceId::from(OTHER)),
        "no screen was built"
    );
    assert!(screened.span_refs(TraceId::from(OTHER)).unwrap().is_empty());
    assert_eq!(drops.get(), 0, "the hit keeps the layer loaded");
}

#[test]
fn a_released_session_reloads_with_the_same_answers() {
    use crate::trace_combine::SpanSource;
    let bloom = bloom_over(&[TRACE]);
    let buf = trace_file_with_bloom(&[(sid(1), SpanId::UNSET), (sid(2), sid(1))], Some(&bloom));
    let id = TraceId::from(TRACE);
    let mut session = owned(&buf).screened();
    assert!(!session.is_loaded());
    let refs = session.span_refs(id).unwrap();
    let spans: Vec<_> = refs
        .iter()
        .map(|r| session.materialize(r.position).unwrap())
        .collect();
    assert!(session.is_loaded());

    session.release();
    assert!(
        !session.is_loaded(),
        "the full reader went back to metadata"
    );
    assert!(session.might_contain(id), "the screen stays");
    assert_eq!(session.span_refs(id).unwrap(), refs);
    for (r, span) in refs.iter().zip(&spans) {
        assert_eq!(&session.materialize(r.position).unwrap(), span);
    }
}

#[test]
fn positions_report_to_loaded_sources() {
    use crate::trace_combine::SpanSource;
    use std::cell::Cell;
    let bloom = bloom_over(&[TRACE, OTHER]);
    let buf = trace_file_with_bloom(&[(sid(1), SpanId::UNSET), (sid(2), sid(1))], Some(&bloom));
    let id = TraceId::from(TRACE);

    // A hit keeps the lookup pair: the session reports loaded, and the
    // tracker releases the least recent one past its limit.
    let mut sessions = vec![owned(&buf), owned(&buf)];
    let mut loaded = crate::LoadedSources::new(1);
    assert_eq!(sessions[0].positions(id).unwrap(), vec![0, 1]);
    assert!(sessions[0].is_loaded());
    loaded.touch(0, &mut sessions);
    assert_eq!(sessions[1].positions(id).unwrap(), vec![0, 1]);
    loaded.touch(1, &mut sessions);
    assert!(!sessions[0].is_loaded(), "released past the limit");
    assert!(sessions[1].is_loaded());
    assert_eq!(sessions[0].positions(id).unwrap(), vec![0, 1], "reloads");

    // A false positive frees what it decoded; a bloom miss decodes
    // nothing; UNSET is a request error.
    let drops = Cell::new(0);
    let mut session = owned(&buf).on_release(|| drops.set(drops.get() + 1));
    assert!(session.positions(TraceId::from(OTHER)).unwrap().is_empty());
    assert!(!session.is_loaded());
    assert_eq!(drops.get(), 1, "the lookup ran and was dropped");
    let miss = (9u8..=255)
        .map(|b| TraceId::from([b; 16]))
        .find(|id| !bloom.might_contain(*id))
        .expect("an id the bloom rules out");
    assert!(session.positions(miss).unwrap().is_empty());
    assert_eq!(drops.get(), 1, "no lookup ran");
    assert!(matches!(
        session.positions(TraceId::UNSET),
        Err(crate::Error::UnsetTraceId)
    ));
}

/// A trace's rollup row and stats come from the session, again after a
/// release; a file sealed before the stats says so, a pre-rollup file
/// too.
#[test]
fn session_reads_rollup_parts() {
    use crate::trace_combine::SpanSource;
    use std::io::Cursor;
    let arena = bumpalo::Bump::new();
    let mut ri = crate::RowIndex::new(&arena, 10);
    let mut ids = TraceIds::with_capacity(3);
    let mut rollup = crate::TraceRollupRows::new();
    for (i, b) in [1u8, 2, 2].into_iter().enumerate() {
        let slot = ri.intern(None, "l=a");
        ri.row(1_000 + i as i64, &[slot]);
        let id = TraceId::from([b; 16]);
        ids.push(id);
        rollup.record_span(
            id,
            sid(i as u8 + 1),
            i == 1,
            1_000 + i as i64,
            10,
            0,
            i == 2,
            None,
            None,
        );
    }
    ri.trace_ids = Some(ids);
    ri.build_trace_id_index = true;
    ri.trace_rollup = Some(rollup);
    let (buf, _s, _m) =
        crate::IndexWriter::write_into(&ri, Cursor::new(Vec::new()), Vec::new()).unwrap();
    let buf = buf.into_inner();

    let mut session = owned(&buf);
    let part = session.rollup_part(TraceId::from([2; 16])).unwrap();
    let crate::PartEvidence::Part(row, stats) = &part else {
        panic!("trace 2 has a part: {part:?}");
    };
    assert_eq!(
        (row.root_claim, row.root_span_id),
        (crate::ROOT_CLAIM_TRUE, sid(2))
    );
    assert_eq!((stats.max_start_ns, stats.root_start_ns), (1_002, 1_001));
    assert_eq!(stats.services, vec![(crate::ROLLUP_NO_REF, 2, 1)]);
    assert_eq!(
        session.rollup_part(TraceId::from([9; 16])).unwrap(),
        crate::PartEvidence::Absent
    );
    session.release();
    assert_eq!(session.rollup_part(TraceId::from([2; 16])).unwrap(), part);

    // Hide TRSS: rename its TOC entry, as a file sealed before it reads.
    let mut legacy = buf.clone();
    let at = legacy
        .windows(4)
        .position(|w| w == b"TRSS")
        .expect("a TOC entry");
    legacy[at..at + 4].copy_from_slice(b"XXSS");
    assert_eq!(
        owned(&legacy).rollup_part(TraceId::from([2; 16])).unwrap(),
        crate::PartEvidence::NoStats
    );
    let plain = trace_file(&[(sid(1), SpanId::UNSET)]);
    assert_eq!(
        owned(&plain).rollup_part(TraceId::from([2; 16])).unwrap(),
        crate::PartEvidence::NoRollup
    );
}

#[test]
fn rollup_rows_stream_from_the_session() {
    use crate::trace_combine::SpanSource;
    use std::io::Cursor;
    let arena = bumpalo::Bump::new();
    let mut ri = crate::RowIndex::new(&arena, 10);
    let mut ids = TraceIds::with_capacity(4);
    let mut rollup = crate::TraceRollupRows::new();
    for (i, b) in [3u8, 1, 3, 2].into_iter().enumerate() {
        let slot = ri.intern(None, "l=a");
        ri.row(1_000 + i as i64, &[slot]);
        let id = TraceId::from([b; 16]);
        ids.push(id);
        rollup.record_span(id, sid(i as u8 + 1), true, 1_000, 10, 0, false, None, None);
    }
    ri.trace_ids = Some(ids);
    ri.build_trace_id_index = true;
    ri.trace_rollup = Some(rollup);
    let (buf, _s, _m) =
        crate::IndexWriter::write_into(&ri, Cursor::new(Vec::new()), Vec::new()).unwrap();
    let buf = buf.into_inner();

    let mut session = owned(&buf);
    let mut got = Vec::new();
    assert_eq!(
        session.for_each_rollup_row(|id, _| got.push(id)).unwrap(),
        Some(3)
    );
    assert_eq!(
        got,
        [1u8, 2, 3].map(|b| TraceId::from([b; 16])),
        "each trace once, in id order"
    );
    assert!(session.is_loaded(), "the rollup stays for later rows");
    assert!(matches!(
        session.rollup_row(TraceId::from([2; 16])).unwrap(),
        crate::RollupEvidence::Row(_)
    ));

    let plain = trace_file(&[(sid(1), SpanId::UNSET)]);
    assert_eq!(owned(&plain).for_each_rollup_row(|_, _| {}).unwrap(), None);
}
