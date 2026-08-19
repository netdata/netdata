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
/// whether the literal `info` token is in the args. One deliberate
/// divergence: tokens parse as u32 (the request fields' width), so an
/// out-of-range token skips instead of synthesizing a number the
/// deserializer rejects — the rt shim still parses u64. Returns `None`
/// when no synthesis happened (no args, or the upstream rt shim
/// already produced a payload), in which case the caller falls back
/// to the original payload.
///
/// LOGS-ONLY: the logs request still carries top-level
/// `info`/`after`/`before`, so this shim's synthesized shape parses
/// there. The traces request is strict (one mode object, no top-level
/// window), so the traces pipeline installs its own
/// [`patch_traces_args_into_payload`] instead — a change here changes
/// only the logs Function's GET behavior.
pub(crate) fn patch_args_into_payload(args: &[String], payload: Option<&[u8]>) -> Option<Vec<u8>> {
    if args.is_empty() || payload.is_some() {
        return None;
    }

    let info = args.iter().any(|a| a == "info");
    let mut map = serde_json::Map::new();
    map.insert("info".into(), serde_json::json!(info));

    for arg in args {
        // u32, matching the request fields: an out-of-range token is
        // SKIPPED like a non-numeric one — parsing wider would synthesize
        // a number the deserializer rejects, failing the whole payload
        // for one bad token.
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

/// The traces pipeline's GET shim. PRESENCE of the literal `info`
/// token synthesizes the strict `{"info": {}}` selector regardless of
/// any other tokens; anything else synthesizes NOTHING — a traces GET
/// data call has no body to express a mode, so it surfaces as the
/// bridge's absent-payload client error rather than a silent default
/// query. Data calls are POST-only by design. The `payload.is_some()`
/// guard is load-bearing: dispatch gives synthesized content priority,
/// so without it an `info` URL arg would overwrite a POST body.
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
