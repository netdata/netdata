use std::fs::{File, OpenOptions};
use std::io::{BufWriter, Write};
use std::path::PathBuf;
use std::sync::Arc;
use std::sync::atomic::{AtomicU64, Ordering};

use crate::Result;
use crate::clock::MonotonicClock;
use crate::config::Config;
use crate::format::{
    COMPRESSION_NONE, FLAG_CRC_ENABLED, FORMAT_VERSION, FRAME_ALIGNMENT, FRAME_HEADER_SIZE,
    FileHeader, HEADER_SIZE, WalEvent,
};
use crate::types::{ByteSize, FileId, TimestampNs};
use crate::waldir::WalDir;

struct ActiveFile {
    id: FileId,
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

/// Source of sequence numbers for file creation.
enum SeqSource {
    /// Writer owns its own counter (standalone mode).
    Local(u64),
    /// Writer shares a counter with other writers.
    Shared(Arc<AtomicU64>),
}

impl SeqSource {
    fn next(&mut self) -> u64 {
        match self {
            SeqSource::Local(seq) => {
                *seq += 1;
                *seq
            }
            SeqSource::Shared(seq) => seq.fetch_add(1, Ordering::Relaxed) + 1,
        }
    }
}

pub struct WalWriter {
    dir: WalDir,
    config: Config,
    clock: MonotonicClock,
    active: Option<ActiveFile>,
    seq_source: SeqSource,
    ns_hash: u64,
    pending_events: Vec<WalEvent>,
}

impl WalWriter {
    /// Create a new WAL writer for a specific service (identified by `ns_hash`).
    ///
    /// Scans the directory to find the highest existing sequence number.
    /// For multi-writer setups, prefer [`with_shared_seq`] to avoid
    /// redundant directory scans.
    pub fn new(dir: WalDir, config: Config, ns_hash: u64) -> Result<Self> {
        std::fs::create_dir_all(dir.path())?;
        let file_seq = dir.scan_max_sequence()?;

        Ok(Self {
            dir,
            config,
            clock: MonotonicClock::new(),
            active: None,
            seq_source: SeqSource::Local(file_seq),
            ns_hash,
            pending_events: Vec::new(),
        })
    }

    /// Create a new WAL writer with an externally managed sequence counter.
    ///
    /// Used when multiple writers share a directory and need a globally
    /// unique, monotonically increasing sequence number.
    pub fn with_shared_seq(
        dir: WalDir,
        config: Config,
        seq: Arc<AtomicU64>,
        ns_hash: u64,
    ) -> Self {
        Self {
            dir,
            config,
            clock: MonotonicClock::new(),
            active: None,
            seq_source: SeqSource::Shared(seq),
            ns_hash,
            pending_events: Vec::new(),
        }
    }

    pub fn write_frame(&mut self, data: &[u8], log_entry_count: usize) -> Result<u64> {
        if self.should_rotate_with(log_entry_count as u64) {
            self.sync()?;
            self.complete_active_file();
        }

        self.ensure_file()?;

        let ts = TimestampNs(self.clock.now_ns());

        let compressed = if self.config.compression_lz4() {
            lz4_flex::block::compress(data)
        } else {
            data.to_vec()
        };

        let payload_len = compressed.len() as u32;
        let uncompressed_len = data.len() as u32;
        let entry_count = log_entry_count as u32;

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
        active.log_entry_count += log_entry_count as u64;
        active.bytes_written = ByteSize(active.bytes_written.0 + (frame_bytes + padding) as u64);
        if active.first_frame_at_ns.is_none() {
            active.first_frame_at_ns = Some(ts);
        }
        if active.min_timestamp_ns == TimestampNs::ZERO || ts < active.min_timestamp_ns {
            active.min_timestamp_ns = ts;
        }
        if ts > active.max_timestamp_ns {
            active.max_timestamp_ns = ts;
        }

        Ok(frame_offset)
    }

    pub fn sync(&mut self) -> Result<()> {
        if let Some(active) = &mut self.active {
            active.writer.flush()?;
            active.writer.get_ref().sync_all()?;

            self.pending_events.push(WalEvent::FileSynced {
                id: active.id,
                valid_up_to: active.bytes_written,
                frame_count: active.frame_count,
                entry_count: active.log_entry_count,
            });
        }
        Ok(())
    }

    pub fn shutdown(&mut self) -> Result<Vec<WalEvent>> {
        self.sync()?;
        self.complete_active_file();
        Ok(self.take_events())
    }

    pub fn take_events(&mut self) -> Vec<WalEvent> {
        std::mem::take(&mut self.pending_events)
    }

    fn ensure_file(&mut self) -> Result<()> {
        if self.active.is_some() {
            return Ok(());
        }

        let file_seq = self.seq_source.next();
        let id = FileId::new(
            self.dir.machine_id(),
            self.dir.boot_id(),
            file_seq,
            self.ns_hash,
        );
        let path = self.dir.wal_path(id);

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

        let created_at_ns = TimestampNs(self.clock.now_ns());
        let header = FileHeader {
            version: FORMAT_VERSION,
            flags,
            created_at: created_at_ns.0,
        };
        writer.write_all(&header.to_bytes())?;
        writer.flush()?;

        fsync_dir(self.dir.path())?;

        self.pending_events
            .push(WalEvent::FileCreated { id, created_at_ns });

        self.active = Some(ActiveFile {
            id,
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

    fn should_rotate_with(&self, incoming_entries: u64) -> bool {
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
            let now = self.clock.last_ns;
            let elapsed_ns = now.saturating_sub(first_frame_at.0);
            if elapsed_ns >= max_dur.as_nanos() as u64 {
                return true;
            }
        }
        false
    }

    fn complete_active_file(&mut self) {
        if let Some(active) = self.active.take() {
            self.pending_events.push(WalEvent::FileCompleted {
                id: active.id,
                frame_count: active.frame_count,
                min_timestamp_ns: active.min_timestamp_ns,
                max_timestamp_ns: active.max_timestamp_ns,
                size: active.bytes_written,
            });
        }
    }
}

const FRAME_ALIGNMENT_HEADER: usize = FRAME_HEADER_SIZE;

impl Drop for WalWriter {
    fn drop(&mut self) {
        self.complete_active_file();
    }
}

impl Config {
    pub(crate) fn compression_lz4(&self) -> bool {
        self.compression_enabled
    }
}

fn fsync_dir(dir: &std::path::Path) -> Result<()> {
    let dir_file = File::open(dir)?;
    dir_file.sync_all()?;
    Ok(())
}
