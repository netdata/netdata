//! The WAL disk/wire format: the 4 KiB file header, the frame layout of the
//! file body, and the serde lifecycle-event envelope (`FileEvent` / `Message`)
//! that carries writer events from the ingestor to the ledger.
//!
//! All multi-byte header and frame fields are little-endian; checksums are
//! CRC32 (`crc32fast`). The header CRC covers its whole page prefix up to the
//! CRC field. A frame's CRC covers its 20 header bytes plus the compressed
//! payload, and is stored/verified only when the header sets
//! [`FLAG_CRC_ENABLED`] (otherwise the field is `0` and unverified).
//!
//! Compat policy: [`FileHeader::from_bytes`] accepts exactly [`FORMAT_VERSION`]
//! and rejects in a fixed order — magic, version, header CRC, unknown flag
//! bits, compression id, `content_meta` cap — so a corrupt or foreign file
//! fails before any field is interpreted. The header carries no partition key
//! and no footer: `part_key` lives only in the filename `FileId` stem (the
//! `file-registry` stem contract), and a file recovered from disk has no
//! durable prefix or log-time range (`wal::registry` re-reads the header for
//! identity only).
//!
//! Consumers: `FileHeader` and the frame constants are used only inside this
//! crate (writer builds, reader and registry parse). `HEADER_SIZE` is
//! re-exported as the first-frame offset shared with the query/index side
//! (sfsq, sfsq-cli, otel-ledger, ng-index); `MAX_CONTENT_META_BYTES` is
//! pre-write-enforced by the ingestor's log service. `FileEvent` is produced
//! by `wal::Writer` in the ingestor and, wrapped in [`Message`], applied by
//! the ledger's per-tenant registries (`file-lifecycle` forwards to
//! `wal::Registry::apply_event`); `Message` is that link's envelope,
//! produced by the ingestor's `LedgerSender` and gap-checked + routed in
//! `otel-ledger`'s ingestor.
//! `Message` rides the ingestor→ledger ferryboat IPC link (length-delimited
//! bincode, 8 MiB default) — that framing is the transport's, independent of
//! the WAL frame layout below.
use serde::{Deserialize, Serialize};

use file_registry::{ByteSize, FileId, TenantId, TimestampNs};

// -- Constants ----------------------------------------------------------

/// Magic bytes at the start of every WAL file.
pub const MAGIC: [u8; 4] = *b"NWAL";

/// Current format version. v1 carries an opaque `payload_format` tag,
/// strict flag bits (unknown bits reject), and a CRC32-protected header
/// page. Readers accept exactly this version: any other value is rejected
/// and the file is skipped as an orphan at recovery.
pub const FORMAT_VERSION: u16 = 1;

/// Total size of the file header in bytes (one 4 KiB page).
pub const HEADER_SIZE: usize = 4096;

/// Max stored length (bytes) of the header's `content_meta` blob — the content
/// plane's opaque per-file identity. It fits easily in the 4 KiB header; the
/// writer rejects a larger blob rather than truncate it (truncation would
/// corrupt the identity).
pub const MAX_CONTENT_META_BYTES: usize = 1024;

/// Byte offset where the header's `content_meta` blob begins:
/// `MAGIC(4) + version(2) + flags(2) + created_at(8) + payload_format(2) +
/// content_meta_len(2)`.
pub const CONTENT_META_OFFSET: usize = 20;

/// Byte offset of the header CRC32: the last 4 bytes of the page. Covers
/// everything before it (`[0, HEADER_CRC_OFFSET)`), reserved zeros included.
pub const HEADER_CRC_OFFSET: usize = HEADER_SIZE - 4;

/// The `content_meta` blob plus its fixed prefix must fit below the header
/// CRC. A compile error here means `MAX_CONTENT_META_BYTES` was raised past
/// the header budget without growing `HEADER_SIZE` — the slice writes/reads in
/// `to_bytes`/`from_bytes` would otherwise overflow into the CRC field. Fix
/// one of the constants.
const _: () = assert!(CONTENT_META_OFFSET + MAX_CONTENT_META_BYTES <= HEADER_CRC_OFFSET);

