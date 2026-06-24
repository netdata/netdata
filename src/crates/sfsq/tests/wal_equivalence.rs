//! Equivalence harness: `WalScan` vs index-then-query, over identical
//! WAL files.
//!
//! The equivalence criterion this harness enforces: for any WAL file and
//! any query, the row-scan evaluator's output must
//! be indistinguishable from indexing that WAL into an SFST and running
//! the engine. Every fixture here goes through the *production* path —
//! OTLP `ResourceLogs` → `otel_ingestor::arrow_bridge::encode` (real
//! OTAP frames, `_nd_kv_hash` sidecars included) → `wal::Writer` →
//! either `sfst_indexer::index` + `LogsShard::evaluate` or
//! `WalScan::scan` + `evaluate` — and the two shards are compared
//! component by component.
//!
//! Scope: this covers both the **statistics** shard (matched count,
//! facets, timeline, field table — `wal_data_stats_equal_whole_file_index`)
//! and the **row table** served through the live path (chunk SFSTs
//! interleaved with the row-scanned tail under the cursor order —
//! `wal_data_rows_match_whole_file_index`). The row test
//! uses monotonic timestamps so the chunked and whole-file total orders
//! coincide; equal-timestamp tie-break legitimately differs across the
//! WAL→SFST transition (the documented cursor seam).
//!
//! Corpora are generated from a seeded deterministic RNG (failures
//! reproduce by seed), sweeping timestamp regimes (monotonic, shuffled,
//! equal runs, the observed-time and ingestion-time fallback tiers),
//! multi-frame files (cross-frame pair dedup, shared resource/scope
//! attributes), multi-valued fields, exotic pairs (`=` in values, empty
//! values), and cardinality-tier boundaries. Queries sweep filters
//! (exact, OR, AND, anchored patterns), full-text terms, histogram and
//! facet field choices (present, absent, multi-valued, high-card), and
//! grid geometries (aligned, sub-window, over-extending).

use std::path::{Path, PathBuf};
use std::sync::Arc;

use file_registry::TimestampNs;
use opentelemetry_proto::tonic::common::v1::{
    AnyValue, ArrayValue, InstrumentationScope, KeyValue, any_value::Value,
};
use opentelemetry_proto::tonic::logs::v1::{LogRecord, ResourceLogs, ScopeLogs};
use opentelemetry_proto::tonic::resource::v1::Resource;

use sfsq::logs::{
    LogSource, LogsData, LogsQuery, LogsQueryBuilder, LogsShard, SfstCandidate, Source, WalScan,
    WalTail, run,
};
use sfst::{Filter, Grid};

/// `run` with inert control parameters — the equivalence harness never
/// cancels and ignores progress.
fn run_plain(sources: Vec<LogSource>, query: LogsQuery) -> LogsData {
    run(
        sources,
        query,
        tokio_util::sync::CancellationToken::new(),
        Arc::new(std::sync::atomic::AtomicUsize::new(0)),
    )
}

// ---------------------------------------------------------------------------
// Deterministic RNG (xorshift64*) — no external dependency, reproducible
// by seed.
// ---------------------------------------------------------------------------

struct Rng(u64);

impl Rng {
    fn new(seed: u64) -> Self {
        Self(seed.wrapping_mul(0x9E3779B97F4A7C15).max(1))
    }

    fn next(&mut self) -> u64 {
        let mut x = self.0;
        x ^= x >> 12;
        x ^= x << 25;
        x ^= x >> 27;
        self.0 = x;
        x.wrapping_mul(0x2545F4914F6CDD1D)
    }

    fn below(&mut self, n: u64) -> u64 {
        self.next() % n
    }

    fn chance(&mut self, percent: u64) -> bool {
        self.below(100) < percent
    }
}

// ---------------------------------------------------------------------------
// OTLP fixture builders
// ---------------------------------------------------------------------------

fn s(v: &str) -> AnyValue {
    AnyValue {
        value: Some(Value::StringValue(v.to_string())),
    }
}

fn kv(key: &str, value: AnyValue) -> KeyValue {
    KeyValue {
        key: key.to_string(),
        value: Some(value),
    }
}

fn string_array(values: &[&str]) -> AnyValue {
    AnyValue {
        value: Some(Value::ArrayValue(ArrayValue {
            values: values.iter().map(|v| s(v)).collect(),
        })),
    }
}

/// Epoch base for fixture timestamps, in seconds. Small enough that the
/// SFST summary's `u32` seconds hold comfortably.
const BASE_S: u64 = 1_000_000;
const NS: u64 = 1_000_000_000;

/// One generated corpus: the batches (one `encode` call — one WAL frame
/// — each) plus the ingestion timestamps stamped on the frames.
struct Corpus {
    batches: Vec<Vec<ResourceLogs>>,
}

