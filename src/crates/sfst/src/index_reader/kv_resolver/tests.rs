//! Tests for [`KvResolver`] over files built with the crate's own writer
//! ([`RowIndex`] + [`IndexWriter::write_into`]).
//!
//! Pins, per fixture, for EVERY KvId of the file:
//!
//! - it resolves to the same string as `build_string_table`, the
//!   independent per-field walk the resolver replaced;
//! - each row's KvIds resolve to exactly the strings the writer interned
//!   for it;
//! - ids at or past the file's total stay unresolved;
//! - the answer does not depend on lookup order, and each mid/high chunk
//!   decodes once per resolver.
//!
//! Fixtures cover dot-extended field families in every tier (the
//! `a.b=` < `a=` sort hazard), the production tier boundaries (99/100 and
//! 999/1000 values), hundreds of interleaved low-card fields, single-tier
//! files, and values that are empty, carry `=`, are unicode, or exceed the
//! session cache's 4 KiB limit. The trace session test pins the decode
//! count of a large trace with long unique values: the cost the walk-based
//! resolution paid once per span.

use std::io::Cursor;

use bumpalo::Bump;

use super::KvResolver;
use crate::schema::FieldTier;
use crate::{IndexReader, IndexWriter, RowIndex, SpanId, TraceId};

/// A value of row `i`: empty, `=`-carrying, unicode, longer than 4 KiB, or
/// plain. Distinct for distinct `i`.
fn value(i: usize) -> String {
    match i % 5 {
        _ if i == 0 => String::new(),
        1 => format!("v{i}=x=y"),
        2 => format!("\u{e9}\u{1f600}{i}"),
        3 => format!("{i}{}", "L".repeat(4_100 + i % 97)),
        _ => format!("v{i:04}"),
    }
}

/// `n` rows of `field=value(i)`, one pair per row.
fn single_pairs(field: &str, n: usize, rows: &mut Vec<Vec<String>>) {
    for i in 0..n {
        rows.push(vec![format!("{field}={}", value(i))]);
    }
}

/// The file of `rows` (ascending timestamps: position == row index).
fn file_of(threshold: u32, rows: &[Vec<String>]) -> Vec<u8> {
    let arena = Bump::new();
    let mut ri = RowIndex::new(&arena, threshold);
    for (i, row) in rows.iter().enumerate() {
        let tokens: Vec<_> = row.iter().map(|kv| ri.intern(None, kv)).collect();
        ri.row(1_000 + i as i64, &tokens);
    }
    let (buf, _s, _m) = IndexWriter::write_into(&ri, Cursor::new(Vec::new()), Vec::new()).unwrap();
    buf.into_inner()
}

/// A deterministic permutation of `0..n` (LCG-shuffled).
fn shuffled(n: u32) -> Vec<u32> {
    let mut ids: Vec<u32> = (0..n).collect();
    let mut state = 0x2545_f491_4f6c_dd1du64;
    for i in (1..ids.len()).rev() {
        state = state
            .wrapping_mul(6_364_136_223_846_793_005)
            .wrapping_add(1_442_695_040_888_963_407);
        ids.swap(i, (state >> 33) as usize % (i + 1));
    }
    ids
}

/// The full equivalence check of one file (see the module docs).
fn assert_equivalent(bytes: &[u8], rows: &[Vec<String>]) {
    let idx = IndexReader::open(bytes).unwrap();
    let total = idx.id_ranges().high_end.0;
    let table = idx.build_string_table(idx.field_table()).unwrap();
    assert_eq!(table.len(), total as usize);
    let chunks = idx
        .field_table()
        .iter()
        .filter(|f| f.tier != FieldTier::Low)
        .count();

    for order in [
        (0..total).collect::<Vec<_>>(),
        (0..total).rev().collect(),
        shuffled(total),
    ] {
        let mut kv = KvResolver::new();
        let mut key = Vec::new();
        for &id in &order {
            assert!(
                kv.key_into(&idx, id, &mut key).unwrap(),
                "id {id} unresolved"
            );
            assert_eq!(String::from_utf8_lossy(&key), table[id as usize], "id {id}");
        }
        for id in [total, total + 1, u32::MAX] {
            assert!(
                !kv.key_into(&idx, id, &mut key).unwrap(),
                "id {id} is past the end"
            );
            assert!(key.is_empty(), "an unresolved id leaves no bytes");
        }
        assert_eq!(kv.loads, chunks, "each mid/high chunk decodes once");
    }

    // The writer's truth: every row's KvIds name the strings it interned.
    let entries = idx.load_all_stream_entries().unwrap();
    assert_eq!(entries.len(), rows.len());
    let all: Vec<u32> = (0..total).collect();
    let strings = KvResolver::new().strings(&idx, &all).unwrap();
    assert_eq!(strings.len(), total as usize);
    for (r, (ids, want)) in entries.iter().zip(rows).enumerate() {
        let mut got: Vec<&str> = ids.iter().map(|id| strings[&id.0].as_str()).collect();
        let mut want: Vec<&str> = want.iter().map(String::as_str).collect();
        got.sort_unstable();
        want.sort_unstable();
        assert_eq!(got, want, "row {r}");
    }
}

