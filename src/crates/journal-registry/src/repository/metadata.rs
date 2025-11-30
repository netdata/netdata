//! Metadata types for journal files
//!
//! This module provides metadata tracking for journal files, including time ranges
//! derived from indexing operations.

use crate::repository::File;
pub use journal_common::TimeRange;

/// Pairs a File with its TimeRange.
#[derive(Debug, Clone)]
pub struct FileInfo {
    /// The journal file
    pub file: File,
    /// Time range from its file index
    pub time_range: TimeRange,
}
