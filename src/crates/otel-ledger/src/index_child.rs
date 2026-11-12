//! Indexing one WAL file in a process of its own.
//!
//! `otel-plugin index` runs [`run`]: it indexes one closed WAL file with the
//! signal's `ng-index` indexing function and writes its answer, an
//! [`IndexedFile`], to stdout. The ledger's
//! [`Indexer`](crate::indexer::Indexer) starts it with [`arguments`] and
//! reads the answer back with [`IndexedFile::decode`]. Both halves of the
//! exchange live here so that they cannot drift apart.
//!
//! The answer is the only thing a successful child writes to stdout. A child
//! that fails writes nothing there: it reports the error on stderr and exits
//! with a non-zero status (`otel-plugin`'s `main`). A child ended by a signal
//! leaves no answer at all; its parent learns that from the wait status.
//!
//! A child that dies midway leaves the WAL file untouched and at most a
//! temporary file beside the index path: the indexing functions write through
//! `file_registry`'s atomic file. The ledger removes that file when the child
//! did not end well, and startup recovery removes it when the ledger was gone
//! too.

use std::ffi::OsString;
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
    /// The most memory the child held at once, in bytes, as the child read
    /// it from its own system when the indexing was done. `None` on a system
    /// where `ng_index::read_rss` has no way to read it.
    pub peak_memory: Option<u64>,
    /// The bytes at the end of the WAL file that were not a whole frame, and
    /// so are not in the index file: a write that never finished
    /// (`wal::Reader::incomplete_tail`). 0 for a file of whole frames.
    pub incomplete_tail: u64,
}

