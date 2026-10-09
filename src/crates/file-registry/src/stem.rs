//! The shared filename-stem codec for the identity prefix that starts every
//! durable artifact's filename: `{machine}-{instance}` — two 32-hex
//! simple-form UUIDs joined by one dash, 65 bytes total. This module owns
//! that prefix alone; the tail after it is each artifact's own:
//!
//! - data files: `FileId`'s `{pipeline_id:05}-{seq:010}-{part_key:016x}`
//!   (the stem contracts live in the `types` module docs);
//! - remote catalogs: `otel-catalog/src/registry.rs` appends
//!   `{max_seq:010}-{min_ts:010}-{max_ts:010}` (max_seq plus a `[min, max]`
//!   timestamp range in seconds) and parses the prefix with
//!   [`parse_uuid_pair`] — the only external user, and the reason this
//!   module is `pub`.
//!
//! Render/parse contract: [`format_uuid_pair`] always emits the canonical
//! lowercase rendering (`Uuid::as_simple`); [`parse_uuid_pair`] is strict
//! about shape (fixed byte offsets, dash separators, exactly-32-char
//! segments) but lenient about hex case — `Uuid::try_parse` on the fixed
//! 32-byte slices decodes upper- and lowercase alike, so non-canonically
//! cased stems still parse. Nil identities are not policed here: a nil
//! UUID is a parseable all-zero segment; each tail owner rejects it when
//! wrapping the parsed UUIDs into the `MachineId`/`InstanceId` newtypes.
//!
//! Consumers: `crate::types` — [`crate::FileId::to_stem`] formats through
//! [`format_uuid_pair`]; [`crate::FileId::parse_stem`] (and path-based
//! [`crate::FileId::parse`]) parse through [`parse_uuid_pair`]. `otel-catalog`
//! is the only external user (above).

use uuid::Uuid;

/// Format the `{machine}-{instance}` prefix: two simple-form UUIDs — 32
/// lowercase hex chars each — joined by one dash; 65 bytes with no
/// trailing separator (the caller starts its own tail with the next
/// dash).
pub fn format_uuid_pair(machine_id: Uuid, instance_id: Uuid) -> String {
    format!("{}-{}", machine_id.as_simple(), instance_id.as_simple())
}

/// Parse a stem's `{machine}-{instance}` prefix and return the two UUIDs
/// plus the tail after the second dash.
///
/// Shape-strict: bytes 32 and 65 must be `-` and the 32-byte segments
/// before and between them must be hex UUIDs — `Uuid::try_parse` on the
/// fixed 32-byte slices, so simple form only (a hyphenated UUID cannot
/// fit) and upper/lowercase hex both decode. `None` for any violation:
/// input shorter than 66 bytes, a non-dash separator, or a segment that
/// does not decode as hex. The returned tail borrows `stem` and may be
/// empty — an empty tail is prefix-valid; validating it is the caller's
/// job (each artifact owns its tail's format; nil identities are not
/// policed here — module docs).
pub fn parse_uuid_pair(stem: &str) -> Option<(Uuid, Uuid, &str)> {
    let machine_str = stem.get(..32)?;
    if stem.as_bytes().get(32)? != &b'-' {
        return None;
    }
    let instance_str = stem.get(33..65)?;
    if stem.as_bytes().get(65)? != &b'-' {
        return None;
    }
    let machine_id = Uuid::try_parse(machine_str).ok()?;
    let instance_id = Uuid::try_parse(instance_str).ok()?;
    Some((machine_id, instance_id, stem.get(66..)?))
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn roundtrip_and_rest() {
        let m = Uuid::from_u128(1);
        let b = Uuid::from_u128(2);
        let stem = format!("{}-tail-1-2", format_uuid_pair(m, b));
        let (pm, pb, rest) = parse_uuid_pair(&stem).unwrap();
        assert_eq!((pm, pb, rest), (m, b, "tail-1-2"));
    }

    #[test]
    fn rejects_malformed_prefixes() {
        assert!(parse_uuid_pair("").is_none());
        assert!(parse_uuid_pair("not-a-uuid").is_none());
        let m = Uuid::from_u128(1).as_simple().to_string();
        // Wrong separator at byte 32; second uuid cut off by a short input.
        assert!(parse_uuid_pair(&format!("{m}x{m}-rest")).is_none());
        assert!(parse_uuid_pair(&format!("{m}-short-rest")).is_none());
        // Empty rest is the caller's problem, not a prefix violation.
        assert!(parse_uuid_pair(&format!("{m}-{m}-")).is_some());
    }
}
