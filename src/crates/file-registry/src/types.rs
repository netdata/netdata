use std::borrow::Borrow;
use std::fmt;
use std::hash::Hasher;
use std::path::Path;
use std::sync::Arc;

use serde::{Deserialize, Deserializer, Serialize, Serializer};
use twox_hash::XxHash64;
use uuid::Uuid;

// ---------------------------------------------------------------------------
// TenantId
// ---------------------------------------------------------------------------

/// Cheaply-cloneable tenant identifier. Wire format is an opaque string.
///
/// Wraps an `Arc<str>` so cloning is a refcount bump and multiple routing
/// entries for the same tenant share one heap allocation.
#[derive(Debug, Clone, PartialEq, Eq, Hash, PartialOrd, Ord)]
pub struct TenantId(Arc<str>);

impl TenantId {
    /// The tenant every record lands under when authentication is
    /// disabled, and the tenant an unscoped query reads. Ingest rejects
    /// clients claiming this id explicitly; the query side must be able
    /// to name it.
    pub const DEFAULT: &'static str = "default";

    /// Byte-length cap shared by both validation policies.
    pub const MAX_LEN: usize = 255;

    /// The [`DEFAULT`](TenantId::DEFAULT) tenant.
    pub fn default_tenant() -> Self {
        Self::from(Self::DEFAULT)
    }

    /// Ingest-side validation: a client-supplied tenant id becomes a
    /// per-tenant directory name, so the rules are strict — 1-255
    /// bytes of `[a-zA-Z0-9._-]`, and never `.`, `..`, or the literal
    /// [`DEFAULT`](TenantId::DEFAULT) (clients may not claim the
    /// auth-disabled tenant). The error is the human-readable reason,
    /// for the transport layer to wrap.
    pub fn validate_ingest(id: &str) -> Result<Self, &'static str> {
        if id.is_empty() || id.len() > Self::MAX_LEN {
            return Err("tenant ID must be 1-255 bytes");
        }
        if id == "." || id == ".." || id == Self::DEFAULT {
            return Err("tenant ID must not be '.', '..', or 'default'");
        }
        if !id
            .bytes()
            .all(|b| b.is_ascii_alphanumeric() || b == b'.' || b == b'_' || b == b'-')
        {
            return Err("tenant ID must contain only [a-zA-Z0-9._-]");
        }
        Ok(Self::from(id))
    }

    /// Query-side resolution: a tenant here is a scoping selector into
    /// registries built at ingest (a `HashMap` key, never a filesystem
    /// path), so the rules are deliberately permissive — and unlike
    /// ingest, the literal `default` must be nameable. Omitted, empty,
    /// or absurdly long values fall back to the default tenant; an
    /// unknown tenant simply matches nothing downstream.
    pub fn resolve_query(raw: Option<&str>) -> Self {
        match raw {
            Some(s) if !s.is_empty() && s.len() <= Self::MAX_LEN => Self::from(s),
            _ => Self::default_tenant(),
        }
    }

    pub fn as_str(&self) -> &str {
        &self.0
    }
}

impl fmt::Display for TenantId {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        f.write_str(&self.0)
    }
}

impl AsRef<str> for TenantId {
    fn as_ref(&self) -> &str {
        &self.0
    }
}

impl Borrow<str> for TenantId {
    fn borrow(&self) -> &str {
        &self.0
    }
}

impl From<&str> for TenantId {
    fn from(s: &str) -> Self {
        Self(Arc::from(s))
    }
}

impl From<String> for TenantId {
    fn from(s: String) -> Self {
        Self(Arc::from(s))
    }
}

impl Serialize for TenantId {
    fn serialize<S: Serializer>(&self, s: S) -> Result<S::Ok, S::Error> {
        s.serialize_str(&self.0)
    }
}

impl<'de> Deserialize<'de> for TenantId {
    fn deserialize<D: Deserializer<'de>>(d: D) -> Result<Self, D::Error> {
        String::deserialize(d).map(|s| TenantId(Arc::from(s)))
    }
}

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
// ns_hash
// ---------------------------------------------------------------------------

