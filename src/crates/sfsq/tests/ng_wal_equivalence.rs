//! Equivalence harness for the **ng-flatten** logs path: `WalScan::scan_flattened`
//! (active-WAL tail) vs an SFST built from the same frames by
//! `ng_index::build_sfst` (sealed).
//!
//! This harness proves that, for any WAL of ng-flatten frames and any query, the
//! tail row-scan's shard is indistinguishable from indexing those frames into an
//! SFST and querying the engine — so tail and sealed results agree by construction.
//!
//! Both sides consume the **same** ng-flatten frame through the **same**
//! `ng_flatten::build_kv` renderer and the **same** `Record.ts`, so parity here is
//! structural, not coincidental. Corpora are seeded/deterministic (failures
//! reproduce by seed) and sweep: multi-frame files, multi-valued scalar arrays
//! (`attributes.tags[]`), array-of-structs (`attributes.endpoints[].host` —
//! collapsed, not positional), nested kvlists, a polymorphic Int/Str path, exotic
//! pairs (`=` in value, empty value), non-ASCII bytes, body text, and the projected
//! scalar fields (`severity_number`/`severity_text`). Queries sweep filters (exact,
//! OR/AND, anchored patterns), full-text, facet/histogram field choices, and grid
//! geometries.
//!
//! Note on timestamps: this harness feeds `flatten_log_request` directly (no
//! ingest normalization), so the fixture builder applies the same
//! event→observed→clock rule
//! before flattening, mirroring production. The clock fallback is a deterministic
//! counter here (vs `ng-ingest`'s monotonic wall clock) — value differs, ordering
//! does not, and it does not affect tail-vs-sealed parity (both read one frozen
//! `Record.ts`).

use std::path::Path;
use std::sync::Arc;

use file_registry::TimestampNs;
use opentelemetry_proto::tonic::collector::logs::v1::ExportLogsServiceRequest;
use opentelemetry_proto::tonic::common::v1::{
    AnyValue, ArrayValue, InstrumentationScope, KeyValue, KeyValueList, any_value::Value,
};
use opentelemetry_proto::tonic::logs::v1::{LogRecord, ResourceLogs, ScopeLogs};
use opentelemetry_proto::tonic::resource::v1::Resource;

use sfsq::logs::{
    Direction, LogSource, LogsData, LogsQuery, LogsQueryBuilder, LogsShard, SfstCandidate, Source,
    WalScan, WalTail, run,
};
use sfst::{Filter, Grid, IndexReader, MaterializedRow};

// ---------------------------------------------------------------------------
// Deterministic RNG (xorshift64*) — reproducible by seed, no external dep.
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

