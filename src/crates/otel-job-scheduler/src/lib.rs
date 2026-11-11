//! Run commands as child processes, as many at once as the limits allow.
//!
//! A [`Scheduler`] takes jobs from any number of callers. A [`Job`] is a
//! command and the memory its child is expected to need. The scheduler starts
//! the jobs as child processes in the order they were submitted, and gives
//! each caller what the child wrote to stdout or the way it failed
//! ([`Error`]), with the time the job took ([`Usage`]).
//!
//! Two [`Limits`] decide when the job at the head of the queue starts: a
//! number of jobs, and a share of the system's free memory that the expected
//! memory of the running jobs may add up to. The head waits until both allow
//! it and nothing passes it, so a large job cannot be starved by smaller ones
//! behind it. One job may always run, whatever its size, so the queue always
//! moves. The queue itself has no bound: the limits are on what runs, not on
//! what waits. [`Scheduler::queue`] tells how many jobs run, how many wait,
//! and for how long the first of them has.
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
//! The crate knows nothing about what the jobs do. What it must ask the
//! system it runs on is in `system`, one implementation per system.

mod system;

use std::collections::{HashMap, VecDeque};
use std::process::{ExitStatus, Stdio};
use std::sync::Arc;
use std::sync::atomic::{AtomicU32, Ordering};
use std::time::Duration;

use tokio::process::Command;
use tokio::sync::{mpsc, oneshot, watch};
use tokio::task::JoinSet;
use tokio::time::Instant;

use system::System;

/// What the jobs of a scheduler may use together.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct Limits {
    /// How many jobs may run at once.
    pub jobs: usize,
    /// The share of the system's free memory, in percent, that the expected
    /// memory of the running jobs may add up to. Free memory here is what
    /// would be free if none of the scheduler's jobs ran: what the system
    /// reports as free when a job is about to start, plus what the running
    /// children hold at that moment.
    ///
    /// On a system where the scheduler cannot read these numbers, only `jobs`
    /// limits it.
    pub memory_percent: u8,
}

/// A scheduler's queue at one moment.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct Queue {
    /// How many jobs run.
    pub running: usize,
    /// How many jobs wait for their start.
    pub waiting: usize,
    /// How long the job at the head of the queue has waited; `None` when no
    /// job waits.
    pub oldest_wait: Option<Duration>,
}

/// A job: a command to run as a child process.
#[derive(Debug)]
pub struct Job {
    /// The scheduler sets the child's stdin, stdout and stderr; whatever the
    /// command says about them is replaced.
    pub command: std::process::Command,
    /// The memory the child is expected to need at its peak, in bytes.
    pub expected_memory: u64,
}

/// What a job cost in time.
///
/// Not what it cost in memory: the scheduler does not measure that. A job
/// that should report its memory measures itself and says so on stdout.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct Usage {
    /// How long the job waited in the queue before its child was started.
    pub waited: Duration,
    /// How long its child ran.
    pub ran: Duration,
}

/// A job whose child ended with success.
#[derive(Debug)]
pub struct Finished {
    /// What the child wrote to stdout.
    pub stdout: Vec<u8>,
    pub usage: Usage,
}

/// How a job failed.
///
/// The I/O errors are part of the message and not a `source`: a caller that
/// prints the whole chain would print them twice.
#[derive(Debug, thiserror::Error)]
pub enum Error {
    /// The child process could not be started.
    #[error("failed to start the job: {0}")]
    Spawn(std::io::Error),
    /// The child ran, but waiting for it or reading its output failed.
    #[error("failed to wait for the job: {0}")]
    Wait(std::io::Error),
    /// The child exited with a non-zero status; `stderr` is what it wrote
    /// there.
    #[error("the job exited with status {code}: {}", .stderr.trim_end())]
    Exited {
        code: i32,
        stderr: String,
        usage: Usage,
    },
    /// A signal ended the child.
    #[error("the job was ended by signal {signal}")]
    Signaled { signal: i32, usage: Usage },
    /// The scheduler stopped before the job finished.
    #[error("the scheduler stopped before the job finished")]
    Stopped,
}

impl Error {
    /// What the job cost, when its child ran to an end.
    pub fn usage(&self) -> Option<Usage> {
        match self {
            Error::Exited { usage, .. } | Error::Signaled { usage, .. } => Some(*usage),
            Error::Spawn(_) | Error::Wait(_) | Error::Stopped => None,
        }
    }
}

