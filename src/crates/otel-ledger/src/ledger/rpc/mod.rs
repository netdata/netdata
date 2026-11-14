//! Function-call dispatch and supervisor request handling.
//!
//! Submodules:
//!
//! - `dispatch`: the run loop's entry points `handle_supervisor_req`,
//!   `handle_outbound_resp`, and the per-call `dispatch_function_call`.
//! - `grid`: the shared time grid behind the histogram views.
//! - `logs`: the `otel-logs` Function.
//! - `traces`: the `otel-traces` Function.
//!
//! The multi-file query engines live in the [`sfsq`] crate. This layer adapts
//! the netdata function protocol to them. This module itself holds only the
//! handler re-exports and the two GET shims.

mod dispatch;
mod grid;
mod logs;
mod traces;

/// The traces fixtures, for the ledger-level tests that need real WALs.
#[cfg(test)]
pub(crate) use traces::fixtures as traces_fixtures;

pub(crate) use logs::OtelLogsHandler;
pub(crate) use traces::OtelTracesHandler;

/// The logs pipeline's GET shim. For query-string calls it synthesizes a
/// JSON payload with an `info` flag (true if the literal `info` token is in
/// the args) and the parsed `after:`/`before:` windows. This mirrors the
/// netdata-plugin-rt GET shim (`handle_function_call` in
/// `netdata-plugin/rt/src/lib.rs`), with one difference. Tokens parse as u32,
/// the width of the request fields
/// ([`after`](logs::wire::OtelLogsRequest::after) /
/// [`before`](logs::wire::OtelLogsRequest::before)). An out-of-range token is
/// then skipped like a non-numeric one, instead of producing a number the
/// deserializer would reject. Returns `None` when there is nothing to
/// synthesize: no args, or the call already carries a payload (a POST body).
///
/// Logs only: the traces `info` is the strict `{"info": {}}` selector, not a
/// flag, so traces use [`patch_traces_args_into_payload`].
pub(crate) fn patch_args_into_payload(args: &[String], payload: Option<&[u8]>) -> Option<Vec<u8>> {
    if args.is_empty() || payload.is_some() {
        return None;
    }

    let info = args.iter().any(|a| a == "info");
    let mut map = serde_json::Map::new();
    map.insert("info".into(), serde_json::json!(info));

    for arg in args {
        // u32: see the fn doc.
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

/// The traces pipeline's GET shim. The literal `info` token synthesizes the
/// strict `{"info": {}}` selector, regardless of any other tokens. Anything
/// else synthesizes nothing, so a GET data call loses its window args. The
/// `payload.is_some()` guard keeps an `info` URL arg from overwriting a POST
/// body, because dispatch prefers a synthesized payload.
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
