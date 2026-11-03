//! The mmap abstraction of the journal layer: the [`MemoryMap`]/[`MemoryMapMut`]
//! trait pair (implemented for memmap2's `Mmap`/`MmapMut`) and the
//! [`WindowManager`] covering one file with a bounded set of windows. In a
//! journal file only the header and the two hash tables keep persistent
//! maps; every other object is read or written through these windows
//! (file/file.rs).
//!
//! Mapping lifecycle: a map covers a fixed file range and is unmapped by
//! being dropped - no open-vs-create split, no in-place resize; changing a
//! window's coverage means unmapping it and mapping a new one (the remap
//! path in `get_window`). Read-only `create` maps `[offset, offset+size)`
//! as-is: nothing validates the range against the file length, so pages
//! past the end are unbacked and touching them raises SIGBUS. Writable
//! `create` extends the file to `offset + size` first (`File::set_len`,
//! zero-filled hole), so its range is always backed - this is how journal
//! appends grow a file. `MemoryMapMut::flush` msyncs a map (blocking);
//! `WindowManager::sync` fdatasyncs the file through the fd the manager
//! owns.
//!
//! Window model: windows start at chunk boundaries and cover whole chunks -
//! at least one, more when a request spans a boundary. A request that does
//! not fit any window remaps the window containing its position,
//! re-centered on the request (sequential appends then slide one window
//! forward instead of growing a single mapping from the file start), or
//! opens a new window, evicting the oldest at `max_windows`. Sizing is
//! caller policy: JournalFile passes 16 windows after `open` / 32 after
//! `create` but takes chunk sizes from its callers - the
//! [`JournalFileOptions`](crate::file::JournalFileOptions) default is 64 KiB,
//! rotation switches to 8 MiB
//! ([`JournalFile::open`](crate::file::JournalFile::open),
//! [`JournalFile::create`](crate::file::JournalFile::create),
//! [`JournalFile::create_successor`](crate::file::JournalFile::create_successor)).
//!
//! Load-bearing contract: any access can unmap or move windows, so a slice
//! handed out earlier dangles once its window goes. Through `&mut self` the
//! borrow checker forbids holding a slice across another access;
//! `JournalFile` reaches its manager through interior mutability instead
//! (file/guarded_cell.rs), where the one-window-view-at-a-time guard is the
//! only protection and a second live view fails with
//! `JournalError::ValueGuardInUse`.
//!
//! Errors: mmap, `set_len`, `metadata`, `flush` and `sync_data` all
//! surface as [`JournalError::Io`](crate::error::JournalError::Io) - the
//! only error this
//! module raises. There is no runtime bounds validation beyond debug
//! assertions: a bad position/size panics on slice indexing, never UB.
//!
//! Threading: no interior mutability - access is `&mut self` only and
//! `Send`/`Sync` follow `M` (both memmap2 types are); the journal file type
//! is `!Sync` because of its `GuardedCell` wrapper, not this module.
//!
//! Consumers: `JournalFile` (file/file.rs) is the only `WindowManager` user,
//! and journal-log-writer imports `MmapMut` from here directly
//! (journal-log-writer/src/log/mod.rs). This module's near-twin is
//! src/crates/jf/window_manager/src/lib.rs (keeps the old window start on
//! remap, no `flush`/`sync`, no failure logging).
//!
//! For the SIGBUS hazard above, the crate ships a recovery handler
//! (file/sigbus.rs, re-exported
//! [`install_sigbus_handler`](crate::install_sigbus_handler)) but
//! nothing in this repo installs it: netflow-plugin installs the published
//! twin's copy (netflow-plugin/src/main.rs) and the FFI reader its own
//! (jf/journal_reader_ffi/src/lib.rs).
use crate::error::Result;
use journal_common::compat::is_multiple_of;
use std::fs::File;
use std::ops::{Deref, DerefMut};
use tracing::error;

// Re-exports memmap2 so dependents can name these types without a direct
// memmap2 dependency; `MmapOptions` is also used unqualified below.
pub use memmap2::{Mmap, MmapMut, MmapOptions};

// Page size; used only to assert that window (chunk) sizes are page-aligned
const PAGE_SIZE: u64 = 4096;

/// A mapped byte range of a file, deref'ing to the mapped bytes. `create`
/// maps `size` bytes at `offset`; what happens past the file's end is the
/// implementor's choice (read-only maps leave it unbacked, writable ones
/// extend the file).
pub trait MemoryMap: Deref<Target = [u8]> {
    fn create(file: &File, offset: u64, size: u64) -> Result<Self>
    where
        Self: Sized;
}

