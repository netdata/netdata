//! The WAL writer: the producing half of the crate (`reader.rs` is the
//! consuming half). A [`Writer`] owns one directory of `.wal` files — one
//! writer per tenant and signal in the otel path — and appends frames to one
//! active file per partition stream (`partition_key`), sealing the file and
//! starting another when a rotation threshold is met. The writer owns the
//! write path: frame emission (compression, CRC, alignment padding), file
//! creation (`create_new` + header page), rotation, and the fsync points.
//! The on-disk format itself — header page, frame header, flags — is
//! `format.rs`'s; the filename/`FileId` vocabulary is file-registry's
//! (`FileDir` derives paths, `Identity`/`FileId` carry the identity,
//! `seq.rs` owns the durable seq allocator).
//!
//! Durability model: frame bytes are buffered per file and made durable only
//! at the fsync points — after a rotation, in a sync ([`Writer::sync_all`],
//! or its split form below) / [`Writer::shutdown_all`], and in the
//! idle-rotation sweep ([`Writer::rotate_expired`]). A new file's directory
//! entry is fsynced at creation (`file_registry::durable::fsync_dir`); its
//! header bytes only at the file's first fsync.
//!
//! Split sync: an fsync waits on the disk, and a caller that serializes
//! access with a lock does not want to hold it meanwhile. So a sync comes in
//! three steps — [`Writer::begin_sync`] under the lock, the fsyncs of the
//! [`SyncTicket`] without it, [`Writer::finish_sync`] under it again — and a
//! [`WriteMark`] tells a caller whether a completed sync already covered the
//! frames it wrote ([`Writer::is_durable`]).
//!
//! Events: every lifecycle step (created, synced, closed) is queued per
//! stream and handed to the caller via [`Writer::take_all_events`]; the
//! writer never sends anything itself. The caller (the ingestor) drains and
//! forwards them to the ledger, which rebuilds live registry state from the
//! events — disk recovery cannot reconstruct any of it (see `registry.rs`).
//! The ledger's gap check runs on the per-signal `frame_seq` the caller's
//! sender assigns, not on anything the writer produces; the events travel as
//! `wal::Message` over the ingestor→ledger ferryboat link (its size limits
//! are ferryboat's, not this file's concern).
//!
//! All methods take `&mut self` with no internal synchronization: the caller
//! serializes access (the ingestor holds one std mutex per tenant writer).
//! Everything is synchronous blocking I/O; the ingestor runs the idle sweep
//! under `spawn_blocking` and the write path inline.
//!
//! Consumers (grep-verified): `otel-ingestor` (the logs and traces services:
//! one writer per tenant, per-frame `write_frame`, one `sync_all` per
//! request, the periodic idle sweep — no production `shutdown_all`; plugin
//! shutdown drops the writers), `ng-ingest` (dev/bench ingest binaries
//! through the same path — the only non-test `shutdown_all` callers), and
//! test fixtures in `sfsq`, `sfsq-cli`, `ng-index`, `otel-ledger`,
//! `file-lifecycle`, and this crate's own tests.
use std::collections::HashMap;
use std::fs::{File, OpenOptions};
use std::io::{self, BufWriter, Write};
use std::path::{Path, PathBuf};
use std::sync::Arc;
use std::time::{SystemTime, UNIX_EPOCH};

use file_registry::FileDir;

use crate::Result;
use crate::config::Config;
use crate::seq::SeqAllocator;
use file_registry::{ByteSize, FileId, Identity, TimestampNs};

use crate::format::{
    COMPRESSION_NONE, FLAG_CRC_ENABLED, FORMAT_VERSION, FRAME_ALIGNMENT, FRAME_HEADER_SIZE,
    FileEvent, FileHeader, HEADER_SIZE,
};

use crate::registry::WAL_EXT;

/// One open `.wal` file a stream is appending to, with the per-file
/// accumulators the lifecycle events report.
struct ActiveFile {
    file_id: FileId,
    // Unused on the write path (hence the attribute); re-derivable as
    // `dir.file_path(file_id)`.
    #[allow(dead_code)]
    path: PathBuf,
    writer: BufWriter<File>,
    frame_count: u64,
    log_entry_count: u64,
    /// Total bytes written into the file — header page + frames + padding.
    /// Buffered, not necessarily durable; the durable value is `synced_up_to`.
    bytes_written: ByteSize,
    /// Log-data time range accumulated from the frames' `log_ts_range`
    /// (`TimestampNs::ZERO` on both = nothing observed yet).
    min_timestamp_ns: TimestampNs,
    max_timestamp_ns: TimestampNs,
    /// `ingestion_ns` of the file's first frame; anchors the duration
    /// rotation arm.
    first_frame_at_ns: Option<TimestampNs>,
    /// Durable prefix / record count as of the last fsync (`sync_data`).
    /// `Closed` carries these — not the live `bytes_written` /
    /// `log_entry_count` — so a sealed file's authoritative prefix never
    /// includes bytes past the last fsync. Matters only on the `Drop` close
    /// path, which writes no sync first.
    synced_up_to: ByteSize,
    synced_entry_count: u64,
}

/// The two opaque identifiers a writer stamps into every file it produces:
/// `pipeline_id` into the filename (`FileId`, the ledger's per-signal
/// routing key) and `payload_format` into the header (the frame-codec tag
/// consumers check before decoding). The caller assigns both — `pipeline_id`
/// from the signal axis (`otel_bridge::signals`), `payload_format` from the frame
/// codec (`ng_flatten` for the OTel signals); the WAL stamps and routes by
/// them, interpreting neither. Named fields so the two `u16`s cannot be
/// swapped at a call site.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct FileStamp {
    pub pipeline_id: u16,
    pub payload_format: u16,
}

/// Per-frame inputs accompanying the payload bytes of a
/// [`write_frame`](Writer::write_frame) call. All fields are mandatory except
/// `log_ts_range`, which is `None` when no record in the frame had a usable
/// timestamp.
#[derive(Debug, Clone, Copy)]
pub struct FrameMeta {
    /// Number of log records in the frame's payload.
    pub entry_count: usize,
    /// The caller's monotonic timestamp for this frame, stamped into the
    /// frame header on disk. Typically from a single process-wide
    /// `file_registry::MonotonicClock` so frame ordering is consistent
    /// across streams — and so the value stays strictly increasing, which
    /// the ingestor also relies on when it reuses this stamp as the
    /// fallback timestamp for records without a usable time.
    pub ingestion_ns: TimestampNs,
    /// `(min, max)` of the record timestamps inside the payload, as resolved
    /// by the caller (for OTel logs, ingest normalization — see
    /// `ng_flatten::normalize_log_request`). Feeds the per-file range
    /// accumulator surfaced in `Synced`/`Closed` events; `None` leaves the
    /// accumulator unchanged.
    pub log_ts_range: Option<(TimestampNs, TimestampNs)>,
}

