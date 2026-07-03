use std::collections::HashMap;
use std::fs::{File, OpenOptions};
use std::io::{BufWriter, Write};
use std::path::{Path, PathBuf};
use std::sync::Arc;
use std::time::{SystemTime, UNIX_EPOCH};

use file_registry::FileDir;
use uuid::Uuid;

use crate::Result;
use crate::config::Config;
use crate::seq::SeqAllocator;
use file_registry::{ByteSize, FileId, TimestampNs};

use crate::format::{
    COMPRESSION_NONE, FLAG_CRC_ENABLED, FORMAT_VERSION, FRAME_ALIGNMENT, FRAME_HEADER_SIZE,
    FileEvent, FileHeader, HEADER_SIZE,
};

use crate::registry::WAL_EXT;

struct ActiveFile {
    file_id: FileId,
    #[allow(dead_code)]
    path: PathBuf,
    writer: BufWriter<File>,
    frame_count: u64,
    log_entry_count: u64,
    bytes_written: ByteSize,
    min_timestamp_ns: TimestampNs,
    max_timestamp_ns: TimestampNs,
    first_frame_at_ns: Option<TimestampNs>,
}

/// The two opaque identifiers a writer stamps into every file it produces:
/// `pipeline_id` into the filename (`FileId`, the signal axis the ledger
/// routes by) and `payload_format` into the header (the frame-codec tag
/// consumers check before decoding). The content plane assigns both; the WAL
/// interprets neither. Named fields so the two `u16`s cannot be swapped at a
/// call site.
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
    /// [`file_registry::MonotonicClock`] so frame ordering is consistent
    /// across streams.
    pub ingestion_ns: TimestampNs,
    /// `(min, max)` of the record timestamps inside the payload, as resolved
    /// by the caller (for OTel logs, ingest normalization — see
    /// `ng_flatten::normalize_log_request`). Feeds the per-file range
    /// accumulator surfaced in `Synced`/`Closed` events; `None` leaves the
    /// accumulator unchanged.
    pub log_ts_range: Option<(TimestampNs, TimestampNs)>,
}

/// Shared sequence counter for globally unique file numbering. Wraps
/// the process-wide [`SeqAllocator`], which never reissues a seq across
/// restarts (see `seq.rs`).
struct SeqCounter(Arc<SeqAllocator>);

impl SeqCounter {
    fn next(&self) -> Result<u64> {
        self.0.next()
    }
}

/// A single WAL output stream for one partition (identified by `part_key`).
///
/// Handles frame writing, compression, rotation, and lifecycle event
/// emission. The stream does not own a clock — every per-frame
/// monotonic timestamp comes from the caller (typically a single
/// process-wide `MonotonicClock` in the ingestor) so that
/// frame-level ordering is consistent across all streams.
struct Stream {
    dir: Arc<FileDir>,
    machine_id: Uuid,
    boot_id: Uuid,
    /// The pipeline/payload-format pair stamped into every file this stream
    /// writes (see [`FileStamp`]).
    stamp: FileStamp,
    config: Config,
    active: Option<ActiveFile>,
    seq: SeqCounter,
    /// Opaque partition key for this stream's files — the file-naming/partition
    /// key. The WAL does not interpret it (the content plane derives it).
    part_key: u64,
    /// Opaque content-plane identity blob recorded in each file's header so the
    /// file's identity is available cheaply without decoding frames.
    content_meta: Vec<u8>,
    pending_events: Vec<FileEvent>,
}

impl Stream {
    #[allow(clippy::too_many_arguments)]
    fn new(
        dir: Arc<FileDir>,
        machine_id: Uuid,
        boot_id: Uuid,
        stamp: FileStamp,
        config: Config,
        seq: SeqCounter,
        part_key: u64,
        content_meta: Vec<u8>,
    ) -> Self {
        Self {
            dir,
            machine_id,
            boot_id,
            stamp,
            config,
            active: None,
            seq,
            part_key,
            content_meta,
            pending_events: Vec::new(),
        }
    }

