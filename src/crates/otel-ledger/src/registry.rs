use std::collections::BTreeMap;
use std::path::{Path, PathBuf};
use std::time::{SystemTime, UNIX_EPOCH};

use wal::{ByteSize, FileId, TimestampNs, WalDir, WalRegistry};

const INDEX_EXT: &str = "sfst";

// ---------------------------------------------------------------------------
// Index files (.sfst)
// ---------------------------------------------------------------------------

#[derive(Debug, Clone)]
pub struct IndexFile {
    pub id: FileId,
    pub created_at_ns: TimestampNs,
    pub size: ByteSize,
    pending_deletion: bool,
}

pub struct IndexRegistry {
    dir: PathBuf,
    files: BTreeMap<u64, IndexFile>,
}

impl IndexRegistry {
    pub fn new(dir: PathBuf) -> Self {
        Self {
            dir,
            files: BTreeMap::new(),
        }
    }

    pub fn dir(&self) -> &Path {
        &self.dir
    }

    /// Derive the on-disk path for an index file from its FileId.
    pub fn path(&self, id: FileId) -> PathBuf {
        self.dir.join(id.to_filename(INDEX_EXT))
    }

    /// Scan the directory for `.sfst` files and reconstruct state.
    pub fn recover(dir: &Path) -> Self {
        let mut registry = Self::new(dir.to_path_buf());

        let entries = match std::fs::read_dir(dir) {
            Ok(e) => e,
            Err(_) => return registry,
        };

        for dir_entry in entries.flatten() {
            let path = dir_entry.path();

            if path.extension().and_then(|e| e.to_str()) != Some(INDEX_EXT) {
                continue;
            }

            let Some(id) = FileId::parse(&path) else {
                tracing::warn!(
                    "skipping index file with unparseable name: {}",
                    path.display()
                );
                continue;
            };

            let meta = match std::fs::metadata(&path) {
                Ok(m) => m,
                Err(e) => {
                    tracing::error!("failed to stat index file {}: {e}", path.display());
                    continue;
                }
            };

            let size = ByteSize(meta.len());

            // Use the file's modification time as an approximation for
            // creation time. The actual WAL `created_at_ns` is not available
            // when the .wal has already been deleted.
            let created_at_ns = TimestampNs(
                meta.modified()
                    .unwrap_or(SystemTime::UNIX_EPOCH)
                    .duration_since(UNIX_EPOCH)
                    .unwrap_or_default()
                    .as_nanos() as u64,
            );

            registry.files.insert(
                id.seq,
                IndexFile {
                    id,
                    created_at_ns,
                    size,
                    pending_deletion: false,
                },
            );
        }

        registry
    }

    pub fn track(&mut self, id: FileId, created_at_ns: TimestampNs, size: ByteSize) {
        self.files.insert(
            id.seq,
            IndexFile {
                id,
                created_at_ns,
                size,
                pending_deletion: false,
            },
        );
    }

    pub fn remove(&mut self, seq: u64) -> Option<IndexFile> {
        self.files.remove(&seq)
    }

    pub fn mark_pending_deletion(&mut self, seq: u64) {
        if let Some(entry) = self.files.get_mut(&seq) {
            entry.pending_deletion = true;
        }
    }

    pub fn clear_pending_deletion(&mut self, seq: u64) {
        if let Some(entry) = self.files.get_mut(&seq) {
            entry.pending_deletion = false;
        }
    }

    pub fn get(&self, seq: u64) -> Option<&IndexFile> {
        self.files.get(&seq)
    }

    pub fn len(&self) -> usize {
        self.files.len()
    }

    pub fn is_empty(&self) -> bool {
        self.files.is_empty()
    }

    /// Evaluate the retention policy and return sequences of files to evict.
    ///
    /// Only files that are not already pending deletion are considered.
    /// Files are evaluated oldest-first (by sequence number). A file is
    /// marked for eviction if any limit is exceeded.
    pub fn evaluate_retention(
        &self,
        retention: &bridge::config::RetentionConfig,
        now_ns: u64,
    ) -> Vec<u64> {
        let max_files = retention.max_files;
        let max_total_size = retention.max_total_size.as_u64();
        let max_age_ns = retention.max_age.as_nanos() as u64;

        let eligible: Vec<&IndexFile> = self
            .files
            .values()
            .filter(|f| !f.pending_deletion)
            .collect();

        let total_files = eligible.len();
        let total_size: u64 = eligible.iter().map(|f| f.size.as_u64()).sum();

        let mut to_evict = Vec::new();
        let mut remaining_files = total_files;
        let mut remaining_size = total_size;

        for entry in &eligible {
            let mut should_evict = false;

            if remaining_files > max_files {
                should_evict = true;
            }
            if remaining_size > max_total_size {
                should_evict = true;
            }
            if now_ns.saturating_sub(entry.created_at_ns.as_u64()) > max_age_ns {
                should_evict = true;
            }

            if should_evict {
                to_evict.push(entry.id.seq);
                remaining_files -= 1;
                remaining_size -= entry.size.as_u64();
            }
        }

        to_evict
    }
}

