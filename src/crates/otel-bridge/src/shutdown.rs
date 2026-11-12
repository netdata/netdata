//! The time budgets of a stop of the plugin. They are in one place because
//! each must end inside the next one.
//!
//! What the agent does (read in `src/plugins.d/pluginsd_parser.c`,
//! `src/plugins.d/plugins_d.c`, `src/libnetdata/spawn_server/` and
//! `src/daemon/daemon-shutdown.c`; not timed against a running agent):
//!
//! 1. Its parser of the plugin's output sends `QUIT` when the agent begins
//!    to shut down or gives up on the plugin. (When the plugin closed its
//!    output first, there is no `QUIT`: the supervisor sees its input end.)
//! 2. `spawn_popen_kill` closes the plugin's pipes and waits up to
//!    [`AGENT_EXIT_WAIT`] for the plugin to leave by itself. Then it sends
//!    SIGTERM, and SIGKILL when the plugin is still there 2 seconds later.
//! 3. When the agent itself shuts down, it cancels the thread that does
//!    these waits at a later step of its own shutdown. From that moment no
//!    wait is left: SIGTERM and SIGKILL follow at once.
//!
//! So the agent promises no time at all: [`AGENT_EXIT_WAIT`] is the most
//! that a stop can get before signals, and an agent shutdown can cut it
//! short at any moment. SIGTERM does not end the supervisor while it stops
//! its workers (it handles that signal itself); SIGKILL does, and its
//! workers then leave at once (`otel-plugin/src/lifeline.rs`).
//!
//! The plugin's own budgets are upper bounds for a worker that hangs, not
//! times that a stop needs: in by-hand runs of an idle plugin the
//! supervisor was gone in less than 10 milliseconds.
//!
//! - On `QUIT` the supervisor sends `Shutdown` to every worker and waits
//!   [`WORKERS_EXIT_WAIT`] for all of them together. A worker that is still
//!   there after that is killed.
//! - A worker's work after `Shutdown` (the ingestor takes no new export,
//!   answers the exports in progress and then writes the open minutes of
//!   the service map; the ledger writes its catalog entries to local
//!   catalog files) has [`WORKER_STOP_BUDGET`]. In by-hand runs under load
//!   the ingestor's answers took a few milliseconds; a peer that holds a
//!   connection open costs the ingestor half of the budget, the most that
//!   the answers get.
//!
//! A stop while the plugin starts, before the ingestor's `Ready`, has no
//! stop work and none of these budgets: the supervisor ends its workers at
//! once (`otel_bridge`, "Handshake protocol").
//!
//! What a stop does not finish, for any of these reasons, the next start
//! recovers: stop work is never needed for what the plugin acknowledged. An
//! export that a stop cuts gets no answer while its frames can be on the
//! disk: its sender sends it again, and those records are then stored
//! twice. Answering the exports in progress first is what keeps that rare.
//!
//! Not bounded by these: after its stop work a worker drops its runtime,
//! which waits for the blocking calls that still run (an fsync). The kill
//! is the bound of that.

use std::time::Duration;

/// How long one worker may work after `Shutdown`.
pub const WORKER_STOP_BUDGET: Duration = Duration::from_secs(1);

/// How long the supervisor waits for its workers to leave after `Shutdown`.
pub const WORKERS_EXIT_WAIT: Duration = Duration::from_secs(2);

/// The longest that the agent waits for the plugin to leave by itself,
/// before it sends signals. The agent owns this value: this is a copy of
/// the argument of `spawn_popen_kill` in `src/plugins.d/plugins_d.c`.
pub const AGENT_EXIT_WAIT: Duration = Duration::from_secs(3);

const _: () = assert!(
    WORKER_STOP_BUDGET.as_millis() < WORKERS_EXIT_WAIT.as_millis()
        && WORKERS_EXIT_WAIT.as_millis() < AGENT_EXIT_WAIT.as_millis(),
    "the stop budgets must nest: worker < supervisor < the agent's longest wait",
);
