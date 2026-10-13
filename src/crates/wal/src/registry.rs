//! The ledger-side WAL file index: one [`Registry`] per tenant directory,
//! rebuilt from disk at startup and kept current by applying the writer's
//! [`FileEvent`]s as they arrive over the ingestor→ledger link. The writer
//! queues events and the ingestor forwards them; the ledger applies them
//! here, so this file owns event application and the candidate filtering
//! the resulting state feeds.
//!
//! - Event application ([`Registry::apply_event`]): every event re-reports
//!   the writer's current accumulator state (not a delta), so applying one
//!   is an overwrite; `Closed` is authoritative and final — a late `Synced`
//!   is ignored. The per-signal `frame_seq` gap-check runs upstream
//!   (otel-ledger's ingestor), so a genuinely lost event leaves an entry
//!   stale or untracked; it cannot corrupt one. Field semantics are
//!   `format.rs`'s.
//! - Candidate filtering ([`Registry::candidates`]): the pure in-memory
//!   time + partition filter the live query planner and the stream selector
//!   drive. It scales the seconds-based `Query` to the registry's
//!   nanosecond file ranges and re-applies the shared `range_overlaps`
//!   rule. Offline WAL discovery (sfsq-cli) deliberately bypasses it —
//!   recovered files carry no log-data range — and enumerates + row-scans
//!   instead.
//!
//! Recovery limits shape both: the WAL format carries no footer, so
//! [`Registry::recover`] restores identity and header facts but not the
//! log-data range or the durable prefix — those exist only as in-process
//! event state. Startup recovery (file-lifecycle's `recover_unindexed`)
//! re-indexes recovered files whole; the resulting SFST's summary carries
//! the authoritative range, and an SFST always wins over its WAL in the
//! query path.
//!
//! Consumers (grep-verified): `file-lifecycle` composes one registry per
//! tenant (its `Registry::wal` field), forwards events via `apply_wal_event`,
//! recovers at startup, and drives queries through [`Registry::candidates`]
//! (`query_snapshot`, `local_streams`, `local_servable_seqs`); `otel-ledger`
//! routes events to it, hands sealed files to the indexer by `file_path`,
//! seeds seq→tenant routing from `archived_files`, untracks entries on
//! cleaner confirmations (`remove_by_seq`), reads entries back after
//! indexing (`get`), and walks `values()` for the `files: true` inventory.
//! The seq-keyed map and path derivation are `file-registry`'s
//! ([`FileRegistry`]); the query vocabulary is `query.rs`'s ([`Query`]).
use std::fs;
use std::path::{Path, PathBuf};

use file_registry::{ByteSize, FileDir, FileId, FileRegistry, Query, TimestampNs};

use crate::format::{FileEvent, HEADER_SIZE};
use crate::{Error, Result};

