//! Ingest-pipeline baseline over a real corpus of serialized OTLP requests.
//!
//! Measures every stage of "logs request received → WAL frame payload", per
//! stage and as the aggregate production recipe:
//!
//! ```text
//! prost_decode → normalize → flatten (incl. emit-time hashes) → encode → lz4
//!                └──────────── prepare_log_frame ───────────┘
//! ```
//!
//! The legs diagnose differently: a delta in `prost_decode` points at prost's
//! OTLP wire decode, in `normalize` at the timestamp/id/JSON-body walk, in
//! `flatten` at schema-tree interning plus the emit-time `xxhash64` hashing
//! of `path=value` (what the SFST builder keys on), in `encode` at bincode
//! frame serialization, in `lz4_compress` at the WAL writer's compression.
//! `prepare_log_frame` is the aggregate `ng-ingest` pays per request — the
//! headline number; the split legs attribute a regression to one stage.
//!
//! Needs a request dump captured by `ng-ingest --dump-requests` (the
//! `u32-LE length + prost bytes` framing of `ng_ingest::append_dumped_request`;
//! the trivial reader is duplicated here because the dependency direction is
//! ng-ingest → ng-flatten):
//!
//! ```text
//! OTLP_BENCH_FILE=~/corpora/otlp/unified/capture/requests.pb \
//!     cargo bench -p ng-flatten
//! ```
//!
//! Exits quietly when `OTLP_BENCH_FILE` is unset so a plain `cargo bench`
//! run without a corpus does not fail.

use std::hint::black_box;

use criterion::{BatchSize, Criterion, criterion_group, criterion_main};
use opentelemetry_proto::tonic::collector::logs::v1::ExportLogsServiceRequest;
use prost::Message;

/// Load the `OTLP_BENCH_FILE` dump (`~/` expands against `$HOME`) and split it
/// into per-request blobs. `None` (env unset) skips the benches; an unreadable
/// path panics.
fn corpus_blobs() -> Option<Vec<Vec<u8>>> {
    let path = std::env::var("OTLP_BENCH_FILE").ok()?;
    let path = match (path.strip_prefix("~/"), std::env::var("HOME")) {
        (Some(rest), Ok(home)) => std::path::PathBuf::from(home).join(rest),
        _ => std::path::PathBuf::from(&path),
    };
    let bytes = std::fs::read(&path).unwrap_or_else(|e| panic!("read {}: {e}", path.display()));
    let mut rest = &bytes[..];
    let mut blobs = Vec::new();
    while !rest.is_empty() {
        let len = u32::from_le_bytes(rest[..4].try_into().expect("length prefix")) as usize;
        blobs.push(rest[4..4 + len].to_vec());
        rest = &rest[4 + len..];
    }
    Some(blobs)
}

/// Decode every blob with prost — one `ExportLogsServiceRequest` per dump entry.
fn decode_all(blobs: &[Vec<u8>]) -> Vec<ExportLogsServiceRequest> {
    blobs
        .iter()
        .map(|b| ExportLogsServiceRequest::decode(&b[..]).expect("prost decode"))
        .collect()
}

/// Production stamps one clock tick (`now`) as the synthetic-timestamp base; a
/// pinned constant keeps the synthesized values (`base + k`) identical across
/// iterations and legs, so the split stages stay comparable with the aggregate.
const FALLBACK_BASE_NS: u64 = 1_700_000_000_000_000_000;

fn bench(c: &mut Criterion) {
    let Some(blobs) = corpus_blobs() else {
        eprintln!("OTLP_BENCH_FILE not set; skipping ingest bench");
        return;
    };

    let pristine = decode_all(&blobs);
    let records: usize = pristine
        .iter()
        .map(|r| {
            r.resource_logs
                .iter()
                .flat_map(|rl| rl.scope_logs.iter())
                .map(|sl| sl.log_records.len())
                .sum::<usize>()
        })
        .sum();
    let raw_bytes: usize = blobs.iter().map(Vec::len).sum();
    eprintln!(
        "corpus: {} requests, {records} records, {:.1} MiB serialized",
        blobs.len(),
        raw_bytes as f64 / (1024.0 * 1024.0)
    );

    let mut g = c.benchmark_group("ingest");
    g.sample_size(10);

    g.bench_function("prost_decode", |b| {
        b.iter(|| {
            for blob in &blobs {
                black_box(ExportLogsServiceRequest::decode(&blob[..]).expect("decode"));
            }
        })
    });

    // Normalization mutates in place — a fresh copy per iteration.
    g.bench_function("normalize", |b| {
        b.iter_batched(
            || pristine.clone(),
            |mut reqs| {
                for req in &mut reqs {
                    black_box(ng_flatten::normalize_log_request(req, FALLBACK_BASE_NS, None));
                }
                reqs
            },
            BatchSize::PerIteration,
        )
    });

    // The stages below consume the normalized form (the production order).
    let normalized = {
        let mut reqs = pristine.clone();
        for req in &mut reqs {
            ng_flatten::normalize_log_request(req, FALLBACK_BASE_NS, None);
        }
        reqs
    };

    // `flatten_log_request` consumes the request by value, so each iteration
    // gets a fresh owned batch (`iter_batched`) — clone cost stays out of the
    // measurement.
    g.bench_function("flatten", |b| {
        b.iter_batched(
            || normalized.clone(),
            |reqs| {
                let mut out = Vec::with_capacity(reqs.len());
                for req in reqs {
                    out.push(black_box(ng_flatten::flatten_log_request(req)));
                }
                out
            },
            BatchSize::PerIteration,
        )
    });

    // Frozen flattened input for the encode and lz4 legs (`.0` drops the
    // sanitized-key count).
    let flattened: Vec<ng_flatten::FlattenedLogRequest> = normalized
        .iter()
        .map(|req| ng_flatten::flatten_log_request(req.clone()).0)
        .collect();

    g.bench_function("encode", |b| {
        b.iter(|| {
            let mut out = Vec::with_capacity(flattened.len());
            for f in &flattened {
                out.push(black_box(ng_flatten::encode_log_frame(f).expect("encode")));
            }
            out
        })
    });

    let encoded: Vec<Vec<u8>> = flattened
        .iter()
        .map(|f| ng_flatten::encode_log_frame(f).expect("encode"))
        .collect();

    g.bench_function("lz4_compress", |b| {
        b.iter(|| {
            for data in &encoded {
                black_box(lz4_flex::block::compress(data));
            }
        })
    });

    // The aggregate production recipe over pristine requests — exactly what
    // `ng_ingest::write_request` runs per request (bounds `None`); the WAL
    // writer adds the lz4 leg above on top of this payload.
    g.bench_function("prepare_log_frame", |b| {
        b.iter_batched(
            || pristine.clone(),
            |reqs| {
                let mut out = Vec::with_capacity(reqs.len());
                for req in reqs {
                    out.push(
                        ng_flatten::prepare_log_frame(req, FALLBACK_BASE_NS, None).expect("prepare"),
                    );
                }
                out
            },
            BatchSize::PerIteration,
        )
    });

    g.finish();
}

criterion_group!(benches, bench);
criterion_main!(benches);