/// Generate a corpus for `seed`. The timestamp regime cycles with the
/// seed so the sweep covers monotonic, shuffled, equal-run, and the
/// observed-time / ingestion-time fallback tiers.
fn gen_corpus(seed: u64) -> Corpus {
    let mut rng = Rng::new(seed);
    let num_logs = 60 + rng.below(120);
    let regime = seed % 6;

    let hosts: Vec<String> = (0..2 + rng.below(8)).map(|i| format!("host-{i}")).collect();
    let codes: Vec<String> = (0..15 + rng.below(40))
        .map(|i| format!("c{i:03}"))
        .collect();
    let levels = ["info", "error", "warn", "debug"];
    let tags = ["red", "green", "blue", "alpha", "beta"];
    let severities = ["", "INFO", "ERROR", "WARN"];
    // Multi-byte UTF-8 values, for byte-oriented regex coverage.
    let cities = ["Zürich", "São Paulo", "日本語", "naïve", "Köln"];
    // Varied body text so full-text search can hit body-flattened pairs.
    let bodies = [
        "connection reset by peer",
        "request completed in 12ms",
        "free text message",
        "user signed out",
    ];

    let mut records: Vec<LogRecord> = Vec::new();
    for i in 0..num_logs {
        // Timestamp regime; 0 means "unset" in OTLP, which exercises
        // the decode's fallback tiers identically on both sides.
        let (time_ns, observed_ns) = match regime {
            0 => ((BASE_S + i) * NS, 0),                   // monotonic event time
            1 => ((BASE_S + rng.below(num_logs)) * NS, 0), // shuffled
            2 => ((BASE_S + i / 8) * NS, 0),               // runs of equal timestamps
            3 => (0, (BASE_S + i) * NS),                   // observed-time fallback
            4 => (0, 0),                                   // ingestion-time fallback
            _ => {
                // Mixed: mostly event time, some rows falling through.
                if rng.chance(20) {
                    (0, if rng.chance(50) { (BASE_S + i) * NS } else { 0 })
                } else {
                    ((BASE_S + rng.below(num_logs)) * NS, 0)
                }
            }
        };

        let mut attributes = Vec::new();
        if rng.chance(95) {
            attributes.push(kv("level", s(levels[rng.below(4) as usize])));
        }
        if rng.chance(75) {
            attributes.push(kv(
                "host",
                s(&hosts[rng.below(hosts.len() as u64) as usize]),
            ));
        }
        if rng.chance(55) {
            attributes.push(kv(
                "code",
                s(&codes[rng.below(codes.len() as u64) as usize]),
            ));
        }
        if rng.chance(30) {
            // Multi-valued: a scalar array flattens to repeated bare-key
            // pairs (one `tags=…` pair per element).
            let n = 1 + rng.below(3) as usize;
            let mut picked: Vec<&str> = Vec::new();
            for _ in 0..n {
                picked.push(tags[rng.below(tags.len() as u64) as usize]);
            }
            attributes.push(kv("tags", string_array(&picked)));
        }
        if rng.chance(10) {
            // Value containing `=` — the first-`=` split must agree.
            attributes.push(kv("note", s("x=y")));
        }
        if rng.chance(10) {
            // Empty value (`blank=`).
            attributes.push(kv("blank", s("")));
        }
        if rng.chance(40) {
            // Multi-byte UTF-8 values: both evaluators match patterns
            // over the raw value *bytes* (`regex::bytes`), so a `.` or
            // `\w` spanning a multi-byte codepoint must behave the same
            // on each side.
            attributes.push(kv(
                "city",
                s(cities[rng.below(cities.len() as u64) as usize]),
            ));
        }

        records.push(LogRecord {
            time_unix_nano: time_ns,
            observed_time_unix_nano: observed_ns,
            severity_number: [0, 9, 17][rng.below(3) as usize],
            severity_text: severities[rng.below(4) as usize].to_string(),
            // Body text varies so a full-text query can discriminate
            // among body-flattened (`log.body=…`) pairs.
            body: if rng.chance(35) {
                Some(s(bodies[rng.below(bodies.len() as u64) as usize]))
            } else {
                None
            },
            attributes,
            ..LogRecord::default()
        });
    }

    // Split into 1–4 batches → multiple WAL frames: cross-frame pair
    // dedup and per-frame resource/scope attribute sharing both get
    // exercised. Resource attrs vary per batch (same service.name —
    // the indexer requires a single stream identity per file).
    let num_batches = 1 + rng.below(4) as usize;
    let per = records.len().div_ceil(num_batches);
    let mut batches = Vec::new();
    for (b, chunk) in records.chunks(per).enumerate() {
        let resource = Resource {
            attributes: vec![
                kv("service.name", s("harness")),
                kv("env", s(if b % 2 == 0 { "prod" } else { "dev" })),
            ],
            ..Resource::default()
        };
        let scope = if rng.chance(60) {
            Some(InstrumentationScope {
                name: format!("scope-{b}"),
                version: "1.0".to_string(),
                ..InstrumentationScope::default()
            })
        } else {
            None
        };
        batches.push(vec![ResourceLogs {
            resource: Some(resource),
            scope_logs: vec![ScopeLogs {
                scope,
                log_records: chunk.to_vec(),
                ..ScopeLogs::default()
            }],
            ..ResourceLogs::default()
        }]);
    }

    Corpus { batches }
}

// ---------------------------------------------------------------------------
// WAL writing + the two evaluation paths
// ---------------------------------------------------------------------------

/// Write the corpus as a real WAL file (production writer, LZ4 frames)
/// and return its path.
fn write_wal(dir: &Path, corpus: &Corpus) -> PathBuf {
    let seq = Arc::new(wal::SeqAllocator::ephemeral(0));
    let mut writer = wal::Writer::new(dir, wal::Config::default(), seq).expect("writer");
    let stream = otel_logs_identity::ServiceStream::new("ns", "svc");
    let part_key = otel_logs_identity::part_key(&stream);
    let content_meta = otel_logs_identity::encode_content_meta(&stream).expect("identity encodes");
    for (i, batch) in corpus.batches.iter().enumerate() {
        let (data, count) =
            otel_ingestor::arrow_bridge::encode(batch.clone()).expect("arrow encode");
        // The ingestion timestamp is the tier-3 fallback base for rows
        // with no event/observed time; +1s per frame keeps fallback
        // rows of different frames distinguishable.
        let ingestion = TimestampNs((BASE_S + 500 + i as u64) * NS);
        writer
            .write_frame(
                part_key,
                &content_meta,
                &data,
                count,
                ingestion,
                TimestampNs::ZERO,
                TimestampNs::ZERO,
            )
            .expect("write frame");
    }
    writer.shutdown_all().expect("shutdown");

    let mut wals: Vec<PathBuf> = std::fs::read_dir(dir)
        .expect("read dir")
        .filter_map(|e| e.ok())
        .map(|e| e.path())
        .filter(|p| p.extension().is_some_and(|x| x == "wal"))
        .collect();
    assert_eq!(wals.len(), 1, "corpus must land in a single WAL file");
    wals.pop().unwrap()
}

