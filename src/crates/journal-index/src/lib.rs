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
    Anchor, Direction, FileIndex, FstIndex, LogEntryId, LogQueryParams, LogQueryParamsBuilder,
};

pub mod file_indexer;
pub use file_indexer::{
    DEFAULT_MAX_FIELD_PAYLOAD_SIZE, DEFAULT_MAX_UNIQUE_VALUES_PER_FIELD, FileIndexer,
    IndexingLimits,
};

pub mod bitmap;
pub use bitmap::Bitmap;

pub mod filter;
pub use filter::{Filter, FstLookup};

pub mod field_types;
pub use field_types::{FieldName, FieldValuePair};