/// A writable map: `create` extends the file to cover the requested range
/// before mapping.
pub trait MemoryMapMut: MemoryMap + DerefMut {
    /// Flushes outstanding modifications to disk (msync, blocking until
    /// persisted).
    fn flush(&self) -> Result<()>;
}

impl MemoryMap for Mmap {
    fn create(file: &File, offset: u64, size: u64) -> Result<Self> {
        // Read-only: maps the range as-is - pages past the file end stay
        // unbacked (SIGBUS when touched).
        let mmap = unsafe {
            MmapOptions::new()
                .offset(offset)
                .len(size as usize)
                .map(file)?
        };

        Ok(mmap)
    }
}

impl MemoryMap for MmapMut {
    fn create(file: &File, offset: u64, size: u64) -> Result<Self> {
        // Writable: extend the file to cover [offset, offset+size) with a
        // zero-filled hole first, so the whole mapped range is backed.
        let required_size = offset + size;

        if required_size > file.metadata()?.len() {
            file.set_len(required_size)?;
        }

        let mmap = unsafe {
            MmapOptions::new()
                .offset(offset)
                .len(size as usize)
                .map_mut(file)?
        };

        Ok(mmap)
    }
}

impl MemoryMapMut for MmapMut {
    fn flush(&self) -> Result<()> {
        MmapMut::flush(self)?;
        Ok(())
    }
}

// One mapping over whole chunks: covers [offset, offset+size) of the
// file. Dropping a window unmaps its range, so remaps and evictions
// below invalidate slices taken from it.
struct Window<M: MemoryMap> {
    offset: u64,
    size: u64,
    mmap: M,
}

// Debug output shows offset/size only, never the mapped bytes
impl<M: MemoryMap> std::fmt::Debug for Window<M> {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.debug_struct("Window")
            .field("offset", &self.offset)
            .field("size", &self.size)
            .finish()
    }
}

impl<M: MemoryMap> Window<M> {
    fn end_offset(&self) -> u64 {
        self.offset + self.size
    }

    fn contains(&self, position: u64) -> bool {
        position >= self.offset && position < self.end_offset()
    }

    fn contains_range(&self, position: u64, size: u64) -> bool {
        position >= self.offset && position + size <= self.end_offset()
    }

    fn get_slice(&self, position: u64, size: u64) -> &[u8] {
        debug_assert!(self.contains_range(position, size));

        let offset = (position - self.offset) as usize;
        &self.mmap[offset..offset + size as usize]
    }
}

impl<M: MemoryMapMut> Window<M> {
    pub fn get_mut_slice(&mut self, position: u64, size: u64) -> &mut [u8] {
        debug_assert!(self.contains_range(position, size));

        let offset = (position - self.offset) as usize;
        &mut self.mmap[offset..offset + size as usize]
    }
}

/// A bounded set of windows over one file, remapped on demand: at most
/// `max_windows` mappings, each starting at a chunk boundary and sized to
/// cover whatever range is requested (see `get_window`). Owns the file
/// descriptor, so `sync` fdatasyncs through it.
///
/// No interior mutability and no guard of its own: any access can unmap or
/// move windows, so a slice handed out earlier must not be held across the
/// next access (module doc). `JournalFile` wraps this in a `GuardedCell`
/// to enforce exactly that (file/guarded_cell.rs).
pub struct WindowManager<M: MemoryMap> {
    file: File,
    // File length at construction; currently unused
    _file_size: u64,
    chunk_size: u64,
    active_window_idx: Option<usize>,
    max_windows: usize,
    windows: Vec<Window<M>>,
}

impl<M: MemoryMap> WindowManager<M> {
    /// Takes ownership of `file` and starts with no windows. `chunk_size`
    /// is the unit and minimum window size (debug-asserted non-zero and
    /// page-aligned; `is_multiple_of` is the MSRV backport of
    /// `u64::is_multiple_of`) and `max_windows` caps the window count
    /// (debug-asserted non-zero). Callers set both: JournalFile passes 16
    /// (open) / 32 (create) windows but takes chunk sizes from its callers -
    /// the `JournalFileOptions` default is 64 KiB, rotation uses 8 MiB
    /// ([`JournalFile::open`](crate::file::JournalFile::open) /
    /// [`JournalFile::create`](crate::file::JournalFile::create)).
    pub fn new(file: File, chunk_size: u64, max_windows: usize) -> Result<Self> {
        debug_assert!(chunk_size != 0 && is_multiple_of(chunk_size, PAGE_SIZE));
        debug_assert!(max_windows != 0);

        let _file_size = file.metadata()?.len();

        Ok(WindowManager {
            file,
            _file_size,
            chunk_size,
            max_windows,
            windows: Vec::new(),
            active_window_idx: None,
        })
    }

