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
//! A failed write or fsync: a file that saw one is closed at once and never
//! written again (`Stream::abandon_active_file`); the stream's next frame
//! starts a new file. A failed write can leave a part of a frame in the
//! file, and nothing may follow it there: a reader ends a file at a frame
//! that is not whole. A failed fsync is reported by the kernel once, so the
//! next fsync of the same file would succeed whatever became of the data:
//! there is no next one. In both cases the writer treats the frames that
//! were not durable at that moment as possibly lost for good
//! (`Writer::frames_in_doubt`): `is_durable` answers
//! [`Error::FramesLost`](crate::Error::FramesLost) for them, and no later
//! sync changes that.
//!
//! Events: every lifecycle step (created, synced, closed) is queued per
//! stream and handed to the caller via [`Writer::take_all_events`]; the
//! writer never sends anything itself. The caller (the ingestor) drains and
//! forwards them to the ledger, which rebuilds live registry state from the
//! events — disk recovery cannot reconstruct any of it (see `registry.rs`).
//! The events travel as `wal::Message` over the ingestor→ledger ferryboat
//! link (its size limits are ferryboat's, not this file's concern).
//!
//! All methods take `&mut self` with no internal synchronization: the caller
//! serializes access (the ingestor holds one std mutex per tenant writer).
//! Everything is synchronous blocking I/O; the ingestor runs the idle sweep
//! and a sync's fsyncs under `spawn_blocking`, and the writes inline.
//!
//! Consumers (grep-verified): `otel-ingestor` (the logs and traces services:
//! one writer per tenant, per-frame `write_frame`, split syncs shared by the
//! tenant's requests, the periodic idle sweep — no production `sync_all` or
//! `shutdown_all`; plugin shutdown drops the writers), `ng-ingest` (dev/bench
//! ingest binaries — the only non-test `shutdown_all` callers), and
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
    /// A sync round opens the file by it ([`Stream::begin_sync`]).
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
    /// Durable prefix / record count as of the last fsync that covered the
    /// file: `sync_data`'s, or a split sync's (`finish_sync`, which also
    /// reads them so that a prefix never moves backwards). `Closed` carries
    /// these — not the live `bytes_written` / `log_entry_count` — so a
    /// sealed file's authoritative prefix never includes bytes past the last
    /// fsync; that differs from the live values when the file is closed
    /// with no sync before it: after a failed write or fsync.
    synced_up_to: ByteSize,
    synced_entry_count: u64,
}

