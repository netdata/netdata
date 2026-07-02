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

fn corpus_blobs() -> Option<Vec<Vec<u8>>> {
    let path = std::env::var("OTLP_BENCH_FILE").ok()?;
    let path = path.strip_prefix("~/").map_or_else(
        || std::path::PathBuf::from(&path),
        |rest| std::path::PathBuf::from(std::env::var("HOME").unwrap()).join(rest),
    );
    let bytes = std::fs::read(&path).unwrap_or_else(|e| panic!("read {}: {e}", path.display()));
    // Split the dump into per-request blobs (framing per
    // `ng_ingest::append_dumped_request`).
    let mut rest = &bytes[..];
    let mut blobs = Vec::new();
    while !rest.is_empty() {
        let len = u32::from_le_bytes(rest[..4].try_into().expect("length prefix")) as usize;
        blobs.push(rest[4..4 + len].to_vec());
        rest = &rest[4 + len..];
    }
    Some(blobs)
}

fn decode_all(blobs: &[Vec<u8>]) -> Vec<ExportLogsServiceRequest> {
    blobs
        .iter()
        .map(|b| ExportLogsServiceRequest::decode(&b[..]).expect("prost decode"))
        .collect()
}

/// Every stage mutates or consumes fresh input; a fixed fallback base keeps
/// normalization deterministic across iterations.
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

    g.bench_function("normalize", |b| {
        b.iter_batched(
            || pristine.clone(),
            |mut reqs| {
                for req in &mut reqs {
                    black_box(ng_flatten::normalize_log_request(req, FALLBACK_BASE_NS));
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
            ng_flatten::normalize_log_request(req, FALLBACK_BASE_NS);
        }
        reqs
    };

    g.bench_function("flatten", |b| {
        b.iter(|| {
            let mut out = Vec::with_capacity(normalized.len());
            for req in &normalized {
                out.push(black_box(ng_flatten::flatten_log_request(req)));
            }
            out
        })
    });

    let flattened: Vec<ng_flatten::FlattenedLogRequest> = normalized
        .iter()
        .map(|req| ng_flatten::flatten_log_request(req).0)
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

    // The aggregate production path over pristine requests.
    g.bench_function("prepare_log_frame", |b| {
        b.iter_batched(
            || pristine.clone(),
            |mut reqs| {
                let mut out = Vec::with_capacity(reqs.len());
                for req in &mut reqs {
                    out.push(
                        ng_flatten::prepare_log_frame(req, FALLBACK_BASE_NS).expect("prepare"),
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