    fn get_chunk_aligned_start(&self, position: u64) -> u64 {
        (position / self.chunk_size) * self.chunk_size
    }

    fn get_chunk_aligned_end(&self, position: u64) -> u64 {
        position.div_ceil(self.chunk_size) * self.chunk_size
    }

    // Maps `chunk_count * chunk_size` bytes at `window_start`; failures
    // are logged with the requested range and propagated.
    fn create_window(&self, window_start: u64, chunk_count: u64) -> Result<Window<M>> {
        debug_assert_ne!(chunk_count, 0);

        let size = chunk_count * self.chunk_size;
        let mmap = M::create(&self.file, window_start, size).map_err(|e| {
            error!(
                window_start,
                size,
                chunk_count,
                chunk_size = self.chunk_size,
                "mmap failed: {e}"
            );
            e
        })?;
        Ok(Window {
            offset: window_start,
            size,
            mmap,
        })
    }

    // Windows are appended in creation order and removals preserve the
    // order of the rest, so index 0 is always the oldest surviving window.
    // Evict it - unless it is the active window and another window exists,
    // then evict index 1 instead.
    fn find_window_to_evict(&self) -> usize {
        if self.active_window_idx == Some(0) && self.windows.len() > 1 {
            1
        } else {
            0
        }
    }

    // Active window first (hot path for repeated access), then all
    // windows; the whole requested range must fit.
    fn lookup_window_by_range(&self, position: u64, size_needed: u64) -> Option<usize> {
        if let Some(idx) = self.active_window_idx {
            if self.windows[idx].contains_range(position, size_needed) {
                return Some(idx);
            }
        }

        for (idx, window) in self.windows.iter().enumerate() {
            if window.contains_range(position, size_needed) {
                return Some(idx);
            }
        }

        None
    }

    // Same, but a position-only hit (range does not fit) still lets the
    // caller remap that window - see `get_window`.
    fn lookup_window_by_position(&self, position: u64) -> Option<usize> {
        if let Some(idx) = self.active_window_idx {
            if self.windows[idx].contains(position) {
                return Some(idx);
            }
        }

        for (idx, window) in self.windows.iter().enumerate() {
            if window.contains(position) {
                return Some(idx);
            }
        }

        None
    }

    // Returns a window covering [position, position+size): reusing an
    // existing window, remapping a partial hit, or creating a new one.
    fn get_window(&mut self, position: u64, size_needed: u64) -> Result<&mut Window<M>> {
        if let Some(idx) = self.lookup_window_by_range(position, size_needed) {
            Ok(&mut self.windows[idx])
        } else if let Some(idx) = self.lookup_window_by_position(position) {

            let _window = self.windows.remove(idx);
            // The removal above shifts (or drops) indices, so clear the
            // active index: if create_window fails below, no stale index
            // may point at a moved or non-existent window.
            self.active_window_idx = None;

            // Re-center the new window on the requested range rather than
            // keeping the old start, which would let sequential append
            // access grow one mapping from the file start toward the tail.
            let window_start = self.get_chunk_aligned_start(position);
            let window_end = self.get_chunk_aligned_end(position + size_needed);
            let num_chunks = (window_end - window_start) / self.chunk_size;

            let new_window = self.create_window(window_start, num_chunks)?;

            self.windows.push(new_window);
            self.active_window_idx = Some(self.windows.len() - 1);
            Ok(self.windows.last_mut().unwrap())
        } else {

            if self.windows.len() >= self.max_windows {
                self.windows.remove(self.find_window_to_evict());
                // The eviction above shifted indices; clear the active
                // index so a failed create below leaves consistent state.
                self.active_window_idx = None;
            }

            {
                // Window spans whole chunks: the chunk holding `position`
                // through the chunk holding the range's end (>= 1 chunk).
                let window_start = self.get_chunk_aligned_start(position);
                let window_end = self.get_chunk_aligned_end(position + size_needed);
                let num_chunks = (window_end - window_start) / self.chunk_size;

                let new_window = self.create_window(window_start, num_chunks)?;

                self.windows.push(new_window);
            }

            self.active_window_idx = Some(self.windows.len() - 1);
            Ok(self.windows.last_mut().unwrap())
        }
    }

