//! The tail/seal rollup parity contract — the test pin behind the
//! "Parity contract (test-pinned)" note in `src/traces/rollup.rs`:
//! folding a WAL tail's decoded spans (`tail_trace_aggregates`) equals
//! reading the `TRSU` rollup of sealing the SAME data
//! (`sealed_trace_envelopes`) — value-for-value across envelopes and
//! stored-row counts.
//!
//! Coverage: the wholesale parity, a semantic-by-semantic audit of the
//! tail fold on one corpus, and the rollup root-ref resolver's closed
//! rule (a ref in another field's range is corruption) that the search
//! gate relies on. Not pinned here (the fixture cannot reach them): the
//! u32::MAX count clamp and the saturating envelope end. Fixtures come
//! from `tests/common/mod.rs`; the sibling `tests/traces_*.rs` suites
//! pin the surrounding query ops — for the overview, these per-source
//! aggregates merge in `src/traces/fold.rs`.

mod common;

use common::{req, sp, write_wal};
use sfsq::traces::{TraceWalScan, sealed_trace_envelopes, tail_trace_aggregates};

/// A corpus covering the rollup semantics the fixture can reach
/// (one trace id per bullet):
/// - trace A: a root (unset parent, SERVER kind) + a child + a RESENT
///   copy of the child (stored-row counts) + an ERROR span stretching
///   the envelope end;
/// - trace B: NO unset-parent span;
/// - trace C: two equal-start roots;
/// - trace D: two copies of one `(start, span_id)` with differing kinds;
/// - one UNSET-trace-id span (excluded everywhere).
fn corpus() -> Vec<common::SpanSpec> {
    let a = [0xA1u8; 16];
    let b = [0xB2u8; 16];
    let c = [0xC3u8; 16];
    let d = [0xD7u8; 16];

    let mut a_root = sp(1, 0, 1_000, "a-root");
    a_root.trace = a;
    a_root.kind = 2; // SERVER
    let mut a_child = sp(2, 1, 1_500, "a-child");
    a_child.trace = a;
    let mut a_err = sp(3, 1, 1_800, "a-err");
    a_err.trace = a;
    a_err.status = Some((2, "boom")); // STATUS_CODE_ERROR
    a_err.end = 9_000; // stretches the envelope end

    let mut b_orphan = sp(4, 9, 5_000, "b-orphan");
    b_orphan.trace = b;

    let mut c_root_hi = sp(7, 0, 3_000, "c-root-hi");
    c_root_hi.trace = c;
    let mut c_root_lo = sp(5, 0, 3_000, "c-root-lo"); // same start, smaller id
    c_root_lo.trace = c;

    // D: same (start, span_id), different kind — the ambiguous tie.
    let mut d_tie_a = sp(8, 0, 4_000, "d-tie");
    d_tie_a.trace = d;
    d_tie_a.kind = 3; // CLIENT
    let mut d_tie_b = sp(8, 0, 4_000, "d-tie");
    d_tie_b.trace = d;
    d_tie_b.kind = 2; // SERVER

    let mut unset = sp(6, 0, 100, "no-trace");
    unset.trace = [0; 16];

    vec![
        a_root,
        a_child.clone(),
        a_child, // the resend — counts twice
        a_err,
        b_orphan,
        c_root_hi,
        c_root_lo,
        d_tie_a,
        d_tie_b,
        unset,
    ]
}

#[test]
fn tail_fold_matches_the_sealed_rollup_value_for_value() {
    let dir = tempfile::tempdir().unwrap();
    let wal = write_wal(dir.path(), vec![req(&corpus())], "parity");

    // Tail side: fold the decoded spans directly.
    let scan = TraceWalScan::scan(&wal).unwrap();
    let tail = tail_trace_aggregates(&scan);

    // Sealed side: run the production sealer over the SAME WAL, then
    // read its TRSU.
    let out = dir.path().join("parity.sfst");
    ng_index::build_sfst_traces_file(&wal, &out, &ng_index::Metrics::new()).unwrap();
    let bytes = std::fs::read(&out).unwrap();
    let reader = sfst::IndexReader::open(&bytes).unwrap();
    let sealed = sealed_trace_envelopes(&reader.trace_rollup().unwrap());

    assert_eq!(tail, sealed, "the parity contract");
}

/// The tail fold's semantics, field by field, on the corpus above —
/// the parity test already equates both sides wholesale, so this runs
/// the tail side alone, asserting WHERE each number comes from.
#[test]
fn the_fold_pins_every_rollup_semantic() {
    let dir = tempfile::tempdir().unwrap();
    let wal = write_wal(dir.path(), vec![req(&corpus())], "semantics");
    let scan = TraceWalScan::scan(&wal).unwrap();
    let aggs = tail_trace_aggregates(&scan);

    // The UNSET-trace-id span vanished; rows sort by trace id.
    assert_eq!(aggs.len(), 4);
    let (a, b, c, d) = (&aggs[0], &aggs[1], &aggs[2], &aggs[3]);
    assert_eq!(a.trace_id, sfst::TraceId::from([0xA1; 16]));

    // A: stored-row counts (4 = root + child + resend + err), one error,
    // envelope stretched by the error span's end.
    assert_eq!(a.span_count, 4, "the resend counts");
    assert_eq!(a.error_count, 1);
    assert_eq!(a.min_start_ns, 1_000);
    assert_eq!(a.max_end_ns, 9_000);

    // B: a trace without a root still folds its counts.
    assert_eq!(b.span_count, 1);
    assert_eq!(b.error_count, 0);

    // C: both equal-start roots count.
    assert_eq!(c.span_count, 2);
    assert_eq!(c.min_start_ns, 3_000);

    // D: both stored copies count (stored-row semantics).
    assert_eq!(d.span_count, 2);
}

#[test]
fn a_root_ref_in_another_fields_range_is_corruption() {
    // The search gate resolves rollup root refs through
    // `sfst::RollupRootResolver`. A ref pointing into ANOTHER field's
    // KvId range stays inside the kv table (chunk validation cannot
    // catch it), so the resolver's closed rule is the only guard: it
    // must answer Corrupt, never the other field's value.
    let dir = tempfile::tempdir().unwrap();
    let wal = write_wal(dir.path(), vec![req(&corpus())], "corrupt-ref");
    let out = dir.path().join("corrupt-ref.sfst");
    ng_index::build_sfst_traces_file(&wal, &out, &ng_index::Metrics::new()).unwrap();
    let bytes = std::fs::read(&out).unwrap();
    let reader = sfst::IndexReader::open(&bytes).unwrap();
    let rollup = reader.trace_rollup().unwrap();
    let service_field = "resource.attributes.service.name";

    // Row 0 (trace A) claims a true root: its own refs resolve...
    assert_eq!(rollup.root_is_true_root[0], sfst::ROOT_CLAIM_TRUE);
    let mut resolver = sfst::RollupRootResolver::new(&reader);
    assert!(matches!(
        resolver.resolve("name", rollup.root_name_refs[0]),
        sfst::RollupRefOutcome::Value("a-root")
    ));
    assert!(matches!(
        resolver.resolve(service_field, rollup.root_service_refs[0]),
        sfst::RollupRefOutcome::Value("svc")
    ));
    // ...while its NAME ref read as a SERVICE ref is corruption.
    assert!(matches!(
        resolver.resolve(service_field, rollup.root_name_refs[0]),
        sfst::RollupRefOutcome::Corrupt
    ));
}
