use std::fs::File;
use std::io::{BufReader, Read, Seek, SeekFrom};
use std::path::Path;

use file_registry::TimestampNs;

use crate::format::{COMPRESSION_LZ4, FRAME_ALIGNMENT, FRAME_HEADER_SIZE, FileHeader, HEADER_SIZE};
use crate::{Error, Result};

/// Reject frames claiming to be larger than 64 MiB.
const MAX_FRAME_PAYLOAD: usize = 64 * 1024 * 1024;

/// A single frame read from the WAL file.
pub struct Frame<'a> {
    /// Ingestion timestamp in nanoseconds since the Unix epoch.
    pub timestamp_ns: TimestampNs,
    /// Number of log entries in this frame.
    pub entry_count: u32,
    /// Decompressed payload data.
    pub data: &'a [u8],
}

/// Reads WAL files produced by [`Writer`](crate::Writer).
pub struct Reader {
    reader: BufReader<File>,
    header: FileHeader,
    compressed_buf: Vec<u8>,
    data_buf: Vec<u8>,
    /// Absolute byte offset of the next frame to read.
    position: u64,
    /// The durable read bound, or `None` for an unbounded read.
    ///
    /// `Some(end)` ([`open_range`](Self::open_range)): frames are yielded
    /// only while they fit fully below `end`; a frame crossing it is a
    /// torn tail past the durable boundary and stops the read **cleanly**
    /// (it is expected — the writer may have a partial frame there).
    ///
    /// `None` ([`open`](Self::open)): read to EOF. A frame whose payload
    /// is short — a *truncated or corrupt* file, not an expected torn
    /// tail — surfaces as an error from the payload read, never a silent
    /// short read. This preserves the pre-bounded-reader behavior for
    /// whole-file callers (the indexer), which have no entry-count
    /// cross-check to catch a silently-dropped frame.
    end_bound: Option<u64>,
}

impl Reader {
    /// Open a WAL file and read every frame, from the first frame to
    /// end-of-file.
    pub fn open(path: &Path) -> Result<Self> {
        Self::open_inner(path, HEADER_SIZE as u64, None)
    }

    /// Open a WAL file and read only the frames within the byte range
    /// `[start, end)`.
    ///
    /// This is how a query reads the durable, fully-written prefix of a
    /// file another process is still appending to (`end =
    /// File::valid_up_to`, the last fsync boundary), or a sub-range of a
    /// sealed file (chunk building). Both `start` and `end` must be
    /// **frame boundaries** — `HEADER_SIZE` or a frame end offset
    /// recorded from a prior read / a `Synced` event's `valid_up_to`;
    /// the reader cannot detect a mid-frame offset and would decode
    /// garbage. `start` defaults to the first frame when it equals
    /// `HEADER_SIZE`.
    ///
    /// Validations (the durable-prefix soundness checks): the file must
    /// physically contain `end` (`file_len >= end`), and `start` must
    /// lie in `[HEADER_SIZE, end]`. A frame is yielded only if it fits
    /// **fully** below `end`; the bytes beyond `end` may be a torn frame
    /// (the writer's buffer can flush mid-frame) and are never read.
    pub fn open_range(path: &Path, range: FrameRange) -> Result<Self> {
        Self::open_inner(path, range.start(), Some(range.end()))
    }

