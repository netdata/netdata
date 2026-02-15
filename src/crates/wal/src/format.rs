use serde::{Deserialize, Serialize};

use crate::types::{ByteSize, FileId, TimestampNs};

// -- Constants ----------------------------------------------------------

/// Magic bytes at the start of every WAL file.
pub const MAGIC: [u8; 4] = *b"NWAL";

/// Current format version.
pub const FORMAT_VERSION: u16 = 1;

/// Total size of the file header in bytes (one 4 KiB page).
pub const HEADER_SIZE: usize = 4096;

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
#[derive(Debug, Clone, Copy)]
pub struct FileHeader {
    pub version: u16,
    pub flags: u16,
    pub created_at: u64,
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
        Ok(Self {
            version,
            flags,
            created_at,
        })
    }
}

// -- Events -------------------------------------------------------------

/// Events produced by the WAL writer during file lifecycle operations.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub enum WalEvent {
    FileCreated {
        id: FileId,
        created_at_ns: TimestampNs,
    },
    FileSynced {
        id: FileId,
        valid_up_to: ByteSize,
        frame_count: u64,
        entry_count: u64,
    },
    FileCompleted {
        id: FileId,
        frame_count: u64,
        min_timestamp_ns: TimestampNs,
        max_timestamp_ns: TimestampNs,
        size: ByteSize,
    },
}

/// A sequenced WAL event sent over IPC.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct WalMessage {
    pub seq: u64,
    pub event: WalEvent,
}