/// Cheaply clonable handle to the shared [`SeqAllocator`] — every stream
/// needs one and a clone is an `Arc` bump. Allocation is strictly
/// increasing, and startup seeding (dir scans + the persisted high-water
/// file) keeps a restart from reissuing a seq a surviving file still holds;
/// the full contract is `seq.rs`'s.
struct SeqCounter(Arc<SeqAllocator>);

impl SeqCounter {
    fn next(&self) -> Result<u64> {
        self.0.next()
    }
}

/// A single WAL output stream for one partition (identified by
/// `partition_key`).
///
/// Handles frame writing, compression, rotation, and lifecycle event
/// emission. The stream does not own a clock — every per-frame
/// monotonic timestamp comes from the caller (typically a single
/// process-wide `MonotonicClock` in the ingestor) so that
/// frame-level ordering is consistent across all streams.
struct Stream {
    dir: Arc<FileDir>,
    identity: Identity,
    /// The pipeline/payload-format pair stamped into every file this stream
    /// writes (see [`FileStamp`]).
    stamp: FileStamp,
    config: Config,
    active: Option<ActiveFile>,
    seq: SeqCounter,
    /// Opaque partition key for this stream's files — the file-naming/partition
    /// key. The WAL does not interpret it (the content plane derives it).
    partition_key: u64,
    /// Opaque content-plane identity blob recorded in each file's header so the
    /// file's identity is available cheaply without decoding frames.
    content_meta: Vec<u8>,
    /// Lifecycle events queued for the caller to drain (`take_events`); the
    /// writer never sends anywhere itself.
    pending_events: Vec<FileEvent>,
}

impl Stream {
    #[allow(clippy::too_many_arguments)]
    fn new(
        dir: Arc<FileDir>,
        identity: Identity,
        stamp: FileStamp,
        config: Config,
        seq: SeqCounter,
        partition_key: u64,
        content_meta: Vec<u8>,
    ) -> Self {
        Self {
            dir,
            identity,
            stamp,
            config,
            active: None,
            seq,
            partition_key,
            content_meta,
            pending_events: Vec::new(),
        }
    }

    /// Create a `FileId` stamped with this stream's pipeline + partition key.
    fn file_id(&self, seq: u64) -> FileId {
        FileId::new(
            self.identity,
            self.stamp.pipeline_id,
            seq,
            self.partition_key,
        )
    }

    /// Append one frame to the stream, creating its file if needed. Rotation
    /// is checked before the frame is written — a triggering frame starts
    /// the new file. Emits the on-disk layout `format.rs` defines: header
    /// fields in its documented order, payload LZ4-block-compressed when the
    /// stream's config says so, the whole frame padded to `FRAME_ALIGNMENT`
    /// so frame boundaries stay 8-aligned (what the reader's offset checks
    /// rely on). Returns the frame's starting byte offset in the file
    /// (header page included).
    fn write_frame(&mut self, data: &[u8], meta: FrameMeta) -> Result<u64> {
        if self.should_rotate_with(meta.entry_count as u64, meta.ingestion_ns) {
            self.sync()?;
            self.close_active_file();
        }

        self.ensure_file()?;

        let ts = meta.ingestion_ns;

        let compressed = if self.config.compression_lz4() {
            lz4_flex::block::compress(data)
        } else {
            data.to_vec()
        };

        let payload_len = compressed.len() as u32;
        let uncompressed_len = data.len() as u32;
        // The on-disk `entry_count` field is u32 (frame-header layout) and
        // nothing on the write path caps a frame's record count, so this
        // assert states the representability bound. In practice a frame is
        // one producer request's records, far below it.
        debug_assert!(meta.entry_count <= u32::MAX as usize);
        let entry_count = meta.entry_count as u32;

        let crc = if self.config.crc_enabled {
            let mut hasher = crc32fast::Hasher::new();
            hasher.update(&payload_len.to_le_bytes());
            hasher.update(&uncompressed_len.to_le_bytes());
            hasher.update(&entry_count.to_le_bytes());
            hasher.update(&ts.0.to_le_bytes());
            hasher.update(&compressed);
            hasher.finalize()
        } else {
            0
        };

        let active = self.active.as_mut().unwrap();
        let frame_offset = active.bytes_written.0;
        // An I/O error below aborts the frame mid-write: the accumulators
        // stay at the last complete frame (they are advanced only after
        // every `write_all` succeeds), but bytes already flushed are not
        // rewound. Bounded reads never see the torn bytes — the next
        // `valid_up_to` stops at the accumulators — yet the next frame
        // appends after them, so a whole-file read trips on the torn frame.
        active.writer.write_all(&payload_len.to_le_bytes())?;
        active.writer.write_all(&uncompressed_len.to_le_bytes())?;
        active.writer.write_all(&entry_count.to_le_bytes())?;
        active.writer.write_all(&ts.0.to_le_bytes())?;
        active.writer.write_all(&crc.to_le_bytes())?;
        active.writer.write_all(&compressed)?;

        let frame_bytes = FRAME_ALIGNMENT_HEADER + compressed.len();
        let padding = (FRAME_ALIGNMENT - (frame_bytes % FRAME_ALIGNMENT)) % FRAME_ALIGNMENT;
        if padding > 0 {
            active
                .writer
                .write_all(&[0u8; FRAME_ALIGNMENT][..padding])?;
        }

        active.frame_count += 1;
        active.log_entry_count += meta.entry_count as u64;
        active.bytes_written = ByteSize(active.bytes_written.0 + (frame_bytes + padding) as u64);
        if active.first_frame_at_ns.is_none() {
            active.first_frame_at_ns = Some(ts);
        }

        // Accumulate log-data min/max for this file; a frame with no usable
        // record timestamps leaves the accumulator unchanged.
        if let Some((log_min_ts_ns, log_max_ts_ns)) = meta.log_ts_range {
            if active.min_timestamp_ns == TimestampNs::ZERO
                || log_min_ts_ns < active.min_timestamp_ns
            {
                active.min_timestamp_ns = log_min_ts_ns;
            }
            if log_max_ts_ns > active.max_timestamp_ns {
                active.max_timestamp_ns = log_max_ts_ns;
            }
        }

        Ok(frame_offset)
    }

    /// Flush and fsync the active file, recording the now-durable prefix on
    /// it. Emits no `Synced` event: the rotation paths use this and then
    /// seal, relying on the authoritative `Closed` (which carries the
    /// prefix) instead of a redundant `Synced` on an already-synced file.
    fn sync_data(&mut self) -> Result<()> {
        if let Some(active) = &mut self.active {
            active.writer.flush()?;
            active.writer.get_ref().sync_all()?;

            // The just-fsynced prefix is now durable; remember it so a later
            // `Closed` (which may not be preceded by a fresh sync on the `Drop`
            // path) reports the durable value, not unsynced tail bytes.
            active.synced_up_to = active.bytes_written;
            active.synced_entry_count = active.log_entry_count;
        }
        Ok(())
    }