/// A job's result: what its child produced, or how the job failed.
pub type Result = std::result::Result<Finished, Error>;

/// A job in the queue: the job, when it was submitted, and where its
/// result goes.
struct Queued {
    job: Job,
    queued_at: Instant,
    result: oneshot::Sender<Result>,
}

/// What the dispatcher tells of its queue, after every change of it.
#[derive(Debug, Clone, Copy, Default)]
struct QueueState {
    running: usize,
    waiting: usize,
    /// When the job at the head of the queue was submitted.
    oldest_queued_at: Option<Instant>,
}

/// What the dispatcher knows of a job that runs.
struct Running {
    expected_memory: u64,
    /// The child's process id; 0 until the child is started.
    pid: Arc<AtomicU32>,
}

/// Runs jobs as child processes, as many at once as its [`Limits`] allow.
///
/// Cheap to clone; every clone submits to the same queue. When the last
/// clone is dropped the scheduler stops: jobs still waiting never start and
/// the running children are killed.
#[derive(Clone)]
pub struct Scheduler {
    queue: mpsc::UnboundedSender<Queued>,
    queue_state: watch::Receiver<QueueState>,
}

impl Scheduler {
    /// A scheduler whose jobs stay within `limits`.
    ///
    /// Must be called within a tokio runtime: the scheduler's dispatcher is a
    /// task on it.
    ///
    /// # Panics
    ///
    /// If `limits.jobs` is zero: no job could ever start.
    pub fn new(limits: Limits) -> Self {
        Self::on(limits, system::Host)
    }

    /// A scheduler that asks `system` for the memory numbers.
    fn on(limits: Limits, system: impl System) -> Self {
        assert!(
            limits.jobs > 0,
            "a scheduler needs room for at least one job"
        );

        let (queue, jobs) = mpsc::unbounded_channel();
        let (state, queue_state) = watch::channel(QueueState::default());
        tokio::spawn(dispatch(limits, system, jobs, state));

        Self { queue, queue_state }
    }

    /// The queue as the dispatcher last left it: a job counts from the
    /// moment the dispatcher has taken it in, which is at once unless the
    /// runtime is busy.
    pub fn queue(&self) -> Queue {
        let state = *self.queue_state.borrow();

        Queue {
            running: state.running,
            waiting: state.waiting,
            oldest_wait: state.oldest_queued_at.map(|queued_at| queued_at.elapsed()),
        }
    }

    /// Run `job` and wait for its result.
    ///
    /// The job takes its place in the queue when this is called, not when the
    /// returned future is first polled.
    ///
    /// Dropping the returned future abandons the job: it never starts if it
    /// is still waiting, and its child is killed if it runs.
    pub fn run(&self, job: Job) -> impl Future<Output = Result> + use<> {
        let (result, finished) = oneshot::channel();
        // A send fails only when the dispatcher is gone; `finished` then
        // reports it, because the job's sender is dropped with the job.
        let _ = self.queue.send(Queued {
            job,
            queued_at: Instant::now(),
            result,
        });

        async move { finished.await.unwrap_or(Err(Error::Stopped)) }
    }
}

/// The scheduler's dispatcher: queue the submitted jobs and start the one at
/// the head while the limits allow it. Ends when every [`Scheduler`] is gone;
/// dropping `waiting` and `running` then fails the jobs that have not
/// finished, and aborting the running ones kills their children.
async fn dispatch(
    limits: Limits,
    system: impl System,
    mut jobs: mpsc::UnboundedReceiver<Queued>,
    state: watch::Sender<QueueState>,
) {
    let mut waiting: VecDeque<Queued> = VecDeque::new();
    let mut running = JoinSet::new();
    let mut running_jobs = HashMap::new();

    loop {
        while let Some(next) = waiting.front()
            && admits(limits, &system, &running_jobs, next)
            && let Some(queued) = waiting.pop_front()
        {
            let job = Running {
                expected_memory: queued.job.expected_memory,
                pid: Arc::default(),
            };
            let task = running.spawn(run_job(queued, Arc::clone(&job.pid)));
            running_jobs.insert(task.id(), job);
        }

        state.send_replace(QueueState {
            running: running_jobs.len(),
            waiting: waiting.len(),
            oldest_queued_at: waiting.front().map(|queued| queued.queued_at),
        });

        tokio::select! {
            queued = jobs.recv() => match queued {
                Some(queued) => waiting.push_back(queued),
                None => break,
            },
            Some(ended) = running.join_next_with_id() => {
                let task = match ended {
                    Ok((task, ())) => task,
                    Err(e) => e.id(),
                };
                running_jobs.remove(&task);
            }
        }
    }
}

