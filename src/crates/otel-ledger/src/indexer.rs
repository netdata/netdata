//! The indexer component: turns closed WAL files into SFST index files.
//!
//! One component type serves both signals — each pipeline spawns its own
//! [`Indexer`] for its signal (`ledger/pipeline.rs`,
//! `ledger/traces_pipeline.rs`). Every index build runs in a child process:
//! this executable as `otel-plugin index` ([`crate::index_child`]), which
//! picks the signal's `ng-index` builder. So the memory a build needs goes
//! back to the system when the build ends, and a build that the kernel kills
//! takes only itself down.
//!
//! The indexers of both signals start their builds through one
//! [`IndexBuilds`], whose scheduler decides how many run at once across the
//! process: at most all CPUs but one ([`max_index_builds`]), and only while
//! the memory the running builds are expected to need stays within three
//! quarters of what is free ([`MEMORY_PERCENT`]). The builds beyond that wait
//! in its queue, in the order they were requested. A host that ingests
//! enough to queue index builds is taken to be there for that work, so the
//! builds may use most of it; what is left is the room for queries and for
//! everything else, which this side does not look at.
//!
//! What a build is expected to need is its WAL file's decoded size times a
//! ratio that each indexer learns from the peaks its children report
//! ([`ExpectedMemory`]).
//!
//! Requests arrive from the ingestor on a WAL `FileEvent::Closed`
//! (`ledger/ingestor.rs`) and from startup recovery of unindexed WALs
//! (`file_lifecycle::recovery::recover_unindexed`); responses go back to
//! the ledger's `handle_indexer_resp` (`ledger/indexer.rs` — a different
//! indexer.rs: the response handler, not this component) or to the recovery
//! drains, in the order the builds finish. Live-tail chunk range builds
//! (`ng_index::build_sfst_range`) bypass this component.
//!
//! Cancelling or closing the request channel exits the loop and kills the
//! builds that still run. That loses nothing: a build writes its index
//! through a temporary file and leaves its WAL file alone, and the next start
//! indexes the file again.

use std::collections::VecDeque;
use std::path::{Path, PathBuf};
use std::process::Command;

use anyhow::Context;
use otel_bridge::signals::Signal;
use otel_job_scheduler::{Job, Limits, Scheduler, Usage};
use tokio::sync::mpsc;
use tokio::task::JoinSet;
use tokio_util::sync::CancellationToken;

use file_lifecycle::component::Component;
use file_lifecycle::ipc::{IndexerRequest, IndexerResponse};

use crate::index_child::{self, IndexedFile};

/// The share of the free memory, in percent, that the index builds of both
/// signals may be expected to need together.
pub const MEMORY_PERCENT: u8 = 75;

/// How many index builds may run at once, over both signals: all the CPUs
/// this process may use but one, and at least one. A build keeps one CPU
/// busy.
pub fn max_index_builds() -> usize {
    let cpus = std::thread::available_parallelism().map_or(1, |cpus| cpus.get());

    cpus.saturating_sub(1).max(1)
}

/// The memory an index build is expected to need, learned from the builds of
/// one signal.
///
/// A build needs memory in proportion to what its WAL file decodes to, by a
/// ratio that depends on the data: about 4 for the data sets measured so
/// far. The expectation uses the highest ratio among the last
/// [`RECENT_BUILDS`](Self::RECENT_BUILDS) builds that reported their peak,
/// so it follows the data and leans to the high side, and adds a margin.
/// Until a build has reported, the ratio is
/// [`FIRST_RATIO`](Self::FIRST_RATIO).
#[derive(Debug, Default)]
struct ExpectedMemory {
    /// Peak memory over decoded size, of the most recent builds that taught
    /// it, oldest first.
    recent_ratios: VecDeque<f64>,
}

impl ExpectedMemory {
    /// The ratio before any build has reported: above what was measured.
    const FIRST_RATIO: f64 = 5.0;
    /// How many builds the ratio looks back on.
    const RECENT_BUILDS: usize = 32;
    /// What the expectation adds on top of the highest recent ratio.
    const MARGIN: f64 = 1.15;
    /// Only a build of a file that decodes to at least this much teaches the
    /// ratio. A child holds about 15 MiB whatever it indexes; for a small
    /// file that is most of its peak and would make the ratio many times too
    /// high.
    const SMALLEST_TEACHING_SIZE: u64 = 16 << 20;

