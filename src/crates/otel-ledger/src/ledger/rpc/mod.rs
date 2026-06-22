//! Function-call dispatch and supervisor request handling.
//!
//! Submodules:
//!
//! - `dispatch` — the run-loop's entry points: `handle_supervisor_req`,
//!   `handle_outbound_resp`, and the per-call `dispatch_function_call`
//!   that spawns handler tasks driven by the `bridge::function` engine.
//! - `handler` — `OtelLogsHandler`, the typed `FunctionHandler` impl,
//!   its declaration, and the otel-logs–specific args→payload shim.
//! - `wire` — the netdata function wire types for `otel-logs`
//!   (request/response/envelope), and `adapter` — the mapping between
//!   those and the wire-neutral [`sfsq::logs`] engine.
//!
//! The multi-file query engine itself lives in the [`sfsq::logs`] crate
//! module; this layer adapts the netdata function protocol to it.

mod adapter;
mod dispatch;
mod handler;
mod wire;

pub(crate) use handler::{OtelLogsHandler, RemoteRead};
