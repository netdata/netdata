//! The tail/seal parity contract: folding a WAL tail's decoded spans
//! ([`tail_trace_aggregates`]) equals reading the `TRSU` rollup of
//! sealing the SAME data ([`sealed_trace_aggregates`]) — value-for-value
//! across envelopes, stored-row counts, and honest roots.

mod common;

use common::{req, sp, write_wal};
use sfsq::traces::{
    TraceWalScan, sealed_trace_aggregates, sealed_trace_envelopes, tail_trace_aggregates,
};

/// A corpus covering every semantic the rollup pins:
/// - trace A: a true root (unset parent, SERVER kind) + a child + a
///   RESENT copy of the child (stored-row counts) + an ERROR span;
/// - trace B: NO unset-parent span (honest root absence);
/// - trace C: two equal-start true roots (the span-id tie-break);
/// - trace D: a FULL `(start, span_id)` tie with differing facets —
///   the abstention (claim withheld on BOTH sides of the parity);
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

    // Sealed side: seal the same WAL, read TRSU, resolve refs.
    let out = dir.path().join("parity.sfst");
    ng_index::build_sfst_traces_file(&wal, &out, &ng_index::Metrics::new()).unwrap();
    let bytes = std::fs::read(&out).unwrap();
    let reader = sfst::IndexReader::open(&bytes).unwrap();
    let sealed = sealed_trace_aggregates(&reader.trace_rollup().unwrap(), &reader).unwrap();

    assert_eq!(tail, sealed, "the parity contract");

    // The roots-free grid view is the SAME rows minus root resolution —
    // no dictionary decode involved, everything else value-for-value.
    let envelopes = sealed_trace_envelopes(&reader.trace_rollup().unwrap());
    let rootless: Vec<_> = sealed
        .into_iter()
        .map(|a| sfsq::traces::TraceAggregate { root: None, ..a })
        .collect();
    assert_eq!(envelopes, rootless, "the envelope view drops only roots");
}

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
    // envelope stretched by the error span's end, a true SERVER root
    // with service + name resolved.
    assert_eq!(a.span_count, 4, "the resend counts");
    assert_eq!(a.error_count, 1);
    assert_eq!(a.min_start_ns, 1_000);
    assert_eq!(a.max_end_ns, 9_000);
    let root = a.root.as_ref().expect("A has a true root");
    assert_eq!(root.kind, 2);
    assert_eq!(root.service.as_deref(), Some("svc"));
    assert_eq!(root.name.as_deref(), Some("a-root"));

    // B: honest absence — counts still fold, no synthesized root.
    assert_eq!(b.span_count, 1);
    assert!(b.root.is_none(), "no unset-parent span → None");

    // C: equal-start roots tie-break by ascending span id.
    let c_root = c.root.as_ref().expect("C has true roots");
    assert_eq!(c_root.span_id, sfst::SpanId::from([5; 8]));
    assert_eq!(c_root.name.as_deref(), Some("c-root-lo"));

    // D: the ambiguous full-key tie abstains — roots exist, none is
    // claimed (the seal marks the row WITHHELD; the fold reads None).
    assert_eq!(d.span_count, 2);
    assert!(d.root.is_none(), "ambiguous tie → claim withheld");
}

#[test]
fn corrupt_root_ref_escalates_instead_of_rendering_a_wrong_root() {
    // A root ref pointing into ANOTHER field's KvId range is corruption
    // evidence for the whole file (the resolver's closed rule) — the
    // sealed fold must error so the caller fails the source, never
    // render the other field's value as a root.
    let dir = tempfile::tempdir().unwrap();
    let wal = write_wal(dir.path(), vec![req(&corpus())], "corrupt-ref");
    let out = dir.path().join("corrupt-ref.sfst");
    ng_index::build_sfst_traces_file(&wal, &out, &ng_index::Metrics::new()).unwrap();
    let bytes = std::fs::read(&out).unwrap();
    let reader = sfst::IndexReader::open(&bytes).unwrap();

    // Row 0 (trace A) claims a true root; its name ref lives in the
    // span-name field's range — as a SERVICE ref it stays inside the
    // kv table (chunk validation cannot catch it) but outside the
    // service field: the class a bare string-table lookup mis-renders.
    let mut rollup = reader.trace_rollup().unwrap();
    rollup.root_service_refs[0] = rollup.root_name_refs[0];
    let err = sealed_trace_aggregates(&rollup, &reader).unwrap_err();
    assert!(matches!(err, sfst::Error::CorruptIndex(_)), "{err}");
}

#[test]
fn sealed_root_without_service_resolves_the_sentinel_to_none() {
    // The ROLLUP_NO_REF → None branch of the sealed resolver: a true
    // root whose resource carries no service.name.
    use common::req_with;
    let mut root = sp(1, 0, 1_000, "svcless-root");
    root.trace = [0xD4; 16];
    let dir = tempfile::tempdir().unwrap();
    let wal = write_wal(dir.path(), vec![req_with(vec![], None, &[root])], "svcless");

    let out = dir.path().join("svcless.sfst");
    ng_index::build_sfst_traces_file(&wal, &out, &ng_index::Metrics::new()).unwrap();
    let bytes = std::fs::read(&out).unwrap();
    let reader = sfst::IndexReader::open(&bytes).unwrap();
    let sealed = sealed_trace_aggregates(&reader.trace_rollup().unwrap(), &reader).unwrap();

    let root = sealed[0].root.as_ref().expect("a true root");
    assert_eq!(root.service, None, "the sentinel resolves to honest None");
    assert_eq!(root.name.as_deref(), Some("svcless-root"));

    // And the tail fold agrees — the parity holds here too.
    let tail = tail_trace_aggregates(&TraceWalScan::scan(&wal).unwrap());
    assert_eq!(tail, sealed);
}
