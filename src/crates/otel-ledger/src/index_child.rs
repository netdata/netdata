//! Indexing one WAL file in a process of its own.
//!
//! `otel-plugin index` runs [`run`]: it builds the index file of one closed
//! WAL file with the signal's `ng-index` builder, the same one the in-process
//! [`Indexer`](crate::indexer::Indexer) calls, and writes its answer, an
//! [`IndexedFile`], to stdout. The process that started it reads the answer
//! back with [`IndexedFile::decode`]. Both halves of the answer live here so
//! that they cannot drift apart.
//!
//! The answer is the only thing a successful child writes to stdout. A child
//! that fails writes nothing there: it reports the error on stderr and exits
//! with a non-zero status (`otel-plugin`'s `main`). A child ended by a signal
//! leaves no answer at all; its parent learns that from the wait status.
//!
//! A child that dies midway leaves the WAL file untouched and at most a
//! temporary file beside the index path, which startup recovery removes: the
//! builders write through `file_registry`'s atomic file.

use std::io::Write;
use std::path::Path;

use anyhow::{Context, Result};
use otel_bridge::signals::Signal;
use serde::{Deserialize, Serialize};

/// What indexing one WAL file produced: the answer of an index child.
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct IndexedFile {
    /// The index file's summary, as the registry tracks it.
    pub summary: sfst::Summary,
    /// Size of the written index file in bytes.
    pub size: u64,
}

impl IndexedFile {
    /// The bytes an index child writes to stdout.
    fn encode(&self) -> Result<Vec<u8>> {
        bincode::serde::encode_to_vec(self, bincode::config::standard())
            .context("encoding the index child's answer")
    }

    /// Read an index child's answer from everything it wrote to stdout.
    pub fn decode(answer: &[u8]) -> Result<Self> {
        let (indexed, read) =
            bincode::serde::decode_from_slice::<Self, _>(answer, bincode::config::standard())
                .context("decoding the index child's answer")?;

        anyhow::ensure!(
            read == answer.len(),
            "the index child's answer has {} bytes after its end",
            answer.len() - read,
        );
        Ok(indexed)
    }
}

/// Build the index file of the WAL file at `wal_path` at `sfst_path` and
/// write the answer to `answer`.
///
/// The body of `otel-plugin index`. Nothing is written to `answer` when the
/// build fails.
pub fn run(
    signal: Signal,
    wal_path: &Path,
    sfst_path: &Path,
    answer: &mut impl Write,
) -> Result<()> {
    let build = match signal {
        Signal::Logs => ng_index::build_sfst_file,
        Signal::Traces => ng_index::build_sfst_traces_file,
    };

    let (summary, size) = build(wal_path, sfst_path, &ng_index::Metrics::new())
        .with_context(|| format!("indexing {}", wal_path.display()))?;

    let indexed = IndexedFile { summary, size };
    answer
        .write_all(&indexed.encode()?)
        .and_then(|()| answer.flush())
        .context("writing the index child's answer")
}

#[cfg(test)]
mod tests {
    use std::path::PathBuf;
    use std::sync::Arc;

    use file_registry::TimestampNs;
    use opentelemetry_proto::tonic::collector::logs::v1::ExportLogsServiceRequest;
    use opentelemetry_proto::tonic::common::v1::{AnyValue, any_value::Value};
    use opentelemetry_proto::tonic::logs::v1::{LogRecord, ResourceLogs, ScopeLogs};

    use super::*;
    use crate::ledger::traces_fixtures::{otlp_req, write_traces_wal};

