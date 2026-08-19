//! Read-path decode baseline over a real corpus SFST.
//!
//! Measures, per chunk kind, the **full typed decode** and the **zstd-only
//! decompression** of the same chunk bytes — the split that prices the
//! zero-copy candidates (borrowed views eliminate the materialization on top
//! of zstd; uncompressed storage eliminates both). Plus `IndexReader::open`
//! (the hot prefix: SUMR + META + field table + PRIM) and the query-level
//! operations a real query pays.
//!
//! Needs a corpus file and the `test-util` feature (the raw-chunk legs go
//! through [`ChunkReader`]):
//!
//! ```text
//! SFST_BENCH_FILE=~/corpora/sfst/unified/<file>.sfst \
//!     cargo bench -p sfst --features test-util
//! ```
//!
//! Exits quietly when `SFST_BENCH_FILE` is unset so a plain `cargo bench`
//! run without a corpus does not fail.

use std::hint::black_box;

use criterion::{Criterion, criterion_group, criterion_main};
use sfst::{ChunkReader, Filter, Grid, IndexReader};

fn corpus() -> Option<Vec<u8>> {
    let path = std::env::var("SFST_BENCH_FILE").ok()?;
    let path = path.strip_prefix("~/").map_or_else(
        || std::path::PathBuf::from(&path),
        |rest| std::path::PathBuf::from(std::env::var("HOME").unwrap()).join(rest),
    );
    Some(std::fs::read(&path).unwrap_or_else(|e| panic!("read {}: {e}", path.display())))
}

/// Decode a chunk payload exactly the way the crate's reader does
/// (zstd → bincode with the standard config).
fn unpack<T: serde::de::DeserializeOwned>(raw: &[u8]) -> T {
    let decompressed = zstd::decode_all(raw).expect("zstd");
    bincode::serde::decode_from_slice(&decompressed, bincode::config::standard())
        .expect("bincode")
        .0
}

/// A `field=value` pair that actually exists in the file's primary FST, for
/// the filter benches — data-dependent, resolved at runtime.
fn sample_low_card_pair(idx: &IndexReader) -> Option<(String, String)> {
    let low = idx
        .field_table()
        .iter()
        .find(|f| matches!(f.tier, sfst::FieldTier::Low))?;
    let pairs = idx.primary_prefix(format!("{}=", low.name).as_bytes());
    let (key, _) = pairs.first()?;
    let kv = String::from_utf8_lossy(key);
    let (f, v) = kv.split_once('=')?;
    Some((f.to_string(), v.to_string()))
}

fn bench_decode(c: &mut Criterion) {
    let Some(data) = corpus() else {
        eprintln!("SFST_BENCH_FILE not set; skipping decode benches");
        return;
    };
    let idx = IndexReader::open(&data).expect("open corpus");
    let raw = ChunkReader::open(&data).expect("open corpus (chunk level)");
    let num_batches = raw.num_stream_batches().expect("num batches");
    let num_high = raw.num_high().expect("num high");
    let num_mid = raw.num_mid().expect("num mid");
    let span_ns = {
        let s = idx.summary();
        (s.min_timestamp_s as i64 * 1_000_000_000)..((s.max_timestamp_s as i64 + 1) * 1_000_000_000)
    };

    // ── Hot prefix ──────────────────────────────────────────────────
    c.bench_function("open_index_reader", |b| {
        b.iter(|| IndexReader::open(black_box(&data)).unwrap())
    });
    c.bench_function("read_summary", |b| {
        b.iter(|| sfst::read_summary(black_box(&data)).unwrap())
    });

    // ── Per-chunk: full decode vs zstd-only ─────────────────────────
    let mut g = c.benchmark_group("chunk");
    g.sample_size(20);

    g.bench_function("tims_full", |b| b.iter(|| raw.timestamps().unwrap()));
    g.bench_function("tims_zstd_only", |b| {
        b.iter(|| zstd::decode_all(raw.timestamps_raw().unwrap()).unwrap())
    });

    g.bench_function("stream_batches_full_all", |b| {
        b.iter(|| {
            for i in 0..num_batches {
                black_box(raw.stream_batch(i).unwrap());
            }
        })
    });
    g.bench_function("stream_batches_zstd_only_all", |b| {
        b.iter(|| {
            for i in 0..num_batches {
                black_box(zstd::decode_all(raw.stream_batch_raw(i).unwrap()).unwrap());
            }
        })
    });

    // Per-row columns the logs pipeline always supplies.
    g.bench_function("trace_ids_full", |b| b.iter(|| raw.trace_ids().unwrap()));
    g.bench_function("observed_ts_full", |b| {
        b.iter(|| raw.observed_timestamps().unwrap())
    });
    g.bench_function("flags_full", |b| b.iter(|| raw.flags().unwrap()));

    if num_high > 0 {
        g.bench_function("high_fields_full_all", |b| {
            b.iter(|| {
                for i in 0..num_high {
                    black_box(unpack::<sfst::HighField>(raw.high_field_raw(i).unwrap()));
                }
            })
        });
        g.bench_function("high_fields_zstd_only_all", |b| {
            b.iter(|| {
                for i in 0..num_high {
                    black_box(zstd::decode_all(raw.high_field_raw(i).unwrap()).unwrap());
                }
            })
        });
    }
    if num_mid > 0 {
        g.bench_function("mid_fields_zstd_only_all", |b| {
            b.iter(|| {
                for i in 0..num_mid {
                    black_box(zstd::decode_all(raw.mid_field_raw(i).unwrap()).unwrap());
                }
            })
        });
    }
    g.bench_function("primary_zstd_only", |b| {
        b.iter(|| zstd::decode_all(raw.primary_raw().unwrap()).unwrap())
    });
    g.finish();

    // ── Query-level ─────────────────────────────────────────────────
    let mut q = c.benchmark_group("query");
    q.sample_size(20);

    if let Some((field, value)) = sample_low_card_pair(&idx) {
        let filter = Filter::new().select(&field, &value);
        q.bench_function("filter_count", |b| {
            b.iter(|| {
                let bf = idx.compile_filter(black_box(&filter), None).unwrap();
                idx.matched_count(&bf, span_ns.clone()).unwrap()
            })
        });

        let empty = idx.compile_filter(&Filter::new(), None).unwrap();
        let facet_fields: Vec<String> = idx
            .field_table()
            .iter()
            .filter(|f| !f.is_high_card())
            .take(3)
            .map(|f| f.name.clone())
            .collect();
        q.bench_function("facets_3_fields", |b| {
            b.iter(|| idx.facets(&facet_fields, &empty, span_ns.clone()).unwrap())
        });

        let grid = Grid::new(span_ns.start, (span_ns.end - span_ns.start) / 60, 60);
        q.bench_function("timeline_60_buckets", |b| {
            b.iter(|| idx.timeline(&field, &empty, grid).unwrap())
        });

        let page: Vec<u32> = (0..100.min(idx.total_logs())).collect();
        q.bench_function("materialize_100_rows", |b| {
            b.iter(|| idx.materialize_rows(black_box(&page)).unwrap())
        });
    } else {
        eprintln!("no low-card field in corpus; skipping query benches");
    }
    q.finish();
}

criterion_group!(benches, bench_decode);
criterion_main!(benches);
