//! Wire envelope and codec for the netipc protocol.
//!
//! Pure byte-layout encode/decode. No I/O, no transport, no allocation on
//! decode. Localhost-only IPC -- all multi-byte fields use host byte order.
//!
//! Decoded `View` types borrow the underlying buffer and are valid only while
//! that buffer lives. Copy immediately if the data is needed later.

mod cgroups_snapshot;
mod increment;
mod lookup;
mod string_reverse;

// Re-export all public symbols from submodules.
pub use cgroups_snapshot::*;
pub use increment::*;
pub use lookup::*;
pub use string_reverse::*;

// ---------------------------------------------------------------------------
//  Constants
// ---------------------------------------------------------------------------

pub const MAGIC_MSG: u32 = 0x4e495043; // "NIPC"
pub const MAGIC_CHUNK: u32 = 0x4e43484b; // "NCHK"
pub const VERSION: u16 = 1;
pub const HEADER_LEN: u16 = 32;
pub const HEADER_SIZE: usize = 32;

// Message kinds
pub const KIND_REQUEST: u16 = 1;
pub const KIND_RESPONSE: u16 = 2;
pub const KIND_CONTROL: u16 = 3;

// Flags
pub const FLAG_BATCH: u16 = 0x0001;

// Transport status
pub const STATUS_OK: u16 = 0;
pub const STATUS_BAD_ENVELOPE: u16 = 1;
pub const STATUS_AUTH_FAILED: u16 = 2;
pub const STATUS_INCOMPATIBLE: u16 = 3;
pub const STATUS_UNSUPPORTED: u16 = 4;
pub const STATUS_LIMIT_EXCEEDED: u16 = 5;
pub const STATUS_INTERNAL_ERROR: u16 = 6;

// Control opcodes
pub const CODE_HELLO: u16 = 1;
pub const CODE_HELLO_ACK: u16 = 2;

// Method codes
pub const METHOD_INCREMENT: u16 = 1;
pub const METHOD_CGROUPS_SNAPSHOT: u16 = 2;
pub const METHOD_STRING_REVERSE: u16 = 3;
pub const METHOD_CGROUPS_LOOKUP: u16 = 4;
pub const METHOD_APPS_LOOKUP: u16 = 5;

// Profile bits
pub const PROFILE_BASELINE: u32 = 0x01;
pub const PROFILE_SHM_HYBRID: u32 = 0x02;
pub const PROFILE_SHM_FUTEX: u32 = 0x04;
pub const PROFILE_SHM_WAITADDR: u32 = 0x08;

// Defaults
pub const MAX_PAYLOAD_DEFAULT: u32 = 1024;

/// Hard cap on negotiated request payload sizes (1 MiB) — prevents a
/// compromised peer from forcing excessive memory allocation.
pub const MAX_PAYLOAD_CAP: u32 = 1024 * 1024;

// Alignment
pub const ALIGNMENT: usize = 8;

// Payload sizes
const HELLO_SIZE: usize = 44;
const HELLO_ACK_SIZE: usize = 48;

// ---------------------------------------------------------------------------
//  Errors
// ---------------------------------------------------------------------------

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum NipcError {
    /// Buffer too short for the expected structure.
    Truncated,
    /// Magic value mismatch.
    BadMagic,
    /// Unsupported version.
    BadVersion,
    /// header_len != 32.
    BadHeaderLen,
    /// Unknown message kind.
    BadKind,
    /// Unknown layout_version in a payload.
    BadLayout,
    /// Offset+length exceeds available data.
    OutOfBounds,
    /// String not NUL-terminated.
    MissingNul,
    /// Item not 8-byte aligned.
    BadAlignment,
    /// Directory inconsistent with payload size.
    BadItemCount,
    /// Builder ran out of space.
    Overflow,
    /// Synchronous call timed out before a complete response arrived.
    Timeout,
    /// Synchronous call was aborted by the caller.
    Aborted,
}

