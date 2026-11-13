//! The indexer component: turns closed WAL files into SFST index files.
//!
//! One component type serves both signals — each pipeline spawns its own
//! [`Indexer`] for its signal (`ledger/pipeline.rs`,
//! `ledger/traces_pipeline.rs`). Every indexing operation runs in a child
//! process: this executable as `otel-plugin index` ([`crate::index_child`]),
//! which picks the signal's `ng-index` indexing function. So the memory an
//! operation needs goes back to the system when the operation ends, and an
//! operation that the kernel kills takes only itself down.
//!
//! The indexers of both signals start their operations through one
//! [`IndexingOperations`], whose scheduler decides how many run at once across
//! the process: at most all CPUs but one ([`max_indexing_operations`]), and
//! only while the memory the running operations are expected to need stays
//! within three quarters of what is free ([`MEMORY_PERCENT`]). The operations
//! beyond that wait in its queue, in the order they were requested. A host that
//! ingests enough to queue indexing operations is taken to be there for that
//! work, so the operations may use most of it; what is left is the room for
//! queries and for everything else, which this side does not look at.
//!
//! Neither limit is a setting. For support, an environment variable replaces
//! each of them when the ledger starts: [`MAX_OPERATIONS_VARIABLE`] and
//! [`MEMORY_PERCENT_VARIABLE`]. They are not documented for users.
//!
//! What an operation is expected to need is its WAL file's decoded size times a
//! ratio that each indexer learns from the peaks its children report
//! (`ExpectedMemory`).
//!
//! Requests arrive from the ingestor on a WAL `FileEvent::Closed`
//! (`ledger/ingestor.rs`) and from startup recovery of unindexed WALs
//! (`file_lifecycle::recovery::recover_unindexed`); responses go back to
//! the ledger's `handle_indexer_resp` (`ledger/indexer.rs` — a different
//! indexer.rs: the response handler, not this component) or to the recovery
//! drains, in the order the operations finish. Chunk indexing for live
//! queries (`ng_index::index_logs_range`) bypasses this component.
//!
//! Cancelling or closing the request channel exits the loop and kills the
//! operations that still run. That loses nothing: an operation writes its index
//! through a temporary file and leaves its WAL file alone, and the next start
//! indexes the file again.

use std::collections::{HashMap, VecDeque};
use std::ffi::OsStr;
use std::path::{Path, PathBuf};
use std::process::Command;
use std::sync::Arc;
use std::sync::atomic::{AtomicU64, Ordering};

use anyhow::Context;
use otel_bridge::signals::Signal;
use otel_job_scheduler::{Job, Limits, Queue, Scheduler, Usage};
use tokio::sync::mpsc;
use tokio::task::JoinSet;
use tokio_util::sync::CancellationToken;

use file_lifecycle::component::Component;
use file_lifecycle::ipc::{IndexerRequest, IndexerResponse};
use file_registry::FileId;

use crate::index_child::{self, IndexedFile};

/// The share of the free memory, in percent, that the indexing operations of
/// both signals may be expected to need together.
pub const MEMORY_PERCENT: u8 = 75;

/// How many indexing operations may run at once, over both signals: all the
/// CPUs this process may use but one, and at least one. An operation keeps one
/// CPU busy.
pub fn max_indexing_operations() -> usize {
    let cpus = std::thread::available_parallelism().map_or(1, |cpus| cpus.get());

    cpus.saturating_sub(1).max(1)
}

/// The environment variable that replaces [`max_indexing_operations`]: a whole
/// number of at least 1. With 1, the operations run one at a time.
pub const MAX_OPERATIONS_VARIABLE: &str = "NETDATA_OTEL_INDEXING_MAX_OPERATIONS";

/// The environment variable that replaces [`MEMORY_PERCENT`]: a whole number
/// from 1 to 100.
pub const MEMORY_PERCENT_VARIABLE: &str = "NETDATA_OTEL_INDEXING_MEMORY_PERCENT";