    /// fsync the active file and queue its `Synced` event. Every call with
    /// an active file emits one, even with no new bytes since the last sync
    /// — the event re-reports the accumulated state (not a delta).
    fn sync(&mut self) -> Result<()> {
        self.sync_data()?;
        if let Some(active) = &self.active {
            let event = FileEvent::Synced {
                file_id: active.file_id,
                valid_up_to: active.synced_up_to,
                frame_count: active.frame_count,
                entry_count: active.synced_entry_count,
                min_timestamp_ns: active.min_timestamp_ns,
                max_timestamp_ns: active.max_timestamp_ns,
            };
            self.pending_events.push(event);
        }
        Ok(())
    }

    /// A split sync's first half for this stream: flush the active file's
    /// buffer and take a second handle to the file, with what is written to
    /// it now. `None` without an active file.
    fn begin_sync(&mut self) -> Result<Option<FileSync>> {
        let Some(active) = &mut self.active else {
            return Ok(None);
        };

        active.writer.flush()?;
        let file = active.writer.get_ref().try_clone()?;

        Ok(Some(FileSync {
            partition_key: self.partition_key,
            file_id: active.file_id,
            file,
            up_to: active.bytes_written,
            entry_count: active.log_entry_count,
            frame_count: active.frame_count,
            min_timestamp_ns: active.min_timestamp_ns,
            max_timestamp_ns: active.max_timestamp_ns,
            result: None,
        }))
    }

    /// A split sync's second half for this stream: record the prefix
    /// `fsynced` made durable and queue its `Synced` event.
    ///
    /// Does nothing when the file was sealed meanwhile (a rotation fsyncs
    /// the file before it seals it, and the `Closed` carries that prefix),
    /// or when a sync that began later already recorded more, so the prefix
    /// and the `Synced` events of a file never move backwards.
    fn finish_sync(&mut self, fsynced: &FileSync) {
        let Some(active) = &mut self.active else {
            return;
        };

        if active.file_id != fsynced.file_id || fsynced.up_to < active.synced_up_to {
            return;
        }

        active.synced_up_to = fsynced.up_to;
        active.synced_entry_count = fsynced.entry_count;
        self.pending_events.push(FileEvent::Synced {
            file_id: fsynced.file_id,
            valid_up_to: fsynced.up_to,
            frame_count: fsynced.frame_count,
            entry_count: fsynced.entry_count,
            min_timestamp_ns: fsynced.min_timestamp_ns,
            max_timestamp_ns: fsynced.max_timestamp_ns,
        });
    }

    fn shutdown(&mut self) -> Result<Vec<FileEvent>> {
        self.sync()?;
        self.close_active_file();
        Ok(self.take_events())
    }

    fn take_events(&mut self) -> Vec<FileEvent> {
        std::mem::take(&mut self.pending_events)
    }

    /// Create this stream's file: allocate the seq, `create_new` the file
    /// (never overwrites — a name collision surfaces as an io error, not a
    /// silently replaced file), write the header page, fsync the directory
    /// entry, and queue the `Created` event. The file's flags are fixed here
    /// from the stream's config, so they describe every frame the file will
    /// hold.
    fn ensure_file(&mut self) -> Result<()> {
        if self.active.is_some() {
            return Ok(());
        }

        let file_seq = self.seq.next()?;
        let file_id = self.file_id(file_seq);
        let path = self.dir.file_path(file_id);

        let file = OpenOptions::new()
            .create_new(true)
            .write(true)
            .open(&path)?;
        let mut writer = BufWriter::new(file);

        let mut flags: u16 = 0;
        // `COMPRESSION_LZ4` is the compression bits' zero value, so a
        // compressed file is the default-flags file and only the disabled
        // case sets an explicit bit.
        if self.config.crc_enabled {
            flags |= FLAG_CRC_ENABLED;
        }
        if !self.config.compression_lz4() {
            flags |= COMPRESSION_NONE;
        }

        // The header's `created_at` is a per-file diagnostic, not used for
        // ordering (frames order by the caller-supplied `ingestion_ns`,
        // files by their seq) — `SystemTime::now()` is sufficient.
        let created_at_ns = TimestampNs(
            SystemTime::now()
                .duration_since(UNIX_EPOCH)
                .expect("system clock before Unix epoch")
                .as_nanos() as u64,
        );
        let header = FileHeader {
            version: FORMAT_VERSION,
            flags,
            created_at: created_at_ns.0,
            payload_format: self.stamp.payload_format,
            content_meta: self.content_meta.clone(),
        };
        writer.write_all(&header.to_bytes())?;
        writer.flush()?;

        // Durability at creation stops at the directory entry: `create_new`
        // plus the `fsync_dir` below make the name survive power loss, but
        // the header bytes stay page-cache until the file's first fsync. A
        // crash in between can leave an empty or headerless file; a header
        // read then fails on it, so recovery skips it rather than misread.
        file_registry::durable::fsync_dir(self.dir.path())?;

        self.pending_events.push(FileEvent::Created {
            file_id,
            created_at_ns,
            content_meta: self.content_meta.clone(),
        });

        self.active = Some(ActiveFile {
            file_id,
            path,
            writer,
            frame_count: 0,
            log_entry_count: 0,
            bytes_written: ByteSize(HEADER_SIZE as u64),
            min_timestamp_ns: TimestampNs::ZERO,
            max_timestamp_ns: TimestampNs::ZERO,
            first_frame_at_ns: None,
            synced_up_to: ByteSize(HEADER_SIZE as u64),
            synced_entry_count: 0,
        });

        Ok(())
    }

    /// The rotation decision, shared by the write path and the idle sweep.
    /// Three arms, any of which triggers: the entries cap (counting the
    /// incoming frame), the size cap (checked before the incoming frame is
    /// written, so a file can overshoot `max_file_size` by one frame), and
    /// the duration cap (from the file's first frame's `ingestion_ns` to
    /// `ingestion_ns`, caller-clock — the sweep passes `now_ns` here).
    fn should_rotate_with(&self, incoming_entries: u64, ingestion_ns: TimestampNs) -> bool {
        let Some(active) = &self.active else {
            return false;
        };
        if active.log_entry_count + incoming_entries > self.config.rotation.max_entries as u64 {
            return true;
        }
        if active.bytes_written >= self.config.rotation.max_file_size {
            return true;
        }
        if let (Some(max_dur), Some(first_frame_at)) =
            (self.config.rotation.max_duration, active.first_frame_at_ns)
        {
            let elapsed_ns = ingestion_ns.0.saturating_sub(first_frame_at.0);
            if elapsed_ns >= max_dur.as_nanos() as u64 {
                return true;
            }
        }
        false
    }

