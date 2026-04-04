use std::path::{Path, PathBuf};

use uuid::Uuid;

use crate::types::FileId;

const WAL_EXT: &str = "wal";

/// A WAL directory handle.
///
/// Owns the directory path and the machine/boot identity. All `FileId`s
/// produced by this handle carry its machine and boot IDs.
#[derive(Clone)]
pub struct WalDir {
    path: PathBuf,
    machine_id: Uuid,
    boot_id: Uuid,
}

impl WalDir {
    pub fn new(path: &Path, machine_id: Uuid, boot_id: Uuid) -> Self {
        Self {
            path: path.to_path_buf(),
            machine_id,
            boot_id,
        }
    }

    pub fn machine_id(&self) -> Uuid {
        self.machine_id
    }

    pub fn boot_id(&self) -> Uuid {
        self.boot_id
    }

    pub fn path(&self) -> &Path {
        &self.path
    }

    /// Derive the on-disk path for a WAL file.
    pub fn wal_path(&self, id: FileId) -> PathBuf {
        debug_assert_eq!(id.machine_id, self.machine_id, "FileId from wrong machine");
        debug_assert_eq!(id.boot_id, self.boot_id, "FileId from wrong boot");
        self.path.join(id.to_filename(WAL_EXT))
    }

    /// Parse a path into a `FileId`, if it matches the WAL filename format.
    pub fn parse(path: &Path) -> Option<FileId> {
        let name = path.file_name()?.to_str()?;
        let stem = name.strip_suffix(&format!(".{WAL_EXT}"))?;
        FileId::parse_stem(stem)
    }

    /// Scan the directory for the highest existing sequence number.
    ///
    /// The sequence number is monotonically increasing across boots, so all
    /// files in the directory are considered regardless of their origin.
    /// Every file in this directory must be a valid WAL file — an unparseable
    /// filename is treated as an error.
    pub fn scan_max_sequence(&self) -> crate::Result<u64> {
        let mut max_seq: u64 = 0;
        let entries = std::fs::read_dir(&self.path)?;
        for entry in entries {
            let entry = entry?;
            let path = entry.path();
            let id = Self::parse(&path).ok_or_else(|| {
                crate::Error::InvalidHeader(
                    format!("unparseable WAL filename: {}", path.display(),),
                )
            })?;
            max_seq = max_seq.max(id.seq);
        }
        Ok(max_seq)
    }
}
