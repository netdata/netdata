//! WAL → SFST indexing pipeline.
//!
//! The build side of the [`sfst`] format: everything that turns interned
//! `(timestamp, key=value)` log rows into an on-disk (or in-memory) SFST
//! index lives here, so the format crate stays free of the build stack and
//! format-only consumers never compile it.
//!
//! A producer fills a [`RowIndex`](row_index::RowIndex) — interning
//! `key=value` attributes and accumulating the string interner + per-attribute
//! bitmaps + per-log entries + per-log timestamps — then [`build_and_write`]
//! (to a file) or [`build_into`] (to an in-memory sink) consumes it and emits
//! the SFST through [`sfst`]'s public format vocabulary (chunk ids, magic,
//! version, `pack`). Decoding WAL frames into those rows is the producer's
//! concern; the `ng-index` crate is the in-tree producer (flattened-frame
//! reader feeding both the seal-time file build and the on-query range build).

mod bitset;
mod error;
mod fst_builder;
pub mod kv_interner;
pub mod row_index;

pub use error::IndexError;
pub use fst_builder::{build_and_write, build_into};
pub use kv_interner::KvSlot;