/// Extension of WAL data files: `{stem}.wal`. The registry's directory
/// scans, the writer's path derivation, and the crate-root
/// `scan_max_sequence_recursive` walk all name files with it.
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
/// `min_timestamp_ns` / `max_timestamp_ns` are the **log-data** time range
/// of the records written into the file — the caller-supplied per-record
/// OTel timestamps, not the frames' wall-clock ingestion stamps. Each
/// `Synced` overwrites both with the writer's current accumulator state
/// (`ZERO` on both = nothing observed yet) and `Closed` finalizes them; the
/// field semantics are `format.rs`'s.
///
/// On recovery the registry is rebuilt from file headers alone, and the WAL
/// format carries no footer: the log-data range, the durable prefix, and
/// the record count are event-only state that cannot be reconstructed from
/// disk. A recovered entry keeps them at `ZERO`/`0`; startup recovery
/// re-indexes the file whole, and the resulting SFST's summary carries the
/// authoritative range.
///
/// `#[non_exhaustive]`: other crates read this type's fields but never
/// construct it, so a future field addition is non-breaking for them.
#[derive(Debug, Clone)]
#[non_exhaustive]
pub struct File {
    pub id: FileId,
    pub status: FileStatus,
    /// Wall-clock creation time, from the file's header stamp (the
    /// `Created` event, or the recovery header read). A diagnostic —
    /// ordering is by seq, not by this.
    pub created_at_ns: TimestampNs,
    /// Total bytes in the file (header page + frames + padding), written
    /// only by `Closed`; an active file stays at `ZERO`, so a caller that
    /// needs a size for an unsealed WAL uses `valid_up_to` as the proxy.
    pub size: ByteSize,
    /// Opaque content-plane identity blob, recovered cheaply from the WAL
    /// header (no frame decode); the WAL never parses it. The content plane
    /// decodes it to name the stream — in the query-side selector and the
    /// `files: true` inventory. The partition key lives in `id` (the
    /// filename `FileId`), not here.
    pub content_meta: Vec<u8>,
    pub min_timestamp_ns: TimestampNs,
    pub max_timestamp_ns: TimestampNs,
    /// Byte offset of the durable, fully-written prefix: the end of the
    /// last frame fsynced to disk, as reported by the latest `Synced` or
    /// `Closed` event. Frame-aligned by construction. A concurrent reader
    /// of an actively-written file must not read past this offset — the
    /// writer's buffer can flush mid-frame, so the bytes beyond it may be
    /// a torn frame.
    ///
    /// `ByteSize::ZERO` means "unknown": the file was recovered from disk
    /// (no in-process event history; the format carries no footer) or has
    /// not been synced yet. A first sync of a fresh file reports the header
    /// page size, so `ZERO` never stands for a known-empty file. `Closed`
    /// reports the durable prefix alongside `size`; the two are equal
    /// whenever the seal follows a sync (the writer's rotation, idle-sweep,
    /// and shutdown paths), but its unsynced-`Drop` close path syncs
    /// nothing, so a sealed file can carry `valid_up_to < size`.
    pub valid_up_to: ByteSize,
    /// Number of log records in the durable prefix, as reported by the
    /// latest `Synced` or `Closed` event; `0` when unknown (recovered from
    /// disk, or no sync yet). A bounded read of `[HEADER_SIZE,
    /// valid_up_to)` must decode exactly this many records — the
    /// cross-check that the prefix was not truncated.
    pub entry_count: u64,
}

// The registry key is the entry's `FileId.seq`; `apply_event` never
// rewrites `id`, so the key derived at insert stays stable for the entry's
// lifetime.
impl file_registry::Sequenced for File {
    fn seq(&self) -> u64 {
        self.id.seq
    }
}

/// The WAL file index over one tenant directory.
///
/// A [`FileRegistry`] keyed by each entry's `FileId.seq` — a `BTreeMap`
/// underneath, so every iterator is ascending-seq (chronological). Purely
/// in-memory and unsynchronized: mutators take `&mut self`, readers
/// `&self`, and the caller serializes access (file-lifecycle wraps the
/// per-tenant registries behind one `RwLock`). Population is the caller's:
/// [`recover`](Self::recover) from disk at startup, then
/// [`apply_event`](Self::apply_event) as the writer's events arrive.
pub struct Registry {
    files: FileRegistry<File>,
}

impl Registry {
    /// An empty registry over `path` (one tenant's WAL directory). Pure
    /// construction — no disk access, and the directory need not exist
    /// yet; population is the caller's (a recovery scan, or events).
    pub fn new(path: &Path) -> Self {
        Self {
            files: FileRegistry::new(FileDir::new(path, WAL_EXT)),
        }
    }

    /// Rebuild the registry from the directory's files: scan it, then read
    /// each file's header. Identity comes from the filename (`FileId`,
    /// including the `part_key` the header does not store); `created_at_ns`,
    /// `size`, and `content_meta` come from the header. The log-data range
    /// and durable prefix stay unknown (see [`File`]'s recovery note), and
    /// every entry lands as `Archived` — whether the file was mid-write at
    /// the last crash is unknowable from the header, so startup recovery
    /// re-indexes every recovered WAL that has no SFST yet.
    ///
    /// Failure policy: an unreadable file (I/O, or a header format
    /// rejection) is logged and skipped — it stays on disk but untracked. A
    /// missing directory scans as empty; any other scan error propagates
    /// (file-registry's scan contract).
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

