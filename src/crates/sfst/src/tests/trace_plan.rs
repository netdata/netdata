//! Per-file trace-search plan evaluation: tier matrix, conjunction,
//! duration bounds, work counting, and the rank-bounded-extraction
//! proof (positions emitted = min(K, matched), never the full set).

use std::io::Cursor;

use bumpalo::Bump;

use crate::{
    CompiledTracePlan, Durations, IndexReader, IndexWriter, PlanTerm, RowIndex, ScanWork,
    TracePlan,
};

/// One fixture row's field values, mirrored for the oracle.
struct FixtureRow {
    l: String,
    m: String,
    h: String,
    h2: String,
    dur: i64,
}

const N: usize = 256;

/// 256 rows, one field per tier (threshold 10): `l` low (2 values),
/// `m` mid (16), `h`/`h2` high (128 each), ascending timestamps
/// (position == insertion index), plus a DURN column `dur = i * 10`.
fn fixture() -> (Vec<u8>, Vec<FixtureRow>) {
    let arena = Bump::new();
    let mut ri = RowIndex::new(&arena, 10);
    let mut rows = Vec::with_capacity(N);
    let mut durations = Vec::with_capacity(N);
    for i in 0..N {
        let row = FixtureRow {
            l: if i % 2 == 0 { "a" } else { "b" }.to_string(),
            m: format!("v{:02}", i % 16),
            h: format!("w{:03}", i % 128),
            h2: format!("x{:03}", (i * 7) % 128),
            dur: (i as i64) * 10,
        };
        let tokens = [
            format!("l={}", row.l),
            format!("m={}", row.m),
            format!("h={}", row.h),
            format!("h2={}", row.h2),
        ];
        let slots: Vec<_> = tokens.iter().map(|kv| ri.intern(None, kv)).collect();
        ri.row(1_000 + i as i64, &slots);
        durations.push(row.dur);
        rows.push(row);
    }
    ri.durations = Some(Durations(durations));
    let (buf, _s, _m) =
        IndexWriter::write_into(&ri, Cursor::new(Vec::new()), Vec::new()).unwrap();
    (buf.into_inner(), rows)
}

fn tokens(field: &str, exact: &[&str], patterns: &[&str]) -> PlanTerm {
    PlanTerm::Tokens {
        field: field.to_string(),
        exact: exact.iter().map(|s| s.to_string()).collect(),
        patterns: patterns.iter().map(|s| s.to_string()).collect(),
    }
}

fn plan(terms: Vec<PlanTerm>) -> TracePlan {
    TracePlan { terms }
}

/// Whole-file, unbounded compile — the shape most tests need.
fn compile(idx: &IndexReader<'_>, p: &TracePlan, work: &mut ScanWork) -> CompiledTracePlan {
    idx.compile_trace_plan(p, (0, idx.summary().record_count), u64::MAX, work)
        .unwrap()
        .expect("an unbounded compile cannot run out of budget")
}

/// The newest `k` matching positions in `[lo, hi)`, ascending — the
/// full-scan oracle the rank-bounded extraction must equal.
fn oracle(
    rows: &[FixtureRow],
    pred: impl Fn(&FixtureRow) -> bool,
    lo: u32,
    hi: u32,
    k: usize,
) -> Vec<u32> {
    let mut matched: Vec<u32> = (lo..hi.min(rows.len() as u32))
        .filter(|&p| pred(&rows[p as usize]))
        .collect();
    let cut = matched.len().saturating_sub(k);
    matched.split_off(cut)
}

