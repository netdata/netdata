use std::fs;
use std::path::{Path, PathBuf};

use file_registry::{ByteSize, FileDir, FileId, FileRegistry, Query, TimestampNs};

use crate::format::{FileEvent, HEADER_SIZE};
use crate::{Error, Result};

pub(crate) const WAL_EXT: &str = "wal";

/// Lifecycle status of a WAL file.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum FileStatus {
    /// The writer is actively writing to this file.
    Active,
    /// The writer has finished writing; the file is immutable.
    Archived,
}

/// A WAL file tracked by the registry.
///
/// `min_timestamp_ns` / `max_timestamp_ns` are the **log-data** time
/// range of the records written into the file (per the OTel hierarchy,
/// `time_unix_nano` → `observed_time_unix_nano`). They're populated
/// incrementally from `FileEvent::Synced` while the file is `Active`,
/// and finalized by `FileEvent::Closed` once it's `Archived`.
///
/// On recovery (registry rebuilt from disk), these fields are left at
/// `TimestampNs::ZERO` — the WAL file format does not yet carry a
/// summary footer, so the values can only come from in-process events.
/// A re-index of the WAL produces an SFST whose summary has the
/// authoritative range.
///
/// `#[non_exhaustive]`: this entry has grown fields over time
/// (`valid_up_to`, `entry_count`) and will likely grow more; external
/// crates read its fields but never construct it, so marking it spares
/// them a breaking change on the next addition.
#[derive(Debug, Clone)]
#[non_exhaustive]
pub struct File {
    pub id: FileId,
    pub status: FileStatus,
    pub created_at_ns: TimestampNs,
    pub size: ByteSize,
    /// Opaque content-plane identity blob, recovered cheaply from the WAL
    /// header (no frame decode). The content plane decodes it to name the
    /// stream in enumeration / the query-side selector; the WAL never parses it.
    /// The partition key lives in `id` (the filename `FileId`), not here.
    pub content_meta: Vec<u8>,
    pub min_timestamp_ns: TimestampNs,
    pub max_timestamp_ns: TimestampNs,
    /// Byte offset of the durable, fully-written prefix — the end of the
    /// last frame fsynced to disk (the `valid_up_to` of the most recent
    /// `Synced` event). Frame-aligned by construction. A concurrent
    /// reader of an actively-written file must not read past this offset:
    /// the writer's buffer can flush mid-frame, so the bytes beyond it may
    /// be a torn frame. `ByteSize::ZERO` means "unknown" — a file recovered
    /// from disk (no in-process event history; the format carries no
    /// footer) or one not yet synced. For a sealed (`Archived`) file the
    /// final `Synced` set this equal to `size`.
    pub valid_up_to: ByteSize,
    /// Number of log records in the durable prefix (the `entry_count` of
    /// the most recent `Synced` event). `0` after recovery (unknown).
    /// A bounded read of `[HEADER, valid_up_to)` must decode exactly this
    /// many records — the cross-check that the prefix wasn't truncated.
    pub entry_count: u64,
}

/// An ordered collection of WAL files.
///
/// Files are keyed by sequence number, which provides chronological ordering.
pub struct Registry {
    files: FileRegistry<File>,
}

impl Registry {
    pub fn new(path: &Path) -> Self {
        Self {
            files: FileRegistry::new(FileDir::new(path, WAL_EXT)),
        }
    }