/// Whether the job at the head of the queue may start now.
fn admits(
    limits: Limits,
    system: &impl System,
    running: &HashMap<tokio::task::Id, Running>,
    next: &Queued,
) -> bool {
    // One job may always run: the queue must move, even past a job that
    // expects more than the whole budget.
    if running.is_empty() {
        return true;
    }
    if running.len() >= limits.jobs {
        return false;
    }

    // What is free now does not hold what the running children took: give
    // it back to see what would be free without them.
    let Some(free_now) = system.free_memory() else {
        return true;
    };
    let held: u64 = running
        .values()
        .filter_map(|job| match job.pid.load(Ordering::Relaxed) {
            0 => None,
            pid => system.resident_memory(pid),
        })
        .sum();
    let expected: u64 = running.values().map(|job| job.expected_memory).sum();

    fits(
        limits.memory_percent,
        free_now.saturating_add(held),
        expected,
        next.job.expected_memory,
    )
}

/// Whether a job that expects `next` bytes fits beside running jobs that
/// expect `running` bytes, when they may use `percent` of `free` bytes.
fn fits(percent: u8, free: u64, running: u64, next: u64) -> bool {
    let budget = u128::from(free) * u128::from(percent) / 100;

    u128::from(running) + u128::from(next) <= budget
}

/// Run one job to its end and send its result, unless the caller stopped
/// waiting: then the job is abandoned. One that has not started never does,
/// and one that runs has its child killed.
async fn run_job(queued: Queued, pid: Arc<AtomicU32>) {
    let Queued {
        job,
        queued_at,
        mut result,
    } = queued;

    tokio::select! {
        // First, so that an abandoned job's child is never started.
        biased;
        // Leaving `run_child` drops the child, which kills it.
        () = result.closed() => {}
        finished = run_child(job.command, queued_at, &pid) => {
            let _ = result.send(finished);
        }
    }
}

/// Start `command` as a child process and wait for it to end. The child's
/// process id goes to `pid` once it is started.
async fn run_child(command: std::process::Command, queued_at: Instant, pid: &AtomicU32) -> Result {
    let started_at = Instant::now();
    let mut child = Command::from(command)
        .stdin(Stdio::piped())
        .stdout(Stdio::piped())
        .stderr(Stdio::piped())
        .kill_on_drop(true)
        .spawn()
        .map_err(Error::Spawn)?;
    pid.store(child.id().unwrap_or(0), Ordering::Relaxed);

    // Taken out of the child so that waiting does not close it: the write end
    // stays open, unwritten, until the child has ended.
    let _lifeline = child.stdin.take();

    let output = child.wait_with_output().await.map_err(Error::Wait)?;
    let usage = Usage {
        waited: started_at.duration_since(queued_at),
        ran: started_at.elapsed(),
    };
    if output.status.success() {
        return Ok(Finished {
            stdout: output.stdout,
            usage,
        });
    }

    Err(failure(output.status, &output.stderr, usage))
}

/// The error of a child that ended with `status`, which is not a success.
fn failure(status: ExitStatus, stderr: &[u8], usage: Usage) -> Error {
    match (status.code(), system::ending_signal(&status)) {
        (Some(code), _) => Error::Exited {
            code,
            stderr: String::from_utf8_lossy(stderr).into_owned(),
            usage,
        },
        (None, Some(signal)) => Error::Signaled { signal, usage },
        (None, None) => Error::Wait(std::io::Error::other(format!(
            "unexpected wait status {status:?}"
        ))),
    }
}

#[cfg(test)]
mod tests {
    use std::path::Path;

    use super::*;

    /// The job `sh -c <script>`, expected to need no memory.
    fn sh(script: &str) -> Job {
        sh_expecting(0, script)
    }

    /// The job `sh -c <script>`, expected to need `expected_memory` bytes.
    fn sh_expecting(expected_memory: u64, script: &str) -> Job {
        let mut command = std::process::Command::new("/bin/sh");
        command.args(["-c", script]);
        Job {
            command,
            expected_memory,
        }
    }