/// Bit 0: frames carry a CRC32 over their header + compressed payload, and the
/// reader verifies it. When clear, the writer stores `0` in the frame's CRC
/// field and the reader skips verification.
pub const FLAG_CRC_ENABLED: u16 = 1 << 0;

/// Bits 1-2 of the header flags: the compression algorithm applied to each
/// frame's payload. Only the two values below are defined; `from_bytes`
/// rejects any other pattern as `Error::UnsupportedCompression`.
pub const COMPRESSION_MASK: u16 = 0b110;
/// LZ4 block compression — the writer's default, so the bits are usually clear.
pub const COMPRESSION_LZ4: u16 = 0b000;
/// Payloads stored as written, uncompressed.
pub const COMPRESSION_NONE: u16 = 0b010;

/// Every flag bit this version defines. `from_bytes` rejects any other bit:
/// an unknown bit means a newer writer gated a feature this reader does not
/// understand, and proceeding would misread the file.
pub const KNOWN_FLAGS_MASK: u16 = FLAG_CRC_ENABLED | COMPRESSION_MASK;

/// Frame header size: `[u32 payload_len] [u32 uncompressed_len] [u32 entry_count] [u64 timestamp_ns] [u32 crc32]`,
/// all little-endian. `timestamp_ns` is the caller's ingestion timestamp
/// ([`FrameMeta::ingestion_ns`](crate::FrameMeta)); `crc32` is `0` unless the
/// header sets [`FLAG_CRC_ENABLED`], in which case it covers these 20 bytes
/// followed by the compressed payload.
pub const FRAME_HEADER_SIZE: usize = 24;

/// Every frame is zero-padded so its total size (header + payload + padding)
/// is a multiple of this. The header page is a multiple of it too, so every
/// frame starts and ends on this boundary — the property that makes a frame
/// end offset a valid `Reader::open_range` start.
pub const FRAME_ALIGNMENT: usize = 8;

// -- File header --------------------------------------------------------

/// Fixed-size file header written once when a WAL file is created.
///
/// Layout: `MAGIC(4)`, `version(2)`, `flags(2)`, `created_at(8)`,
/// `payload_format(2)`, then `content_meta` length-prefixed —
/// `content_meta_len(2) content_meta` — capped at [`MAX_CONTENT_META_BYTES`].
/// The rest of the page is zero except the last 4 bytes, a CRC32 over
/// everything before it. The header stores no partition key: the `part_key`
/// is the single source of truth in the filename (`FileId`), and
/// `content_meta` is an opaque content-plane blob (for OTel logs, the encoded
/// service identity) the WAL never interprets.
#[derive(Debug, Clone)]
pub struct FileHeader {
    pub version: u16,
    pub flags: u16,
    /// Wall-clock creation time (nanoseconds since the Unix epoch), stamped
    /// once at file creation. A diagnostic: ordering uses the frames'
    /// ingestion timestamps, not this.
    pub created_at: u64,
    /// Opaque per-file frame-codec tag. The content plane assigns the ids and
    /// checks them before decoding frames; the WAL stamps and exposes the
    /// value, never interprets it. `0` is reserved ("unspecified") and refused
    /// at `Writer::new`; the reader does not re-check it.
    pub payload_format: u16,
    /// Opaque content-plane metadata, recorded so the file's identity is
    /// available cheaply (recovery, the stream selector) without decoding any
    /// frame. The WAL never interprets it.
    pub content_meta: Vec<u8>,
}

impl FileHeader {
    pub fn crc_enabled(&self) -> bool {
        self.flags & FLAG_CRC_ENABLED != 0
    }

    pub fn compression(&self) -> u16 {
        self.flags & COMPRESSION_MASK
    }

