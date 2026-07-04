use std::borrow::Borrow;
use std::fmt;
use std::path::Path;
use std::sync::Arc;

use serde::{Deserialize, Deserializer, Serialize, Serializer};
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

    /// Byte-length cap shared by both validation policies. 64 keeps the
    /// deepest tenant-derived path well under legacy Windows `MAX_PATH`
    /// (260) and every path component under eCryptfs's ~143-char limit;
    /// the charset is ASCII-only, so bytes equal characters.
    pub const MAX_LEN: usize = 64;

    /// The [`DEFAULT`](TenantId::DEFAULT) tenant.
    pub fn default_tenant() -> Self {
        Self::from(Self::DEFAULT)
    }

    /// Ingest-side validation: a client-supplied tenant id becomes a
    /// per-tenant directory name, so the rules are strict — 1-64
    /// bytes of `[a-zA-Z0-9._-]`, and never `.`, `..`, or the literal
    /// [`DEFAULT`](TenantId::DEFAULT) (clients may not claim the
    /// auth-disabled tenant). The error is the human-readable reason,
    /// for the transport layer to wrap.
    pub fn validate_ingest(id: &str) -> Result<Self, &'static str> {
        if id.is_empty() || id.len() > Self::MAX_LEN {
            return Err("tenant ID must be 1-64 bytes");
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
// Identity: MachineId, InstanceId
// ---------------------------------------------------------------------------

/// Constructing a [`MachineId`] or [`InstanceId`] from the nil UUID failed. A
/// nil identity would render into filenames as a valid 32-hex zero prefix and is
/// never garbage-collected from remote storage, so it is refused at the type
/// boundary rather than re-checked at each write site.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct NilIdentity;

impl fmt::Display for NilIdentity {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        f.write_str("identity must not be the nil UUID")
    }
}

impl std::error::Error for NilIdentity {}

/// The Netdata machine GUID — the permanent node identity. Non-nil by
/// construction. Serializes as a bare UUID (identical wire to a raw `Uuid`), so
/// giving an existing `machine_id` field this type is wire-neutral.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash, PartialOrd, Ord, Serialize, Deserialize)]
#[serde(try_from = "Uuid", into = "Uuid")]
pub struct MachineId(Uuid);

/// The per-process instance identity — a fresh v4 UUID generated once per plugin
/// process, so each process (including a crash-respawn under one agent) has a
/// distinct identity. Non-nil by construction; wire-neutral (see [`MachineId`]).
#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash, PartialOrd, Ord, Serialize, Deserialize)]
#[serde(try_from = "Uuid", into = "Uuid")]
pub struct InstanceId(Uuid);

impl MachineId {
    /// Wrap a UUID as a machine id, rejecting the nil UUID.
    pub fn new(id: Uuid) -> Result<Self, NilIdentity> {
        if id.is_nil() {
            Err(NilIdentity)
        } else {
            Ok(Self(id))
        }
    }

    /// The underlying UUID (for filename rendering, comparison, logging).
    pub fn as_uuid(self) -> Uuid {
        self.0
    }
}

impl InstanceId {
    /// Wrap a UUID as an instance id, rejecting the nil UUID.
    pub fn new(id: Uuid) -> Result<Self, NilIdentity> {
        if id.is_nil() {
            Err(NilIdentity)
        } else {
            Ok(Self(id))
        }
    }

    /// Generate a fresh per-process instance id: a random v4 UUID, never nil.
    pub fn generate() -> Self {
        Self(Uuid::new_v4())
    }

    /// The underlying UUID (for filename rendering, comparison, logging).
    pub fn as_uuid(self) -> Uuid {
        self.0
    }
}

impl TryFrom<Uuid> for MachineId {
    type Error = NilIdentity;
    fn try_from(id: Uuid) -> Result<Self, NilIdentity> {
        Self::new(id)
    }
}

impl TryFrom<Uuid> for InstanceId {
    type Error = NilIdentity;
    fn try_from(id: Uuid) -> Result<Self, NilIdentity> {
        Self::new(id)
    }
}

impl From<MachineId> for Uuid {
    fn from(v: MachineId) -> Self {
        v.0
    }
}

impl From<InstanceId> for Uuid {
    fn from(v: InstanceId) -> Self {
        v.0
    }
}

impl fmt::Display for MachineId {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        fmt::Display::fmt(&self.0, f)
    }
}

impl fmt::Display for InstanceId {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        fmt::Display::fmt(&self.0, f)
    }
}