/// Index the WAL into an SFST and wrap it as an engine candidate.
fn index_candidate(wal_path: &Path, dir: &Path) -> SfstCandidate {
    let sfst_path = dir.join("harness.sfst");
    let result = sfst_indexer::index(wal_path, &sfst_path).expect("index");
    SfstCandidate {
        summary: result.summary,
        file_seq: 1,
        part: sfsq::logs::Part::Indexed(0), // sealed SFST
        source: sfsq::logs::Source::File(sfst_path),
    }
}

/// Assert the two shards are identical, component by component (better
/// failure locality than one big comparison).
fn assert_equiv(ctx: &str, via_sfst: &LogsShard, via_scan: &LogsShard) {
    assert_eq!(
        via_sfst.matched, via_scan.matched,
        "matched diverged [{ctx}]"
    );
    assert_eq!(
        via_sfst.fields, via_scan.fields,
        "field table diverged [{ctx}]"
    );
    assert_eq!(via_sfst.facets, via_scan.facets, "facets diverged [{ctx}]");
    assert_eq!(
        via_sfst.timeline, via_scan.timeline,
        "timeline diverged [{ctx}]"
    );
}

/// Run every query against both paths and assert equivalence. Returns
/// the number of queries that matched at least one row (so the caller
/// can guard against an all-vacuous matrix — a matrix where every query
/// trivially agrees at zero matches proves little).
fn check_corpus(
    label: &str,
    corpus: &Corpus,
    queries: impl Fn(&sfst::Summary) -> Vec<(String, LogsQuery)>,
) -> usize {
    let dir = tempfile::tempdir().expect("tempdir");
    let wal_path = write_wal(dir.path(), corpus);
    let candidate = index_candidate(&wal_path, dir.path());
    let scan = WalScan::scan(&wal_path).expect("scan");

    assert_eq!(
        candidate.summary.record_count as usize,
        scan.num_rows(),
        "row count diverged [{label}]"
    );

    let mut nonzero = 0usize;
    for (qlabel, query) in queries(&candidate.summary) {
        let via_sfst = LogsShard::evaluate(&candidate, &query);
        let via_scan = scan.evaluate(&query);
        assert_equiv(&format!("{label} / {qlabel}"), &via_sfst, &via_scan);
        if via_sfst.matched > 0 {
            nonzero += 1;
        }
    }
    nonzero
}

// ---------------------------------------------------------------------------
// The query matrix
// ---------------------------------------------------------------------------

/// Queries derived from the file's summary range: filters (exact / OR /
/// AND / patterns / absent fields), full-text terms, histogram and facet
/// choices, and several grid geometries.
fn query_matrix(summary: &sfst::Summary) -> Vec<(String, LogsQuery)> {
    let start = summary.min_timestamp_s as i64 * NS as i64;
    let span = ((summary.max_timestamp_s - summary.min_timestamp_s) as i64 + 1) * NS as i64;

    let g_one = Grid::new(start, span, 1);
    let g_eight = Grid::new(start, (span + 7) / 8, 8);
    // A sub-window over the middle half — exercises clipping.
    let g_sub = Grid::new(start + span / 4, (span / 8).max(1), 4);
    // Over-extending on both sides — empty outer buckets.
    let g_wide = Grid::new(start - 5 * NS as i64, (span + 3) / 4 + 3 * NS as i64, 4);

    let b = LogsQueryBuilder::new;
    let mut out: Vec<(String, LogsQuery)> = Vec::new();

    out.push(("defaults".into(), b(g_eight).build()));
    out.push((
        "exact".into(),
        b(g_eight)
            .filter(Filter::new().select("level", "error"))
            .build(),
    ));
    out.push((
        "or-and".into(),
        b(g_eight)
            .filter(
                Filter::new()
                    .select("level", "error")
                    .select("level", "info")
                    .select("host", "host-1"),
            )
            .histogram_field("level")
            .facet_fields(vec!["level".into(), "host".into(), "tags".into()])
            .build(),
    ));
    out.push((
        "patterns".into(),
        b(g_eight)
            .filter(
                Filter::new()
                    .select_pattern("code", "c0.*")
                    .select_pattern("level", "(info|warn)"),
            )
            .facet_fields(vec!["code".into(), "level".into()])
            .build(),
    ));
    out.push((
        "anchored-miss".into(),
        // `err` must not match `error` (full-value anchoring).
        b(g_one)
            .filter(Filter::new().select_pattern("level", "err"))
            .build(),
    ));
    out.push((
        "absent-field".into(),
        b(g_eight)
            .filter(Filter::new().select("nope", "x"))
            .histogram_field("host")
            .facet_fields(vec!["level".into(), "nope".into()])
            .build(),
    ));
    out.push((
        "fulltext".into(),
        b(g_eight)
            .query("err")
            .facet_fields(vec!["level".into()])
            .build(),
    ));
    out.push((
        "fulltext-key-scoped".into(),
        b(g_eight)
            .query("host=host-[02]")
            .histogram_field("host")
            .build(),
    ));
    out.push((
        "fulltext-plus-filter".into(),
        b(g_sub)
            .query("o")
            .filter(Filter::new().select("level", "info"))
            .histogram_field("level")
            .facet_fields(vec!["host".into(), "level".into()])
            .build(),
    ));
    out.push((
        "hist-multivalued".into(),
        b(g_eight)
            .histogram_field("tags")
            .facet_fields(vec!["tags".into()])
            .build(),
    ));
    out.push((
        "hist-absent".into(),
        b(g_eight).histogram_field("nope").build(),
    ));
    out.push((
        "hist-projected-severity".into(),
        b(g_wide)
            .histogram_field("severity_number")
            .facet_fields(vec!["severity_text".into(), "scope.name".into()])
            .build(),
    ));
    out.push((
        "exotic-pairs".into(),
        b(g_one)
            .filter(Filter::new().select("note", "x=y"))
            .facet_fields(vec!["note".into(), "blank".into()])
            .histogram_field("blank")
            .build(),
    ));
    out.push((
        "resource-attr".into(),
        b(g_eight)
            .filter(Filter::new().select("env", "prod"))
            .histogram_field("env")
            .build(),
    ));
    out.push((
        // Byte-oriented regex over multi-byte UTF-8 values: `.` spans a
        // codepoint boundary differently in `regex::bytes`, so both
        // evaluators must agree on the same value bytes.
        "non-ascii-pattern".into(),
        b(g_eight)
            .filter(Filter::new().select_pattern("city", "Z.rich|.*Paulo"))
            .facet_fields(vec!["city".into()])
            .build(),
    ));
    out.push((
        // Full-text against body-flattened pairs (`log.body=…`).
        "fulltext-body".into(),
        b(g_eight).query("request|reset").build(),
    ));
    out.push((
        // Pre-epoch (negative) bucket origin: the bucket arithmetic
        // `(ts − start) / width` must agree with the SFST partition.
        "negative-grid".into(),
        b(Grid::new(
            -3600 * NS as i64,
            (start + span + 3600 * NS as i64) / 6,
            6,
        ))
        .histogram_field("level")
        .build(),
    ));
    out.push((
        // A window entirely after the data: every count is zero, the
        // timeline is all-empty — both paths must produce the same
        // empty shard, not merely "no rows".
        "window-past-data".into(),
        b(Grid::new(start + span + NS as i64, NS as i64, 4))
            .facet_fields(vec!["level".into()])
            .histogram_field("level")
            .build(),
    ));
    out
}

