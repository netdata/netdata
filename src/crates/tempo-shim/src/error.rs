//! Parse-boundary errors. Every variant is a request defect (HTTP 400);
//! the messages are the operator-visible explanation (Grafana surfaces
//! only the status line on `/api/search`, so the body text serves curl
//! and logs — keep it self-contained).

/// A `q` filter-string parse/translation failure.
#[derive(Debug, thiserror::Error, PartialEq, Eq)]
pub enum ParseError {
    /// Valid-looking TraceQL outside the form-generated subset the shim
    /// implements (pipelines, structural ops, aggregates, unsupported
    /// scopes/quoting) — deliberate 400, plan decisions D2/D3.
    #[error(
        "unsupported TraceQL at byte {pos}: {what}; only the Grafana \
         search-form filter grammar is supported"
    )]
    Unsupported { pos: usize, what: String },

    /// Not a well-formed query at all.
    #[error("malformed query at byte {pos}: {what}")]
    Malformed { pos: usize, what: String },

    /// A `kind`/`status` value outside the closed keyword set.
    #[error("unknown {field} value {value:?} at byte {pos}; expected one of: {allowed}")]
    UnknownKeyword {
        pos: usize,
        field: &'static str,
        value: String,
        allowed: &'static str,
    },

    /// A duration comparison whose literal does not parse.
    #[error("invalid duration {lit:?} at byte {pos}: {why}")]
    BadDuration { pos: usize, lit: String, why: String },
}