    pub fn to_bytes(&self) -> [u8; HEADER_SIZE] {
        // The writer bounds `content_meta` to `MAX_CONTENT_META_BYTES` before
        // constructing the header (it rejects oversize rather than truncate), so
        // this is always within budget. Enforced in all builds (not just
        // `debug_assert`): a direct constructor passing an oversized blob would
        // otherwise truncate the `u16` length and write past the budget — fail
        // loudly instead.
        assert!(
            self.content_meta.len() <= MAX_CONTENT_META_BYTES,
            "WAL content_meta {} exceeds {MAX_CONTENT_META_BYTES}",
            self.content_meta.len()
        );
        let mut buf = [0u8; HEADER_SIZE];
        buf[0..4].copy_from_slice(&MAGIC);
        buf[4..6].copy_from_slice(&self.version.to_le_bytes());
        buf[6..8].copy_from_slice(&self.flags.to_le_bytes());
        buf[8..16].copy_from_slice(&self.created_at.to_le_bytes());
        buf[16..18].copy_from_slice(&self.payload_format.to_le_bytes());

        let len = self.content_meta.len();
        buf[CONTENT_META_OFFSET - 2..CONTENT_META_OFFSET]
            .copy_from_slice(&(len as u16).to_le_bytes());
        buf[CONTENT_META_OFFSET..CONTENT_META_OFFSET + len].copy_from_slice(&self.content_meta);

        let crc = crc32fast::hash(&buf[..HEADER_CRC_OFFSET]);
        buf[HEADER_CRC_OFFSET..].copy_from_slice(&crc.to_le_bytes());
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
        // Version precedes the CRC and every layout-dependent read: a
        // different version may place other fields where this layout reads
        // them, so reject on the version alone.
        if version != FORMAT_VERSION {
            return Err(crate::Error::UnsupportedVersion(version));
        }

        // CRC before the remaining fields, so a corrupt-but-parseable header
        // (flipped flag bit, altered identity blob) rejects instead of misreads.
        let stored_crc = u32::from_le_bytes(buf[HEADER_CRC_OFFSET..].try_into().unwrap());
        let actual_crc = crc32fast::hash(&buf[..HEADER_CRC_OFFSET]);
        if stored_crc != actual_crc {
            return Err(crate::Error::CrcMismatch {
                expected: stored_crc,
                actual: actual_crc,
            });
        }

        let flags = u16::from_le_bytes([buf[6], buf[7]]);
        if flags & !KNOWN_FLAGS_MASK != 0 {
            return Err(crate::Error::InvalidHeader(format!(
                "unknown flag bits {:#06x} (known mask {KNOWN_FLAGS_MASK:#06x}) — \
                 written by a newer version?",
                flags & !KNOWN_FLAGS_MASK
            )));
        }
        let compression = flags & COMPRESSION_MASK;
        if compression != COMPRESSION_LZ4 && compression != COMPRESSION_NONE {
            return Err(crate::Error::UnsupportedCompression(
                (compression >> 1) as u8,
            ));
        }
        let created_at = u64::from_le_bytes(buf[8..16].try_into().unwrap());
        let payload_format = u16::from_le_bytes([buf[16], buf[17]]);

        let content_meta_len =
            u16::from_le_bytes([buf[CONTENT_META_OFFSET - 2], buf[CONTENT_META_OFFSET - 1]])
                as usize;
        if content_meta_len > MAX_CONTENT_META_BYTES {
            return Err(crate::Error::InvalidHeader(format!(
                "content_meta length {content_meta_len} exceeds {MAX_CONTENT_META_BYTES}"
            )));
        }
        let content_meta =
            buf[CONTENT_META_OFFSET..CONTENT_META_OFFSET + content_meta_len].to_vec();

        Ok(Self {
            version,
            flags,
            created_at,
            payload_format,
            content_meta,
        })
    }
}

// -- Events -------------------------------------------------------------