impl core::fmt::Display for NipcError {
    fn fmt(&self, f: &mut core::fmt::Formatter<'_>) -> core::fmt::Result {
        match self {
            NipcError::Truncated => write!(f, "buffer too short"),
            NipcError::BadMagic => write!(f, "magic value mismatch"),
            NipcError::BadVersion => write!(f, "unsupported version"),
            NipcError::BadHeaderLen => write!(f, "header_len != 32"),
            NipcError::BadKind => write!(f, "unknown message kind"),
            NipcError::BadLayout => write!(f, "unknown layout_version"),
            NipcError::OutOfBounds => write!(f, "offset+length exceeds data"),
            NipcError::MissingNul => write!(f, "string not NUL-terminated"),
            NipcError::BadAlignment => write!(f, "item not 8-byte aligned"),
            NipcError::BadItemCount => write!(f, "item count inconsistent"),
            NipcError::Overflow => write!(f, "builder out of space"),
            NipcError::Timeout => write!(f, "synchronous call timed out"),
            NipcError::Aborted => write!(f, "synchronous call aborted"),
        }
    }
}

impl std::error::Error for NipcError {}

// ---------------------------------------------------------------------------
//  Utility
// ---------------------------------------------------------------------------

/// Round `v` up to the next multiple of 8.
#[inline]
pub fn align8(v: usize) -> usize {
    (v + 7) & !7
}

// ---------------------------------------------------------------------------
//  Outer message header (32 bytes)
// ---------------------------------------------------------------------------

#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
pub struct Header {
    pub magic: u32,
    pub version: u16,
    pub header_len: u16,
    pub kind: u16,
    pub flags: u16,
    pub code: u16,
    pub transport_status: u16,
    pub payload_len: u32,
    pub item_count: u32,
    pub message_id: u64,
}

impl Header {
    /// Encode into `buf`. Returns 32 on success, 0 if buf is too small.
    pub fn encode(&self, buf: &mut [u8]) -> usize {
        if buf.len() < HEADER_SIZE {
            return 0;
        }
        buf[0..4].copy_from_slice(&self.magic.to_ne_bytes());
        buf[4..6].copy_from_slice(&self.version.to_ne_bytes());
        buf[6..8].copy_from_slice(&self.header_len.to_ne_bytes());
        buf[8..10].copy_from_slice(&self.kind.to_ne_bytes());
        buf[10..12].copy_from_slice(&self.flags.to_ne_bytes());
        buf[12..14].copy_from_slice(&self.code.to_ne_bytes());
        buf[14..16].copy_from_slice(&self.transport_status.to_ne_bytes());
        buf[16..20].copy_from_slice(&self.payload_len.to_ne_bytes());
        buf[20..24].copy_from_slice(&self.item_count.to_ne_bytes());
        buf[24..32].copy_from_slice(&self.message_id.to_ne_bytes());
        HEADER_SIZE
    }

    /// Decode from `buf`. Validates magic, version, header_len, kind.
    pub fn decode(buf: &[u8]) -> Result<Self, NipcError> {
        if buf.len() < HEADER_SIZE {
            return Err(NipcError::Truncated);
        }
        let hdr = Header {
            magic: u32::from_ne_bytes(buf[0..4].try_into().unwrap()),
            version: u16::from_ne_bytes(buf[4..6].try_into().unwrap()),
            header_len: u16::from_ne_bytes(buf[6..8].try_into().unwrap()),
            kind: u16::from_ne_bytes(buf[8..10].try_into().unwrap()),
            flags: u16::from_ne_bytes(buf[10..12].try_into().unwrap()),
            code: u16::from_ne_bytes(buf[12..14].try_into().unwrap()),
            transport_status: u16::from_ne_bytes(buf[14..16].try_into().unwrap()),
            payload_len: u32::from_ne_bytes(buf[16..20].try_into().unwrap()),
            item_count: u32::from_ne_bytes(buf[20..24].try_into().unwrap()),
            message_id: u64::from_ne_bytes(buf[24..32].try_into().unwrap()),
        };

        if hdr.magic != MAGIC_MSG {
            return Err(NipcError::BadMagic);
        }
        if hdr.version != VERSION {
            return Err(NipcError::BadVersion);
        }
        if hdr.header_len != HEADER_LEN {
            return Err(NipcError::BadHeaderLen);
        }
        if hdr.kind < KIND_REQUEST || hdr.kind > KIND_CONTROL {
            return Err(NipcError::BadKind);
        }
        Ok(hdr)
    }
}

