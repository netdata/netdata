use std::collections::BTreeMap;
use std::ops::Range;

use chrono::NaiveDate;
use chunk_file::ChunkId;
use chunk_file::container::{Container, ContainerBuilder};
use file_registry::{FileId, Query, TenantId};
use serde::{Deserialize, Serialize};
use uuid::Uuid;

use crate::entry::CatalogEntry;
use crate::{CONTAINER_MAGIC, CONTAINER_VERSION, Error, FORMAT_VERSION};

/// Chunk id of the JSON payload inside the catalog container. A future
/// binary payload is a new chunk id beside (or instead of) this one —
/// not a new file format.
const CHUNK_JSON: ChunkId = *b"JSON";

/// Per-tenant, per-date, per-machine, per-instance record of uploaded SFSTs.
///
/// The catalog file's identifying metadata (tenant, date, machine, instance)
/// is encoded in the path; entries carry their own per-SFST timestamps,
/// and the file's union `[min, max]` time range is encoded in the
/// filename. No per-catalog "created_at" timestamp is stored — nothing
/// in the planner reads it.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Catalog {
    pub tenant_id: TenantId,
    pub date: NaiveDate,
    pub machine_id: Uuid,
    pub instance_id: Uuid,
    pub entries: BTreeMap<FileId, CatalogEntry>,
}

impl Catalog {
    pub fn new(
        tenant_id: TenantId,
        date: NaiveDate,
        machine_id: Uuid,
        instance_id: Uuid,
    ) -> Self {
        Self {
            tenant_id,
            date,
            machine_id,
            instance_id,
            entries: BTreeMap::new(),
        }
    }

    pub fn add(&mut self, entry: CatalogEntry) {
        self.entries.insert(entry.id, entry);
    }

    pub fn remove(&mut self, id: &FileId) -> Option<CatalogEntry> {
        self.entries.remove(id)
    }

    // TODO: O(n) scan. The BTreeMap is keyed by FileId, not time, so range
    // filtering touches every entry. Fine at current scales (~hundreds of
    // entries per scope); revisit with an interval index or date-bucketed
    // key if query planner workloads show it matters.
    /// Iterate entries whose `[min_timestamp_s, max_timestamp_s]` range
    /// (inclusive on both ends) intersects the query's `[start, end)`
    /// range (half-open) — matching the convention used by
    /// `sfst::Registry::candidates` and `wal::Registry::candidates`.
    pub fn find<'a>(&'a self, q: &Query) -> impl Iterator<Item = &'a CatalogEntry> + 'a {
        // Extract q's contents upfront so the filter closures don't borrow
        // q. Decouples the iterator's lifetime from q's, letting callers
        // pass a temporary `Query`.
        let q_range = q.time_range.clone();
        let partition_keys = q.partition_keys.clone();
        self.entries
            .values()
            .filter(move |e| range_overlaps(e, &q_range))
            .filter(move |e| partition_keys.is_empty() || partition_keys.contains(&e.id.part_key))
    }

    /// Serialize to the on-disk container: magic `NCAT` + framing
    /// version + chunk-file TOC + a single `JSON` chunk holding the
    /// catalog JSON, with a crc32 trailer. All durable catalog bytes go
    /// through this — never raw JSON.
    pub fn to_container_bytes(&self) -> Result<Vec<u8>, Error> {
        let json = self.to_json()?;
        let mut builder = ContainerBuilder::new(CONTAINER_MAGIC, CONTAINER_VERSION);
        builder.add_chunk(CHUNK_JSON, &json);
        let mut out = Vec::new();
        builder.write_to(&mut out)?;
        Ok(out)
    }

    /// Parse the on-disk container produced by
    /// [`to_container_bytes`](Catalog::to_container_bytes), verifying
    /// magic, framing version and the `JSON` chunk's crc32 before
    /// deserializing.
    pub fn from_container_bytes(bytes: &[u8]) -> Result<Self, Error> {
        let container = Container::open(bytes, &CONTAINER_MAGIC, CONTAINER_VERSION)?;
        let json = container.chunk(CHUNK_JSON)?;
        Self::from_json(json)
    }

    fn to_json(&self) -> Result<Vec<u8>, Error> {
        let env = Envelope {
            version: FORMAT_VERSION,
            tenant_id: self.tenant_id.clone(),
            date: self.date,
            machine_id: self.machine_id,
            instance_id: self.instance_id,
            entries: self.entries.values().cloned().collect(),
        };
        Ok(serde_json::to_vec(&env)?)
    }

    fn from_json(bytes: &[u8]) -> Result<Self, Error> {
        // Peek the version before the full parse. An older-schema catalog
        // (e.g. v1's `total_logs`/`stream` entries) lacks the current required
        // fields, so a direct full deserialize would fail with a confusing
        // serde "missing field" error instead of a clean `UnsupportedVersion`.
        // This matches the WAL (header) and SFST (container) tiers, which both
        // reject on version before touching the payload.
        #[derive(serde::Deserialize)]
        struct VersionOnly {
            version: u32,
        }
        let VersionOnly { version } = serde_json::from_slice(bytes)?;
        if version != FORMAT_VERSION {
            return Err(Error::UnsupportedVersion(version));
        }
        let env: Envelope = serde_json::from_slice(bytes)?;
        let mut entries = BTreeMap::new();
        for entry in env.entries {
            entries.insert(entry.id, entry);
        }
        Ok(Self {
            tenant_id: env.tenant_id,
            date: env.date,
            machine_id: env.machine_id,
            instance_id: env.instance_id,
            entries,
        })
    }
}

