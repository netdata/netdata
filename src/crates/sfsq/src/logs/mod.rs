//! Multi-source log-query subsystem.
//!
//! Turns a set of time-overlapping sources plus a [`LogsQuery`] into a
//! single [`LogsData`] — filter → facets / histogram → pagination → row
//! materialization. [`run`] is the all-in-one entry point for the local
//! case.
//!
//! Two kinds of source feed the *same* query, so there is one query
//! semantics rather than two:
//! - [`SfstCandidate`] — a sealed on-disk SFST, or an in-memory SFST
//!   built from a chunk of an active WAL's durable prefix; evaluated
//!   through the indexed SFST engine.
//! - [`WalTail`] — an active WAL's most-recent records, not yet in any
//!   SFST; evaluated by a bounded row scan ([`WalScan`]) instead of an
//!   index.
//!
//! All sources interleave under one cursor order
//! `(timestamp_ns, file_seq, part, position)`, so the statistics and
//! the row table reflect every source as if it were a single index.
//!
//! The work splits into two steps. Step 1 (statistics — matched, facets,
//! histogram, fields) is an aggregatable monoid: [`LogsShard::evaluate`]
//! produces a [`LogsShard`] per source and [`LogsShard::merge`] folds
//! them, so the query can fan out across nodes and aggregate. Step 2 (row
//! materialization) needs a global order and lives in the pagination
//! path. [`run`] composes both.
//!
//! The API is neutral — plain Rust data in ([`LogsQuery`]), plain Rust
//! data out ([`LogsData`], built from `sfst` types); no transport or wire
//! concerns.
//!
//! This subsystem is the query *mechanism*: it evaluates the sources it
//! is handed. Which bytes become a sealed SFST, an in-memory chunk, or a
//! tail — and why the durable prefix is indexed while the tail is scanned
//! — is *policy* resolved by the caller (the ledger). The query is pure
//! and synchronous; opening and decompressing sources is its only I/O,
//! which the caller schedules off any async runtime thread.

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
pub use wal_scan::{FlattenedScanError, WalScan, WalScanError};