    /// Recovers registry state by scanning the directory and reading file headers.
    pub fn recover(&mut self) -> Result<()> {
        let entries = self.files.dir().scan()?;

        for (file_id, meta) in entries {
            let path = self.files.file_path(file_id);

            let header = match read_header(&path) {
                Ok(h) => h,
                Err(e) => {
                    tracing::error!("failed to read WAL header {}: {e}", path.display());
                    continue;
                }
            };

            let size = ByteSize(meta.len());

            self.files.insert(
                file_id.seq,
                File {
                    id: file_id,
                    status: FileStatus::Archived,
                    created_at_ns: TimestampNs(header.created_at),
                    size,
                    // Recovered cheaply from the header.
                    content_meta: header.content_meta,
                    // Recovery cannot retrieve log-data range from the
                    // WAL file format today. Re-indexing populates the
                    // SFST summary with the authoritative values.
                    min_timestamp_ns: TimestampNs::ZERO,
                    max_timestamp_ns: TimestampNs::ZERO,
                    // Likewise unknown without event history: a crash may
                    // have left a torn tail past the last sync, and the
                    // file carries no durable-prefix marker. ZERO = "do
                    // not trust a byte bound"; such files are re-indexed
                    // whole by the existing pipeline.
                    valid_up_to: ByteSize::ZERO,
                    entry_count: 0,
                },
            );
        }

        Ok(())
    }

    pub fn path(&self) -> &Path {
        self.files.dir().path()
    }

    /// Derive the on-disk path for a WAL file.
    pub fn file_path(&self, id: FileId) -> PathBuf {
        self.files.file_path(id)
    }

    /// Scan the directory for the highest existing sequence number.
    pub fn scan_max_sequence(&self) -> Result<u64> {
        Ok(self.files.dir().scan_max_sequence()?)
    }

    /// Applies a `FileEvent` from the writer.
    pub fn apply_event(&mut self, event: &FileEvent) -> Result<()> {
        match event {
            FileEvent::Created {
                file_id,
                created_at_ns,
                content_meta,
            } => {
                if self.files.contains(file_id.seq) {
                    return Err(Error::DuplicateSequence(file_id.seq));
                }
                self.files.insert(
                    file_id.seq,
                    File {
                        id: *file_id,
                        status: FileStatus::Active,
                        created_at_ns: *created_at_ns,
                        size: ByteSize::ZERO,
                        content_meta: content_meta.clone(),
                        min_timestamp_ns: TimestampNs::ZERO,
                        max_timestamp_ns: TimestampNs::ZERO,
                        valid_up_to: ByteSize::ZERO,
                        entry_count: 0,
                    },
                );
                Ok(())
            }
            FileEvent::Synced {
                file_id,
                valid_up_to,
                entry_count,
                min_timestamp_ns,
                max_timestamp_ns,
                ..
            } => {
                // The event carries the writer's current accumulator
                // state (not a delta), so a direct overwrite is correct.
                let entry = self
                    .files
                    .get_mut(file_id.seq)
                    .ok_or(Error::UnknownSequence(file_id.seq))?;
                entry.min_timestamp_ns = *min_timestamp_ns;
                entry.max_timestamp_ns = *max_timestamp_ns;
                entry.valid_up_to = *valid_up_to;
                entry.entry_count = *entry_count;
                Ok(())
            }
            FileEvent::Closed {
                file_id,
                size,
                min_timestamp_ns,
                max_timestamp_ns,
                ..
            } => {
                let entry = self
                    .files
                    .get_mut(file_id.seq)
                    .ok_or(Error::UnknownSequence(file_id.seq))?;
                entry.status = FileStatus::Archived;
                entry.size = *size;
                entry.min_timestamp_ns = *min_timestamp_ns;
                entry.max_timestamp_ns = *max_timestamp_ns;
                Ok(())
            }
        }
    }

    /// Look up a file by sequence number.
    pub fn get(&self, seq: u64) -> Option<&File> {
        self.files.get(seq)
    }

    /// Removes a file by sequence number.
    pub fn remove_by_seq(&mut self, seq: u64) -> Option<File> {
        self.files.remove(seq)
    }

    /// Returns all archived files, ordered by sequence number.
    pub fn archived_files(&self) -> impl Iterator<Item = &File> {
        self.files
            .values()
            .filter(|f| f.status == FileStatus::Archived)
    }

    /// Every tracked file (active and archived), ordered by sequence
    /// number. Used by stream enumeration to list a tenant's streams from
    /// the per-file `stream` recorded in the header — including streams
    /// that exist only as an unsealed WAL with no SFST summary yet.
    pub fn values(&self) -> impl Iterator<Item = &File> {
        self.files.values()
    }

