//! Function-call dispatch and supervisor request handling.
//!
//! Submodules:
//!
//! - `dispatch` — the run-loop's entry points: `handle_supervisor_req`,
//!   `handle_outbound_resp`, and the per-call `dispatch_function_call`
//!   that spawns handler tasks driven by the `bridge::function` engine.
//!   Signal-neutral: it routes by each pipeline's declared function name.
//! - `grid` — the shared time-grid derivation behind the otel
//!   Functions' histogram views: picks a "nice" bucket width and snaps
//!   the window outward before handing the engine an exact
//!   [`sfst::Grid`]. Consumed by the logs histogram and the traces
//!   overview; a change here re-grids every otel Function view.
//! - `logs` — the `otel-logs` Function: wire types, engine adapter, and
//!   the `OtelLogsHandler` glue over the wire-neutral [`sfsq::logs`]
//!   engine.
//! - `traces` — the `otel-traces` Function: wire types, source
//!   resolution, and the `OtelTracesHandler` glue over the wire-neutral
//!   [`sfsq::traces`] engine.
//!
//! The multi-file query engines themselves live in the [`sfsq`] crate;
//! this layer adapts the netdata function protocol to them. The module
//! itself holds only glue: the handler re-exports below — each signal's
//! pipeline builder takes its handler through them — and the two GET
//! shims, installed one per pipeline as its `arg_shim`
//! ([`build_pipeline`](super::pipeline::build_pipeline),
//! [`build_traces_pipeline`](super::traces_pipeline::build_traces_pipeline)).

mod dispatch;
mod grid;
mod logs;
mod traces;

pub(crate) use logs::OtelLogsHandler;
pub(crate) use traces::OtelTracesHandler;

/// The logs pipeline's GET shim (its `arg_shim`, installed by
/// [`build_pipeline`](super::pipeline::build_pipeline)): replicate the
/// rt-level GET shim (`netdata-plugin/rt/src/lib.rs` `handle_function_call`)
/// for query-string calls — synthesize a JSON payload with an `info`
/// flag (true iff the literal token is in the args) plus parsed
/// `after:`/`before:` windows. Divergence: tokens parse as u32, the
/// request fields' width ([`after`](logs::wire::OtelLogsRequest::after) /
/// [`before`](logs::wire::OtelLogsRequest::before)), so an out-of-range
/// token skips like a non-numeric one instead of synthesizing a number
/// the deserializer would reject (the rt shim parses u64). Returns
/// `None` when there is nothing to synthesize — no args, or the call
/// already carries a payload (a POST body) — and the caller
/// (`rpc/dispatch.rs` `dispatch_function_call`) keeps the original.
///
/// LOGS-ONLY: the logs request has top-level `info`/`after`/`before`
/// fields, so this shape parses there. The strict traces request (one
/// mode object, no top-level window) takes its own shim,
/// [`patch_traces_args_into_payload`] — a change here affects only the
/// logs Function's GET behavior.
pub(crate) fn patch_args_into_payload(args: &[String], payload: Option<&[u8]>) -> Option<Vec<u8>> {
    if args.is_empty() || payload.is_some() {
        return None;
    }

    let info = args.iter().any(|a| a == "info");
    let mut map = serde_json::Map::new();
    map.insert("info".into(), serde_json::json!(info));

    for arg in args {
        // u32: see the fn doc — an out-of-range token must skip, not
        // synthesize a value the deserializer rejects.
        if let Some(rest) = arg.strip_prefix("after:") {
            if let Ok(v) = rest.parse::<u32>() {
                map.insert("after".into(), serde_json::json!(v));
            }
        } else if let Some(rest) = arg.strip_prefix("before:") {
            if let Ok(v) = rest.parse::<u32>() {
                map.insert("before".into(), serde_json::json!(v));
            }
        }
    }

    serde_json::to_vec(&serde_json::Value::Object(map)).ok()
}

/// The traces pipeline's GET shim, installed by
/// [`build_traces_pipeline`](super::traces_pipeline::build_traces_pipeline).
/// PRESENCE of the literal `info` token synthesizes the strict `{"info": {}}`
/// selector regardless of any other tokens; anything else synthesizes
/// NOTHING — a traces GET data call has no body to express a mode, so
/// it surfaces as the bridge's absent-payload client error rather than
/// a silent default query (`netdata-plugin/bridge/src/function.rs`
/// `HandlerAdapter::handle_raw`). Data calls are
/// POST-only by design. The `payload.is_some()` guard is load-bearing:
/// dispatch gives synthesized content priority (`rpc/dispatch.rs`
/// `dispatch_function_call`), so
/// without it an `info` URL arg would overwrite a POST body.
pub(crate) fn patch_traces_args_into_payload(
    args: &[String],
    payload: Option<&[u8]>,
) -> Option<Vec<u8>> {
    if payload.is_some() || !args.iter().any(|a| a == "info") {
        return None;
    }
    Some(br#"{"info": {}}"#.to_vec())
}

#[cfg(test)]
mod tests;