/// Every tier (exact and regex), cross-tier conjunctions, and duration
/// bounds return exactly the oracle's newest-K, at several K and windows.
#[test]
fn tier_matrix_matches_the_oracle() {
    let (bytes, rows) = fixture();
    let idx = IndexReader::open(&bytes).unwrap();
    type Pred = Box<dyn Fn(&FixtureRow) -> bool>;
    let cases: Vec<(TracePlan, Pred)> = vec![
        // Single-term, per tier, exact.
        (
            plan(vec![tokens("l", &["a"], &[])]),
            Box::new(|r: &FixtureRow| r.l == "a"),
        ),
        (
            plan(vec![tokens("m", &["v03"], &[])]),
            Box::new(|r: &FixtureRow| r.m == "v03"),
        ),
        (
            plan(vec![tokens("h", &["w005"], &[])]),
            Box::new(|r: &FixtureRow| r.h == "w005"),
        ),
        // Multi-value OR within one field.
        (
            plan(vec![tokens("m", &["v01", "v02"], &[])]),
            Box::new(|r: &FixtureRow| r.m == "v01" || r.m == "v02"),
        ),
        // Regex per tier (full-value anchored).
        (
            plan(vec![tokens("l", &[], &["a|b"])]),
            Box::new(|_| true),
        ),
        (
            plan(vec![tokens("m", &[], &["v0[12]"])]),
            Box::new(|r: &FixtureRow| r.m == "v01" || r.m == "v02"),
        ),
        (
            plan(vec![tokens("h", &[], &["w00."])]),
            Box::new(|r: &FixtureRow| r.h.starts_with("w00")),
        ),
        // Cross-tier conjunction (low ∧ mid ∧ high).
        (
            plan(vec![
                tokens("l", &["a"], &[]),
                tokens("m", &["v02"], &[]),
                tokens("h", &[], &["w0.."]),
            ]),
            Box::new(|r: &FixtureRow| {
                r.l == "a" && r.m == "v02" && r.h.starts_with("w0")
            }),
        ),
        // Two high-card terms (the shared stream-batch pass).
        (
            plan(vec![
                tokens("h", &[], &["w0[0-5]."]),
                tokens("h2", &[], &["x0.."]),
            ]),
            Box::new(|r: &FixtureRow| {
                let d = r.h.as_bytes()[2];
                r.h.starts_with("w0") && (b'0'..=b'5').contains(&d) && r.h2.starts_with("x0")
            }),
        ),
        // Duration bounds, alone and with a token term.
        (
            plan(vec![PlanTerm::Duration {
                min_ns: Some(100),
                max_ns: Some(500),
            }]),
            Box::new(|r: &FixtureRow| (100..=500).contains(&r.dur)),
        ),
        (
            plan(vec![
                tokens("l", &["b"], &[]),
                PlanTerm::Duration {
                    min_ns: Some(1_000),
                    max_ns: None,
                },
            ]),
            Box::new(|r: &FixtureRow| r.l == "b" && r.dur >= 1_000),
        ),
        // The empty conjunction: every row.
        (TracePlan::default(), Box::new(|_| true)),
    ];

    for (ci, (p, pred)) in cases.iter().enumerate() {
        let mut work = ScanWork::default();
        let compiled = compile(&idx, p, &mut work);
        for &(lo, hi) in &[(0u32, N as u32), (10, 200), (100, 101), (0, 5)] {
            for &k in &[1usize, 5, N] {
                let mut w = ScanWork::default();
                let got = compiled.newest_in_range(lo, hi, k, &mut w);
                let want = oracle(rows.as_slice(), pred, lo, hi, k);
                assert_eq!(got, want, "case {ci}, window [{lo},{hi}), k={k}");
                assert_eq!(
                    w.rows_visited,
                    want.len() as u64,
                    "case {ci}: extraction work = emitted positions"
                );
                assert_eq!(
                    compiled.count_in_range(lo, hi),
                    oracle(rows.as_slice(), pred, lo, hi, N).len() as u64,
                    "case {ci}: count agrees with the oracle"
                );
            }
        }
    }
}

/// An absent field matches nothing and collapses any conjunction.
#[test]
fn absent_field_collapses_the_conjunction() {
    let (bytes, _) = fixture();
    let idx = IndexReader::open(&bytes).unwrap();
    let mut work = ScanWork::default();
    let compiled = compile(
        &idx,
        &plan(vec![tokens("l", &["a"], &[]), tokens("absent", &["x"], &[])]),
        &mut work,
    );
    assert_eq!(compiled.count_in_range(0, N as u32), 0);
    assert_eq!(
        compiled.newest_in_range(0, N as u32, 5, &mut work),
        Vec::<u32>::new()
    );
    // Short-circuit: the high-card scan after the collapse never ran.
    let mut work = ScanWork::default();
    compile(
        &idx,
        &plan(vec![
            tokens("absent", &["x"], &[]),
            tokens("h", &[], &["w.*"]),
        ]),
        &mut work,
    );
    assert_eq!(work.rows_visited, 0, "collapsed plan skips the SB scan");
}