/// True iff the entry's `[min_timestamp_s, max_timestamp_s]` range
/// (inclusive on both ends) shares any second with the query's
/// `[start, end)` range (half-open) — the shared
/// [`file_registry::range_overlaps`] rule.
fn range_overlaps(entry: &CatalogEntry, q: &Range<u32>) -> bool {
    file_registry::range_overlaps(q, entry.min_timestamp_s, entry.max_timestamp_s)
}

#[derive(Serialize, Deserialize)]
struct Envelope {
    version: u32,
    tenant_id: TenantId,
    date: NaiveDate,
    machine_id: Uuid,
    instance_id: Uuid,
    entries: Vec<CatalogEntry>,
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::entry::opaque_part_key;
    use file_registry::{ByteSize, TimestampNs};

    fn test_catalog() -> Catalog {
        Catalog::new(
            TenantId::from("tenant1"),
            NaiveDate::from_ymd_opt(2026, 4, 17).unwrap(),
            Uuid::nil(),
            Uuid::from_u128(1),
        )
    }

    fn entry_at(seq: u64, min_s: u32, max_s: u32, ns: &str, name: &str) -> CatalogEntry {
        let part_key = opaque_part_key(ns, name);
        CatalogEntry {
            id: FileId::new(Uuid::nil(), Uuid::from_u128(1), 0, seq, part_key),
            remote_key: format!("tenant1/sfst/2026-04-17/{seq}.sfst"),
            min_timestamp_s: min_s,
            max_timestamp_s: max_s,
            record_count: 10,
            content_meta: Vec::new(),
            size: ByteSize(1024),
            uploaded_at_ns: TimestampNs(2_000_000_000),
            remote_etag: None,
        }
    }

    #[test]
    fn new_has_empty_entries() {
        let c = test_catalog();
        assert!(c.entries.is_empty());
    }

    #[test]
    fn add_then_remove_returns_to_empty() {
        let mut c = test_catalog();
        let e = entry_at(1, 100, 200, "", "");
        c.add(e.clone());
        assert_eq!(c.entries.len(), 1);

        let removed = c.remove(&e.id).unwrap();
        assert_eq!(removed, e);
        assert!(c.entries.is_empty());
    }

