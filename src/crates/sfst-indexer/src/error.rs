//! Error type for the WAL → SFST indexing pipeline.

use sfst::Error;

/// Error type for the WAL → SFST indexing pipeline
/// ([`crate::build_and_write`], [`crate::build_into`]).
///
/// Wraps the failure modes the build layer touches: output I/O, FST
/// construction, and the SFST format writer (via
/// [`Format`](IndexError::Format)). Decoding WAL frames into rows is the
/// producer's concern and surfaces its own error type.
#[derive(Debug, thiserror::Error)]
pub enum IndexError {
    /// Underlying I/O failed while writing the SFST output or renaming the
    /// temp file into place.
    #[error("I/O error: {0}")]
    Io(#[from] std::io::Error),

    /// The SFST format layer (writer / `pack`) failed while emitting a
    /// chunk or assembling the on-disk container. Includes FST-build failures
    /// (e.g. duplicate keys), which the writer now surfaces as
    /// [`sfst::Error::PrefixMapBuild`].
    #[error("SFST format error: {0}")]
    Format(#[from] Error),

    /// A per-row column supplied to the builder has a different length than the
    /// row count, so it cannot be aligned per row. A caller bug (each column must
    /// hold exactly one value per row); recoverable, not a panic.
    #[error("per-row column {column} has {got} values, expected {expected} (one per row)")]
    ColumnLengthMismatch {
        column: &'static str,
        got: usize,
        expected: usize,
    },
}