/// Compilation work: dictionary terms are free; high-card terms cost one
/// shared stream-batch pass however many there are.
#[test]
fn work_counts_one_shared_stream_batch_pass() {
    let (bytes, _) = fixture();
    let idx = IndexReader::open(&bytes).unwrap();

    // Low + mid + duration: no rows visited at compile.
    let mut work = ScanWork::default();
    compile(
        &idx,
        &plan(vec![
            tokens("l", &["a"], &[]),
            tokens("m", &[], &["v.*"]),
            PlanTerm::Duration {
                min_ns: Some(0),
                max_ns: None,
            },
        ]),
        &mut work,
    );
    assert_eq!(work.rows_visited, 0, "dictionary/column terms are free");

    // One high term matching every value: the pass visits every row once.
    let mut one = ScanWork::default();
    compile(&idx, &plan(vec![tokens("h", &[], &["w.*"])]), &mut one);
    assert_eq!(one.rows_visited, N as u64);

    // TWO high terms: still ONE pass — not 2×N.
    let mut two = ScanWork::default();
    compile(
        &idx,
        &plan(vec![
            tokens("h", &[], &["w.*"]),
            tokens("h2", &[], &["x.*"]),
        ]),
        &mut two,
    );
    assert_eq!(two.rows_visited, N as u64, "shared pass counted once");

    // A narrow exact high term visits only its masked batches.
    let mut narrow = ScanWork::default();
    compile(&idx, &plan(vec![tokens("h", &["w005"], &[])]), &mut narrow);
    let batch_size = crate::stream_batch_size(N as u32);
    let rows_of_batch = |b: u32| -> u64 {
        let start = b * batch_size;
        u64::from((N as u32 - start).min(batch_size))
    };
    // Value w005 lives on rows 5 and 133 — the union of their batches.
    let mut batches: Vec<u32> = vec![5 / batch_size, 133 / batch_size];
    batches.dedup();
    let expected: u64 = batches.into_iter().map(rows_of_batch).sum();
    assert_eq!(narrow.rows_visited, expected, "masked batches only");
}

/// The rank-bounded proof (pin R3-1): with EVERY stream batch corrupted,
/// the match-all plan still compiles and extracts — it touches no row
/// data — and emission is exactly K, not the file's row count.
#[test]
fn match_all_extraction_is_rank_bounded_and_row_free() {
    let (bytes, _) = fixture();
    let mut corrupted = bytes.clone();
    {
        let cr = crate::reader::ChunkReader::open(&bytes).unwrap();
        let n = crate::num_stream_batches(cr.summary().unwrap().record_count);
        assert!(n > 0, "fixture must have stream batches");
        let base = bytes.as_ptr() as usize;
        for i in 0..n {
            let raw = cr.stream_batch_raw(i).unwrap();
            let off = raw.as_ptr() as usize - base;
            corrupted[off] ^= 0xFF;
        }
    }
    let idx = IndexReader::open(&corrupted).unwrap();
    let mut work = ScanWork::default();
    let compiled = compile(&idx, &TracePlan::default(), &mut work);
    assert_eq!(work.rows_visited, 0, "match-all compiles without row work");
    let got = compiled.newest_in_range(0, N as u32, 3, &mut work);
    assert_eq!(got, vec![N as u32 - 3, N as u32 - 2, N as u32 - 1]);
    assert_eq!(work.rows_visited, 3, "O(K) emission for match-all");
    // Low/mid dictionary terms work on the corrupted file too.
    let mut w2 = ScanWork::default();
    let low = compile(&idx, &plan(vec![tokens("l", &["a"], &[])]), &mut w2);
    assert_eq!(low.count_in_range(0, N as u32), (N / 2) as u64);
}