    #[test]
    fn roundtrip_container_preserves_entries_and_metadata() {
        let mut c = test_catalog();
        c.add(entry_at(1, 100, 200, "prod", "api"));
        c.add(entry_at(2, 300, 500, "", ""));

        let bytes = c.to_container_bytes().unwrap();
        assert_eq!(&bytes[0..4], &CONTAINER_MAGIC, "container leads with NCAT");

        // v6 wire contract: the envelope key is `instance_id` (renamed from
        // `invocation_id` in v6, which was itself `boot_id` through v4). Pin the
        // JSON wire so a future serde rename can't silently flip it back without
        // failing here.
        let container =
            chunk_file::container::Container::open(&bytes, &CONTAINER_MAGIC, CONTAINER_VERSION)
                .unwrap();
        let json = std::str::from_utf8(container.chunk(CHUNK_JSON).unwrap()).unwrap();
        assert!(
            json.contains("\"instance_id\""),
            "v6 catalog JSON must use the instance_id key"
        );
        assert!(
            !json.contains("\"boot_id\""),
            "v6 catalog JSON must not carry the old boot_id key"
        );

        let parsed = Catalog::from_container_bytes(&bytes).unwrap();
        assert_eq!(parsed, c);
    }

    #[test]
    fn from_container_bytes_rejects_corruption_and_foreign_bytes() {
        let mut c = test_catalog();
        c.add(entry_at(1, 100, 200, "prod", "api"));
        let clean = c.to_container_bytes().unwrap();

        // Any flipped payload byte fails the JSON chunk's crc32.
        let mut corrupt = clean.clone();
        let last = corrupt.len() - 1;
        corrupt[last] ^= 0x01;
        match Catalog::from_container_bytes(&corrupt) {
            Err(Error::Container(chunk_file::container::Error::CrcMismatch { .. })) => {}
            other => panic!("expected CrcMismatch, got {other:?}"),
        }

        // Raw (legacy) JSON is not a container — rejected by magic.
        match Catalog::from_container_bytes(b"{\"version\":1}") {
            Err(Error::Container(chunk_file::container::Error::BadMagic)) => {}
            other => panic!("expected BadMagic, got {other:?}"),
        }

        // Unknown framing version.
        let mut wrong_version = clean;
        wrong_version[4..8].copy_from_slice(&99u32.to_le_bytes());
        match Catalog::from_container_bytes(&wrong_version) {
            Err(Error::Container(chunk_file::container::Error::UnsupportedVersion(99))) => {}
            other => panic!("expected UnsupportedVersion, got {other:?}"),
        }
    }

    #[test]
    fn find_range_overlap_semantics() {
        let mut c = test_catalog();
        c.add(entry_at(1, 100, 200, "", ""));
        c.add(entry_at(2, 300, 400, "", ""));
        c.add(entry_at(3, 150, 350, "", ""));

        // Window [50, 250) — file 1's max=200 is in range, file 3's
        // min=150 is in range, file 2's min=300 is past the upper bound.
        let q = Query {
            time_range: 50..250,
            partition_keys: Vec::new(),
        };
        let hits: Vec<u64> = c.find(&q).map(|e| e.id.seq).collect();
        assert_eq!(hits, vec![1, 3]);

        // Inclusive lower / exclusive upper edges. Window [200, 300):
        //  - file 1: max=200 ≥ 200 ✓ and min=100 < 300 ✓ → in
        //  - file 2: max=400 ≥ 200 ✓ and min=300 < 300 ✗ → out
        //  - file 3: max=350 ≥ 200 ✓ and min=150 < 300 ✓ → in
        let q = Query {
            time_range: 200..300,
            partition_keys: Vec::new(),
        };
        let hits: Vec<u64> = c.find(&q).map(|e| e.id.seq).collect();
        assert_eq!(hits, vec![1, 3]);

        let q = Query {
            time_range: 500..600,
            partition_keys: Vec::new(),
        };
        assert_eq!(c.find(&q).count(), 0);

        // Single-second range [200, 201) hits file 1 (max=200) and
        // file 3 (min=150, max=350); file 2's min=300 is out.
        let q = Query {
            time_range: 200..201,
            partition_keys: Vec::new(),
        };
        let hits: Vec<u64> = c.find(&q).map(|e| e.id.seq).collect();
        assert_eq!(hits, vec![1, 3]);
    }