/// The tier each named field landed in.
fn tiers(bytes: &[u8]) -> Vec<(String, FieldTier, u32)> {
    let idx = IndexReader::open(bytes).unwrap();
    idx.field_table()
        .iter()
        .map(|f| (f.name.clone(), f.tier, f.cardinality))
        .collect()
}

/// Dot-extended families (and their `-`, `_`, bare-suffix and unicode
/// neighbours, which sort around them) in every tier at once.
#[test]
fn every_id_of_prefix_families_in_all_tiers() {
    let mut rows = Vec::new();
    for (base, n) in [("l", 3), ("m", 20), ("h", 120)] {
        for suffix in ["", "-x", ".x", ".x.y", "_x", "x", "\u{e9}"] {
            single_pairs(&format!("{base}{suffix}"), n, &mut rows);
        }
    }
    // Rows that carry one pair of every field, so rows mix tiers too.
    let names: Vec<String> = rows
        .iter()
        .map(|r| r[0].split_once('=').unwrap().0.to_string())
        .collect::<std::collections::BTreeSet<_>>()
        .into_iter()
        .collect();
    for i in 0..3 {
        rows.push(names.iter().map(|f| format!("{f}={}", value(i))).collect());
    }
    let bytes = file_of(10, &rows);
    for (name, tier, _) in tiers(&bytes) {
        let want = match name.as_bytes()[0] {
            b'l' => FieldTier::Low,
            b'm' => FieldTier::Mid,
            _ => FieldTier::High,
        };
        assert_eq!(tier, want, "{name}");
    }
    assert_equivalent(&bytes, &rows);
}

/// The production threshold (100): 99 values stay low, 100 go mid, 999
/// stay mid, 1000 go high, each field dot-extending the previous one.
#[test]
fn every_id_at_the_production_tier_boundaries() {
    let mut rows = Vec::new();
    single_pairs("b", 99, &mut rows);
    single_pairs("b.c", 100, &mut rows);
    single_pairs("b.c.d", 999, &mut rows);
    single_pairs("b.c.d.e", 1_000, &mut rows);
    single_pairs("bc", 1, &mut rows);
    let bytes = file_of(100, &rows);
    assert_eq!(
        tiers(&bytes),
        vec![
            ("b".to_string(), FieldTier::Low, 99),
            ("bc".to_string(), FieldTier::Low, 1),
            ("b.c".to_string(), FieldTier::Mid, 100),
            ("b.c.d".to_string(), FieldTier::Mid, 999),
            ("b.c.d.e".to_string(), FieldTier::High, 1_000),
        ]
    );
    assert_equivalent(&bytes, &rows);
}

/// Hundreds of low-card fields whose keys interleave in the primary FST,
/// so each field's base position differs from the running id sum.
#[test]
fn every_id_of_many_interleaved_low_fields() {
    let suffixes = ["", ".a", ".a.b", "_a", "-a", "a", ".", "..", "\u{e9}", "/z"];
    let mut rows = Vec::new();
    for i in 0..400 {
        let name = format!("f{}{}", i / suffixes.len(), suffixes[i % suffixes.len()]);
        single_pairs(&name, 1 + i % 3, &mut rows);
    }
    let bytes = file_of(100, &rows);
    assert!(tiers(&bytes).iter().all(|(_, t, _)| *t == FieldTier::Low));
    assert_equivalent(&bytes, &rows);
}

