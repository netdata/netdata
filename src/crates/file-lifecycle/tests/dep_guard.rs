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
    // Strip comments first, then check the forbidden names against the remaining
    // text. The doc comment above this crate's `[dependencies]` deliberately
    // names all three, so we must ignore comments — but we must NOT rely on a
    // single declaration shape, because Cargo accepts several:
    //   `sfsq = ...`            (key form)
    //   `[dependencies.sfsq]`   (table-header form)
    //   `x = { package = "sfsq", path = "../sfsq" }`  (rename / path form)
    // After stripping comments, the only way any of these exact crate names
    // appears anywhere in the manifest is a real dependency edge — the allowed
    // deps (`sfst`, `otel-catalog`, …) do not contain them as substrings — so a
    // plain substring check over the comment-free text catches every form.
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