    #[test]
    fn find_with_part_key_filter_matches_only_that_partition() {
        let mut c = test_catalog();
        // Two entries on one partition, one on another.
        c.add(entry_at(1, 100, 200, "prod", "api"));
        c.add(entry_at(2, 100, 200, "prod", "worker"));
        c.add(entry_at(3, 100, 200, "prod", "api"));

        let q = Query {
            time_range: 0..1000,
            partition_keys: vec![opaque_part_key("prod", "api")],
        };
        let hits: Vec<u64> = c.find(&q).map(|e| e.id.seq).collect();
        assert_eq!(hits, vec![1, 3]);
    }

    #[test]
    fn find_with_part_key_filter_excludes_non_matching_entries() {
        let mut c = test_catalog();
        c.add(entry_at(1, 100, 200, "", ""));
        c.add(entry_at(2, 100, 200, "prod", "api"));

        // The catalog compares `part_key` as an opaque u64: only the entry
        // whose key equals the query's is kept (entry 2 has a different key).
        let q = Query {
            time_range: 0..1000,
            partition_keys: vec![opaque_part_key("", "")],
        };
        let hits: Vec<u64> = c.find(&q).map(|e| e.id.seq).collect();
        assert_eq!(hits, vec![1]);
    }

    #[test]
    fn find_empty_query_matches_nothing() {
        let mut c = test_catalog();
        c.add(entry_at(1, 100, 200, "", ""));

        // start == end → empty window.
        let q = Query {
            time_range: 200..200,
            partition_keys: Vec::new(),
        };
        assert_eq!(c.find(&q).count(), 0);
    }

