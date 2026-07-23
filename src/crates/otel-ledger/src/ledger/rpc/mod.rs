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
mod grid;
mod logs;
mod traces;

pub(crate) use logs::{OtelLogsHandler, RemoteRead};
pub(crate) use traces::OtelTracesHandler;

/// Replicate the rt-level GET shim (`netdata-plugin/rt/src/lib.rs`):
/// when args carry `after:N` / `before:N` tokens, synthesize a JSON
/// payload with the parsed window plus an `info` flag determined by
/// whether the literal `info` token is in the args. Returns `None`
/// when no synthesis happened (no args, or the upstream rt shim
/// already produced a payload), in which case the caller falls back
/// to the original payload.
///
/// Signal-agnostic — `info`/`after`/`before` exist on both signals'
/// request types — and used as the ArgShim by BOTH the logs and traces
/// pipelines; a change here changes every otel Function's GET behavior.
pub(crate) fn patch_args_into_payload(args: &[String], payload: Option<&[u8]>) -> Option<Vec<u8>> {
    if args.is_empty() || payload.is_some() {
        return None;
    }

    let info = args.iter().any(|a| a == "info");
    let mut map = serde_json::Map::new();
    map.insert("info".into(), serde_json::json!(info));

    for arg in args {
        if let Some(rest) = arg.strip_prefix("after:") {
            if let Ok(v) = rest.parse::<u64>() {
                map.insert("after".into(), serde_json::json!(v));
            }
        } else if let Some(rest) = arg.strip_prefix("before:") {
            if let Ok(v) = rest.parse::<u64>() {
                map.insert("before".into(), serde_json::json!(v));
            }
        }
    }

    serde_json::to_vec(&serde_json::Value::Object(map)).ok()
}

#[cfg(test)]
mod tests;
