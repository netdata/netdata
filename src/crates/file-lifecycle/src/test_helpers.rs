//! Shared, content-agnostic test fixtures.
//!
//! `file-lifecycle` is signal-neutral, so its tests fabricate opaque partition
//! keys and `content_meta` blobs directly — with NO dependency on any
//! content-plane identity codec (e.g. `otel-logs-identity`). A logical stream is
//! named by a `(namespace, name)` pair purely for test readability; the
//! substrate treats `part_key` as an opaque `u64` and `content_meta` as opaque
//! bytes and never decodes either.

/// Deterministic opaque partition key for a logical `(namespace, name)`.
/// Same label → same key (`DefaultHasher`); the substrate ascribes it no
/// meaning. Mirrors the per-crate `opaque_part_key` test helpers in the other
/// substrate crates (intentionally duplicated — sharing would re-couple them).
pub(crate) fn opaque_part_key(namespace: &str, name: &str) -> u64 {
    use std::hash::{Hash, Hasher};
    let mut h = std::collections::hash_map::DefaultHasher::new();
    namespace.hash(&mut h);
    name.hash(&mut h);
    h.finish()
}

/// Encode a `(namespace, name)` pair into opaque `content_meta` bytes. The
/// substrate stores/forwards these verbatim; tests round-trip them via
/// [`decode_opaque`] to assert identity survives the lifecycle. Length-prefixed
/// and local to the tests — NOT the production content codec.
pub(crate) fn encode_opaque(namespace: &str, name: &str) -> Vec<u8> {
    let mut out = Vec::new();
    out.extend_from_slice(&(namespace.len() as u32).to_le_bytes());
    out.extend_from_slice(namespace.as_bytes());
    out.extend_from_slice(&(name.len() as u32).to_le_bytes());
    out.extend_from_slice(name.as_bytes());
    out
}

/// Inverse of [`encode_opaque`]. Tolerant: any input that is not a complete
/// `encode_opaque` payload (empty, truncated, or non-UTF-8) decodes to the empty
/// identity `("", "")` rather than panicking, so a future test author who hands
/// it a crafted slice gets a clear result instead of a slice/UTF-8 panic. Real
/// inputs come from [`encode_opaque`] and round-trip exactly.
pub(crate) fn decode_opaque(bytes: &[u8]) -> (String, String) {
    fn read_at(bytes: &[u8], at: usize) -> Option<(String, usize)> {
        let body = at.checked_add(4)?;
        let len = u32::from_le_bytes(bytes.get(at..body)?.try_into().ok()?) as usize;
        let end = body.checked_add(len)?;
        let s = String::from_utf8(bytes.get(body..end)?.to_vec()).ok()?;
        Some((s, end))
    }
    let Some((ns, next)) = read_at(bytes, 0) else {
        return (String::new(), String::new());
    };
    let Some((name, _)) = read_at(bytes, next) else {
        return (String::new(), String::new());
    };
    (ns, name)
}

/// `(part_key, content_meta)` for a logical stream, as a content plane would
/// derive them at index time — but with opaque test encodings.
pub(crate) fn identity_for(namespace: &str, name: &str) -> (u64, Vec<u8>) {
    (
        opaque_part_key(namespace, name),
        encode_opaque(namespace, name),
    )
}

/// A `Summary` for the logical stream with the given range and record count.
/// The partition key is NOT in the summary — it lives in the file's `FileId`;
/// tests that filter by partition pass the key via [`identity_for`] (or
/// [`opaque_part_key`]) into the `FileId`.
pub(crate) fn summary_for(
    namespace: &str,
    name: &str,
    record_count: u32,
    min_s: u32,
    max_s: u32,
) -> sfst::Summary {
    sfst::Summary {
        min_timestamp_s: min_s,
        max_timestamp_s: max_s,
        record_count,
        content_meta: encode_opaque(namespace, name),
    }
}

/// A zero-valued `Summary` with an empty identity. Used by registry/recovery
/// tests that call `Registry::track` without caring about the contents.
pub(crate) fn empty_summary() -> sfst::Summary {
    summary_for("", "", 0, 0, 0)
}
