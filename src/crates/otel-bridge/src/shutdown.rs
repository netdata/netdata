//! The time budgets of a stop of the plugin. They are in one place because
//! each must end inside the next one.
//!
//! The agent stops the plugin with `spawn_popen_kill` (called with 3 seconds
//! in `src/plugins.d/plugins_d.c`; the waits are in
//! `src/libnetdata/spawn_server/`): it sends `QUIT`, closes the plugin's
//! pipes, waits for the plugin to leave by itself, sends SIGTERM, and sends
//! SIGKILL [`AGENT_KILL_GRACE`] later. The wait before SIGTERM depends on
//! why the plugin stops:
//!
//! - The agent itself shuts down (the usual stop): the thread that runs the
//!   plugin is already cancelled, so the wait ends at once. SIGTERM follows
//!   `QUIT`, and SIGKILL comes [`AGENT_KILL_GRACE`] after it.
//! - Only the plugin stops (it closed its output, or the agent gave up on
//!   it): the wait is up to 3 seconds.
//!
//! SIGTERM does not end the supervisor while it stops its workers: it
//! handles that signal itself, as a request to stop. So the time that a
//! stop can count on is [`AGENT_KILL_GRACE`] from `QUIT`:
//!
//! - On `QUIT` the supervisor sends `Shutdown` to every worker and waits
//!   [`WORKERS_EXIT_WAIT`] for all of them together. A worker that is still
//!   there after that is killed.
//! - A worker's work after `Shutdown` (the ingestor writes the open minutes
//!   of the service map, the ledger writes its catalog entries to local
//!   catalog files) has [`WORKER_STOP_BUDGET`]. What is not done by then is
//!   left: the next start recovers it.
//!
//! [`WORKERS_EXIT_WAIT`] may be as long as [`AGENT_KILL_GRACE`], with no
//! margin: at that moment a worker that is still there is killed either
//! way, by the supervisor, or by the end of the supervisor that the agent
//! killed (`otel-plugin/src/lifeline.rs`). What must fit with a margin is
//! [`WORKER_STOP_BUDGET`].
//!
//! Not bounded by these: after its stop work a worker drops its runtime,
//! which waits for the blocking calls that still run (an fsync). The kill
//! is the bound of that.

use std::time::Duration;

/// How long one worker may work after `Shutdown`.
pub const WORKER_STOP_BUDGET: Duration = Duration::from_secs(1);

/// How long the supervisor waits for its workers to leave after `Shutdown`.
pub const WORKERS_EXIT_WAIT: Duration = Duration::from_secs(2);

/// How long after SIGTERM the agent sends SIGKILL. The agent owns this
/// value: this is a copy of `SPAWN_KILL_DEFAULT_GRACE_MS` in
/// `src/libnetdata/spawn_server/spawn_server_internals.h`.
pub const AGENT_KILL_GRACE: Duration = Duration::from_secs(2);

const _: () = assert!(
    WORKER_STOP_BUDGET.as_millis() < WORKERS_EXIT_WAIT.as_millis()
        && WORKERS_EXIT_WAIT.as_millis() <= AGENT_KILL_GRACE.as_millis(),
    "the stop budgets must nest: worker < supervisor <= agent",
);
