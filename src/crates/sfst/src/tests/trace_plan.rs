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
    /// Sparse low-card field: `yes` on i%4==0, `no` on i%4==2, ABSENT on
    /// odd rows — the negation/presence semantics probe.
    s: Option<String>,
    /// Mid-card numeric-valued field (`i % 50`).
    num: i64,
    /// High-card numeric-valued field (`(i % 130).5`).
    fnum: f64,
    dur: i64,
}

const N: usize = 256;

/// 256 rows, one field per tier (threshold 10): `l` low (2 values),
/// `m` mid (16), `h`/`h2` high (128 each), `s` sparse low, `num` mid
/// numeric (50), `fnum` high numeric (130), ascending timestamps
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
            s: match i % 4 {
                0 => Some("yes".to_string()),
                2 => Some("no".to_string()),
                _ => None,
            },
            num: (i % 50) as i64,
            fnum: (i % 130) as f64 + 0.5,
            dur: (i as i64) * 10,
        };
        let mut tokens = vec![
            format!("l={}", row.l),
            format!("m={}", row.m),
            format!("h={}", row.h),
            format!("h2={}", row.h2),
            format!("num={}", row.num),
            format!("fnum={}", row.fnum),
        ];
        if let Some(s) = &row.s {
            tokens.push(format!("s={s}"));
        }
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
    PlanTerm::tokens(
        field,
        exact.iter().map(|s| s.to_string()).collect(),
        patterns.iter().map(|s| s.to_string()).collect(),
    )
}

fn plan(terms: Vec<PlanTerm>) -> TracePlan {
    TracePlan { terms }
}

fn not_tokens(field: &str, exact: &[&str], patterns: &[&str]) -> PlanTerm {
    let PlanTerm::Fields {
        fields, matcher, ..
    } = tokens(field, exact, patterns)
    else {
        unreachable!()
    };
    PlanTerm::Fields {
        fields,
        matcher,
        negated: true,
    }
}

fn number(field: &str, cmp: crate::NumberCmp, values: &[f64], negated: bool) -> PlanTerm {
    PlanTerm::Fields {
        fields: vec![field.to_string()],
        matcher: crate::PlanMatcher::Number {
            cmp,
            values: values.to_vec(),
        },
        negated,
    }
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
            plan(vec![PlanTerm::duration(Some(100), Some(500))]),
            Box::new(|r: &FixtureRow| (100..=500).contains(&r.dur)),
        ),
        (
            plan(vec![
                tokens("l", &["b"], &[]),
                PlanTerm::duration(Some(1_000), None),
            ]),
            Box::new(|r: &FixtureRow| r.l == "b" && r.dur >= 1_000),
        ),
        // ── Stage B: negation (presence ∩ complement) per tier ──────
        // Sparse low field: absent rows never satisfy a negation.
        (
            plan(vec![not_tokens("s", &["yes"], &[])]),
            Box::new(|r: &FixtureRow| r.s.as_deref().is_some_and(|s| s != "yes")),
        ),
        // Negated regex on a sparse field.
        (
            plan(vec![not_tokens("s", &[], &["y.*"])]),
            Box::new(|r: &FixtureRow| r.s.as_deref().is_some_and(|s| !s.starts_with('y'))),
        ),
        // Mid and high tiers (fields present on every row).
        (
            plan(vec![not_tokens("m", &["v03", "v04"], &[])]),
            Box::new(|r: &FixtureRow| r.m != "v03" && r.m != "v04"),
        ),
        (
            plan(vec![not_tokens("h", &["w005"], &[])]),
            Box::new(|r: &FixtureRow| r.h != "w005"),
        ),
        // A negated term on an ABSENT field matches nothing.
        (plan(vec![not_tokens("absent", &["x"], &[])]), Box::new(|_| false)),
        // ── Stage B: multi-field OR (the unscoped disjunction) ──────
        (
            plan(vec![PlanTerm::Fields {
                fields: vec!["s".to_string(), "l".to_string()],
                matcher: crate::PlanMatcher::Tokens {
                    exact: vec!["yes".to_string(), "a".to_string()],
                    patterns: vec![],
                },
                negated: false,
            }]),
            Box::new(|r: &FixtureRow| {
                r.s.as_deref() == Some("yes") || r.l == "a"
            }),
        ),
        // Negated multi-field: present in either, matching in neither.
        (
            plan(vec![PlanTerm::Fields {
                fields: vec!["s".to_string(), "l".to_string()],
                matcher: crate::PlanMatcher::Tokens {
                    exact: vec!["yes".to_string(), "a".to_string()],
                    patterns: vec![],
                },
                negated: true,
            }]),
            Box::new(|r: &FixtureRow| {
                let matches =
                    r.s.as_deref() == Some("yes") || r.l == "a";
                !matches // l is always present, so presence always holds
            }),
        ),
        // ── Stage B: dictionary numerics ────────────────────────────
        (
            plan(vec![number("num", crate::NumberCmp::Gte, &[45.0], false)]),
            Box::new(|r: &FixtureRow| r.num >= 45),
        ),
        (
            plan(vec![number("fnum", crate::NumberCmp::Lt, &[3.0], false)]),
            Box::new(|r: &FixtureRow| r.fnum < 3.0),
        ),
        (
            plan(vec![number(
                "num",
                crate::NumberCmp::Eq,
                &[5.0, 7.0],
                false,
            )]),
            Box::new(|r: &FixtureRow| r.num == 5 || r.num == 7),
        ),
        // Negated numeric equality: present and no value equals.
        (
            plan(vec![number("num", crate::NumberCmp::Eq, &[5.0], true)]),
            Box::new(|r: &FixtureRow| r.num != 5),
        ),
        // Unparseable stored values never match a numeric comparison.
        (
            plan(vec![number("m", crate::NumberCmp::Gte, &[0.0], false)]),
            Box::new(|_| false),
        ),
        // ── Stage B: duration interval sets, straight and negated ───
        (
            plan(vec![PlanTerm::Duration {
                intervals: vec![(Some(0), Some(50)), (Some(1_000), Some(1_100))],
                negated: false,
            }]),
            Box::new(|r: &FixtureRow| {
                (0..=50).contains(&r.dur) || (1_000..=1_100).contains(&r.dur)
            }),
        ),
        (
            plan(vec![PlanTerm::Duration {
                intervals: vec![(Some(100), Some(100)), (Some(200), Some(200))],
                negated: true,
            }]),
            Box::new(|r: &FixtureRow| r.dur != 100 && r.dur != 200),
        ),
        // ── Stage B composed: negation ∧ numeric ∧ token ────────────
        (
            plan(vec![
                tokens("l", &["a"], &[]),
                not_tokens("m", &["v02"], &[]),
                number("num", crate::NumberCmp::Lt, &[40.0], false),
            ]),
            Box::new(|r: &FixtureRow| r.l == "a" && r.m != "v02" && r.num < 40),
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
            PlanTerm::duration(Some(0), None),
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

    // Batch masking itself is proven by
    // `narrow_high_terms_visit_only_their_masked_batches` — this 256-row
    // fixture seals into a single stream batch (num_stream_batches(256)
    // == 1), so any per-batch expectation here would equal a full scan.
}

