//! Content-plane identity for OTel **logs**.
//!
//! This crate owns the OTel-logs notion of "which stream a file belongs to" —
//! the `(service.namespace, service.name)` pair ([`ServiceStream`]) — and how
//! that identity is derived into the substrate's opaque partition key and
//! serialized into the substrate's opaque per-file metadata blob.
//!
//! It is the content-plane counterpart to the content-agnostic substrate
//! (`file-registry`/`wal`/`sfst`/`otel-catalog`): the substrate stores an opaque
//! `part_key: u64` and an opaque `content_meta: Vec<u8>` and never interprets
//! either. This crate is the one place that gives them meaning, for logs — the
//! producer (`otel-ingestor`) encodes the identity here before writing, and the
//! query layer (`otel-ledger`) decodes it here for display. A second signal
//! (traces) will get its own sibling identity crate; the opinion-free hash
//! primitive is only extracted into a shared leaf crate if and when a second
//! signal actually needs to share it.

use std::hash::Hasher;

use serde::{Deserialize, Serialize};
use twox_hash::XxHash64;

// ---------------------------------------------------------------------------
// ServiceStream — the OTel-logs stream identity (owned here, content plane)
// ---------------------------------------------------------------------------

/// `(namespace, name)` pair identifying a log stream.
///
/// This is the canonical OTel-logs stream identifier. The content plane derives
/// it into the substrate's opaque `part_key` (via [`part_key`]) and serializes
/// it into the opaque `content_meta` blob (via [`encode_content_meta`]); the
/// substrate (`wal`/`sfst`/`otel-catalog`) never sees this type.
///
/// Absent equals empty: a missing attribute is stored as an empty string, so a
/// stream that carries no `service.namespace` and one that carries an
/// empty-string namespace are the *same* stream (OpenTelemetry treats a
/// zero-length namespace as unspecified). The derived `PartialEq`/`Hash` then
/// compare the stored strings byte-for-byte, with no case-folding or trimming,
/// so `("Prod", "api")` and `("prod", "api")` remain distinct.
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

/// Low-level `ns_hash` primitive: xxhash64 (seed 0) over the present fields.
///
/// - Both `None` → `0` (the "no attribution" sentinel).
/// - Otherwise feed each present field as `"service.namespace="` + value and/or
///   `"service.name="` + value; if the digest is `0`, remap to `u64::MAX` so the
///   sentinel stays unambiguous.
///
/// Identity-layer callers MUST go through [`ServiceStream::ns_hash`], which
/// applies the absent-equals-empty rule. Calling this directly with `Some("")`
/// vs `None` is what produced the absent-vs-empty divergence the type guards
/// against.
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

/// Schema version stamped as the first byte of every [`encode_content_meta`]
/// blob. Bump when the on-the-wire layout changes; [`decode_content_meta`]
/// rejects any other version so a format drift fails loudly rather than
/// silently mis-decoding.
pub const CONTENT_META_VERSION: u8 = 1;

/// Maximum byte length of a single `content_meta` field (`namespace` or
/// `name`). The on-wire length prefix is a `u16`, so a field cannot exceed
/// this; [`encode_content_meta`] returns `None` rather than panicking on a
/// longer field. OTel service-identity attributes are short in practice, but
/// they are attacker-controlled (copied verbatim from OTLP resource
/// attributes), so the encoder rejects pathological input instead of crashing
/// the writer.
pub const MAX_FIELD_BYTES: usize = u16::MAX as usize;

/// Derive the substrate partition key for a logs stream.
///
/// This is the content-plane entry point the write side uses to turn a
/// [`ServiceStream`] into the opaque `u64` the substrate files under. It is the
/// canonical `ServiceStream::ns_hash` (absent-equals-empty collapse included);
/// the substrate ascribes the result no meaning.
pub fn part_key(stream: &ServiceStream) -> u64 {
    stream.ns_hash()
}

/// Serialize a logs [`ServiceStream`] into the substrate's opaque `content_meta`
/// blob, or `None` if a field exceeds [`MAX_FIELD_BYTES`].
///
/// Layout: a 1-byte [`CONTENT_META_VERSION`] tag, then each field as a
/// little-endian `u16` byte-length prefix followed by its UTF-8 bytes
/// (`namespace` then `name`). The substrate stores these bytes verbatim and
/// never parses them; only this crate does. The encoder is fallible (mirroring
/// the decoder) so an over-long, attacker-controlled identity is rejected by
/// the caller rather than panicking the write path.
///
/// ```
/// use otel_logs_identity::{ServiceStream, encode_content_meta, decode_content_meta};
/// let s = ServiceStream::new("prod", "api");
/// let blob = encode_content_meta(&s).expect("a short identity always encodes");
/// assert_eq!(decode_content_meta(&blob).as_ref(), Some(&s));
/// ```
pub fn encode_content_meta(stream: &ServiceStream) -> Option<Vec<u8>> {
    let ns = stream.namespace.as_bytes();
    let name = stream.name.as_bytes();
    // Bound the attacker-controlled fields to the u16 length prefix here — the
    // single source of truth — and reject (None) before allocating, so a
    // pathological identity can neither panic nor drive a large allocation.
    let ns_len = u16::try_from(ns.len()).ok()?;
    let name_len = u16::try_from(name.len()).ok()?;
    let mut out = Vec::with_capacity(1 + 2 + ns.len() + 2 + name.len());
    out.push(CONTENT_META_VERSION);
    put_field(&mut out, ns_len, ns);
    put_field(&mut out, name_len, name);
    Some(out)
}

