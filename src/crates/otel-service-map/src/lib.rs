//! Trace-derived service map for the otel-plugin: who calls whom, how often,
//! how many calls fail and how long they take, per minute.
//!
//! The ingestor hands every accepted OTLP traces export request to this crate
//! and forgets about it; everything else (pairing CLIENT and SERVER spans,
//! per-minute aggregates, their files, the `topology:tracing` Function) lives
//! here. The crate reads no WAL or SFST data and the ingestion, indexing and
//! query code knows nothing about it, so either side can change alone.

pub mod engine;
pub mod facts;
pub mod histogram;
pub mod peer;
pub mod stats;

#[cfg(test)]
mod engine_tests;