// ---------------------------------------------------------------------------
// The tests
// ---------------------------------------------------------------------------

#[test]
fn random_corpora_match_index_then_query() {
    // Coverage facts accumulated across the sweep — a fixture refactor
    // that quietly stops exercising a claimed dimension trips these,
    // rather than silently shrinking what the equivalence proves.
    let mut multi_frame = false;
    let mut has_multivalued = false;
    let mut has_non_ascii = false;
    let mut total_nonzero = 0usize;

    for seed in 1..=18 {
        let corpus = gen_corpus(seed);

        if corpus.batches.len() > 1 {
            multi_frame = true;
        }
        for batch in &corpus.batches {
            for rl in batch {
                for sl in &rl.scope_logs {
                    for lr in &sl.log_records {
                        // A multi-valued field is an array attribute with
                        // ≥2 elements: `encode` flattens it into that many
                        // repeated bare-key pairs. (In the raw record it's
                        // still one `KeyValue` holding an `ArrayValue`.)
                        let multivalued = lr.attributes.iter().any(|a| {
                            matches!(
                                a.value.as_ref().and_then(|v| v.value.as_ref()),
                                Some(Value::ArrayValue(arr)) if arr.values.len() > 1
                            )
                        });
                        if multivalued {
                            has_multivalued = true;
                        }
                        if lr.attributes.iter().any(|a| a.key == "city") {
                            has_non_ascii = true;
                        }
                    }
                }
            }
        }

        total_nonzero += check_corpus(&format!("seed={seed}"), &corpus, query_matrix);
    }

    // Each corpus is split into frames per its own RNG draw; one frame
    // is a `ResourceLogs` per batch, and `write_wal` concatenates the
    // batches into a single multi-frame WAL — so a multi-frame corpus
    // is what actually exercises cross-frame pair dedup.
    assert!(multi_frame, "no multi-frame corpus generated");
    assert!(
        has_multivalued,
        "no multi-valued (repeated-key) field generated"
    );
    assert!(has_non_ascii, "no non-ASCII attribute generated");
    // Across 18 corpora × the query matrix, the great majority of
    // queries must actually match rows — otherwise the sweep is
    // agreeing trivially at zero.
    assert!(
        total_nonzero >= 18 * 8,
        "matrix is too sparse: only {total_nonzero} matching queries across the sweep"
    );
}