/// Extraction edges: k = 0, empty/inverted/overshooting windows, and
/// k exceeding the match count.
#[test]
fn extraction_edges() {
    let (bytes, rows) = fixture();
    let idx = IndexReader::open(&bytes).unwrap();
    let mut work = ScanWork::default();
    let p = plan(vec![tokens("m", &["v03"], &[])]);
    let compiled = compile(&idx, &p, &mut work);
    let matched = oracle(&rows, |r| r.m == "v03", 0, N as u32, N);

    assert!(compiled.newest_in_range(0, N as u32, 0, &mut work).is_empty());
    assert!(compiled.newest_in_range(50, 50, 5, &mut work).is_empty());
    assert!(compiled.newest_in_range(60, 50, 5, &mut work).is_empty());
    // hi past the universe clamps.
    assert_eq!(
        compiled.newest_in_range(0, u32::MAX, N, &mut work),
        matched
    );
    // k beyond the match count returns all matches.
    assert_eq!(
        compiled.newest_in_range(0, N as u32, N * 10, &mut work),
        matched
    );
    assert_eq!(compiled.count_in_range(0, u32::MAX), matched.len() as u64);
}

/// A malformed regex is a hard compile error, whichever tier the field
/// lives in (request-boundary validation upstream; defense here).
#[test]
fn malformed_pattern_fails_compilation() {
    let (bytes, _) = fixture();
    let idx = IndexReader::open(&bytes).unwrap();
    for field in ["l", "m", "h"] {
        let mut work = ScanWork::default();
        assert!(
            matches!(
                idx.compile_trace_plan(
                    &plan(vec![tokens(field, &[], &["("])]),
                    (0, N as u32),
                    u64::MAX,
                    &mut work,
                ),
                Err(crate::Error::InvalidPattern(_))
            ),
            "field {field}"
        );
    }
}

/// A duration term against a file with no DURN column (not a traces
/// file) fails loudly rather than matching nothing.
#[test]
fn duration_term_requires_the_durn_column() {
    let arena = Bump::new();
    let mut ri = RowIndex::new(&arena, 10);
    let t = ri.intern(None, "l=a");
    ri.row(1_000, &[t]);
    let (buf, _s, _m) =
        IndexWriter::write_into(&ri, Cursor::new(Vec::new()), Vec::new()).unwrap();
    let bytes = buf.into_inner();
    let idx = IndexReader::open(&bytes).unwrap();
    let mut work = ScanWork::default();
    assert!(
        idx.compile_trace_plan(
            &plan(vec![PlanTerm::Duration {
                min_ns: Some(0),
                max_ns: None,
            }]),
            (0, 1),
            u64::MAX,
            &mut work,
        )
        .is_err()
    );
}

/// The compile budget is enforced INSIDE the stream-batch scan: a
/// high-card plan whose scan would exceed the ceiling stops and returns
/// `None` (a truncated plan must never be used) with the counter
/// stopped one past the ceiling — never a whole-file overshoot.
#[test]
fn compile_budget_stops_the_stream_batch_scan() {
    let (bytes, _) = fixture();
    let idx = IndexReader::open(&bytes).unwrap();
    let p = plan(vec![tokens("h", &[], &["w.*"])]);
    let mut work = ScanWork::default();
    let out = idx
        .compile_trace_plan(&p, (0, N as u32), 10, &mut work)
        .unwrap();
    assert!(out.is_none(), "budget-truncated compile yields no plan");
    assert_eq!(work.rows_visited, 11, "stopped one past the ceiling");
    // Dictionary-only plans never consume the budget: a ceiling of 0
    // still compiles them.
    let mut w2 = ScanWork::default();
    let low = idx
        .compile_trace_plan(&plan(vec![tokens("l", &["a"], &[])]), (0, N as u32), 0, &mut w2)
        .unwrap();
    assert!(low.is_some());
    assert_eq!(w2.rows_visited, 0);
}

/// The DURN pass is clipped to the caller's range (what keeps it out of
/// the work units): positions outside the range never enter the set.
#[test]
fn duration_scan_is_clipped_to_the_range() {
    let (bytes, rows) = fixture();
    let idx = IndexReader::open(&bytes).unwrap();
    let p = plan(vec![PlanTerm::Duration {
        min_ns: Some(0),
        max_ns: None,
    }]);
    let mut work = ScanWork::default();
    let compiled = idx
        .compile_trace_plan(&p, (10, 20), u64::MAX, &mut work)
        .unwrap()
        .expect("in budget");
    assert_eq!(compiled.count_in_range(10, 20), 10);
    // Nothing outside the compile range was collected.
    assert_eq!(compiled.count_in_range(0, N as u32), 10);
    assert_eq!(
        compiled.newest_in_range(10, 20, N, &mut work),
        oracle(&rows, |_| true, 10, 20, N)
    );
}
