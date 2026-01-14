use crate::error::{Result, WriterError};
use crate::log::RetentionPolicy;
use journal_common::Microseconds;
use journal_core::JournalFile;
use journal_core::collections::HashMap;
use journal_core::file::Mmap;
use journal_registry::repository;
use journal_registry::repository::File;
use std::path::PathBuf;
use uuid::Uuid;

#[allow(unused_imports)]
use tracing::{error, info, instrument};

// Helper function to create a File with archived status
fn create_chain_file(
    path: &PathBuf,
    seqnum_id: Uuid,
    head_seqnum: u64,
    head_realtime: u64,
) -> Option<repository::File> {
    // Format the path using the same logic as journal_registry
    let filename = format!(
        "system@{}-{:016x}-{:016x}.journal",
        seqnum_id.simple(),
        head_seqnum,
        head_realtime
    );

    let path = path.join(filename);

    repository::File::from_path(&path)
}

/// Manages a directory of journal files with automatic cleanup.
///
/// Scans the directory for existing files, tracks their sizes, and enforces retention
/// policies. Typically not used directly - see [`JournalLog`](crate::JournalLog) instead.
#[derive(Debug)]
pub(super) struct OwnedChain {
    pub(super) path: PathBuf,
    pub(super) machine_id: Uuid,

    pub(super) inner: repository::Chain,
    pub(super) file_sizes: HashMap<File, u64>,
    pub(super) total_size: u64,
}

impl OwnedChain {
    pub(super) fn new(path: PathBuf, machine_id: Uuid) -> Result<Self> {
        #[cfg(debug_assertions)]
        {
            use std::os::unix::ffi::OsStrExt;

            debug_assert!(path.exists() && path.is_dir());

            let filename = path.file_name().unwrap().as_bytes();
            debug_assert_eq!(Ok(machine_id), Uuid::try_parse_ascii(filename));
        }

        let mut chain = Self {
            path,
            machine_id,
            inner: repository::Chain::default(),
            file_sizes: HashMap::default(),
            total_size: 0,
        };

        for entry in std::fs::read_dir(&chain.path)? {
            let Ok(file_path) = entry.map(|e| e.path()) else {
                continue;
            };

            let Some(file) = repository::File::from_path(&file_path) else {
                continue;
            };

            let Ok(size) = std::fs::metadata(file.path()).map(|m| m.len()) else {
                continue;
            };

            chain.total_size += size;
            chain.file_sizes.insert(file.clone(), size);
            chain.inner.insert_file(file);
        }

        Ok(chain)
    }

    pub(super) fn tail_seqnum(&self) -> Result<u64> {
        let Some(file) = self.inner.back() else {
            return Ok(0);
        };

        let window_size = 4096;
        let jf = JournalFile::<Mmap>::open(file, window_size)?;

        Ok(jf.journal_header_ref().tail_entry_seqnum)
    }

    pub(super) fn tail_realtime(&self) -> Result<Option<Microseconds>> {
        let Some(file) = self.inner.back() else {
            return Ok(None);
        };

        let window_size = 4096;
        let jf = JournalFile::<Mmap>::open(file, window_size)?;

        let realtime = jf.journal_header_ref().tail_entry_realtime;
        if realtime == 0 {
            Ok(None)
        } else {
            Ok(Some(Microseconds::new(realtime)))
        }
    }

    /// Registers a new journal file with the directory.
    pub(super) fn create_file(
        &mut self,
        seqnum_id: Uuid,
        head_seqnum: u64,
        head_realtime: u64,
    ) -> Result<repository::File> {
        let Some(file) = create_chain_file(&self.path, seqnum_id, head_seqnum, head_realtime)
        else {
            return Err(WriterError::FileCreation(format!(
                "failed to create journal file in {}",
                self.path.display()
            )));
        };
        self.inner.insert_file(file.clone());
        Ok(file)
    }

    /// Updates the tracked size of a file in the chain
    pub(super) fn update_file_size(&mut self, file: &File, new_size: u64) {
        let old_size = self.file_sizes.get(file).copied().unwrap_or(0);
        self.file_sizes.insert(file.clone(), new_size);
        self.total_size = self
            .total_size
            .saturating_sub(old_size)
            .saturating_add(new_size);
    }

    /// Retains the files that satisfy retention policy limits.
    #[tracing::instrument(skip_all, fields(reason))]
    pub(super) fn retain(&mut self, retention_policy: &RetentionPolicy) -> Result<()> {
        // Remove by file count limit
        if let Some(max_files) = retention_policy.number_of_journal_files {
            while self.inner.len() > max_files {
                let reason = format!("num_files({}) > max_files({})", self.inner.len(), max_files);
                tracing::Span::current().record("reason", reason);
                self.delete_oldest_file()?;
            }
        }

        // Remove by total size limit
        if let Some(max_total_size) = retention_policy.size_of_journal_files {
            while self.total_size > max_total_size && !self.inner.is_empty() {
                let reason = format!(
                    "total_size({}) > max_size({})",
                    self.total_size, max_total_size
                );
                tracing::Span::current().record("reason", reason);
                self.delete_oldest_file()?;
            }
        }

        // Remove by entry age limit
        if let Some(max_entry_age) = retention_policy.duration_of_journal_files {
            self.delete_files_older_than(max_entry_age)?;
        }

        Ok(())
    }

    /// Remove the oldest file
    #[tracing::instrument(skip_all)]
    fn delete_oldest_file(&mut self) -> Result<()> {
        let Some(file) = self.inner.pop_front() else {
            return Ok(());
        };

        info!("deleting {}", file.path());

        let file_size = self.file_sizes.get(&file).copied().unwrap_or(0);

        // Remove from filesystem
        if let Err(e) = std::fs::remove_file(file.path()) {
            // Log error but continue cleanup - file might already be deleted
            error!("failed to remove journal file {:?}: {}", file.path(), e);
        }

        self.file_sizes.remove(&file);
        self.total_size = self.total_size.saturating_sub(file_size);
        Ok(())
    }

    /// Remove files older than the specified cutoff time
    #[tracing::instrument(skip(self))]
    fn delete_files_older_than(&mut self, max_entry_age: std::time::Duration) -> Result<()> {
        let cutoff_time = Microseconds::now()
            .get()
            .saturating_sub(max_entry_age.as_micros() as u64);

        for file in self.inner.drain(cutoff_time) {
            info!("deleting {}", file.path());
            let file_size = self.file_sizes.get(&file).copied().unwrap_or(0);

            if let Err(e) = std::fs::remove_file(file.path()) {
                error!("failed to remove journal file {:?}: {}", file.path(), e);
                continue;
            }

            self.file_sizes.remove(&file);
            self.total_size = self.total_size.saturating_sub(file_size);
        }

        Ok(())
    }
}
