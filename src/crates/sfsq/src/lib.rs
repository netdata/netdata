//! Query layer over SFST log indexes.
//!
//! [`logs`] is the multi-file log-query engine: given a set of
//! overlapping SFST files and a [`logs::LogsQuery`], it produces a single
//! [`logs::LogsData`] — facets, a histogram, and a paginated,
//! materialized page of log rows. The API is wire-neutral (plain Rust in
//! and out); a consumer maps its own request/response format onto it.
//! See [`logs::run`] for the entry point.

pub mod logs;