            self.files.insert(File {
                id: file_id,
                status: FileStatus::Archived,
                created_at_ns: TimestampNs(header.created_at),
                size,
                // Recovered cheaply from the header.
                content_meta: header.content_meta,
                // Unknown without in-process event history; see `File`'s
                // recovery note.
                min_timestamp_ns: TimestampNs::ZERO,
                max_timestamp_ns: TimestampNs::ZERO,
                // Unknown without in-process event history; recovery
                // re-indexes such files whole. ZERO = do not trust a byte
                // bound: a crash can leave a torn tail past the last sync,
                // and the file carries no durable-prefix marker.
                valid_up_to: ByteSize::ZERO,
                entry_count: 0,
            });
        }

        Ok(())
    }

    pub fn path(&self) -> &Path {
        self.files.dir().path()
    }

    /// Derive the on-disk path for a WAL file. The registry itself never
    /// opens files; the derived path is what the ledger hands to the
    /// indexer and what the query readers open.
    pub fn file_path(&self, id: FileId) -> PathBuf {
        self.files.file_path(id)
    }

    /// The highest `FileId.seq` among the directory's parseable `.wal`
    /// files — `FileDir::scan_max_sequence`'s contract: `0` for a missing
    /// or empty directory; scan-open errors propagate. Startup seq seeding
    /// goes through the crate-root `scan_max_sequence_recursive` walk, not
    /// through this method.
    pub fn scan_max_sequence(&self) -> Result<u64> {
        Ok(self.files.dir().scan_max_sequence()?)
    }

    /// Apply one writer event, updating the entry it names.
    ///
    /// - `Created`: register a new `Active` file — header-derived identity
    ///   (`created_at_ns`, `content_meta`), accumulators at ZERO/0.
    ///   `Error::DuplicateSequence` when the seq is already tracked: the
    ///   [`FileRegistry`] itself would silently replace the entry, so the
    ///   check here is the dedup.
    /// - `Synced`: overwrite the log-data range and the durable prefix with
    ///   the writer's current accumulator state — every event re-reports
    ///   the totals, not a delta, so a direct overwrite is correct. Ignored
    ///   (reported as success) for an `Archived` file: `Closed` is
    ///   authoritative and final, and a late or reordered `Synced` must not
    ///   regress a sealed file's prefix.
    /// - `Closed`: seal — set `Archived` and write the final size, range,
    ///   and durable prefix. The event carries the durable prefix itself,
    ///   so the sealed entry is correct even if the final `Synced` was
    ///   reordered or lost in flight.
    ///
    /// Errors: `Error::UnknownSequence` when `Synced`/`Closed` names a file
    /// the registry does not track (its `Created` was lost, or the entry
    /// was already removed). otel-ledger logs and drops the event on either
    /// error. The `frame_count` field is informational; the registry
    /// ignores it.
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
                self.files.insert(File {
                    id: *file_id,
                    status: FileStatus::Active,
                    created_at_ns: *created_at_ns,
                    size: ByteSize::ZERO,
                    content_meta: content_meta.clone(),
                    min_timestamp_ns: TimestampNs::ZERO,
                    max_timestamp_ns: TimestampNs::ZERO,
                    valid_up_to: ByteSize::ZERO,
                    entry_count: 0,
                });
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
                let entry = self
                    .files
                    .get_mut(file_id.seq)
                    .ok_or(Error::UnknownSequence(file_id.seq))?;
                if entry.status == FileStatus::Archived {
                    return Ok(());
                }
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
                valid_up_to,
                entry_count,
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
                entry.valid_up_to = *valid_up_to;
                entry.entry_count = *entry_count;
                Ok(())
            }
        }
    }

    /// Look up a file by sequence number.
    pub fn get(&self, seq: u64) -> Option<&File> {
        self.files.get(seq)
    }

    /// Untrack the file for `seq`, returning the removed entry. Registry-only:
    /// it never touches the file's bytes. Callers untrack when a WAL's
    /// lifecycle completes (the cleaner's delete confirmation after
    /// indexing) or as the orphan policy at recovery (a file whose indexing
    /// failed stays on disk, untracked, and is retried at the next restart).
    pub fn remove_by_seq(&mut self, seq: u64) -> Option<File> {
        self.files.remove(seq)
    }

    /// The sealed files (`Archived`), ascending-seq — the set startup
    /// recovery walks (index the unindexed; delete WALs that already have
    /// an SFST) and that otel-ledger's pipeline uses to seed seq→tenant
    /// routing.
    pub fn archived_files(&self) -> impl Iterator<Item = &File> {
        self.files
            .values()
            .filter(|f| f.status == FileStatus::Archived)
    }

    /// Every tracked file, `Active` and `Archived`, ascending-seq. The
    /// `files: true` inventory in otel-ledger's logs handler walks this to
    /// list each tenant's files, decoding each file's `content_meta` to
    /// name its stream — including a stream that exists only as an active,
    /// un-indexed WAL (no SFST summary yet).
    pub fn values(&self) -> impl Iterator<Item = &File> {
        self.files.values()
    }

    /// The tracked files whose log-data range intersects `q` — a pure
    /// in-memory filter that opens no WAL file. Both `Active` and
    /// `Archived` files match: active files are how the planner reaches
    /// real-time data not yet flushed into an SFST.
    ///
    /// Time filter: `q.time_range` is seconds (the `Query` unit); it is
    /// scaled to the registry's nanoseconds and tested with
    /// `range_overlaps`' shared rule (inclusive `[min, max]` data range vs
    /// half-open `[start, end)` window; a degenerate window matches
    /// nothing).
    ///
    /// Partition filter: membership over the opaque `id.part_key` — an
    /// empty `partition_keys` matches every partition. One key per stream
    /// identity per tenant (the ingestor's collision table keeps it so),
    /// so key equality is stream equality.
    ///
    /// Files with `min_timestamp_ns == ZERO` are skipped: no event has
    /// reported an authoritative range yet — a file between `Created` and
    /// its first sync, or one recovered from disk. Either way there is no
    /// range to filter against, so the planner drops the file;
    /// file-lifecycle also excludes candidates with `valid_up_to == 0`, so
    /// an unsynced entry can never reach a reader. A recovered file is
    /// never a steady-state candidate: recovery re-indexes it (the SFST
    /// wins) or untracks it as an orphan on a failed seal, so no `Archived`
    /// ZERO-range entry survives recovery.
    pub fn candidates<'a>(&'a self, q: &Query) -> impl Iterator<Item = &'a File> + 'a {
        // Extract q's fields upfront so the `move` closures capture values,
        // not `q`: the returned iterator's lifetime then stays bound to
        // `self` alone, and a caller may hand over a temporary `Query`.
        let q_min_ns = (q.time_range.start as u64) * 1_000_000_000;
        let q_max_ns = (q.time_range.end as u64) * 1_000_000_000;
        let partition_keys = q.partition_keys.clone();

        self.files
            .values()
            .filter(|f| f.min_timestamp_ns != TimestampNs::ZERO)
            .filter(move |f| range_overlaps_ns(f, q_min_ns, q_max_ns))
            .filter(move |f| partition_keys.is_empty() || partition_keys.contains(&f.id.part_key))
    }

    pub fn len(&self) -> usize {
        self.files.len()
    }

    pub fn is_empty(&self) -> bool {
        self.files.is_empty()
    }
}

