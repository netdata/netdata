use std::fmt;
use std::path::Path;

use serde::{Deserialize, Serialize};
use uuid::Uuid;

// ---------------------------------------------------------------------------
// TimestampNs
// ---------------------------------------------------------------------------

/// Nanoseconds since the Unix epoch.
#[derive(
    Debug, Clone, Copy, PartialEq, Eq, PartialOrd, Ord, Hash, Default, Serialize, Deserialize,
)]
pub struct TimestampNs(pub u64);

impl TimestampNs {
    pub const ZERO: Self = Self(0);

    pub fn as_u64(self) -> u64 {
        self.0
    }

    pub fn saturating_sub(self, rhs: Self) -> u64 {
        self.0.saturating_sub(rhs.0)
    }
}

impl fmt::Display for TimestampNs {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        write!(f, "{}ns", self.0)
    }
}

// ---------------------------------------------------------------------------
// ByteSize
// ---------------------------------------------------------------------------

/// A byte count (file size, offset, etc.).
#[derive(
    Debug, Clone, Copy, PartialEq, Eq, PartialOrd, Ord, Hash, Default, Serialize, Deserialize,
)]
pub struct ByteSize(pub u64);

impl ByteSize {
    pub const ZERO: Self = Self(0);

    pub fn as_u64(self) -> u64 {
        self.0
    }
}

impl fmt::Display for ByteSize {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        write!(f, "{} bytes", self.0)
    }
}

// ---------------------------------------------------------------------------
// FileId
// ---------------------------------------------------------------------------

/// Uniquely identifies a WAL file across machines, boots, and sequences.
///
/// The filename format is: `<machine_id>-<boot_id>-<seq:010>-<ns_hash:016x>.<ext>`
/// where machine_id and boot_id are 32-character lowercase hex (no hyphens),
/// seq is a 10-digit zero-padded decimal, and ns_hash is a 16-character
/// zero-padded hex u64.
///
/// The `ns_hash` is the FNV-1a hash of sorted `service.namespace` and
/// `service.name`. Zero means "no service attribution." If FNV-1a genuinely
/// produces zero, it is remapped to `u64::MAX`.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash, Serialize, Deserialize)]
pub struct FileId {
    pub machine_id: Uuid,
    pub boot_id: Uuid,
    pub seq: u64,
    pub ns_hash: u64,
}

impl FileId {
    pub fn new(machine_id: Uuid, boot_id: Uuid, seq: u64, ns_hash: u64) -> Self {
        Self {
            machine_id,
            boot_id,
            seq,
            ns_hash,
        }
    }

    /// Format the stem portion: `<machine_id>-<boot_id>-<seq:010>-<ns_hash:016x>`
    pub fn to_stem(&self) -> String {
        format!(
            "{}-{}-{:010}-{:016x}",
            self.machine_id.as_simple(),
            self.boot_id.as_simple(),
            self.seq,
            self.ns_hash,
        )
    }

    /// Format a full filename: `<stem>.<ext>`
    pub fn to_filename(&self, ext: &str) -> String {
        format!("{}.{}", self.to_stem(), ext)
    }

    /// Parse a filename (not a full path) into a FileId.
    ///
    /// Expects: `<machine_id>-<boot_id>-<seq>-<ns_hash>.<ext>`
    pub fn parse(path: &Path) -> Option<Self> {
        let name = path.file_stem()?.to_str()?;
        Self::parse_stem(name)
    }