    /// What a build of a file that decodes to `decoded_size` bytes is
    /// expected to need at its peak, in bytes.
    fn of(&self, decoded_size: u64) -> u64 {
        let ratio = self
            .recent_ratios
            .iter()
            .copied()
            .reduce(f64::max)
            .unwrap_or(Self::FIRST_RATIO);

        (decoded_size as f64 * ratio * Self::MARGIN) as u64
    }

    /// Take in what a build reported: the peak memory of the build of a file
    /// that decodes to `decoded_size` bytes.
    fn learn(&mut self, decoded_size: u64, peak_memory: u64) {
        if decoded_size < Self::SMALLEST_TEACHING_SIZE {
            return;
        }

        if self.recent_ratios.len() == Self::RECENT_BUILDS {
            self.recent_ratios.pop_front();
        }
        self.recent_ratios
            .push_back(peak_memory as f64 / decoded_size as f64);
    }
}

/// What the build of one file needed, for [`ExpectedMemory::learn`].
struct Measured {
    decoded_size: u64,
    peak_memory: u64,
}

/// Starts index builds as child processes. One for the whole ledger, shared
/// by the indexers of both signals, so that one limit covers them all.
#[derive(Clone)]
pub struct IndexBuilds {
    scheduler: Scheduler,
    /// The executable to run as `otel-plugin index`.
    executable: PathBuf,
}

impl IndexBuilds {
    /// Index builds that run this process's own executable.
    ///
    /// Must be called within a tokio runtime (see [`Scheduler::new`]).
    pub fn new() -> anyhow::Result<Self> {
        let executable =
            std::env::current_exe().context("failed to resolve the current executable")?;

        Ok(Self::with_executable(executable))
    }

    fn with_executable(executable: PathBuf) -> Self {
        Self {
            scheduler: Scheduler::new(Limits {
                jobs: max_index_builds(),
                memory_percent: MEMORY_PERCENT,
            }),
            executable,
        }
    }

    /// Build the index file of the WAL file at `wal_path` at `sfst_path` and
    /// wait for the result. `decoded_size` is what the WAL file decodes to,
    /// when that could be read, and `expected_memory` what the build is
    /// expected to need.
    ///
    /// Returns the response for the ledger and, when the build reported its
    /// peak, what it needed.
    async fn build(
        &self,
        signal: Signal,
        wal_path: PathBuf,
        sfst_path: PathBuf,
        decoded_size: Option<u64>,
        expected_memory: u64,
    ) -> (IndexerResponse, Option<Measured>) {
        // Parsed from the WAL file name; 0 when it doesn't parse. The ledger
        // routes the response to a tenant by it.
        let seq = file_registry::FileId::parse(&wal_path)
            .map(|id| id.seq)
            .unwrap_or(0);

        let mut command = Command::new(&self.executable);
        command
            .args(index_child::arguments(signal, &wal_path, &sfst_path))
            // The child has one thread. glibc reads the limit from the
            // environment when the process starts.
            .env("MALLOC_ARENA_MAX", "1");

        tracing::info!(
            "indexing queued wal={} index={} decoded_kb={} expected_rss_kb={}",
            wal_path.display(),
            sfst_path.display(),
            decoded_size.unwrap_or(0) / 1024,
            expected_memory / 1024,
        );

        let job = Job {
            command,
            expected_memory,
        };
        let finished = match self.scheduler.run(job).await {
            Ok(finished) => finished,
            Err(e) => {
                let usage = e.usage().map(usage_text).unwrap_or_default();
                tracing::error!("indexing failed wal={}{usage}: {e}", wal_path.display());
                let failed = IndexerResponse::IndexFailed {
                    path: wal_path,
                    error: e.to_string(),
                };
                return (failed, None);
            }
        };

        let usage = usage_text(finished.usage);
        match IndexedFile::decode(&finished.stdout) {
            Ok(IndexedFile {
                summary,
                size,
                peak_memory,
            }) => {
                let peak = peak_memory
                    .map(|bytes| format!(" peak_rss_kb={}", bytes / 1024))
                    .unwrap_or_default();
                tracing::info!("indexing done seq={seq}{usage}{peak}");
                let indexed = IndexerResponse::Indexed {
                    seq,
                    path: sfst_path,
                    summary,
                    size: file_registry::ByteSize(size),
                };
                let measured = decoded_size
                    .zip(peak_memory)
                    .map(|(decoded_size, peak_memory)| Measured {
                        decoded_size,
                        peak_memory,
                    });
                (indexed, measured)
            }
            Err(e) => {
                tracing::error!("indexing failed wal={}{usage}: {e:#}", wal_path.display());
                let failed = IndexerResponse::IndexFailed {
                    path: wal_path,
                    error: format!("{e:#}"),
                };
                (failed, None)
            }
        }
    }
}