    #[test]
    fn from_json_rejects_unsupported_version() {
        let json = br#"{
            "version": 999,
            "tenant_id": "t",
            "date": "2026-04-17",
            "machine_id": "00000000-0000-0000-0000-000000000000",
            "boot_id": "00000000-0000-0000-0000-000000000000",
            "entries": []
        }"#;
        match Catalog::from_json(json) {
            Err(Error::UnsupportedVersion(999)) => {}
            other => panic!("expected UnsupportedVersion(999), got {other:?}"),
        }
    }

    #[test]
    fn from_json_rejects_old_schema_on_version_not_serde() {
        // A real v1 catalog: old-schema entry (`total_logs`/`stream`, missing
        // the current `record_count`/`content_meta`). The version
        // peek must reject it as `UnsupportedVersion(1)` — not a serde
        // "missing field" error from attempting the full parse.
        let json = br#"{
            "version": 1,
            "tenant_id": "t",
            "date": "2026-04-17",
            "machine_id": "00000000-0000-0000-0000-000000000000",
            "boot_id": "00000000-0000-0000-0000-000000000000",
            "entries": [
                {"id": "x", "total_logs": 5, "stream": {"namespace": "p", "name": "a"}}
            ]
        }"#;
        match Catalog::from_json(json) {
            Err(Error::UnsupportedVersion(1)) => {}
            other => panic!("expected UnsupportedVersion(1), got {other:?}"),
        }
    }

    #[test]
    fn from_json_rejects_v2_schema_on_version_not_serde() {
        // The immediately-superseded v2 schema carried a top-level `part_key`
        // on each entry (dropped in v3). The version peek must reject a v2
        // catalog as `UnsupportedVersion(2)` before any per-entry parse — never
        // misread the v2 `part_key` field into the v3 schema.
        let json = br#"{
            "version": 2,
            "tenant_id": "t",
            "date": "2026-04-17",
            "machine_id": "00000000-0000-0000-0000-000000000000",
            "boot_id": "00000000-0000-0000-0000-000000000000",
            "entries": [
                {"id": "x", "record_count": 5, "part_key": 42, "content_meta": []}
            ]
        }"#;
        match Catalog::from_json(json) {
            Err(Error::UnsupportedVersion(2)) => {}
            other => panic!("expected UnsupportedVersion(2), got {other:?}"),
        }
    }

    #[test]
    fn from_json_rejects_v3_schema_on_version_not_serde() {
        // The immediately-superseded v3 schema is structurally identical to v4,
        // but its entries' `remote_key`s reference the old segment-less remote
        // layout (`v1/tenants/...`). The version peek must reject a v3 catalog as
        // `UnsupportedVersion(3)` so its stale keys are never republished.
        let json = br#"{
            "version": 3,
            "tenant_id": "t",
            "date": "2026-04-17",
            "machine_id": "00000000-0000-0000-0000-000000000000",
            "boot_id": "00000000-0000-0000-0000-000000000000",
            "entries": [
                {"id": "x", "remote_key": "v1/tenants/t/sfst/2026-04-17/x.sfst",
                 "min_timestamp_s": 1, "max_timestamp_s": 2, "record_count": 5,
                 "content_meta": [], "size": 10, "uploaded_at_ns": 0, "remote_etag": null}
            ]
        }"#;
        match Catalog::from_json(json) {
            Err(Error::UnsupportedVersion(3)) => {}
            other => panic!("expected UnsupportedVersion(3), got {other:?}"),
        }
    }

    #[test]
    fn from_json_rejects_v4_schema_on_version_not_serde() {
        // v4 carried the envelope field `boot_id` (renamed to `invocation_id`
        // in v5, then to `instance_id` in v6). The version peek must reject a v4
        // catalog before the rename surfaces as a serde "missing field
        // `instance_id`" error. Pre-GA break; there is no migration.
        let json = br#"{
            "version": 4,
            "tenant_id": "t",
            "date": "2026-04-17",
            "machine_id": "00000000-0000-0000-0000-000000000000",
            "boot_id": "00000000-0000-0000-0000-000000000000",
            "entries": [
                {"id": "x", "remote_key": "v2/logs/tenants/t/sfst/2026-04-17/x.sfst",
                 "min_timestamp_s": 1, "max_timestamp_s": 2, "record_count": 5,
                 "content_meta": [], "size": 10, "uploaded_at_ns": 0, "remote_etag": null}
            ]
        }"#;
        match Catalog::from_json(json) {
            Err(Error::UnsupportedVersion(4)) => {}
            other => panic!("expected UnsupportedVersion(4), got {other:?}"),
        }
    }

    #[test]
    fn from_json_rejects_v5_schema_on_version_not_serde() {
        // v5 carried the envelope field `invocation_id` (renamed to
        // `instance_id` in v6). The version peek must reject a v5 catalog before
        // the rename surfaces as a serde "missing field `instance_id`" error.
        // Pre-GA break; there is no migration.
        let json = br#"{
            "version": 5,
            "tenant_id": "t",
            "date": "2026-04-17",
            "machine_id": "00000000-0000-0000-0000-000000000000",
            "invocation_id": "00000000-0000-0000-0000-000000000000",
            "entries": [
                {"id": "x", "remote_key": "v2/logs/tenants/t/sfst/2026-04-17/x.sfst",
                 "min_timestamp_s": 1, "max_timestamp_s": 2, "record_count": 5,
                 "content_meta": [], "size": 10, "uploaded_at_ns": 0, "remote_etag": null}
            ]
        }"#;
        match Catalog::from_json(json) {
            Err(Error::UnsupportedVersion(5)) => {}
            other => panic!("expected UnsupportedVersion(5), got {other:?}"),
        }
    }

    #[test]
    fn from_json_rejects_truncated_json() {
        let truncated = b"{\"version\": 1, \"tenant_id\": \"t";
        match Catalog::from_json(truncated) {
            Err(Error::Json(_)) => {}
            other => panic!("expected Json error, got {other:?}"),
        }
    }
}