/// The limits of the indexing operations: [`max_indexing_operations`] and
/// [`MEMORY_PERCENT`], each replaced by the value of its environment variable
/// when that is set.
///
/// A value that is not valid is an error, so that a mistyped value cannot
/// leave a limit at its usual number unnoticed.
fn limits_from(
    max_operations: Option<&OsStr>,
    memory_percent: Option<&OsStr>,
) -> anyhow::Result<Limits> {
    let jobs = match max_operations {
        Some(value) => {
            let jobs: usize = whole_number(MAX_OPERATIONS_VARIABLE, value)?;
            anyhow::ensure!(
                jobs >= 1,
                "invalid value for {MAX_OPERATIONS_VARIABLE}: '{jobs}': expected at least 1"
            );
            tracing::info!(
                "{MAX_OPERATIONS_VARIABLE} is set: at most {jobs} indexing operations run at once, not {}",
                max_indexing_operations()
            );
            jobs
        }
        None => max_indexing_operations(),
    };

    let memory_percent = match memory_percent {
        Some(value) => {
            let percent: u8 = whole_number(MEMORY_PERCENT_VARIABLE, value)?;
            anyhow::ensure!(
                (1..=100).contains(&percent),
                "invalid value for {MEMORY_PERCENT_VARIABLE}: '{percent}': expected 1 to 100"
            );
            tracing::info!(
                "{MEMORY_PERCENT_VARIABLE} is set: indexing operations may use {percent}% of the free \
                 memory, not {MEMORY_PERCENT}%"
            );
            percent
        }
        None => MEMORY_PERCENT,
    };

    Ok(Limits {
        jobs,
        memory_percent,
    })
}

/// The whole number that the environment variable `name` holds as `value`.
fn whole_number<T>(name: &str, value: &OsStr) -> anyhow::Result<T>
where
    T: std::str::FromStr,
    T::Err: std::fmt::Display,
{
    let text = value
        .to_str()
        .with_context(|| format!("{name} contains invalid UTF-8"))?;

    text.parse()
        .map_err(|e| anyhow::anyhow!("invalid value for {name}: '{text}': {e}"))
}

/// The memory an indexing operation is expected to need, learned from the
/// operations of one signal.
///
/// An operation needs memory in proportion to what its WAL file decodes to, by
/// a ratio that depends on the data: about 4 for the data sets measured so far.
/// The expectation uses the highest ratio among the last
/// [`RECENT_OPERATIONS`](Self::RECENT_OPERATIONS) operations that reported
/// their peak, so it follows the data and leans to the high side, and adds a
/// margin. Until an operation has reported, the ratio is
/// [`FIRST_RATIO`](Self::FIRST_RATIO).
#[derive(Debug, Default)]
struct ExpectedMemory {
    /// Peak memory over decoded size, of the most recent operations that taught
    /// it, oldest first.
    recent_ratios: VecDeque<f64>,
}

impl ExpectedMemory {
    /// The ratio before any operation has reported: above what was measured.
    const FIRST_RATIO: f64 = 5.0;
    /// How many operations the ratio looks back on.
    const RECENT_OPERATIONS: usize = 32;
    /// What the expectation adds on top of the highest recent ratio.
    const MARGIN: f64 = 1.15;
    /// Only an operation of a file that decodes to at least this much teaches
    /// the ratio. A child holds about 15 MiB whatever it indexes; for a small
    /// file that is most of its peak and would make the ratio many times too
    /// high.
    const SMALLEST_TEACHING_SIZE: u64 = 16 << 20;

    /// What an operation of a file that decodes to `decoded_size` bytes is
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

    /// Take in what an operation reported: the peak memory of the operation of
    /// a file that decodes to `decoded_size` bytes.
    fn learn(&mut self, decoded_size: u64, peak_memory: u64) {
        if decoded_size < Self::SMALLEST_TEACHING_SIZE {
            return;
        }

        if self.recent_ratios.len() == Self::RECENT_OPERATIONS {
            self.recent_ratios.pop_front();
        }
        self.recent_ratios
            .push_back(peak_memory as f64 / decoded_size as f64);
    }
}

/// What the operation of one file needed, for [`ExpectedMemory::learn`].
struct Measured {
    decoded_size: u64,
    peak_memory: u64,
}

/// How many indexing operations have ended since the ledger started, by
/// outcome.
#[derive(Debug, Default)]
struct Finished {
    succeeded: AtomicU64,
    failed: AtomicU64,
}

/// The indexing operations of the ledger at one moment, for its charts
/// (`indexing_charts`).
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) struct Statistics {
    /// The operations that run and the operations that wait.
    pub(crate) queue: Queue,
    /// How many operations have ended with an index file since the ledger
    /// started.
    pub(crate) succeeded: u64,
    /// How many operations have ended without one since the ledger started.
    pub(crate) failed: u64,
}

