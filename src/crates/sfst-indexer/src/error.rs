//! Error type for the WAL → SFST indexing pipeline.

use sfst::Error;

/// The read-and-decode conveniences surface a two-sided error; split
/// it onto the pipeline error's existing WAL and decode variants.
impl From<wal_otap::ReadError> for IndexError {
    fn from(e: wal_otap::ReadError) -> Self {
        match e {
            wal_otap::ReadError::Wal(e) => IndexError::Wal(e),
            wal_otap::ReadError::Decode(e) => IndexError::Decode(e),
        }
    }
}

/// Error type for the WAL → SFST indexing pipeline
/// ([`crate::index`], [`crate::build_and_write`]).
///
/// Wraps the failure modes of every layer the pipeline touches: the WAL
/// reader, the OTAP/Arrow frame decoder, FST construction, and the SFST
/// format writer (via [`Format`](IndexError::Format)).
#[derive(Debug, thiserror::Error)]
pub enum IndexError {
    /// Underlying I/O failed while reading the WAL, writing the SFST
    /// output, or renaming the temp file into place.
    #[error("I/O error: {0}")]
    Io(#[from] std::io::Error),

    /// The SFST format layer (writer / `pack`) failed while emitting a
    /// chunk or assembling the on-disk container.
    #[error("SFST format error: {0}")]
    Format(#[from] Error),

    /// The WAL reader rejected the input — bad header, CRC mismatch,
    /// unsupported version, or a frame that failed to deserialize.
    #[error("WAL error: {0}")]
    Wal(#[from] wal::Error),

    /// A WAL frame's OTAP payload didn't decode into rows — Arrow IPC
    /// failure, truncated sub-stream, or an unknown payload tag (see
    /// [`wal_otap::DecodeError`]).
    #[error("frame decode failed: {0}")]
    Decode(#[from] wal_otap::DecodeError),

    /// FST construction failed — almost always because the key set
    /// wasn't sortable into the FST's required lexicographic order.
    #[error("FST build error: {0}")]
    FstBuild(#[from] fst_index::BuildError),

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
}
