//! Query engine over SFST indexes — the read side of the OTLP log/trace
//! storage stack (`ng-flatten` writes the flattened WAL, `ng-index`
//! seals SFSTs from it, this crate answers the queries). This crate is
//! the query *mechanism*: it evaluates whatever sources it is handed —
//! sealed SFST files, in-memory SFSTs built from active-WAL chunks, and
//! WAL tails — and owns no storage; which bytes take which form is
//! *policy*, resolved by the caller (`otel-ledger`'s ledger).
//!
//! Both engines are wire-neutral: plain Rust data in, plain Rust data
//! out; each consumer maps its own request/response format onto them.
//! Consumers: `otel-ledger` (the RPC layer) and `sfsq-cli`. The traces
//! contracts are pinned by the integration suites under `tests/`; the
//! design authority is the phase-4 design record in the traces plan
//! repo.
//!
//! # The two engines
//!
//! [`logs`] — multi-file log queries. A set of time-overlapping sources
//! ([`logs::LogSource`]: a sealed SFST or an in-memory WAL chunk through
//! the indexed engine, or an un-indexed WAL tail row-scanned by
//! [`logs::WalScan`]) plus a [`logs::LogsQuery`] produce one
//! [`logs::LogsData`] — matched counts, facets, histogram, and a
//! paginated page of materialized rows. All sources interleave under
//! one global cursor order; step-1 statistics are a mergeable monoid
//! ([`logs::LogsShard`]), so a query can fan out and aggregate.
//! [`logs::run`] is the all-in-one local entry point.
//!
//! [`traces`] — trace queries over the same source kinds, with five
//! operation surfaces: [`traces::trace_by_id`] (cross-source assembly
//! through one shared combiner, `sfst::trace_combine`),
//! [`traces::search()`], [`traces::overview()`], [`traces::slowest()`],
//! and attribute enumeration. Unlike logs there is no silent
//! degradation: a source that fails is an explicit partial
//! ([`traces::QueryStatus`]), and source identity is validated up
//! front — duplicate ids and overlapping WAL ranges are rejected.
//!
//! # Module map
//!
//! - `source` (private) — signal-neutral plumbing shared by both
//!   engines: [`Source`] (bytes provenance: sealed file or in-memory
//!   chunk image) and its mapping to shared bytes. Also re-exported as
//!   `logs::Source`, a path kept valid for existing consumers.
//! - `logs/` — the two steps of [`logs::run`]: step 1, per-source
//!   statistics (`aggregate`, folded cross-source by `merge`); step 2,
//!   row materialization (`page`) under the `cursor` total order.
//!   `query`/`result` are the neutral I/O types; `wal_scan` is the
//!   tail's row-scan evaluator; `mmap` maps sources (log-and-degrade).
//! - `traces/` — `sources` (validated source identity), `predicate`
//!   (the shared predicate AST), `status` (the partial-result
//!   contract), and one module per operation (`by_id`, `search` with
//!   its `gate` pre-assembly filter, `overview`, `slowest`,
//!   `attributes`); `fold` merges cross-source aggregates over
//!   `rollup`'s per-trace shapes; `vocab`/`window`/`wal_scan` are
//!   shared support.
//!
//! Each subsystem module's own docs are the authority for its pieces;
//! their `pub use`s are the crate's public surface.

pub mod logs;
pub mod traces;

mod source;

pub use source::Source;