    /// A job whose command cannot be started.
    fn unstartable() -> Job {
        Job {
            command: std::process::Command::new("/nonexistent/otel-job-scheduler-test"),
            expected_memory: 0,
        }
    }

    /// A scheduler on this host that runs at most `jobs` jobs at once.
    fn scheduler(jobs: usize) -> Scheduler {
        Scheduler::new(Limits {
            jobs,
            memory_percent: 100,
        })
    }

    /// A system with fixed answers: `free` bytes are free, and every running
    /// child holds `held_by_a_child` bytes.
    struct FixedSystem {
        free: Option<u64>,
        held_by_a_child: u64,
    }

    impl System for FixedSystem {
        fn free_memory(&self) -> Option<u64> {
            self.free
        }

        fn resident_memory(&self, _pid: u32) -> Option<u64> {
            Some(self.held_by_a_child)
        }
    }

    /// A scheduler of up to 10 jobs that may use three quarters of the free
    /// memory of `system`.
    fn scheduler_on(system: FixedSystem) -> Scheduler {
        let limits = Limits {
            jobs: 10,
            memory_percent: 75,
        };
        Scheduler::on(limits, system)
    }

    /// The most jobs that ran at once, from a log of `start` and `end` lines.
    fn most_at_once(log: &Path) -> i32 {
        let mut running = 0;
        let mut most = 0;
        for line in std::fs::read_to_string(log).unwrap().lines() {
            match line {
                "start" => running += 1,
                "end" => running -= 1,
                other => panic!("unexpected log line {other:?}"),
            }
            most = most.max(running);
        }
        most
    }

    /// A script that logs its start, runs for `seconds`, and logs its end.
    /// `echo` appends one short line with one write, so the lines of
    /// different jobs do not mix.
    fn logged(log: &Path, seconds: &str) -> String {
        format!(
            "echo start >> {log}; sleep {seconds}; echo end >> {log}",
            log = log.display()
        )
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
        let scheduler = scheduler(1);

        let finished = scheduler.run(sh("printf answer; echo noise >&2")).await;

        assert_eq!(finished.unwrap().stdout, b"answer");
    }

    #[tokio::test]
    async fn a_failed_child_reports_its_status_and_stderr() {
        let scheduler = scheduler(1);

        let err = scheduler
            .run(sh("echo partial; echo 'it broke' >&2; exit 3"))
            .await
            .unwrap_err();

        assert!(
            matches!(&err, Error::Exited { code: 3, stderr, .. } if stderr == "it broke\n"),
            "{err:?}"
        );
        assert_eq!(err.to_string(), "the job exited with status 3: it broke");
    }

    #[tokio::test]
    async fn a_child_ended_by_a_signal_reports_the_signal() {
        let scheduler = scheduler(1);

        let err = scheduler.run(sh("kill -KILL $$")).await.unwrap_err();

        assert!(matches!(err, Error::Signaled { signal: 9, .. }), "{err:?}");
    }

    #[tokio::test]
    async fn a_command_that_cannot_start_reports_the_spawn_error() {
        let scheduler = scheduler(1);
        let err = scheduler.run(unstartable()).await.unwrap_err();

        assert!(matches!(err, Error::Spawn(_)), "{err:?}");
        assert!(
            err.to_string().contains("No such file or directory"),
            "{err}"
        );
        assert!(std::error::Error::source(&err).is_none());
    }

    #[tokio::test]
    async fn the_queue_tells_what_runs_and_what_waits() {
        let scheduler = scheduler(1);
        let empty = Queue {
            running: 0,
            waiting: 0,
            oldest_wait: None,
        };
        assert_eq!(scheduler.queue(), empty);

        let first = scheduler.run(sh("sleep 30"));
        let second = scheduler.run(sh("sleep 30"));
        tokio::time::sleep(Duration::from_millis(150)).await;
        let third = scheduler.run(sh("sleep 30"));
        queue_becomes(&scheduler, |queue| queue.waiting == 2).await;

        let queue = scheduler.queue();
        assert_eq!(queue.running, 1);
        // The second job's wait, not the third's.
        assert!(queue.oldest_wait.unwrap() >= Duration::from_millis(150));

        // Abandoned: the running job's child is killed, and the waiting ones
        // leave the queue as they reach its head.
        drop((first, second, third));
        queue_becomes(&scheduler, |queue| queue == empty).await;
    }