/// Reconstruct a logs [`ServiceStream`] from a `content_meta` blob produced by
/// [`encode_content_meta`].
///
/// Returns `None` on an unknown version, truncated/over-long input, trailing
/// bytes, or non-UTF-8 fields. This gives *structural* integrity only — it does
/// not detect content corruption that preserves structure (e.g. a bit-flip into
/// different valid UTF-8). A content checksum, if ever needed, belongs to the
/// Stage 2 framing layer, not this codec. (A typed error enum distinguishing the
/// rejection reasons for recovery diagnostics is likewise deferred to the first
/// real caller — the Stage 2 format flip.)
pub fn decode_content_meta(bytes: &[u8]) -> Option<ServiceStream> {
    let (&version, rest) = bytes.split_first()?;
    if version != CONTENT_META_VERSION {
        return None;
    }
    let (namespace, rest) = take_field(rest)?;
    let (name, rest) = take_field(rest)?;
    if !rest.is_empty() {
        return None;
    }
    Some(ServiceStream::new(
        std::str::from_utf8(namespace).ok()?,
        std::str::from_utf8(name).ok()?,
    ))
}

/// Decode `content_meta` for display, falling back to the empty stream
/// (`("", "")`) when the blob is absent or unparseable.
///
/// Stream-listing paths use this so a corrupt or missing blob still appears
/// (under an empty name) instead of vanishing; the authoritative `part_key`
/// is carried separately. Centralizing the fallback here keeps every listing
/// call site identical — the policy cannot drift between them.
pub fn decode_content_meta_or_empty(bytes: &[u8]) -> ServiceStream {
    decode_content_meta(bytes).unwrap_or_else(|| ServiceStream::new("", ""))
}

/// Append a field with its pre-validated `u16` length prefix. The length is
/// computed and bounded by the sole caller ([`encode_content_meta`]), so this
/// helper holds no cast that could silently truncate.
fn put_field(out: &mut Vec<u8>, len: u16, field: &[u8]) {
    out.extend_from_slice(&len.to_le_bytes());
    out.extend_from_slice(field);
}

