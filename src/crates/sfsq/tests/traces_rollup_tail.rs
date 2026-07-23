//! The phase-2 parity contract: folding a WAL tail's decoded spans
//! ([`tail_trace_aggregates`]) equals reading the `TRSU` rollup of
//! sealing the SAME data ([`sealed_trace_aggregates`]) — value-for-value
//! across envelopes, stored-row counts, and honest roots.

mod common;

use common::{req, sp, write_wal};
use sfsq::traces::{TraceWalScan, sealed_trace_aggregates, tail_trace_aggregates};

/// A corpus covering every semantic the rollup pins:
/// - trace A: a true root (unset parent, SERVER kind) + a child + a
///   RESENT copy of the child (stored-row counts) + an ERROR span;
/// - trace B: NO unset-parent span (honest root absence);
/// - trace C: two equal-start true roots (the span-id tie-break);
/// - one UNSET-trace-id span (excluded everywhere).
fn corpus() -> Vec<common::SpanSpec> {
    let a = [0xA1u8; 16];
    let b = [0xB2u8; 16];
    let c = [0xC3u8; 16];

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

    let mut unset = sp(6, 0, 100, "no-trace");
    unset.trace = [0; 16];

    vec![
        a_root,
        a_child.clone(),
        a_child, // the resend — counts twice (D9)
        a_err,
        b_orphan,
        c_root_hi,
        c_root_lo,
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

    // Sealed side: seal the same WAL, read TRSU, resolve refs.
    let out = dir.path().join("parity.sfst");
    ng_index::build_sfst_traces_file(&wal, &out, &ng_index::Metrics::new()).unwrap();
    let bytes = std::fs::read(&out).unwrap();
    let reader = sfst::IndexReader::open(&bytes).unwrap();
    let strings = reader.build_string_table(reader.field_table()).unwrap();
    let sealed = sealed_trace_aggregates(&reader.trace_rollup().unwrap(), &strings);

    assert_eq!(tail, sealed, "the parity contract");
}

#[test]
fn the_fold_pins_every_rollup_semantic() {
    let dir = tempfile::tempdir().unwrap();
    let wal = write_wal(dir.path(), vec![req(&corpus())], "semantics");
    let scan = TraceWalScan::scan(&wal).unwrap();
    let aggs = tail_trace_aggregates(&scan);

    // The UNSET-trace-id span vanished; rows sort by trace id.
    assert_eq!(aggs.len(), 3);
    let (a, b, c) = (&aggs[0], &aggs[1], &aggs[2]);
    assert_eq!(a.trace_id, sfst::TraceId::from([0xA1; 16]));

    // A: stored-row counts (4 = root + child + resend + err), one error,
    // envelope stretched by the error span's end, a true SERVER root
    // with service + name resolved.
    assert_eq!(a.span_count, 4, "the resend counts (D9)");
    assert_eq!(a.error_count, 1);
    assert_eq!(a.min_start_ns, 1_000);
    assert_eq!(a.max_end_ns, 9_000);
    let root = a.root.as_ref().expect("A has a true root");
    assert_eq!(root.kind, 2);
    assert_eq!(root.service.as_deref(), Some("svc"));
    assert_eq!(root.name.as_deref(), Some("a-root"));

    // B: honest absence — counts still fold, no synthesized root.
    assert_eq!(b.span_count, 1);
    assert!(b.root.is_none(), "no unset-parent span → None (D8)");

    // C: equal-start roots tie-break by ascending span id.
    let c_root = c.root.as_ref().expect("C has true roots");
    assert_eq!(c_root.span_id, sfst::SpanId::from([5; 8]));
    assert_eq!(c_root.name.as_deref(), Some("c-root-lo"));
}