/// Starts indexing operations as child processes. One for the whole ledger,
/// shared by the indexers of both signals, so that one limit covers them all.
#[derive(Clone)]
pub struct IndexingOperations {
    scheduler: Scheduler,
    /// The executable to run as `otel-plugin index`.
    executable: PathBuf,
    finished: Arc<Finished>,
}

impl IndexingOperations {
    /// Indexing operations that run this process's own executable, within the
    /// limits that the environment leaves or sets ([`MAX_OPERATIONS_VARIABLE`],
    /// [`MEMORY_PERCENT_VARIABLE`]).
    ///
    /// Must be called within a tokio runtime (see [`Scheduler::new`]).
    pub fn new() -> anyhow::Result<Self> {
        let executable =
            std::env::current_exe().context("failed to resolve the current executable")?;
        let limits = limits_from(
            std::env::var_os(MAX_OPERATIONS_VARIABLE).as_deref(),
            std::env::var_os(MEMORY_PERCENT_VARIABLE).as_deref(),
        )?;

        Ok(Self::with_executable(executable, limits))
    }

    fn with_executable(executable: PathBuf, limits: Limits) -> Self {
        Self {
            scheduler: Scheduler::new(limits),
            executable,
            finished: Arc::default(),
        }
    }

    /// The operations of both signals as they are now.
    pub(crate) fn statistics(&self) -> Statistics {
        Statistics {
            queue: self.scheduler.queue(),
            succeeded: self.finished.succeeded.load(Ordering::Relaxed),
            failed: self.finished.failed.load(Ordering::Relaxed),
        }
    }

    /// Index the WAL file at `wal_path` into the index file at `sfst_path`.
    /// `decoded_size` is what the WAL file decodes to, when that could be
    /// read, and `expected_memory` what the operation is expected to need.
    ///
    /// The operation takes its place in the queue when this is called, not
    /// when the returned future is first polled, so operations start in the
    /// order of the calls. The future waits for the result: the response for
    /// the ledger and, when the operation reported its peak, what it needed.
    fn run(
        &self,
        signal: Signal,
        file_id: FileId,
        wal_path: PathBuf,
        sfst_path: PathBuf,
        decoded_size: Option<u64>,
        expected_memory: u64,
    ) -> impl Future<Output = (IndexerResponse, Option<Measured>)> + use<> {
        let seq = file_id.seq;

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
        let result = self.scheduler.run(job);
        let counters = Arc::clone(&self.finished);

        async move {
            let finished = match result.await {
                Ok(finished) => finished,
                Err(e) => {
                    counters.failed.fetch_add(1, Ordering::Relaxed);
                    remove_failed_index(&sfst_path);
                    let usage = e.usage().map(usage_text).unwrap_or_default();
                    tracing::error!("indexing failed wal={}{usage}: {e}", wal_path.display());
                    let failed = IndexerResponse::IndexFailed {
                        file_id,
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
                    incomplete_tail,
                }) => {
                    if incomplete_tail > 0 {
                        // Not corruption, but data a sender sent that is
                        // not stored: after a crash, or a failed write.
                        tracing::warn!(
                            "the WAL file seq={seq} ends inside a frame: its last \
                             {incomplete_tail} bytes are a write that never finished and are \
                             not indexed"
                        );
                    }
                    let peak = peak_memory
                        .map(|bytes| format!(" peak_rss_kb={}", bytes / 1024))
                        .unwrap_or_default();
                    counters.succeeded.fetch_add(1, Ordering::Relaxed);
                    tracing::info!("indexing done seq={seq}{usage}{peak}");
                    let indexed = IndexerResponse::Indexed {
                        file_id,
                        summary,
                        size: file_registry::ByteSize(size),
                    };
                    let measured =
                        decoded_size
                            .zip(peak_memory)
                            .map(|(decoded_size, peak_memory)| Measured {
                                decoded_size,
                                peak_memory,
                            });
                    (indexed, measured)
                }
                Err(e) => {
                    counters.failed.fetch_add(1, Ordering::Relaxed);
                    remove_failed_index(&sfst_path);
                    tracing::error!("indexing failed wal={}{usage}: {e:#}", wal_path.display());
                    let failed = IndexerResponse::IndexFailed {
                        file_id,
                        error: format!("{e:#}"),
                    };
                    (failed, None)
                }
            }
        }
    }
}