    /// Wait until the scheduler's queue is as `wanted` says.
    async fn queue_becomes(scheduler: &Scheduler, wanted: impl Fn(Queue) -> bool) {
        let became = async {
            while !wanted(scheduler.queue()) {
                tokio::time::sleep(Duration::from_millis(5)).await;
            }
        };

        tokio::time::timeout(Duration::from_secs(10), became)
            .await
            .expect("the queue did not get there");
    }

    #[tokio::test]
    async fn at_most_the_job_limit_runs_at_once() {
        let dir = tempfile::tempdir().unwrap();
        let log = dir.path().join("log");
        let scheduler = scheduler(2);

        let jobs: Vec<_> = (0..6)
            .map(|_| scheduler.run(sh(&logged(&log, "0.1"))))
            .collect();
        for job in jobs {
            job.await.unwrap();
        }

        assert_eq!(most_at_once(&log), 2);
    }

    #[test]
    fn a_job_fits_while_the_expected_memory_stays_within_the_share() {
        // Three quarters of 1,000 bytes: 750.
        assert!(fits(75, 1_000, 300, 450));
        assert!(!fits(75, 1_000, 300, 451));
        assert!(fits(75, 1_000, 0, 750));
        assert!(!fits(75, 1_000, 750, 1));
        assert!(fits(100, 1_000, 400, 600));
        assert!(!fits(0, 1_000, 0, 1));
        // No overflow at the ends of the range.
        assert!(!fits(100, u64::MAX, u64::MAX, 1));
        assert!(fits(100, u64::MAX, u64::MAX, 0));
    }

    #[tokio::test]
    async fn the_expected_memory_of_the_running_jobs_stays_within_the_budget() {
        let dir = tempfile::tempdir().unwrap();
        let log = dir.path().join("log");
        // The budget is 750: two jobs that expect 300 fit, a third does not.
        let scheduler = scheduler_on(FixedSystem {
            free: Some(1_000),
            held_by_a_child: 0,
        });

        let jobs: Vec<_> = (0..6)
            .map(|_| scheduler.run(sh_expecting(300, &logged(&log, "0.1"))))
            .collect();
        for job in jobs {
            job.await.unwrap();
        }

        assert_eq!(most_at_once(&log), 2);
    }

    #[tokio::test]
    async fn what_the_running_children_hold_counts_as_free() {
        let dir = tempfile::tempdir().unwrap();
        let log = dir.path().join("log");
        // Every running child gives 100 back to "free": the budget is 825
        // beside one child, 900 beside two, 975 beside three. So a third job
        // that expects 300 fits (900), and a fourth (1,200) does not.
        let scheduler = scheduler_on(FixedSystem {
            free: Some(1_000),
            held_by_a_child: 100,
        });

        let mut jobs = Vec::new();
        for _ in 0..4 {
            jobs.push(scheduler.run(sh_expecting(300, &logged(&log, "0.5"))));
            // Each job arrives when the ones before it have started.
            tokio::time::sleep(Duration::from_millis(60)).await;
        }
        for job in jobs {
            job.await.unwrap();
        }

        assert_eq!(most_at_once(&log), 3);
    }

    #[tokio::test]
    async fn a_job_that_expects_more_than_the_budget_runs_alone() {
        let dir = tempfile::tempdir().unwrap();
        let log = dir.path().join("log");
        let scheduler = scheduler_on(FixedSystem {
            free: Some(1_000),
            held_by_a_child: 0,
        });

        let large = scheduler.run(sh_expecting(5_000, &logged(&log, "0.1")));
        let small = scheduler.run(sh_expecting(1, &logged(&log, "0.1")));
        large.await.unwrap();
        small.await.unwrap();

        assert_eq!(most_at_once(&log), 1);
    }