// ---------------------------------------------------------------------------
//  Chunk continuation header (32 bytes)
// ---------------------------------------------------------------------------

#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
pub struct ChunkHeader {
    pub magic: u32,
    pub version: u16,
    pub flags: u16,
    pub message_id: u64,
    pub total_message_len: u32,
    pub chunk_index: u32,
    pub chunk_count: u32,
    pub chunk_payload_len: u32,
}

impl ChunkHeader {
    /// Encode into `buf`. Returns 32 on success, 0 if buf is too small.
    pub fn encode(&self, buf: &mut [u8]) -> usize {
        if buf.len() < HEADER_SIZE {
            return 0;
        }
        buf[0..4].copy_from_slice(&self.magic.to_ne_bytes());
        buf[4..6].copy_from_slice(&self.version.to_ne_bytes());
        buf[6..8].copy_from_slice(&self.flags.to_ne_bytes());
        buf[8..16].copy_from_slice(&self.message_id.to_ne_bytes());
        buf[16..20].copy_from_slice(&self.total_message_len.to_ne_bytes());
        buf[20..24].copy_from_slice(&self.chunk_index.to_ne_bytes());
        buf[24..28].copy_from_slice(&self.chunk_count.to_ne_bytes());
        buf[28..32].copy_from_slice(&self.chunk_payload_len.to_ne_bytes());
        HEADER_SIZE
    }

    /// Decode from `buf`. Validates magic and version.
    pub fn decode(buf: &[u8]) -> Result<Self, NipcError> {
        if buf.len() < HEADER_SIZE {
            return Err(NipcError::Truncated);
        }
        let chk = ChunkHeader {
            magic: u32::from_ne_bytes(buf[0..4].try_into().unwrap()),
            version: u16::from_ne_bytes(buf[4..6].try_into().unwrap()),
            flags: u16::from_ne_bytes(buf[6..8].try_into().unwrap()),
            message_id: u64::from_ne_bytes(buf[8..16].try_into().unwrap()),
            total_message_len: u32::from_ne_bytes(buf[16..20].try_into().unwrap()),
            chunk_index: u32::from_ne_bytes(buf[20..24].try_into().unwrap()),
            chunk_count: u32::from_ne_bytes(buf[24..28].try_into().unwrap()),
            chunk_payload_len: u32::from_ne_bytes(buf[28..32].try_into().unwrap()),
        };

        if chk.magic != MAGIC_CHUNK {
            return Err(NipcError::BadMagic);
        }
        if chk.version != VERSION {
            return Err(NipcError::BadVersion);
        }
        if chk.flags != 0 {
            return Err(NipcError::BadLayout);
        }
        if chk.chunk_payload_len == 0 {
            return Err(NipcError::BadLayout);
        }
        Ok(chk)
    }
}

// ---------------------------------------------------------------------------
//  Batch item directory
// ---------------------------------------------------------------------------

/// One entry in a batch item directory (8 bytes on wire).
#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
pub struct BatchEntry {
    pub offset: u32,
    pub length: u32,
}

/// Encode `entries` into `buf`. Returns total bytes written (entries.len() * 8),
/// or 0 if buf is too small.
pub fn batch_dir_encode(entries: &[BatchEntry], buf: &mut [u8]) -> usize {
    let need = entries.len() * 8;
    if buf.len() < need {
        return 0;
    }
    for (i, e) in entries.iter().enumerate() {
        let base = i * 8;
        buf[base..base + 4].copy_from_slice(&e.offset.to_ne_bytes());
        buf[base + 4..base + 8].copy_from_slice(&e.length.to_ne_bytes());
    }
    need
}

/// Decode `item_count` directory entries from `buf`. Validates alignment and
/// that each entry falls within `packed_area_len`.
pub fn batch_dir_decode(
    buf: &[u8],
    item_count: u32,
    packed_area_len: u32,
) -> Result<Vec<BatchEntry>, NipcError> {
    let count = item_count as usize;
    let dir_size = count.checked_mul(8).ok_or(NipcError::BadLayout)?;
    if buf.len() < dir_size {
        return Err(NipcError::Truncated);
    }

    let mut out = Vec::with_capacity(count);
    for i in 0..count {
        let base = i * 8;
        let offset = u32::from_ne_bytes(buf[base..base + 4].try_into().unwrap());
        let length = u32::from_ne_bytes(buf[base + 4..base + 8].try_into().unwrap());

        if (offset as usize) % ALIGNMENT != 0 {
            return Err(NipcError::BadAlignment);
        }
        if (offset as u64) + (length as u64) > packed_area_len as u64 {
            return Err(NipcError::OutOfBounds);
        }
        out.push(BatchEntry { offset, length });
    }
    Ok(out)
}

