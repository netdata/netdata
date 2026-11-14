//! The catalog's row type: one uploaded SFST as the remote catalog records it.
//!
//! Serde: entries serialize under their Rust field names inside the `JSON`
//! chunk. A well-formed body with wrong values is not checked here.
use file_registry::{ByteSize, FileId, TimestampNs};
use serde::{Deserialize, Serialize};

/// One uploaded SFST as one catalog row.
///
/// One entry describes exactly one SFST, and an SFST holds exactly one
/// partition: its `partition_key` lives only in `id` and is what query
/// filtering matches on; the stream identity itself is the opaque
/// `content_meta` blob.
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct CatalogEntry {
    /// The SFST's full identity (`FileId` — see file-registry `types.rs`) and
    /// the map key of `Catalog::entries`: one entry per `FileId` per catalog.
    /// `id.partition_key` is the partition `Query::partition_keys` filters on.
    pub id: FileId,
    /// The remote object key of the SFST's bytes.
    pub remote_key: String,
    /// Earliest record timestamp in the file, seconds since the Unix epoch.
    /// From the SFST summary.
    pub min_timestamp_s: u32,
    /// Latest record timestamp in the file, seconds since the Unix epoch.
    /// From the SFST summary.
    pub max_timestamp_s: u32,
    /// Number of records (rows) the file holds, from the SFST summary.
    pub record_count: u32,
    /// Opaque stream identity blob, stored verbatim and never parsed by the
    /// catalog. Carried through `summary()`. The partition key is not stored
    /// here; it lives only in `id`.
    pub content_meta: Vec<u8>,
    /// The SFST's file size in bytes.
    pub size: ByteSize,
    /// Nanoseconds since the Unix epoch, stamped at entry construction: the
    /// upload-completion time on the normal path, "now" on the remote-LIST
    /// rebuild (the true upload time is unknown).
    pub uploaded_at_ns: TimestampNs,
    /// Remote object validator (the S3 ETag, when the backend returns one)
    /// captured at upload time. `None` when the backend returned none and for
    /// entries rebuilt from a remote LIST. `#[serde(default)]` covers a
    /// same-version catalog whose writer omitted the key (it parses as
    /// `None`). It is not a cross-version compat mechanism: catalogs of any
    /// other `FORMAT_VERSION` are rejected before any field is deserialized.
    /// Serialization always writes the key (`null` when `None`).
    #[serde(default)]
    pub remote_etag: Option<String>,
}

impl CatalogEntry {
    /// Rebuild the content-agnostic `file_registry::FileSummary` from the
    /// entry's own fields.
    pub fn summary(&self) -> file_registry::FileSummary {
        file_registry::FileSummary {
            min_timestamp_s: self.min_timestamp_s,
            max_timestamp_s: self.max_timestamp_s,
            record_count: self.record_count,
            content_meta: self.content_meta.clone(),
        }
    }
}

/// Deterministic opaque partition key for tests. The catalog treats
/// `partition_key` as an opaque `u64` and never decodes it, so tests fabricate
/// distinct keys per logical stream without depending on the stream identity
/// codec — same label → same key, different label → (almost surely)
/// different key.
#[cfg(test)]
pub(crate) fn opaque_partition_key(namespace: &str, name: &str) -> u64 {
    use std::hash::{Hash, Hasher};
    let mut h = std::collections::hash_map::DefaultHasher::new();
    namespace.hash(&mut h);
    name.hash(&mut h);
    h.finish()
}

#[cfg(test)]
mod tests {
    use super::*;
    use file_registry::{Identity, InstanceId, MachineId};
    use uuid::Uuid;

    #[test]
    fn catalog_entry_roundtrip() {
        let ident = Identity::new(
            MachineId::new(Uuid::from_u128(0xa1)).unwrap(),
            InstanceId::new(Uuid::from_u128(1)).unwrap(),
        );
        let entry = CatalogEntry {
            id: FileId::new(ident, 0, 1, 42),
            remote_key: "tenant/sfst/2026-04-17/foo.sfst".into(),
            min_timestamp_s: 1_700_000_000,
            max_timestamp_s: 1_700_003_600,
            record_count: 1234,
            // A hand-built blob for ("prod", "api"); the catalog itself
            // never parses it. The partition key lives in `id` above.
            content_meta: vec![1, 4, 0, b'p', b'r', b'o', b'd', 3, 0, b'a', b'p', b'i'],
            size: ByteSize(9876),
            uploaded_at_ns: TimestampNs(1_700_003_700_000_000_000),
            remote_etag: Some("\"d41d8cd98f00b204e9800998ecf8427e\"".into()),
        };
        let json = serde_json::to_vec(&entry).unwrap();
        let parsed: CatalogEntry = serde_json::from_slice(&json).unwrap();
        assert_eq!(parsed, entry);

        // The stored name of the partition key is part of the catalog format,
        // so renaming the Rust field must not change it.
        let text = String::from_utf8(json).unwrap();
        assert!(text.contains(r#""part_key":42"#), "{text}");
        assert!(!text.contains("partition_key"), "{text}");
    }
}