fn int_val(n: i64) -> AnyValue {
    AnyValue {
        value: Some(Value::IntValue(n)),
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

fn kvlist(pairs: Vec<KeyValue>) -> AnyValue {
    AnyValue {
        value: Some(Value::KvlistValue(KeyValueList { values: pairs })),
    }
}

fn struct_array(items: Vec<AnyValue>) -> AnyValue {
    AnyValue {
        value: Some(Value::ArrayValue(ArrayValue { values: items })),
    }
}

const BASE_S: u64 = 1_000_000;
const NS: u64 = 1_000_000_000;

/// One generated corpus: the batches (one `flatten_log_request` call — one WAL
/// frame — each).
struct Corpus {
    batches: Vec<Vec<ResourceLogs>>,
}

/// Generate a corpus for `seed`. The timestamp regime cycles with the seed so
/// the sweep covers monotonic, shuffled, equal-run, and the observed/clock
/// fallback tiers — the fallback is applied *here* (ng-flatten reads only
/// `time_unix_nano`), mirroring `ng_flatten::normalize_log_request`.
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
    let cities = ["Zürich", "São Paulo", "日本語", "naïve", "Köln"];
    let bodies = [
        "connection reset by peer",
        "request completed in 12ms",
        "free text message",
        "user signed out",
    ];

    // Deterministic clock fallback (mirrors ng-ingest's monotonic clock, but
    // reproducible): strictly increasing, well after the event-time band.
    let mut clock = (BASE_S + 900) * NS;
    let mut next_clock = || {
        clock += 1;
        clock
    };

    let mut records: Vec<LogRecord> = Vec::new();
    for i in 0..num_logs {
        let (time_ns, observed_ns) = match regime {
            0 => ((BASE_S + i) * NS, 0),
            1 => ((BASE_S + rng.below(num_logs)) * NS, 0),
            2 => ((BASE_S + i / 8) * NS, 0),
            3 => (0, (BASE_S + i) * NS),
            4 => (0, 0),
            _ => {
                if rng.chance(20) {
                    (0, if rng.chance(50) { (BASE_S + i) * NS } else { 0 })
                } else {
                    ((BASE_S + rng.below(num_logs)) * NS, 0)
                }
            }
        };
        // ng-ingest's rule: time else observed else clock — applied at ingest,
        // not in flatten. Resolve here so `Record.ts` is concrete and sane.
        let resolved_time = if time_ns != 0 {
            time_ns
        } else if observed_ns != 0 {
            observed_ns
        } else {
            next_clock()
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
            // Multi-valued scalar array → repeated `attributes.tags[]` pairs.
            let n = 1 + rng.below(3) as usize;
            let picked: Vec<&str> = (0..n)
                .map(|_| tags[rng.below(tags.len() as u64) as usize])
                .collect();
            attributes.push(kv("tags", string_array(&picked)));
        }
        if rng.chance(25) {
            // Array-of-structs → collapsed `attributes.endpoints[].host` /
            // `[].port` (collapsed, not positional `.0.host`).
            let h0 = &hosts[rng.below(hosts.len() as u64) as usize];
            let h1 = &hosts[rng.below(hosts.len() as u64) as usize];
            attributes.push(kv(
                "endpoints",
                struct_array(vec![
                    kvlist(vec![kv("host", s(h0)), kv("port", int_val(443))]),
                    kvlist(vec![kv("host", s(h1)), kv("port", int_val(80))]),
                ]),
            ));
        }
        if rng.chance(20) {
            // Nested kvlist → `attributes.meta.region`.
            attributes.push(kv(
                "meta",
                kvlist(vec![kv(
                    "region",
                    s(if rng.chance(50) { "eu" } else { "us" }),
                )]),
            ));
        }
        if rng.chance(20) {
            // Polymorphic path: `attributes.mixed` alternates Int / Str.
            let v = if rng.chance(50) {
                int_val(rng.below(50) as i64)
            } else {
                s(&format!("v{}", rng.below(50)))
            };
            attributes.push(kv("mixed", v));
        }
        if rng.chance(10) {
            attributes.push(kv("note", s("x=y"))); // value containing `=`
        }
        if rng.chance(10) {
            attributes.push(kv("blank", s(""))); // empty value
        }
        if rng.chance(40) {
            attributes.push(kv(
                "city",
                s(cities[rng.below(cities.len() as u64) as usize]),
            ));
        }

        records.push(LogRecord {
            time_unix_nano: resolved_time,
            observed_time_unix_nano: observed_ns,
            severity_number: [0, 9, 17][rng.below(3) as usize],
            severity_text: severities[rng.below(4) as usize].to_string(),
            body: if rng.chance(35) {
                Some(s(bodies[rng.below(bodies.len() as u64) as usize]))
            } else {
                None
            },
            attributes,
            ..LogRecord::default()
        });
    }

    // Split into 1–4 batches → multiple WAL frames (cross-frame pair dedup +
    // shared resource/scope attrs). Same stream identity (service.name) per file.
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
// WAL writing (ng-flatten frames) + the two evaluation paths
// ---------------------------------------------------------------------------

/// Write the corpus as a flattened-frame WAL (one frame per batch) using the
/// `ng-flatten` encode + the production `wal::Writer`.
fn write_flattened_wal(dir: &Path, corpus: &Corpus) -> std::path::PathBuf {
    let seq = std::sync::Arc::new(wal::SeqAllocator::ephemeral(0));
    let mut writer = wal::Writer::new(dir, wal::Config::default(), seq, 0).expect("writer");
    for (i, batch) in corpus.batches.iter().enumerate() {
        let request = ExportLogsServiceRequest {
            resource_logs: batch.clone(),
        };
        let (flattened, _) = ng_flatten::flatten_log_request(request);
        let bytes = ng_flatten::encode_log_frame(&flattened).expect("encode frame");
        let count: usize = batch
            .iter()
            .flat_map(|rl| &rl.scope_logs)
            .map(|sl| sl.log_records.len())
            .sum();
        let ingestion = TimestampNs((BASE_S + 500 + i as u64) * NS);
        writer
            .write_frame(
                0,
                &[],
                &bytes,
                count,
                ingestion,
                TimestampNs::ZERO,
                TimestampNs::ZERO,
            )
            .expect("write frame");
    }
    writer.shutdown_all().expect("shutdown");

    let mut wals: Vec<std::path::PathBuf> = std::fs::read_dir(dir)
        .expect("read dir")
        .filter_map(|e| e.ok())
        .map(|e| e.path())
        .filter(|p| p.extension().is_some_and(|x| x == "wal"))
        .collect();
    assert_eq!(wals.len(), 1, "corpus must land in a single WAL file");
    wals.pop().unwrap()
}

/// Build the sealed SFST from the flattened WAL via `ng_index::build_sfst` and
/// wrap it as an engine candidate (summary read back from the written file).
fn ng_index_candidate(wal_dir: &Path) -> SfstCandidate {
    let sfst_path = wal_dir.join("harness-ng.sfst");
    ng_index::build_sfst(wal_dir, &sfst_path, &ng_index::Metrics::new()).expect("build_sfst");
    let bytes = std::fs::read(&sfst_path).expect("read sfst");
    let summary = IndexReader::open(&bytes)
        .expect("open sfst")
        .summary()
        .clone();
    SfstCandidate {
        summary,
        file_seq: 1,
        part: sfsq::logs::Part::Indexed(0),
        source: Source::File(sfst_path),
    }
}

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

/// Run every query against both ng paths and assert equivalence. Returns the
/// number of queries that matched at least one row (guards against a vacuous
/// matrix that agrees only at zero matches).
fn check_corpus(
    label: &str,
    corpus: &Corpus,
    queries: impl Fn(&sfst::Summary) -> Vec<(String, LogsQuery)>,
) -> usize {
    let dir = tempfile::tempdir().expect("tempdir");
    let wal_path = write_flattened_wal(dir.path(), corpus);
    let candidate = ng_index_candidate(dir.path());
    let scan = WalScan::scan_flattened(&wal_path).expect("scan_flattened");

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
// Query matrix (ng path field names: `attributes.*`, `resource.attributes.*`,
// `scope.*`, top-level `severity_*` / `body`, `[]` array collapse)
// ---------------------------------------------------------------------------

fn query_matrix(summary: &sfst::Summary) -> Vec<(String, LogsQuery)> {
    let start = summary.min_timestamp_s as i64 * NS as i64;
    let span = ((summary.max_timestamp_s - summary.min_timestamp_s) as i64 + 1) * NS as i64;

    let g_one = Grid::new(start, span, 1);
    let g_eight = Grid::new(start, (span + 7) / 8, 8);
    let g_sub = Grid::new(start + span / 4, (span / 8).max(1), 4);
    let g_wide = Grid::new(start - 5 * NS as i64, (span + 3) / 4 + 3 * NS as i64, 4);

    let b = LogsQueryBuilder::new;
    let mut out: Vec<(String, LogsQuery)> = Vec::new();

    out.push(("defaults".into(), b(g_eight).build()));
    out.push((
        "exact".into(),
        b(g_eight)
            .filter(Filter::new().select("attributes.level", "error"))
            .build(),
    ));
    out.push((
        "or-and".into(),
        b(g_eight)
            .filter(
                Filter::new()
                    .select("attributes.level", "error")
                    .select("attributes.level", "info")
                    .select("attributes.host", "host-1"),
            )
            .histogram_field("attributes.level")
            .facet_fields(vec![
                "attributes.level".into(),
                "attributes.host".into(),
                "attributes.tags[]".into(),
            ])
            .build(),
    ));
    out.push((
        "patterns".into(),
        b(g_eight)
            .filter(
                Filter::new()
                    .select_pattern("attributes.code", "c0.*")
                    .select_pattern("attributes.level", "(info|warn)"),
            )
            .facet_fields(vec!["attributes.code".into(), "attributes.level".into()])
            .build(),
    ));
    out.push((
        "anchored-miss".into(),
        b(g_one)
            .filter(Filter::new().select_pattern("attributes.level", "err"))
            .build(),
    ));
    out.push((
        "absent-field".into(),
        b(g_eight)
            .filter(Filter::new().select("nope", "x"))
            .histogram_field("attributes.host")
            .facet_fields(vec!["attributes.level".into(), "nope".into()])
            .build(),
    ));
    out.push((
        "fulltext".into(),
        b(g_eight)
            .query("err")
            .facet_fields(vec!["attributes.level".into()])
            .build(),
    ));
    out.push((
        "fulltext-key-scoped".into(),
        b(g_eight)
            .query("attributes.host=host-[02]")
            .histogram_field("attributes.host")
            .build(),
    ));
    out.push((
        "fulltext-plus-filter".into(),
        b(g_sub)
            .query("o")
            .filter(Filter::new().select("attributes.level", "info"))
            .histogram_field("attributes.level")
            .facet_fields(vec!["attributes.host".into(), "attributes.level".into()])
            .build(),
    ));
    out.push((
        "hist-multivalued".into(),
        b(g_eight)
            .histogram_field("attributes.tags[]")
            .facet_fields(vec!["attributes.tags[]".into()])
            .build(),
    ));
    out.push((
        "array-of-structs".into(),
        b(g_eight)
            .histogram_field("attributes.endpoints[].host")
            .facet_fields(vec![
                "attributes.endpoints[].host".into(),
                "attributes.endpoints[].port".into(),
            ])
            .build(),
    ));
    out.push((
        "nested-kvlist".into(),
        b(g_eight)
            .filter(Filter::new().select("attributes.meta.region", "eu"))
            .facet_fields(vec!["attributes.meta.region".into()])
            .build(),
    ));
    out.push((
        "polymorphic".into(),
        b(g_eight)
            .facet_fields(vec!["attributes.mixed".into()])
            .histogram_field("attributes.mixed")
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
            .filter(Filter::new().select("attributes.note", "x=y"))
            .facet_fields(vec!["attributes.note".into(), "attributes.blank".into()])
            .histogram_field("attributes.blank")
            .build(),
    ));
    out.push((
        "resource-attr".into(),
        b(g_eight)
            .filter(Filter::new().select("resource.attributes.env", "prod"))
            .histogram_field("resource.attributes.env")
            .build(),
    ));
    out.push((
        "non-ascii-pattern".into(),
        b(g_eight)
            .filter(Filter::new().select_pattern("attributes.city", "Z.rich|.*Paulo"))
            .facet_fields(vec!["attributes.city".into()])
            .build(),
    ));
    out.push((
        "fulltext-body".into(),
        b(g_eight).query("request|reset").build(),
    ));
    out.push((
        "negative-grid".into(),
        b(Grid::new(
            -3600 * NS as i64,
            (start + span + 3600 * NS as i64) / 6,
            6,
        ))
        .histogram_field("attributes.level")
        .build(),
    ));
    out.push((
        "window-past-data".into(),
        b(Grid::new(start + span + NS as i64, NS as i64, 4))
            .facet_fields(vec!["attributes.level".into()])
            .histogram_field("attributes.level")
            .build(),
    ));
    out
}

// ---------------------------------------------------------------------------
// The test
// ---------------------------------------------------------------------------

#[test]
fn ng_flattened_tail_matches_sealed_index() {
    // Coverage facts accumulated across the sweep — a fixture refactor that
    // quietly stops exercising a claimed dimension trips these.
    let mut multi_frame = false;
    let mut has_multivalued = false;
    let mut has_array_of_structs = false;
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
                        for a in &lr.attributes {
                            if let Some(Value::ArrayValue(arr)) =
                                a.value.as_ref().and_then(|v| v.value.as_ref())
                            {
                                if arr.values.len() > 1 {
                                    has_multivalued = true;
                                }
                                if arr.values.iter().any(|e| {
                                    matches!(e.value.as_ref(), Some(Value::KvlistValue(_)))
                                }) {
                                    has_array_of_structs = true;
                                }
                            }
                            if a.key == "city" {
                                has_non_ascii = true;
                            }
                        }
                    }
                }
            }
        }

        total_nonzero += check_corpus(&format!("seed={seed}"), &corpus, query_matrix);
    }

    assert!(multi_frame, "no multi-frame corpus generated");
    assert!(
        has_multivalued,
        "no multi-valued (repeated-key) field generated"
    );
    assert!(has_array_of_structs, "no array-of-structs field generated");
    assert!(has_non_ascii, "no non-ASCII attribute generated");
    assert!(
        total_nonzero >= 18 * 8,
        "matrix is too sparse: only {total_nonzero} matching queries across the sweep"
    );
}