/// What the WAL file at `path` decodes to, in bytes, from its frame headers.
/// `None` when the file cannot be read: its build then expects no memory,
/// and reports what is wrong with the file.
async fn decoded_size(path: &Path) -> Option<u64> {
    let wal_path = path.to_owned();
    let size = tokio::task::spawn_blocking(move || wal::decoded_size(&wal_path)).await;

    match size {
        Ok(Ok(size)) => Some(size),
        Ok(Err(e)) => {
            tracing::warn!("cannot size wal={}: {e}", path.display());
            None
        }
        Err(_) => None,
    }
}

/// What a build cost in time, as the fields that follow the subject of its
/// log line: the time it waited for a slot and the time its child ran.
fn usage_text(usage: Usage) -> String {
    format!(
        " waited_ms={} ran_ms={}",
        usage.waited.as_millis(),
        usage.ran.as_millis(),
    )
}

/// The indexer component: a unit struct — all state lives in
/// `Component::run`'s locals; one instance per signal pipeline.
pub struct Indexer;

impl Component for Indexer {
    type Request = IndexerRequest;
    type Response = IndexerResponse;
    type Args = (Signal, IndexBuilds);

    async fn run(
        (signal, index_builds): Self::Args,
        mut rx: mpsc::UnboundedReceiver<IndexerRequest>,
        tx: mpsc::UnboundedSender<IndexerResponse>,
        cancel: CancellationToken,
    ) {
        let mut expected_memory = ExpectedMemory::default();
        // Dropped when the loop exits, which abandons the builds still in it.
        let mut builds = JoinSet::new();

        loop {
            tokio::select! {
                _ = cancel.cancelled() => break,
                request = rx.recv() => match request {
                    Some(IndexerRequest::Index { wal_path, sfst_path }) => {
                        // Sized here, not in the build's task: the builds
                        // must reach the scheduler in the order of their
                        // requests.
                        let decoded_size = decoded_size(&wal_path).await;
                        let expected = decoded_size.map_or(0, |size| expected_memory.of(size));

                        let index_builds = index_builds.clone();
                        builds.spawn(async move {
                            index_builds
                                .build(signal, wal_path, sfst_path, decoded_size, expected)
                                .await
                        });
                    }
                    None => break,
                },
                Some(built) = builds.join_next() => {
                    // A build task ends without a response only if it
                    // panicked: there is nothing to forward then.
                    if let Ok((response, measured)) = built {
                        if let Some(measured) = measured {
                            expected_memory.learn(measured.decoded_size, measured.peak_memory);
                        }
                        let _ = tx.send(response);
                    }
                }
            }
        }
    }
}

#[cfg(test)]
mod tests {
    use file_lifecycle::component::ComponentHandle;

    use super::*;

    /// A WAL file name that parses to `seq`, under `dir`. The file need not
    /// exist: the fake executables never read it.
    fn wal_path(dir: &Path, seq: u64) -> PathBuf {
        dir.join(crate::ledger::traces_fixtures::test_file_id(seq).to_filename("wal"))
    }

