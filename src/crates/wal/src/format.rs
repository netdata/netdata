use serde::{Deserialize, Serialize};

use file_registry::{ByteSize, FileId, ServiceStream, TenantId, TimestampNs};

// -- Constants ----------------------------------------------------------

/// Magic bytes at the start of every WAL file.
pub const MAGIC: [u8; 4] = *b"NWAL";

/// Current format version. v2 records the file's `ServiceStream`
/// `(service.namespace, service.name)` in the header. There is no v1
/// back-compat: a v1 file is rejected (the OTel logs feature is
/// experimental and WAL files are short-lived).
pub const FORMAT_VERSION: u16 = 2;

/// Total size of the file header in bytes (one 4 KiB page).
pub const HEADER_SIZE: usize = 4096;

/// Max stored length (bytes) for each stream field in the header. A longer
/// value is truncated for display only — the partition key is the filename's
/// `ns_hash`, which is unaffected. Two fields fit easily in the 4 KiB header.
pub const MAX_STREAM_FIELD_BYTES: usize = 256;

/// Bit 0: CRC32 checksums are present in batch frames.
pub const FLAG_CRC_ENABLED: u16 = 1 << 0;

// Bits 1-2: compression algorithm for batch payloads.
pub const COMPRESSION_MASK: u16 = 0b110;
pub const COMPRESSION_LZ4: u16 = 0b000;
pub const COMPRESSION_NONE: u16 = 0b010;

/// Frame header size: `[u32 payload_len] [u32 uncompressed_len] [u32 entry_count] [u64 timestamp_ns] [u32 crc32]`.
pub const FRAME_HEADER_SIZE: usize = 24;

/// Alignment boundary for frames within the file.
pub const FRAME_ALIGNMENT: usize = 8;

// -- File header --------------------------------------------------------

/// Fixed-size file header written once when a WAL file is created.
///
/// Layout: `MAGIC(4)`, `version(2)`, `flags(2)`, `created_at(8)`, then the
/// stream as two length-prefixed UTF-8 fields — `namespace_len(2) namespace`,
/// `name_len(2) name` — each capped at [`MAX_STREAM_FIELD_BYTES`]. The rest of
/// the 4 KiB page is zero.
#[derive(Debug, Clone)]
pub struct FileHeader {
    pub version: u16,
    pub flags: u16,
    pub created_at: u64,
    /// The single stream this file holds. Recorded so the file's
    /// `(namespace, name)` is available cheaply (recovery, the stream
    /// selector) without decoding any frame.
    pub stream: ServiceStream,
}

impl FileHeader {
    pub fn crc_enabled(&self) -> bool {
        self.flags & FLAG_CRC_ENABLED != 0
    }

    pub fn compression(&self) -> u16 {
        self.flags & COMPRESSION_MASK
    }

    pub fn to_bytes(&self) -> [u8; HEADER_SIZE] {
        let mut buf = [0u8; HEADER_SIZE];
        buf[0..4].copy_from_slice(&MAGIC);
        buf[4..6].copy_from_slice(&self.version.to_le_bytes());
        buf[6..8].copy_from_slice(&self.flags.to_le_bytes());
        buf[8..16].copy_from_slice(&self.created_at.to_le_bytes());

        let mut off = 16;
        for field in [self.stream.namespace.as_str(), self.stream.name.as_str()] {
            let bytes = truncate_field(field);
            buf[off..off + 2].copy_from_slice(&(bytes.len() as u16).to_le_bytes());
            off += 2;
            buf[off..off + bytes.len()].copy_from_slice(bytes);
            off += bytes.len();
        }
        // The two ≤256-byte fields always fit (16 + 2 + 256 + 2 + 256 = 532);
        // guard the header budget so a future added field fails loudly here
        // rather than silently colliding with frame bytes.
        debug_assert!(off <= HEADER_SIZE, "WAL header overflow: {off} > {HEADER_SIZE}");
        buf
    }

    pub fn from_bytes(buf: &[u8; HEADER_SIZE]) -> crate::Result<Self> {
        if buf[0..4] != MAGIC {
            return Err(crate::Error::InvalidHeader(format!(
                "bad magic: {:?}",
                &buf[0..4]
            )));
        }
        let version = u16::from_le_bytes([buf[4], buf[5]]);
        if version != FORMAT_VERSION {
            return Err(crate::Error::UnsupportedVersion(version));
        }
        let flags = u16::from_le_bytes([buf[6], buf[7]]);
        let compression = flags & COMPRESSION_MASK;
        if compression != COMPRESSION_LZ4 && compression != COMPRESSION_NONE {
            return Err(crate::Error::UnsupportedCompression(
                (compression >> 1) as u8,
            ));
        }
        let created_at = u64::from_le_bytes(buf[8..16].try_into().unwrap());

        let mut off = 16;
        let namespace = read_field(buf, &mut off)?;
        let name = read_field(buf, &mut off)?;

        Ok(Self {
            version,
            flags,
            created_at,
            stream: ServiceStream::new(namespace, name),
        })
    }
}