    fn open_inner(path: &Path, start: u64, end: Option<u64>) -> Result<Self> {
        let file = File::open(path)?;

        // Validate the durable bound against the physical file: it must
        // actually be present on disk. (Unbounded reads have nothing to
        // validate — they stop at EOF.)
        if let Some(end) = end {
            let file_len = file.metadata()?.len();
            if end > file_len {
                return Err(Error::Deserialization(format!(
                    "durable bound ({end} bytes) exceeds file length ({file_len} bytes)"
                )));
            }
        }

        if start < HEADER_SIZE as u64 || end.is_some_and(|e| start > e) {
            return Err(Error::Deserialization(format!(
                "start offset ({start}) outside the readable range (header={HEADER_SIZE}, end={end:?})"
            )));
        }
        // `start` must be a frame boundary; we can't fully verify that
        // (we don't know the boundaries without reading), but alignment
        // is necessary, and a dev-time assert catches the obvious misuse.
        debug_assert!(
            start == HEADER_SIZE as u64 || start % FRAME_ALIGNMENT as u64 == 0,
            "start offset {start} is not frame-aligned"
        );

        let mut reader = BufReader::new(file);

        let mut header_buf = [0u8; HEADER_SIZE];
        reader.read_exact(&mut header_buf)?;
        let header = FileHeader::from_bytes(&header_buf)?;

        // The header read leaves the cursor at `HEADER_SIZE`; seek only
        // when starting at a later frame boundary.
        if start > HEADER_SIZE as u64 {
            reader.seek(SeekFrom::Start(start))?;
        }

        Ok(Self {
            reader,
            header,
            compressed_buf: Vec::with_capacity(1024 * 1024),
            data_buf: Vec::with_capacity(1024 * 1024),
            position: start,
            end_bound: end,
        })
    }

    pub fn header(&self) -> &FileHeader {
        &self.header
    }

    /// Advise the kernel to drop the file's pages from the page cache.
    /// Call this after you're done reading the file.
    pub fn drop_cache(&self) {
        #[cfg(target_os = "linux")]
        {
            use nix::fcntl::{PosixFadviseAdvice, posix_fadvise};
            let _ = posix_fadvise(
                self.reader.get_ref(),
                0,
                0,
                PosixFadviseAdvice::POSIX_FADV_DONTNEED,
            );
        }
    }

    pub fn next_frame(&mut self) -> Result<Option<Frame<'_>>> {
        // Bounded read: stop cleanly when the next frame's header
        // wouldn't fit below the durable bound. `valid_up_to` is
        // frame-aligned, so in the normal case this fires exactly at the
        // last whole frame's end. Unbounded reads fall through to the
        // EOF/short-read handling below.
        if let Some(end) = self.end_bound {
            if self.position + FRAME_HEADER_SIZE as u64 > end {
                return Ok(None);
            }
        }

        let mut frame_header = [0u8; FRAME_HEADER_SIZE];
        match self.reader.read_exact(&mut frame_header) {
            Ok(()) => {}
            Err(e) if e.kind() == std::io::ErrorKind::UnexpectedEof => return Ok(None),
            Err(e) => return Err(e.into()),
        }

        let payload_len = u32::from_le_bytes(frame_header[0..4].try_into().unwrap()) as usize;
        let uncompressed_len = u32::from_le_bytes(frame_header[4..8].try_into().unwrap()) as usize;
        let entry_count = u32::from_le_bytes(frame_header[8..12].try_into().unwrap());
        let timestamp_ns = u64::from_le_bytes(frame_header[12..20].try_into().unwrap());
        let stored_crc = u32::from_le_bytes(frame_header[20..24].try_into().unwrap());

        if payload_len > MAX_FRAME_PAYLOAD {
            return Err(Error::Deserialization(format!(
                "frame payload ({payload_len} bytes) exceeds maximum ({MAX_FRAME_PAYLOAD} bytes)"
            )));
        }
        if uncompressed_len > MAX_FRAME_PAYLOAD {
            return Err(Error::Deserialization(format!(
                "uncompressed size ({uncompressed_len} bytes) exceeds maximum ({MAX_FRAME_PAYLOAD} bytes)"
            )));
        }

        let frame_bytes = FRAME_HEADER_SIZE + payload_len;
        let padding = (FRAME_ALIGNMENT - (frame_bytes % FRAME_ALIGNMENT)) % FRAME_ALIGNMENT;
        let total = (frame_bytes + padding) as u64;

        // Bounded read: the header fit, but the whole frame would cross
        // the durable bound — a torn tail past `valid_up_to`. Don't read
        // the partial payload; latch done (so a re-call also stops) and
        // stop cleanly, leaving the resulting short read for the caller's
        // entry-count cross-check. (For an unbounded read this branch is
        // skipped, so a truncated payload reaches the `read_exact` below
        // and errors — corruption is not silently dropped.)
        if let Some(end) = self.end_bound {
            if self.position + total > end {
                self.position = end;
                return Ok(None);
            }
        }

