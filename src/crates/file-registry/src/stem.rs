//! Shared filename-stem codec for the `{machine}-{invocation}-...` shape.
//!
//! Every durable artifact's filename starts with the producing
//! machine's id and invocation id as 32-hex-char simple UUIDs; the fields
//! after that prefix are the artifact's own (pipeline + seq + part_key for
//! data files, seq + time bounds for catalogs). This module owns the shared
//! prefix so no format parses it by hand.

use uuid::Uuid;

/// Format the `{machine}-{invocation}` prefix (two simple-form UUIDs).
pub fn format_uuid_pair(machine_id: Uuid, invocation_id: Uuid) -> String {
    format!("{}-{}", machine_id.as_simple(), invocation_id.as_simple())
}

/// Parse a stem's `{32hex}-{32hex}-` prefix; returns the two UUIDs and
/// the rest of the stem after the second separator. `None` for any
/// shape violation (short input, wrong separators, non-hex).
pub fn parse_uuid_pair(stem: &str) -> Option<(Uuid, Uuid, &str)> {
    let machine_str = stem.get(..32)?;
    if stem.as_bytes().get(32)? != &b'-' {
        return None;
    }
    let invocation_str = stem.get(33..65)?;
    if stem.as_bytes().get(65)? != &b'-' {
        return None;
    }
    let machine_id = Uuid::try_parse(machine_str).ok()?;
    let invocation_id = Uuid::try_parse(invocation_str).ok()?;
    Some((machine_id, invocation_id, stem.get(66..)?))
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
        // Missing separator / second uuid / non-hex.
        assert!(parse_uuid_pair(&format!("{m}x{m}-rest")).is_none());
        assert!(parse_uuid_pair(&format!("{m}-short-rest")).is_none());
        // Empty rest is the caller's problem, not a prefix violation.
        assert!(parse_uuid_pair(&format!("{m}-{m}-")).is_some());
    }
}
