//! Signal-neutral source plumbing shared by the per-signal engines.
//!
//! [`Source`] says where an SFST candidate's bytes come from; [`Mapped`]
//! is those bytes obtained (memory-mapped or shared in-memory), and
//! [`map_source`] converts one to the other. The logs engine wraps
//! [`map_source`] with its historical log-and-degrade behavior
//! (`logs::mmap`); the traces engine consumes the structured error
//! directly, because a source that fails to map must surface as an
//! explicit partial-result reason rather than silently contributing
//! nothing (the design-record status model).

use std::fs::File;
use std::path::{Path, PathBuf};
use std::sync::Arc;

use memmap2::{Mmap, UncheckedAdvice};

/// Where an SFST candidate's bytes come from.
///
/// `File` is the steady-state case — a sealed index on disk, memory-
/// mapped lazily. `Memory` is an in-memory SFST built from a chunk of an
/// active WAL (`ng_index::build_sfst_range` / `build_sfst_traces_range`);
/// the bytes are shared (`Arc`) so a query holds them alive even if the
/// producing cache evicts the entry mid-query.
#[derive(Clone)]
pub enum Source {
    File(PathBuf),
    Memory(Arc<Vec<u8>>),
}

impl Source {
    /// A short label for log/error context.
    pub(crate) fn describe(&self) -> std::borrow::Cow<'_, str> {
        match self {
            Source::File(p) => p.display().to_string().into(),
            Source::Memory(_) => "<in-memory chunk>".into(),
        }
    }
}

/// A candidate's bytes, however they are backed: a memory-mapped file or
/// an in-memory chunk image. Both deref to `&[u8]` for
/// [`sfst::IndexReader::open`]; only the file variant participates in
/// cold-suffix page-cache release (an in-memory chunk has no file pages
/// to advise away).
///
/// Cloning is a refcount bump: one mapping created at the start of a
/// query is shared by every pass over it, so a file unlinked by retention
/// mid-query stays readable (the open mapping pins the inode) and all
/// passes see the same source set.
#[derive(Clone)]
pub(crate) enum Mapped {
    File(Arc<Mmap>),
    Memory(Arc<Vec<u8>>),
}

impl Mapped {
    pub(crate) fn bytes(&self) -> &[u8] {
        match self {
            Mapped::File(m) => m,
            Mapped::Memory(v) => v,
        }
    }
}

/// A failure obtaining a source's bytes — distinguishing "the source is
/// broken" from "the source is empty" (an engine that treated the two the
/// same would silently under-report; see the design-record status model).
#[derive(Debug, thiserror::Error)]
pub(crate) enum MapError {
    #[error("open {path}: {source}")]
    Open {
        path: PathBuf,
        source: std::io::Error,
    },
    #[error("mmap {path}: {source}")]
    Mmap {
        path: PathBuf,
        source: std::io::Error,
    },
}

/// Obtain a candidate's bytes from its [`Source`]. A `File` is
/// memory-mapped; a `Memory` chunk's `Arc` is cloned (cheap — a refcount
/// bump that keeps the bytes alive for the query even if the producing
/// cache evicts the entry). Failures are structured — the caller decides
/// whether to degrade (logs) or report (traces).
pub(crate) fn map_source(source: &Source) -> Result<Mapped, MapError> {
    match source {
        Source::File(path) => map_file(path).map(|m| Mapped::File(Arc::new(m))),
        Source::Memory(bytes) => Ok(Mapped::Memory(Arc::clone(bytes))),
    }
}

/// Memory-map an SFST file read-only.
fn map_file(path: &Path) -> Result<Mmap, MapError> {
    let file = File::open(path).map_err(|source| MapError::Open {
        path: path.to_owned(),
        source,
    })?;
    // SAFETY: SFST files are immutable once the ingestor finalizes them
    // (it rolls a new file rather than mutating), so a read-only memory map
    // of one is sound for the mapping's lifetime.
    unsafe { Mmap::map(&file) }.map_err(|source| MapError::Mmap {
        path: path.to_owned(),
        source,
    })
}

/// Advise the kernel to drop a file's cold suffix from the page cache.
/// `region` is the raw `(offset, len)` from
/// [`sfst::IndexReader::cold_region`]; it is aligned **inward** to whole
/// pages so the advice never frees a hot-prefix edge page (e.g. the
/// primary FST's tail), then released in a single `madvise` call.
pub(crate) fn release_cold_region(mapping: &Mmap, region: (usize, usize)) {
    let (offset, len) = region;
    let page = page_size();
    let start = offset.next_multiple_of(page);
    let end = (offset + len) / page * page;
    if end <= start {
        return; // span shorter than a page once aligned inward — nothing to drop
    }
    // SAFETY: the mapping is a read-only view of an immutable, finalized
    // SFST file. `MADV_DONTNEED` frees only clean pages, which re-fault to
    // identical bytes from the file on next access, so the mapping's
    // contents are unchanged and any later borrow observes the same data.
    let advised =
        unsafe { mapping.unchecked_advise_range(UncheckedAdvice::DontNeed, start, end - start) };
    if let Err(e) = advised {
        // Best-effort hint — on failure the cold pages simply stay cached.
        tracing::debug!("sfsq: releasing cold region failed: {e}");
    }
}

/// The process's memory-page size, cached after the first lookup.
fn page_size() -> usize {
    use std::sync::OnceLock;
    static PAGE_SIZE: OnceLock<usize> = OnceLock::new();
    *PAGE_SIZE.get_or_init(|| {
        // SAFETY: `sysconf(_SC_PAGESIZE)` takes no pointer arguments and
        // cannot fail for this query.
        let value = unsafe { libc::sysconf(libc::_SC_PAGESIZE) };
        // 4096 is the minimum page size on every supported architecture;
        // `sysconf` only returns <= 0 when `_SC_PAGESIZE` is unsupported,
        // which doesn't happen on the platforms we run on.
        if value > 0 { value as usize } else { 4096 }
    })
}

#[cfg(test)]
mod tests {
    use super::*;

    /// The property the query's open-once design rests on: a mapping
    /// taken at the start of a query pins the file's inode, so an SFST
    /// unlinked by retention mid-query stays readable for later passes.
    #[test]
    fn mapping_survives_unlink() {
        let dir = tempfile::tempdir().unwrap();
        let path = dir.path().join("victim.sfst");
        std::fs::write(&path, b"bytes that outlive the unlink").unwrap();

        let mapped = map_source(&Source::File(path.clone())).unwrap();
        let shared = mapped.clone(); // a later pass's handle
        std::fs::remove_file(&path).unwrap();

        assert_eq!(mapped.bytes(), b"bytes that outlive the unlink");
        assert_eq!(shared.bytes(), b"bytes that outlive the unlink");
        // A fresh open of the unlinked path fails — and the failure is
        // structured, distinguishing a broken source from an empty one.
        assert!(matches!(
            map_source(&Source::File(path)),
            Err(MapError::Open { .. })
        ));
    }

    /// An in-memory chunk never fails to map and shares (not copies) its
    /// bytes.
    #[test]
    fn memory_source_maps_by_refcount() {
        let bytes = Arc::new(b"chunk image".to_vec());
        let mapped = map_source(&Source::Memory(Arc::clone(&bytes))).unwrap();
        assert_eq!(mapped.bytes(), b"chunk image");
        assert_eq!(Arc::strong_count(&bytes), 2);
    }
}
