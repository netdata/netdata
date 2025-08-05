//! Time range metadata for indexed journal files

use crate::Seconds;

/// Time range information for a journal file derived from indexing it.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum TimeRange {
    /// File has not been indexed yet, time range unknown. These files will
    /// be queued for indexing and reported in subsequent poll cycles.
    Unknown,

    /// Active file currently being written to. The end time represents
    /// the latest entry seen when the file was indexed, but new entries
    /// may have been written since.
    Active {
        start: Seconds,
        end: Seconds,
        indexed_at: Seconds,
    },

    /// Archived file with known start and end times.
    Bounded {
        start: Seconds,
        end: Seconds,
        indexed_at: Seconds,
    },
}
