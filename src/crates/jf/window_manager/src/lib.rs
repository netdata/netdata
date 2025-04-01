use error::Result;
use memmap2::{Mmap, MmapOptions};
use std::fs::File;
use std::ops::Deref;

const PAGE_SIZE: u64 = 4096;

pub trait MemoryMap: Deref<Target = [u8]> {
    /// Create a new memory map
    fn create(file: &File, offset: u64, size: u64) -> Result<Self>
    where
        Self: Sized;
}

impl MemoryMap for Mmap {
    fn create(file: &File, offset: u64, size: u64) -> Result<Self> {
        let mmap = unsafe {
            MmapOptions::new()
                .offset(offset)
                .len(size as usize)
                .map(file)?
        };

        Ok(mmap)
    }
}

pub struct Window<T: MemoryMap> {
    pub offset: u64,
    pub size: u64,
    pub mmap: T,
}

impl<T: MemoryMap> std::fmt::Debug for Window<T> {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.debug_struct("Window")
            .field("offset", &self.offset)
            .field("size", &self.size)
            // mmap field is intentionally omitted
            .finish()
    }
}

impl<T: MemoryMap> Window<T> {
    pub fn end_offset(&self) -> u64 {
        self.offset + self.size
    }

    pub fn contains(&self, position: u64) -> bool {
        position >= self.offset && position < self.end_offset()
    }

    pub fn contains_range(&self, position: u64, size: u64) -> bool {
        position >= self.offset && position + size <= self.end_offset()
    }

    pub fn get_slice(&self, position: u64, size: u64) -> &[u8] {
        debug_assert!(self.contains_range(position, size));

        let offset = (position - self.offset) as usize;
        &self.mmap[offset..offset + size as usize]
    }
}

#[derive(Clone, Copy, Debug, Default)]
pub struct WindowManagerStatistics {
    direct_lookups: usize,
    indirect_lookups: usize,
    missed_lookups: usize,
}

pub struct WindowManager<M: MemoryMap> {
    chunk_size: u64,
    active_window_idx: Option<usize>,
    max_windows: usize,
    windows: Vec<Window<M>>,
    statistics: WindowManagerStatistics,
}

impl<M: MemoryMap> WindowManager<M> {
    /// Creates a new WindowManager with the specified chunk size
    pub fn new(chunk_size: u64, max_windows: usize) -> Self {
        debug_assert!(chunk_size != 0 && (chunk_size % PAGE_SIZE) == 0);
        debug_assert!(max_windows != 0);

        WindowManager {
            chunk_size,
            max_windows,
            windows: Vec::new(),
            active_window_idx: None,
            statistics: Default::default(),
        }
    }

    /// Gets the start position aligned to the chunk size
    fn get_chunk_aligned_start(&self, position: u64) -> u64 {
        (position / self.chunk_size) * self.chunk_size
    }

    fn get_chunk_aligned_end(&self, position: u64) -> u64 {
        position.div_ceil(self.chunk_size) * self.chunk_size
    }

    fn create_window(&self, file: &File, window_start: u64, chunk_count: u64) -> Result<Window<M>> {
        debug_assert_ne!(chunk_count, 0);

        let size = chunk_count * self.chunk_size;
        let mmap = M::create(file, window_start, size).expect("memory map to succeed");
        Ok(Window {
            offset: window_start,
            size,
            mmap,
        })
    }

    /// Finds the oldest window to evict, avoiding the active window if possible
    fn find_window_to_evict(&self) -> usize {
        // The oldest window is at index 0, but we want to avoid evicting the active window
        if self.active_window_idx == Some(0) && self.windows.len() > 1 {
            // If the active window is the oldest, return the second oldest
            1
        } else {
            // Otherwise, return the oldest window
            0
        }
    }

    fn lookup_window_by_range(&self, position: u64, size_needed: u64) -> Option<usize> {
        // First, check if the active window covers this position
        if let Some(idx) = self.active_window_idx {
            if self.windows[idx].contains_range(position, size_needed) {
                return Some(idx);
            }
        }

        // If not in active window, look through all windows
        for (idx, window) in self.windows.iter().enumerate() {
            if window.contains_range(position, size_needed) {
                return Some(idx);
            }
        }

        None
    }

    fn lookup_window_by_position(&self, position: u64) -> Option<usize> {
        // First, check if the active window contains this position
        if let Some(idx) = self.active_window_idx {
            if self.windows[idx].contains(position) {
                return Some(idx);
            }
        }

        // If not in active window, look through all windows
        for (idx, window) in self.windows.iter().enumerate() {
            if window.contains(position) {
                return Some(idx);
            }
        }

        None
    }

    /// Ensures a window exists that covers the given position and size, and returns its index
    pub fn get_window(
        &mut self,
        file: &File,
        position: u64,
        size_needed: u64,
    ) -> Result<&mut Window<M>> {
        // Check if we already have a window that covers this range
        if let Some(idx) = self.lookup_window_by_range(position, size_needed) {
            self.statistics.direct_lookups += 1;
            Ok(&mut self.windows[idx])
        }
        // Check if we already have a window that we can remap/resize
        else if let Some(idx) = self.lookup_window_by_position(position) {
            self.statistics.indirect_lookups += 1;

            let window = self.windows.remove(idx);

            let window_start = window.offset;
            let window_end = self.get_chunk_aligned_end(position + size_needed);
            let num_chunks = (window_end - window_start) / self.chunk_size;

            let new_window = self.create_window(file, window_start, num_chunks)?;

            self.windows.push(new_window);
            self.active_window_idx = Some(self.windows.len() - 1);
            Ok(self.windows.last_mut().unwrap())
        }
        // We have to create a new window
        else {
            self.statistics.missed_lookups += 1;

            // Check if we have to evict a window prior to creating a new one
            if self.windows.len() >= self.max_windows {
                self.windows.remove(self.find_window_to_evict());
            }

            // Calculate window start for this position
            let window_start = self.get_chunk_aligned_start(position);
            let window_end = self.get_chunk_aligned_end(position + size_needed);
            let num_chunks = (window_end - window_start) / self.chunk_size;

            let new_window = self.create_window(file, window_start, num_chunks)?;

            // Add the new window to the end of the vector
            self.windows.push(new_window);

            // Update active window index to the new window
            self.active_window_idx = Some(self.windows.len() - 1);

            // Return the new window
            Ok(self.windows.last_mut().unwrap())
        }
    }

    pub fn stats(&self) -> WindowManagerStatistics {
        self.statistics
    }
}