/// Batch masking with a REAL multi-batch file: a value confined to one
/// batch visits only that batch's rows. Needs both (a) enough rows for
/// two stream batches (≥ 2·MIN_LOGS_PER_BATCH) and (b) needle values
/// living in exactly one batch — the main fixture satisfies neither
/// (256 rows, and its `h = w{i%128}` recurs in every batch at any N).
#[test]
fn narrow_high_terms_visit_only_their_masked_batches() {
    const ROWS: usize = 2048;
    let arena = Bump::new();
    let mut ri = RowIndex::new(&arena, 10);
    for i in 0..ROWS {
        // High-card field: a unique filler per row, except two needles —
        // "front" only in batch 0, "back" only in batch 1.
        let h = match i {
            5 | 133 => "front".to_string(),
            1500 | 1600 => "back".to_string(),
            _ => format!("w{i:04}"),
        };
        let kv = format!("h={h}");
        let slots = vec![ri.intern(None, &kv)];
        ri.row(1_000 + i as i64, &slots);
    }
    let (buf, _s, _m) =
        IndexWriter::write_into(&ri, Cursor::new(Vec::new()), Vec::new()).unwrap();
    let bytes = buf.into_inner();
    let idx = IndexReader::open(&bytes).unwrap();

    let batch_size = crate::stream_batch_size(ROWS as u32);
    assert_eq!(batch_size, 1024, "the fixture must seal into two batches");

    // Each needle's visit count is ONE batch — half the file. A masking
    // regression that scans every batch reports 2048 and fails.
    let mut front = ScanWork::default();
    compile(&idx, &plan(vec![tokens("h", &["front"], &[])]), &mut front);
    assert_eq!(front.rows_visited, u64::from(batch_size), "batch 0 only");

    let mut back = ScanWork::default();
    compile(&idx, &plan(vec![tokens("h", &["back"], &[])]), &mut back);
    assert_eq!(back.rows_visited, u64::from(batch_size), "batch 1 only");
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
            &plan(vec![PlanTerm::duration(Some(0), None)]),
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
    let p = plan(vec![PlanTerm::duration(Some(0), None)]);
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

/// A high-card term matching NO dictionary value collapses the
/// conjunction before the stream-batch scan runs — the other high
/// terms' scan would be wasted work.
#[test]
fn empty_high_targets_skip_the_stream_batch_scan() {
    let (bytes, _) = fixture();
    let idx = IndexReader::open(&bytes).unwrap();
    let mut work = ScanWork::default();
    let compiled = compile(
        &idx,
        &plan(vec![
            tokens("h", &["no-such-value"], &[]),
            tokens("h2", &[], &["x.*"]),
        ]),
        &mut work,
    );
    assert_eq!(compiled.count_in_range(0, N as u32), 0);
    assert_eq!(work.rows_visited, 0, "no batch was scanned");
}

/// A statically impossible subgroup (this fixture has no LNKB/EVNB
/// chunks) joins the pre-scan short-circuit: the unrelated high-card
/// scan never runs, so a provably irrelevant file cannot burn the
/// budget into a false WorkCeiling.
#[test]
fn impossible_subgroup_skips_the_budgeted_scan() {
    let (bytes, _) = fixture();
    let idx = IndexReader::open(&bytes).unwrap();
    let p = plan(vec![
        tokens("h", &[], &["w.*"]),
        PlanTerm::LinkGroup {
            conditions: vec![crate::GroupCondition::LinkSpanIds(vec![
                crate::SpanId::from([0x11; 8]),
            ])],
        },
    ]);
    let mut work = ScanWork::default();
    // Even a ZERO budget compiles: nothing needs scanning.
    let compiled = idx
        .compile_trace_plan(&p, (0, N as u32), 0, &mut work)
        .unwrap()
        .expect("no scan needed");
    assert_eq!(compiled.count_in_range(0, N as u32), 0);
    assert_eq!(work.rows_visited, 0);
    // The empty-KvId arm is proven separately by
    // `empty_event_kvid_set_short_circuits_with_the_chunk_present` —
    // THIS fixture has no EVNB chunk, so an event-group case here would
    // pass through the chunk_absent arm, not the empty-set one.
}

/// The OTHER impossible arm: the EVNB chunk is PRESENT and the events
/// field has real dictionary entries, but the queried value matches no
/// KvId — the empty matching set alone must short-circuit the budgeted
/// scan (a zero budget still compiles).
#[test]
fn empty_event_kvid_set_short_circuits_with_the_chunk_present() {
    const ROWS: usize = 128; // enough distinct h values for the high tier
    let arena = Bump::new();
    let mut ri = RowIndex::new(&arena, 10);
    let mut events = crate::EventRows::new();
    for i in 0..ROWS {
        let h = ri.intern(None, &format!("h=w{i:03}"));
        // events.name must be HIGH-TIER too: a low/mid-tier field's
        // prefilter parts are all Ready, and their AND-to-empty arm of
        // group_ready_empty short-circuits on its own — only a
        // probe-based (non-Ready) prefilter leaves the empty-KvId
        // disjunct as the load-bearing check.
        let name = ri.intern(None, &format!("events.name=e{i:03}"));
        ri.row(1_000 + i as i64, &[h, name]);
        events.push_event(1_000 + i as u64, 0, name, &[]);
        events.end_row(0);
    }
    ri.events = Some(events);
    let (buf, _s, _m) =
        IndexWriter::write_into(&ri, Cursor::new(Vec::new()), Vec::new()).unwrap();
    let bytes = buf.into_inner();
    let idx = IndexReader::open(&bytes).unwrap();
    assert!(idx.has_event_index(), "the fixture must carry EVNB");

    let p = plan(vec![
        tokens("h", &[], &["w.*"]),
        PlanTerm::EventGroup {
            conditions: vec![crate::GroupCondition::Field {
                field: "events.name".to_string(),
                matcher: crate::PlanMatcher::Tokens {
                    exact: vec!["no-such-event".to_string()],
                    patterns: vec![],
                },
            }],
        },
    ]);
    let mut work = ScanWork::default();
    let compiled = idx
        .compile_trace_plan(&p, (0, ROWS as u32), 0, &mut work)
        .unwrap()
        .expect("no scan needed");
    assert_eq!(compiled.count_in_range(0, ROWS as u32), 0);
    assert_eq!(work.rows_visited, 0);

    // Control: the value that DOES exist compiles a real plan (the
    // group is not impossible merely because it is an event group).
    let p = plan(vec![PlanTerm::EventGroup {
        conditions: vec![crate::GroupCondition::Field {
            field: "events.name".to_string(),
            matcher: crate::PlanMatcher::Tokens {
                exact: vec!["e005".to_string()],
                patterns: vec![],
            },
        }],
    }]);
    let mut work = ScanWork::default();
    let compiled = idx
        .compile_trace_plan(&p, (0, ROWS as u32), u64::MAX, &mut work)
        .unwrap()
        .expect("within budget");
    assert_eq!(compiled.count_in_range(0, ROWS as u32), 1, "only row 5 carries e005");
}