    /// Seal the active file (if any) and queue its `Closed` event — the
    /// file's final state, with the durable prefix rather than the live
    /// counters.
    fn close_active_file(&mut self) {
        if let Some(active) = self.active.take() {
            self.pending_events.push(FileEvent::Closed {
                file_id: active.file_id,
                frame_count: active.frame_count,
                min_timestamp_ns: active.min_timestamp_ns,
                max_timestamp_ns: active.max_timestamp_ns,
                size: active.bytes_written,
                valid_up_to: active.synced_up_to,
                entry_count: active.synced_entry_count,
            });
        }
    }

    /// Rotate this stream's active file if it has met a rotation threshold
    /// as of `now_ns`, without any new frame. Reuses the write-path decision
    /// (`should_rotate_with` with zero incoming entries) so idle rotation
    /// shares the exact thresholds; the duration arm is the usual trigger,
    /// and the size arm also seals a file whose last write pushed it over
    /// `max_file_size` with no further write to rotate it (write-path
    /// rotation is checked only at the next `write_frame`). A stream with no
    /// active file is a no-op (never creates a file). Fsyncs via `sync_data`
    /// but emits no `Synced`: the `Closed` it queues carries the durable
    /// prefix, so a redundant `Synced` is not sent.
    fn rotate_if_expired(&mut self, now_ns: TimestampNs) -> Result<bool> {
        if self.active.is_none() || !self.should_rotate_with(0, now_ns) {
            return Ok(false);
        }
        self.sync_data()?;
        self.close_active_file();
        Ok(true)
    }
}

/// Local alias for [`FRAME_HEADER_SIZE`] used in the frame-size math below.
const FRAME_ALIGNMENT_HEADER: usize = FRAME_HEADER_SIZE;

impl Drop for Stream {
    fn drop(&mut self) {
        // No final sync here: the `Closed` this queues carries the durable
        // prefix (`synced_up_to`), not the unsynced tail — see `ActiveFile`.
        self.close_active_file();
    }
}

impl Config {
    /// The only compression this writer emits is LZ4, so `Config`'s
    /// `compression_enabled` bool means LZ4; the write path names the
    /// meaning here rather than re-deriving it at each use.
    pub(crate) fn compression_lz4(&self) -> bool {
        self.compression_enabled
    }
}

// ---------------------------------------------------------------------------
// Split sync
// ---------------------------------------------------------------------------

/// A point in a writer's sequence of frames: every frame it had written
/// when the mark was taken ([`Writer::write_mark`]). A caller that needs its
/// frames durable takes one after writing them and asks
/// [`Writer::is_durable`].
#[derive(Debug, Clone, Copy, PartialEq, Eq, PartialOrd, Ord)]
pub struct WriteMark(u64);

/// One file of a [`SyncTicket`]: a second handle to the file, and the state
/// of the file when the sync began.
pub struct FileSync {
    partition_key: u64,
    file_id: FileId,
    file: File,
    /// What the file held when the sync began. Its `Synced` event reports
    /// these, not what was written since.
    up_to: ByteSize,
    entry_count: u64,
    frame_count: u64,
    min_timestamp_ns: TimestampNs,
    max_timestamp_ns: TimestampNs,
    /// `None` until [`FileSync::fsync`] ran.
    result: Option<io::Result<()>>,
}

impl FileSync {
    /// fsync the file. Blocks on the disk and needs no lock.
    pub fn fsync(mut self) -> Self {
        self.result = Some(self.file.sync_all());
        self
    }
}

/// A sync split around the caller's lock: begun under it
/// ([`Writer::begin_sync`]), fsynced without it, completed under it again
/// ([`Writer::finish_sync`]).
///
/// [`SyncTicket::fsync`] runs the fsyncs one after the other. A caller that
/// wants them to overlap takes the files out ([`SyncTicket::take_files`]),
/// runs [`FileSync::fsync`] on each wherever it likes, and gives each one
/// back ([`SyncTicket::give_back`]).
pub struct SyncTicket {
    files: Vec<FileSync>,
    /// How many files the sync began with; one that never comes back fails
    /// the sync.
    file_count: usize,
    /// The frames this sync makes durable when all of it succeeds.
    mark: WriteMark,
    /// The first file that could not be flushed or get a second handle.
    error: Option<crate::Error>,
}

impl SyncTicket {
    /// fsync every file of the ticket, one after the other. Blocks.
    pub fn fsync(mut self) -> Self {
        for file in self.take_files() {
            self.give_back(file.fsync());
        }

        self
    }

    /// Take the files out of the ticket, to fsync them elsewhere.
    pub fn take_files(&mut self) -> Vec<FileSync> {
        std::mem::take(&mut self.files)
    }

    /// Give a file taken with [`take_files`](Self::take_files) back.
    pub fn give_back(&mut self, file: FileSync) {
        self.files.push(file);
    }
}

// ---------------------------------------------------------------------------
// Writer
// ---------------------------------------------------------------------------

/// Manages multiple WAL output streams (one per `partition_key`) over one
/// directory, with a shared seq counter so file sequence numbers are unique
/// across everything sharing the allocator — all tenants and signals in the
/// otel path, where the ingestor hands every writer the same
/// `Arc<SeqAllocator>`. One writer stamps one fixed [`FileStamp`] — the
/// ingestor runs one per (tenant, signal).
pub struct Writer {
    dir: Arc<FileDir>,
    identity: Identity,
    /// The pipeline/payload-format pair stamped into every file this writer
    /// produces (see [`FileStamp`]).
    stamp: FileStamp,
    config: Config,
    seq: Arc<SeqAllocator>,
    streams: HashMap<u64, Stream>,
    /// Frames written so far, and how many of them a completed sync covers
    /// (see [`WriteMark`]).
    frames_written: u64,
    durable_frames: u64,
}

impl Writer {
    /// Create a writer over `path`. `stamp` names the two identifiers written
    /// into every file (see [`FileStamp`]); there are no defaults, and the
    /// reserved `payload_format` `0` is rejected.
    ///
    /// `identity` is the producer `(machine, instance)` pair stamped into every
    /// file's `FileId`. It comes from the caller (the otel path resolves the
    /// machine GUID from `NETDATA_REGISTRY_UNIQUE_ID` and generates a fresh
    /// instance id per process); the `Identity` newtypes make it non-nil by
    /// construction, so no nil check is needed here. `seq` is the shared
    /// sequence counter — in the otel path one allocator spans every tenant
    /// and signal writer. The directory is created if it doesn't exist.
    ///
    /// Errors: the reserved `payload_format` (`Error::InvalidHeader`) and
    /// directory-creation I/O.
    pub fn new(
        path: &Path,
        config: Config,
        seq: Arc<SeqAllocator>,
        stamp: FileStamp,
        identity: Identity,
    ) -> Result<Self> {
        // A producer stamping the reserved id would write files no query-path
        // decoder accepts — acknowledged but unqueryable data. Refuse at construction.
        if stamp.payload_format == 0 {
            return Err(crate::Error::InvalidHeader(
                "payload_format 0 is reserved (unspecified); producers must pass their codec's id"
                    .to_string(),
            ));
        }
        let dir = Arc::new(FileDir::new(path, WAL_EXT));
        std::fs::create_dir_all(dir.path())?;
        Ok(Self {
            dir,
            identity,
            stamp,
            config,
            seq,
            streams: HashMap::new(),
            frames_written: 0,
            durable_frames: 0,
        })
    }