#[test]
fn cardinality_tier_boundaries_match() {
    // One corpus with fields engineered to land exactly on the tier
    // boundaries (threshold 100): 99 distinct → low, 100 → mid,
    // 999 → mid, 1000+ → high. `unique` is per-log-unique (high).
    //
    // 6000 logs → 5 stream batches (one per ~1024 logs, capped at 8),
    // so the high-card field's *multi-batch* scan — the batch-mask
    // union and cross-batch position gather — is exercised, not just
    // the single-batch degenerate case.
    let num_logs: usize = 6000;
    let mut records = Vec::new();
    for i in 0..num_logs {
        records.push(LogRecord {
            time_unix_nano: (BASE_S as usize + i) as u64 * NS,
            severity_text: "INFO".to_string(),
            attributes: vec![
                kv("low99", s(&format!("v{:02}", i % 99))),
                kv("mid100", s(&format!("v{:03}", i % 100))),
                kv("mid999", s(&format!("v{:03}", i % 999))),
                kv("unique", s(&format!("u{i:05}"))),
            ],
            ..LogRecord::default()
        });
    }
    let corpus = Corpus {
        batches: records
            .chunks(400)
            .map(|chunk| {
                vec![ResourceLogs {
                    resource: Some(Resource {
                        attributes: vec![kv("service.name", s("harness"))],
                        ..Resource::default()
                    }),
                    scope_logs: vec![ScopeLogs {
                        log_records: chunk.to_vec(),
                        ..ScopeLogs::default()
                    }],
                    ..ResourceLogs::default()
                }]
            })
            .collect(),
    };

    check_corpus("tier-boundaries", &corpus, |summary| {
        let start = summary.min_timestamp_s as i64 * NS as i64;
        let span = ((summary.max_timestamp_s - summary.min_timestamp_s) as i64 + 1) * NS as i64;
        let g = Grid::new(start, (span + 3) / 4, 4);
        let b = LogsQueryBuilder::new;
        vec![
            // Field-table equality (tiers) is asserted on every query;
            // these exercise the high-card refusal paths on both sides.
            ("hist-high".into(), b(g).histogram_field("unique").build()),
            (
                "facet-mix".into(),
                b(g).facet_fields(vec![
                    "low99".into(),
                    "mid100".into(),
                    "mid999".into(),
                    "unique".into(),
                ])
                .histogram_field("mid100")
                .build(),
            ),
            (
                "filter-high-card-field".into(),
                b(g).filter(Filter::new().select("unique", "u00042"))
                    .facet_fields(vec!["low99".into()])
                    .histogram_field("low99")
                    .build(),
            ),
            (
                "pattern-on-high".into(),
                b(g).filter(Filter::new().select_pattern("unique", "u000.[13]"))
                    .build(),
            ),
            (
                // Full-text over a high-card field exercises the SFST's
                // `query_positions` high-card branch (dictionary scan +
                // stream-batch position gather), distinct from the
                // `field_values_or` path the two queries above hit.
                "fulltext-on-high".into(),
                b(g).query("unique=u00[0-9]42").build(),
            ),
        ]
    });
}

#[test]
fn named_regressions_match() {
    // The two divergences found by external review of the row scan,
    // plus the multi-valued `unset` fixture — kept as permanent
    // equivalence cases.
    let records = vec![
        LogRecord {
            time_unix_nano: BASE_S * NS,
            attributes: vec![
                kv("a", s("b=c")), // pair `a=b=c`: field `a`, value `b=c`
                kv("lang", string_array(&["en", "fr"])),
                kv("svc", s("x")),
            ],
            ..LogRecord::default()
        },
        LogRecord {
            time_unix_nano: (BASE_S + 1) * NS,
            attributes: vec![kv("lang", s("en")), kv("svc", s("y"))],
            ..LogRecord::default()
        },
        LogRecord {
            time_unix_nano: (BASE_S + 2) * NS,
            attributes: vec![kv("svc", s("x"))],
            ..LogRecord::default()
        },
    ];
    let corpus = Corpus {
        batches: vec![vec![ResourceLogs {
            resource: Some(Resource {
                attributes: vec![kv("service.name", s("harness"))],
                ..Resource::default()
            }),
            scope_logs: vec![ScopeLogs {
                log_records: records,
                ..ScopeLogs::default()
            }],
            ..ResourceLogs::default()
        }]],
    };

    check_corpus("regressions", &corpus, |summary| {
        let start = summary.min_timestamp_s as i64 * NS as i64;
        let span = ((summary.max_timestamp_s - summary.min_timestamp_s) as i64 + 1) * NS as i64;
        let g = Grid::new(start, span, 1);
        let b = LogsQueryBuilder::new;
        vec![
            // Divergence #1: a requested field name containing `=`
            // concatenates to an existing pair string — must match
            // nothing on both paths.
            (
                "eq-in-field-name".into(),
                b(g).filter(Filter::new().select("a=b", "c")).build(),
            ),
            (
                "eq-in-value".into(),
                b(g).filter(Filter::new().select("a", "b=c")).build(),
            ),
            // Divergence #2: a malformed pattern on an *absent* field is
            // resolved to the empty set before compiling on both paths
            // (normal zero-match shard, not a degrade).
            (
                "bad-pattern-absent-field".into(),
                b(g).filter(Filter::new().select_pattern("missing", "(unclosed"))
                    .facet_fields(vec!["svc".into()])
                    .histogram_field("lang")
                    .build(),
            ),
            // Multi-valued `unset`: row 0 carries two `lang` values,
            // row 2 none.
            (
                "multivalued-unset".into(),
                b(g).histogram_field("lang")
                    .facet_fields(vec!["lang".into()])
                    .build(),
            ),
        ]
    });
}

#[test]
fn index_range_whole_file_matches_disk_index() {
    // The in-memory range index over the whole durable prefix must
    // produce the very same SFST the disk indexer produces: identical
    // frames in, identical deterministic build, identical bytes out.
    // (Interior-range / chunk-boundary correctness is exercised once the
    // boundary scan lands and can supply real chunk offsets.)
    for seed in 1..=6 {
        let corpus = gen_corpus(seed);
        let dir = tempfile::tempdir().expect("tempdir");
        let wal_path = write_wal(dir.path(), &corpus);
        let file_len = std::fs::metadata(&wal_path).unwrap().len();

        let (mem_summary, mem_bytes) = sfst_indexer::index_range(
            &wal_path,
            wal::FrameRange::new(wal::HEADER_SIZE as u64, file_len),
        )
        .expect("index_range");

        let sfst_path = dir.path().join("disk.sfst");
        let disk = sfst_indexer::index(&wal_path, &sfst_path).expect("index");
        let disk_bytes = std::fs::read(&sfst_path).unwrap();

        assert_eq!(
            mem_summary.record_count, disk.summary.record_count,
            "seed={seed}: record_count diverged"
        );
        assert_eq!(
            mem_bytes, disk_bytes,
            "seed={seed}: in-memory range index differs from the disk index"
        );
    }
}

