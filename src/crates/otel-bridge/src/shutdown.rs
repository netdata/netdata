//! The time budgets of a stop of the plugin. They are in one place because
//! each must end inside the next one.
//!
//! The agent stops the plugin in this order (`spawn_popen_kill` with 3
//! seconds in `src/plugins.d/plugins_d.c`; the waits are in
//! `src/libnetdata/spawn_server/`):
//!
//! 1. It sends `QUIT` and closes the plugin's pipes.
//! 2. It waits [`AGENT_EXIT_GRACE`] for the plugin to leave by itself.
//! 3. It sends SIGTERM, and 2 seconds later SIGKILL.
//!
//! The supervisor must be gone inside step 2: a supervisor that the agent
//! kills cannot stop its workers, and they leave at once when its pipe
//! closes (`otel-plugin/src/lifeline.rs`), with their stop work not done.
//! So:
//!
//! - On `QUIT` the supervisor sends `Shutdown` to every worker and waits
//!   [`WORKERS_EXIT_WAIT`] for all of them together. A worker that is still
//!   there after that is killed.
//! - A worker's work after `Shutdown` (the ingestor writes the open minutes
//!   of the service map, the ledger writes its catalog entries to local
//!   catalog files) has [`WORKER_STOP_BUDGET`]. What is not done by then is
//!   left: the next start recovers it.
//!
//! Not bounded by these: after its stop work a worker drops its runtime,
//! which waits for the blocking calls that still run (an fsync). The
//! supervisor's kill is the bound of that.

use std::time::Duration;

/// How long one worker may work after `Shutdown`.
pub const WORKER_STOP_BUDGET: Duration = Duration::from_secs(1);

/// How long the supervisor waits for its workers to leave after `Shutdown`.
pub const WORKERS_EXIT_WAIT: Duration = Duration::from_secs(2);

/// How long the agent waits for the plugin to leave by itself, before it
/// sends signals. The agent owns this value: this is a copy of the argument
/// of `spawn_popen_kill` in `src/plugins.d/plugins_d.c`.
pub const AGENT_EXIT_GRACE: Duration = Duration::from_secs(3);

const _: () = assert!(
    WORKER_STOP_BUDGET.as_millis() < WORKERS_EXIT_WAIT.as_millis()
        && WORKERS_EXIT_WAIT.as_millis() < AGENT_EXIT_GRACE.as_millis(),
    "the stop budgets must nest: worker < supervisor < agent",
);