    /// An executable shell script under `dir` to stand in for `otel-plugin`.
    ///
    /// A child process writes the file, not this one: a file that this
    /// process holds open for writing can be inherited by the fork of another
    /// test's spawn, and running it then fails with "Text file busy".
    fn fake_executable(dir: &Path, script: &str) -> PathBuf {
        let path = dir.join("fake-otel-plugin");
        let written = Command::new("/bin/sh")
            .arg("-c")
            .arg(r#"printf '#!/bin/sh\n%s\n' "$1" > "$0" && chmod +x "$0""#)
            .arg(&path)
            .arg(script)
            .status()
            .unwrap();
        assert!(written.success());
        path
    }

    /// An indexer for logs whose builds run `executable`.
    fn spawn_indexer(
        executable: PathBuf,
        cancel: &CancellationToken,
    ) -> ComponentHandle<IndexerRequest, IndexerResponse> {
        let index_builds = IndexBuilds::with_executable(executable);
        ComponentHandle::spawn::<Indexer>((Signal::Logs, index_builds), cancel.child_token())
    }

    fn summary() -> sfst::Summary {
        sfst::Summary {
            min_timestamp_s: 10,
            max_timestamp_s: 20,
            record_count: 3,
            content_meta: b"meta".to_vec(),
        }
    }

    const MIB: u64 = 1 << 20;

    /// What `ExpectedMemory` makes of `decoded_size` bytes at `ratio`.
    fn at_ratio(decoded_size: u64, ratio: f64) -> u64 {
        (decoded_size as f64 * ratio * ExpectedMemory::MARGIN) as u64
    }

    #[test]
    fn expected_memory_starts_from_the_first_ratio() {
        let expected_memory = ExpectedMemory::default();

        assert_eq!(
            expected_memory.of(100 * MIB),
            at_ratio(100 * MIB, ExpectedMemory::FIRST_RATIO)
        );
        // 100 MiB at 5 with 15% on top.
        assert_eq!(expected_memory.of(100 * MIB), 575 * MIB);
    }

    #[test]
    fn expected_memory_follows_the_highest_recent_ratio() {
        let mut expected_memory = ExpectedMemory::default();

        expected_memory.learn(100 * MIB, 300 * MIB);
        expected_memory.learn(100 * MIB, 400 * MIB);
        expected_memory.learn(100 * MIB, 350 * MIB);

        assert_eq!(expected_memory.of(10 * MIB), at_ratio(10 * MIB, 4.0));
    }

    #[test]
    fn expected_memory_forgets_the_builds_before_the_recent_ones() {
        let mut expected_memory = ExpectedMemory::default();

        expected_memory.learn(100 * MIB, 900 * MIB);
        for _ in 0..ExpectedMemory::RECENT_BUILDS - 1 {
            expected_memory.learn(100 * MIB, 300 * MIB);
        }
        assert_eq!(expected_memory.of(10 * MIB), at_ratio(10 * MIB, 9.0));

        expected_memory.learn(100 * MIB, 300 * MIB);
        assert_eq!(expected_memory.of(10 * MIB), at_ratio(10 * MIB, 3.0));
    }

    #[test]
    fn a_build_of_a_small_file_does_not_teach_the_ratio() {
        let mut expected_memory = ExpectedMemory::default();

        // A child's own 15 MiB over a 1 MiB file: a ratio of 15.
        expected_memory.learn(MIB, 15 * MIB);
        expected_memory.learn(ExpectedMemory::SMALLEST_TEACHING_SIZE - 1, 900 * MIB);
        assert_eq!(
            expected_memory.of(100 * MIB),
            at_ratio(100 * MIB, ExpectedMemory::FIRST_RATIO)
        );

        expected_memory.learn(ExpectedMemory::SMALLEST_TEACHING_SIZE, 64 * MIB);
        assert_eq!(expected_memory.of(100 * MIB), at_ratio(100 * MIB, 4.0));
    }

    #[test]
    fn index_builds_leave_one_cpu_and_never_drop_to_none() {
        let cpus = std::thread::available_parallelism().unwrap().get();

        assert_eq!(max_index_builds(), if cpus > 1 { cpus - 1 } else { 1 });
    }

    /// A WAL file under `dir` whose frames decode to `frames` times 1 KiB.
    /// The bytes are not log records: only its headers are ever read.
    fn write_wal(dir: &Path, frames: usize) -> PathBuf {
        let mut writer = wal::Writer::new(
            dir,
            wal::Config::default(),
            std::sync::Arc::new(wal::SeqAllocator::ephemeral(0)),
            wal::FileStamp {
                pipeline_id: Signal::Logs.pipeline_id(),
                payload_format: ng_flatten::LOG_FRAME_PAYLOAD_FORMAT,
            },
            wal::test_identity(),
        )
        .unwrap();
        for frame in 0..frames {
            writer
                .write_frame(
                    0,
                    &[],
                    &[0u8; 1024],
                    wal::FrameMeta {
                        entry_count: 1,
                        ingestion_ns: file_registry::TimestampNs(frame as u64 + 1),
                        log_ts_range: None,
                    },
                )
                .unwrap();
        }
        writer.shutdown_all().unwrap();

        std::fs::read_dir(dir)
            .unwrap()
            .map(|entry| entry.unwrap().path())
            .find(|path| path.extension().is_some_and(|ext| ext == "wal"))
            .expect("a wal file was written")
    }

    #[tokio::test]
    async fn a_build_reports_the_decoded_size_and_the_peak_its_child_answered() {
        let dir = tempfile::tempdir().unwrap();
        let wal = write_wal(dir.path(), 3);
        let answer = dir.path().join("answer");
        let indexed = IndexedFile {
            summary: summary(),
            size: 4096,
            peak_memory: Some(77 * MIB),
        };
        std::fs::write(&answer, indexed.encode().unwrap()).unwrap();
        let executable = fake_executable(dir.path(), &format!("cat {}", answer.display()));
        let index_builds = IndexBuilds::with_executable(executable);

        let size = decoded_size(&wal).await;
        let (response, measured) = index_builds
            .build(Signal::Logs, wal, dir.path().join("out.sfst"), size, 0)
            .await;

        assert_eq!(size, Some(3 * 1024));
        assert!(
            matches!(response, IndexerResponse::Indexed { .. }),
            "{response:?}"
        );
        let measured = measured.expect("the build reported what it needed");
        assert_eq!(
            (measured.decoded_size, measured.peak_memory),
            (3 * 1024, 77 * MIB)
        );
    }

    #[tokio::test]
    async fn a_file_that_cannot_be_sized_is_built_without_an_expectation() {
        let dir = tempfile::tempdir().unwrap();

        assert_eq!(decoded_size(&dir.path().join("missing.wal")).await, None);
    }

    #[tokio::test]
    async fn a_build_runs_the_index_command_and_answers_with_the_childs_answer() {
        let dir = tempfile::tempdir().unwrap();
        let (wal, sfst) = (wal_path(dir.path(), 7), dir.path().join("7.sfst"));
        let (answer, seen) = (dir.path().join("answer"), dir.path().join("seen"));
        let indexed = IndexedFile {
            summary: summary(),
            size: 4096,
            peak_memory: Some(1 << 30),
        };
        std::fs::write(&answer, indexed.encode().unwrap()).unwrap();
        // The fake records its arguments and its arena limit, one per line,
        // then answers.
        let executable = fake_executable(
            dir.path(),
            &format!(
                "printf '%s\\n' \"$@\" \"$MALLOC_ARENA_MAX\" > {seen}; cat {answer}",
                seen = seen.display(),
                answer = answer.display(),
            ),
        );
        let mut indexer = spawn_indexer(executable, &CancellationToken::new());

        indexer
            .send(IndexerRequest::Index {
                wal_path: wal.clone(),
                sfst_path: sfst.clone(),
            })
            .unwrap();
        let response = indexer.recv().await.unwrap();

        let IndexerResponse::Indexed {
            seq,
            path,
            summary: got_summary,
            size,
        } = response
        else {
            panic!("expected Indexed, got {response:?}");
        };
        assert_eq!((seq, path, size.0), (7, sfst.clone(), 4096));
        assert_eq!(got_summary, summary());
        assert_eq!(
            std::fs::read_to_string(&seen).unwrap(),
            format!(
                "index\n--signal\nlogs\n--wal\n{}\n--sfst\n{}\n--lifeline\n1\n",
                wal.display(),
                sfst.display(),
            )
        );
    }

    /// The `IndexFailed` error an indexer answers with when its builds run
    /// `script`.
    async fn failure_of(script: &str) -> (PathBuf, PathBuf, String) {
        let dir = tempfile::tempdir().unwrap();
        let wal = wal_path(dir.path(), 9);
        let executable = fake_executable(dir.path(), script);
        let mut indexer = spawn_indexer(executable, &CancellationToken::new());

        indexer
            .send(IndexerRequest::Index {
                wal_path: wal.clone(),
                sfst_path: dir.path().join("9.sfst"),
            })
            .unwrap();
        let response = indexer.recv().await.unwrap();

        let IndexerResponse::IndexFailed { path, error } = response else {
            panic!("expected IndexFailed, got {response:?}");
        };
        (wal, path, error)
    }

    #[tokio::test]
    async fn a_failed_child_answers_index_failed_with_its_error() {
        let (wal, path, error) = failure_of("echo 'error: no such file' >&2; exit 1").await;

        assert_eq!(path, wal);
        assert_eq!(error, "the job exited with status 1: error: no such file");
    }

    #[tokio::test]
    async fn a_killed_child_answers_index_failed_with_the_signal() {
        let (wal, path, error) = failure_of("kill -KILL $$").await;

        assert_eq!(path, wal);
        assert_eq!(error, "the job was ended by signal 9");
    }

    #[tokio::test]
    async fn an_answer_that_does_not_decode_answers_index_failed() {
        let (_, _, error) = failure_of("printf garbage").await;

        assert!(
            error.starts_with("decoding the index child's answer"),
            "{error}"
        );
    }

    #[tokio::test]
    async fn responses_arrive_as_builds_finish() {
        let dir = tempfile::tempdir().unwrap();
        // The build of the file with seq 1 takes longer than that of seq 2.
        let executable = fake_executable(
            dir.path(),
            "case \"$5\" in *0000000001-*) sleep 0.3;; esac; echo failed >&2; exit 1",
        );
        let mut indexer = spawn_indexer(executable, &CancellationToken::new());

        for seq in [1, 2] {
            indexer
                .send(IndexerRequest::Index {
                    wal_path: wal_path(dir.path(), seq),
                    sfst_path: dir.path().join(format!("{seq}.sfst")),
                })
                .unwrap();
        }
        let mut finished = Vec::new();
        for _ in 0..2 {
            let IndexerResponse::IndexFailed { path, .. } = indexer.recv().await.unwrap() else {
                panic!("expected IndexFailed");
            };
            finished.push(path);
        }

        assert_eq!(finished, [wal_path(dir.path(), 2), wal_path(dir.path(), 1)]);
    }

    #[tokio::test]
    async fn cancelling_the_indexer_kills_its_running_builds() {
        let dir = tempfile::tempdir().unwrap();
        let pid_file = dir.path().join("pid");
        let executable = fake_executable(
            dir.path(),
            &format!("echo $$ > {}; exec sleep 60", pid_file.display()),
        );
        let cancel = CancellationToken::new();
        let mut indexer = spawn_indexer(executable, &cancel);

        indexer
            .send(IndexerRequest::Index {
                wal_path: wal_path(dir.path(), 3),
                sfst_path: dir.path().join("3.sfst"),
            })
            .unwrap();
        let written = async {
            loop {
                match std::fs::read_to_string(&pid_file) {
                    Ok(text) if text.ends_with('\n') => break text.trim().to_owned(),
                    _ => tokio::time::sleep(std::time::Duration::from_millis(5)).await,
                }
            }
        };
        let pid = tokio::time::timeout(std::time::Duration::from_secs(5), written)
            .await
            .expect("the build never started");

        cancel.cancel();

        // The component ends without a response for the build, and the child
        // is gone: its /proc entry disappears or shows a zombie.
        let ended = tokio::time::timeout(std::time::Duration::from_secs(5), indexer.recv());
        assert!(ended.await.expect("the indexer still runs").is_none());
        let gone = async {
            loop {
                match std::fs::read_to_string(format!("/proc/{pid}/stat")) {
                    Ok(stat) if !stat.contains(") Z ") => {
                        tokio::time::sleep(std::time::Duration::from_millis(5)).await
                    }
                    _ => break,
                }
            }
        };
        tokio::time::timeout(std::time::Duration::from_secs(5), gone)
            .await
            .expect("the child still runs");
    }
}