/// Compute the namespace hash for a `(service.namespace, service.name)` pair.
///
/// - Both absent -> `0` (sentinel: "no service attribution").
/// - Otherwise, feed each present field into an xxhash64 (seed 0) hasher
///   as `"service.namespace="` + value and/or `"service.name="` + value.
/// - If the resulting hash is `0`, remap to `u64::MAX` so that the sentinel
///   remains unambiguous.
///
/// This is the low-level primitive. Identity-layer callers (the ingestor's
/// file naming, the query planner's stream filter, the offline CLI) MUST go
/// through [`ServiceStream::ns_hash`] instead, which applies the
/// absent-equals-empty rule. Calling this directly with `Some("")` vs `None`
/// is what produced the absent-vs-empty divergence this type guards against.
pub fn compute_ns_hash(namespace: Option<&str>, name: Option<&str>) -> u64 {
    if namespace.is_none() && name.is_none() {
        return 0;
    }

    let mut h = XxHash64::default();
    if let Some(ns) = namespace {
        h.write(b"service.namespace=");
        h.write(ns.as_bytes());
    }
    if let Some(n) = name {
        h.write(b"service.name=");
        h.write(n.as_bytes());
    }

    match h.finish() {
        0 => u64::MAX,
        v => v,
    }
}

// ---------------------------------------------------------------------------
// ServiceStream
// ---------------------------------------------------------------------------

/// `(namespace, name)` pair identifying a log stream.
///
/// Each WAL/SFST file holds exactly one stream — the WAL writer partitions
/// frames by [`ns_hash`](ServiceStream::ns_hash), and the indexer asserts
/// that all data in a single WAL file resolves to one stream. Hash
/// collisions are detected at the ingestor; a colliding WAL file is
/// permanently un-indexable until the operator removes it.
///
/// This is the canonical stream identifier across the codebase — the
/// registry, the catalog, the indexer, and the query planner all use it.
///
/// Absent equals empty: a missing attribute is stored as an empty string,
/// so a stream that carries no `service.namespace` and one that carries an
/// empty-string namespace are the *same* stream (OpenTelemetry treats a
/// zero-length namespace as unspecified). The derived `PartialEq`/`Hash`
/// then compare the stored strings byte-for-byte, with no case-folding or
/// trimming, so `("Prod", "api")` and `("prod", "api")` remain distinct.
#[derive(Debug, Clone, PartialEq, Eq, Hash, Serialize, Deserialize)]
pub struct ServiceStream {
    /// The OTLP `service.namespace` resource attribute, stored verbatim;
    /// empty string when the source carries no such attribute (an empty
    /// value and an absent one are equivalent — see [`ServiceStream::ns_hash`]).
    pub namespace: String,
    /// The OTLP `service.name` resource attribute, stored verbatim;
    /// empty string when the source carries no such attribute.
    pub name: String,
}

impl ServiceStream {
    pub fn new<N: Into<String>, M: Into<String>>(namespace: N, name: M) -> Self {
        Self {
            namespace: namespace.into(),
            name: name.into(),
        }
    }

    /// Canonical identity-layer hash for this stream.
    ///
    /// An empty field is treated as absent before hashing, so a stream that
    /// carries no `service.namespace` and one that carries an empty-string
    /// namespace resolve to the *same* `ns_hash`. This matches the
    /// OpenTelemetry rule that a zero-length namespace equals an unspecified
    /// one, and keeps a single file partition per logical stream. An all-empty
    /// stream therefore hashes to the `0` "no attribution" sentinel.
    ///
    /// The collapse is symmetric — both fields follow the empty→absent rule —
    /// because `ServiceStream` stores absent and empty alike as `""` and cannot
    /// tell them apart after extraction. OTel requires `service.name` to be
    /// non-empty, so the name-side collapse only triggers for a non-conformant
    /// sender; the namespace-side collapse is the one the spec mandates.
    ///
    /// This is the only correct way to derive an `ns_hash` from a
    /// `ServiceStream`; [`compute_ns_hash`] is the underlying primitive and
    /// must not be called with the absent/empty distinction at this layer.
    ///
    /// Note this is unrelated to the type's derived [`Hash`]: that hashes the
    /// stored strings byte-for-byte for `HashMap` bucketing, whereas `ns_hash`
    /// is the xxhash64 identity digest used for file naming and stream filters.
    pub fn ns_hash(&self) -> u64 {
        let ns = (!self.namespace.is_empty()).then_some(self.namespace.as_str());
        let name = (!self.name.is_empty()).then_some(self.name.as_str());
        compute_ns_hash(ns, name)
    }
}