#[test]
fn index_range_interior_split_partitions_logs() {
    // Split a multi-frame WAL at a real frame boundary (from the
    // header scan) into two chunks, and confirm the chunks partition
    // the file's logs exactly — no record lost or double-counted across
    // the split. (Full-shard equality of merge-vs-whole would *not*
    // hold: merge_field_tables is conservative on cardinality, so a
    // field's tier can legitimately differ; we check the
    // partition-invariant facts instead.)
    let mut checked = false;
    for seed in 1..=10 {
        let corpus = gen_corpus(seed);
        let dir = tempfile::tempdir().expect("tempdir");
        let wal_path = write_wal(dir.path(), &corpus);
        let file_len = std::fs::metadata(&wal_path).unwrap().len();

        let frames = wal::scan_frame_boundaries(
            &wal_path,
            wal::FrameRange::new(wal::HEADER_SIZE as u64, file_len),
        )
        .unwrap();
        if frames.len() < 2 {
            continue;
        }

        // Split after the middle frame — a real frame boundary.
        let split = frames[frames.len() / 2 - 1].end_offset;
        let (a_sum, a_bytes) = sfst_indexer::index_range(
            &wal_path,
            wal::FrameRange::new(wal::HEADER_SIZE as u64, split),
        )
        .unwrap();
        let (b_sum, b_bytes) =
            sfst_indexer::index_range(&wal_path, wal::FrameRange::new(split, file_len)).unwrap();

        let whole_path = dir.path().join("whole.sfst");
        let whole = sfst_indexer::index(&wal_path, &whole_path).unwrap();

        // Logs partition exactly, by both the index and the scan.
        assert_eq!(
            a_sum.record_count + b_sum.record_count,
            whole.summary.record_count,
            "seed={seed}: chunk record_count don't sum to the whole"
        );
        let scan_entries: u32 = frames.iter().map(|f| f.entry_count).sum();
        assert_eq!(
            scan_entries, whole.summary.record_count,
            "seed={seed}: scanned entry counts don't sum to the whole"
        );

        // Both chunks are valid, queryable SFSTs.
        sfst::IndexReader::open(&a_bytes).unwrap();
        sfst::IndexReader::open(&b_bytes).unwrap();

        // matched is a partition-invariant: a low-card filter over a
        // window covering everything must match the same rows whether
        // counted on the whole file or summed over the two chunks.
        let a_cand = candidate_from_bytes(dir.path(), "a.sfst", a_sum, &a_bytes);
        let b_cand = candidate_from_bytes(dir.path(), "b.sfst", b_sum, &b_bytes);
        let whole_cand = SfstCandidate {
            summary: whole.summary.clone(),
            file_seq: 3,
            part: sfsq::logs::Part::Indexed(0), // sealed SFST
            source: sfsq::logs::Source::File(whole_path),
        };

        let start = whole.summary.min_timestamp_s as i64 * NS as i64;
        let span = ((whole.summary.max_timestamp_s - whole.summary.min_timestamp_s) as i64 + 1)
            * NS as i64;
        for (qlabel, q) in [
            (
                "all",
                LogsQueryBuilder::new(Grid::new(start, span, 1)).build(),
            ),
            (
                "level=error",
                LogsQueryBuilder::new(Grid::new(start, span, 1))
                    .filter(Filter::new().select("level", "error"))
                    .build(),
            ),
        ] {
            let whole_matched = LogsShard::evaluate(&whole_cand, &q).matched;
            let summed =
                LogsShard::evaluate(&a_cand, &q).matched + LogsShard::evaluate(&b_cand, &q).matched;
            assert_eq!(
                whole_matched, summed,
                "seed={seed} q={qlabel}: matched count not partitioned across the split"
            );
        }

        checked = true;
        break;
    }
    assert!(checked, "no multi-frame corpus found in seeds 1..=10");
}

/// Write SFST `bytes` to `dir/name` and wrap as a candidate with the
/// given summary (the in-memory range index has no file of its own).
fn candidate_from_bytes(
    dir: &Path,
    name: &str,
    summary: sfst::Summary,
    bytes: &[u8],
) -> SfstCandidate {
    let path = dir.join(name);
    std::fs::write(&path, bytes).unwrap();
    SfstCandidate {
        summary,
        file_seq: 1,
        part: sfsq::logs::Part::Indexed(0), // sealed SFST
        source: sfsq::logs::Source::File(path),
    }
}

