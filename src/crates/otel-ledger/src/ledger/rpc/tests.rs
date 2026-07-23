use super::patch_args_into_payload;
use serde_json::{Value, json};

// The shim synthesizes raw JSON bytes; each signal's wire tests cover
// parsing that JSON into its own request type. Here the synthesized
// object itself is pinned.

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
fn no_synthesis_without_args_or_over_an_existing_payload() {
    // No args: nothing to synthesize. Existing payload (a POST body,
    // or the upstream rt shim's output): must pass through untouched.
    assert!(patch_args_into_payload(&[], None).is_none());
    assert!(patch_args_into_payload(&["info".to_string()], Some(b"{}")).is_none());
}
