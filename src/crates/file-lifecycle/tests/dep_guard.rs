//! Hard dependency guard: the manifest of `file-lifecycle` — the
//! content-agnostic file substrate (the `Cargo.toml` header comment says
//! so), reused by both signals through `otel-ledger` — must never declare a
//! log-content crate, so the traces signal never compiles the logs
//! ones. Cargo already makes importing an undeclared crate impossible;
//! this test backstops the declaration side: the whole manifest, read
//! comment-free, must not mention a forbidden crate. A future edit that
//! adds one of these fails here instead of silently re-coupling the
//! substrate.

/// The log-content crates: `sfsq` (the logs/traces query engines over
/// SFST; see the `sfsq/src/lib.rs` crate docs) and `otel-logs-identity`
/// (OTel logs content-plane identity; see the
/// `otel-logs-identity/src/lib.rs` crate docs). The
/// neutral crates `sfst` and `otel-catalog` stay allowed; they are not
/// listed here.
const FORBIDDEN: &[&str] = &["sfsq", "otel-logs-identity"];

#[test]
fn manifest_declares_no_content_crate() {
    // Truncate each line at the first `#`, then substring-match the
    // forbidden names against what is left. Stripping is required: the
    // manifest's own header comment deliberately names both crates.
    // After stripping, a plain substring
    // check sees every declaration shape — dependency key,
    // `[dependencies.<name>]` table header, `package = "…"` rename —
    // across the whole manifest (not just the dependency tables), and
    // is safe because no allowed dependency name contains a forbidden
    // one as a substring.
    let manifest = include_str!("../Cargo.toml");
    let code: String = manifest
        .lines()
        .map(|line| line.split('#').next().unwrap_or(""))
        .collect::<Vec<_>>()
        .join("\n");

    for name in FORBIDDEN {
        assert!(
            !code.contains(name),
            "file-lifecycle must stay content-agnostic, but its Cargo.toml references \
             the log-content crate `{name}` outside a comment. Remove it: the substrate \
             is reused by other signals (traces) that must not compile logs."
        );
    }
}