#[test]
fn wal_data_stats_equal_whole_file_index() {
    // Statistics equivalence: querying an active WAL through the live
    // path — chunk SFSTs (built by index_range, fed in as Source::Memory)
    // plus a row-scanned tail, folded by the engine — must yield the
    // same *statistics* (matched, facets, histogram) as indexing the
    // whole WAL and querying that. Only stats: M4a paginates on-disk
    // SFSTs only, and chunk/whole field-table cardinality can legitimately
    // differ (conservative merge), so rows/fields are out of scope here.
    let header = wal::HEADER_SIZE as u64;
    let mut any_matched = false;

    for seed in 1..=8 {
        let corpus = gen_corpus(seed);
        let dir = tempfile::tempdir().expect("tempdir");
        let wal_path = write_wal(dir.path(), &corpus);
        let file_len = std::fs::metadata(&wal_path).unwrap().len();

        // Ground truth: the whole WAL as one on-disk SFST.
        let whole = index_candidate(&wal_path, dir.path());
        let total = whole.summary.record_count;
        let start = whole.summary.min_timestamp_s as i64 * NS as i64;
        let span = ((whole.summary.max_timestamp_s - whole.summary.min_timestamp_s) as i64 + 1)
            * NS as i64;

        let queries = || -> Vec<(String, LogsQuery)> {
            let b = LogsQueryBuilder::new;
            vec![
                (
                    "all".into(),
                    b(Grid::new(start, (span + 7) / 8, 8))
                        .histogram_field("level")
                        .facet_fields(vec!["level".into()])
                        .build(),
                ),
                (
                    "level=error".into(),
                    b(Grid::new(start, span, 1))
                        .filter(Filter::new().select("level", "error"))
                        .histogram_field("level")
                        .facet_fields(vec!["level".into()])
                        .build(),
                ),
            ]
        };

        assert!(total > 0, "seed={seed}: fixture WAL has no records");

        // Two splits: all-tail (no chunks → pure WalScan path) and a
        // small threshold (chunks + maybe a tail → the merge path).
        // Partitioned by the production rule (`wal::prefix`), so the
        // harness exercises exactly the splits the ledger would make.
        for min_entries in [u64::MAX, 25] {
            let chunks = wal::prefix::chunk_boundaries(
                &wal::scan_frame_boundaries(&wal_path, wal::FrameRange::new(header, file_len))
                    .unwrap(),
                header,
                min_entries,
            );
            // Confirm each split exercises the path it's meant to: the
            // MAX threshold yields no chunks (everything is tail), and
            // the small threshold yields at least one chunk (the
            // Source::Memory merge path is actually under test).
            if min_entries == u64::MAX {
                assert!(
                    chunks.is_empty(),
                    "seed={seed}: MAX threshold should make all tail"
                );
            } else {
                assert!(
                    !chunks.is_empty(),
                    "seed={seed}: small threshold should produce a chunk"
                );
            }

            let mut live_candidates: Vec<SfstCandidate> = Vec::new();
            for (i, chunk) in chunks.iter().enumerate() {
                let (summary, bytes) = sfst_indexer::index_range(&wal_path, chunk.range).unwrap();
                live_candidates.push(SfstCandidate {
                    summary,
                    file_seq: i as u64,
                    part: sfsq::logs::Part::Indexed(0),
                    source: Source::Memory(Arc::new(bytes)),
                });
            }
            let tail_begin = wal::prefix::tail_start(&chunks, header);
            let tails = vec![WalTail {
                file_seq: 9999,
                path: wal_path.clone(),
                range: wal::FrameRange::new(tail_begin, file_len),
            }];

            for (qlabel, q) in queries() {
                let live = run_plain(
                    sources(live_candidates_clone(&live_candidates), tails_clone(&tails)),
                    q.clone(),
                );
                let truth = run_plain(vec![LogSource::Sfst(clone_candidate(&whole))], q);
                let ctx = format!("seed={seed} min_entries={min_entries} q={qlabel}");
                assert_eq!(live.matched, truth.matched, "matched [{ctx}]");
                assert_eq!(live.facets, truth.facets, "facets [{ctx}]");
                assert_eq!(live.histogram, truth.histogram, "histogram [{ctx}]");
                if truth.matched > 0 {
                    any_matched = true;
                }
            }
        }
    }
    // Guard against a silently-vacuous sweep (every query matching zero
    // on both sides would "pass" while testing nothing).
    assert!(any_matched, "no query matched any rows across the sweep");
}

// Combine the two source kinds into the single list `run` now takes.
fn sources(candidates: Vec<SfstCandidate>, tails: Vec<WalTail>) -> Vec<LogSource> {
    candidates
        .into_iter()
        .map(LogSource::Sfst)
        .chain(tails.into_iter().map(LogSource::Tail))
        .collect()
}

// run() consumes its candidates; these clone the small fixtures so the
// loop can reuse them across queries.
fn clone_candidate(c: &SfstCandidate) -> SfstCandidate {
    SfstCandidate {
        summary: c.summary.clone(),
        file_seq: c.file_seq,
        part: c.part,
        source: c.source.clone(),
    }
}
fn live_candidates_clone(cs: &[SfstCandidate]) -> Vec<SfstCandidate> {
    cs.iter().map(clone_candidate).collect()
}
fn tails_clone(ts: &[WalTail]) -> Vec<WalTail> {
    ts.iter()
        .map(|t| WalTail {
            file_seq: t.file_seq,
            path: t.path.clone(),
            range: t.range,
        })
        .collect()
}

