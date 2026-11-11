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
//! [`IndexBuilds`], whose scheduler bounds how many run at once across the
//! process ([`MAX_INDEX_BUILDS`]); the builds beyond that wait in its queue,
//! in the order they were requested.
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

use std::path::PathBuf;
use std::process::Command;

use anyhow::Context;
use otel_bridge::signals::Signal;
use otel_job_scheduler::Scheduler;
use tokio::sync::mpsc;
use tokio::task::JoinSet;
use tokio::time::Instant;
use tokio_util::sync::CancellationToken;

use file_lifecycle::component::Component;
use file_lifecycle::ipc::{IndexerRequest, IndexerResponse};

use crate::index_child::{self, IndexedFile};

/// How many index builds run at once, over both signals. It is the number
/// that could run before the builds moved to child processes: one per signal.
pub const MAX_INDEX_BUILDS: usize = 2;

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
            scheduler: Scheduler::new(MAX_INDEX_BUILDS),
            executable,
        }
    }

    /// Build the index file a request asks for and wait for the result.
    async fn build(&self, signal: Signal, request: IndexerRequest) -> IndexerResponse {
        let IndexerRequest::Index {
            wal_path,
            sfst_path,
        } = request;

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
            "indexing queued wal={} index={}",
            wal_path.display(),
            sfst_path.display(),
        );
        let queued_at = Instant::now();

        let indexed = self
            .scheduler
            .run(command)
            .await
            .map_err(anyhow::Error::from)
            .and_then(|answer| IndexedFile::decode(&answer));

        match indexed {
            Ok(IndexedFile { summary, size }) => {
                // The time includes the wait for a free slot.
                tracing::info!(
                    "indexing done seq={seq} elapsed_ms={}",
                    queued_at.elapsed().as_millis(),
                );
                IndexerResponse::Indexed {
                    seq,
                    path: sfst_path,
                    summary,
                    size: file_registry::ByteSize(size),
                }
            }
            Err(e) => {
                tracing::error!("indexing failed wal={}: {e:#}", wal_path.display());
                IndexerResponse::IndexFailed {
                    path: wal_path,
                    error: format!("{e:#}"),
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
    type Args = (Signal, IndexBuilds);

    async fn run(
        (signal, index_builds): Self::Args,
        mut rx: mpsc::UnboundedReceiver<IndexerRequest>,
        tx: mpsc::UnboundedSender<IndexerResponse>,
        cancel: CancellationToken,
    ) {
        // Dropped when the loop exits, which abandons the builds still in it.
        let mut builds = JoinSet::new();

        loop {
            tokio::select! {
                _ = cancel.cancelled() => break,
                request = rx.recv() => match request {
                    Some(request) => {
                        let index_builds = index_builds.clone();
                        builds.spawn(async move { index_builds.build(signal, request).await });
                    }
                    None => break,
                },
                Some(built) = builds.join_next() => {
                    // A build task ends without a response only if it
                    // panicked: there is nothing to forward then.
                    if let Ok(response) = built {
                        let _ = tx.send(response);
                    }
                }
            }
        }
    }
}

#[cfg(test)]
mod tests {
    use std::path::Path;

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

    #[tokio::test]
    async fn a_build_runs_the_index_command_and_answers_with_the_childs_answer() {
        let dir = tempfile::tempdir().unwrap();
        let (wal, sfst) = (wal_path(dir.path(), 7), dir.path().join("7.sfst"));
        let (answer, seen) = (dir.path().join("answer"), dir.path().join("seen"));
        let indexed = IndexedFile {
            summary: summary(),
            size: 4096,
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
