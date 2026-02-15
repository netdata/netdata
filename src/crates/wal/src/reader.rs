use std::fs::File;
use std::io::{BufReader, Read};
use std::path::Path;

use crate::format::{COMPRESSION_LZ4, FRAME_ALIGNMENT, FRAME_HEADER_SIZE, FileHeader, HEADER_SIZE};
use crate::types::TimestampNs;
use crate::{Error, Result};

/// Reject frames claiming to be larger than 64 MiB.
const MAX_FRAME_PAYLOAD: usize = 64 * 1024 * 1024;

/// A single frame read from the WAL file.
pub struct WalFrame<'a> {
    /// Ingestion timestamp in nanoseconds since the Unix epoch.
    pub timestamp_ns: TimestampNs,
    /// Number of log entries in this frame.
    pub entry_count: u32,
    /// Decompressed payload data.
    pub data: &'a [u8],
}

/// Reads WAL files produced by [`WalWriter`](crate::WalWriter).
pub struct WalReader {
    reader: BufReader<File>,
    header: FileHeader,
    compressed_buf: Vec<u8>,
    data_buf: Vec<u8>,
}

impl WalReader {
    pub fn open(path: &Path) -> Result<Self> {
        let file = File::open(path)?;
        let mut reader = BufReader::new(file);

        let mut header_buf = [0u8; HEADER_SIZE];
        reader.read_exact(&mut header_buf)?;
        let header = FileHeader::from_bytes(&header_buf)?;

        Ok(Self {
            reader,
            header,
            compressed_buf: Vec::with_capacity(1024 * 1024),
            data_buf: Vec::with_capacity(1024 * 1024),
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

    pub fn next_frame(&mut self) -> Result<Option<WalFrame<'_>>> {
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

        self.compressed_buf.clear();
        self.compressed_buf.resize(payload_len, 0);
        self.reader.read_exact(&mut self.compressed_buf)?;

        let frame_bytes = FRAME_HEADER_SIZE + payload_len;
        let padding = (FRAME_ALIGNMENT - (frame_bytes % FRAME_ALIGNMENT)) % FRAME_ALIGNMENT;
        if padding > 0 {
            let mut pad_buf = [0u8; FRAME_ALIGNMENT];
            self.reader.read_exact(&mut pad_buf[..padding])?;
        }

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
            self.data_buf.clear();
            self.data_buf.reserve(uncompressed_len);
            // SAFETY: decompress_into writes all output bytes before they are read.
            // We set the length so the slice is large enough, then truncate to actual output.
            unsafe {
                self.data_buf.set_len(uncompressed_len);
            }
            let n = lz4_flex::block::decompress_into(&self.compressed_buf, &mut self.data_buf)
                .map_err(|e| Error::Decompression(e.to_string()))?;
            self.data_buf.truncate(n);
        } else {
            self.data_buf.clear();
            self.data_buf.extend_from_slice(&self.compressed_buf);
        }

        Ok(Some(WalFrame {
            timestamp_ns: TimestampNs(timestamp_ns),
            entry_count,
            data: &self.data_buf,
        }))
    }
}
