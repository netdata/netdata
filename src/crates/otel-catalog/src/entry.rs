pub use file_registry::ServiceStream;
use file_registry::{ByteSize, FileId, TimestampNs};
use serde::{Deserialize, Serialize};

/// One uploaded SFST file tracked by the catalog.
///
/// Each entry corresponds to exactly one SFST, which itself contains
/// exactly one stream — see [`file_registry::ServiceStream`].
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct CatalogEntry {
    pub id: FileId,
    pub remote_key: String,
    pub min_timestamp_s: u32,
    pub max_timestamp_s: u32,
    pub total_logs: u32,
    pub stream: ServiceStream,
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

#[cfg(test)]
mod tests {
    use super::*;
    use uuid::Uuid;

    #[test]
    fn stream_entry_roundtrip() {
        let s = ServiceStream::new("prod", "api");
        let json = serde_json::to_string(&s).unwrap();
        let parsed: ServiceStream = serde_json::from_str(&json).unwrap();
        assert_eq!(parsed, s);
    }

    #[test]
    fn stream_entry_empty_strings_roundtrip() {
        let s = ServiceStream::new("", "");
        let json = serde_json::to_string(&s).unwrap();
        let parsed: ServiceStream = serde_json::from_str(&json).unwrap();
        assert_eq!(parsed, s);
    }

    #[test]
    fn catalog_entry_roundtrip() {
        let entry = CatalogEntry {
            id: FileId::new(Uuid::nil(), Uuid::from_u128(1), 1, 42),
            remote_key: "tenant/sfst/2026-04-17/foo.sfst".into(),
            min_timestamp_s: 1_700_000_000,
            max_timestamp_s: 1_700_003_600,
            total_logs: 1234,
            stream: ServiceStream::new("prod", "api"),
            size: ByteSize(9876),
            uploaded_at_ns: TimestampNs(1_700_003_700_000_000_000),
            remote_etag: Some("\"d41d8cd98f00b204e9800998ecf8427e\"".into()),
        };
        let json = serde_json::to_vec(&entry).unwrap();
        let parsed: CatalogEntry = serde_json::from_slice(&json).unwrap();
        assert_eq!(parsed, entry);
    }
}