    /// Files in the registry whose log-data range intersects `q`.
    ///
    /// Pure filter — does not open any WAL file. Both `Active` (currently
    /// being written) and `Archived` (sealed, awaiting indexing) files
    /// are included; Active files are how the planner reaches real-time
    /// data not yet flushed into an SFST.
    ///
    /// Files with `min_timestamp_ns == ZERO` are skipped: such a file
    /// either hasn't received its first `Synced` event yet (a sub-second
    /// window between `Created` and the first sync of a brand-new
    /// stream) or was recovered from disk after a process restart
    /// (recovery cannot retrieve the log-data range from the WAL format
    /// today). Either way the planner has no authoritative range to
    /// filter against. The same `Archived(ZERO, ZERO)` case from a
    /// failed `recover_unindexed` is impossible in steady state because
    /// the ledger refuses to start when indexing fails.
    ///
    /// Partition filter is matched against the file's `id.part_key`
    /// (one key per `(namespace, name)` pair); equivalent to comparing
    /// canonical stream identities given the ingestor's collision-
    /// detection invariant.
    pub fn candidates<'a>(&'a self, q: &Query) -> impl Iterator<Item = &'a File> + 'a {
        // Extract q's contents upfront so the filter closures don't borrow
        // q. This decouples the iterator's lifetime from q's, letting
        // callers pass a temporary `Query` without binding it to a local.
        let q_min_ns = (q.time_range.start as u64) * 1_000_000_000;
        let q_max_ns = (q.time_range.end as u64) * 1_000_000_000;
        let partition_keys = q.partition_keys.clone();

        self.files
            .values()
            .filter(|f| f.min_timestamp_ns != TimestampNs::ZERO)
            .filter(move |f| range_overlaps_ns(f, q_min_ns, q_max_ns))
            .filter(move |f| {
                partition_keys.is_empty() || partition_keys.contains(&f.id.part_key)
            })
    }

    pub fn len(&self) -> usize {
        self.files.len()
    }

    pub fn is_empty(&self) -> bool {
        self.files.is_empty()
    }
}

/// True iff the file's nanosecond range `[min, max]` (inclusive on both
/// ends) overlaps the query's `[q_start_ns, q_end_ns)` (half-open) —
/// the shared [`file_registry::range_overlaps`] rule in nanoseconds.
fn range_overlaps_ns(file: &File, q_start_ns: u64, q_end_ns: u64) -> bool {
    file_registry::range_overlaps(
        &(q_start_ns..q_end_ns),
        file.min_timestamp_ns.0,
        file.max_timestamp_ns.0,
    )
}

