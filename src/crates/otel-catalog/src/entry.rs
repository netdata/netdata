//! The catalog's row type: one uploaded SFST as the remote catalog records it.
//!
//! `CatalogEntry` is the row type behind both roles of `Catalog` (sibling
//! `catalog.rs`): the file-lifecycle catalog builder accumulates rows and
//! serializes them into the `Envelope`'s `entries` array, and every reader
//! parses them back out of it. Production construction goes through
//! file-lifecycle `helpers::build_catalog_entry`, the only production
//! constructor: it copies `id`, the summary fields and `size` from the
//! registered `sfst::File`, and stamps the upload's `remote_key`,
//! `uploaded_at_ns` and `remote_etag`. The remote-LIST reconcile
//! (file-lifecycle `recovery/remote.rs`) is the second caller — the
//! flush-failure repair re-cataloging uploaded-but-uncataloged SFSTs —
//! stamping "now" and `None` when the true upload metadata is unknown.
//!
//! Serde: entries serialize under their Rust field names inside the `JSON`
//! chunk; `remote_etag` is the only `#[serde(default)]` field, and this
//! module's roundtrip test pins a lossless serialize↔parse round trip.
//! Parse failures surface as `Error::Json` through `Catalog::from_json`; a
//! well-formed body with wrong values is not checked here — recovery's
//! `validate_catalog` rejects it by cross-checking every entry against its
//! catalog and remote key.
//!
//! Consumers (grep-verified): sibling `catalog.rs` (map values, `find`
//! yields) and `registry.rs` (`read_entries` returns them); file-lifecycle
//! `query.rs` (remote-plan selection), `remote_read.rs` (fetch-back:
//! downloads through `remote_key`, returns `summary()` results as
//! `SelectedFile`s), `registry.rs::with_catalog` (folds `content_meta` into
//! the stream selector), `ipc.rs` (`AddEntry` carries a row) and
//! `recovery/startup.rs` (`validate_catalog` parses each entry's
//! `remote_key`); otel-ledger `ledger/uploader.rs` (constructs through the
//! helper) and its rpc handlers (consume the remote plan; traces
//! `rpc/traces/sources.rs` also calls `summary()`).
use file_registry::{ByteSize, FileId, TimestampNs};
use serde::{Deserialize, Serialize};

/// One uploaded SFST as one catalog row.
///
/// One entry describes exactly one SFST, and an SFST holds exactly one
/// partition: its `part_key` lives only in `id` and is what query filtering
/// matches on; the stream identity itself is the opaque `content_meta` blob.
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct CatalogEntry {
    /// The SFST's full identity (`FileId` — see file-registry `types.rs`) and
    /// the map key of `Catalog::entries`: one entry per `FileId` per catalog.
    /// `id.part_key` is the partition `Query::partition_keys` filters on;
    /// `id.seq` sorts remote-plan output and, with the identity, forms the
    /// `SeqKey` lifecycle key.
    pub id: FileId,
    /// The remote object key of the SFST's bytes, built at upload-request
    /// time by file-lifecycle `remote_keys::sfst` and echoed back on
    /// `Uploaded`. The fetch-back path downloads through it
    /// (`remote_read.rs`), and recovery's `validate_catalog` requires it to
    /// parse as an SFST key of this machine, tenant and signal, dated like
    /// its catalog, whose embedded `FileId` equals `id`.
    pub remote_key: String,
    /// Earliest record timestamp in the file, seconds since the Unix epoch.
    /// From the SFST summary; drives the inclusive `[min, max]` overlap
    /// filter in `Catalog::find` and the min-ts filename field
    /// `Catalog::fold` computes.
    pub min_timestamp_s: u32,
    /// Latest record timestamp in the file, seconds since the Unix epoch.
    /// From the SFST summary; the max-ts of the range `Catalog::fold`
    /// computes.
    pub max_timestamp_s: u32,
    /// Number of records (rows) the file holds, from the SFST summary.
    /// Production entries are never empty: otel-ledger deletes an empty
    /// index before upload, so nothing with `record_count == 0` is cataloged.
    pub record_count: u32,
    /// Opaque content-plane identity blob, stored verbatim and never parsed
    /// by the catalog; the content plane decodes it for display. Carried
    /// through `summary()` and the stream selector's fold, so the query layer
    /// sees the blob the file was written with. The partition key is NOT
    /// stored here — it lives only in `id`, the single source of truth.
    pub content_meta: Vec<u8>,
    /// The SFST's file size in bytes, from the registered `sfst::File`.
    /// Sums into the remote plan's footprint (`RemotePlanInput::plan_within`)
    /// and bounds the fetch-back download (`remote_read.rs` sizes the
    /// deadline from it, and the download cache verifies the returned bytes
    /// against it).
    pub size: ByteSize,
    /// Nanoseconds since the Unix epoch, stamped at entry construction: the
    /// upload-completion time on the normal path, "now" on the remote-LIST
    /// rebuild (the true upload time is unknown). Recorded but not read by
    /// any production code today.
    pub uploaded_at_ns: TimestampNs,
    /// Remote object validator (the S3 ETag, when the backend returns one)
    /// captured at upload time: the uploader forwards the `Storage` upload
    /// metadata and `build_catalog_entry` stamps it; `None` when the backend
    /// returned none and for entries rebuilt from a remote LIST. No
    /// production code reads it today — recorded for future integrity/scrub
    /// use. `#[serde(default)]` covers a same-version catalog whose writer
    /// omitted the key (it parses as `None`); it is NOT a cross-version
    /// compat mechanism — catalogs of any other `FORMAT_VERSION` are
    /// hard-rejected before any field is deserialized. Serialization always
    /// writes the key (`null` when `None`).
    #[serde(default)]
    pub remote_etag: Option<String>,
}

impl CatalogEntry {
    /// Rebuild the content-agnostic `file_registry::FileSummary` from the
    /// entry's own fields — the inverse of what `build_catalog_entry` copied
    /// in, so a fetch-back hands the query layer the same summary the SFST
    /// carried (`remote_read.rs`, traces `rpc/traces/sources.rs`).
    pub fn summary(&self) -> file_registry::FileSummary {
        file_registry::FileSummary {
            min_timestamp_s: self.min_timestamp_s,
            max_timestamp_s: self.max_timestamp_s,
            record_count: self.record_count,
            content_meta: self.content_meta.clone(),
        }
    }
}

/// Deterministic opaque partition key for tests. The catalog treats `part_key`
/// as an opaque `u64` and never decodes it, so tests fabricate distinct keys
/// per logical stream without depending on the content-plane identity codec —
/// same label → same key, different label → (almost surely) different key.
/// A `pub(crate)` fixture for the sibling tests (`catalog.rs`, `registry.rs`);
/// the same helper is mirrored in file-lifecycle (`test_helpers.rs`), `wal`
/// and `sfst`.
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
            // A hand-built blob for ("prod", "api") in the content codec's
            // layout: version 1, u16-LE-len-prefixed namespace then name —
            // a reader with the codec would decode it, while the catalog
            // itself never parses it. The partition key lives in `id` above.
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