    /// Create a [`FileId`] stamped with this stream's pipeline + partition key.
    fn file_id(&self, seq: u64) -> FileId {
        FileId::new(
            self.machine_id,
            self.boot_id,
            self.stamp.pipeline_id,
            seq,
            self.part_key,
        )
    }

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
        // The on-disk frame header stores u32; the payload cap keeps real
        // frames orders of magnitude below it.
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

    fn sync(&mut self) -> Result<()> {
        if let Some(active) = &mut self.active {
            active.writer.flush()?;
            active.writer.get_ref().sync_all()?;

            self.pending_events.push(FileEvent::Synced {
                file_id: active.file_id,
                valid_up_to: active.bytes_written,
                frame_count: active.frame_count,
                entry_count: active.log_entry_count,
                min_timestamp_ns: active.min_timestamp_ns,
                max_timestamp_ns: active.max_timestamp_ns,
            });
        }
        Ok(())
    }

    fn shutdown(&mut self) -> Result<Vec<FileEvent>> {
        self.sync()?;
        self.close_active_file();
        Ok(self.take_events())
    }

    fn take_events(&mut self) -> Vec<FileEvent> {
        std::mem::take(&mut self.pending_events)
    }

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
        if self.config.crc_enabled {
            flags |= FLAG_CRC_ENABLED;
        }
        if !self.config.compression_lz4() {
            flags |= COMPRESSION_NONE;
        }

