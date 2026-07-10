//! Memory-mapped access to SFST files, shared by both query steps.
//!
//! A query maps a candidate file read-only and lets the SFST reader fault
//! in only the chunks it touches; the stream batches and unused field
//! chunks never enter memory. Once done, the query releases the file's
//! cold suffix (mid/high field chunks + stream batches) from the page
//! cache via [`release_cold_region`], keeping only the hot prefix
//! (summary / metadata / timestamps / primary) resident across queries.

use std::fs::File;
use std::path::Path;
use std::sync::Arc;

use memmap2::{Mmap, UncheckedAdvice};

use super::engine::Source;

/// A candidate's bytes, however they are backed: a memory-mapped file or
/// an in-memory chunk image. Both deref to `&[u8]` for
/// [`sfst::IndexReader::open`]; only the file variant participates in
/// cold-suffix page-cache release (an in-memory chunk has no file pages
/// to advise away).
///
/// Cloning is a refcount bump: one mapping created at the start of a
/// query is shared by both the stats and the page pass, so a file
/// unlinked by retention mid-query stays readable (the open mapping
/// pins the inode) and both passes see the same source set.
#[derive(Clone)]
pub(super) enum Mapped {
    File(Arc<Mmap>),
    Memory(Arc<Vec<u8>>),
}

impl Mapped {
    pub(super) fn bytes(&self) -> &[u8] {
        match self {
            Mapped::File(m) => m,
            Mapped::Memory(v) => v,
        }
    }
}

/// Obtain a candidate's bytes from its [`Source`], logging and returning
/// `None` on failure (so one bad source never sinks a query). A `File`
/// is memory-mapped; a `Memory` chunk's `Arc` is cloned (cheap — a
/// refcount bump that keeps the bytes alive for the query even if the
/// producing cache evicts the entry).
pub(super) fn map_source(source: &Source) -> Option<Mapped> {
    match source {
        Source::File(path) => map_file(path).map(|m| Mapped::File(Arc::new(m))),
        Source::Memory(bytes) => Some(Mapped::Memory(Arc::clone(bytes))),
    }
}

/// Memory-map an SFST file read-only, logging and returning `None` on
/// failure (so one bad file never sinks a query).
pub(super) fn map_file(path: &Path) -> Option<Mmap> {
    let file = match File::open(path) {
        Ok(file) => file,
        Err(e) => {
            tracing::warn!("sfsq: failed to open {}: {e}", path.display());
            return None;
        }
    };
    // SAFETY: SFST files are immutable once the ingestor finalizes them
    // (it rolls a new file rather than mutating), so a read-only memory map
    // of one is sound for the mapping's lifetime.
    match unsafe { Mmap::map(&file) } {
        Ok(mapping) => Some(mapping),
        Err(e) => {
            tracing::warn!("sfsq: failed to mmap {}: {e}", path.display());
            None
        }
    }
}

/// Advise the kernel to drop a file's cold suffix from the page cache.
/// `region` is the raw `(offset, len)` from
/// [`sfst::IndexReader::cold_region`]; it is aligned **inward** to whole
/// pages so the advice never frees a hot-prefix edge page (e.g. the
/// primary FST's tail), then released in a single `madvise` call.
pub(super) fn release_cold_region(mapping: &Mmap, region: (usize, usize)) {
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

#[cfg(test)]
mod tests {
    use super::*;

    /// The property the query's open-once design rests on: a mapping
    /// taken at the start of a query pins the file's inode, so an SFST
    /// unlinked by retention mid-query stays readable for the page pass.
    #[test]
    fn mapping_survives_unlink() {
        let dir = tempfile::tempdir().unwrap();
        let path = dir.path().join("victim.sfst");
        std::fs::write(&path, b"bytes that outlive the unlink").unwrap();

        let mapped = map_source(&Source::File(path.clone())).unwrap();
        let shared = mapped.clone(); // the page pass's handle
        std::fs::remove_file(&path).unwrap();

        assert_eq!(mapped.bytes(), b"bytes that outlive the unlink");
        assert_eq!(shared.bytes(), b"bytes that outlive the unlink");
        // A fresh open of the unlinked path would fail — the old
        // two-open behavior this design replaces.
        assert!(map_source(&Source::File(path)).is_none());
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
