//! Function-call dispatch and supervisor request handling.
//!
//! Submodules:
//!
//! - `dispatch` — the run-loop's entry points: `handle_supervisor_req`,
//!   `handle_outbound_resp`, and the per-call `dispatch_function_call`
//!   that spawns handler tasks driven by the `bridge::function` engine.
//!   Signal-neutral: it routes by each pipeline's declared function name.
//! - `logs` — the `otel-logs` Function: wire types, engine adapter, and
//!   the `OtelLogsHandler` glue over the wire-neutral [`sfsq::logs`]
//!   engine.
//! - `traces` — the `otel-traces` Function: wire types, source
//!   resolution, and the `OtelTracesHandler` glue over the wire-neutral
//!   [`sfsq::traces`] engine.
//!
//! The multi-file query engines themselves live in the [`sfsq`] crate;
//! this layer adapts the netdata function protocol to them.

mod dispatch;
mod logs;
mod traces;

pub(crate) use logs::{OtelLogsHandler, RemoteRead, patch_args_into_payload};
pub(crate) use traces::OtelTracesHandler;