/// Whether the file's inclusive nanosecond range `[min, max]` overlaps the
/// query's half-open `[q_start_ns, q_end_ns)` window — the shared
/// `file_registry::range_overlaps` rule applied over ns bounds.
fn range_overlaps_ns(file: &File, q_start_ns: u64, q_end_ns: u64) -> bool {
    file_registry::range_overlaps(
        &(q_start_ns..q_end_ns),
        file.min_timestamp_ns.0,
        file.max_timestamp_ns.0,
    )
}

/// Read exactly the first `HEADER_SIZE` bytes of `path` and parse the
/// header page — the cheap identity read recovery uses (no frame is
/// decoded). Failures are I/O (open, short read) or the header's format
/// rejections (`format.rs`); `recover` logs and skips the file on either.
fn read_header(path: &std::path::Path) -> Result<crate::format::FileHeader> {
    use std::io::Read;
    let mut file = fs::File::open(path)?;
    let mut buf = [0u8; HEADER_SIZE];
    file.read_exact(&mut buf)?;
    crate::format::FileHeader::from_bytes(&buf)
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::format::FileEvent;
    use crate::{Config, RotationConfig, Writer};

    fn test_file_id(seq: u64) -> FileId {
        let identity = crate::test_identity();
        FileId::new(identity, 0, seq, 0)
    }

    /// Helper: create a Writer, write entries, shutdown, and return all events.
    fn write_wal_files(dir: &std::path::Path, entry_counts: &[usize]) -> Vec<FileEvent> {
        let entries_per_file: usize = *entry_counts.iter().max().unwrap_or(&10);
        let config = Config {
            rotation: RotationConfig {
                max_entries: entries_per_file,
                max_file_size: ByteSize(u64::MAX),
                max_duration: None,
            },
            crc_enabled: false,
            compression_enabled: true,
        };
        let seq = std::sync::Arc::new(crate::SeqAllocator::ephemeral(0));
        let identity = crate::test_identity();
        let mut writer = Writer::new(
            dir,
            config,
            seq,
            crate::FileStamp {
                pipeline_id: 0,
                payload_format: 7,
            },
            identity,
        )
        .unwrap();
        let mut all_events = Vec::new();
        for &count in entry_counts {
            for i in 0..count {
                writer
                    .write_frame(
                        crate::opaque_part_key("ns", "svc"),
                        &[],
                        &(i as u32).to_le_bytes(),
                        crate::FrameMeta {
                            entry_count: 1,
                            ingestion_ns: TimestampNs(i as u64 + 1),
                            log_ts_range: None,
                        },
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
        // header — the header stores no part_key.
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
                valid_up_to: ByteSize(200),
                entry_count: 0,
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

        // Closed carries the authoritative durable prefix and record count.
        registry
            .apply_event(&FileEvent::Closed {
                file_id: id,
                frame_count: 2,
                min_timestamp_ns: TimestampNs(100),
                max_timestamp_ns: TimestampNs(300),
                size: ByteSize(8704),
                valid_up_to: ByteSize(8704),
                entry_count: 30,
            })
            .unwrap();
        let f = registry.get(3).unwrap();
        assert_eq!(f.status, FileStatus::Archived);
        assert_eq!(f.valid_up_to, ByteSize(8704));
        assert_eq!(f.entry_count, 30);
    }

    #[test]
    fn closed_prefix_wins_over_stale_or_lost_synced() {
        let dir = tempfile::tempdir().unwrap();
        let mut registry = Registry::new(dir.path());
        let id = test_file_id(9);

        registry
            .apply_event(&FileEvent::Created {
                file_id: id,
                created_at_ns: TimestampNs(1),
                content_meta: Vec::new(),
            })
            .unwrap();
        // Closed establishes the authoritative prefix even if no final Synced
        // ever arrived (lost in flight).
        registry
            .apply_event(&FileEvent::Closed {
                file_id: id,
                frame_count: 3,
                min_timestamp_ns: TimestampNs(100),
                max_timestamp_ns: TimestampNs(300),
                size: ByteSize(9000),
                valid_up_to: ByteSize(9000),
                entry_count: 42,
            })
            .unwrap();
        // A stale/reordered Synced arriving after Closed must not regress it.
        registry
            .apply_event(&FileEvent::Synced {
                file_id: id,
                valid_up_to: ByteSize(4096),
                frame_count: 1,
                entry_count: 10,
                min_timestamp_ns: TimestampNs(100),
                max_timestamp_ns: TimestampNs(200),
            })
            .unwrap();
        let f = registry.get(9).unwrap();
        assert_eq!(f.status, FileStatus::Archived);
        assert_eq!(f.valid_up_to, ByteSize(9000));
        assert_eq!(f.entry_count, 42);
        assert_eq!(f.max_timestamp_ns, TimestampNs(300));
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
                valid_up_to: ByteSize(4096),
                entry_count: 0,
            })
            .unwrap();

        assert_eq!(registry.archived_files().count(), 1);
    }

    // ── candidates() tests ───────────────────────────────────────

    fn fid_with(seq: u64, part_key: u64) -> FileId {
        let identity = crate::test_identity();
        FileId::new(identity, 0, seq, part_key)
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
                    valid_up_to: ByteSize(0),
                    entry_count: 0,
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
        track(
            &mut reg,
            2,
            worker,
            100 * NS,
            200 * NS,
            FileStatus::Archived,
        );
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