/// Validate a batch directory without allocating. Checks alignment and
/// that each entry falls within `packed_area_len`. Mirrors C's
/// `nipc_batch_dir_validate`.
pub fn batch_dir_validate(
    buf: &[u8],
    item_count: u32,
    packed_area_len: u32,
) -> Result<(), NipcError> {
    let count = item_count as usize;
    let dir_size = count.checked_mul(8).ok_or(NipcError::BadLayout)?;
    if buf.len() < dir_size {
        return Err(NipcError::Truncated);
    }
    for i in 0..count {
        let base = i * 8;
        let offset = u32::from_ne_bytes(buf[base..base + 4].try_into().unwrap());
        let length = u32::from_ne_bytes(buf[base + 4..base + 8].try_into().unwrap());
        if (offset as usize) % ALIGNMENT != 0 {
            return Err(NipcError::BadAlignment);
        }
        if (offset as u64) + (length as u64) > packed_area_len as u64 {
            return Err(NipcError::OutOfBounds);
        }
    }
    Ok(())
}

/// Extract a single batch item by index from a complete batch payload.
/// Returns (item_slice, item_len) on success.
pub fn batch_item_get(
    payload: &[u8],
    item_count: u32,
    index: u32,
) -> Result<(&[u8], u32), NipcError> {
    if index >= item_count {
        return Err(NipcError::OutOfBounds);
    }

    let dir_size = (item_count as usize)
        .checked_mul(8)
        .ok_or(NipcError::BadLayout)?;
    let dir_aligned = align8(dir_size);

    if payload.len() < dir_aligned {
        return Err(NipcError::Truncated);
    }

    let idx = index as usize;
    let base = idx * 8;
    let off = u32::from_ne_bytes(payload[base..base + 4].try_into().unwrap());
    let len = u32::from_ne_bytes(payload[base + 4..base + 8].try_into().unwrap());

    let packed_area_start = dir_aligned;
    let packed_area_len = payload.len() - packed_area_start;

    if (off as usize) % ALIGNMENT != 0 {
        return Err(NipcError::BadAlignment);
    }
    if (off as u64) + (len as u64) > packed_area_len as u64 {
        return Err(NipcError::OutOfBounds);
    }

    let start = packed_area_start + off as usize;
    let end = start + len as usize;
    Ok((&payload[start..end], len))
}

// ---------------------------------------------------------------------------
//  Batch builder
// ---------------------------------------------------------------------------

/// Builds a batch payload: [directory] [align-pad] [packed items].
pub struct BatchBuilder<'a> {
    buf: &'a mut [u8],
    item_count: u32,
    max_items: u32,
    dir_end: usize,     // byte offset where directory reservation ends
    data_offset: usize, // current offset within the packed data area (relative)
}

impl<'a> BatchBuilder<'a> {
    /// Create a new batch builder. `buf` must be large enough for
    /// `max_items * 8` (directory) + packed data.
    pub fn new(buf: &'a mut [u8], max_items: u32) -> Self {
        let dir_end = align8(max_items as usize * 8);
        BatchBuilder {
            buf,
            item_count: 0,
            max_items,
            dir_end,
            data_offset: 0,
        }
    }

    /// Add an item payload. Handles alignment padding.
    pub fn add(&mut self, item: &[u8]) -> Result<(), NipcError> {
        if self.item_count >= self.max_items {
            return Err(NipcError::Overflow);
        }

        let aligned_off = align8(self.data_offset);
        let abs_pos = self.dir_end + aligned_off;

        if abs_pos + item.len() > self.buf.len() {
            return Err(NipcError::Overflow);
        }

        // Zero alignment padding
        if aligned_off > self.data_offset {
            let pad_start = self.dir_end + self.data_offset;
            let pad_end = self.dir_end + aligned_off;
            self.buf[pad_start..pad_end].fill(0);
        }

        self.buf[abs_pos..abs_pos + item.len()].copy_from_slice(item);

        // Write directory entry
        let idx = self.item_count as usize;
        let dir_base = idx * 8;
        self.buf[dir_base..dir_base + 4].copy_from_slice(&(aligned_off as u32).to_ne_bytes());
        self.buf[dir_base + 4..dir_base + 8].copy_from_slice(&(item.len() as u32).to_ne_bytes());

        self.data_offset = aligned_off + item.len();
        self.item_count += 1;
        Ok(())
    }