#[test]
fn wal_data_rows_match_whole_file_index() {
    // Row equivalence: the row table served from the live path
    // (chunk SFSTs interleaved with the row-scanned tail under the
    // cursor order) must match the rows from indexing the whole WAL.
    //
    // Timestamps are strictly monotonic here on purpose: equal-timestamp
    // rows tie-break by (seq, part, position), which legitimately
    // differs between a chunked WAL and its eventual single SFST (the
    // documented WAL->SFST cursor seam). With distinct timestamps the two
    // total orders coincide, so we can assert exact row order.
    let header = wal::HEADER_SIZE as u64;
    let levels = ["info", "error", "warn"];
    let records: Vec<LogRecord> = (0..200)
        .map(|i| LogRecord {
            time_unix_nano: (BASE_S + i) * NS,
            severity_text: "INFO".to_string(),
            attributes: vec![
                kv("level", s(levels[(i % 3) as usize])),
                kv("row", s(&format!("r{i:04}"))),
            ],
            ..LogRecord::default()
        })
        .collect();
    // 40 records per frame; min_entries 50 → 2 chunks of 80 + an 40-row
    // tail, so the page interleaves chunk and tail rows.
    let corpus = Corpus {
        batches: records
            .chunks(40)
            .map(|chunk| {
                vec![ResourceLogs {
                    resource: Some(Resource {
                        attributes: vec![kv("service.name", s("harness"))],
                        ..Resource::default()
                    }),
                    scope_logs: vec![ScopeLogs {
                        log_records: chunk.to_vec(),
                        ..ScopeLogs::default()
                    }],
                    ..ResourceLogs::default()
                }]
            })
            .collect(),
    };

    let dir = tempfile::tempdir().expect("tempdir");
    let wal_path = write_wal(dir.path(), &corpus);
    let file_len = std::fs::metadata(&wal_path).unwrap().len();
    let whole = index_candidate(&wal_path, dir.path());

    let chunks = wal::prefix::chunk_boundaries(
        &wal::scan_frame_boundaries(&wal_path, wal::FrameRange::new(header, file_len)).unwrap(),
        header,
        50,
    );
    assert_eq!(chunks.len(), 2, "fixture should split into 2 chunks");
    let tail_begin = wal::prefix::tail_start(&chunks, header);
    assert!(
        tail_begin < file_len,
        "fixture should leave a non-empty tail"
    );

    let mut live: Vec<SfstCandidate> = Vec::new();
    for (i, chunk) in chunks.iter().enumerate() {
        let (summary, bytes) = sfst_indexer::index_range(&wal_path, chunk.range).unwrap();
        live.push(SfstCandidate {
            summary,
            file_seq: 1,
            part: sfsq::logs::Part::Indexed(i as u32),
            source: Source::Memory(Arc::new(bytes)),
        });
    }
    let tails = vec![WalTail {
        file_seq: 1,
        path: wal_path.clone(),
        range: wal::FrameRange::new(tail_begin, file_len),
    }];

    let start = whole.summary.min_timestamp_s as i64 * NS as i64;
    let span =
        ((whole.summary.max_timestamp_s - whole.summary.min_timestamp_s) as i64 + 1) * NS as i64;

    let rows_of = |data: &sfsq::logs::LogsData| -> Vec<sfst::MaterializedRow> {
        data.rows.iter().map(|(_, row)| row.clone()).collect()
    };

    for (qlabel, q) in [
        (
            "all",
            LogsQueryBuilder::new(Grid::new(start, span, 1))
                .limit(300)
                .build(),
        ),
        (
            "level=error",
            LogsQueryBuilder::new(Grid::new(start, span, 1))
                .filter(Filter::new().select("level", "error"))
                .limit(300)
                .build(),
        ),
        (
            // Forward exercises the `<=` partition_point (vs backward's
            // `<`) and the page reversal in finalize.
            "forward",
            LogsQueryBuilder::new(Grid::new(start, span, 1))
                .direction(sfsq::logs::Direction::Forward)
                .limit(300)
                .build(),
        ),
    ] {
        let live_rows = rows_of(&run_plain(
            sources(live_candidates_clone(&live), tails_clone(&tails)),
            q.clone(),
        ));
        let whole_rows = rows_of(&run_plain(vec![LogSource::Sfst(clone_candidate(&whole))], q));
        assert!(!live_rows.is_empty(), "q={qlabel}: no rows");
        assert_eq!(
            live_rows, whole_rows,
            "q={qlabel}: live rows != whole-file rows"
        );
    }
}

#[test]
fn index_range_empty_range_is_a_valid_zero_log_sfst() {
    // A zero-frame range (start == end) builds a valid, parseable SFST
    // with no logs — the degenerate end of the build path. No real
    // caller passes this (chunks need >= 16K entries), but the behavior
    // should be well-defined rather than a panic.
    let corpus = gen_corpus(1);
    let dir = tempfile::tempdir().expect("tempdir");
    let wal_path = write_wal(dir.path(), &corpus);

    let (summary, bytes) = sfst_indexer::index_range(
        &wal_path,
        wal::FrameRange::new(wal::HEADER_SIZE as u64, wal::HEADER_SIZE as u64),
    )
    .expect("index_range over an empty range");
    assert_eq!(summary.record_count, 0);
    sfst::IndexReader::open(&bytes).expect("empty-range SFST parses");
}

#[test]
fn single_row_degenerate_corpus_matches() {
    // The smallest non-empty file: one record, one bucket. Exercises
    // the degenerate end of every loop on both sides.
    let corpus = Corpus {
        batches: vec![vec![ResourceLogs {
            resource: Some(Resource {
                attributes: vec![kv("service.name", s("harness"))],
                ..Resource::default()
            }),
            scope_logs: vec![ScopeLogs {
                log_records: vec![LogRecord {
                    time_unix_nano: BASE_S * NS,
                    severity_text: "INFO".to_string(),
                    attributes: vec![kv("level", s("info"))],
                    ..LogRecord::default()
                }],
                ..ScopeLogs::default()
            }],
            ..ResourceLogs::default()
        }]],
    };

    check_corpus("single-row", &corpus, |summary| {
        let start = summary.min_timestamp_s as i64 * NS as i64;
        let b = LogsQueryBuilder::new;
        vec![
            // Matches the one row.
            ("match".into(), b(Grid::new(start, NS as i64, 1)).build()),
            // A window before the only row: zero matches, empty shard.
            (
                "miss-before".into(),
                b(Grid::new(start - 10 * NS as i64, NS as i64, 4)).build(),
            ),
            (
                "filtered-out".into(),
                b(Grid::new(start, NS as i64, 1))
                    .filter(Filter::new().select("level", "error"))
                    .build(),
            ),
        ]
    });
}