/// Remove what a failed operation left of the index file at `sfst_path`:
/// the temporary file its child wrote to, and the index file itself.
///
/// A child that fails removes its temporary file, but one that is killed
/// while it writes cannot, and nothing else would remove the file before the
/// next start. A child can also fail after it has put the index file in
/// place: it exits with an error, is killed, or prints something that is not
/// an answer. The ledger tracks an index file by its answer (the summary and
/// the size), so that file would sit untracked next to its WAL file. A failed
/// operation leaves no index file.
fn remove_failed_index(sfst_path: &Path) {
    file_registry::durable::remove_stale_tmp(&file_registry::durable::tmp_path(sfst_path));
    match std::fs::remove_file(sfst_path) {
        Ok(()) => {}
        Err(e) if e.kind() == std::io::ErrorKind::NotFound => {}
        Err(e) => tracing::warn!("cannot remove index={}: {e}", sfst_path.display()),
    }
}

/// What the WAL file at `path` decodes to, in bytes, from its frame headers.
/// `None` when the file cannot be read: its operation then expects no memory,
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

/// What an operation cost in time, as the fields that follow the subject of its
/// log line: the time it waited for a slot and the time its child ran.
fn usage_text(usage: Usage) -> String {
    format!(
        " waited_ms={} ran_ms={}",
        usage.waited.as_millis(),
        usage.ran.as_millis(),
    )
}

/// The indexing operations that run, each in a task of its own, with the
/// file of each. It keeps one rule: every operation that starts gives one
/// response, also when its task ends without one (it panicked, or the
/// runtime cancelled it at a stop). Start-up
/// recovery waits for exactly one response per request, with no time limit
/// (`file_lifecycle::component::batch_recover`).
struct RunningOperations {
    tasks: JoinSet<(IndexerResponse, Option<Measured>)>,
    files: HashMap<tokio::task::Id, FileId>,
    /// The ledger's count of ended operations: an operation counts itself,
    /// and one whose task panicked is counted here.
    finished: Arc<Finished>,
}

impl RunningOperations {
    fn new(finished: Arc<Finished>) -> Self {
        Self {
            tasks: JoinSet::new(),
            files: HashMap::new(),
            finished,
        }
    }

    /// Run `operation`, which indexes the file `file_id`, in a new task.
    fn spawn(
        &mut self,
        file_id: FileId,
        operation: impl Future<Output = (IndexerResponse, Option<Measured>)> + Send + 'static,
    ) {
        let task = self.tasks.spawn(operation);
        self.files.insert(task.id(), file_id);
    }

    /// The response of the next operation that ends, with what it measured.
    /// `None` when no operation runs. Cancel safe.
    async fn next(&mut self) -> Option<(IndexerResponse, Option<Measured>)> {
        loop {
            match self.tasks.join_next_with_id().await? {
                Ok((task, ended)) => {
                    self.files.remove(&task);
                    return Some(ended);
                }
                Err(e) => {
                    // Every task is in `files`: the loop only guards the map.
                    let Some(file_id) = self.files.remove(&e.id()) else {
                        continue;
                    };
                    // Nothing here cancels a task: only the runtime does, when
                    // the ledger stops while it still waits here. That is no
                    // failure of the operation, and the next start indexes
                    // the file. A task that panicked is a defect.
                    if e.is_cancelled() {
                        tracing::info!(
                            file = %file_id.to_filename("wal"),
                            "an indexing operation was cut by the stop",
                        );
                    } else {
                        self.finished.failed.fetch_add(1, Ordering::Relaxed);
                        tracing::error!(
                            file = %file_id.to_filename("wal"),
                            "the task of an indexing operation ended without a response: {e}",
                        );
                    }
                    let response = IndexerResponse::IndexFailed {
                        file_id,
                        error: format!("the task of the indexing operation failed: {e}"),
                    };
                    return Some((response, None));
                }
            }
        }
    }
}

/// The indexer component: a unit struct — all state lives in
/// `Component::run`'s locals; one instance per signal pipeline.
pub struct Indexer;