// ---------------------------------------------------------------------------
// Remote files (uploaded to object storage)
// ---------------------------------------------------------------------------

#[derive(Debug, Clone)]
pub struct RemoteFile {
    pub id: FileId,
    pub remote_key: String,
    pub uploaded_at_ns: TimestampNs,
}

pub struct RemoteRegistry {
    files: BTreeMap<u64, RemoteFile>,
}

impl RemoteRegistry {
    pub fn new() -> Self {
        Self {
            files: BTreeMap::new(),
        }
    }

    /// Recover remote state by listing files in object storage.
    ///
    /// Returns `Ok` with the recovered registry, or `Err` if the remote
    /// is unreachable. The caller should skip upload recovery on failure
    /// — uploads will happen naturally during normal operation once the
    /// remote becomes available.
    pub async fn recover(operator: &opendal::Operator) -> Result<Self, opendal::Error> {
        let mut registry = Self::new();
        let entries = operator.list("").await?;

        for entry in entries {
            let path = entry.path();
            if let Some(id) = FileId::parse(std::path::Path::new(path)) {
                registry.track(id, path.to_string());
            }
        }

        if !registry.is_empty() {
            tracing::info!("recovered {} remote files", registry.len());
        }

        Ok(registry)
    }

    pub fn track(&mut self, id: FileId, remote_key: String) {
        self.files.insert(
            id.seq,
            RemoteFile {
                id,
                remote_key,
                uploaded_at_ns: TimestampNs(
                    SystemTime::now()
                        .duration_since(UNIX_EPOCH)
                        .unwrap_or_default()
                        .as_nanos() as u64,
                ),
            },
        );
    }

    pub fn contains(&self, seq: u64) -> bool {
        self.files.contains_key(&seq)
    }

    pub fn get(&self, seq: u64) -> Option<&RemoteFile> {
        self.files.get(&seq)
    }

    pub fn remove(&mut self, seq: u64) -> Option<RemoteFile> {
        self.files.remove(&seq)
    }

    pub fn len(&self) -> usize {
        self.files.len()
    }

    pub fn is_empty(&self) -> bool {
        self.files.is_empty()
    }
}

// ---------------------------------------------------------------------------
// Composition
// ---------------------------------------------------------------------------

pub struct Registry {
    pub wal: WalRegistry,
    pub index: IndexRegistry,
    pub remote: RemoteRegistry,
}

impl Registry {
    /// Recover registries from disk.
    ///
    /// Cleans up stale `.tmp` files (from interrupted index writes) before
    /// scanning.
    pub fn recover(wal_dir: WalDir, index_dir: &Path) -> Self {
        cleanup_temp_files(index_dir);

        let wal = WalRegistry::recover(wal_dir).unwrap_or_else(|e| {
            tracing::error!("failed to recover WAL registry: {e}");
            panic!("WAL registry recovery failed");
        });
        let index = IndexRegistry::recover(index_dir);
        let remote = RemoteRegistry::new();

        if !wal.is_empty() || !index.is_empty() {
            tracing::info!(
                "recovered files from disk: wal_files={} index_files={}",
                wal.len(),
                index.len(),
            );
        }

        Self { wal, index, remote }
    }

    /// Returns FileIds of archived WAL files that have no corresponding index.
    pub fn unindexed_ids(&self) -> Vec<FileId> {
        self.wal
            .archived_files()
            .filter(|entry| self.index.get(entry.id.seq).is_none())
            .map(|entry| entry.id)
            .collect()
    }

    /// Returns FileIds of indexed files that have not been uploaded to remote storage.
    pub fn unuploaded_ids(&self) -> Vec<FileId> {
        self.index
            .files
            .values()
            .filter(|entry| !self.remote.contains(entry.id.seq))
            .map(|entry| entry.id)
            .collect()
    }
}

fn cleanup_temp_files(dir: &Path) {
    let entries = match std::fs::read_dir(dir) {
        Ok(e) => e,
        Err(_) => return,
    };

    for dir_entry in entries.flatten() {
        let path = dir_entry.path();
        if path.extension().is_some_and(|ext| ext == "tmp") {
            match std::fs::remove_file(&path) {
                Ok(()) => tracing::info!("removed stale tmp file path={}", path.display()),
                Err(e) if e.kind() == std::io::ErrorKind::NotFound => {}
                Err(e) => {
                    tracing::warn!(
                        "failed to remove stale tmp file path={}: {e}",
                        path.display()
                    )
                }
            }
        }
    }
}