    /// Finalize the batch. Returns (total_payload_size, item_count).
    /// Compacts if fewer items were added than max_items.
    pub fn finish(self) -> (usize, u32) {
        let count = self.item_count;
        let final_dir_aligned = align8(count as usize * 8);

        if final_dir_aligned < self.dir_end && self.data_offset > 0 {
            // Shift packed data left
            self.buf.copy_within(
                self.dir_end..self.dir_end + self.data_offset,
                final_dir_aligned,
            );
        }

        let total = final_dir_aligned + align8(self.data_offset);
        // Zero trailing alignment padding to avoid leaking stale buffer data
        if total < self.buf.len() {
            let pad_start = final_dir_aligned + self.data_offset;
            if pad_start < total {
                self.buf[pad_start..total].fill(0);
            }
        }
        (total, count)
    }
}

// ---------------------------------------------------------------------------
//  Hello payload (44 bytes)
// ---------------------------------------------------------------------------

#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
pub struct Hello {
    pub layout_version: u16,
    pub flags: u16,
    pub supported_profiles: u32,
    pub preferred_profiles: u32,
    pub max_request_payload_bytes: u32,
    pub max_request_batch_items: u32,
    pub max_response_payload_bytes: u32,
    pub max_response_batch_items: u32,
    pub auth_token: u64,
    pub packet_size: u32,
}

impl Hello {
    /// Encode into `buf`. Returns 44 on success, 0 if buf is too small.
    pub fn encode(&self, buf: &mut [u8]) -> usize {
        if buf.len() < HELLO_SIZE {
            return 0;
        }
        buf[0..2].copy_from_slice(&self.layout_version.to_ne_bytes());
        buf[2..4].copy_from_slice(&self.flags.to_ne_bytes());
        buf[4..8].copy_from_slice(&self.supported_profiles.to_ne_bytes());
        buf[8..12].copy_from_slice(&self.preferred_profiles.to_ne_bytes());
        buf[12..16].copy_from_slice(&self.max_request_payload_bytes.to_ne_bytes());
        buf[16..20].copy_from_slice(&self.max_request_batch_items.to_ne_bytes());
        buf[20..24].copy_from_slice(&self.max_response_payload_bytes.to_ne_bytes());
        buf[24..28].copy_from_slice(&self.max_response_batch_items.to_ne_bytes());
        buf[28..32].copy_from_slice(&0u32.to_ne_bytes()); // padding
        buf[32..40].copy_from_slice(&self.auth_token.to_ne_bytes());
        buf[40..44].copy_from_slice(&self.packet_size.to_ne_bytes());
        HELLO_SIZE
    }

    /// Decode from `buf`. Validates layout_version.
    pub fn decode(buf: &[u8]) -> Result<Self, NipcError> {
        if buf.len() < HELLO_SIZE {
            return Err(NipcError::Truncated);
        }
        let h = Hello {
            layout_version: u16::from_ne_bytes(buf[0..2].try_into().unwrap()),
            flags: u16::from_ne_bytes(buf[2..4].try_into().unwrap()),
            supported_profiles: u32::from_ne_bytes(buf[4..8].try_into().unwrap()),
            preferred_profiles: u32::from_ne_bytes(buf[8..12].try_into().unwrap()),
            max_request_payload_bytes: u32::from_ne_bytes(buf[12..16].try_into().unwrap()),
            max_request_batch_items: u32::from_ne_bytes(buf[16..20].try_into().unwrap()),
            max_response_payload_bytes: u32::from_ne_bytes(buf[20..24].try_into().unwrap()),
            max_response_batch_items: u32::from_ne_bytes(buf[24..28].try_into().unwrap()),
            // buf[28..32] is reserved padding, must be zero
            auth_token: u64::from_ne_bytes(buf[32..40].try_into().unwrap()),
            packet_size: u32::from_ne_bytes(buf[40..44].try_into().unwrap()),
        };

        if h.layout_version != 1 {
            return Err(NipcError::BadLayout);
        }

        // Validate padding bytes 28..32 are zero
        if u32::from_ne_bytes(buf[28..32].try_into().unwrap()) != 0 {
            return Err(NipcError::BadLayout);
        }

        Ok(h)
    }
}