/// Truncate a stream field to at most [`MAX_STREAM_FIELD_BYTES`], on a UTF-8
/// char boundary so the stored bytes are always valid UTF-8.
fn truncate_field(s: &str) -> &[u8] {
    if s.len() <= MAX_STREAM_FIELD_BYTES {
        return s.as_bytes();
    }
    let mut end = MAX_STREAM_FIELD_BYTES;
    while !s.is_char_boundary(end) {
        end -= 1;
    }
    &s.as_bytes()[..end]
}

/// Read one length-prefixed UTF-8 stream field at `*off`, advancing `off`.
/// Bounded by [`MAX_STREAM_FIELD_BYTES`], so the two fields stay well within
/// the 4 KiB header.
fn read_field(buf: &[u8; HEADER_SIZE], off: &mut usize) -> crate::Result<String> {
    let len = u16::from_le_bytes([buf[*off], buf[*off + 1]]) as usize;
    if len > MAX_STREAM_FIELD_BYTES {
        return Err(crate::Error::InvalidHeader(format!(
            "stream field length {len} exceeds {MAX_STREAM_FIELD_BYTES}"
        )));
    }
    let start = *off + 2;
    let end = start + len;
    let s = std::str::from_utf8(&buf[start..end])
        .map_err(|e| crate::Error::InvalidHeader(format!("invalid UTF-8 in stream field: {e}")))?;
    *off = end;
    Ok(s.to_string())
}

// -- Events -------------------------------------------------------------

/// Events produced by the WAL writer during file lifecycle operations.
///
/// `min_timestamp_ns` / `max_timestamp_ns` carry the **log-data** time
/// range accumulated so far for the file — derived from each frame's
/// per-row OTel timestamps as supplied by the caller of
/// `Writer::write_frame`. They are *not* wall-clock frame-arrival times.
/// `TimestampNs::ZERO` on both means "no log-data timestamps observed
/// yet" (e.g. all frames so far had logs missing both `time_unix_nano`
/// and `observed_time_unix_nano`).
#[derive(Debug, Clone, Serialize, Deserialize)]
pub enum FileEvent {
    Created {
        file_id: FileId,
        created_at_ns: TimestampNs,
        /// The stream this file holds (one stream per file), so the registry
        /// can name it without decoding frames.
        stream: ServiceStream,
    },
    Synced {
        file_id: FileId,
        valid_up_to: ByteSize,
        frame_count: u64,
        entry_count: u64,
        /// Earliest log-data timestamp accumulated for this file so far.
        min_timestamp_ns: TimestampNs,
        /// Latest log-data timestamp accumulated for this file so far.
        max_timestamp_ns: TimestampNs,
    },
    Closed {
        file_id: FileId,
        frame_count: u64,
        /// Earliest log-data timestamp in this file (final value).
        min_timestamp_ns: TimestampNs,
        /// Latest log-data timestamp in this file (final value).
        max_timestamp_ns: TimestampNs,
        size: ByteSize,
    },
}

/// A sequenced file event sent over IPC.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Message {
    pub frame_seq: u64,
    pub tenant_id: TenantId,
    pub event: FileEvent,
}

#[cfg(test)]
mod tests {
    use super::*;

    fn header(stream: ServiceStream) -> FileHeader {
        FileHeader {
            version: FORMAT_VERSION,
            flags: 0,
            created_at: 12345,
            stream,
        }
    }

    #[test]
    fn header_roundtrips_stream() {
        for s in [
            ServiceStream::new("prod", "api"),
            // Absent namespace is stored as an empty string.
            ServiceStream::new("", "api"),
            ServiceStream::new("", ""),
        ] {
            let h = header(s.clone());
            let parsed = FileHeader::from_bytes(&h.to_bytes()).unwrap();
            assert_eq!(parsed.version, FORMAT_VERSION);
            assert_eq!(parsed.created_at, 12345);
            assert_eq!(parsed.stream, s);
        }
    }

    #[test]
    fn header_truncates_oversize_field_on_char_boundary() {
        // A 4-byte char repeated past the cap; truncation must land on a char
        // boundary so the stored bytes stay valid UTF-8.
        let long_name = "🦀".repeat(100); // 400 bytes
        let h = header(ServiceStream::new("ns", long_name));
        let parsed = FileHeader::from_bytes(&h.to_bytes()).unwrap();
        assert!(parsed.stream.name.len() <= MAX_STREAM_FIELD_BYTES);
        // Whole crabs only — 256 / 4 = 64 of them.
        assert_eq!(parsed.stream.name, "🦀".repeat(64));
        assert_eq!(parsed.stream.namespace, "ns");
    }

    #[test]
    fn header_rejects_v1() {
        // A v1 header (no stream fields) is hard-rejected — no back-compat.
        let mut buf = [0u8; HEADER_SIZE];
        buf[0..4].copy_from_slice(&MAGIC);
        buf[4..6].copy_from_slice(&1u16.to_le_bytes());
        let err = FileHeader::from_bytes(&buf).unwrap_err();
        assert!(matches!(err, crate::Error::UnsupportedVersion(1)));
    }
}
