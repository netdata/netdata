use crate::error::Result;
use journal_common::compat::is_multiple_of;
use std::fs::File;
use std::ops::{Deref, DerefMut};

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
        let mmap = M::create(&self.file, window_start, size)?;
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
            }

            // NOTE: the active window index might have been invalidated. In
            // the scope that follows, we should not use code that relies on it.
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