/// Read and parse the WAL file header.
fn read_header(path: &std::path::Path) -> Result<crate::format::FileHeader> {
    use std::io::Read;
    let mut file = fs::File::open(path)?;
    let mut buf = [0u8; HEADER_SIZE];
    file.read_exact(&mut buf)?;
    Ok(crate::format::FileHeader::from_bytes(&buf)?)
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::format::FileEvent;
    use crate::{Config, RotationConfig, Writer};

    fn test_file_id(seq: u64) -> FileId {
        let machine_id = uuid::Uuid::try_parse("550e8400e29b41d4a716446655440000").unwrap();
        let boot_id = uuid::Uuid::try_parse("7f3b2a1e9c4d4f8ab1c2d3e4f5a6b7c8").unwrap();
        FileId::new(machine_id, boot_id, seq, 0)
    }

    /// Helper: create a Writer, write entries, shutdown, and return all events.
    fn write_wal_files(dir: &std::path::Path, entry_counts: &[usize]) -> Vec<FileEvent> {
        let entries_per_file: usize = *entry_counts.iter().max().unwrap_or(&10);
        let config = Config {
            rotation: RotationConfig {
                max_log_entries: entries_per_file,
                max_file_size: ByteSize(u64::MAX),
                max_duration: None,
            },
            crc_enabled: false,
            compression_enabled: true,
        };
        let seq = std::sync::Arc::new(crate::SeqAllocator::ephemeral(0));
        let mut writer = Writer::new(dir, config, seq).unwrap();
        let mut all_events = Vec::new();
        for &count in entry_counts {
            for i in 0..count {
                writer
                    .write_frame(crate::opaque_part_key("ns", "svc"), &[],
                        &(i as u32).to_le_bytes(),
                        1,
                        TimestampNs(i as u64 + 1),
                        TimestampNs::ZERO,
                        TimestampNs::ZERO,
                    )
                    .unwrap();
            }
            all_events.extend(writer.take_all_events());
        }
        all_events.extend(writer.shutdown_all().unwrap());
        all_events
    }

    #[test]
    fn apply_events_tracks_files() {
        let dir = tempfile::tempdir().unwrap();
        let events = write_wal_files(dir.path(), &[10, 10, 10]);

        let mut registry = Registry::new(dir.path());
        registry.recover().unwrap();
        // recover finds all files as Archived; clear them to test apply_event from scratch
        for seq in [1u64, 2, 3] {
            registry.remove_by_seq(seq);
        }

        for event in &events {
            registry.apply_event(event).unwrap();
        }

        assert_eq!(registry.len(), 3);
        assert!(registry.archived_files().count() == 3);

        let seqs: Vec<u64> = registry.archived_files().map(|f| f.id.seq).collect();
        assert_eq!(seqs, vec![1, 2, 3]);
    }

    #[test]
    fn recover_from_directory() {
        let dir = tempfile::tempdir().unwrap();
        let _ = write_wal_files(dir.path(), &[10, 10]);

        let mut registry = Registry::new(dir.path());
        registry.recover().unwrap();
        assert_eq!(registry.len(), 2);
        assert_eq!(registry.archived_files().count(), 2);

        let seqs: Vec<u64> = registry.archived_files().map(|f| f.id.seq).collect();
        assert_eq!(seqs, vec![1, 2]);

        // Recovery reads the partition key from the filename (FileId), not the
        // header — since v4 the header no longer stores part_key.
        assert!(
            registry
                .archived_files()
                .all(|f| f.id.part_key == crate::opaque_part_key("ns", "svc")),
            "recovery must recover each file's partition key from its filename (FileId)"
        );
    }

    #[test]
    fn remove_by_seq() {
        let dir = tempfile::tempdir().unwrap();
        let _events = write_wal_files(dir.path(), &[10, 10]);

        let mut registry = Registry::new(dir.path());
        registry.recover().unwrap();
        assert_eq!(registry.len(), 2);

        let removed = registry.remove_by_seq(1).unwrap();
        assert_eq!(removed.id.seq, 1);
        assert_eq!(registry.len(), 1);
    }

    #[test]
    fn apply_event_tracks_log_ts_range() {
        let dir = tempfile::tempdir().unwrap();
        let mut registry = Registry::new(dir.path());
        let id = test_file_id(7);

        registry
            .apply_event(&FileEvent::Created {
                file_id: id,
                created_at_ns: TimestampNs(1),
                content_meta: Vec::new(),
            })
            .unwrap();
        // Created starts at ZERO/ZERO.
        let f = registry.get(7).unwrap();
        assert_eq!(f.min_timestamp_ns, TimestampNs::ZERO);
        assert_eq!(f.max_timestamp_ns, TimestampNs::ZERO);

        // First Synced sets the range.
        registry
            .apply_event(&FileEvent::Synced {
                file_id: id,
                valid_up_to: ByteSize(100),
                frame_count: 1,
                entry_count: 5,
                min_timestamp_ns: TimestampNs(200),
                max_timestamp_ns: TimestampNs(300),
            })
            .unwrap();
        let f = registry.get(7).unwrap();
        assert_eq!(f.min_timestamp_ns, TimestampNs(200));
        assert_eq!(f.max_timestamp_ns, TimestampNs(300));

        // Second Synced overwrites with the writer's current accumulator
        // state — wider range now.
        registry
            .apply_event(&FileEvent::Synced {
                file_id: id,
                valid_up_to: ByteSize(200),
                frame_count: 2,
                entry_count: 10,
                min_timestamp_ns: TimestampNs(150),
                max_timestamp_ns: TimestampNs(400),
            })
            .unwrap();
        let f = registry.get(7).unwrap();
        assert_eq!(f.min_timestamp_ns, TimestampNs(150));
        assert_eq!(f.max_timestamp_ns, TimestampNs(400));

        // Closed finalizes.
        registry
            .apply_event(&FileEvent::Closed {
                file_id: id,
                frame_count: 2,
                min_timestamp_ns: TimestampNs(150),
                max_timestamp_ns: TimestampNs(400),
                size: ByteSize(200),
            })
            .unwrap();
        let f = registry.get(7).unwrap();
        assert_eq!(f.status, FileStatus::Archived);
        assert_eq!(f.min_timestamp_ns, TimestampNs(150));
        assert_eq!(f.max_timestamp_ns, TimestampNs(400));
    }

    #[test]
    fn apply_event_tracks_valid_up_to_and_entry_count() {
        let dir = tempfile::tempdir().unwrap();
        let mut registry = Registry::new(dir.path());
        let id = test_file_id(3);

        registry
            .apply_event(&FileEvent::Created {
                file_id: id,
                created_at_ns: TimestampNs(1),
                content_meta: Vec::new(),
            })
            .unwrap();
        // Created: durable prefix unknown.
        let f = registry.get(3).unwrap();
        assert_eq!(f.valid_up_to, ByteSize::ZERO);
        assert_eq!(f.entry_count, 0);

        // Synced carries the current durable prefix and record count.
        registry
            .apply_event(&FileEvent::Synced {
                file_id: id,
                valid_up_to: ByteSize(4608),
                frame_count: 1,
                entry_count: 12,
                min_timestamp_ns: TimestampNs(100),
                max_timestamp_ns: TimestampNs(200),
            })
            .unwrap();
        let f = registry.get(3).unwrap();
        assert_eq!(f.valid_up_to, ByteSize(4608));
        assert_eq!(f.entry_count, 12);

        // A later Synced overwrites with the grown prefix.
        registry
            .apply_event(&FileEvent::Synced {
                file_id: id,
                valid_up_to: ByteSize(8704),
                frame_count: 2,
                entry_count: 30,
                min_timestamp_ns: TimestampNs(100),
                max_timestamp_ns: TimestampNs(300),
            })
            .unwrap();
        let f = registry.get(3).unwrap();
        assert_eq!(f.valid_up_to, ByteSize(8704));
        assert_eq!(f.entry_count, 30);

        // Closed carries no prefix fields; the final Synced's values
        // (which equal the file size for a sealed file) are preserved.
        registry
            .apply_event(&FileEvent::Closed {
                file_id: id,
                frame_count: 2,
                min_timestamp_ns: TimestampNs(100),
                max_timestamp_ns: TimestampNs(300),
                size: ByteSize(8704),
            })
            .unwrap();
        let f = registry.get(3).unwrap();
        assert_eq!(f.status, FileStatus::Archived);
        assert_eq!(f.valid_up_to, ByteSize(8704));
        assert_eq!(f.entry_count, 30);
    }

    #[test]
    fn recovered_files_have_unknown_durable_prefix() {
        let dir = tempfile::tempdir().unwrap();
        let _ = write_wal_files(dir.path(), &[10]);

        let mut registry = Registry::new(dir.path());
        registry.recover().unwrap();
        let f = registry.archived_files().next().unwrap();
        // No event history on recovery: durable prefix is unknown, so a
        // bounded read must not trust a byte bound from it.
        assert_eq!(f.valid_up_to, ByteSize::ZERO);
        assert_eq!(f.entry_count, 0);
    }

    #[test]
    fn active_then_archived() {
        let dir = tempfile::tempdir().unwrap();
        let id = test_file_id(1);

        let mut registry = Registry::new(dir.path());
        registry.recover().unwrap();

        registry
            .apply_event(&FileEvent::Created {
                file_id: id,
                created_at_ns: TimestampNs(1_000_000_000),
                content_meta: Vec::new(),
            })
            .unwrap();

        // Active files are not in archived_files
        assert_eq!(registry.len(), 1);
        assert_eq!(registry.archived_files().count(), 0);

        registry
            .apply_event(&FileEvent::Closed {
                file_id: id,
                frame_count: 1,
                min_timestamp_ns: TimestampNs(1_000_000_000),
                max_timestamp_ns: TimestampNs(1_000_000_000),
                size: ByteSize(4096),
            })
            .unwrap();

        assert_eq!(registry.archived_files().count(), 1);
    }

    // ── candidates() tests ───────────────────────────────────────

    fn fid_with(seq: u64, part_key: u64) -> FileId {
        let machine_id = uuid::Uuid::try_parse("550e8400e29b41d4a716446655440000").unwrap();
        let boot_id = uuid::Uuid::try_parse("7f3b2a1e9c4d4f8ab1c2d3e4f5a6b7c8").unwrap();
        FileId::new(machine_id, boot_id, seq, part_key)
    }

    /// Insert a file via the event flow with the given (min, max) range
    /// in nanoseconds and the requested status.
    fn track(
        reg: &mut Registry,
        seq: u64,
        part_key: u64,
        min_ns: u64,
        max_ns: u64,
        status: FileStatus,
    ) -> FileId {
        let id = fid_with(seq, part_key);
        reg.apply_event(&FileEvent::Created {
            file_id: id,
            created_at_ns: TimestampNs(0),
            content_meta: Vec::new(),
        })
        .unwrap();
        match status {
            FileStatus::Active => {
                reg.apply_event(&FileEvent::Synced {
                    file_id: id,
                    valid_up_to: ByteSize(0),
                    frame_count: 0,
                    entry_count: 0,
                    min_timestamp_ns: TimestampNs(min_ns),
                    max_timestamp_ns: TimestampNs(max_ns),
                })
                .unwrap();
            }
            FileStatus::Archived => {
                reg.apply_event(&FileEvent::Closed {
                    file_id: id,
                    frame_count: 0,
                    min_timestamp_ns: TimestampNs(min_ns),
                    max_timestamp_ns: TimestampNs(max_ns),
                    size: ByteSize(0),
                })
                .unwrap();
            }
        }
        id
    }

    fn seqs<'a>(iter: impl Iterator<Item = &'a File>) -> Vec<u64> {
        let mut v: Vec<u64> = iter.map(|f| f.id.seq).collect();
        v.sort();
        v
    }

    /// Convert a seconds-since-epoch value to nanoseconds for fixture
    /// readability.
    const NS: u64 = 1_000_000_000;

    #[test]
    fn candidates_filter_by_time_range_overlap() {
        let dir = tempfile::tempdir().unwrap();
        let mut reg = Registry::new(dir.path());

        track(&mut reg, 1, 7, 100 * NS, 200 * NS, FileStatus::Archived);
        track(&mut reg, 2, 7, 300 * NS, 400 * NS, FileStatus::Archived);
        track(&mut reg, 3, 7, 150 * NS, 350 * NS, FileStatus::Archived);

        let q = Query {
            time_range: 50..250,
            partition_keys: Vec::new(),
        };
        assert_eq!(seqs(reg.candidates(&q)), vec![1, 3]);
    }

    #[test]
    fn candidates_inclusive_lower_exclusive_upper() {
        let dir = tempfile::tempdir().unwrap();
        let mut reg = Registry::new(dir.path());

        track(&mut reg, 1, 7, 100 * NS, 200 * NS, FileStatus::Archived);
        track(&mut reg, 2, 7, 200 * NS, 300 * NS, FileStatus::Archived);
        track(&mut reg, 3, 7, 300 * NS, 400 * NS, FileStatus::Archived);

        // Query [200, 300):
        // - file 1: max=200 ≥ 200 (inclusive lower) and min=100 < 300 → in
        // - file 2: max=300 ≥ 200 and min=200 < 300 → in
        // - file 3: min=300, but query.end=300 (exclusive) → out
        let q = Query {
            time_range: 200..300,
            partition_keys: Vec::new(),
        };
        assert_eq!(seqs(reg.candidates(&q)), vec![1, 2]);
    }

    #[test]
    fn candidates_empty_query_matches_nothing() {
        let dir = tempfile::tempdir().unwrap();
        let mut reg = Registry::new(dir.path());
        track(&mut reg, 1, 7, 100 * NS, 200 * NS, FileStatus::Archived);

        let q = Query {
            time_range: 200..200,
            partition_keys: Vec::new(),
        };
        assert!(reg.candidates(&q).next().is_none());
    }

    #[test]
    fn candidates_filter_by_part_key() {
        // The WAL filter matches files by their opaque `part_key` (the content
        // plane's stream-hash collapse — absent vs empty namespace — is the
        // content plane's concern, covered by otel-logs-identity's tests).
        let dir = tempfile::tempdir().unwrap();
        let mut reg = Registry::new(dir.path());

        let api = crate::opaque_part_key("prod", "api");
        let worker = crate::opaque_part_key("prod", "worker");
        track(&mut reg, 1, api, 100 * NS, 200 * NS, FileStatus::Archived);
        track(&mut reg, 2, worker, 100 * NS, 200 * NS, FileStatus::Archived);
        track(&mut reg, 3, api, 100 * NS, 200 * NS, FileStatus::Active);

        let q = Query {
            time_range: 0..u32::MAX,
            partition_keys: vec![api],
        };
        assert_eq!(seqs(reg.candidates(&q)), vec![1, 3]);
    }

    #[test]
    fn candidates_skip_files_with_zero_min_ts() {
        let dir = tempfile::tempdir().unwrap();
        let mut reg = Registry::new(dir.path());

        // File with zero min — its first Synced event hasn't happened
        // yet (or this is a recovery-from-disk file with no in-process
        // event history). Must be excluded.
        let id_zero = fid_with(1, 7);
        reg.apply_event(&FileEvent::Created {
            file_id: id_zero,
            created_at_ns: TimestampNs(0),
            content_meta: Vec::new(),
        })
        .unwrap();

        // File with a real range — must be included.
        track(&mut reg, 2, 7, 100 * NS, 200 * NS, FileStatus::Active);

        let q = Query {
            time_range: 0..u32::MAX,
            partition_keys: Vec::new(),
        };
        assert_eq!(seqs(reg.candidates(&q)), vec![2]);
    }

    #[test]
    fn candidates_includes_active_and_archived() {
        let dir = tempfile::tempdir().unwrap();
        let mut reg = Registry::new(dir.path());

        track(&mut reg, 1, 7, 100 * NS, 200 * NS, FileStatus::Active);
        track(&mut reg, 2, 7, 100 * NS, 200 * NS, FileStatus::Archived);

        let q = Query {
            time_range: 0..u32::MAX,
            partition_keys: Vec::new(),
        };
        assert_eq!(seqs(reg.candidates(&q)), vec![1, 2]);
    }

    #[test]
    fn candidates_on_empty_registry() {
        let dir = tempfile::tempdir().unwrap();
        let reg = Registry::new(dir.path());
        let q = Query {
            time_range: 0..u32::MAX,
            partition_keys: Vec::new(),
        };
        assert!(reg.candidates(&q).next().is_none());
    }

    #[test]
    fn duplicate_sequence_rejected() {
        let dir = tempfile::tempdir().unwrap();
        let id = test_file_id(1);

        let mut registry = Registry::new(dir.path());
        registry.recover().unwrap();

        registry
            .apply_event(&FileEvent::Created {
                file_id: id,
                created_at_ns: TimestampNs(1_000_000_000),
                content_meta: Vec::new(),
            })
            .unwrap();
        let err = registry
            .apply_event(&FileEvent::Created {
                file_id: id,
                created_at_ns: TimestampNs(2_000_000_000),
                content_meta: Vec::new(),
            })
            .unwrap_err();
        assert!(matches!(err, Error::DuplicateSequence(1)));
    }
}