// ---------------------------------------------------------------------------
// FileId
// ---------------------------------------------------------------------------

/// Uniquely identifies a file across machines, boots, pipelines, and sequences.
///
/// The filename format is:
/// `<machine_id>-<boot_id>-<pipeline_id:05>-<seq:010>-<part_key:016x>.<ext>`
/// where machine_id and boot_id are 32-character lowercase hex (no hyphens),
/// pipeline_id is a 5-digit zero-padded decimal, seq is a 10-digit zero-padded
/// decimal, and part_key is a 16-character zero-padded hex u64.
///
/// `part_key` is an opaque partition key and `pipeline_id` an opaque
/// signal/pipeline discriminator: this crate never interprets either. The
/// content plane derives them and assigns their meaning (for OTel logs today,
/// `part_key` is the service-stream hash — see [`ServiceStream::ns_hash`]).
/// `seq` is a single global counter, unique across pipelines, so it alone
/// identifies a file; `pipeline_id` routes it to its owning pipeline.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash, Serialize, Deserialize)]
pub struct FileId {
    pub machine_id: Uuid,
    pub boot_id: Uuid,
    /// Opaque pipeline/signal discriminator assigned by the integration layer.
    /// Today there is a single pipeline ([`FileId::DEFAULT_PIPELINE`]); distinct
    /// ids per signal are assigned in a later restructure stage.
    pub pipeline_id: u16,
    pub seq: u64,
    /// Opaque partition key; the content plane derives it.
    pub part_key: u64,
}

impl FileId {
    /// The default (and currently only) pipeline. The integration layer will
    /// assign distinct pipeline ids per signal in a later restructure stage;
    /// `file-registry` ascribes no meaning to the value.
    pub const DEFAULT_PIPELINE: u16 = 0;

    /// Construct a `FileId` in the [`DEFAULT_PIPELINE`](Self::DEFAULT_PIPELINE).
    pub fn new(machine_id: Uuid, boot_id: Uuid, seq: u64, part_key: u64) -> Self {
        Self::with_pipeline(machine_id, boot_id, Self::DEFAULT_PIPELINE, seq, part_key)
    }

    /// Construct a `FileId` in an explicit pipeline.
    pub fn with_pipeline(
        machine_id: Uuid,
        boot_id: Uuid,
        pipeline_id: u16,
        seq: u64,
        part_key: u64,
    ) -> Self {
        Self {
            machine_id,
            boot_id,
            pipeline_id,
            seq,
            part_key,
        }
    }

    /// Format the stem portion:
    /// `<machine_id>-<boot_id>-<pipeline_id:05>-<seq:010>-<part_key:016x>`
    pub fn to_stem(&self) -> String {
        format!(
            "{}-{:05}-{:010}-{:016x}",
            crate::stem::format_uuid_pair(self.machine_id, self.boot_id),
            self.pipeline_id,
            self.seq,
            self.part_key,
        )
    }

    /// Format a full filename: `<stem>.<ext>`
    pub fn to_filename(&self, ext: &str) -> String {
        format!("{}.{}", self.to_stem(), ext)
    }

    /// Parse a filename (not a full path) into a FileId.
    ///
    /// Expects: `<machine_id>-<boot_id>-<pipeline_id>-<seq>-<part_key>.<ext>`
    pub fn parse(path: &Path) -> Option<Self> {
        let name = path.file_stem()?.to_str()?;
        Self::parse_stem(name)
    }