fn take_field(bytes: &[u8]) -> Option<(&[u8], &[u8])> {
    let (len_bytes, rest) = bytes.split_at_checked(2)?;
    let len = u16::from_le_bytes([len_bytes[0], len_bytes[1]]) as usize;
    rest.split_at_checked(len)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn part_key_matches_ns_hash_and_distinguishes_streams() {
        // The cross-component invariant the file naming + Services selector
        // rely on: part_key IS exactly ServiceStream::ns_hash.
        let s = ServiceStream::new("prod", "api");
        assert_eq!(part_key(&s), s.ns_hash());
        // Distinct streams resolve to distinct keys.
        assert_ne!(
            part_key(&ServiceStream::new("prod", "api")),
            part_key(&ServiceStream::new("prod", "worker")),
        );
        // The all-empty stream collapses to the `0` "no attribution" sentinel.
        assert_eq!(part_key(&ServiceStream::new("", "")), 0);
    }

    #[test]
    fn content_meta_roundtrips() {
        for s in [
            ServiceStream::new("prod", "api"),
            ServiceStream::new("", "api"),  // absent namespace
            ServiceStream::new("prod", ""), // absent name
            ServiceStream::new("", ""),     // both absent
            ServiceStream::new("ns-with-dash", "svc.with.dots"),
            ServiceStream::new("производство", "日本-🦀"), // multi-byte UTF-8 preserved
        ] {
            let blob = encode_content_meta(&s).expect("short identity encodes");
            assert_eq!(blob[0], CONTENT_META_VERSION, "version tag first");
            assert_eq!(decode_content_meta(&blob).as_ref(), Some(&s));
        }
    }

    #[test]
    fn encode_rejects_oversize_field() {
        let huge = "x".repeat(MAX_FIELD_BYTES + 1);
        assert_eq!(encode_content_meta(&ServiceStream::new(huge.clone(), "api")), None);
        assert_eq!(encode_content_meta(&ServiceStream::new("prod", huge)), None);
        // Exactly at the limit still encodes.
        let max = "x".repeat(MAX_FIELD_BYTES);
        assert!(encode_content_meta(&ServiceStream::new(max, "api")).is_some());
    }

    #[test]
    fn decode_rejects_unknown_version() {
        let mut blob = encode_content_meta(&ServiceStream::new("prod", "api")).unwrap();
        blob[0] = 0xff; // a version this codec does not know (only 1 is valid)
        assert_eq!(decode_content_meta(&blob), None);
    }

    #[test]
    fn decode_rejects_non_utf8_field() {
        // Invalid UTF-8 in the namespace slot: version=1, ns len=2=[0xff,0xfe], empty name.
        assert_eq!(
            decode_content_meta(&[CONTENT_META_VERSION, 2, 0, 0xff, 0xfe, 0, 0]),
            None
        );
        // Symmetric: invalid UTF-8 in the name slot (empty namespace, name=[0xff,0xfe]).
        assert_eq!(
            decode_content_meta(&[CONTENT_META_VERSION, 0, 0, 2, 0, 0xff, 0xfe]),
            None
        );
    }

    #[test]
    fn decode_rejects_truncated_trailing_and_oversized_prefix() {
        let blob = encode_content_meta(&ServiceStream::new("prod", "api")).unwrap();
        // Truncated mid-field.
        assert_eq!(decode_content_meta(&blob[..blob.len() - 1]), None);
        // Empty input (no version byte).
        assert_eq!(decode_content_meta(&[]), None);
        // Bare version byte (no first length prefix).
        assert_eq!(decode_content_meta(&[CONTENT_META_VERSION]), None);
        // Trailing garbage after a valid blob.
        let mut extra = blob.clone();
        extra.push(0xff);
        assert_eq!(decode_content_meta(&extra), None);
        // Length prefix claims more bytes than are present.
        let oversized = [CONTENT_META_VERSION, 100, 0, b'x'];
        assert_eq!(decode_content_meta(&oversized), None);
    }

    #[test]
    fn decode_or_empty_falls_back_to_empty_stream() {
        // A valid blob round-trips through the fallback decoder.
        let blob = encode_content_meta(&ServiceStream::new("prod", "api")).unwrap();
        assert_eq!(
            decode_content_meta_or_empty(&blob),
            ServiceStream::new("prod", "api")
        );
        // Absent (empty) and unparseable blobs both fall back to the empty
        // stream rather than vanishing — the listing/display contract.
        let empty = ServiceStream::new("", "");
        assert_eq!(decode_content_meta_or_empty(&[]), empty);
        assert_eq!(decode_content_meta_or_empty(&[0xff, 0xff, 0xff]), empty);
    }

    // ── compute_ns_hash / ServiceStream::ns_hash semantics ──────────────

    #[test]
    fn service_stream_serde_roundtrip() {
        for s in [ServiceStream::new("prod", "api"), ServiceStream::new("", "")] {
            let json = serde_json::to_string(&s).unwrap();
            let parsed: ServiceStream = serde_json::from_str(&json).unwrap();
            assert_eq!(parsed, s);
        }
    }

    #[test]
    fn ns_hash_both_missing_is_zero() {
        assert_eq!(compute_ns_hash(None, None), 0);
    }

    #[test]
    fn ns_hash_with_namespace_only() {
        let h = compute_ns_hash(Some("prod"), None);
        assert_ne!(h, 0);
        assert_eq!(h, compute_ns_hash(Some("prod"), None), "must be deterministic");
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
    fn ns_hash_empty_field_collapses_to_absent() {
        // The absent-equals-empty rule `ServiceStream::ns_hash` enforces, on
        // BOTH fields: an empty-string field hashes the same as an absent one.
        assert_eq!(
            ServiceStream::new("", "api").ns_hash(),
            compute_ns_hash(None, Some("api"))
        );
        assert_eq!(
            ServiceStream::new("prod", "").ns_hash(),
            compute_ns_hash(Some("prod"), None)
        );
        assert_eq!(ServiceStream::new("", "").ns_hash(), 0);
    }

    #[test]
    fn ns_hash_matches_primitive_when_both_present() {
        // When both fields are present, `ns_hash` delegates straight to the
        // primitive (no collapse applies).
        assert_eq!(
            ServiceStream::new("prod", "api").ns_hash(),
            compute_ns_hash(Some("prod"), Some("api"))
        );
    }

    #[test]
    fn ns_hash_differs_from_literal_empty_namespace_primitive() {
        // The collapse means an empty-namespace stream does NOT hash like the
        // raw `Some("")` primitive — a literal-empty-namespace file written
        // before the collapse existed re-partitions under the new key (a
        // one-time rollover the substrate handled for short-lived WAL files).
        assert_ne!(
            ServiceStream::new("", "api").ns_hash(),
            compute_ns_hash(Some(""), Some("api"))
        );
    }
}