    #[tokio::test]
    async fn nothing_passes_the_job_at_the_head_of_the_queue() {
        let dir = tempfile::tempdir().unwrap();
        let log = dir.path().join("log");
        let scheduler = scheduler_on(FixedSystem {
            free: Some(1_000),
            held_by_a_child: 0,
        });
        let named = |name: &str, seconds: &str| {
            format!("echo {name} >> {}; sleep {seconds}", log.display())
        };

        // Beside the first job (300 of 750), the second (700) does not fit
        // and the third (100) would.
        let jobs = [
            scheduler.run(sh_expecting(300, &named("first", "0.2"))),
            scheduler.run(sh_expecting(700, &named("second", "0"))),
            scheduler.run(sh_expecting(100, &named("third", "0"))),
        ];
        for job in jobs {
            job.await.unwrap();
        }

        assert_eq!(
            std::fs::read_to_string(&log).unwrap(),
            "first\nsecond\nthird\n"
        );
    }

    #[tokio::test]
    async fn without_memory_numbers_only_the_job_limit_counts() {
        let dir = tempfile::tempdir().unwrap();
        let log = dir.path().join("log");
        let scheduler = scheduler_on(FixedSystem {
            free: None,
            held_by_a_child: 0,
        });

        let jobs: Vec<_> = (0..3)
            .map(|_| scheduler.run(sh_expecting(u64::MAX / 4, &logged(&log, "0.1"))))
            .collect();
        for job in jobs {
            job.await.unwrap();
        }

        assert_eq!(most_at_once(&log), 3);
    }

    #[tokio::test]
    async fn jobs_start_in_the_order_they_were_submitted() {
        let dir = tempfile::tempdir().unwrap();
        let log = dir.path().join("log");
        let scheduler = scheduler(1);

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
        let scheduler = scheduler(1);

        let job = scheduler.run(sh(&format!(
            "echo $$ > {}; exec sleep 60",
            pid_file.display()
        )));
        let pid: u32 = read_when_written(&pid_file).await.trim().parse().unwrap();
        assert!(is_running(pid));

        drop(job);

        wait_until_gone(pid).await;
        // The slot is free again.
        assert_eq!(
            scheduler.run(sh("printf next")).await.unwrap().stdout,
            b"next"
        );
    }

    #[tokio::test]
    async fn a_waiting_job_never_starts_when_its_caller_stops_waiting() {
        let dir = tempfile::tempdir().unwrap();
        let marker = dir.path().join("started");
        let scheduler = scheduler(1);

        let first = scheduler.run(sh("sleep 0.1"));
        let abandoned = scheduler.run(sh(&format!("echo yes > {}", marker.display())));
        let last = scheduler.run(sh("printf last"));
        drop(abandoned);

        first.await.unwrap();
        assert_eq!(last.await.unwrap().stdout, b"last");
        assert!(!marker.exists());
    }

    #[tokio::test]
    async fn running_jobs_are_killed_when_the_scheduler_is_dropped() {
        let dir = tempfile::tempdir().unwrap();
        let pid_file = dir.path().join("pid");
        let scheduler = scheduler(1);

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
        let scheduler = scheduler(1);

        // `cat` returns at once on end-of-file; on an open pipe that nobody
        // writes to it waits until `timeout` ends it, with status 124.
        let finished = scheduler.run(sh("timeout 0.2 cat; echo $?")).await.unwrap();

        assert_eq!(finished.stdout, b"124\n");
    }

    #[tokio::test]
    async fn usage_tells_the_wait_from_the_run() {
        let scheduler = scheduler(1);

        let first = scheduler.run(sh("sleep 0.3"));
        let second = scheduler.run(sh("true"));
        let (first, second) = (first.await.unwrap().usage, second.await.unwrap().usage);

        let (short, long) = (Duration::from_millis(150), Duration::from_millis(300));
        assert!(first.waited < short && first.ran >= long, "{first:?}");
        assert!(second.waited >= long && second.ran < short, "{second:?}");
    }

    #[tokio::test]
    async fn a_failed_job_reports_its_usage() {
        let scheduler = scheduler(1);

        let exited = scheduler.run(sh("sleep 0.15; exit 3")).await.unwrap_err();
        let signaled = scheduler
            .run(sh("sleep 0.15; kill -KILL $$"))
            .await
            .unwrap_err();
        let unstarted = scheduler.run(unstartable()).await.unwrap_err();

        let ran = Duration::from_millis(150);
        assert!(
            exited.usage().is_some_and(|usage| usage.ran >= ran),
            "{exited:?}"
        );
        assert!(
            signaled.usage().is_some_and(|usage| usage.ran >= ran),
            "{signaled:?}"
        );
        assert_eq!(unstarted.usage(), None);
    }
}