    /// Write a frame to the writer for the given partition.
    ///
    /// Lazily creates a new per-partition writer (keyed by `partition_key`) if
    /// one doesn't exist yet; the opaque `content_meta` identity blob is
    /// recorded in each file's header. A `content_meta` larger than
    /// [`MAX_CONTENT_META_BYTES`](crate::format::MAX_CONTENT_META_BYTES) is
    /// rejected — the caller drops the records — rather than truncated.
    ///
    /// `meta` carries the frame's record count, the caller's monotonic
    /// ingestion timestamp (stamped into the frame header), and the optional
    /// record time range feeding the per-file accumulator — see [`FrameMeta`].
    /// Returns the frame's starting byte offset in the file (header page
    /// included). On I/O error the frame is aborted mid-write: the durable
    /// prefix stays sound, the bytes beyond it are a torn frame (the write
    /// path above records the details).
    pub fn write_frame(
        &mut self,
        partition_key: u64,
        content_meta: &[u8],
        data: &[u8],
        meta: FrameMeta,
    ) -> Result<u64> {
        if content_meta.len() > crate::format::MAX_CONTENT_META_BYTES {
            return Err(crate::Error::InvalidHeader(format!(
                "content_meta length {} exceeds {}",
                content_meta.len(),
                crate::format::MAX_CONTENT_META_BYTES
            )));
        }
        let frame_offset = self
            .get_or_create(partition_key, content_meta)
            .write_frame(data, meta)?;

        self.frames_written += 1;

        Ok(frame_offset)
    }

    /// Get or lazily create the writer for `partition_key`. On first use the
    /// stream records `content_meta` in each file's header; later frames for
    /// the same key reuse the already-recorded blob.
    fn get_or_create(&mut self, partition_key: u64, content_meta: &[u8]) -> &mut Stream {
        self.streams.entry(partition_key).or_insert_with(|| {
            Stream::new(
                Arc::clone(&self.dir),
                self.identity,
                self.stamp,
                self.config.clone(),
                SeqCounter(Arc::clone(&self.seq)),
                partition_key,
                content_meta.to_vec(),
            )
        })
    }

    /// Drain the pending lifecycle events of every stream into one batch.
    /// Each stream's own events keep their creation order; the
    /// concatenation across streams follows `HashMap` iteration order, so
    /// only the per-file ordering is meaningful. Does not sync: only events
    /// queued by already-completed operations come out.
    pub fn take_all_events(&mut self) -> Vec<FileEvent> {
        let mut events = Vec::new();
        for stream in self.streams.values_mut() {
            events.append(&mut stream.take_events());
        }
        events
    }

    /// Flush and fsync every stream with an active file, queuing a `Synced`
    /// event for each — including streams that saw no new bytes since their
    /// last sync (the event re-reports the accumulated state).
    ///
    /// Every stream is attempted: a `Synced` is queued for each one whose
    /// own fsync succeeded, and the first error is returned. The whole of a
    /// split sync in one call, so it blocks on the disk for as long as the
    /// caller holds the writer.
    pub fn sync_all(&mut self) -> Result<()> {
        let ticket = self.begin_sync().fsync();
        self.finish_sync(ticket)
    }

    /// The frames written so far.
    pub fn write_mark(&self) -> WriteMark {
        WriteMark(self.frames_written)
    }

    /// Whether a completed sync covers every frame of `mark`.
    pub fn is_durable(&self, mark: WriteMark) -> bool {
        self.durable_frames >= mark.0
    }

    /// First step of a split sync, under the caller's lock: flush every
    /// active file's buffer and note what is written. The ticket's fsyncs
    /// then run without the lock, and [`finish_sync`](Self::finish_sync)
    /// completes the sync.
    pub fn begin_sync(&mut self) -> SyncTicket {
        let mut files = Vec::with_capacity(self.streams.len());
        let mut error = None;

        for stream in self.streams.values_mut() {
            match stream.begin_sync() {
                Ok(Some(file)) => files.push(file),
                Ok(None) => {}
                Err(e) => {
                    error.get_or_insert(e);
                }
            }
        }

        SyncTicket {
            file_count: files.len(),
            files,
            mark: self.write_mark(),
            error,
        }
    }

    /// Last step of a split sync, under the caller's lock again: queue a
    /// `Synced` for every file whose fsync succeeded, and return the first
    /// error. Only a sync without any error makes its [`WriteMark`] durable:
    /// a frame can sit in any of the files.
    pub fn finish_sync(&mut self, ticket: SyncTicket) -> Result<()> {
        let mut first_error = ticket.error;

        if ticket.files.len() != ticket.file_count {
            first_error
                .get_or_insert_with(|| io::Error::other("a WAL file's fsync was lost").into());
        }

        for mut file in ticket.files {
            match file.result.take() {
                Some(Ok(())) => {
                    if let Some(stream) = self.streams.get_mut(&file.partition_key) {
                        stream.finish_sync(&file);
                    }
                }
                Some(Err(e)) => {
                    first_error.get_or_insert(e.into());
                }
                None => {
                    first_error.get_or_insert_with(|| {
                        io::Error::other("a WAL file was not fsynced").into()
                    });
                }
            }
        }

        if let Some(e) = first_error {
            return Err(e);
        }
        self.durable_frames = self.durable_frames.max(ticket.mark.0);

        Ok(())
    }

    /// Rotate every stream whose active file has met a rotation threshold as
    /// of `now_ns`, writing no frames and creating no files; streams with no
    /// active file are skipped. The idle-rotation sweep: the ingestor calls
    /// it periodically so a quiet stream still seals, gets indexed, and (with
    /// remote storage) uploaded. Rotation events queue in the streams —
    /// drain them with [`take_all_events`](Self::take_all_events) and
    /// forward to the ledger.
    ///
    /// Best-effort across streams: every stream is attempted even after one
    /// fsync fails, so one failure cannot strand the other idle streams; the
    /// first error is surfaced afterward, and the events queued by the
    /// streams that did rotate are still there to drain. Returns the number
    /// of files rotated on full success.
    pub fn rotate_expired(&mut self, now_ns: TimestampNs) -> Result<usize> {
        let mut rotated = 0;
        let mut first_err: Option<crate::Error> = None;
        for stream in self.streams.values_mut() {
            match stream.rotate_if_expired(now_ns) {
                Ok(true) => rotated += 1,
                Ok(false) => {}
                Err(e) => {
                    if first_err.is_none() {
                        first_err = Some(e);
                    }
                }
            }
        }
        match first_err {
            Some(e) => Err(e),
            None => Ok(rotated),
        }
    }