// ---------------------------------------------------------------------------
// Run-level equivalence (the live production path): chunk SFSTs built by
// `ng_index::build_sfst_range` (fed as Source::Memory) + a row-scanned tail
// (`scan_flattened`), folded by `run`, must match indexing the whole WAL — the
// path the ledger actually drives (build_sfst_range chunks + scan_flattened
// tails → run).
// ---------------------------------------------------------------------------

fn run_plain(sources: Vec<LogSource>, query: LogsQuery) -> LogsData {
    run(
        sources,
        query,
        tokio_util::sync::CancellationToken::new(),
        Arc::new(std::sync::atomic::AtomicUsize::new(0)),
    )
}

fn sources(candidates: Vec<SfstCandidate>, tails: Vec<WalTail>) -> Vec<LogSource> {
    candidates
        .into_iter()
        .map(LogSource::Sfst)
        .chain(tails.into_iter().map(LogSource::Tail))
        .collect()
}

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
fn ng_run_stats_equal_whole_file_index() {
    // Statistics equivalence over the live path: chunk SFSTs (built by
    // build_sfst_range, fed as Source::Memory) + a row-scanned tail, folded by
    // `run`, must yield the same matched/facets/histogram as indexing the whole
    // WAL. Only stats (pagination is on-disk-SFST only; chunk/whole field-table
    // cardinality can legitimately differ under the conservative merge).
    let header = wal::HEADER_SIZE as u64;
    let mut any_matched = false;

    for seed in 1..=8 {
        let corpus = gen_corpus(seed);
        let dir = tempfile::tempdir().expect("tempdir");
        let wal_path = write_flattened_wal(dir.path(), &corpus);
        let file_len = std::fs::metadata(&wal_path).unwrap().len();

        let whole = ng_index_candidate(dir.path());
        let total = whole.summary.record_count;
        let start = whole.summary.min_timestamp_s as i64 * NS as i64;
        let span = ((whole.summary.max_timestamp_s - whole.summary.min_timestamp_s) as i64 + 1)
            * NS as i64;
        assert!(total > 0, "seed={seed}: fixture WAL has no records");

        let queries = || -> Vec<(String, LogsQuery)> {
            let b = LogsQueryBuilder::new;
            vec![
                (
                    "all".into(),
                    b(Grid::new(start, (span + 7) / 8, 8))
                        .histogram_field("attributes.level")
                        .facet_fields(vec!["attributes.level".into()])
                        .build(),
                ),
                (
                    "level=error".into(),
                    b(Grid::new(start, span, 1))
                        .filter(Filter::new().select("attributes.level", "error"))
                        .histogram_field("attributes.level")
                        .facet_fields(vec!["attributes.level".into()])
                        .build(),
                ),
            ]
        };

        // All-tail (no chunks → pure scan_flattened) and a small threshold
        // (chunks + tail → the merge path), partitioned by the production rule.
        for min_entries in [u64::MAX, 25] {
            let chunks = wal::prefix::chunk_boundaries(
                &wal::scan_frame_boundaries(&wal_path, wal::FrameRange::new(header, file_len))
                    .unwrap(),
                header,
                min_entries,
            );
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
                let (summary, bytes) = ng_index::build_sfst_range(&wal_path, chunk.range).unwrap();
                live_candidates.push(SfstCandidate {
                    summary,
                    // Mirror production (handler.rs): chunks of one WAL share
                    // file_seq and differ by part, so the cursor treats them as
                    // chunks of one file, not distinct files.
                    file_seq: 1,
                    part: sfsq::logs::Part::Indexed(i as u32),
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
    assert!(any_matched, "no query matched any rows across the sweep");
}

#[test]
fn ng_run_rows_match_whole_file_index() {
    // Row equivalence over the live path: rows served from chunk SFSTs
    // interleaved with the scan_flattened tail (under the cursor order) must
    // match the rows from indexing the whole WAL. Strictly-monotonic timestamps
    // so the chunked and whole-file total orders coincide (equal-ts tie-break is
    // the documented WAL->SFST cursor seam, out of scope).
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
    let wal_path = write_flattened_wal(dir.path(), &corpus);
    let file_len = std::fs::metadata(&wal_path).unwrap().len();
    let whole = ng_index_candidate(dir.path());

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
        let (summary, bytes) = ng_index::build_sfst_range(&wal_path, chunk.range).unwrap();
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

    let rows_of = |data: &LogsData| -> Vec<MaterializedRow> {
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
                .filter(Filter::new().select("attributes.level", "error"))
                .limit(300)
                .build(),
        ),
        (
            "forward",
            LogsQueryBuilder::new(Grid::new(start, span, 1))
                .direction(Direction::Forward)
                .limit(300)
                .build(),
        ),
    ] {
        let live_rows = rows_of(&run_plain(
            sources(live_candidates_clone(&live), tails_clone(&tails)),
            q.clone(),
        ));
        let whole_rows = rows_of(&run_plain(
            vec![LogSource::Sfst(clone_candidate(&whole))],
            q,
        ));
        assert!(!live_rows.is_empty(), "q={qlabel}: no rows");
        assert_eq!(
            live_rows, whole_rows,
            "q={qlabel}: live rows != whole-file rows"
        );
    }
}

#[test]
fn duplicate_tail_file_seq_is_skipped_not_double_served() {
    // Tail cursors are routed by `file_seq` alone, so each WAL must
    // contribute at most one tail. A caller violating that invariant must not
    // double-count stats or misroute rows: `run` drops the duplicate at its
    // entry (with a warning) and results equal the single-tail run.
    let records: Vec<LogRecord> = (0..20)
        .map(|i| LogRecord {
            time_unix_nano: (BASE_S + i) * NS,
            attributes: vec![kv("row", s(&format!("r{i:02}")))],
            ..LogRecord::default()
        })
        .collect();
    let corpus = Corpus {
        batches: vec![vec![ResourceLogs {
            scope_logs: vec![ScopeLogs {
                log_records: records,
                ..ScopeLogs::default()
            }],
            ..ResourceLogs::default()
        }]],
    };
    let dir = tempfile::tempdir().expect("tempdir");
    let wal_path = write_flattened_wal(dir.path(), &corpus);
    let file_len = std::fs::metadata(&wal_path).unwrap().len();
    let tail_dup = || WalTail {
        file_seq: 7,
        path: wal_path.clone(),
        range: wal::FrameRange::new(wal::HEADER_SIZE as u64, file_len),
    };

    let start = BASE_S as i64 * NS as i64;
    let span = 20 * NS as i64;
    let q = LogsQueryBuilder::new(Grid::new(start, span, 1))
        .limit(100)
        .build();

    let single = run_plain(sources(vec![], vec![tail_dup()]), q.clone());
    assert_eq!(single.matched, 20, "fixture sanity: all rows in the tail");

    let dup = run_plain(sources(vec![], tails_clone(&[tail_dup(), tail_dup()])), q);
    assert_eq!(dup.matched, single.matched, "duplicate tail double-counted");
    assert_eq!(dup.rows, single.rows, "duplicate tail changed served rows");
    assert_eq!(dup.histogram, single.histogram);
}