    /// Parse just the stem: `<machine_id>-<boot_id>-<seq>-<ns_hash>`
    pub fn parse_stem(stem: &str) -> Option<Self> {
        // machine_id is 32 hex chars, then '-', boot_id is 32 hex chars, then '-',
        // seq (variable digits), then '-', ns_hash (16 hex chars)
        if stem.len() < 32 + 1 + 32 + 1 + 1 + 1 + 16 {
            return None;
        }

        let machine_str = &stem[..32];
        if stem.as_bytes()[32] != b'-' {
            return None;
        }
        let boot_str = &stem[33..65];
        if stem.as_bytes()[65] != b'-' {
            return None;
        }

        // Find the last '-' to separate seq from ns_hash
        let rest = &stem[66..];
        let last_dash = rest.rfind('-')?;
        let seq_str = &rest[..last_dash];
        let hash_str = &rest[last_dash + 1..];

        let machine_id = Uuid::try_parse(machine_str).ok()?;
        let boot_id = Uuid::try_parse(boot_str).ok()?;
        let seq = seq_str.parse().ok()?;
        let ns_hash = u64::from_str_radix(hash_str, 16).ok()?;

        Some(Self {
            machine_id,
            boot_id,
            seq,
            ns_hash,
        })
    }
}

impl fmt::Display for FileId {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        write!(f, "{}", self.to_stem())
    }
}

impl Ord for FileId {
    fn cmp(&self, other: &Self) -> std::cmp::Ordering {
        self.machine_id
            .as_bytes()
            .cmp(other.machine_id.as_bytes())
            .then_with(|| self.boot_id.as_bytes().cmp(other.boot_id.as_bytes()))
            .then_with(|| self.seq.cmp(&other.seq))
            .then_with(|| self.ns_hash.cmp(&other.ns_hash))
    }
}

impl PartialOrd for FileId {
    fn partial_cmp(&self, other: &Self) -> Option<std::cmp::Ordering> {
        Some(self.cmp(other))
    }
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

#[cfg(test)]
mod tests {
    use super::*;

    fn test_machine_id() -> Uuid {
        Uuid::try_parse("550e8400e29b41d4a716446655440000").unwrap()
    }

    fn test_boot_id() -> Uuid {
        Uuid::try_parse("7f3b2a1e9c4d4f8ab1c2d3e4f5a6b7c8").unwrap()
    }

    #[test]
    fn file_id_stem_roundtrip() {
        let id = FileId::new(test_machine_id(), test_boot_id(), 42, 0xa1b2c3d4e5f60001);
        let stem = id.to_stem();
        assert_eq!(
            stem,
            "550e8400e29b41d4a716446655440000-7f3b2a1e9c4d4f8ab1c2d3e4f5a6b7c8-0000000042-a1b2c3d4e5f60001"
        );
        let parsed = FileId::parse_stem(&stem).unwrap();
        assert_eq!(parsed, id);
    }

    #[test]
    fn file_id_filename_roundtrip() {
        let id = FileId::new(test_machine_id(), test_boot_id(), 1, 0);
        let filename = id.to_filename("wal");
        let path = Path::new(&filename);
        let parsed = FileId::parse(path).unwrap();
        assert_eq!(parsed, id);
    }

    #[test]
    fn file_id_zero_hash() {
        let id = FileId::new(test_machine_id(), test_boot_id(), 1, 0);
        let stem = id.to_stem();
        assert!(stem.ends_with("-0000000000000000"));
        let parsed = FileId::parse_stem(&stem).unwrap();
        assert_eq!(parsed.ns_hash, 0);
    }

    #[test]
    fn file_id_max_hash() {
        let id = FileId::new(test_machine_id(), test_boot_id(), 1, u64::MAX);
        let stem = id.to_stem();
        assert!(stem.ends_with("-ffffffffffffffff"));
        let parsed = FileId::parse_stem(&stem).unwrap();
        assert_eq!(parsed.ns_hash, u64::MAX);
    }

    #[test]
    fn file_id_parse_invalid() {
        assert!(FileId::parse_stem("").is_none());
        assert!(FileId::parse_stem("not-a-valid-id").is_none());
        assert!(FileId::parse_stem("wal-0000000001").is_none());
    }

    #[test]
    fn file_id_ordering() {
        let a = FileId::new(test_machine_id(), test_boot_id(), 1, 0);
        let b = FileId::new(test_machine_id(), test_boot_id(), 2, 0);
        assert!(a < b);

        // Same seq, different ns_hash
        let c = FileId::new(test_machine_id(), test_boot_id(), 1, 1);
        let d = FileId::new(test_machine_id(), test_boot_id(), 1, 2);
        assert!(c < d);
    }
}