impl Component for Indexer {
    type Request = IndexerRequest;
    type Response = IndexerResponse;
    type Args = (Signal, IndexingOperations);

    async fn run(
        (signal, indexing_operations): Self::Args,
        mut rx: mpsc::UnboundedReceiver<IndexerRequest>,
        tx: mpsc::UnboundedSender<IndexerResponse>,
        cancel: CancellationToken,
    ) {
        let mut expected_memory = ExpectedMemory::default();
        // Dropped when the loop exits, which abandons the operations still in
        // it.
        let mut operations = RunningOperations::new(Arc::clone(&indexing_operations.finished));

        loop {
            tokio::select! {
                _ = cancel.cancelled() => break,
                request = rx.recv() => match request {
                    Some(IndexerRequest::Index { file_id, wal_path, sfst_path }) => {
                        // Sized and queued here, not in the operation's task:
                        // the operations must reach the scheduler in the order
                        // of their requests.
                        let decoded_size = decoded_size(&wal_path).await;
                        let expected = decoded_size.map_or(0, |size| expected_memory.of(size));

                        let operation = indexing_operations.run(
                            signal,
                            file_id,
                            wal_path,
                            sfst_path,
                            decoded_size,
                            expected,
                        );
                        operations.spawn(file_id, operation);
                    }
                    None => break,
                },
                Some((response, measured)) = operations.next() => {
                    if let Some(measured) = measured {
                        expected_memory.learn(measured.decoded_size, measured.peak_memory);
                    }
                    let _ = tx.send(response);
                }
            }
        }
    }
}

#[cfg(test)]
mod tests {
    use file_lifecycle::component::ComponentHandle;

    use super::*;

    /// The id of the test file with `seq`.
    fn file_id(seq: u64) -> FileId {
        crate::ledger::traces_fixtures::test_file_id(seq)
    }

    #[tokio::test]
    async fn an_operation_whose_task_panics_still_gives_its_one_response() {
        let finished = Arc::<Finished>::default();
        let mut operations = RunningOperations::new(Arc::clone(&finished));
        operations.spawn(file_id(7), async {
            panic!("a defect in an indexing operation")
        });

        let (response, measured) = operations.next().await.unwrap();

        assert!(matches!(
            response,
            IndexerResponse::IndexFailed { file_id: failed, .. } if failed == file_id(7)
        ));
        assert!(measured.is_none());
        assert_eq!(finished.failed.load(Ordering::Relaxed), 1);
        assert!(operations.files.is_empty());
        assert!(operations.next().await.is_none());
    }

    /// At a stop the runtime cancels the tasks that still run. That is the
    /// end of the ledger, not a failed indexing operation: the next start
    /// indexes the file.
    #[tokio::test]
    async fn an_operation_whose_task_is_cancelled_is_not_counted_as_failed() {
        let finished = Arc::<Finished>::default();
        let mut operations = RunningOperations::new(Arc::clone(&finished));
        operations.spawn(file_id(7), std::future::pending());
        operations.tasks.abort_all();

        let (response, measured) = operations.next().await.unwrap();

        assert!(matches!(
            response,
            IndexerResponse::IndexFailed { file_id: cut, .. } if cut == file_id(7)
        ));
        assert!(measured.is_none());
        assert_eq!(finished.failed.load(Ordering::Relaxed), 0);
        assert!(operations.files.is_empty());
    }

