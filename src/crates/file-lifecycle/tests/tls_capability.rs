//! Guards the HTTPS capability of the shared reqwest build.
//!
//! opendal is declared with `default-features = false`, which once silently
//! dropped its `reqwest-rustls-tls` feature — reqwest then resolved with no
//! TLS backend and rejected every `https://` URL in-process ("scheme is not
//! http"), making S3/STS unreachable in production while all plain-HTTP dev
//! setups kept working.
//!
//! This crate's own reqwest dev-dependency enables NO TLS feature, so the
//! capability can only arrive via cargo feature unification from opendal's
//! `reqwest-rustls-tls`. If that feature is ever dropped again, this test
//! fails with the scheme error instead of a network-level error.

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
    let rendered = format!("{err:?}");
    assert!(
        !rendered.contains("scheme"),
        "reqwest resolved without a TLS backend (https rejected in-process): {rendered}"
    );
}