/// The `(machine, instance)` identity pair stamped into every [`FileId`]. Passed
/// as one value through the producer/threading layers (the WAL writer, the
/// ingestor services, the plugin config) so the two ids cannot be transposed at
/// a call site. Serializable so it can ride the plugin-config IPC to workers.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash, PartialOrd, Ord, Serialize, Deserialize)]
pub struct Identity {
    pub machine_id: MachineId,
    pub instance_id: InstanceId,
}

impl Identity {
    pub fn new(machine_id: MachineId, instance_id: InstanceId) -> Self {
        Self {
            machine_id,
            instance_id,
        }
    }
}

/// A fixed non-nil [`Identity`] shared by tests across the workspace. The exact
/// UUIDs are arbitrary — any non-nil pair works — but a single shared fixture
/// keeps test identities consistent and avoids re-spelling the constructor.
pub fn test_identity() -> Identity {
    Identity::new(
        MachineId::new(Uuid::from_u128(0x1111_2222_3333_4444_5555_6666_7777_8888)).unwrap(),
        InstanceId::new(Uuid::from_u128(0x9999_aaaa_bbbb_cccc_dddd_eeee_ffff_0000)).unwrap(),
    )
}

/// The `(machine, instance, seq)` projection of a [`FileId`], dropping the
/// pipeline/partition dimensions. It is the key for lifecycle state that crosses
/// an identity boundary — the upload/rotate/remote-cataloged marks derived from
/// remote LIST results or catalogs — so a seq reused by a different process
/// instance (post-wipe reseed) or a different machine (shared bucket) cannot
/// alias another's state. `seq` alone is only unique WITHIN one process
/// instance's local files, which is why cross-identity state needs the full key.
///
/// Deliberately NOT `Serialize`/`Deserialize` (unlike [`Identity`]/[`FileId`]):
/// it is a purely in-process map/IPC key over tokio channels, never persisted
/// and never crossing the ferryboat plugin-config boundary. Durable/cross-process
/// identity travels as a [`FileId`] (catalog wire) or [`Identity`] instead.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash, PartialOrd, Ord)]
pub struct SeqKey {
    pub machine_id: MachineId,
    pub instance_id: InstanceId,
    pub seq: u64,
}

impl SeqKey {
    /// Build a key from an identity and a seq (for sites that hold the two
    /// separately — e.g. a single-identity catalog's scope key plus its seqs).
    pub fn new(identity: Identity, seq: u64) -> Self {
        Self {
            machine_id: identity.machine_id,
            instance_id: identity.instance_id,
            seq,
        }
    }
}

impl From<&FileId> for SeqKey {
    fn from(id: &FileId) -> Self {
        Self {
            machine_id: id.machine_id,
            instance_id: id.instance_id,
            seq: id.seq,
        }
    }
}

impl std::fmt::Display for SeqKey {
    /// `<seq>@<machine>/<instance>` — the seq leads (the operator-facing handle)
    /// with the identity that disambiguates it after a reseed or in a shared
    /// bucket. Used in lifecycle log lines.
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        write!(
            f,
            "{}@{}/{}",
            self.seq,
            self.machine_id.as_uuid(),
            self.instance_id.as_uuid()
        )
    }
}

// ---------------------------------------------------------------------------
// FileId
// ---------------------------------------------------------------------------

/// Uniquely identifies a file across machines, process instances, pipelines,
/// and sequences.
///
/// The filename format is:
/// `<machine_id>-<instance_id>-<pipeline_id:05>-<seq:010>-<part_key:016x>.<ext>`
/// where machine_id and instance_id are 32-character lowercase hex (no
/// hyphens), pipeline_id is a 5-digit zero-padded decimal, seq is a 10-digit
/// zero-padded decimal, and part_key is a 16-character zero-padded hex u64.
///
/// `part_key` is an opaque partition key and `pipeline_id` an opaque
/// signal/pipeline discriminator: this crate never interprets either. The
/// content plane derives them and assigns their meaning (for OTel logs today,
/// `part_key` is the content plane's service-stream hash).
/// `seq` is a single counter, unique within ONE process instance across
/// pipelines — so it alone identifies a LOCAL file, but not across process
/// instances or machines (a post-wipe reseed or a shared bucket can repeat a
/// seq). State that crosses an identity boundary is keyed by [`SeqKey`], not
/// bare `seq`. `pipeline_id` routes a file to its owning pipeline.
///
/// Identity contract: `machine_id` is the Netdata machine GUID (permanent node
/// identity); `instance_id` is a fresh UUID the plugin generates once per
/// process at startup, so each plugin process — including a crash-respawn under
/// one agent — has a distinct identity. Both are UUIDs; the 32-hex filename
/// shape is unchanged from when this field held the OS boot id.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash, PartialOrd, Ord, Serialize, Deserialize)]
pub struct FileId {
    pub machine_id: MachineId,
    pub instance_id: InstanceId,
    /// Opaque pipeline/signal discriminator assigned by the integration layer
    /// (the signal↔id mapping is the integration layer's, e.g. `bridge::signals`;
    /// `file-registry` ascribes no meaning to the value). Always chosen
    /// explicitly at construction — there is no default pipeline.
    pub pipeline_id: u16,
    pub seq: u64,
    /// Opaque partition key; the content plane derives it.
    pub part_key: u64,
}