impl IndexedFile {
    /// The bytes an index child writes to stdout.
    pub(crate) fn encode(&self) -> Result<Vec<u8>> {
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

/// The nice level an index child gives itself.
#[cfg(unix)]
const NICE: libc::c_int = 10;

/// Lower the CPU priority of this process to that of background work.
///
/// `otel-plugin index` calls it before anything else, on its first thread, so
/// that an indexing operation gives way to the ledger, the ingestor and
/// everything else on the host when CPUs are short. It costs the operation
/// nothing while CPUs are idle. If the system refuses, the operation runs at
/// the priority it was started with.
///
/// One implementation per system. On Linux the level belongs to the calling
/// thread and to the threads it starts later, which is why this comes first. A
/// system without an implementation here runs its indexing operations at normal
/// priority.
pub fn lower_priority() {
    #[cfg(unix)]
    // SAFETY: `setpriority` takes integers only and reads or writes no memory
    // of this process.
    unsafe {
        libc::setpriority(libc::PRIO_PROCESS, 0, NICE);
    }
}

/// The arguments that make `otel-plugin` index the WAL file at `wal_path`
/// into `sfst_path`: the `index` subcommand of its `main`. The child also
/// exits when its stdin closes (`--lifeline`).
pub fn arguments(signal: Signal, wal_path: &Path, sfst_path: &Path) -> Vec<OsString> {
    vec![
        "index".into(),
        "--signal".into(),
        signal.segment().into(),
        "--wal".into(),
        wal_path.into(),
        "--sfst".into(),
        sfst_path.into(),
        "--lifeline".into(),
    ]
}

/// Index the WAL file at `wal_path` into the index file at `sfst_path` and
/// write the answer to `answer`.
///
/// The body of `otel-plugin index`. Nothing is written to `answer` when the
/// indexing fails.
pub fn run(
    signal: Signal,
    wal_path: &Path,
    sfst_path: &Path,
    answer: &mut impl Write,
) -> Result<()> {
    let index = match signal {
        Signal::Logs => ng_index::index_logs_file,
        Signal::Traces => ng_index::index_traces_file,
    };

    let metrics = ng_index::Metrics::new();
    let (summary, size) = index(wal_path, sfst_path, &metrics)
        .with_context(|| format!("indexing {}", wal_path.display()))?;

    let indexed = IndexedFile {
        summary,
        size,
        peak_memory: ng_index::read_rss().map(|rss| rss.peak_kb * 1024),
        incomplete_tail: metrics.incomplete_tail(),
    };
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

    /// An `ng-index` indexing function, as `run` calls it.
    type IndexFn =
        fn(&Path, &Path, &ng_index::Metrics) -> Result<(sfst::Summary, u64), ng_index::Error>;

    /// The child's answer and index file are those of a direct call of
    /// `index`.
    fn assert_same_as_direct_call(signal: Signal, wal_path: &Path, index: IndexFn) {
        let dir = tempfile::tempdir().unwrap();
        let (direct_path, child_path) = (
            dir.path().join("direct.sfst"),
            dir.path().join("child.sfst"),
        );

        let (summary, size) = index(wal_path, &direct_path, &ng_index::Metrics::new()).unwrap();
        let mut answer = Vec::new();
        run(signal, wal_path, &child_path, &mut answer).unwrap();

        let indexed = IndexedFile::decode(&answer).unwrap();
        assert_eq!((indexed.summary, indexed.size), (summary, size));
        // The peak of this test process stands in for the child's: it is
        // what `run` reads, where the system has it.
        assert_eq!(
            indexed.peak_memory.is_some(),
            ng_index::read_rss().is_some()
        );
        assert!(indexed.peak_memory.is_none_or(|peak| peak > 1 << 20));
        assert_eq!(
            std::fs::read(&child_path).unwrap(),
            std::fs::read(&direct_path).unwrap()
        );
    }

    #[cfg(unix)]
    #[test]
    fn lower_priority_brings_the_calling_thread_to_the_nice_level() {
        // On a thread of its own: on Linux the level is the thread's, so the
        // other tests keep theirs.
        let (before, after) = std::thread::spawn(|| {
            // SAFETY: `getpriority` takes integers only.
            let nice = || unsafe { libc::getpriority(libc::PRIO_PROCESS, 0) };
            let before = nice();
            lower_priority();
            (before, nice())
        })
        .join()
        .unwrap();

        // A process cannot raise its priority: one that starts below the
        // level stays where it is.
        assert_eq!(after, before.max(NICE));
    }

    /// A crash during a write leaves a file that ends inside a frame. The
    /// frame was never acknowledged; the frames before it were.
    #[test]
    fn a_file_that_ends_inside_a_frame_is_indexed_to_its_last_whole_frame() {
        let dir = tempfile::tempdir().unwrap();
        let whole_path = write_logs_wal(dir.path(), 7);
        let whole = std::fs::read(&whole_path).unwrap();
        // After the whole frame: the 24-byte header of a second frame and
        // six bytes of its payload.
        let second_frame = &whole[wal::HEADER_SIZE..wal::HEADER_SIZE + 30];
        let mut cut_short = whole.clone();
        cut_short.extend_from_slice(second_frame);
        let cut_short_path = dir.path().join("cut-short.wal");
        std::fs::write(&cut_short_path, cut_short).unwrap();

        let (whole_index, cut_short_index) = (
            dir.path().join("whole.sfst"),
            dir.path().join("cut-short.sfst"),
        );
        let (mut whole_answer, mut cut_short_answer) = (Vec::new(), Vec::new());
        run(Signal::Logs, &whole_path, &whole_index, &mut whole_answer).unwrap();
        run(
            Signal::Logs,
            &cut_short_path,
            &cut_short_index,
            &mut cut_short_answer,
        )
        .unwrap();

        let (whole, cut_short) = (
            IndexedFile::decode(&whole_answer).unwrap(),
            IndexedFile::decode(&cut_short_answer).unwrap(),
        );
        assert_eq!(cut_short.summary, whole.summary);
        assert_eq!((whole.incomplete_tail, cut_short.incomplete_tail), (0, 30));
        assert_eq!(
            std::fs::read(&cut_short_index).unwrap(),
            std::fs::read(&whole_index).unwrap()
        );
    }

    #[test]
    fn a_logs_file_is_indexed_as_by_a_direct_call() {
        let dir = tempfile::tempdir().unwrap();
        let wal_path = write_logs_wal(dir.path(), 7);

        assert_same_as_direct_call(Signal::Logs, &wal_path, ng_index::index_logs_file);
    }

    #[test]
    fn a_traces_file_is_indexed_as_by_a_direct_call() {
        let dir = tempfile::tempdir().unwrap();
        let wal_path = write_traces_wal(
            dir.path(),
            vec![otlp_req(1, 3, 1_000), otlp_req(2, 2, 9_000)],
        );

        assert_same_as_direct_call(Signal::Traces, &wal_path, ng_index::index_traces_file);
    }

    #[test]
    fn the_signal_selects_the_index_function() {
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
    fn failed_indexing_names_the_file_and_writes_no_answer() {
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
            peak_memory: Some(5),
            incomplete_tail: 0,
        };
        let mut answer = indexed.encode().unwrap();
        assert_eq!(IndexedFile::decode(&answer).unwrap(), indexed);

        answer.push(0);

        assert!(IndexedFile::decode(&answer).is_err());
    }
}
