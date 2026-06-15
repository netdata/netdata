//! OTLP log-record normalization helpers.
//!
//! [`normalize_body`] detects JSON-object strings in log bodies and
//! converts them to structured `KvlistValue`s so the downstream
//! flattener (`otel-ingestor`'s `arrow_bridge`) indexes their fields.
//!
//! Formerly named `otel-flatten`, after a record flattener it carried;
//! that flattener had no consumers and different array semantics than
//! the live one in `arrow_bridge`, so it was removed and the crate
//! renamed to what it actually does.

mod normalize;

pub use normalize::normalize_body;