        // The header's `created_at` is a per-file diagnostic, not used for
        // ordering — `SystemTime::now()` is sufficient. Frame-level
        // ordering is the caller-supplied `ingestion_ns` instead.
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
        });

        Ok(())
    }

    fn should_rotate_with(&self, incoming_entries: u64, ingestion_ns: TimestampNs) -> bool {
        let Some(active) = &self.active else {
            return false;
        };
        if active.log_entry_count + incoming_entries > self.config.rotation.max_log_entries as u64 {
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

    fn close_active_file(&mut self) {
        if let Some(active) = self.active.take() {
            self.pending_events.push(FileEvent::Closed {
                file_id: active.file_id,
                frame_count: active.frame_count,
                min_timestamp_ns: active.min_timestamp_ns,
                max_timestamp_ns: active.max_timestamp_ns,
                size: active.bytes_written,
            });
        }
    }
}

const FRAME_ALIGNMENT_HEADER: usize = FRAME_HEADER_SIZE;

impl Drop for Stream {
    fn drop(&mut self) {
        self.close_active_file();
    }
}

impl Config {
    pub(crate) fn compression_lz4(&self) -> bool {
        self.compression_enabled
    }
}

// ---------------------------------------------------------------------------
// Writer
// ---------------------------------------------------------------------------

/// Manages multiple WAL output streams (one per `part_key`), with a shared
/// monotonic sequence counter so that file sequence numbers are globally
/// unique within the WAL directory.
pub struct Writer {
    dir: Arc<FileDir>,
    machine_id: Uuid,
    boot_id: Uuid,
    /// The pipeline/payload-format pair stamped into every file this writer
    /// produces (see [`FileStamp`]). One writer process serves one signal.
    stamp: FileStamp,
    config: Config,
    seq: Arc<SeqAllocator>,
    streams: HashMap<u64, Stream>,
}

impl Writer {
    /// Create a new writer. `stamp` names the two identifiers written into
    /// every file (see [`FileStamp`]); there are no defaults, and the
    /// reserved `payload_format` `0` is rejected.
    ///
    /// Machine and boot IDs are loaded from the system. The caller provides
    /// a shared sequence counter (e.g., shared across per-tenant writers).
    /// The directory is created if it doesn't exist.
    pub fn new(
        path: &Path,
        config: Config,
        seq: Arc<SeqAllocator>,
        stamp: FileStamp,
    ) -> Result<Self> {
        // A producer stamping the reserved id would write files every reader
        // rejects — acknowledged but unqueryable data. Refuse at construction.
        if stamp.payload_format == 0 {
            return Err(crate::Error::InvalidHeader(
                "payload_format 0 is reserved (unspecified); producers must pass their codec's id"
                    .to_string(),
            ));
        }
        let machine_id = journal_common::load_machine_id().map_err(crate::Error::Io)?;
        let boot_id = journal_common::load_boot_id().map_err(crate::Error::Io)?;
        let dir = Arc::new(FileDir::new(path, WAL_EXT));
        std::fs::create_dir_all(dir.path())?;
        Ok(Self {
            dir,
            machine_id,
            boot_id,
            stamp,
            config,
            seq,
            streams: HashMap::new(),
        })
    }

    /// Write a frame to the writer for the given partition.
    ///
    /// Lazily creates a new per-partition writer (keyed by `part_key`) if one
    /// doesn't exist yet; the opaque `content_meta` identity blob is recorded
    /// in each file's header. A `content_meta` larger than
    /// [`MAX_CONTENT_META_BYTES`](crate::format::MAX_CONTENT_META_BYTES) is
    /// rejected (the caller drops the record) rather than truncated.
    ///
    /// `meta` carries the frame's record count, the caller's monotonic
    /// ingestion timestamp (stamped into the frame header), and the optional
    /// record time range feeding the per-file accumulator — see [`FrameMeta`].
    pub fn write_frame(
        &mut self,
        part_key: u64,
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
        self.get_or_create(part_key, content_meta)
            .write_frame(data, meta)
    }

    /// Get or lazily create the writer for `part_key`. On first use the stream
    /// records `content_meta` in each file's header; later frames for the same
    /// key reuse the already-recorded blob.
    fn get_or_create(&mut self, part_key: u64, content_meta: &[u8]) -> &mut Stream {
        self.streams.entry(part_key).or_insert_with(|| {
            Stream::new(
                Arc::clone(&self.dir),
                self.machine_id,
                self.boot_id,
                self.stamp,
                self.config.clone(),
                SeqCounter(Arc::clone(&self.seq)),
                part_key,
                content_meta.to_vec(),
            )
        })
    }

    /// Drain pending events from all streams.
    pub fn take_all_events(&mut self) -> Vec<FileEvent> {
        let mut events = Vec::new();
        for stream in self.streams.values_mut() {
            events.append(&mut stream.take_events());
        }
        events
    }

    /// Sync all active streams to disk.
    pub fn sync_all(&mut self) -> Result<()> {
        for stream in self.streams.values_mut() {
            stream.sync()?;
        }
        Ok(())
    }

    /// Shut down all streams, returning any remaining events.
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
        Writer::new(
            tmp,
            Config::default(),
            seq,
            crate::FileStamp {
                pipeline_id: 0,
                payload_format: 7,
            },
        )
        .unwrap()
    }

    /// A distinct opaque `part_key` per label — distinct labels give distinct
    /// keys, so a test can partition by key without hand-picking values.
    fn pk(label: u64) -> u64 {
        crate::opaque_part_key("ns", &format!("svc{label}"))
    }

    #[test]
    fn rejects_reserved_payload_format_zero() {
        // 0 is the reserved "unspecified" id; construction must refuse it in
        // every build profile, before any file can be written.
        let tmp = tempfile::tempdir().unwrap();
        let seq = Arc::new(SeqAllocator::ephemeral(0));
        match Writer::new(
            tmp.path(),
            Config::default(),
            seq,
            crate::FileStamp {
                pipeline_id: 0,
                payload_format: 0,
            },
        ) {
            Err(crate::Error::InvalidHeader(msg)) => {
                assert!(msg.contains("reserved"), "got: {msg}")
            }
            Err(e) => panic!("wrong error: {e:?}"),
            Ok(_) => panic!("payload_format 0 must be rejected"),
        }
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

        // Filenames carry each stream's distinct ns_hash.
        let mut hashes: Vec<u64> = wal_files
            .iter()
            .map(|e| FileId::parse(&e.path()).unwrap().part_key)
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
        // Frame whose logs all lacked time/observed timestamps — must
        // not regress the accumulator. (In production the ingestor would
        // synthesize a fallback range; this test exercises the defense-
        // in-depth ZERO/ZERO skip.)
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
}
