use super::*;
use crate::flow::{FlowDirection, FlowFields, FlowRecord};
use std::collections::BTreeMap;

fn materialize_row_fields(store: &TierFlowIndexStore, row: &OpenTierRow) -> FlowFields {
    let mut fields = store
        .materialize_fields(row.flow_ref)
        .expect("materialize row fields");
    row.metrics.write_fields(&mut fields);
    fields
}

#[test]
fn accumulator_flushes_closed_bucket() {
    let mut acc = TierAccumulator::new(TierKind::Minute1);
    let mut store = TierFlowIndexStore::default();
    let ts = 120_000_000;

    let mut rec = FlowRecord::default();
    rec.protocol = 6;
    rec.src_addr = Some("10.0.0.1".parse().unwrap());
    rec.dst_addr = Some("10.0.0.2".parse().unwrap());
    rec.src_port = 12345;
    rec.dst_port = 443;
    rec.bytes = 100;
    rec.packets = 2;
    rec.flows = 1;

    let flow_ref = store
        .get_or_insert_record_flow(ts, &rec)
        .expect("intern tier flow");
    acc.observe_flow(ts, flow_ref, FlowMetrics::from_record(&rec));

    let rows = acc.flush_closed_rows(180_000_000);
    assert_eq!(rows.len(), 1);
    let fields = materialize_row_fields(&store, &rows[0]);
    assert_eq!(fields.get("PROTOCOL").map(String::as_str), Some("6"));
    assert_eq!(fields.get("BYTES").map(String::as_str), Some("100"));
    assert!(fields.get("SRC_ADDR").is_none());
    assert!(fields.get("DST_ADDR").is_none());
    assert!(fields.get("SRC_PORT").is_none());
    assert!(fields.get("DST_PORT").is_none());
}

#[test]
fn metrics_defaults_to_zero() {
    let fields = BTreeMap::new();
    let m = FlowMetrics::from_fields(&fields);
    assert_eq!(m.bytes, 0);
    assert_eq!(m.packets, 0);
}

#[test]
fn metrics_from_record_matches_from_fields() {
    let mut rec = FlowRecord::default();
    rec.bytes = 12345;
    rec.packets = 67;

    let fields = rec.to_fields();
    let m_fields = FlowMetrics::from_fields(&fields);
    let m_record = FlowMetrics::from_record(&rec);

    assert_eq!(m_fields, m_record);
}

#[test]
fn rollup_dimensions_round_trip() {
    let mut store = TierFlowIndexStore::default();
    let mut rec = FlowRecord::default();
    rec.flow_version = "v9";
    rec.exporter_ip = Some("192.0.2.10".parse().unwrap());
    rec.exporter_port = 12345;
    rec.protocol = 6;
    rec.set_etype(2048);
    rec.src_as = 64512;
    rec.dst_as = 15169;
    rec.src_as_name = "AS64512 Example Transit".to_string();
    rec.dst_as_name = "AS15169 Google LLC".to_string();
    rec.in_if = 10;
    rec.out_if = 20;
    rec.set_sampling_rate(100);
    rec.set_direction(FlowDirection::Ingress);
    rec.src_country = "US".to_string();
    rec.dst_country = "DE".to_string();

    let flow_ref = store
        .get_or_insert_record_flow(120_000_000, &rec)
        .expect("intern tier flow");
    let fields = store
        .materialize_fields(flow_ref)
        .expect("materialize fields");
    assert_eq!(fields.get("PROTOCOL").map(String::as_str), Some("6"));
    assert_eq!(fields.get("ETYPE").map(String::as_str), Some("2048"));
    assert_eq!(fields.get("SRC_AS").map(String::as_str), Some("64512"));
    assert_eq!(fields.get("DST_AS").map(String::as_str), Some("15169"));
    assert_eq!(
        fields.get("SRC_AS_NAME").map(String::as_str),
        Some("AS64512 Example Transit")
    );
    assert_eq!(
        fields.get("DST_AS_NAME").map(String::as_str),
        Some("AS15169 Google LLC")
    );
    assert_eq!(fields.get("SRC_COUNTRY").map(String::as_str), Some("US"));
    assert_eq!(fields.get("DST_COUNTRY").map(String::as_str), Some("DE"));
    assert_eq!(fields.get("DIRECTION").map(String::as_str), Some("ingress"));
    assert!(fields.get("SAMPLING_RATE").is_none());
}