/// Files without a low tier, and without mid and high tiers.
#[test]
fn every_id_of_single_tier_files() {
    let mut rows = Vec::new();
    single_pairs("k", 50, &mut rows);
    single_pairs("k.v", 12, &mut rows);
    let bytes = file_of(1, &rows); // low < 1: none; mid [1, 10); high ≥ 10
    assert!(tiers(&bytes).iter().all(|(_, t, _)| *t == FieldTier::High));
    assert_equivalent(&bytes, &rows);

    let mut rows = Vec::new();
    single_pairs("k", 5, &mut rows);
    single_pairs("k.v", 2, &mut rows);
    let bytes = file_of(100, &rows);
    assert!(tiers(&bytes).iter().all(|(_, t, _)| *t == FieldTier::Low));
    assert_equivalent(&bytes, &rows);
}

/// Metadata claiming more values for a low-card field than the primary
/// FST holds (corruption a CRC cannot catch): `base + offset` then lands
/// on the next field's key, which must stay unresolved, as the walk left
/// it; the next field still resolves at its own base, and a field with no
/// key at all resolves nothing.
#[test]
fn a_low_field_short_of_its_cardinality_leaves_the_excess_unresolved() {
    use crate::writer::{ChunkCounts, ChunkWriter, ColumnsPresent};
    use crate::{
        BitmapValue, ColumnsTable, FieldEntry, Histogram, IdRanges, KvId, Metadata, SchemaTree,
        StreamBatch, Summary,
    };
    let bitmap = || {
        let mut data = Vec::new();
        let desc = treight::Bitmap::from_sorted_iter([0].into_iter(), 1, &mut data);
        BitmapValue { desc, data }
    };
    let field = |name: &str, cardinality| FieldEntry {
        name: name.into(),
        cardinality,
        tier: FieldTier::Low,
    };
    // KvIds by the metadata: a 0..2, b 2..3, c 3..4. PRIM holds a=x, b=y.
    let metadata = Metadata {
        histogram: Histogram {
            timestamps: vec![1_700_000_000],
            counts: vec![1],
        },
        id_ranges: IdRanges {
            low_end: KvId(4),
            mid_end: KvId(4),
            high_end: KvId(4),
        },
        tree: SchemaTree::flat(&vec![field("a", 2), field("b", 1), field("c", 1)].into()),
        columns: ColumnsTable::default(),
    };
    let counts = ChunkCounts {
        columns: ColumnsPresent::default(),
        trace_id_index: false,
        trace_id_bloom: false,
        event_index: false,
        link_index: false,
        trace_rollup: false,
        trace_rollup_stats: false,
        mid_fields: 0,
        high_fields: 0,
        stream_batches: 1,
    };
    let mut writer = ChunkWriter::new(Cursor::new(Vec::new()), counts).unwrap();
    writer
        .summary(&Summary {
            min_timestamp_s: 1_700_000_000,
            max_timestamp_s: 1_700_000_000,
            record_count: 1,
            content_meta: Vec::new(),
        })
        .unwrap();
    writer.metadata(&metadata).unwrap();
    writer.timestamps(&[1_700_000_000_000_000_000]).unwrap();
    writer
        .primary(vec![("a=x", bitmap()), ("b=y", bitmap())])
        .unwrap();
    writer
        .add_stream_batch(&StreamBatch::for_write(&[vec![KvId(0)]]))
        .unwrap();
    let bytes = writer.finish().unwrap().into_inner();

    let idx = IndexReader::open(&bytes).unwrap();
    let mut kv = KvResolver::new();
    let mut key = Vec::new();
    let mut resolve = |id| {
        kv.key_into(&idx, id, &mut key)
            .unwrap()
            .then(|| key.clone())
    };
    assert_eq!(resolve(0), Some(b"a=x".to_vec()));
    assert_eq!(resolve(1), None, "a's second id would read b=y");
    assert_eq!(resolve(2), Some(b"b=y".to_vec()));
    assert_eq!(resolve(3), None, "c has no key");
}

fn sid(i: usize) -> SpanId {
    SpanId::from((i as u64 + 1).to_be_bytes())
}