    /// The WAL file name of [`file_id`]`(seq)`, under `dir`. The file need
    /// not exist: the fake executables never read it.
    fn wal_path(dir: &Path, seq: u64) -> PathBuf {
        dir.join(file_id(seq).to_filename("wal"))
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

    /// An indexer for logs whose operations run `executable`.
    fn spawn_indexer(
        executable: PathBuf,
        cancel: &CancellationToken,
    ) -> ComponentHandle<IndexerRequest, IndexerResponse> {
        let indexing_operations =
            IndexingOperations::with_executable(executable, limits_from(None, None).unwrap());
        ComponentHandle::spawn::<Indexer>((Signal::Logs, indexing_operations), cancel.child_token())
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
    fn expected_memory_forgets_the_operations_before_the_recent_ones() {
        let mut expected_memory = ExpectedMemory::default();

        expected_memory.learn(100 * MIB, 900 * MIB);
        for _ in 0..ExpectedMemory::RECENT_OPERATIONS - 1 {
            expected_memory.learn(100 * MIB, 300 * MIB);
        }
        assert_eq!(expected_memory.of(10 * MIB), at_ratio(10 * MIB, 9.0));

        expected_memory.learn(100 * MIB, 300 * MIB);
        assert_eq!(expected_memory.of(10 * MIB), at_ratio(10 * MIB, 3.0));
    }

    #[test]
    fn an_operation_on_a_small_file_does_not_teach_the_ratio() {
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
    fn indexing_operations_leave_one_cpu_and_never_drop_to_none() {
        let cpus = std::thread::available_parallelism().unwrap().get();

        assert_eq!(
            max_indexing_operations(),
            if cpus > 1 { cpus - 1 } else { 1 }
        );
    }

    #[test]
    fn each_environment_variable_replaces_its_own_limit() {
        let usual = Limits {
            jobs: max_indexing_operations(),
            memory_percent: MEMORY_PERCENT,
        };

        assert_eq!(limits_from(None, None).unwrap(), usual);
        assert_eq!(
            limits_from(Some(OsStr::new("1")), None).unwrap(),
            Limits { jobs: 1, ..usual }
        );
        assert_eq!(
            limits_from(None, Some(OsStr::new("40"))).unwrap(),
            Limits {
                memory_percent: 40,
                ..usual
            }
        );
        assert_eq!(
            limits_from(Some(OsStr::new("500")), Some(OsStr::new("100"))).unwrap(),
            Limits {
                jobs: 500,
                memory_percent: 100
            }
        );
    }

    #[test]
    fn an_environment_variable_with_an_invalid_value_is_an_error() {
        let invalid = ["0", "", "-1", "four", " 4", "4.5"];
        for value in invalid {
            let error = limits_from(Some(OsStr::new(value)), None).unwrap_err();
            assert!(
                error.to_string().contains(MAX_OPERATIONS_VARIABLE),
                "{value:?}: {error}"
            );
        }

        let invalid = ["0", "101", "", "-1", "half", "75%"];
        for value in invalid {
            let error = limits_from(None, Some(OsStr::new(value))).unwrap_err();
            assert!(
                error.to_string().contains(MEMORY_PERCENT_VARIABLE),
                "{value:?}: {error}"
            );
        }
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
    async fn an_operation_reports_the_decoded_size_and_the_peak_its_child_answered() {
        let dir = tempfile::tempdir().unwrap();
        let wal = write_wal(dir.path(), 3);
        let answer = dir.path().join("answer");
        let indexed = IndexedFile {
            summary: summary(),
            size: 4096,
            peak_memory: Some(77 * MIB),
            incomplete_tail: 0,
        };
        std::fs::write(&answer, indexed.encode().unwrap()).unwrap();
        let executable = fake_executable(dir.path(), &format!("cat {}", answer.display()));
        let indexing_operations =
            IndexingOperations::with_executable(executable, limits_from(None, None).unwrap());

        let size = decoded_size(&wal).await;
        let (response, measured) = indexing_operations
            .run(
                Signal::Logs,
                file_id(1),
                wal,
                dir.path().join("out.sfst"),
                size,
                0,
            )
            .await;

        assert_eq!(size, Some(3 * 1024));
        assert!(
            matches!(response, IndexerResponse::Indexed { .. }),
            "{response:?}"
        );
        let measured = measured.expect("the operation reported what it needed");
        assert_eq!(
            (measured.decoded_size, measured.peak_memory),
            (3 * 1024, 77 * MIB)
        );
    }

    #[tokio::test]
    async fn finished_operations_are_counted_by_their_outcome() {
        let dir = tempfile::tempdir().unwrap();
        let answer = dir.path().join("answer");
        let indexed = IndexedFile {
            summary: summary(),
            size: 4096,
            peak_memory: None,
            incomplete_tail: 0,
        };
        std::fs::write(&answer, indexed.encode().unwrap()).unwrap();
        // One answer that decodes, one that does not, one failed child.
        let script = format!(
            r#"case "$*" in *good.wal*) cat {};; *garbage.wal*) echo garbage;; *) exit 3;; esac"#,
            answer.display()
        );
        let executable = fake_executable(dir.path(), &script);
        let indexing_operations =
            IndexingOperations::with_executable(executable, limits_from(None, None).unwrap());

        for name in ["good.wal", "garbage.wal", "failed.wal"] {
            let wal = dir.path().join(name);
            let sfst = dir.path().join("out.sfst");
            indexing_operations
                .run(Signal::Logs, file_id(1), wal, sfst, None, 0)
                .await;
        }

        let statistics = indexing_operations.statistics();
        assert_eq!((statistics.succeeded, statistics.failed), (1, 2));
    }

    #[tokio::test]
    async fn a_killed_child_leaves_no_temporary_index_file() {
        let dir = tempfile::tempdir().unwrap();
        let sfst = dir.path().join("out.sfst");
        let temporary = file_registry::durable::tmp_path(&sfst);
        let started = dir.path().join("out.sfst.started");
        // The child opens the temporary file of its index and is killed
        // before it can rename it.
        let executable =
            fake_executable(dir.path(), r#": > "$7.started"; : > "$7.tmp"; kill -9 $$"#);
        let indexing_operations =
            IndexingOperations::with_executable(executable, limits_from(None, None).unwrap());

        let wal = dir.path().join("in.wal");
        let (response, _) = indexing_operations
            .run(Signal::Logs, file_id(1), wal, sfst, None, 0)
            .await;

        assert!(
            matches!(response, IndexerResponse::IndexFailed { .. }),
            "{response:?}"
        );
        assert!(started.exists(), "the child did not get the index path");
        assert!(!temporary.exists());
    }

    #[tokio::test]
    async fn a_child_whose_answer_does_not_decode_leaves_no_index_file() {
        let dir = tempfile::tempdir().unwrap();
        let sfst = dir.path().join("out.sfst");
        // The child puts its index file in place and exits with status 0,
        // but what it prints is not an answer.
        let executable = fake_executable(dir.path(), r#": > "$7"; printf garbage"#);
        let indexing_operations =
            IndexingOperations::with_executable(executable, limits_from(None, None).unwrap());

        let wal = dir.path().join("in.wal");
        let (response, _) = indexing_operations
            .run(Signal::Logs, file_id(1), wal, sfst.clone(), None, 0)
            .await;

        assert!(
            matches!(response, IndexerResponse::IndexFailed { .. }),
            "{response:?}"
        );
        assert!(!sfst.exists());
    }

    #[tokio::test]
    async fn a_child_that_fails_after_its_index_file_is_in_place_leaves_none() {
        let dir = tempfile::tempdir().unwrap();
        let sfst = dir.path().join("out.sfst");
        // The child puts its index file in place and then fails, as one that
        // cannot write its answer does.
        let executable = fake_executable(dir.path(), r#": > "$7"; exit 1"#);
        let indexing_operations =
            IndexingOperations::with_executable(executable, limits_from(None, None).unwrap());

        let wal = dir.path().join("in.wal");
        let (response, _) = indexing_operations
            .run(Signal::Logs, file_id(1), wal, sfst.clone(), None, 0)
            .await;

        assert!(
            matches!(response, IndexerResponse::IndexFailed { .. }),
            "{response:?}"
        );
        assert!(!sfst.exists());
    }

    #[tokio::test]
    async fn a_file_that_cannot_be_sized_is_indexed_without_an_expectation() {
        let dir = tempfile::tempdir().unwrap();

        assert_eq!(decoded_size(&dir.path().join("missing.wal")).await, None);
    }

    #[tokio::test]
    async fn an_operation_runs_the_index_command_and_answers_with_the_childs_answer() {
        let dir = tempfile::tempdir().unwrap();
        let (wal, sfst) = (wal_path(dir.path(), 7), dir.path().join("7.sfst"));
        let (answer, seen) = (dir.path().join("answer"), dir.path().join("seen"));
        let indexed = IndexedFile {
            summary: summary(),
            size: 4096,
            peak_memory: Some(1 << 30),
            incomplete_tail: 0,
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
                file_id: file_id(7),
                wal_path: wal.clone(),
                sfst_path: sfst.clone(),
            })
            .unwrap();
        let response = indexer.recv().await.unwrap();

        let IndexerResponse::Indexed {
            file_id: got_file_id,
            summary: got_summary,
            size,
        } = response
        else {
            panic!("expected Indexed, got {response:?}");
        };
        assert_eq!((got_file_id, size.0), (file_id(7), 4096));
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

    /// The `IndexFailed` error an indexer answers with, for the file with seq
    /// 9, when its operations run `script`.
    async fn failure_of(script: &str) -> String {
        let dir = tempfile::tempdir().unwrap();
        let executable = fake_executable(dir.path(), script);
        let mut indexer = spawn_indexer(executable, &CancellationToken::new());

        indexer
            .send(IndexerRequest::Index {
                file_id: file_id(9),
                wal_path: wal_path(dir.path(), 9),
                sfst_path: dir.path().join("9.sfst"),
            })
            .unwrap();
        let response = indexer.recv().await.unwrap();

        let IndexerResponse::IndexFailed {
            file_id: failed,
            error,
        } = response
        else {
            panic!("expected IndexFailed, got {response:?}");
        };
        assert_eq!(failed, file_id(9));
        error
    }

    #[tokio::test]
    async fn a_failed_child_answers_index_failed_with_its_error() {
        let error = failure_of("echo 'error: no such file' >&2; exit 1").await;

        assert_eq!(error, "the job exited with status 1: error: no such file");
    }

    #[tokio::test]
    async fn a_killed_child_answers_index_failed_with_the_signal() {
        let error = failure_of("kill -KILL $$").await;

        assert_eq!(error, "the job was ended by signal 9");
    }

    #[tokio::test]
    async fn an_answer_that_does_not_decode_answers_index_failed() {
        let error = failure_of("printf garbage").await;

        assert!(
            error.starts_with("decoding the index child's answer"),
            "{error}"
        );
    }

    #[tokio::test]
    async fn operations_start_in_the_order_of_their_requests() {
        let dir = tempfile::tempdir().unwrap();
        let started = dir.path().join("started");
        // A child that notes which WAL file it was started for, and fails.
        let executable = fake_executable(
            dir.path(),
            &format!(r#"echo "$5" >> {}; exit 1"#, started.display()),
        );
        // One at a time, so the order of the notes is the scheduler's order.
        let one_at_a_time = Limits {
            jobs: 1,
            memory_percent: 100,
        };
        let indexing_operations = IndexingOperations::with_executable(executable, one_at_a_time);
        let cancel = CancellationToken::new();
        let mut indexer = ComponentHandle::spawn::<Indexer>(
            (Signal::Logs, indexing_operations),
            cancel.child_token(),
        );

        let requested: Vec<PathBuf> = (1..=8).map(|seq| wal_path(dir.path(), seq)).collect();
        for (seq, wal_path) in (1..=8).zip(&requested) {
            indexer
                .send(IndexerRequest::Index {
                    file_id: file_id(seq),
                    wal_path: wal_path.clone(),
                    sfst_path: dir.path().join("out.sfst"),
                })
                .unwrap();
        }
        for _ in &requested {
            indexer.recv().await.unwrap();
        }

        let started: Vec<PathBuf> = std::fs::read_to_string(&started)
            .unwrap()
            .lines()
            .map(PathBuf::from)
            .collect();
        assert_eq!(started, requested);
    }

    #[tokio::test]
    async fn responses_arrive_as_operations_finish() {
        let dir = tempfile::tempdir().unwrap();
        // The operation of the file with seq 1 takes longer than that of seq 2.
        let executable = fake_executable(
            dir.path(),
            "case \"$5\" in *0000000001-*) sleep 0.3;; esac; echo failed >&2; exit 1",
        );
        let mut indexer = spawn_indexer(executable, &CancellationToken::new());

        for seq in [1, 2] {
            indexer
                .send(IndexerRequest::Index {
                    file_id: file_id(seq),
                    wal_path: wal_path(dir.path(), seq),
                    sfst_path: dir.path().join(format!("{seq}.sfst")),
                })
                .unwrap();
        }
        let mut finished = Vec::new();
        for _ in 0..2 {
            let IndexerResponse::IndexFailed { file_id, .. } = indexer.recv().await.unwrap() else {
                panic!("expected IndexFailed");
            };
            finished.push(file_id.seq);
        }

        assert_eq!(finished, [2, 1]);
    }

    #[tokio::test]
    async fn cancelling_the_indexer_kills_its_running_operations() {
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
                file_id: file_id(3),
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
            .expect("the operation never started");

        cancel.cancel();

        // The component ends without a response for the operation, and the
        // child is gone: its /proc entry disappears or shows a zombie.
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