// ---------------------------------------------------------------------------
//  Hello-ack payload (48 bytes)
// ---------------------------------------------------------------------------

#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
pub struct HelloAck {
    pub layout_version: u16,
    pub flags: u16,
    pub server_supported_profiles: u32,
    pub intersection_profiles: u32,
    pub selected_profile: u32,
    pub agreed_max_request_payload_bytes: u32,
    pub agreed_max_request_batch_items: u32,
    pub agreed_max_response_payload_bytes: u32,
    pub agreed_max_response_batch_items: u32,
    pub agreed_packet_size: u32,
    pub session_id: u64,
}

impl HelloAck {
    /// Encode into `buf`. Returns 48 on success, 0 if buf is too small.
    pub fn encode(&self, buf: &mut [u8]) -> usize {
        if buf.len() < HELLO_ACK_SIZE {
            return 0;
        }
        buf[0..2].copy_from_slice(&self.layout_version.to_ne_bytes());
        buf[2..4].copy_from_slice(&self.flags.to_ne_bytes());
        buf[4..8].copy_from_slice(&self.server_supported_profiles.to_ne_bytes());
        buf[8..12].copy_from_slice(&self.intersection_profiles.to_ne_bytes());
        buf[12..16].copy_from_slice(&self.selected_profile.to_ne_bytes());
        buf[16..20].copy_from_slice(&self.agreed_max_request_payload_bytes.to_ne_bytes());
        buf[20..24].copy_from_slice(&self.agreed_max_request_batch_items.to_ne_bytes());
        buf[24..28].copy_from_slice(&self.agreed_max_response_payload_bytes.to_ne_bytes());
        buf[28..32].copy_from_slice(&self.agreed_max_response_batch_items.to_ne_bytes());
        buf[32..36].copy_from_slice(&self.agreed_packet_size.to_ne_bytes());
        buf[36..40].copy_from_slice(&0u32.to_ne_bytes()); // padding
        buf[40..48].copy_from_slice(&self.session_id.to_ne_bytes());
        HELLO_ACK_SIZE
    }

    /// Decode from `buf`. Validates layout_version.
    pub fn decode(buf: &[u8]) -> Result<Self, NipcError> {
        if buf.len() < HELLO_ACK_SIZE {
            return Err(NipcError::Truncated);
        }
        let h = HelloAck {
            layout_version: u16::from_ne_bytes(buf[0..2].try_into().unwrap()),
            flags: u16::from_ne_bytes(buf[2..4].try_into().unwrap()),
            server_supported_profiles: u32::from_ne_bytes(buf[4..8].try_into().unwrap()),
            intersection_profiles: u32::from_ne_bytes(buf[8..12].try_into().unwrap()),
            selected_profile: u32::from_ne_bytes(buf[12..16].try_into().unwrap()),
            agreed_max_request_payload_bytes: u32::from_ne_bytes(buf[16..20].try_into().unwrap()),
            agreed_max_request_batch_items: u32::from_ne_bytes(buf[20..24].try_into().unwrap()),
            agreed_max_response_payload_bytes: u32::from_ne_bytes(buf[24..28].try_into().unwrap()),
            agreed_max_response_batch_items: u32::from_ne_bytes(buf[28..32].try_into().unwrap()),
            agreed_packet_size: u32::from_ne_bytes(buf[32..36].try_into().unwrap()),
            // skip padding at 36..40
            session_id: u64::from_ne_bytes(buf[40..48].try_into().unwrap()),
        };

        if h.layout_version != 1 {
            return Err(NipcError::BadLayout);
        }
        if h.flags != 0 {
            return Err(NipcError::BadLayout);
        }
        Ok(h)
    }
}

#[cfg(test)]
mod tests;
