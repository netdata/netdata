//! Grafana **Tempo HTTP API shim** over [`sfsq::traces`] — TEMPORARY
//! SCAFFOLDING to unblock UI work (SOW-20260712-traces-tempo-shim): it
//! will be dropped once the product's own trace operations are defined,
//! so everything Tempo-specific lives in this one crate.
//!
//! Per the plan's decision 16A the query engine is wire-neutral: ALL
//! TraceQL strings — the form-filter grammar, the intrinsic name table,
//! the kind/status keyword↔storage-label maps — live here, next to the
//! wire adapters. The engine consumes only the typed
//! [`sfsq::traces::Predicate`] / request structures.
//!
//! Scope (plan decision D3): the shim parses the **form-generated filter
//! grammar** the Grafana Tempo datasource plugin emits (verified against
//! plugin v13.1.5, `language_provider.ts` / `SearchTraceQLEditor/
//! utils.ts`, plus the bare ad-hoc-filter forms) — NOT TraceQL the
//! language. Pipelines, structural operators, aggregates, and anything
//! else reachable only through the raw TraceQL editor fail with a clear
//! [`ParseError`] the HTTP layer maps to 400.

mod duration;
mod error;
mod keywords;
mod parse;
mod request;

pub use duration::parse_duration_ns;
pub use error::ParseError;
pub use parse::parse_query;
pub use request::{RequestError, parse_trace_id_hex, window_from_unix_seconds};
