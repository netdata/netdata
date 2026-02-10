//! Indexing functionality for systemd journal files.
//!
//! This crate provides:
//! - Histograms for time-based aggregation
//! - File indexing for fast lookups
//! - Bitmap-based filtering
//! - Field type definitions

pub use journal_common::{Microseconds, Seconds};

pub mod error;
pub use error::{IndexError, Result};

pub mod histogram;
pub use histogram::{Bucket, Histogram};

pub mod file_index;
pub use file_index::{
    Anchor, Direction, FileIndex, LogEntryId, LogQueryParams, LogQueryParamsBuilder,
};

pub mod file_indexer;
pub use file_indexer::FileIndexer;

pub mod bitmap;
pub use bitmap::Bitmap;

pub mod filter;
pub use filter::Filter;

pub mod field_types;
pub use field_types::{FieldName, FieldValuePair};
