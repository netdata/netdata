//! Trace-derived service map for the otel-plugin: who calls whom, how often,
//! how many calls fail and how long they take, per minute.
//!
//! The crate receives every accepted OTLP traces export request and does the
//! rest itself: pairing CLIENT and SERVER spans, per-minute aggregates, their
//! files, and the `topology:tracing` Function. It reads no WAL or SFST data.

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
