//! Multi-source log-query engine: a caller-preselected set of
//! time-overlapping sources plus a [`LogsQuery`] becomes one
//! [`LogsData`] — filter → facets / histogram → pagination → row
//! materialization. [`run`] (engine.rs) is the all-in-one local entry
//! point.
//!
//! One query semantics for two source kinds, handed over as one mixed
//! list ([`LogSource`]) the engine never pre-buckets:
//! - [`SfstCandidate`] — an indexed source: a sealed on-disk SFST, or
//!   an in-memory SFST built from a chunk of an active WAL's durable
//!   prefix; evaluated through the SFST engine.
//! - [`WalTail`] — an active WAL's most-recent records, not yet in any
//!   SFST; evaluated by a bounded row scan ([`WalScan`]) instead of an
//!   index.
//! All sources interleave under one cursor order `(timestamp_ns,
//! file_seq, part, position)` ([`Cursor`]), so the statistics and the
//! row table reflect every source as if it were a single index.
//!
//! `run` composes two steps: step 1, statistics (matched, facets,
//! histogram, fields), is an aggregatable monoid —
//! [`LogsShard::evaluate`] produces a [`LogsShard`] per source,
//! [`LogsShard::merge`] folds them, so the query can fan out across
//! nodes; step 2, row materialization, needs that global order and
//! lives in the pagination path.
//!
//! This is the query *mechanism*: it evaluates the sources it is
//! handed; which bytes become a sealed SFST, an in-memory chunk, or a
//! tail — and why the durable prefix is indexed while the tail is
//! scanned — is *policy*, resolved by the caller (the ledger). Pure and
//! synchronous: opening and decompressing sources is its only I/O,
//! which the caller schedules off any async runtime thread; plain Rust
//! data in, plain Rust data out.
//!
//! # Module map
//!
//! - aggregate / merge — step 1: per-source evaluation and the pure
//!   cross-file folds.
//! - page — step 2: per-source page candidates ([`PageShard`]), merged
//!   and materialized; `paginate` is the local orchestration.
//! - cursor — [`Cursor`] / [`Part`]: the order above, encoded as the
//!   opaque pagination string.
//! - query / result — the neutral input ([`Anchor`], [`Direction`],
//!   [`LogsQueryBuilder`]) and output ([`LogsData`]) types.
//! - engine — [`run`] and the source model (the kinds above, plus
//!   [`Source`], the bytes provenance: sealed file or in-memory chunk
//!   image).
//! - mmap — log-and-degrade source mapping shared by both passes; WAL
//!   tails are never mapped.
//! - wal_scan — the tail's row-scan evaluator, contract-equal to the
//!   SFST engine.

mod aggregate;
mod cursor;
mod engine;
mod merge;
mod mmap;
mod page;
mod query;
mod result;
mod wal_scan;

pub use aggregate::LogsShard;
pub use cursor::{Cursor, Part};
pub use engine::{LogSource, SfstCandidate, Source, WalTail, run};
pub use page::PageShard;
pub use query::{Anchor, Direction, LogsQuery, LogsQueryBuilder};
pub use result::LogsData;
pub use wal_scan::{FlattenedScanError, WalScan};
