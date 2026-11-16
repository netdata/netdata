//! Tests for the netdata-function wire types in `wire.rs` (this file is its
//! child `mod tests`): the serde behavior wire consumers depend on that
//! plain derives don't make obvious — which JSON type selects which
//! untagged `AnchorParam` variant, and the hand-written `Serialize` /
//! `Deserialize` impls on `DataPoint`.
//!
//! Fixtures are inline JSON literals driven straight through serde_json —
//! no files, engine, or handler involved. These pin transport shapes only;
//! the mapping onto the `sfsq::logs` engine is `adapter/tests.rs`.
//!
//! Not pinned here: `AnchorParam` serialization, rejection of an `anchor`
//! that is neither string nor number, the timestamp-only bucket (an array
//! with no `[v, arp, pa]` triples — currently accepted), and the rest of
//! the module's derived wire surface (request defaults, the `v` / `type`
//! field renames, the untagged response envelopes).
use super::*;

/// `anchor` in an `OtelLogsRequest` accepts both forms the UI sends, the
/// untagged enum choosing the variant by JSON type alone: a string is the
/// opaque row cursor echoed from a boundary row's hidden `cursor` column,
/// a bare number the microsecond timestamp of a histogram-bar click.
#[test]
fn anchor_param_deserializes_string_and_number() {
    let s: OtelLogsRequest = serde_json::from_slice(br#"{"anchor":"100:2:3"}"#).unwrap();
    assert!(matches!(s.anchor, Some(AnchorParam::Cursor(ref c)) if c == "100:2:3"));
    let n: OtelLogsRequest = serde_json::from_slice(br#"{"anchor":1780056601000000}"#).unwrap();
    assert!(matches!(
        n.anchor,
        Some(AnchorParam::TimestampUs(1780056601000000))
    ));
}

/// `DataPoint` serializes as the flat `[timestamp_ms, [v, arp, pa], …]`
/// array the cloud-frontend chart renderer expects, not a
/// `{"timestamp_ms":…, "items":…}` object. The comparison against an exact
/// `serde_json::Value` pins the shape — nesting and element order — not
/// just parseability.
#[test]
fn data_point_serializes_as_flat_array() {
    let dp = DataPoint {
        timestamp_ms: 1_700_000_000_000,
        items: vec![[5, 0, 0], [3, 0, 0]],
    };
    let v = serde_json::to_value(&dp).unwrap();
    assert_eq!(
        v,
        serde_json::json!([1_700_000_000_000u64, [5, 0, 0], [3, 0, 0]])
    );
}

/// The hand-written `Deserialize` impl reads back exactly what the
/// hand-written `Serialize` impl emits: serialize → parse reproduces the
/// original `timestamp_ms` and `items`. Both directions are manual, so
/// their mutual symmetry is a real contract rather than a derive guarantee.
#[test]
fn data_point_round_trip() {
    let dp = DataPoint {
        timestamp_ms: 42,
        items: vec![[1, 2, 3], [4, 5, 6]],
    };
    let s = serde_json::to_string(&dp).unwrap();
    let back: DataPoint = serde_json::from_str(&s).unwrap();
    assert_eq!(back.timestamp_ms, 42);
    assert_eq!(back.items, vec![[1, 2, 3], [4, 5, 6]]);
}
