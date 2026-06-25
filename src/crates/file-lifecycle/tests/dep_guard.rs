//! Hard dependency guard.
//!
//! `file-lifecycle` is the content-agnostic substrate: it MUST NOT depend on any
//! log-content crate, so a second signal (traces) can reuse it without pulling
//! logs in. Cargo already makes importing an undeclared crate impossible; this
//! test backstops that by asserting the manifest never *declares* one — in
//! `[dependencies]` OR `[dev-dependencies]`. If a future edit adds one of these,
//! this test fails loudly instead of silently re-coupling the substrate.

/// The three log-content crates. The substrate may depend on the neutral
/// container/catalog crates (`sfst`, `otel-catalog`) — proven content-agnostic
/// in earlier stages — but never on these.
const FORBIDDEN: &[&str] = &["sfsq", "sfst-indexer", "otel-logs-identity"];

#[test]
fn manifest_declares_no_content_crate() {
    let manifest = include_str!("../Cargo.toml");
    for name in FORBIDDEN {
        // A Cargo dependency is declared as a table key at the start of a line:
        // `name = ...` or `name.workspace = ...`. Match exactly that shape so a
        // comment mentioning the crate, or a different crate sharing a prefix
        // (e.g. `sfst` vs `sfst-indexer`), never false-triggers.
        let declared = manifest.lines().any(|line| {
            line.trim_start().strip_prefix(name).is_some_and(|rest| {
                let rest = rest.trim_start();
                rest.starts_with('=') || rest.starts_with('.')
            })
        });
        assert!(
            !declared,
            "file-lifecycle must stay content-agnostic, but its Cargo.toml declares \
             the log-content crate `{name}`. Remove it: the substrate is reused by \
             other signals (traces) that must not compile logs."
        );
    }
}
