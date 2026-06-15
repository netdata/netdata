use super::*;

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

#[test]
fn empty_stub_has_expected_shape() {
    let r = LogsResult::empty_stub(100, 200, 200);
    let v = serde_json::to_value(&r).unwrap();
    assert_eq!(v["status"], 200);
    assert_eq!(v["progress"], 100);
    assert_eq!(v["v"], 3);
    assert_eq!(v["type"], "table");
    assert!(v["facets"].as_array().unwrap().is_empty());
    assert!(v["available_histograms"].as_array().unwrap().is_empty());
    assert!(v["data"].as_array().unwrap().is_empty());
    assert!(v["columns"].is_object());
    assert_eq!(v["items"]["max_to_return"], 200);
    assert_eq!(v["items"]["matched"], 0);
    assert_eq!(v["histogram"]["chart"]["view"]["after"], 100);
    assert_eq!(v["histogram"]["chart"]["view"]["before"], 200);
    assert_eq!(v["pagination"]["key"], "anchor");
}
