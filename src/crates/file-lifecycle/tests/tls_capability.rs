//! Guards the HTTPS capability of the shared reqwest build.
//!
//! opendal is declared `default-features = false` in the workspace manifest
//! (`src/crates/Cargo.toml`), and this crate's own reqwest dev-dependency
//! enables no TLS feature, so the only TLS backend reqwest can pick up here is
//! opendal's `reqwest-rustls-tls`, via cargo feature unification. If a
//! manifest edit drops that feature, reqwest still compiles and keeps serving
//! plain HTTP, but rejects every `https://` URL in-process with a scheme
//! error — S3/STS become unreachable with no network-level hint of why. This
//! test fails loudly in that case instead.
//!
//! The manifests' own comments point back here: the workspace `opendal`
//! entry and this crate's `[dev-dependencies]` reqwest entry.

use std::time::Duration;

#[tokio::test]
async fn https_requests_reach_the_network_instead_of_failing_in_process() {
    // RFC 5737 TEST-NET-1 address: never routable, so with a TLS backend the
    // request fails at the network layer (connect timeout). Without one it
    // fails instantly, in-process, with a URL-scheme error.
    let client = reqwest::Client::builder()
        .connect_timeout(Duration::from_millis(300))
        .build()
        .expect("client builds without TLS-specific options");
    let err = client
        .get("https://192.0.2.1/")
        .send()
        .await
        .expect_err("unroutable address must not succeed");
    // The guard keys on the TLS-less rejection wording ("invalid URL, scheme
    // is not http"), raised by the plain HTTP connector reqwest falls back to
    // without a TLS backend (hyper-util's `enforce_http`), so it only shows up
    // in the error chain that `{err:?}` — not Display — renders. If a
    // reqwest/hyper-util upgrade rephrases it, this assertion passes
    // vacuously — re-check the wording then.
    let rendered = format!("{err:?}");
    assert!(
        !rendered.contains("scheme"),
        "reqwest resolved without a TLS backend (https rejected in-process): {rendered}"
    );
}