    /// Parse just the stem: `<machine_id>-<boot_id>-<pipeline_id>-<seq>-<part_key>`
    pub fn parse_stem(stem: &str) -> Option<Self> {
        let (machine_id, boot_id, rest) = crate::stem::parse_uuid_pair(stem)?;

        // rest = "<pipeline_id>-<seq>-<part_key>": split off pipeline (first
        // '-'), then split the remainder into seq and part_key (last '-').
        let first_dash = rest.find('-')?;
        let pipeline_str = &rest[..first_dash];
        let after = &rest[first_dash + 1..];
        let last_dash = after.rfind('-')?;
        let seq_str = &after[..last_dash];
        let key_str = &after[last_dash + 1..];

        let pipeline_id = pipeline_str.parse().ok()?;
        let seq = seq_str.parse().ok()?;
        let part_key = u64::from_str_radix(key_str, 16).ok()?;

        Some(Self {
            machine_id,
            boot_id,
            pipeline_id,
            seq,
            part_key,
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
            .then_with(|| self.pipeline_id.cmp(&other.pipeline_id))
            .then_with(|| self.seq.cmp(&other.seq))
            .then_with(|| self.part_key.cmp(&other.part_key))
    }
}

impl PartialOrd for FileId {
    fn partial_cmp(&self, other: &Self) -> Option<std::cmp::Ordering> {
        Some(self.cmp(other))
    }
}

// ---------------------------------------------------------------------------
// FileSummary
// ---------------------------------------------------------------------------

/// Neutral, content-agnostic per-file summary: the cheap facts the substrate
/// needs to select, age, and account for a file, plus an opaque `content_meta`
/// blob that the content plane (de)serializes and the substrate never
/// interprets.
///
/// This is the substrate's replacement for the content-typed per-file summary
/// the storage tiers carried before the restructure (where the summary embedded
/// an OTLP `ServiceStream`). The time range and `record_count` drive candidate
/// selection and retention; `part_key` is the opaque partition key (for OTel
/// logs, the service-stream hash); `content_meta` carries the content plane's
/// per-file identity (for logs, the encoded `(namespace, name)` — see the
/// `otel-logs-identity` crate).
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct FileSummary {
    /// Earliest record timestamp in the file, seconds since the Unix epoch.
    pub min_timestamp_s: u32,
    /// Latest record timestamp in the file, seconds since the Unix epoch.
    pub max_timestamp_s: u32,
    /// Number of records (rows) the file holds.
    pub record_count: u32,
    /// Opaque partition key; the content plane derives it.
    pub part_key: u64,
    /// Opaque content-plane metadata, stored verbatim and never parsed by the
    /// substrate.
    pub content_meta: Vec<u8>,
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
            "550e8400e29b41d4a716446655440000-7f3b2a1e9c4d4f8ab1c2d3e4f5a6b7c8-00000-0000000042-a1b2c3d4e5f60001"
        );
        let parsed = FileId::parse_stem(&stem).unwrap();
        assert_eq!(parsed, id);
    }

    #[test]
    fn file_id_with_pipeline_roundtrip() {
        let id = FileId::with_pipeline(test_machine_id(), test_boot_id(), 7, 42, 0xdeadbeef);
        let stem = id.to_stem();
        assert!(stem.contains("-00007-0000000042-"));
        let parsed = FileId::parse_stem(&stem).unwrap();
        assert_eq!(parsed, id);
        assert_eq!(parsed.pipeline_id, 7);
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
        assert_eq!(parsed.part_key, 0);
    }

    #[test]
    fn file_id_max_hash() {
        let id = FileId::new(test_machine_id(), test_boot_id(), 1, u64::MAX);
        let stem = id.to_stem();
        assert!(stem.ends_with("-ffffffffffffffff"));
        let parsed = FileId::parse_stem(&stem).unwrap();
        assert_eq!(parsed.part_key, u64::MAX);
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

        // Same seq, different part_key
        let c = FileId::new(test_machine_id(), test_boot_id(), 1, 1);
        let d = FileId::new(test_machine_id(), test_boot_id(), 1, 2);
        assert!(c < d);

        // pipeline_id orders ahead of seq: a lower pipeline sorts first even
        // when its seq is higher.
        let p0 = FileId::with_pipeline(test_machine_id(), test_boot_id(), 0, 9, 0);
        let p1 = FileId::with_pipeline(test_machine_id(), test_boot_id(), 1, 1, 0);
        assert!(p0 < p1);
    }

    #[test]
    fn ns_hash_both_missing_is_zero() {
        assert_eq!(compute_ns_hash(None, None), 0);
    }

    #[test]
    fn ns_hash_with_namespace_only() {
        let h = compute_ns_hash(Some("prod"), None);
        assert_ne!(h, 0);
        assert_eq!(
            h,
            compute_ns_hash(Some("prod"), None),
            "must be deterministic"
        );
    }