        self.compressed_buf.clear();
        self.compressed_buf.resize(payload_len, 0);
        self.reader.read_exact(&mut self.compressed_buf)?;

        if padding > 0 {
            let mut pad_buf = [0u8; FRAME_ALIGNMENT];
            self.reader.read_exact(&mut pad_buf[..padding])?;
        }
        self.position += total;

        if self.header.crc_enabled() {
            let mut hasher = crc32fast::Hasher::new();
            hasher.update(&(payload_len as u32).to_le_bytes());
            hasher.update(&(uncompressed_len as u32).to_le_bytes());
            hasher.update(&entry_count.to_le_bytes());
            hasher.update(&timestamp_ns.to_le_bytes());
            hasher.update(&self.compressed_buf);
            let actual_crc = hasher.finalize();
            if actual_crc != stored_crc {
                return Err(Error::CrcMismatch {
                    expected: stored_crc,
                    actual: actual_crc,
                });
            }
        }

        let lz4 = self.header.compression() == COMPRESSION_LZ4;
        if lz4 {
            // `resize` zero-fills the buffer before lz4 overwrites it —
            // a memset that is noise next to the decompression itself,
            // and it keeps the buffer initialized without `unsafe`
            // (the previous reserve + set_len tripped clippy's
            // `uninit_vec` deny). The buffer is reused across frames,
            // so the allocation amortizes either way.
            self.data_buf.clear();
            self.data_buf.resize(uncompressed_len, 0);
            let n = lz4_flex::block::decompress_into(&self.compressed_buf, &mut self.data_buf)
                .map_err(|e| Error::Decompression(e.to_string()))?;
            self.data_buf.truncate(n);
        } else {
            self.data_buf.clear();
            self.data_buf.extend_from_slice(&self.compressed_buf);
        }

        Ok(Some(Frame {
            timestamp_ns: TimestampNs(timestamp_ns),
            entry_count,
            data: &self.data_buf,
        }))
    }
}

/// One frame's position, read from its 24-byte header alone — no payload
/// decompression, no CRC. Produced by [`scan_frame_boundaries`].
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct FrameBoundary {
    /// Byte offset just past this frame (its end, and the start of the
    /// next frame). A valid `start` / `end` for [`Reader::open_range`].
    pub end_offset: u64,
    /// Log records in this frame.
    pub entry_count: u32,
}

/// A half-open `[start, end)` byte range into a WAL, with both offsets on
/// **frame boundaries** (`HEADER_SIZE`, a frame end offset, or a `Synced`
/// event's `valid_up_to`). A mid-frame offset can't be detected and would
/// decode garbage; this type names that contract so a range is passed as
/// one value rather than two loose offsets that could be swapped. The
/// frame-alignment itself is checked when the range is read (the reader
/// walks from `start`).
#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash)]
pub struct FrameRange {
    start: u64,
    end: u64,
}

impl FrameRange {
    /// A range over `[start, end)`. Requires `start <= end` (debug-checked).
    pub fn new(start: u64, end: u64) -> Self {
        debug_assert!(start <= end, "FrameRange start {start} exceeds end {end}");
        Self { start, end }
    }

    /// Byte offset of the first frame (inclusive).
    pub fn start(&self) -> u64 {
        self.start
    }

    /// Byte offset just past the last frame (exclusive).
    pub fn end(&self) -> u64 {
        self.end
    }
}