    /// Returns the bytes at `[position, position+size)`, mapping a window
    /// over them first. The returned slice borrows the manager, so the
    /// borrow checker itself forbids holding it across another access; the
    /// interior-mutability path (JournalFile/GuardedCell) serializes
    /// instead (module doc).
    pub fn get_slice(&mut self, position: u64, size: u64) -> Result<&[u8]> {
        let window = self.get_window(position, size)?;
        Ok(window.get_slice(position, size))
    }
}

impl<M: MemoryMapMut> WindowManager<M> {
    /// Mutable variant of `get_slice`, for writable maps (`MmapMut`).
    pub fn get_slice_mut(&mut self, position: u64, size: u64) -> Result<&mut [u8]> {
        let window = self.get_window(position, size)?;
        Ok(window.get_mut_slice(position, size))
    }

    /// fdatasync of the file through the fd the manager owns. Windows and
    /// hash-table maps reach disk only through this;
    /// [`JournalFile::sync`](crate::file::JournalFile::sync)
    /// additionally msyncs the header map first.
    pub fn sync(&self) -> Result<()> {
        self.file.sync_data()?;
        Ok(())
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::error::JournalError;
    use std::cell::Cell;
    use std::io::Write;
    use std::rc::Rc;
    use tempfile::NamedTempFile;

    const PAGE_SIZE_TEST: u64 = 4096;

    /// Mock map that fails on demand (see `MockController`), used to test
    /// WindowManager state after a failed window creation.
    struct FailingMmap {
        data: Vec<u8>,
    }

    impl Deref for FailingMmap {
        type Target = [u8];
        fn deref(&self) -> &[u8] {
            &self.data
        }
    }

    /// Shared state to control when the mock should fail
    struct MockController {
        fail_next_create: Cell<bool>,
        create_count: Cell<usize>,
    }

    impl MockController {
        fn new() -> Self {
            Self {
                fail_next_create: Cell::new(false),
                create_count: Cell::new(0),
            }
        }

        fn set_fail_next(&self, fail: bool) {
            self.fail_next_create.set(fail);
        }

        fn should_fail(&self) -> bool {
            let count = self.create_count.get();
            self.create_count.set(count + 1);
            self.fail_next_create.get()
        }
    }

    thread_local! {
        static MOCK_CONTROLLER: Rc<MockController> = Rc::new(MockController::new());
    }

    impl MemoryMap for FailingMmap {
        fn create(_file: &File, _offset: u64, size: u64) -> Result<Self> {
            MOCK_CONTROLLER.with(|ctrl| {
                if ctrl.should_fail() {
                    return Err(JournalError::Io(std::io::Error::new(
                        std::io::ErrorKind::Other,
                        "simulated mmap failure",
                    )));
                }
                // Create a mock mmap with zeros
                Ok(FailingMmap {
                    data: vec![0u8; size as usize],
                })
            })
        }
    }

    /// A failed remap must leave the manager consistent: the old window is
    /// removed before create_window fails, so `windows` ends up empty, the
    /// active index must be `None` (no stale index), and the next access
    /// must recover by creating a fresh window.
    #[test]
    fn test_consistent_state_after_failed_remap() {
        let mut temp_file = NamedTempFile::new().unwrap();
        temp_file.write_all(&[0u8; 8192]).unwrap();
        temp_file.flush().unwrap();

        let file = File::open(temp_file.path()).unwrap();

        // Create WindowManager with mock mmap, 4KB chunks, max 1 window
        let mut wm: WindowManager<FailingMmap> =
            WindowManager::new(file, PAGE_SIZE_TEST, 1).unwrap();

        // Reset controller state
        MOCK_CONTROLLER.with(|ctrl| {
            ctrl.set_fail_next(false);
            ctrl.create_count.set(0);
        });

        // First read: creates window [0, 4096)
        {
            let slice = wm.get_slice(0, 100).unwrap();
            assert_eq!(slice.len(), 100);
        }
        assert_eq!(wm.windows.len(), 1);
        assert_eq!(wm.active_window_idx, Some(0));

        // Configure mock to fail on the next create call
        MOCK_CONTROLLER.with(|ctrl| ctrl.set_fail_next(true));

        // Position 100 is inside window [0, 4096) but [100, 4100) does not
        // fit: remap branch - old window removed, then create_window fails.
        let remap_result = wm.get_slice(100, 4000);
        assert!(remap_result.is_err(), "Expected remap to fail");

        assert_eq!(wm.windows.len(), 0);
        assert_eq!(wm.active_window_idx, None);

        // Allow the next create to succeed
        MOCK_CONTROLLER.with(|ctrl| ctrl.set_fail_next(false));

        // Recovery: the next access creates a fresh window
        let result = wm.get_slice(0, 100);
        assert!(
            result.is_ok(),
            "Expected get_slice to succeed after recovery"
        );
        assert_eq!(wm.windows.len(), 1);
    }

    /// A failed window creation in the eviction path must leave the
    /// manager consistent too: the evicted window is gone, `windows` ends
    /// up empty and the active index `None`, and the next access recovers.
    #[test]
    fn test_consistent_state_after_failed_eviction() {
        let mut temp_file = NamedTempFile::new().unwrap();
        temp_file.write_all(&[0u8; 8192]).unwrap();
        temp_file.flush().unwrap();

        let file = File::open(temp_file.path()).unwrap();

        // Create WindowManager with mock mmap, 4KB chunks, max 1 window
        let mut wm: WindowManager<FailingMmap> =
            WindowManager::new(file, PAGE_SIZE_TEST, 1).unwrap();

        // Reset controller state
        MOCK_CONTROLLER.with(|ctrl| {
            ctrl.set_fail_next(false);
            ctrl.create_count.set(0);
        });

        // Create first window at offset 0
        {
            let _slice = wm.get_slice(0, 100).unwrap();
        }
        assert_eq!(wm.windows.len(), 1);
        assert_eq!(wm.active_window_idx, Some(0));

        // Configure mock to fail on the next create call
        MOCK_CONTROLLER.with(|ctrl| ctrl.set_fail_next(true));

        // Position 4096 is outside window [0, 4096): brand-new-window
        // branch - the old window is evicted (max_windows = 1), then
        // create_window fails.
        let result = wm.get_slice(4096, 100);
        assert!(result.is_err(), "Expected mmap to fail");

        assert_eq!(wm.windows.len(), 0);
        assert_eq!(wm.active_window_idx, None);

        // Allow the next create to succeed
        MOCK_CONTROLLER.with(|ctrl| ctrl.set_fail_next(false));

        // Recovery: the next access creates a fresh window
        let result = wm.get_slice(0, 100);
        assert!(
            result.is_ok(),
            "Expected get_slice to succeed after recovery"
        );
        assert_eq!(wm.windows.len(), 1);
    }

    /// Sequential access crossing chunk boundaries: the first crossing
    /// re-centers the window and grows it to two chunks, later crossings
    /// slide the two-chunk window forward - never growing past two chunks
    /// or re-anchoring at the file start.
    #[test]
    fn sequential_boundary_crossing_slides_window_instead_of_growing_from_start() {
        let mut temp_file = NamedTempFile::new().unwrap();
        temp_file.write_all(&[0u8; 64 * 1024]).unwrap();
        temp_file.flush().unwrap();

        let file = File::open(temp_file.path()).unwrap();
        let mut wm: WindowManager<FailingMmap> =
            WindowManager::new(file, PAGE_SIZE_TEST, 1).unwrap();

        MOCK_CONTROLLER.with(|ctrl| {
            ctrl.set_fail_next(false);
            ctrl.create_count.set(0);
        });

        let _ = wm.get_slice(0, 100).unwrap();
        assert_eq!(wm.windows[0].offset, 0);
        assert_eq!(wm.windows[0].size, PAGE_SIZE_TEST);

        let _ = wm.get_slice(PAGE_SIZE_TEST - 6, 32).unwrap();
        assert_eq!(wm.windows[0].offset, 0);
        assert_eq!(wm.windows[0].size, PAGE_SIZE_TEST * 2);

        let _ = wm.get_slice((PAGE_SIZE_TEST * 2) - 12, 32).unwrap();
        assert_eq!(wm.windows[0].offset, PAGE_SIZE_TEST);
        assert_eq!(wm.windows[0].size, PAGE_SIZE_TEST * 2);

        let _ = wm.get_slice((PAGE_SIZE_TEST * 3) - 20, 32).unwrap();
        assert_eq!(wm.windows[0].offset, PAGE_SIZE_TEST * 2);
        assert_eq!(wm.windows[0].size, PAGE_SIZE_TEST * 2);
    }
}