    /// A logs WAL file under `dir` holding one frame of `records` records.
    fn write_logs_wal(dir: &Path, records: usize) -> PathBuf {
        let log_records = (0..records)
            .map(|i| LogRecord {
                time_unix_nano: 1_700_000_000_000_000_000 + i as u64,
                body: Some(AnyValue {
                    value: Some(Value::StringValue(format!("record {i}"))),
                }),
                ..Default::default()
            })
            .collect();
        let request = ExportLogsServiceRequest {
            resource_logs: vec![ResourceLogs {
                scope_logs: vec![ScopeLogs {
                    log_records,
                    ..Default::default()
                }],
                ..Default::default()
            }],
        };

        let mut writer = wal::Writer::new(
            dir,
            wal::Config::default(),
            Arc::new(wal::SeqAllocator::ephemeral(0)),
            wal::FileStamp {
                pipeline_id: Signal::Logs.pipeline_id(),
                payload_format: ng_flatten::LOG_FRAME_PAYLOAD_FORMAT,
            },
            wal::test_identity(),
        )
        .unwrap();
        let (flattened, _) = ng_flatten::flatten_log_request(request);
        writer
            .write_frame(
                0,
                &[],
                &ng_flatten::encode_log_frame(&flattened).unwrap(),
                wal::FrameMeta {
                    entry_count: records,
                    ingestion_ns: TimestampNs(1),
                    log_ts_range: None,
                },
            )
            .unwrap();
        writer.shutdown_all().unwrap();

        std::fs::read_dir(dir)
            .unwrap()
            .map(|entry| entry.unwrap().path())
            .find(|path| path.extension().is_some_and(|ext| ext == "wal"))
            .expect("a wal file was written")
    }

    /// The child's answer and index file are those of a direct call of
    /// `build`.
    fn assert_same_as_direct_call(signal: Signal, wal_path: &Path, build: crate::indexer::SealFn) {
        let dir = tempfile::tempdir().unwrap();
        let (direct_path, child_path) = (
            dir.path().join("direct.sfst"),
            dir.path().join("child.sfst"),
        );

        let (summary, size) = build(wal_path, &direct_path, &ng_index::Metrics::new()).unwrap();
        let mut answer = Vec::new();
        run(signal, wal_path, &child_path, &mut answer).unwrap();

        assert_eq!(
            IndexedFile::decode(&answer).unwrap(),
            IndexedFile { summary, size }
        );
        assert_eq!(
            std::fs::read(&child_path).unwrap(),
            std::fs::read(&direct_path).unwrap()
        );
    }

    #[test]
    fn a_logs_file_is_indexed_as_by_a_direct_call() {
        let dir = tempfile::tempdir().unwrap();
        let wal_path = write_logs_wal(dir.path(), 7);

        assert_same_as_direct_call(Signal::Logs, &wal_path, ng_index::build_sfst_file);
    }

    #[test]
    fn a_traces_file_is_indexed_as_by_a_direct_call() {
        let dir = tempfile::tempdir().unwrap();
        let wal_path = write_traces_wal(
            dir.path(),
            vec![otlp_req(1, 3, 1_000), otlp_req(2, 2, 9_000)],
        );

        assert_same_as_direct_call(Signal::Traces, &wal_path, ng_index::build_sfst_traces_file);
    }

    #[test]
    fn the_signal_selects_the_builder() {
        let dir = tempfile::tempdir().unwrap();
        let wal_path = write_logs_wal(dir.path(), 1);
        let mut answer = Vec::new();

        let err = run(
            Signal::Traces,
            &wal_path,
            &dir.path().join("out.sfst"),
            &mut answer,
        )
        .unwrap_err();

        assert!(
            matches!(
                err.downcast_ref::<ng_index::Error>(),
                Some(ng_index::Error::PayloadFormat { .. })
            ),
            "{err:#}"
        );
    }

    #[test]
    fn a_failed_build_names_the_file_and_writes_no_answer() {
        let dir = tempfile::tempdir().unwrap();
        let wal_path = dir.path().join("missing.wal");
        let mut answer = Vec::new();

        let err = run(
            Signal::Logs,
            &wal_path,
            &dir.path().join("out.sfst"),
            &mut answer,
        )
        .unwrap_err();

        assert!(format!("{err:#}").contains("missing.wal"), "{err:#}");
        assert!(answer.is_empty());
    }

    #[test]
    fn an_answer_with_trailing_bytes_is_refused() {
        let indexed = IndexedFile {
            summary: sfst::Summary {
                min_timestamp_s: 1,
                max_timestamp_s: 2,
                record_count: 3,
                content_meta: b"meta".to_vec(),
            },
            size: 4,
        };
        let mut answer = indexed.encode().unwrap();
        assert_eq!(IndexedFile::decode(&answer).unwrap(), indexed);

        answer.push(0);

        assert!(IndexedFile::decode(&answer).is_err());
    }
}