/// Walk the frame headers in `range` and report each whole frame's
/// boundary, **without decoding any payload**.
///
/// This is how a caller decides where to split a WAL into chunks: it
/// reads only the 24-byte header of each frame (for its `payload_len`
/// and `entry_count`) and seeks past the payload + padding to the next.
/// Cheap relative to indexing — no decompression, no CRC — so it can run
/// on the durable prefix of an active file to plan chunk boundaries
/// before paying to build any.
///
/// Same windowing and soundness rules as [`Reader::open_range`]: `start`
/// and `end` must be frame boundaries (`HEADER_SIZE` / a prior
/// `end_offset` / a `Synced` event's `valid_up_to`), the file must
/// physically contain `end`, and a frame is reported only if it fits
/// fully below `end` — a frame crossing the bound is a torn tail and
/// ends the scan. The returned boundaries are in file order; the last
/// one's `end_offset` is the durable extent actually covered (`<= end`,
/// and `== end` when `end` is frame-aligned, the normal case).
pub fn scan_frame_boundaries(path: &Path, range: FrameRange) -> Result<Vec<FrameBoundary>> {
    let (start, end) = (range.start(), range.end());
    let mut file = File::open(path)?;
    let file_len = file.metadata()?.len();
    if end > file_len {
        return Err(Error::Deserialization(format!(
            "durable bound ({end} bytes) exceeds file length ({file_len} bytes)"
        )));
    }
    if start < HEADER_SIZE as u64 || start > end {
        return Err(Error::Deserialization(format!(
            "start offset ({start}) outside the readable range (header={HEADER_SIZE}, end={end})"
        )));
    }
    debug_assert!(
        start == HEADER_SIZE as u64 || start % FRAME_ALIGNMENT as u64 == 0,
        "start offset {start} is not frame-aligned"
    );

    // Validate the file header (magic / version), then walk from `start`.
    let mut header_buf = [0u8; HEADER_SIZE];
    file.read_exact(&mut header_buf)?;
    FileHeader::from_bytes(&header_buf)?;
    if start > HEADER_SIZE as u64 {
        file.seek(SeekFrom::Start(start))?;
    }

    let mut boundaries = Vec::new();
    let mut position = start;
    let mut frame_header = [0u8; FRAME_HEADER_SIZE];
    loop {
        // The whole header must fit below the bound (mirrors
        // `next_frame`'s header-room guard).
        if position + FRAME_HEADER_SIZE as u64 > end {
            break;
        }
        match file.read_exact(&mut frame_header) {
            Ok(()) => {}
            Err(e) if e.kind() == std::io::ErrorKind::UnexpectedEof => break,
            Err(e) => return Err(e.into()),
        }

        let payload_len = u32::from_le_bytes(frame_header[0..4].try_into().unwrap()) as usize;
        let entry_count = u32::from_le_bytes(frame_header[8..12].try_into().unwrap());
        if payload_len > MAX_FRAME_PAYLOAD {
            return Err(Error::Deserialization(format!(
                "frame payload ({payload_len} bytes) exceeds maximum ({MAX_FRAME_PAYLOAD} bytes)"
            )));
        }

        let frame_bytes = FRAME_HEADER_SIZE + payload_len;
        let padding = (FRAME_ALIGNMENT - (frame_bytes % FRAME_ALIGNMENT)) % FRAME_ALIGNMENT;
        let total = (frame_bytes + padding) as u64;
        // Frame crosses the bound: a torn tail past the durable prefix.
        if position + total > end {
            break;
        }

        position += total;
        boundaries.push(FrameBoundary {
            end_offset: position,
            entry_count,
        });

        // Skip the payload + padding (we already consumed the header).
        file.seek(SeekFrom::Start(position))?;
    }

    Ok(boundaries)
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::{Config, SeqAllocator, Writer};
    use std::sync::Arc;

    /// Write one frame per payload, syncing after each so every frame
    /// boundary surfaces as a `Synced { valid_up_to }`. Returns the WAL
    /// path and the cumulative `valid_up_to` after each frame (i.e. the
    /// byte offset of the end of frame `i`).
    fn write_frames(dir: &Path, payloads: &[&[u8]]) -> (std::path::PathBuf, Vec<u64>) {
        let seq = Arc::new(SeqAllocator::ephemeral(0));
        let (machine_id, instance_id) = crate::test_identity();
        let mut writer = Writer::new(
            dir,
            Config::default(),
            seq,
            crate::FileStamp {
                pipeline_id: 0,
                payload_format: 7,
            },
            machine_id,
            instance_id,
        )
        .unwrap();
        let mut bounds = Vec::new();
        for (i, payload) in payloads.iter().enumerate() {
            writer
                .write_frame(
                    crate::opaque_part_key("ns", "svc"),
                    &[],
                    payload,
                    crate::FrameMeta {
                        entry_count: 1,
                        ingestion_ns: TimestampNs(i as u64 + 1),
                        log_ts_range: None,
                    },
                )
                .unwrap();
            writer.sync_all().unwrap();
            let valid_up_to = writer
                .take_all_events()
                .iter()
                .rev()
                .find_map(|e| match e {
                    crate::FileEvent::Synced { valid_up_to, .. } => Some(valid_up_to.0),
                    _ => None,
                })
                .expect("a Synced event after sync_all");
            bounds.push(valid_up_to);
        }
        writer.shutdown_all().unwrap();

        let path = std::fs::read_dir(dir)
            .unwrap()
            .filter_map(|e| e.ok())
            .map(|e| e.path())
            .find(|p| p.extension().is_some_and(|x| x == "wal"))
            .expect("a .wal file");
        (path, bounds)
    }

    fn collect(reader: &mut Reader) -> Vec<(u32, Vec<u8>)> {
        let mut out = Vec::new();
        while let Some(frame) = reader.next_frame().unwrap() {
            out.push((frame.entry_count, frame.data.to_vec()));
        }
        out
    }

    #[test]
    fn open_reads_every_frame_to_eof() {
        let dir = tempfile::tempdir().unwrap();
        let (path, _) = write_frames(dir.path(), &[b"alpha", b"bravo", b"charlie"]);

        let mut reader = Reader::open(&path).unwrap();
        // The writer's payload_format (7 in the test helper) survives the
        // writer → disk → reader path.
        assert_eq!(reader.header().payload_format, 7);
        let frames = collect(&mut reader);
        assert_eq!(
            frames,
            vec![
                (1, b"alpha".to_vec()),
                (1, b"bravo".to_vec()),
                (1, b"charlie".to_vec()),
            ]
        );
    }

    #[test]
    fn open_range_stops_at_the_bound() {
        let dir = tempfile::tempdir().unwrap();
        let (path, bounds) = write_frames(dir.path(), &[b"alpha", b"bravo", b"charlie"]);

        // Bound at the end of frame 1 → exactly the first two frames.
        let mut reader =
            Reader::open_range(&path, FrameRange::new(HEADER_SIZE as u64, bounds[1])).unwrap();
        let frames = collect(&mut reader);
        assert_eq!(frames, vec![(1, b"alpha".to_vec()), (1, b"bravo".to_vec())]);
    }

    #[test]
    fn open_range_starts_at_a_frame_offset() {
        let dir = tempfile::tempdir().unwrap();
        let (path, bounds) = write_frames(dir.path(), &[b"alpha", b"bravo", b"charlie"]);

        // Start at the end of frame 0 (= start of frame 1), read to EOF
        // of the durable prefix (end of frame 2).
        let mut reader = Reader::open_range(&path, FrameRange::new(bounds[0], bounds[2])).unwrap();
        let frames = collect(&mut reader);
        assert_eq!(
            frames,
            vec![(1, b"bravo".to_vec()), (1, b"charlie".to_vec())]
        );
    }

    #[test]
    fn open_range_header_fits_but_payload_crosses_bound() {
        let dir = tempfile::tempdir().unwrap();
        let (path, bounds) = write_frames(dir.path(), &[b"alpha", b"bravo", b"charlie"]);

        // A bound exactly one frame-header past frame 2's start: frame
        // 2's *header* fits below the bound (so it is read and parsed),
        // but its payload would cross the bound — exercising the
        // payload-fit latch, not the header-room guard. Only the first
        // two frames are yielded; the partial tail is never read.
        let bound = bounds[1] + FRAME_HEADER_SIZE as u64;
        let mut reader =
            Reader::open_range(&path, FrameRange::new(HEADER_SIZE as u64, bound)).unwrap();
        let frames = collect(&mut reader);
        assert_eq!(frames, vec![(1, b"alpha".to_vec()), (1, b"bravo".to_vec())]);
        // Re-entrancy: the latch holds — a further call still stops.
        assert!(reader.next_frame().unwrap().is_none());
    }

    #[test]
    fn open_to_eof_errors_on_a_truncated_payload() {
        // A whole-file read (no durable bound) must not silently drop a
        // torn/corrupt final frame: a header that promises a payload the
        // file doesn't contain is an error, not a clean stop. (The
        // bounded reader treats the same shape as an expected torn tail;
        // `open` must not, since its callers have no count cross-check.)
        let dir = tempfile::tempdir().unwrap();
        let (path, bounds) = write_frames(dir.path(), &[b"alpha", b"bravo"]);

        // Truncate mid-payload of frame 1: keep its 24-byte header plus
        // 2 payload bytes, drop the rest.
        let truncated_len = bounds[0] + FRAME_HEADER_SIZE as u64 + 2;
        std::fs::OpenOptions::new()
            .write(true)
            .open(&path)
            .unwrap()
            .set_len(truncated_len)
            .unwrap();

        let mut reader = Reader::open(&path).unwrap();
        // Frame 0 is intact.
        assert_eq!(
            reader.next_frame().unwrap().map(|f| f.data.to_vec()),
            Some(b"alpha".to_vec())
        );
        // Frame 1's payload is truncated → error, not Ok(None).
        assert!(reader.next_frame().is_err());
    }

    #[test]
    fn open_range_rejects_bound_past_eof() {
        let dir = tempfile::tempdir().unwrap();
        let (path, bounds) = write_frames(dir.path(), &[b"alpha"]);
        match Reader::open_range(&path, FrameRange::new(HEADER_SIZE as u64, bounds[0] + 4096)) {
            Err(Error::Deserialization(_)) => {}
            Err(e) => panic!("wrong error: {e:?}"),
            Ok(_) => panic!("expected a bound-past-EOF error"),
        }
    }

    #[test]
    fn open_range_rejects_start_below_header() {
        let dir = tempfile::tempdir().unwrap();
        let (path, bounds) = write_frames(dir.path(), &[b"alpha"]);
        // A start below the header is a valid `FrameRange` structurally but
        // an invalid read window; `open_range` rejects it.
        assert!(Reader::open_range(&path, FrameRange::new(0, bounds[0])).is_err());
    }

    // `start > end` is rejected at construction now (it can't reach
    // `open_range`). Debug-only: the `debug_assert` is compiled out under
    // `--release`, where the panic wouldn't fire.
    #[cfg(debug_assertions)]
    #[test]
    #[should_panic(expected = "exceeds end")]
    fn frame_range_rejects_inverted() {
        let _ = FrameRange::new(100, 50);
    }

    #[test]
    fn open_range_empty_window_yields_nothing() {
        let dir = tempfile::tempdir().unwrap();
        let (path, _) = write_frames(dir.path(), &[b"alpha", b"bravo"]);
        // start == end (at the header): a zero-length durable prefix.
        let mut reader = Reader::open_range(
            &path,
            FrameRange::new(HEADER_SIZE as u64, HEADER_SIZE as u64),
        )
        .unwrap();
        assert!(reader.next_frame().unwrap().is_none());
    }

    /// Write frames with the given entry counts (one frame each),
    /// syncing after each. Returns the path and per-frame end offsets.
    fn write_counted_frames(dir: &Path, counts: &[usize]) -> (std::path::PathBuf, Vec<u64>) {
        let seq = Arc::new(SeqAllocator::ephemeral(0));
        let (machine_id, instance_id) = crate::test_identity();
        let mut writer = Writer::new(
            dir,
            Config::default(),
            seq,
            crate::FileStamp {
                pipeline_id: 0,
                payload_format: 7,
            },
            machine_id,
            instance_id,
        )
        .unwrap();
        let mut bounds = Vec::new();
        for (i, &count) in counts.iter().enumerate() {
            writer
                .write_frame(
                    crate::opaque_part_key("ns", "svc"),
                    &[],
                    b"payload",
                    crate::FrameMeta {
                        entry_count: count,
                        ingestion_ns: TimestampNs(i as u64 + 1),
                        log_ts_range: None,
                    },
                )
                .unwrap();
            writer.sync_all().unwrap();
            let valid_up_to = writer
                .take_all_events()
                .iter()
                .rev()
                .find_map(|e| match e {
                    crate::FileEvent::Synced { valid_up_to, .. } => Some(valid_up_to.0),
                    _ => None,
                })
                .unwrap();
            bounds.push(valid_up_to);
        }
        writer.shutdown_all().unwrap();
        let path = std::fs::read_dir(dir)
            .unwrap()
            .filter_map(|e| e.ok())
            .map(|e| e.path())
            .find(|p| p.extension().is_some_and(|x| x == "wal"))
            .unwrap();
        (path, bounds)
    }

    #[test]
    fn scan_boundaries_reports_offsets_and_counts() {
        let dir = tempfile::tempdir().unwrap();
        let (path, bounds) = write_counted_frames(dir.path(), &[5, 3, 7]);
        let file_len = std::fs::metadata(&path).unwrap().len();

        let scanned =
            scan_frame_boundaries(&path, FrameRange::new(HEADER_SIZE as u64, file_len)).unwrap();
        assert_eq!(
            scanned,
            vec![
                FrameBoundary {
                    end_offset: bounds[0],
                    entry_count: 5
                },
                FrameBoundary {
                    end_offset: bounds[1],
                    entry_count: 3
                },
                FrameBoundary {
                    end_offset: bounds[2],
                    entry_count: 7
                },
            ]
        );
        // The scan never decodes payloads; the offsets it reports are
        // valid `open_range` boundaries — read the chunk they delimit.
        let mut reader =
            Reader::open_range(&path, FrameRange::new(HEADER_SIZE as u64, bounds[1])).unwrap();
        let frames = collect(&mut reader);
        assert_eq!(
            frames.iter().map(|(c, _)| *c).collect::<Vec<_>>(),
            vec![5, 3]
        );
    }

    #[test]
    fn scan_boundaries_respects_start_and_end() {
        let dir = tempfile::tempdir().unwrap();
        let (path, bounds) = write_counted_frames(dir.path(), &[5, 3, 7]);

        // A sub-window covering frames 1 and 2 only.
        let scanned = scan_frame_boundaries(&path, FrameRange::new(bounds[0], bounds[2])).unwrap();
        assert_eq!(
            scanned,
            vec![
                FrameBoundary {
                    end_offset: bounds[1],
                    entry_count: 3
                },
                FrameBoundary {
                    end_offset: bounds[2],
                    entry_count: 7
                },
            ]
        );
    }

    #[test]
    fn scan_boundaries_stops_before_a_frame_crossing_the_bound() {
        let dir = tempfile::tempdir().unwrap();
        let (path, bounds) = write_counted_frames(dir.path(), &[5, 3, 7]);

        // Bound one header past frame 2's start: its header fits but its
        // payload crosses, so the scan reports only frames 0 and 1.
        let bound = bounds[1] + FRAME_HEADER_SIZE as u64;
        let scanned =
            scan_frame_boundaries(&path, FrameRange::new(HEADER_SIZE as u64, bound)).unwrap();
        assert_eq!(
            scanned.iter().map(|b| b.entry_count).collect::<Vec<_>>(),
            vec![5, 3]
        );
        assert_eq!(scanned.last().unwrap().end_offset, bounds[1]);
    }

    #[test]
    fn scan_boundaries_validates_the_window() {
        let dir = tempfile::tempdir().unwrap();
        let (path, bounds) = write_counted_frames(dir.path(), &[5]);
        assert!(
            scan_frame_boundaries(&path, FrameRange::new(HEADER_SIZE as u64, bounds[0] + 4096))
                .is_err()
        );
        assert!(scan_frame_boundaries(&path, FrameRange::new(0, bounds[0])).is_err());
    }

    #[test]
    fn scan_boundaries_empty_range_yields_nothing() {
        let dir = tempfile::tempdir().unwrap();
        let (path, _) = write_counted_frames(dir.path(), &[5, 3]);
        // start == end (at the header): a zero-length durable prefix —
        // the header-room guard fires immediately, no frames reported.
        let scanned = scan_frame_boundaries(
            &path,
            FrameRange::new(HEADER_SIZE as u64, HEADER_SIZE as u64),
        )
        .unwrap();
        assert!(scanned.is_empty());
    }
}