impl ActiveFile {
    /// The file's `Closed` event: its final state, with the durable prefix
    /// rather than the live counters.
    fn closed_event(&self) -> FileEvent {
        FileEvent::Closed {
            file_id: self.file_id,
            frame_count: self.frame_count,
            min_timestamp_ns: self.min_timestamp_ns,
            max_timestamp_ns: self.max_timestamp_ns,
            size: self.bytes_written,
            valid_up_to: self.synced_up_to,
            entry_count: self.synced_entry_count,
        }
    }
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
            self.seal()?;
        }

        self.ensure_file()?;

        let ts = meta.ingestion_ns;

        let compressed = if self.config.compression_lz4() {
            lz4_flex::block::compress(data)
        } else {
            data.to_vec()
        };

        // `Writer::write_frame` took at most `MAX_FRAME_DATA` bytes of data,
        // so both lengths are in what the reader accepts and in a `u32`.
        debug_assert!(compressed.len() <= crate::format::MAX_FRAME_PAYLOAD);
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

        let mut frame_header = [0u8; FRAME_HEADER_SIZE];
        frame_header[0..4].copy_from_slice(&payload_len.to_le_bytes());
        frame_header[4..8].copy_from_slice(&uncompressed_len.to_le_bytes());
        frame_header[8..12].copy_from_slice(&entry_count.to_le_bytes());
        frame_header[12..20].copy_from_slice(&ts.0.to_le_bytes());
        frame_header[20..24].copy_from_slice(&crc.to_le_bytes());
        let frame_bytes = FRAME_ALIGNMENT_HEADER + compressed.len();
        let padding = (FRAME_ALIGNMENT - (frame_bytes % FRAME_ALIGNMENT)) % FRAME_ALIGNMENT;

        let active = self.active.as_mut().unwrap();
        let frame_offset = active.bytes_written.0;
        if let Err(e) = write_whole_frame(&mut active.writer, &frame_header, &compressed, padding) {
            self.abandon_active_file();
            return Err(crate::Error::Write(e));
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

    /// Close the active file (if any) the one way a healthy file is closed:
    /// fsync it, then queue its `Closed`, which carries the durable prefix.
    /// No `Synced` comes before it: the `Closed` says the same. The
    /// rotation on the write path, the idle sweep and a shutdown all end a
    /// file here; only a file whose write or fsync failed ends another way
    /// ([`Self::abandon_active_file`]).
    fn seal(&mut self) -> Result<()> {
        self.sync_data()?;
        self.close_active_file();
        Ok(())
    }

    /// Flush and fsync the active file, recording the now-durable prefix on
    /// it. Emits no event: [`Self::seal`] queues the `Closed`.
    fn sync_data(&mut self) -> Result<()> {
        if let Some(active) = &mut self.active {
            let synced = match active.writer.flush() {
                Ok(()) => fsync(active.writer.get_ref()),
                Err(e) => Err(crate::Error::Write(e)),
            };
            if let Err(e) = synced {
                self.abandon_active_file();
                return Err(e);
            }

            // The just-fsynced prefix is now durable; remember it so a later
            // `Closed` with no fresh sync before it (a file abandoned after a
            // failed write or fsync) reports the durable value, not unsynced
            // tail bytes.
            active.synced_up_to = active.bytes_written;
            active.synced_entry_count = active.log_entry_count;
        }
        Ok(())
    }

    /// A split sync's first half for this stream: flush the active file's
    /// buffer and open the file a second time, with what is written to it
    /// now. `None` without an active file.
    ///
    /// The second handle is an open of its own, not a duplicate of the
    /// writer's. The kernel reports a failed write-back once for each open
    /// of a file (Linux 4.13 and later: the `errseq_t` of the file's
    /// mapping, with one position for each open; an older kernel reports it
    /// once for the file, and there this open changes nothing). The round
    /// fsyncs without the lock, so a rotation or the idle sweep can fsync
    /// the same file through the writer's handle meanwhile. With two opens
    /// each of them learns of a failure; a duplicate would share the
    /// writer's position, and only the first of the two to ask would learn
    /// of it.
    fn begin_sync(&mut self) -> Result<Option<FileSync>> {
        let Some(active) = &mut self.active else {
            return Ok(None);
        };

        if let Err(e) = active.writer.flush() {
            self.abandon_active_file();
            return Err(crate::Error::Write(e));
        }
        let file = File::options().write(true).open(&active.path)?;

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
        self.seal()?;
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
        // Durability at creation stops at the directory entry: `create_new`
        // plus the `fsync_dir` below make the name survive power loss, but
        // the header bytes stay page-cache until the file's first fsync. A
        // crash in between can leave an empty or headerless file; a header
        // read then fails on it, so recovery skips it rather than misread.
        let created = write_file_header(&mut writer, &header.to_bytes())
            .and_then(|()| file_registry::durable::fsync_dir(self.dir.path()));
        if let Err(e) = created {
            // Nothing was told of this file and nothing will write to it:
            // on a full disk every request would leave one behind.
            let (_file, _unwritten) = writer.into_parts();
            let _ = std::fs::remove_file(&path);
            return Err(e.into());
        }

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

    /// Take the active file (if any) out of the stream and queue its
    /// `Closed` event — the file's final state, with the durable prefix
    /// rather than the live counters. [`Self::seal`] is the caller.
    fn close_active_file(&mut self) {
        if let Some(active) = self.active.take() {
            self.pending_events.push(active.closed_event());
        }
    }

    /// Close the active file after a write or an fsync of it failed, for
    /// good: no frame may follow a frame that is not whole, and no later
    /// fsync may vouch for bytes whose fsync failed. The bytes still in the
    /// buffer are not written (dropping the `BufWriter` would flush them).
    /// The `Closed` carries the prefix that was durable before the failure.
    fn abandon_active_file(&mut self) {
        if let Some(mut active) = self.active.take() {
            // The bytes in the buffer are counted but not in the file: the
            // `Closed` gives the file's real size, when the file tells it.
            if let Ok(metadata) = active.writer.get_ref().metadata() {
                active.bytes_written = ByteSize(metadata.len());
            }
            self.pending_events.push(active.closed_event());
            let (_file, _unwritten) = active.writer.into_parts();
        }
    }

    /// [`abandon_active_file`](Self::abandon_active_file), if the active
    /// file is still `file_id`.
    fn abandon_file(&mut self, file_id: FileId) {
        if self
            .active
            .as_ref()
            .is_some_and(|active| active.file_id == file_id)
        {
            self.abandon_active_file();
        }
    }

    /// Rotate this stream's active file if it has met a rotation threshold
    /// as of `now_ns`, without any new frame. Reuses the write-path decision
    /// (`should_rotate_with` with zero incoming entries) so idle rotation
    /// shares the exact thresholds; the duration arm is the usual trigger,
    /// and the size arm also seals a file whose last write pushed it over
    /// `max_file_size` with no further write to rotate it (write-path
    /// rotation is checked only at the next `write_frame`). A stream with no
    /// active file is a no-op (never creates a file).
    fn rotate_if_expired(&mut self, now_ns: TimestampNs) -> Result<bool> {
        if self.active.is_none() || !self.should_rotate_with(0, now_ns) {
            return Ok(false);
        }
        self.seal()?;
        Ok(true)
    }
}

/// Write a new file's header page. A test can make the next one fail.
fn write_file_header(writer: &mut BufWriter<File>, header: &[u8]) -> io::Result<()> {
    #[cfg(test)]
    if tests::FAIL_NEXT_WRITE.with(|fail| fail.replace(false)) {
        return Err(io::Error::other("injected"));
    }

    writer.write_all(header)?;
    writer.flush()
}

/// Write one frame whole: its header, its payload, its padding. The one
/// place the writer does it, and a test can make the next one fail half-way.
fn write_whole_frame(
    writer: &mut BufWriter<File>,
    frame_header: &[u8; FRAME_HEADER_SIZE],
    payload: &[u8],
    padding: usize,
) -> io::Result<()> {
    #[cfg(test)]
    if tests::FAIL_NEXT_WRITE.with(|fail| fail.replace(false)) {
        // The frame's header and a part of its payload are written, into
        // the buffer: bytes that must never reach the file.
        writer.write_all(frame_header)?;
        writer.write_all(&payload[..payload.len() / 2])?;
        return Err(io::Error::other("injected"));
    }

    writer.write_all(frame_header)?;
    writer.write_all(payload)?;
    writer.write_all(&[0u8; FRAME_ALIGNMENT][..padding])
}

/// fsync a WAL file: the one place the writer does it. A failure is an
/// [`Error::Fsync`](crate::Error::Fsync), which the [`Writer`] acts on, and a
/// test can make the next one fail.
fn fsync(file: &File) -> Result<()> {
    #[cfg(test)]
    if tests::FAIL_NEXT_FSYNC.with(|fail| fail.replace(false)) {
        return Err(crate::Error::Fsync(io::Error::other("injected")));
    }

    file.sync_all().map_err(crate::Error::Fsync)
}

/// Local alias for [`FRAME_HEADER_SIZE`] used in the frame-size math below.
const FRAME_ALIGNMENT_HEADER: usize = FRAME_HEADER_SIZE;

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
/// frames durable takes one after writing them, without releasing the writer
/// in between, and asks [`Writer::is_durable`].
#[derive(Debug, Clone, Copy, PartialEq, Eq, PartialOrd, Ord)]
pub struct WriteMark(u64);

/// The frames an fsync failure left in doubt: those numbered above `after`
/// up to `through`. See [`Writer::frames_in_doubt`].
#[derive(Clone, Copy)]
struct LostFrames {
    after: u64,
    through: u64,
}

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
    result: Option<Result<()>>,
}

