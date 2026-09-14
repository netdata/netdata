use super::{patch_args_into_payload, patch_traces_args_into_payload};
use serde_json::{Value, json};

// The shims synthesize raw JSON bytes; each signal's wire tests cover
// parsing that JSON into its own request type. Here the synthesized
// objects themselves are pinned. `patch_args_into_payload` serves the
// LOGS pipeline only; traces installs its own strict-shape shim.

fn patched(args: &[&str]) -> Value {
    let args: Vec<String> = args.iter().map(|s| s.to_string()).collect();
    let bytes = patch_args_into_payload(&args, None).expect("args present, payload absent");
    serde_json::from_slice(&bytes).unwrap()
}

#[test]
fn data_request_args_synthesize_window_with_info_false() {
    // No "info" token — a data request. `info` must be false so the
    // handler runs the data path, not capability discovery; unknown
    // tokens are ignored.
    assert_eq!(
        patched(&["after:100", "before:200", "slice:true"]),
        json!({"info": false, "after": 100, "before": 200})
    );
}

#[test]
fn info_token_synthesizes_info_true() {
    assert_eq!(
        patched(&["info", "after:100", "before:200"]),
        json!({"info": true, "after": 100, "before": 200})
    );
}

#[test]
fn out_of_range_window_tokens_are_skipped_like_non_numeric_ones() {
    // The request fields are u32: a wider parse would synthesize a
    // number the deserializer rejects, failing the whole payload for
    // one bad token.
    assert_eq!(
        patched(&["after:5000000000", "before:200"]),
        json!({"info": false, "before": 200})
    );
}

#[test]
fn no_synthesis_without_args_or_over_an_existing_payload() {
    // No args: nothing to synthesize. Existing payload (a POST body,
    // or the upstream rt shim's output): must pass through untouched.
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
    // No info token: no payload — the request surfaces as the bridge's
    // absent-payload client error, never a silent default query.
    assert!(patch_traces_args_into_payload(&args(&[]), None).is_none());
    assert!(patch_traces_args_into_payload(&args(&["after:100", "before:200"]), None).is_none());
    // The payload guard is load-bearing: dispatch gives synthesized
    // content priority, so an `info` URL arg must never overwrite a
    // POST body.
    assert!(patch_traces_args_into_payload(&args(&["info"]), Some(b"{}")).is_none());
}
