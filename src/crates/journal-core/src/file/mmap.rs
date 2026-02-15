use crate::error::Result;
use journal_common::compat::is_multiple_of;
use std::fs::File;
use std::ops::{Deref, DerefMut};
use tracing::error;

// Re-export memmap2 types for other crates and import for internal use
pub use memmap2::{Mmap, MmapMut, MmapOptions};

const PAGE_SIZE: u64 = 4096;

pub trait MemoryMap: Deref<Target = [u8]> {
    fn create(file: &File, offset: u64, size: u64) -> Result<Self>
    where
        Self: Sized;
}

pub trait MemoryMapMut: MemoryMap + DerefMut {
    /// Flushes outstanding memory map modifications to disk
    fn flush(&self) -> Result<()>;
}

impl MemoryMap for Mmap {
    fn create(file: &File, offset: u64, size: u64) -> Result<Self> {
        let mmap = unsafe {
            MmapOptions::new()
                .offset(offset)
                .len(size as usize)
                .populate()
                .map(file)?
        };

        Ok(mmap)
    }
}

impl MemoryMap for MmapMut {
    fn create(file: &File, offset: u64, size: u64) -> Result<Self> {
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

struct Window<M: MemoryMap> {
    offset: u64,
    size: u64,
    mmap: M,
}

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

pub struct WindowManager<M: MemoryMap> {
    file: File,
    _file_size: u64,
    chunk_size: u64,
    active_window_idx: Option<usize>,
    max_windows: usize,
    windows: Vec<Window<M>>,
}

impl<M: MemoryMap> WindowManager<M> {
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

    fn find_window_to_evict(&self) -> usize {
        if self.active_window_idx == Some(0) && self.windows.len() > 1 {
            1
        } else {
            0
        }
    }

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

    fn get_window(&mut self, position: u64, size_needed: u64) -> Result<&mut Window<M>> {
        if let Some(idx) = self.lookup_window_by_range(position, size_needed) {
            // Use the existing window
            Ok(&mut self.windows[idx])
        } else if let Some(idx) = self.lookup_window_by_position(position) {
            // Remap the window

            let window = self.windows.remove(idx);
            // Invalidate active_window_idx before removal to maintain consistency.
            // If create_window fails, the index won't point to a non-existent window.
            self.active_window_idx = None;

            let window_start = window.offset;
            let window_end = self.get_chunk_aligned_end(position + size_needed);
            let num_chunks = (window_end - window_start) / self.chunk_size;

            let new_window = self.create_window(window_start, num_chunks)?;

            self.windows.push(new_window);
            self.active_window_idx = Some(self.windows.len() - 1);
            Ok(self.windows.last_mut().unwrap())
        } else {
            // Create a brand new window

            if self.windows.len() >= self.max_windows {
                self.windows.remove(self.find_window_to_evict());
                // Invalidate active_window_idx after removal to maintain consistency.
                // If create_window fails below, the index won't point to a non-existent window.
                self.active_window_idx = None;
            }

            {
                // Calculate window start for this position
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

    pub fn get_slice(&mut self, position: u64, size: u64) -> Result<&[u8]> {
        let window = self.get_window(position, size)?;
        Ok(window.get_slice(position, size))
    }
}

impl<M: MemoryMapMut> WindowManager<M> {
    pub fn get_slice_mut(&mut self, position: u64, size: u64) -> Result<&mut [u8]> {
        let window = self.get_window(position, size)?;
        Ok(window.get_mut_slice(position, size))
    }

    /// Syncs all file data to disk
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

    /// A mock MemoryMap that can be configured to fail on specific calls.
    /// This allows us to test error handling in WindowManager.
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

    // Thread-local controller for the mock
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

    /// This test verifies that WindowManager maintains consistent state
    /// after a failed remap operation.
    ///
    /// The scenario:
    /// 1. A window exists at some position
    /// 2. A request comes in that requires remapping (position in window, but size extends beyond)
    /// 3. The old window is removed
    /// 4. Creating the new (larger) window fails (e.g., mmap error)
    /// 5. The WindowManager should remain in a consistent state
    /// 6. Subsequent operations should not panic
    #[test]
    fn test_consistent_state_after_failed_remap() {
        // Create a temporary file (content doesn't matter for mock)
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

        // First read: creates a window at offset 0, size 4KB (this should succeed)
        {
            let slice = wm.get_slice(0, 100).unwrap();
            assert_eq!(slice.len(), 100);
        }
        assert_eq!(wm.windows.len(), 1);
        assert_eq!(wm.active_window_idx, Some(0));

        // Configure mock to fail on the next create call
        MOCK_CONTROLLER.with(|ctrl| ctrl.set_fail_next(true));

        // Request a slice that requires remapping:
        // - Position 100 is within the existing window [0, 4096)
        // - But size 4000 means we need bytes [100, 4100), which extends beyond window
        // - This triggers the "Remap the window" branch
        // - The old window is removed
        // - Then create_window is called and FAILS
        let remap_result = wm.get_slice(100, 4000);
        assert!(remap_result.is_err(), "Expected remap to fail");

        // Verify state is consistent after the failure:
        // - windows is empty (the old window was removed, new one failed to create)
        // - active_window_idx should be None (not pointing to non-existent window)
        assert_eq!(wm.windows.len(), 0);
        assert_eq!(wm.active_window_idx, None);

        // Allow the next create to succeed
        MOCK_CONTROLLER.with(|ctrl| ctrl.set_fail_next(false));

        // The next operation should NOT panic - it should succeed by creating a new window
        let result = wm.get_slice(0, 100);
        assert!(
            result.is_ok(),
            "Expected get_slice to succeed after recovery"
        );
        assert_eq!(wm.windows.len(), 1);
    }

    /// This test verifies that WindowManager maintains consistent state
    /// after a failed window creation in the eviction path.
    ///
    /// The scenario:
    /// 1. A window exists and we're at max_windows
    /// 2. A request comes in for a different region requiring a new window
    /// 3. The old window is evicted to make room
    /// 4. Creating the new window fails (e.g., mmap error)
    /// 5. The WindowManager should remain in a consistent state
    /// 6. Subsequent operations should not panic
    #[test]
    fn test_consistent_state_after_failed_eviction() {
        // Create a temporary file
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

        // Request a slice at a completely different position (second page)
        // This triggers:
        // - lookup_window_by_range returns None (position 4096 not in window [0, 4096))
        // - lookup_window_by_position returns None
        // - "Create a brand new window" branch
        // - Eviction: windows.remove(0) since we're at max_windows
        // - create_window fails
        let result = wm.get_slice(4096, 100);
        assert!(result.is_err(), "Expected mmap to fail");

        // Verify state is consistent after the failure:
        // - windows is empty (the old window was evicted, new one failed to create)
        // - active_window_idx should be None (not pointing to non-existent window)
        assert_eq!(wm.windows.len(), 0);
        assert_eq!(wm.active_window_idx, None);

        // Allow the next create to succeed
        MOCK_CONTROLLER.with(|ctrl| ctrl.set_fail_next(false));

        // The next operation should NOT panic - it should succeed by creating a new window
        let result = wm.get_slice(0, 100);
        assert!(
            result.is_ok(),
            "Expected get_slice to succeed after recovery"
        );
        assert_eq!(wm.windows.len(), 1);
    }
}
