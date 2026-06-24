use file_registry::{ByteSize, FileId, TimestampNs};
use serde::{Deserialize, Serialize};

/// One uploaded SFST file tracked by the catalog.
///
/// Each entry corresponds to exactly one SFST, which holds exactly one
/// partition (`part_key`); the content plane decodes `content_meta` to recover
/// the stream identity for display.
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct CatalogEntry {
    pub id: FileId,
    pub remote_key: String,
    pub min_timestamp_s: u32,
    pub max_timestamp_s: u32,
    pub record_count: u32,
    /// Opaque content-plane identity blob, stored verbatim and never parsed by
    /// the catalog. The partition key is NOT stored here — it lives only in
    /// `id` (the filename `FileId`), the single source of truth.
    pub content_meta: Vec<u8>,
    pub size: ByteSize,
    pub uploaded_at_ns: TimestampNs,
    /// Remote object validator (the S3 ETag, when the backend returns one)
    /// captured at upload time. An opaque token for later integrity/scrub
    /// checks; `None` for entries reconstructed from a remote LIST or read from
    /// pre-existing catalogs. `#[serde(default)]` keeps older catalog files —
    /// written before this field existed — readable.
    #[serde(default)]
    pub remote_etag: Option<String>,
}

/// Deterministic opaque partition key for tests. The catalog treats `part_key`
/// as an opaque `u64` and never decodes it, so tests fabricate distinct keys
/// per logical stream without depending on the content-plane identity codec —
/// same label → same key, different label → (almost surely) different key.
#[cfg(test)]
pub(crate) fn opaque_part_key(namespace: &str, name: &str) -> u64 {
    use std::hash::{Hash, Hasher};
    let mut h = std::collections::hash_map::DefaultHasher::new();
    namespace.hash(&mut h);
    name.hash(&mut h);
    h.finish()
}

#[cfg(test)]
mod tests {
    use super::*;
    use uuid::Uuid;

    #[test]
    fn catalog_entry_roundtrip() {
        let entry = CatalogEntry {
            id: FileId::new(Uuid::nil(), Uuid::from_u128(1), 1, 42),
            remote_key: "tenant/sfst/2026-04-17/foo.sfst".into(),
            min_timestamp_s: 1_700_000_000,
            max_timestamp_s: 1_700_003_600,
            record_count: 1234,
            // The catalog treats `content_meta` as opaque (content-agnostic, no
            // identity-codec dep): a hand-built blob for ("prod", "api") —
            // version 1, u16-LE-len-prefixed namespace then name. The partition
            // key lives in `id` above, not as a separate field.
            content_meta: vec![1, 4, 0, b'p', b'r', b'o', b'd', 3, 0, b'a', b'p', b'i'],
            size: ByteSize(9876),
            uploaded_at_ns: TimestampNs(1_700_003_700_000_000_000),
            remote_etag: Some("\"d41d8cd98f00b204e9800998ecf8427e\"".into()),
        };
        let json = serde_json::to_vec(&entry).unwrap();
        let parsed: CatalogEntry = serde_json::from_slice(&json).unwrap();
        assert_eq!(parsed, entry);
    }
}