impl FileSync {
    /// fsync the file. Blocks on the disk and needs no lock.
    pub fn fsync(mut self) -> Self {
        self.result = Some(fsync(&self.file));
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
    /// Frames that may be lost: no sync can make them durable any more.
    lost_frames: Option<LostFrames>,
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
            lost_frames: None,
        })
    }

    /// Write a frame to the writer for the given partition.
    ///
    /// Lazily creates a new per-partition writer (keyed by `partition_key`) if
    /// one doesn't exist yet; the opaque `content_meta` identity blob is
    /// recorded in each file's header. A `content_meta` larger than
    /// [`MAX_CONTENT_META_BYTES`](crate::format::MAX_CONTENT_META_BYTES) is
    /// rejected — the caller drops the records — rather than truncated.
    /// So is `data` over [`MAX_FRAME_DATA`](crate::MAX_FRAME_DATA): the
    /// reader would refuse the frame.
    ///
    /// `meta` carries the frame's record count, the caller's monotonic
    /// ingestion timestamp (stamped into the frame header), and the optional
    /// record time range feeding the per-file accumulator — see [`FrameMeta`].
    /// Returns the frame's starting byte offset in the file (header page
    /// included). A failed write or fsync closes the stream's file and puts
    /// the writer's frames that were not durable yet in doubt (module docs).
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
        if data.len() > crate::format::MAX_FRAME_DATA {
            return Err(crate::Error::FrameTooLarge {
                bytes: data.len(),
                limit: crate::format::MAX_FRAME_DATA,
            });
        }
        let written = self
            .get_or_create(partition_key, content_meta)
            .write_frame(data, meta);
        if written.as_ref().is_err_and(|e| e.leaves_frames_in_doubt()) {
            self.frames_in_doubt();
        }
        let frame_offset = written?;

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

    /// Whether a completed sync covers the frames a caller wrote just
    /// before it took `mark`.
    ///
    /// # Errors
    ///
    /// [`Error::FramesLost`](crate::Error::FramesLost): an fsync failed
    /// while those frames were not durable yet.
    pub fn is_durable(&self, mark: WriteMark) -> Result<bool> {
        if self.is_lost(mark) {
            return Err(crate::Error::FramesLost);
        }

        Ok(self.durable_frames >= mark.0)
    }

    fn is_lost(&self, mark: WriteMark) -> bool {
        match self.lost_frames {
            Some(lost) => lost.after < mark.0 && mark.0 <= lost.through,
            None => false,
        }
    }

    /// An fsync of one of this writer's files failed: every frame that is
    /// not durable yet is in doubt, and stays so.
    ///
    /// The kernel reports a failed write-back once for each open of a file.
    /// So the next fsync through the same open succeeds, and its success
    /// says nothing about the frames written before the failure. (A sync
    /// round has an open of its own, so that it and the writer each learn
    /// of a failure: [`Stream::begin_sync`].) Frames of another file are
    /// put in doubt too: the writer counts frames, not files.
    ///
    /// A second failure widens the range up to the frames written by then.
    /// It can take in frames that became durable in between; their callers
    /// then get an error they did not need, and send the records again.
    fn frames_in_doubt(&mut self) {
        let after = match self.lost_frames {
            Some(lost) => lost.after,
            None => self.durable_frames,
        };
        self.lost_frames = Some(LostFrames {
            after,
            through: self.frames_written,
        });
    }

    /// First step of a split sync, under the caller's lock: flush every
    /// active file's buffer and note what is written. The ticket's fsyncs
    /// then run without the lock, and [`finish_sync`](Self::finish_sync)
    /// completes the sync.
    pub fn begin_sync(&mut self) -> SyncTicket {
        let mut files = Vec::with_capacity(self.streams.len());
        let mut error = None;
        let mut in_doubt = false;

        for stream in self.streams.values_mut() {
            match stream.begin_sync() {
                Ok(Some(file)) => files.push(file),
                Ok(None) => {}
                Err(e) => {
                    in_doubt |= e.leaves_frames_in_doubt();
                    error.get_or_insert(e);
                }
            }
        }
        if in_doubt {
            self.frames_in_doubt();
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
    ///
    /// # Errors
    ///
    /// The first file that could not be synced; or
    /// [`Error::FramesLost`](crate::Error::FramesLost) when all of this sync
    /// succeeded but another fsync of the writer (a rotation's, the idle
    /// sweep's) failed while it was out.
    pub fn finish_sync(&mut self, ticket: SyncTicket) -> Result<()> {
        let mut first_error = ticket.error;
        // A file that never came back may have been fsynced, and failed.
        let file_lost = ticket.files.len() != ticket.file_count;
        let mut fsync_failed = file_lost;

        if file_lost {
            first_error
                .get_or_insert_with(|| io::Error::other("a WAL file's fsync was lost").into());
        }

        let mut came_back = Vec::with_capacity(ticket.files.len());
        for mut file in ticket.files {
            came_back.push(file.partition_key);
            match file.result.take() {
                Some(Ok(())) => {
                    if let Some(stream) = self.streams.get_mut(&file.partition_key) {
                        stream.finish_sync(&file);
                    }
                }
                Some(Err(e)) => {
                    fsync_failed = true;
                    first_error.get_or_insert(e);
                    if let Some(stream) = self.streams.get_mut(&file.partition_key) {
                        stream.abandon_file(file.file_id);
                    }
                }
                None => {
                    first_error.get_or_insert_with(|| {
                        io::Error::other("a WAL file was not fsynced").into()
                    });
                }
            }
        }

        if file_lost {
            // Which file it was is not known: it is one that did not come
            // back.
            for (partition_key, stream) in &mut self.streams {
                if !came_back.contains(partition_key) {
                    stream.abandon_active_file();
                }
            }
        }
        if fsync_failed {
            self.frames_in_doubt();
        }
        if let Some(e) = first_error {
            return Err(e);
        }
        self.durable_frames = self.durable_frames.max(ticket.mark.0);

        if self.is_lost(ticket.mark) {
            return Err(crate::Error::FramesLost);
        }

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
        let mut fsync_failed = false;
        for stream in self.streams.values_mut() {
            match stream.rotate_if_expired(now_ns) {
                Ok(true) => rotated += 1,
                Ok(false) => {}
                Err(e) => {
                    if e.leaves_frames_in_doubt() {
                        fsync_failed = true;
                    }
                    if first_err.is_none() {
                        first_err = Some(e);
                    }
                }
            }
        }
        if fsync_failed {
            self.frames_in_doubt();
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

    thread_local! {
        /// Makes the next [`fsync`] of this test's thread fail.
        pub(super) static FAIL_NEXT_FSYNC: std::cell::Cell<bool> =
            const { std::cell::Cell::new(false) };
        /// Makes the next write of this test's thread to a WAL file fail: a
        /// [`write_whole_frame`], after a part of the frame is in the
        /// buffer, or a [`write_file_header`].
        pub(super) static FAIL_NEXT_WRITE: std::cell::Cell<bool> =
            const { std::cell::Cell::new(false) };
    }

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

    #[test]
    fn a_frame_that_the_reader_would_refuse_is_not_written() {
        let tmp = tempfile::tempdir().unwrap();
        let mut writer = test_writer(tmp.path());
        let meta = FrameMeta {
            entry_count: 1,
            ingestion_ns: TimestampNs(1),
            log_ts_range: None,
        };

        let too_large = vec![0u8; crate::MAX_FRAME_DATA + 1];
        let refused = writer.write_frame(pk(1), &[], &too_large, meta);

        assert!(matches!(
            refused,
            Err(crate::Error::FrameTooLarge { bytes, limit })
                if bytes == crate::MAX_FRAME_DATA + 1 && limit == crate::MAX_FRAME_DATA
        ));
        assert_eq!(std::fs::read_dir(tmp.path()).unwrap().count(), 0);
        // Nothing is in doubt: the writer takes the next frame.
        writer.write_frame(pk(1), &[], b"frame", meta).unwrap();
    }

    #[test]
    fn the_largest_frame_that_the_writer_takes_fits_the_reader_when_it_does_not_compress() {
        assert!(
            lz4_flex::block::get_maximum_output_size(crate::MAX_FRAME_DATA)
                <= crate::format::MAX_FRAME_PAYLOAD
        );
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
    fn a_rotation_on_the_write_path_closes_its_file_with_a_closed_event_only() {
        // The default `max_entries` is 100,000: the second frame does not
        // fit the first file.
        let tmp = tempfile::tempdir().unwrap();
        let mut writer = test_writer(tmp.path());
        let meta = |entry_count, ns| crate::FrameMeta {
            entry_count,
            ingestion_ns: TimestampNs(ns),
            log_ts_range: None,
        };
        writer
            .write_frame(pk(1), &[], b"full", meta(100_000, 1))
            .unwrap();
        let _ = writer.take_all_events();

        writer.write_frame(pk(1), &[], b"next", meta(1, 2)).unwrap();

        // The old file: fsynced, then one `Closed` whose durable prefix is
        // the whole file, and no `Synced` before it. Then the new file.
        let events = writer.take_all_events();
        assert!(matches!(
            events.as_slice(),
            [
                FileEvent::Closed { size, valid_up_to, entry_count: 100_000, .. },
                FileEvent::Created { .. },
            ] if size == valid_up_to
        ));
    }

    #[test]
    fn a_sync_round_has_an_open_of_its_own_of_each_file() {
        use std::io::Seek;

        let tmp = tempfile::tempdir().unwrap();
        let mut writer = test_writer(tmp.path());
        let meta = crate::FrameMeta {
            entry_count: 1,
            ingestion_ns: TimestampNs(1),
            log_ts_range: None,
        };
        writer.write_frame(pk(1), &[], b"frame", meta).unwrap();

        let ticket = writer.begin_sync();

        // A duplicate of the writer's handle would share its position, at
        // the end of what is written. A new open starts at 0.
        let mut file = &ticket.files[0].file;
        assert_eq!(file.stream_position().unwrap(), 0);
        writer.finish_sync(ticket.fsync()).unwrap();
    }

    #[test]
    fn a_shutdown_closes_each_file_with_a_closed_event_only() {
        let tmp = tempfile::tempdir().unwrap();
        let mut writer = test_writer(tmp.path());
        let meta = crate::FrameMeta {
            entry_count: 1,
            ingestion_ns: TimestampNs(1),
            log_ts_range: None,
        };
        writer.write_frame(pk(1), &[], b"frame", meta).unwrap();
        let _ = writer.take_all_events();

        let events = writer.shutdown_all().unwrap();

        assert!(matches!(
            events.as_slice(),
            [FileEvent::Closed { size, valid_up_to, .. }] if size == valid_up_to
        ));
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
        assert!(
            writer.is_durable(writer.write_mark()).unwrap(),
            "nothing written"
        );

        write_one(&mut writer, 1, b"one", 1);
        write_one(&mut writer, 2, b"two", 2);
        let mark = writer.write_mark();
        assert!(!writer.is_durable(mark).unwrap(), "nothing synced yet");

        writer.sync_all().unwrap();
        assert!(writer.is_durable(mark).unwrap());
        let synced = synced_of(&writer.take_all_events());
        let mut expected = vec![pk(1), pk(2)];
        expected.sort();
        assert_eq!(synced.iter().map(|s| s.0).collect::<Vec<_>>(), expected);

        // A frame written after the sync needs another one.
        write_one(&mut writer, 2, b"more", 3);
        let later = writer.write_mark();
        assert!(
            writer.is_durable(mark).unwrap(),
            "an older mark stays durable"
        );
        assert!(!writer.is_durable(later).unwrap());

        let ticket = writer.begin_sync().fsync();
        writer.finish_sync(ticket).unwrap();
        assert!(writer.is_durable(later).unwrap());
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
        assert!(writer.is_durable(mark).unwrap());
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
        assert!(
            writer.is_durable(mark).unwrap(),
            "the newer sync stays recorded"
        );
    }

    #[test]
    fn a_failed_fsync_fails_the_sync_and_the_other_files_are_still_reported() {
        let tmp = tempfile::tempdir().unwrap();
        let mut writer = test_writer(tmp.path());
        write_one(&mut writer, 1, b"one", 1);
        write_one(&mut writer, 2, b"two", 2);
        writer.take_all_events();
        let mark = writer.write_mark();

        // Only stream 1's file must fail, so its failure is written into
        // the ticket.
        let mut ticket = writer.begin_sync();
        for mut file in ticket.take_files() {
            file = file.fsync();
            if file.partition_key == pk(1) {
                file.result = Some(Err(crate::Error::Fsync(io::Error::other("injected"))));
            }
            ticket.give_back(file);
        }

        let error = writer.finish_sync(ticket).unwrap_err();
        assert!(error.to_string().contains("injected"), "{error}");
        let synced = synced_of(&writer.take_all_events());
        assert_eq!(synced.iter().map(|s| s.0).collect::<Vec<_>>(), vec![pk(2)]);
        assert!(
            matches!(writer.is_durable(mark), Err(crate::Error::FramesLost)),
            "a frame can sit in the file that failed"
        );
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
        // The lost fsync may have run, and failed.
        assert!(matches!(
            writer.is_durable(mark),
            Err(crate::Error::FramesLost)
        ));
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
        assert!(!writer.is_durable(mark).unwrap());
    }

    /// The kernel reports a failed write-back once: the next fsync of the
    /// file succeeds although the data may be gone. So a sync that succeeds
    /// after one that failed must not make the earlier frames durable.
    #[test]
    fn a_later_sync_does_not_cover_frames_an_fsync_failure_left_in_doubt() {
        let tmp = tempfile::tempdir().unwrap();
        let mut writer = test_writer(tmp.path());
        write_one(&mut writer, 1, b"durable", 1);
        let durable = writer.write_mark();
        writer.sync_all().unwrap();
        write_one(&mut writer, 1, b"in doubt", 2);
        let in_doubt = writer.write_mark();

        FAIL_NEXT_FSYNC.set(true);
        assert!(matches!(writer.sync_all(), Err(crate::Error::Fsync(_))));
        assert!(matches!(
            writer.is_durable(in_doubt),
            Err(crate::Error::FramesLost)
        ));

        // The next fsync succeeds. The writer does not believe it.
        assert!(matches!(writer.sync_all(), Err(crate::Error::FramesLost)));
        assert!(matches!(
            writer.is_durable(in_doubt),
            Err(crate::Error::FramesLost)
        ));

        // What was durable before stays so, and frames written from here on
        // are judged on their own.
        assert!(writer.is_durable(durable).unwrap());
        write_one(&mut writer, 1, b"after", 3);
        let after = writer.write_mark();
        assert!(!writer.is_durable(after).unwrap());
        writer.sync_all().unwrap();
        assert!(writer.is_durable(after).unwrap());
    }

    /// The frames a whole-file read of `path` yields, and the bytes it left.
    fn read_whole_file(path: &std::path::Path) -> (Vec<Vec<u8>>, u64) {
        let mut reader = crate::Reader::open(path).unwrap();
        let mut frames = Vec::new();
        while let Some(frame) = reader.next_frame().unwrap() {
            frames.push(frame.data.to_vec());
        }
        (frames, reader.incomplete_tail())
    }

    /// The WAL files under `dir`, oldest first.
    fn wal_files(dir: &std::path::Path) -> Vec<PathBuf> {
        let mut files: Vec<PathBuf> = std::fs::read_dir(dir)
            .unwrap()
            .map(|entry| entry.unwrap().path())
            .filter(|path| path.extension().is_some_and(|ext| ext == "wal"))
            .collect();
        files.sort();
        files
    }

    /// On a full disk every request would leave a file behind otherwise.
    #[test]
    fn a_file_whose_header_could_not_be_written_is_removed() {
        let tmp = tempfile::tempdir().unwrap();
        let mut writer = test_writer(tmp.path());

        FAIL_NEXT_WRITE.set(true);
        let failed = writer.write_frame(
            pk(1),
            &[],
            b"frame",
            crate::FrameMeta {
                entry_count: 1,
                ingestion_ns: TimestampNs(1),
                log_ts_range: None,
            },
        );

        assert!(failed.is_err());
        assert!(wal_files(tmp.path()).is_empty());
        assert!(writer.take_all_events().is_empty());
        // The next frame gets its file as usual.
        write_one(&mut writer, 1, b"frame", 2);
        assert_eq!(wal_files(tmp.path()).len(), 1);
    }

    #[test]
    fn a_failed_frame_write_closes_the_file() {
        let tmp = tempfile::tempdir().unwrap();
        let mut writer = test_writer(tmp.path());
        write_one(&mut writer, 1, b"synced", 1);
        let ticket = writer.begin_sync();
        writer.finish_sync(ticket.fsync()).unwrap();
        // A whole frame that no sync has covered: it is still in the buffer.
        write_one(&mut writer, 1, b"buffered", 2);
        let mark = writer.write_mark();
        writer.take_all_events();

        FAIL_NEXT_WRITE.set(true);
        let failed = writer.write_frame(
            pk(1),
            &[],
            b"cut short, cut short, cut short",
            crate::FrameMeta {
                entry_count: 1,
                ingestion_ns: TimestampNs(3),
                log_ts_range: None,
            },
        );

        assert!(matches!(failed, Err(crate::Error::Write(_))), "{failed:?}");
        // The file is closed at once, and the frame written before it, which
        // no sync had covered, is in doubt: it is among the bytes that never
        // reach the file.
        let events = writer.take_all_events();
        let closed_size = match events.as_slice() {
            [FileEvent::Closed { size, .. }] => *size,
            other => panic!("expected one `Closed`: {other:?}"),
        };
        assert!(matches!(
            writer.is_durable(mark),
            Err(crate::Error::FramesLost)
        ));
        // The size it reports is the file's, without the bytes that were
        // only in the buffer.
        let old_file = &wal_files(tmp.path())[0];
        assert_eq!(closed_size.0, std::fs::metadata(old_file).unwrap().len());

        // The next frame goes to a new file. The old file holds what was
        // synced and nothing of the buffer: neither the whole frame nor the
        // cut-short one was written when the file was closed.
        write_one(&mut writer, 1, b"next", 4);
        let events = writer.take_all_events();
        assert!(
            matches!(events.as_slice(), [FileEvent::Created { .. }]),
            "{events:?}"
        );
        let files = wal_files(tmp.path());
        assert_eq!(files.len(), 2);
        let (frames, incomplete_tail) = read_whole_file(&files[0]);
        assert_eq!(frames, vec![b"synced".to_vec()]);
        assert_eq!(incomplete_tail, 0);
    }

    #[test]
    fn a_failed_sweep_fsync_closes_the_file() {
        let tmp = tempfile::tempdir().unwrap();
        let mut writer = test_writer(tmp.path());
        let hour_ns: u64 = 3600 * 1_000_000_000;
        write_one(&mut writer, 1, b"idle", 1);
        writer.take_all_events();

        FAIL_NEXT_FSYNC.set(true);
        let swept = writer.rotate_expired(TimestampNs(hour_ns + 2));

        assert!(matches!(swept, Err(crate::Error::Fsync(_))), "{swept:?}");
        // Closed with the prefix that was durable: the header page.
        let events = writer.take_all_events();
        match events.as_slice() {
            [FileEvent::Closed { valid_up_to, .. }] => {
                assert_eq!(*valid_up_to, ByteSize(HEADER_SIZE as u64))
            }
            other => panic!("expected one `Closed`: {other:?}"),
        }
        // No second try on the same file: the next sweep finds nothing.
        assert_eq!(writer.rotate_expired(TimestampNs(hour_ns + 3)).unwrap(), 0);
    }

    #[test]
    fn a_failed_fsync_closes_the_file() {
        let tmp = tempfile::tempdir().unwrap();
        let mut writer = test_writer(tmp.path());
        write_one(&mut writer, 1, b"synced", 1);
        let mut ticket = writer.begin_sync();
        for file in ticket.take_files() {
            ticket.give_back(file.fsync());
        }
        writer.finish_sync(ticket).unwrap();
        let synced = synced_of(&writer.take_all_events());
        let (_, synced_up_to) = synced[0];

        write_one(&mut writer, 1, b"in doubt", 2);
        FAIL_NEXT_FSYNC.set(true);
        let mut ticket = writer.begin_sync();
        for file in ticket.take_files() {
            ticket.give_back(file.fsync());
        }
        let error = writer.finish_sync(ticket).unwrap_err();

        assert!(matches!(error, crate::Error::Fsync(_)), "{error}");
        // The file is closed with the prefix that was durable before: no
        // later fsync of it can vouch for the frame in doubt.
        let events = writer.take_all_events();
        match events.as_slice() {
            [FileEvent::Closed { valid_up_to, .. }] => assert_eq!(*valid_up_to, synced_up_to),
            other => panic!("expected one `Closed`: {other:?}"),
        }
        write_one(&mut writer, 1, b"next", 3);
        assert_eq!(wal_files(tmp.path()).len(), 2);
    }

    #[test]
    fn a_failed_rotation_fsync_puts_the_unsynced_frames_in_doubt() {
        // The frame that makes stream 1 rotate arrives an hour later: the
        // rotation fsyncs the old file first, and that fsync fails.
        let tmp = tempfile::tempdir().unwrap();
        let mut writer = test_writer(tmp.path());
        let hour_ns: u64 = 3600 * 1_000_000_000;
        write_one(&mut writer, 1, b"unsynced", 1);
        let mark = writer.write_mark();

        FAIL_NEXT_FSYNC.set(true);
        let rotating_write = writer.write_frame(
            pk(1),
            &[],
            b"next file",
            crate::FrameMeta {
                entry_count: 1,
                ingestion_ns: TimestampNs(hour_ns + 1),
                log_ts_range: None,
            },
        );

        assert!(matches!(rotating_write, Err(crate::Error::Fsync(_))));
        assert!(matches!(
            writer.is_durable(mark),
            Err(crate::Error::FramesLost)
        ));
    }

    #[test]
    fn a_sync_fails_when_the_sweep_fsync_failed_while_it_was_out() {
        // The sweep's fsync of stream 1's file takes the kernel's error; the
        // fsyncs of the sync that is out then succeed.
        let tmp = tempfile::tempdir().unwrap();
        let mut writer = test_writer(tmp.path());
        let hour_ns: u64 = 3600 * 1_000_000_000;
        write_one(&mut writer, 1, b"old", 1);
        write_one(&mut writer, 2, b"new", hour_ns + 1);
        let mark = writer.write_mark();

        let ticket = writer.begin_sync();
        FAIL_NEXT_FSYNC.set(true);
        assert!(matches!(
            writer.rotate_expired(TimestampNs(hour_ns + 2)),
            Err(crate::Error::Fsync(_))
        ));

        assert!(matches!(
            writer.finish_sync(ticket.fsync()),
            Err(crate::Error::FramesLost)
        ));
        assert!(matches!(
            writer.is_durable(mark),
            Err(crate::Error::FramesLost)
        ));
    }
}