    /// Shut every stream down cleanly: sync, seal, and return the events.
    /// On the first per-stream error the remaining streams are left
    /// un-synced and the events collected so far are discarded — the caller
    /// sees only the error.
    pub fn shutdown_all(&mut self) -> Result<Vec<FileEvent>> {
        let mut events = Vec::new();
        for stream in self.streams.values_mut() {
            events.append(&mut stream.shutdown()?);
        }
        Ok(events)
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::Config;

    fn test_writer(tmp: &std::path::Path) -> Writer {
        let seq = Arc::new(SeqAllocator::ephemeral(0));
        let identity = crate::test_identity();
        Writer::new(
            tmp,
            Config::default(),
            seq,
            crate::FileStamp {
                pipeline_id: 0,
                payload_format: 7,
            },
            identity,
        )
        .unwrap()
    }

    /// A distinct opaque `partition_key` per label — distinct labels give
    /// distinct keys, so a test can partition by key without hand-picking
    /// values.
    fn pk(label: u64) -> u64 {
        crate::opaque_partition_key("ns", &format!("svc{label}"))
    }

    #[test]
    fn rejects_reserved_payload_format_zero() {
        // 0 is the reserved "unspecified" id; construction must refuse it in
        // every build profile, before any file can be written.
        let tmp = tempfile::tempdir().unwrap();
        let seq = Arc::new(SeqAllocator::ephemeral(0));
        let identity = crate::test_identity();
        match Writer::new(
            tmp.path(),
            Config::default(),
            seq,
            crate::FileStamp {
                pipeline_id: 0,
                payload_format: 0,
            },
            identity,
        ) {
            Err(crate::Error::InvalidHeader(msg)) => {
                assert!(msg.contains("reserved"), "got: {msg}")
            }
            Err(e) => panic!("wrong error: {e:?}"),
            Ok(_) => panic!("payload_format 0 must be rejected"),
        }
    }

    // Nil-identity rejection lives in the type: `MachineId`/`InstanceId`
    // cannot hold the nil UUID, so `Writer::new` cannot be handed one.
    // See `file_registry::types::tests::identity_newtypes_reject_nil`.

    #[test]
    fn filename_embeds_passed_identity() {
        // The caller-supplied machine_id/instance_id MUST land in the FileId
        // stem so the file is attributable to this node and process instance.
        // The 32-hex simple form is the filename's only copy of these values.
        let tmp = tempfile::tempdir().unwrap();
        let identity = crate::test_identity();
        let seq = Arc::new(SeqAllocator::ephemeral(0));
        let mut writer = Writer::new(
            tmp.path(),
            Config::default(),
            seq,
            crate::FileStamp {
                pipeline_id: 0,
                payload_format: 7,
            },
            identity,
        )
        .unwrap();
        writer
            .write_frame(
                crate::opaque_partition_key("ns", "svc"),
                &[],
                b"x",
                crate::FrameMeta {
                    entry_count: 1,
                    ingestion_ns: TimestampNs(1),
                    log_ts_range: None,
                },
            )
            .unwrap();

        // The WAL filename is `{machine:32hex}-{instance:32hex}-{...}.wal`.
        let entries = std::fs::read_dir(tmp.path()).unwrap();
        let wal_name = entries
            .filter_map(|e| e.ok())
            .find(|e| e.path().extension().is_some_and(|x| x == "wal"))
            .expect("a WAL file must have been created")
            .file_name();
        let wal_name = wal_name.to_str().unwrap();
        let machine_hex = identity.machine_id.as_uuid().as_simple().to_string();
        let instance_hex = identity.instance_id.as_uuid().as_simple().to_string();
        assert!(
            wal_name.starts_with(&format!("{machine_hex}-{instance_hex}-")),
            "filename {wal_name} must start with the passed identity pair"
        );
    }

    #[test]
    fn rotate_expired_seals_only_expired_streams() {
        // Default rotation `max_duration` is 1h; the idle sweep rotates a
        // stream whose lone frame is older than that, leaves a fresh one, and
        // never creates a file for a stream with no active file.
        let tmp = tempfile::tempdir().unwrap();
        let mut writer = test_writer(tmp.path());
        let hour_ns: u64 = 3600 * 1_000_000_000;
        let now = TimestampNs(hour_ns + 2);

        // Stream A: sole frame long ago (expired at `now`).
        writer
            .write_frame(
                pk(1),
                &[],
                b"aaa",
                crate::FrameMeta {
                    entry_count: 3,
                    ingestion_ns: TimestampNs(1),
                    log_ts_range: None,
                },
            )
            .unwrap();
        // Stream B: a fresh frame just before `now` (not expired).
        writer
            .write_frame(
                pk(2),
                &[],
                b"b",
                crate::FrameMeta {
                    entry_count: 1,
                    ingestion_ns: TimestampNs(hour_ns + 1),
                    log_ts_range: None,
                },
            )
            .unwrap();
        // Clear the writes' `Created` events so we observe only sweep events.
        let _ = writer.take_all_events();

        // Sweep: only A's idle file is past `max_duration`.
        assert_eq!(writer.rotate_expired(now).unwrap(), 1);
        let events = writer.take_all_events();
        let closed: Vec<_> = events
            .iter()
            .filter_map(|e| match e {
                FileEvent::Closed {
                    file_id,
                    valid_up_to,
                    entry_count,
                    size,
                    ..
                } => Some((file_id.partition_key, *valid_up_to, *entry_count, *size)),
                _ => None,
            })
            .collect();
        assert_eq!(
            closed.len(),
            1,
            "exactly one stream should seal: {events:?}"
        );
        let (partition_key, valid_up_to, entry_count, size) = closed[0];
        assert_eq!(
            partition_key,
            pk(1),
            "the expired stream (A) must be sealed"
        );
        assert!(
            valid_up_to.0 > HEADER_SIZE as u64,
            "valid_up_to must include the written frame"
        );
        assert_eq!(valid_up_to, size, "sealed file: valid_up_to == size");
        assert_eq!(entry_count, 3, "entry_count carried from the accumulator");

        // A now has no active file; B is still fresh — another sweep no-ops and
        // creates nothing.
        assert_eq!(writer.rotate_expired(now).unwrap(), 0);
        assert!(writer.take_all_events().is_empty());
    }

    #[test]
    fn creates_separate_files_per_ns_hash() {
        let tmp = tempfile::tempdir().unwrap();
        let mut writer = test_writer(tmp.path());

        let data = b"test payload";

        writer
            .write_frame(
                pk(1),
                &[],
                data,
                crate::FrameMeta {
                    entry_count: 1,
                    ingestion_ns: TimestampNs(1),
                    log_ts_range: None,
                },
            )
            .unwrap();
        writer
            .write_frame(
                pk(2),
                &[],
                data,
                crate::FrameMeta {
                    entry_count: 1,
                    ingestion_ns: TimestampNs(2),
                    log_ts_range: None,
                },
            )
            .unwrap();
        writer
            .write_frame(
                pk(1),
                &[],
                data,
                crate::FrameMeta {
                    entry_count: 1,
                    ingestion_ns: TimestampNs(3),
                    log_ts_range: None,
                },
            )
            .unwrap();

        writer.sync_all().unwrap();

        // Two distinct streams → two WAL files.
        let wal_files: Vec<_> = std::fs::read_dir(tmp.path())
            .unwrap()
            .filter_map(|e| e.ok())
            .filter(|e| e.path().extension().is_some_and(|ext| ext == "wal"))
            .collect();
        assert_eq!(wal_files.len(), 2);

        // Filenames carry each stream's distinct partition_key.
        let mut hashes: Vec<u64> = wal_files
            .iter()
            .map(|e| FileId::parse(&e.path()).unwrap().partition_key)
            .collect();
        hashes.sort();
        let mut expected = vec![pk(1), pk(2)];
        expected.sort();
        assert_eq!(hashes, expected);
    }

    #[test]
    fn accumulates_log_ts_range_across_frames() {
        let tmp = tempfile::tempdir().unwrap();
        let mut writer = test_writer(tmp.path());
        let data = b"x";

        // Three frames with growing & overlapping ranges.
        writer
            .write_frame(
                pk(1),
                &[],
                data,
                crate::FrameMeta {
                    entry_count: 1,
                    ingestion_ns: TimestampNs(1),
                    log_ts_range: Some((TimestampNs(200), TimestampNs(300))),
                },
            )
            .unwrap();
        writer
            .write_frame(
                pk(1),
                &[],
                data,
                crate::FrameMeta {
                    entry_count: 1,
                    ingestion_ns: TimestampNs(2),
                    log_ts_range: Some((TimestampNs(150), TimestampNs(250))),
                },
            )
            .unwrap();
        writer
            .write_frame(
                pk(1),
                &[],
                data,
                crate::FrameMeta {
                    entry_count: 1,
                    ingestion_ns: TimestampNs(3),
                    log_ts_range: Some((TimestampNs(180), TimestampNs(400))),
                },
            )
            .unwrap();

        writer.sync_all().unwrap();
        let events = writer.take_all_events();

        let synced = events
            .iter()
            .find_map(|e| match e {
                FileEvent::Synced {
                    min_timestamp_ns,
                    max_timestamp_ns,
                    ..
                } => Some((*min_timestamp_ns, *max_timestamp_ns)),
                _ => None,
            })
            .expect("expected a Synced event");
        assert_eq!(synced, (TimestampNs(150), TimestampNs(400)));

        let final_events = writer.shutdown_all().unwrap();
        let closed = final_events
            .iter()
            .find_map(|e| match e {
                FileEvent::Closed {
                    min_timestamp_ns,
                    max_timestamp_ns,
                    ..
                } => Some((*min_timestamp_ns, *max_timestamp_ns)),
                _ => None,
            })
            .expect("expected a Closed event");
        assert_eq!(closed, (TimestampNs(150), TimestampNs(400)));
    }

    #[test]
    fn zero_log_ts_does_not_clobber_accumulator() {
        let tmp = tempfile::tempdir().unwrap();
        let mut writer = test_writer(tmp.path());
        let data = b"x";

        writer
            .write_frame(
                pk(1),
                &[],
                data,
                crate::FrameMeta {
                    entry_count: 1,
                    ingestion_ns: TimestampNs(1),
                    log_ts_range: Some((TimestampNs(500), TimestampNs(600))),
                },
            )
            .unwrap();
        // Frame whose logs all lacked time/observed timestamps: in production
        // the ingestor synthesizes a fallback range, so this arm would not be
        // hit; the test exercises the defense-in-depth `None` case — the
        // accumulator must be left untouched.
        writer
            .write_frame(
                pk(1),
                &[],
                data,
                crate::FrameMeta {
                    entry_count: 1,
                    ingestion_ns: TimestampNs(2),
                    log_ts_range: None,
                },
            )
            .unwrap();

        writer.sync_all().unwrap();
        let events = writer.take_all_events();
        let synced = events
            .iter()
            .find_map(|e| match e {
                FileEvent::Synced {
                    min_timestamp_ns,
                    max_timestamp_ns,
                    ..
                } => Some((*min_timestamp_ns, *max_timestamp_ns)),
                _ => None,
            })
            .unwrap();
        assert_eq!(synced, (TimestampNs(500), TimestampNs(600)));
    }

    #[test]
    fn shared_seq_is_globally_unique() {
        let tmp = tempfile::tempdir().unwrap();
        let mut writer = test_writer(tmp.path());

        let data = b"test payload";
        writer
            .write_frame(
                pk(10),
                &[],
                data,
                crate::FrameMeta {
                    entry_count: 1,
                    ingestion_ns: TimestampNs(1),
                    log_ts_range: None,
                },
            )
            .unwrap();
        writer
            .write_frame(
                pk(20),
                &[],
                data,
                crate::FrameMeta {
                    entry_count: 1,
                    ingestion_ns: TimestampNs(2),
                    log_ts_range: None,
                },
            )
            .unwrap();

        writer.sync_all().unwrap();

        let mut seqs: Vec<u64> = std::fs::read_dir(tmp.path())
            .unwrap()
            .filter_map(|e| e.ok())
            .map(|e| FileId::parse(&e.path()).unwrap().seq)
            .collect();
        seqs.sort();
        // Sequences must be distinct (1, 2) — not both 1.
        assert_eq!(seqs, vec![1, 2]);
    }

    // --- split sync: write_mark / is_durable / begin_sync / finish_sync ---

    fn write_one(writer: &mut Writer, label: u64, payload: &[u8], ingestion_ns: u64) {
        writer
            .write_frame(
                pk(label),
                &[],
                payload,
                crate::FrameMeta {
                    entry_count: 1,
                    ingestion_ns: TimestampNs(ingestion_ns),
                    log_ts_range: None,
                },
            )
            .unwrap();
    }

    /// `(partition_key, valid_up_to)` of every `Synced` in `events`, sorted.
    fn synced_of(events: &[FileEvent]) -> Vec<(u64, ByteSize)> {
        let mut synced = Vec::new();
        for event in events {
            if let FileEvent::Synced {
                file_id,
                valid_up_to,
                ..
            } = event
            {
                synced.push((file_id.partition_key, *valid_up_to));
            }
        }
        synced.sort();
        synced
    }

    #[test]
    fn a_sync_makes_the_frames_written_before_it_durable() {
        let tmp = tempfile::tempdir().unwrap();
        let mut writer = test_writer(tmp.path());
        assert!(writer.is_durable(writer.write_mark()), "nothing written");

        write_one(&mut writer, 1, b"one", 1);
        write_one(&mut writer, 2, b"two", 2);
        let mark = writer.write_mark();
        assert!(!writer.is_durable(mark), "nothing synced yet");

        writer.sync_all().unwrap();
        assert!(writer.is_durable(mark));
        let synced = synced_of(&writer.take_all_events());
        let mut expected = vec![pk(1), pk(2)];
        expected.sort();
        assert_eq!(synced.iter().map(|s| s.0).collect::<Vec<_>>(), expected);

        // A frame written after the sync needs another one.
        write_one(&mut writer, 2, b"more", 3);
        let later = writer.write_mark();
        assert!(writer.is_durable(mark), "an older mark stays durable");
        assert!(!writer.is_durable(later));

        let ticket = writer.begin_sync().fsync();
        writer.finish_sync(ticket).unwrap();
        assert!(writer.is_durable(later));
    }

    #[test]
    fn finish_sync_skips_a_file_sealed_meanwhile() {
        // The sweep fsyncs and seals stream 1's file while the ticket is
        // out, and the stream starts another file: the sealed file's
        // `Closed` carries its prefix, so the sync adds no `Synced` for the
        // stream, and the frames still become durable.
        let tmp = tempfile::tempdir().unwrap();
        let mut writer = test_writer(tmp.path());
        let hour_ns: u64 = 3600 * 1_000_000_000;
        write_one(&mut writer, 1, b"old", 1);
        write_one(&mut writer, 2, b"new", hour_ns + 1);
        writer.take_all_events();
        let mark = writer.write_mark();

        let ticket = writer.begin_sync();
        assert_eq!(writer.rotate_expired(TimestampNs(hour_ns + 2)).unwrap(), 1);
        let sealed = writer.take_all_events();
        assert!(sealed.iter().any(|e| matches!(
            e,
            FileEvent::Closed { file_id, .. } if file_id.partition_key == pk(1)
        )));
        write_one(&mut writer, 1, b"next file", hour_ns + 3);
        writer.take_all_events();

        writer.finish_sync(ticket.fsync()).unwrap();
        let synced = synced_of(&writer.take_all_events());
        assert_eq!(synced.iter().map(|s| s.0).collect::<Vec<_>>(), vec![pk(2)]);
        assert!(writer.is_durable(mark));
    }

    #[test]
    fn finish_sync_never_moves_a_prefix_backwards() {
        // Two syncs that finish out of order: the older one reports nothing.
        let tmp = tempfile::tempdir().unwrap();
        let mut writer = test_writer(tmp.path());
        write_one(&mut writer, 1, b"first", 1);
        let older = writer.begin_sync();
        write_one(&mut writer, 1, b"second", 2);
        let newer = writer.begin_sync();
        let mark = writer.write_mark();
        writer.take_all_events();

        writer.finish_sync(newer.fsync()).unwrap();
        let reported = synced_of(&writer.take_all_events());
        assert_eq!(reported.len(), 1);

        writer.finish_sync(older.fsync()).unwrap();
        assert!(
            synced_of(&writer.take_all_events()).is_empty(),
            "an older prefix must not be reported after a newer one"
        );
        assert!(writer.is_durable(mark), "the newer sync stays recorded");

        // The file's recorded prefix is still the newer one: a `Closed`
        // reports it.
        let closed = writer.shutdown_all().unwrap();
        let sealed_up_to = closed.iter().find_map(|e| match e {
            FileEvent::Closed { valid_up_to, .. } => Some(*valid_up_to),
            _ => None,
        });
        assert_eq!(sealed_up_to, Some(reported[0].1));
    }

    #[test]
    fn a_failed_fsync_fails_the_sync_and_the_other_files_are_still_reported() {
        let tmp = tempfile::tempdir().unwrap();
        let mut writer = test_writer(tmp.path());
        write_one(&mut writer, 1, b"one", 1);
        write_one(&mut writer, 2, b"two", 2);
        writer.take_all_events();
        let mark = writer.write_mark();

        // The crate has no seam to make an fsync fail, so the failure of
        // stream 1's file is written into the ticket.
        let mut ticket = writer.begin_sync();
        for mut file in ticket.take_files() {
            file = file.fsync();
            if file.partition_key == pk(1) {
                file.result = Some(Err(io::Error::other("injected")));
            }
            ticket.give_back(file);
        }

        let error = writer.finish_sync(ticket).unwrap_err();
        assert!(error.to_string().contains("injected"), "{error}");
        let synced = synced_of(&writer.take_all_events());
        assert_eq!(synced.iter().map(|s| s.0).collect::<Vec<_>>(), vec![pk(2)]);
        assert!(
            !writer.is_durable(mark),
            "a frame can sit in the file that failed"
        );

        // The next sync that succeeds covers the frames.
        writer.sync_all().unwrap();
        assert!(writer.is_durable(mark));
        assert_eq!(synced_of(&writer.take_all_events()).len(), 2);
    }

    #[test]
    fn a_file_missing_from_the_ticket_fails_the_sync() {
        // A caller that fsyncs the files elsewhere and loses one (its task
        // panicked, say) must not make the frames durable.
        let tmp = tempfile::tempdir().unwrap();
        let mut writer = test_writer(tmp.path());
        write_one(&mut writer, 1, b"one", 1);
        write_one(&mut writer, 2, b"two", 2);
        writer.take_all_events();
        let mark = writer.write_mark();

        let mut ticket = writer.begin_sync();
        let mut files = ticket.take_files();
        drop(files.pop());
        for file in files {
            ticket.give_back(file.fsync());
        }

        assert!(writer.finish_sync(ticket).is_err());
        assert_eq!(synced_of(&writer.take_all_events()).len(), 1);
        assert!(!writer.is_durable(mark));
    }

    #[test]
    fn a_file_that_was_not_fsynced_fails_the_sync() {
        let tmp = tempfile::tempdir().unwrap();
        let mut writer = test_writer(tmp.path());
        write_one(&mut writer, 1, b"one", 1);
        writer.take_all_events();
        let mark = writer.write_mark();

        let ticket = writer.begin_sync();

        assert!(writer.finish_sync(ticket).is_err());
        assert!(synced_of(&writer.take_all_events()).is_empty());
        assert!(!writer.is_durable(mark));
    }
}