impl FileId {
    /// Construct a `FileId`. The `pipeline_id` is always chosen explicitly by the
    /// integration layer (the signal↔id mapping is its concern, e.g.
    /// `bridge::signals`); there is no default pipeline.
    pub fn new(identity: Identity, pipeline_id: u16, seq: u64, part_key: u64) -> Self {
        Self {
            machine_id: identity.machine_id,
            instance_id: identity.instance_id,
            pipeline_id,
            seq,
            part_key,
        }
    }

    /// Format the stem portion:
    /// `<machine_id>-<instance_id>-<pipeline_id:05>-<seq:010>-<part_key:016x>`
    pub fn to_stem(&self) -> String {
        format!(
            "{}-{:05}-{:010}-{:016x}",
            crate::stem::format_uuid_pair(self.machine_id.as_uuid(), self.instance_id.as_uuid()),
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
    /// Expects: `<machine_id>-<instance_id>-<pipeline_id>-<seq>-<part_key>.<ext>`
    pub fn parse(path: &Path) -> Option<Self> {
        let name = path.file_stem()?.to_str()?;
        Self::parse_stem(name)
    }

    /// Parse just the stem: `<machine_id>-<instance_id>-<pipeline_id>-<seq>-<part_key>`
    pub fn parse_stem(stem: &str) -> Option<Self> {
        let (machine_uuid, instance_uuid, rest) = crate::stem::parse_uuid_pair(stem)?;

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

        // A nil-bearing name is not a valid identity — reject it here so a
        // corrupt or pre-identity file surfaces as "unparseable" (warn+skip on
        // recovery) rather than a queryable file with zero provenance.
        Some(Self {
            machine_id: MachineId::new(machine_uuid).ok()?,
            instance_id: InstanceId::new(instance_uuid).ok()?,
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

// `Ord`/`PartialOrd` are derived: field order (machine_id, instance_id,
// pipeline_id, seq, part_key) yields the same ordering the previous hand-written
// impl produced, and the `MachineId`/`InstanceId` newtypes compare by their
// inner UUID's byte order (identical to the old `as_bytes().cmp()`).

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
/// an OTLP service stream). The time range and `record_count` drive candidate
/// selection and retention; `content_meta` carries the content plane's
/// per-file identity (for logs, the encoded `(namespace, name)` — see the
/// `otel-logs-identity` crate). The partition key is NOT a summary field — it
/// lives only in the file's [`FileId`] (filename), the single source of truth;
/// candidate filtering reads `id.part_key`.
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct FileSummary {
    /// Earliest record timestamp in the file, seconds since the Unix epoch.
    pub min_timestamp_s: u32,
    /// Latest record timestamp in the file, seconds since the Unix epoch.
    pub max_timestamp_s: u32,
    /// Number of records (rows) the file holds.
    pub record_count: u32,
    /// Opaque content-plane metadata, stored verbatim and never parsed by the
    /// substrate. The partition key is NOT stored here — it lives only in the
    /// file's `FileId` (filename), the single source of truth.
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

    fn test_instance_id() -> Uuid {
        Uuid::try_parse("7f3b2a1e9c4d4f8ab1c2d3e4f5a6b7c8").unwrap()
    }

    fn ident() -> Identity {
        Identity::new(
            MachineId::new(test_machine_id()).unwrap(),
            InstanceId::new(test_instance_id()).unwrap(),
        )
    }

    #[test]
    fn identity_newtypes_reject_nil() {
        assert_eq!(MachineId::new(Uuid::nil()), Err(NilIdentity));
        assert_eq!(InstanceId::new(Uuid::nil()), Err(NilIdentity));
        assert!(MachineId::new(test_machine_id()).is_ok());
        assert!(InstanceId::new(test_instance_id()).is_ok());
    }

    #[test]
    fn identity_newtypes_roundtrip_through_uuid() {
        // `TryFrom<Uuid>` / `From<_> for Uuid` are what serde uses; a non-nil
        // UUID must survive the round-trip unchanged, keeping the wire identical
        // to a bare `Uuid`.
        let m = MachineId::new(test_machine_id()).unwrap();
        let i = InstanceId::new(test_instance_id()).unwrap();
        assert_eq!(Uuid::from(m), test_machine_id());
        assert_eq!(Uuid::from(i), test_instance_id());
        assert_eq!(MachineId::try_from(test_machine_id()).unwrap(), m);
    }

    #[test]
    fn file_id_parse_stem_rejects_nil_identity() {
        // A nil-bearing stem (all-zero machine or instance) has no provenance;
        // the newtype constructors reject nil, so parse must return None and the
        // file routes to the recovery warn+skip path rather than becoming a
        // queryable zero-provenance file.
        let nil = "00000000000000000000000000000000";
        let good = MachineId::new(test_machine_id()).unwrap().as_uuid().simple().to_string();
        assert!(
            FileId::parse_stem(&format!("{nil}-{good}-00000-0000000001-0000000000000000")).is_none(),
            "nil machine_id must be rejected"
        );
        assert!(
            FileId::parse_stem(&format!("{good}-{nil}-00000-0000000001-0000000000000000")).is_none(),
            "nil instance_id must be rejected"
        );
    }

    #[test]
    fn instance_id_generate_is_non_nil_and_unique() {
        let a = InstanceId::generate();
        let b = InstanceId::generate();
        assert!(!a.as_uuid().is_nil());
        assert_ne!(a, b, "each generated instance id must be distinct");
    }

    #[test]
    fn file_id_stem_roundtrip() {
        let id = FileId::new(ident(), 0, 42, 0xa1b2c3d4e5f60001);
        let stem = id.to_stem();
        assert_eq!(
            stem,
            "550e8400e29b41d4a716446655440000-7f3b2a1e9c4d4f8ab1c2d3e4f5a6b7c8-00000-0000000042-a1b2c3d4e5f60001"
        );
        let parsed = FileId::parse_stem(&stem).unwrap();
        assert_eq!(parsed, id);
    }

    #[test]
    fn file_id_nonzero_pipeline_roundtrip() {
        let id = FileId::new(ident(), 7, 42, 0xdeadbeef);
        let stem = id.to_stem();
        assert!(stem.contains("-00007-0000000042-"));
        let parsed = FileId::parse_stem(&stem).unwrap();
        assert_eq!(parsed, id);
        assert_eq!(parsed.pipeline_id, 7);
    }

    #[test]
    fn file_id_filename_roundtrip() {
        let id = FileId::new(ident(), 0, 1, 0);
        let filename = id.to_filename("wal");
        let path = Path::new(&filename);
        let parsed = FileId::parse(path).unwrap();
        assert_eq!(parsed, id);
    }

    #[test]
    fn file_id_zero_hash() {
        let id = FileId::new(ident(), 0, 1, 0);
        let stem = id.to_stem();
        assert!(stem.ends_with("-0000000000000000"));
        let parsed = FileId::parse_stem(&stem).unwrap();
        assert_eq!(parsed.part_key, 0);
    }

    #[test]
    fn file_id_max_hash() {
        let id = FileId::new(ident(), 0, 1, u64::MAX);
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
        let a = FileId::new(ident(), 0, 1, 0);
        let b = FileId::new(ident(), 0, 2, 0);
        assert!(a < b);

        // Same seq, different part_key
        let c = FileId::new(ident(), 0, 1, 1);
        let d = FileId::new(ident(), 0, 1, 2);
        assert!(c < d);

        // pipeline_id orders ahead of seq: a lower pipeline sorts first even
        // when its seq is higher.
        let p0 = FileId::new(ident(), 0, 9, 0);
        let p1 = FileId::new(ident(), 1, 1, 0);
        assert!(p0 < p1);
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
            (
                "exactly max length",
                ("x".repeat(TenantId::MAX_LEN).leak() as &str, true),
            ),
            ("empty", ("", false)),
            (
                "over max length",
                ("x".repeat(TenantId::MAX_LEN + 1).leak() as &str, false),
            ),
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
        let long = "x".repeat(TenantId::MAX_LEN + 1);
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
