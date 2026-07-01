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

    /// The WAL contains records that resolve to more than one
    /// `(service.namespace, service.name)` pair. Each SFST file is
    /// required to hold exactly one stream identity, so this fails the
    /// build. Almost always indicates an `ns_hash` collision that
    /// slipped past the ingestor's canonical-stream table, or an
    /// ingestor bug that routed mismatched writes into the same file.
    #[error(
        "WAL contains multiple stream identities (ns_hash collision or ingestor bug): \
         namespaces={namespaces:?}, names={names:?}"
    )]
    MultipleStreams {
        namespaces: Vec<String>,
        names: Vec<String>,
    },

    /// The resolved stream identity is too large to encode into the
    /// substrate's `content_meta` blob (a field exceeds the codec's size
    /// limit). Fails the build rather than truncating identity.
    ///
    /// Unreachable for WALs this binary produces: the ingestor drops any frame
    /// whose identity exceeds the codec or WAL-header caps *before* it reaches
    /// the WAL, and the indexer re-derives identity from those same rows — so a
    /// persisted frame always re-encodes within budget. This guards only
    /// hand-crafted or future-producer WALs.
    #[error(
        "service identity too large to encode into content_meta; \
         shorten the service.namespace / service.name attributes"
    )]
    IdentityTooLarge,

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