/// A large trace in one file, every span with long unique values (a mid
/// field above the 4 KiB cache limit, a high-card id) and an event whose
/// attribute is long too: the shape of a long agent session. The session
/// rebuilds it exactly, decoding each touched field chunk ONCE — the walk
/// this replaced re-decoded and re-walked them per span — and again after
/// a release.
#[test]
fn a_large_trace_decodes_each_field_chunk_once() {
    const SPANS: usize = 300;
    let trace = TraceId::from([7u8; 16]);
    let arena = Bump::new();
    let mut ri = RowIndex::new(&arena, 10); // mid [10, 100), high ≥ 100
    let mut trace_ids = crate::TraceIds::with_capacity(SPANS);
    let mut span_ids = crate::SpanIds::with_capacity(SPANS);
    let mut parents = crate::ParentSpanIds::with_capacity(SPANS);
    let mut events = crate::EventRows::new();
    let mut want: Vec<Vec<(String, String)>> = Vec::new();
    for i in 0..SPANS {
        // 50 distinct values above the 4 KiB cache limit: mid tier.
        let output = format!("{:02}{}", i % 50, "o".repeat(5_000));
        let pairs = [
            ("name", format!("op{}", i % 4)),
            ("attributes.output", output.clone()),
            ("attributes.call_id", format!("call-{i:04}")),
        ];
        let tokens: Vec<_> = pairs
            .iter()
            .map(|(k, v)| ri.intern(None, &format!("{k}={v}")))
            .collect();
        let event_name = ri.intern(None, "events.name=tool");
        let event_attr = ri.intern(None, &format!("events.attributes.result={output}"));
        let mut row = tokens.clone();
        row.extend([event_name, event_attr]);
        ri.row(1_000 + i as i64, &row);
        events.push_event(1_000 + i as u64, 0, event_name, &[event_attr]);
        events.end_row(0);
        trace_ids.push(trace);
        span_ids.push(sid(i));
        parents.push(if i == 0 { SpanId::UNSET } else { sid(0) });
        want.push(
            pairs
                .iter()
                .map(|(k, v)| (k.to_string(), v.clone()))
                .collect(),
        );
    }
    ri.trace_ids = Some(trace_ids);
    ri.span_ids = Some(span_ids);
    ri.parent_span_ids = Some(parents);
    ri.durations = Some(crate::Durations(vec![5; SPANS]));
    ri.flags = Some(crate::Flags(vec![0; SPANS]));
    ri.dropped_attribute_counts = Some(crate::DroppedAttributeCounts(vec![0; SPANS]));
    ri.events = Some(events);
    ri.build_trace_id_index = true;
    let (buf, _s, _m) = IndexWriter::write_into(&ri, Cursor::new(Vec::new()), Vec::new()).unwrap();
    let bytes = buf.into_inner();
    let idx = IndexReader::open(&bytes).unwrap();
    let mid_high: Vec<String> = idx
        .field_table()
        .iter()
        .filter(|f| f.tier != FieldTier::Low)
        .map(|f| f.name.clone())
        .collect();
    assert_eq!(
        mid_high,
        [
            "attributes.output",
            "events.attributes.result",
            "attributes.call_id"
        ],
        "fixture tiers"
    );

    let check = |trace_spans: &[crate::TraceSpan]| {
        assert_eq!(trace_spans.len(), SPANS);
        for span in trace_spans {
            let i = (u64::from_be_bytes(*span.span_id.as_bytes()) - 1) as usize;
            let mut fields = span.fields.clone();
            fields.sort();
            let mut expected = want[i].clone();
            expected.sort();
            assert_eq!(fields, expected, "span {i} fields");
            assert_eq!(span.events.len(), 1, "span {i} events");
            assert_eq!(span.events[0].name, "tool");
            assert_eq!(
                span.events[0].attributes,
                vec![("result".to_string(), want[i][1].1.clone())],
                "span {i} event attributes"
            );
        }
    };

    let mut session = crate::TraceFileSession::open(&idx);
    for round in 0..2 {
        let mut sources: [&mut dyn crate::trace_combine::SpanSource; 1] = [&mut session];
        let outcome =
            crate::trace_combine::combine(&mut sources, trace, None, None, None, &|| false);
        assert!(outcome.failures.is_empty(), "round {round}");
        check(&outcome.trace.spans);
        assert_eq!(
            session.kv_loads(),
            mid_high.len(),
            "round {round}: one decode per chunk"
        );
        crate::trace_combine::SpanSource::release(&mut session);
        assert_eq!(session.kv_loads(), 0, "release drops the dictionaries");
    }
    check(&idx.trace_by_id(trace).unwrap().spans);
}
