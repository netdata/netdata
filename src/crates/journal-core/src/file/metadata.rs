//! After-arena metadata for journal files.
//!
//! [`AfterArena`] manages an opaque metadata region placed **after** the
//! journal arena (`header_size + arena_size`).  Systemd only reads within
//! the arena, so this region is invisible to `journalctl`.
//!
//! ```text
//! [JournalHeader]
//! [Arena: journal objects]
//! ─── end of arena ───
//! [magic "NDMETA01" 8B] [total_size: u64 le] [payload …]
//! ```

use super::mmap::{MemoryMap, Mmap, MmapMut};
use super::object::JournalHeader;
use crate::{JournalError, Result};
use std::fs::{File, OpenOptions};
use std::path::Path;
use zerocopy::FromBytes;

const METADATA_MAGIC: &[u8; 8] = b"NDMETA01";
const META_HEADER_SIZE: usize = 16; // magic(8) + total_size(8)

/// Handle for reading and writing the after-arena metadata region of a
/// journal file.
///
/// Opens the journal file, reads the header to locate the end of the arena,
/// and provides [`read`](Self::read) / [`write`](Self::write) access to the
/// metadata blob that lives past it.
pub struct AfterArena {
    fd: File,
    end_of_arena: u64,
}

impl AfterArena {
    /// Open a journal file for after-arena metadata access.
    pub fn open(path: &Path) -> Result<Self> {
        let fd = OpenOptions::new()
            .read(true)
            .write(true)
            .open(path)
            .map_err(JournalError::Io)?;

        let header_size = std::mem::size_of::<JournalHeader>() as u64;
        let header_map = Mmap::create(&fd, 0, header_size)?;
        let header = JournalHeader::ref_from_prefix(&header_map).unwrap().0;
        let end_of_arena = header.header_size + header.arena_size;

        Ok(Self { fd, end_of_arena })
    }

    /// Write an opaque metadata blob after the arena.
    ///
    /// Any previous metadata is replaced. The file is truncated to the arena
    /// boundary first, then `[magic][total_size][data]` is written via an
    /// `MmapMut` and flushed.
    pub fn write(&self, data: &[u8]) -> Result<()> {
        let blob_size = (META_HEADER_SIZE + data.len()) as u64;

        // Truncate away any previous metadata.
        self.fd
            .set_len(self.end_of_arena)
            .map_err(JournalError::Io)?;

        // MmapMut::create extends the file to fit.
        let mut map = MmapMut::create(&self.fd, self.end_of_arena, blob_size)?;

        map[..8].copy_from_slice(METADATA_MAGIC);
        map[8..16].copy_from_slice(&blob_size.to_le_bytes());
        map[META_HEADER_SIZE..].copy_from_slice(data);

        map.flush()?;
        Ok(())
    }

    /// Read the metadata blob, or `None` if none is present.
    pub fn read(&self) -> Result<Option<Vec<u8>>> {
        let file_len = self.fd.metadata().map_err(JournalError::Io)?.len();

        if file_len < self.end_of_arena + META_HEADER_SIZE as u64 {
            return Ok(None);
        }

        let extra = file_len - self.end_of_arena;
        let map = Mmap::create(&self.fd, self.end_of_arena, extra)?;

        if &map[..8] != METADATA_MAGIC {
            return Ok(None);
        }

        let stored_size = u64::from_le_bytes(map[8..16].try_into().unwrap()) as usize;
        if stored_size > map.len() {
            return Err(JournalError::Io(std::io::Error::new(
                std::io::ErrorKind::UnexpectedEof,
                "metadata blob truncated",
            )));
        }

        Ok(Some(map[META_HEADER_SIZE..stored_size].to_vec()))
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::file::mmap::MmapMut;
    use crate::file::{CreateJournalFile, JournalFile};
    use journal_registry::repository;

    /// Helper: create a minimal journal file and return its path.
    fn create_test_journal(dir: &Path) -> std::path::PathBuf {
        let machine_id = uuid::Uuid::nil();
        let seqnum_id = uuid::Uuid::new_v4();

        // from_path expects: <dir>/<machine_id>/system@<seqnum_id>-<seq>-<rt>.journal
        let machine_dir = dir.join(machine_id.simple().to_string());
        std::fs::create_dir_all(&machine_dir).unwrap();

        let filename = format!(
            "system@{}-{:016x}-{:016x}.journal",
            seqnum_id.simple(),
            1u64,
            1u64,
        );
        let path = machine_dir.join(filename);
        let repo_file = repository::File::from_path(&path).unwrap();

        let boot_id = uuid::Uuid::nil();
        let _jf: JournalFile<MmapMut> = CreateJournalFile::new(machine_id, boot_id, seqnum_id)
            .with_window_size(4096)
            .create(&repo_file)
            .unwrap();
        path
    }

    #[test]
    fn round_trip() {
        let dir = tempfile::TempDir::new().unwrap();
        let path = create_test_journal(dir.path());

        let meta = AfterArena::open(&path).unwrap();

        // No metadata yet.
        assert!(meta.read().unwrap().is_none());

        // Write, then read back.
        let payload = b"hello after arena";
        meta.write(payload).unwrap();

        let got = meta.read().unwrap().expect("should have metadata");
        assert_eq!(got, payload);
    }

    #[test]
    fn write_replaces_previous() {
        let dir = tempfile::TempDir::new().unwrap();
        let path = create_test_journal(dir.path());

        let meta = AfterArena::open(&path).unwrap();

        meta.write(b"first").unwrap();
        meta.write(b"second, longer").unwrap();

        let got = meta.read().unwrap().unwrap();
        assert_eq!(got, b"second, longer");
    }
}