    #[test]
    fn ns_hash_with_name_only() {
        let h = compute_ns_hash(None, Some("myapp"));
        assert_ne!(h, 0);
        assert_eq!(h, compute_ns_hash(None, Some("myapp")));
    }

    #[test]
    fn ns_hash_with_both() {
        let h = compute_ns_hash(Some("prod"), Some("myapp"));
        assert_ne!(h, 0);
        // Different from either field alone.
        assert_ne!(h, compute_ns_hash(Some("prod"), None));
        assert_ne!(h, compute_ns_hash(None, Some("myapp")));
    }

    #[test]
    fn ns_hash_different_values_differ() {
        let a = compute_ns_hash(Some("prod"), Some("app1"));
        let b = compute_ns_hash(Some("prod"), Some("app2"));
        let c = compute_ns_hash(Some("staging"), Some("app1"));
        assert_ne!(a, b);
        assert_ne!(a, c);
        assert_ne!(b, c);
    }

    #[test]
    fn service_stream_ns_hash_collapses_absent_and_empty() {
        // An empty-string field is treated as absent: a stream with an
        // empty namespace hashes identically to one with no namespace.
        assert_eq!(
            ServiceStream::new("", "myapp").ns_hash(),
            compute_ns_hash(None, Some("myapp")),
        );
        assert_eq!(
            ServiceStream::new("prod", "").ns_hash(),
            compute_ns_hash(Some("prod"), None),
        );
    }

    #[test]
    fn service_stream_ns_hash_all_empty_is_sentinel() {
        // Empty -> absent on both fields collapses to the "no attribution"
        // sentinel, matching `compute_ns_hash(None, None)`.
        assert_eq!(ServiceStream::new("", "").ns_hash(), 0);
    }

    #[test]
    fn service_stream_ns_hash_matches_primitive_when_present() {
        assert_eq!(
            ServiceStream::new("prod", "api").ns_hash(),
            compute_ns_hash(Some("prod"), Some("api")),
        );
    }
}

#[cfg(test)]
mod tenant_id_tests {
    use super::TenantId;

    #[test]
    fn validate_ingest_contract() {
        let cases: std::collections::HashMap<&str, (&str, bool)> = [
            ("simple id", ("tenant-a", true)),
            ("all allowed classes", ("a.B_9-z", true)),
            ("exactly max length", ("x".repeat(255).leak() as &str, true)),
            ("empty", ("", false)),
            ("over max length", ("x".repeat(256).leak() as &str, false)),
            ("dot", (".", false)),
            ("dotdot", ("..", false)),
            ("reserved default", ("default", false)),
            ("space", ("a b", false)),
            ("slash", ("a/b", false)),
            ("nul", ("a\0b", false)),
            ("non-ascii", ("café", false)),
        ]
        .into_iter()
        .collect();
        for (name, (input, ok)) in cases {
            assert_eq!(
                TenantId::validate_ingest(input).is_ok(),
                ok,
                "case '{name}' (input {input:?})"
            );
        }
    }

    #[test]
    fn resolve_query_contract() {
        // Omitted / empty / oversized fall back to the default tenant.
        assert_eq!(TenantId::resolve_query(None), TenantId::default_tenant());
        assert_eq!(
            TenantId::resolve_query(Some("")),
            TenantId::default_tenant()
        );
        let long = "x".repeat(256);
        assert_eq!(
            TenantId::resolve_query(Some(&long)),
            TenantId::default_tenant()
        );
        // In-range values pass through untouched.
        assert_eq!(
            TenantId::resolve_query(Some("tenant-a")).as_str(),
            "tenant-a"
        );
        // The asymmetric guarantee: the query side CAN name the literal
        // default tenant that ingest refuses to let clients claim.
        assert_eq!(
            TenantId::resolve_query(Some(TenantId::DEFAULT)),
            TenantId::default_tenant()
        );
        assert!(TenantId::validate_ingest(TenantId::DEFAULT).is_err());
    }

    #[test]
    fn default_tenant_is_the_constant() {
        assert_eq!(TenantId::default_tenant().as_str(), TenantId::DEFAULT);
    }
}