#[test]
fn indexed_field_lookup_matches_rollup_materialization_semantics() {
    let mut store = TierFlowIndexStore::default();
    let mut rec = FlowRecord::default();
    rec.set_direction(FlowDirection::Ingress);
    rec.protocol = 6;
    rec.src_as_name = "AS0 Private IP Address Space".to_string();

    let flow_ref = store
        .get_or_insert_record_flow(120_000_000, &rec)
        .expect("intern tier flow");

    assert_eq!(
        store.field_value_string(flow_ref, "DIRECTION").as_deref(),
        Some("ingress")
    );
    assert_eq!(
        store.field_value_string(flow_ref, "PROTOCOL").as_deref(),
        Some("6")
    );
    assert_eq!(
        store.field_value_string(flow_ref, "SRC_AS_NAME").as_deref(),
        Some("AS0 Private IP Address Space")
    );
    assert_eq!(
        store.field_value_string(flow_ref, "EXPORTER_IP").as_deref(),
        Some("")
    );
    assert_eq!(
        store.field_value_string(flow_ref, "NEXT_HOP").as_deref(),
        Some("")
    );
}

#[test]
fn same_dimensions_aggregate() {
    let mut acc = TierAccumulator::new(TierKind::Minute1);
    let mut store = TierFlowIndexStore::default();
    let ts = 120_000_000;

    let mut rec = FlowRecord::default();
    rec.protocol = 6;
    rec.bytes = 100;
    rec.packets = 2;

    let flow_ref = store
        .get_or_insert_record_flow(ts, &rec)
        .expect("intern tier flow");
    acc.observe_flow(ts, flow_ref, FlowMetrics::from_record(&rec));

    rec.bytes = 200;
    rec.packets = 3;
    let same_flow_ref = store
        .get_or_insert_record_flow(ts, &rec)
        .expect("reuse tier flow");
    assert_eq!(flow_ref, same_flow_ref);
    acc.observe_flow(ts, same_flow_ref, FlowMetrics::from_record(&rec));

    let rows = acc.flush_closed_rows(180_000_000);
    assert_eq!(rows.len(), 1);
    let fields = materialize_row_fields(&store, &rows[0]);
    assert_eq!(fields.get("BYTES").map(String::as_str), Some("300"));
    assert_eq!(fields.get("PACKETS").map(String::as_str), Some("5"));
}

#[test]
fn different_dimensions_separate() {
    let mut acc = TierAccumulator::new(TierKind::Minute1);
    let mut store = TierFlowIndexStore::default();
    let ts = 120_000_000;

    let mut rec1 = FlowRecord::default();
    rec1.protocol = 6;
    rec1.bytes = 100;
    rec1.packets = 2;

    let mut rec2 = FlowRecord::default();
    rec2.protocol = 17;
    rec2.bytes = 200;
    rec2.packets = 3;

    let flow_ref_1 = store
        .get_or_insert_record_flow(ts, &rec1)
        .expect("intern first flow");
    let flow_ref_2 = store
        .get_or_insert_record_flow(ts, &rec2)
        .expect("intern second flow");
    acc.observe_flow(ts, flow_ref_1, FlowMetrics::from_record(&rec1));
    acc.observe_flow(ts, flow_ref_2, FlowMetrics::from_record(&rec2));

    let rows = acc.flush_closed_rows(180_000_000);
    assert_eq!(rows.len(), 2);
}
