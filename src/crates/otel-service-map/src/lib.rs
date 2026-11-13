//! Trace-derived service map for the otel-plugin: who calls whom, how often,
//! how many calls fail and how long they take, per minute.
//!
//! The ingestor hands every accepted OTLP traces export request to this crate
//! and forgets about it; everything else (pairing CLIENT and SERVER spans,
//! per-minute aggregates, their files, the `topology:tracing` Function) lives
//! here. The crate reads no WAL or SFST data. The ingestor starts it and
//! registers its Function, but its export path only calls the
//! `TraceObserver` trait, and the indexing and query code does not use it,
//! so either side can change alone.

pub mod engine;
pub mod facts;
pub mod function;
pub mod histogram;
pub mod payload;
pub mod peer;
pub mod service;
pub mod stats;
pub mod store;
pub mod view;

#[cfg(test)]
mod engine_tests;
