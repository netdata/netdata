//! Run commands as child processes, a bounded number at once.
//!
//! A [`Scheduler`] takes jobs from any number of callers. A job is a command;
//! the scheduler starts it as a child process when a slot is free, in the
//! order the jobs were submitted, and gives the caller what the child wrote to
//! stdout or the way it failed ([`Error`]). Jobs beyond the limit wait in a
//! queue that has no bound: the limit is on what runs, not on what waits.
//!
//! The work runs in processes, not threads, for three reasons. The memory a
//! job needs goes back to the system when the job ends. A job that fails,
//! or that the kernel kills, takes nothing else with it. And a job can be
//! stopped: a child is killed when nobody waits for its result any more.
//!
//! Each child gets a pipe as stdin that the scheduler never writes to and
//! holds until the child ends. A child that reads end-of-file there knows the
//! scheduler's process is gone, however it ended, and can exit.
//!
//! The crate knows nothing about what the jobs do.

use std::collections::VecDeque;
use std::process::{ExitStatus, Stdio};

use tokio::process::Command;
use tokio::sync::{mpsc, oneshot};
use tokio::task::JoinSet;

/// How a job failed.
#[derive(Debug, thiserror::Error)]
pub enum Error {
    /// The child process could not be started.
    #[error("failed to start the job: {0}")]
    Spawn(#[source] std::io::Error),
    /// The child ran, but waiting for it or reading its output failed.
    #[error("failed to wait for the job: {0}")]
    Wait(#[source] std::io::Error),
    /// The child exited with a non-zero status; `stderr` is what it wrote
    /// there.
    #[error("the job exited with status {code}: {}", .stderr.trim_end())]
    Exited { code: i32, stderr: String },
    /// A signal ended the child.
    #[error("the job was ended by signal {signal}")]
    Signaled { signal: i32 },
    /// The scheduler stopped before the job finished.
    #[error("the scheduler stopped before the job finished")]
    Stopped,
}

/// A job's result: the child's stdout, or how the job failed.
pub type Result = std::result::Result<Vec<u8>, Error>;

/// A job in the queue: the command to run and where its result goes.
struct Job {
    command: std::process::Command,
    result: oneshot::Sender<Result>,
}

/// Runs jobs as child processes, at most a fixed number at once.
///
/// Cheap to clone; every clone submits to the same queue. When the last
/// clone is dropped the scheduler stops: jobs still waiting never start and
/// the running children are killed.
#[derive(Clone)]
pub struct Scheduler {
    queue: mpsc::UnboundedSender<Job>,
}

impl Scheduler {
    /// A scheduler that runs at most `limit` jobs at once.
    ///
    /// Must be called within a tokio runtime: the scheduler's dispatcher is a
    /// task on it.
    ///
    /// # Panics
    ///
    /// If `limit` is zero: no job could ever start.
    pub fn new(limit: usize) -> Self {
        assert!(limit > 0, "a scheduler needs room for at least one job");

        let (queue, jobs) = mpsc::unbounded_channel();
        tokio::spawn(dispatch(limit, jobs));

        Self { queue }
    }

    /// Run `command` as a job and wait for its result.
    ///
    /// The job takes its place in the queue when this is called, not when the
    /// returned future is first polled. The scheduler sets the child's stdin,
    /// stdout and stderr; whatever `command` says about them is replaced.
    ///
    /// Dropping the returned future abandons the job: it never starts if it
    /// is still waiting, and its child is killed if it runs.
    pub fn run(&self, command: std::process::Command) -> impl Future<Output = Result> + use<> {
        let (result, finished) = oneshot::channel();
        // A send fails only when the dispatcher is gone; `finished` then
        // reports it, because the job's sender is dropped with the job.
        let _ = self.queue.send(Job { command, result });

        async move { finished.await.unwrap_or(Err(Error::Stopped)) }
    }
}

/// The scheduler's dispatcher: queue the submitted jobs and start them in
/// order while fewer than `limit` run. Ends when every [`Scheduler`] is gone;
/// dropping `waiting` and `running` then fails the jobs that have not
/// finished, and aborting the running ones kills their children.
async fn dispatch(limit: usize, mut jobs: mpsc::UnboundedReceiver<Job>) {
    let mut waiting = VecDeque::new();
    let mut running = JoinSet::new();

    loop {
        while running.len() < limit
            && let Some(job) = waiting.pop_front()
        {
            running.spawn(run_job(job));
        }

        tokio::select! {
            job = jobs.recv() => match job {
                Some(job) => waiting.push_back(job),
                None => break,
            },
            Some(_) = running.join_next() => {}
        }
    }
}

/// Run one job to its end and send its result, unless the caller stopped
/// waiting: then the job is abandoned. One that has not started never does,
/// and one that runs has its child killed.
async fn run_job(job: Job) {
    let Job {
        command,
        mut result,
    } = job;

    tokio::select! {
        // First, so that an abandoned job's child is never started.
        biased;
        // Leaving `run_child` drops the child, which kills it.
        () = result.closed() => {}
        finished = run_child(command) => {
            let _ = result.send(finished);
        }
    }
}

/// Start `command` as a child process and wait for it to end.
async fn run_child(command: std::process::Command) -> Result {
    let mut child = Command::from(command)
        .stdin(Stdio::piped())
        .stdout(Stdio::piped())
        .stderr(Stdio::piped())
        .kill_on_drop(true)
        .spawn()
        .map_err(Error::Spawn)?;

    // Taken out of the child so that waiting does not close it: the write end
    // stays open, unwritten, until the child has ended.
    let _lifeline = child.stdin.take();

    let output = child.wait_with_output().await.map_err(Error::Wait)?;
    if output.status.success() {
        return Ok(output.stdout);
    }

    Err(failure(output.status, &output.stderr))
}

/// The error of a child that ended with `status`, which is not a success.
fn failure(status: ExitStatus, stderr: &[u8]) -> Error {
    use std::os::unix::process::ExitStatusExt;

    match (status.code(), status.signal()) {
        (Some(code), _) => Error::Exited {
            code,
            stderr: String::from_utf8_lossy(stderr).into_owned(),
        },
        (None, Some(signal)) => Error::Signaled { signal },
        (None, None) => Error::Wait(std::io::Error::other(format!(
            "unexpected wait status {status:?}"
        ))),
    }
}

#[cfg(test)]
mod tests {
    use std::path::Path;
    use std::time::Duration;

    use super::*;

    /// The command `sh -c <script>`.
    fn sh(script: &str) -> std::process::Command {
        let mut command = std::process::Command::new("/bin/sh");
        command.args(["-c", script]);
        command
    }

    /// Whether process `pid` still runs: it exists and is not a zombie.
    fn is_running(pid: u32) -> bool {
        let Ok(stat) = std::fs::read_to_string(format!("/proc/{pid}/stat")) else {
            return false;
        };
        // The state is the first field after the parenthesised command name.
        stat.rsplit_once(") ")
            .is_some_and(|(_, rest)| !rest.starts_with('Z'))
    }

    /// Wait until process `pid` no longer runs.
    async fn wait_until_gone(pid: u32) {
        let gone = async {
            while is_running(pid) {
                tokio::time::sleep(Duration::from_millis(5)).await;
            }
        };
        tokio::time::timeout(Duration::from_secs(5), gone)
            .await
            .expect("the process still runs");
    }

    /// Wait until `path` holds a line, and return its text.
    async fn read_when_written(path: &Path) -> String {
        loop {
            match std::fs::read_to_string(path) {
                Ok(text) if text.ends_with('\n') => return text,
                _ => tokio::time::sleep(Duration::from_millis(5)).await,
            }
        }
    }

    #[tokio::test]
    async fn a_job_returns_what_its_child_wrote_to_stdout() {
        let scheduler = Scheduler::new(1);

        let stdout = scheduler.run(sh("printf answer; echo noise >&2")).await;

        assert_eq!(stdout.unwrap(), b"answer");
    }

    #[tokio::test]
    async fn a_failed_child_reports_its_status_and_stderr() {
        let scheduler = Scheduler::new(1);

        let err = scheduler
            .run(sh("echo partial; echo 'it broke' >&2; exit 3"))
            .await
            .unwrap_err();

        assert!(
            matches!(&err, Error::Exited { code: 3, stderr } if stderr == "it broke\n"),
            "{err:?}"
        );
        assert_eq!(err.to_string(), "the job exited with status 3: it broke");
    }

    #[tokio::test]
    async fn a_child_ended_by_a_signal_reports_the_signal() {
        let scheduler = Scheduler::new(1);

        let err = scheduler.run(sh("kill -KILL $$")).await.unwrap_err();

        assert!(matches!(err, Error::Signaled { signal: 9 }), "{err:?}");
    }

    #[tokio::test]
    async fn a_command_that_cannot_start_reports_the_spawn_error() {
        let scheduler = Scheduler::new(1);
        let command = std::process::Command::new("/nonexistent/otel-job-scheduler-test");

        let err = scheduler.run(command).await.unwrap_err();

        assert!(matches!(err, Error::Spawn(_)), "{err:?}");
    }

    #[tokio::test]
    async fn at_most_the_limit_runs_at_once() {
        let dir = tempfile::tempdir().unwrap();
        let log = dir.path().join("log");
        let scheduler = Scheduler::new(2);

        // Each job logs its start and its end. `echo` appends one short line
        // with one write, so the lines of different jobs do not mix.
        let script = format!(
            "echo start >> {log}; sleep 0.1; echo end >> {log}",
            log = log.display()
        );
        let jobs: Vec<_> = (0..6).map(|_| scheduler.run(sh(&script))).collect();
        for job in jobs {
            job.await.unwrap();
        }

        let mut running = 0;
        let mut most = 0;
        for line in std::fs::read_to_string(&log).unwrap().lines() {
            match line {
                "start" => running += 1,
                "end" => running -= 1,
                other => panic!("unexpected log line {other:?}"),
            }
            most = most.max(running);
        }
        assert_eq!(most, 2);
    }

    #[tokio::test]
    async fn jobs_start_in_the_order_they_were_submitted() {
        let dir = tempfile::tempdir().unwrap();
        let log = dir.path().join("log");
        let scheduler = Scheduler::new(1);

        // Submitted in order, awaited in reverse: the order of the calls
        // decides, not the order in which the futures are polled.
        let mut jobs: Vec<_> = (0..5)
            .map(|i| scheduler.run(sh(&format!("echo {i} >> {}", log.display()))))
            .collect();
        while let Some(job) = jobs.pop() {
            job.await.unwrap();
        }

        assert_eq!(std::fs::read_to_string(&log).unwrap(), "0\n1\n2\n3\n4\n");
    }

    #[tokio::test]
    async fn a_running_job_is_killed_when_its_caller_stops_waiting() {
        let dir = tempfile::tempdir().unwrap();
        let pid_file = dir.path().join("pid");
        let scheduler = Scheduler::new(1);

        let job = scheduler.run(sh(&format!(
            "echo $$ > {}; exec sleep 60",
            pid_file.display()
        )));
        let pid: u32 = read_when_written(&pid_file).await.trim().parse().unwrap();
        assert!(is_running(pid));

        drop(job);

        wait_until_gone(pid).await;
        // The slot is free again.
        assert_eq!(scheduler.run(sh("printf next")).await.unwrap(), b"next");
    }

    #[tokio::test]
    async fn a_waiting_job_never_starts_when_its_caller_stops_waiting() {
        let dir = tempfile::tempdir().unwrap();
        let marker = dir.path().join("started");
        let scheduler = Scheduler::new(1);

        let first = scheduler.run(sh("sleep 0.1"));
        let abandoned = scheduler.run(sh(&format!("echo yes > {}", marker.display())));
        let last = scheduler.run(sh("printf last"));
        drop(abandoned);

        first.await.unwrap();
        assert_eq!(last.await.unwrap(), b"last");
        assert!(!marker.exists());
    }

    #[tokio::test]
    async fn running_jobs_are_killed_when_the_scheduler_is_dropped() {
        let dir = tempfile::tempdir().unwrap();
        let pid_file = dir.path().join("pid");
        let scheduler = Scheduler::new(1);

        let job = scheduler.run(sh(&format!(
            "echo $$ > {}; exec sleep 60",
            pid_file.display()
        )));
        let pid: u32 = read_when_written(&pid_file).await.trim().parse().unwrap();

        drop(scheduler);

        assert!(matches!(job.await, Err(Error::Stopped)));
        wait_until_gone(pid).await;
    }

    #[tokio::test]
    async fn a_child_reads_no_end_of_file_from_stdin_while_it_runs() {
        let scheduler = Scheduler::new(1);

        // `cat` returns at once on end-of-file; on an open pipe that nobody
        // writes to it waits until `timeout` ends it, with status 124.
        let stdout = scheduler.run(sh("timeout 0.2 cat; echo $?")).await.unwrap();

        assert_eq!(stdout, b"124\n");
    }
}
