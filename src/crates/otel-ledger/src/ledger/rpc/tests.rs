//! Tests for the two GET arg→payload shims in `mod.rs`:
//! `patch_args_into_payload` for logs and `patch_traces_args_into_payload`
//! for traces. Both are pure functions over `(args, payload)`, so there are
//! no fixtures: each test calls a shim directly (the synthesis cases parse
//! the bytes back to a `serde_json::Value`).
//!
//! Not tested here: parsing of the synthesized payloads into each signal's
//! request type.

use super::{patch_args_into_payload, patch_traces_args_into_payload};
use serde_json::{Value, json};

/// Run the logs shim on `args` and parse its synthesized bytes back to JSON.
fn patched(args: &[&str]) -> Value {
    let args: Vec<String> = args.iter().map(|s| s.to_string()).collect();
    let bytes = patch_args_into_payload(&args, None).expect("args present, payload absent");
    serde_json::from_slice(&bytes).unwrap()
}

#[test]
fn data_request_args_synthesize_window_with_info_false() {
    // No `info` token, so the synthesized flag is `false`. Unknown
    // tokens (`slice:true`) are dropped from synthesis: only
    // info/after/before reach the payload.
    assert_eq!(
        patched(&["after:100", "before:200", "slice:true"]),
        json!({"info": false, "after": 100, "before": 200})
    );
}

#[test]
fn info_token_synthesizes_info_true() {
    // The flag rides beside the parsed window: `info:true` does not
    // suppress window synthesis.
    assert_eq!(
        patched(&["info", "after:100", "before:200"]),
        json!({"info": true, "after": 100, "before": 200})
    );
}

#[test]
fn out_of_range_window_tokens_are_skipped_like_non_numeric_ones() {
    // `after:`/`before:` parse as u32, the request fields' width: an
    // out-of-range token is skipped like a non-numeric one instead of
    // synthesizing a number the deserializer would reject — one bad
    // token must not fail the whole payload (`before:200` survives).
    assert_eq!(
        patched(&["after:5000000000", "before:200"]),
        json!({"info": false, "before": 200})
    );
}

#[test]
fn no_synthesis_without_args_or_over_an_existing_payload() {
    // No args: nothing to synthesize. An existing payload (a POST
    // body) is never overwritten: the shim returns None.
    assert!(patch_args_into_payload(&[], None).is_none());
    assert!(patch_args_into_payload(&["info".to_string()], Some(b"{}")).is_none());
}

#[test]
fn traces_shim_synthesizes_only_the_strict_info_object() {
    let args = |a: &[&str]| a.iter().map(|s| s.to_string()).collect::<Vec<_>>();
    // The info token — alone or beside any other tokens — synthesizes
    // exactly the strict empty-object selector.
    for a in [
        args(&["info"]),
        args(&["info", "after:100", "before:200"]),
        args(&["after:100", "info"]),
    ] {
        let bytes = patch_traces_args_into_payload(&a, None).expect("info token present");
        let v: Value = serde_json::from_slice(&bytes).unwrap();
        assert_eq!(v, json!({"info": {}}));
    }
}

#[test]
fn traces_shim_synthesizes_nothing_for_data_gets() {
    let args = |a: &[&str]| a.iter().map(|s| s.to_string()).collect::<Vec<_>>();
    // No info token: no payload. The GET's window args are dropped.
    assert!(patch_traces_args_into_payload(&args(&[]), None).is_none());
    assert!(patch_traces_args_into_payload(&args(&["after:100", "before:200"]), None).is_none());
    // The payload guard is load-bearing: dispatch gives synthesized
    // content priority (`.or(payload)`), so an `info` URL arg must
    // never overwrite a POST body.
    assert!(patch_traces_args_into_payload(&args(&["info"]), Some(b"{}")).is_none());
}