/// Events produced by the WAL writer during file lifecycle operations.
///
/// Applied by the ledger's per-tenant registries — `file-lifecycle`'s
/// `apply_wal_event` forwards each event to
/// [`Registry::apply_event`](crate::Registry); a `Closed` event schedules the
/// file's SFST indexing job in `otel-ledger`'s ingestor.
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
        /// Opaque content-plane metadata for the file (the content plane
        /// interprets it; the registry stores it for cheap identity/display).
        /// The partition key is carried by `file_id`, not duplicated here.
        content_meta: Vec<u8>,
    },
    Synced {
        file_id: FileId,
        /// Durable byte prefix at sync time: the end of the last frame fsynced
        /// to disk (frame-aligned). A concurrent reader may read up to here.
        valid_up_to: ByteSize,
        /// Frames written to the file so far. Informational: the registry
        /// ignores it.
        frame_count: u64,
        /// Log records in that durable prefix.
        entry_count: u64,
        /// Earliest log-data timestamp accumulated for this file so far.
        min_timestamp_ns: TimestampNs,
        /// Latest log-data timestamp accumulated for this file so far.
        max_timestamp_ns: TimestampNs,
    },
    Closed {
        file_id: FileId,
        /// Frames written to the file (final). Informational: the registry
        /// ignores it.
        frame_count: u64,
        /// Earliest log-data timestamp in this file (final value).
        min_timestamp_ns: TimestampNs,
        /// Latest log-data timestamp in this file (final value).
        max_timestamp_ns: TimestampNs,
        size: ByteSize,
        /// Durable prefix at seal time (== the last `Synced`'s `valid_up_to`).
        /// Carried so a sealed file's prefix is authoritative even if the final
        /// `Synced` was reordered or lost in flight to the ledger.
        valid_up_to: ByteSize,
        /// Total records at seal time (== the last `Synced`'s `entry_count`).
        entry_count: u64,
    },
}

impl FileEvent {
    /// The signal axis (`pipeline_id`) of the file this event concerns. Every
    /// variant carries a [`FileId`], which carries the pipeline. The ingestor
    /// keys its per-signal `Message::frame_seq` stream by this value, and the
    /// ledger routes the event to the owning pipeline by it (decoding the raw
    /// id via `bridge::signals::Signal::try_from`).
    pub fn pipeline_id(&self) -> u16 {
        match self {
            FileEvent::Created { file_id, .. }
            | FileEvent::Synced { file_id, .. }
            | FileEvent::Closed { file_id, .. } => file_id.pipeline_id,
        }
    }
}

/// One lifecycle event sent from the ingestor to the ledger over the
/// WAL-event IPC link: every [`FileEvent`] the ingestor drains from its
/// writers is wrapped in one of these (the ledger is the only consumer).
/// Cross-process wire contract: the two ends are separate processes
/// exchanging bincode over the ferryboat link (8 MiB default), so enum
/// variant order and field shapes must not change silently.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Message {
    /// The link's per-signal message sequence, not a WAL frame index: the
    /// ingestor numbers each signal's messages from 1 (keyed by
    /// [`FileEvent::pipeline_id`]) and the ledger gap-checks per signal — a
    /// gap is a genuinely lost event for that signal.
    pub frame_seq: u64,
    /// Tenant the event's file belongs to; routes the event to that tenant's
    /// registry (the event itself carries no tenant).
    pub tenant_id: TenantId,
    pub event: FileEvent,
}

#[cfg(test)]
mod tests {
    use super::*;

    fn header(content_meta: Vec<u8>) -> FileHeader {
        FileHeader {
            version: FORMAT_VERSION,
            flags: 0,
            created_at: 12345,
            payload_format: 7,
            content_meta,
        }
    }

    /// Re-seal a forged header so it reaches the check under test instead of
    /// failing the CRC gate first.
    fn reseal(buf: &mut [u8; HEADER_SIZE]) {
        let crc = crc32fast::hash(&buf[..HEADER_CRC_OFFSET]);
        buf[HEADER_CRC_OFFSET..].copy_from_slice(&crc.to_le_bytes());
    }

