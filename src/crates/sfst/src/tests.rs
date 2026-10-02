//! Test-module root for the sfst crate — the `#[cfg(test)] mod tests;`
//! target in `lib.rs`, so everything under here is test-only. Each
//! child module is one test group; see its own header for the exact
//! contracts it pins.
//!
//! - `fixture` — shared buffer-all test scaffolding (`FixtureWriter`),
//!   holding no tests itself. Deliberately `pub`: sibling test modules
//!   outside this tree import it (`registry/tests.rs` —
//!   `crate::tests::fixture::FixtureWriter`), which a private `mod`
//!   could not serve.
//! - `materialize` — materialized-row label correctness
//!   (tests/materialize.rs).
//! - `query` — the `IndexReader` query API: `matched_count`,
//!   `matched_positions`, `facets`, and `timeline` (tests/query.rs).
//! - `round_trip` — format round-trip: write files with the looser
//!   `FixtureWriter`/`pack`, read them back with `ChunkReader`, and pin
//!   what decodes (tests/round_trip.rs).
//! - `trace_plan` — per-file trace-search plan evaluation: tier matrix,
//!   conjunction, duration bounds, work counting, and the
//!   rank-bounded-extraction proof (tests/trace_plan.rs).
pub mod fixture;
mod materialize;
mod query;
mod round_trip;
mod trace_plan;