    #[test]
    fn header_roundtrips_content_meta() {
        for cm in [
            Vec::new(),
            vec![1, 2, 3, 4, 5],
            // A blob the size of an encoded (namespace, name) identity.
            b"\x01\x04\x00prod\x03\x00api".to_vec(),
        ] {
            let h = header(cm.clone());
            let parsed = FileHeader::from_bytes(&h.to_bytes()).unwrap();
            assert_eq!(parsed.version, FORMAT_VERSION);
            assert_eq!(parsed.created_at, 12345);
            assert_eq!(parsed.payload_format, 7);
            assert_eq!(parsed.content_meta, cm);
        }
    }

    #[test]
    fn from_bytes_rejects_oversize_content_meta() {
        let mut buf = header(Vec::new()).to_bytes();
        // Forge a content_meta length above the cap (resealed, so the length
        // bound itself rejects, not the CRC gate).
        buf[CONTENT_META_OFFSET - 2..CONTENT_META_OFFSET]
            .copy_from_slice(&((MAX_CONTENT_META_BYTES + 1) as u16).to_le_bytes());
        reseal(&mut buf);
        assert!(matches!(
            FileHeader::from_bytes(&buf).unwrap_err(),
            crate::Error::InvalidHeader(_)
        ));
    }

    #[test]
    fn from_bytes_rejects_unknown_flag_bits() {
        // A future writer claims flag bit 3; this reader must refuse rather
        // than misread the file. Resealed so the flags check itself fires.
        let mut buf = header(Vec::new()).to_bytes();
        let flags = 1u16 << 3;
        buf[6..8].copy_from_slice(&flags.to_le_bytes());
        reseal(&mut buf);
        match FileHeader::from_bytes(&buf).unwrap_err() {
            crate::Error::InvalidHeader(msg) => {
                assert!(msg.contains("unknown flag bits"), "got: {msg}")
            }
            e => panic!("wrong error: {e:?}"),
        }
    }

    #[test]
    fn from_bytes_rejects_corrupt_header() {
        // A bit-flip anywhere in the CRC-covered page — a parseable field
        // (created_at) and the reserved zero region alike — fails the CRC
        // gate before any field is interpreted.
        for flip_at in [9, HEADER_CRC_OFFSET - 1] {
            let mut buf = header(b"id".to_vec()).to_bytes();
            buf[flip_at] ^= 0x01;
            assert!(matches!(
                FileHeader::from_bytes(&buf).unwrap_err(),
                crate::Error::CrcMismatch { .. }
            ));
        }
    }

    #[test]
    fn header_rejects_other_versions() {
        // Exactly one version is accepted; anything else — zero, the next
        // version up, or far future — is hard-rejected, never misread. The
        // version check precedes the CRC gate, so no reseal is needed.
        for v in [0u16, 2, 999] {
            let mut buf = header(Vec::new()).to_bytes();
            buf[4..6].copy_from_slice(&v.to_le_bytes());
            let err = FileHeader::from_bytes(&buf).unwrap_err();
            assert!(matches!(err, crate::Error::UnsupportedVersion(x) if x == v));
        }
    }

    #[test]
    fn header_rejects_unknown_version_before_misreading_offset() {
        // A different format version may place other fields where this
        // layout reads `content_meta_len`. Rejection must happen at the
        // version check *before* any layout-dependent field is read. Forge
        // a version-2 header with a huge value at the current length offset
        // and confirm the failure is the version rejection — not oversize
        // content-meta, and not the (absent) CRC.
        let mut buf = [0u8; HEADER_SIZE];
        buf[0..4].copy_from_slice(&MAGIC);
        buf[4..6].copy_from_slice(&2u16.to_le_bytes());
        buf[CONTENT_META_OFFSET - 2..CONTENT_META_OFFSET].copy_from_slice(&u16::MAX.to_le_bytes());
        let err = FileHeader::from_bytes(&buf).unwrap_err();
        assert!(matches!(err, crate::Error::UnsupportedVersion(2)));
    }
}
