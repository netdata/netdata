use super::*;
use crate::flow::{FlowDirection, FlowFields, FlowRecord};
use std::collections::{BTreeMap, BTreeSet};
use std::mem::size_of;

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
fn tier_flow_index_store_cardinality_counts_hours_and_flows() {
    let mut store = TierFlowIndexStore::default();

    let mut tcp = FlowRecord::default();
    tcp.protocol = 6;
    tcp.src_port = 12345;
    tcp.dst_port = 443;

    let mut udp = FlowRecord::default();
    udp.protocol = 17;
    udp.src_port = 5353;
    udp.dst_port = 5353;

    store
        .get_or_insert_record_flow(120_000_000, &tcp)
        .expect("intern tcp flow");
    store
        .get_or_insert_record_flow(120_000_001, &udp)
        .expect("intern udp flow");
    store
        .get_or_insert_record_flow(3_720_000_000, &tcp)
        .expect("intern next-hour tcp flow");

    let cardinality = store.cardinality();
    assert_eq!(cardinality.hours, 2);
    assert_eq!(cardinality.flows, 3);
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

/// Feed the same observations into two fresh accumulators so the legacy
/// row-expanding flush and the container-moving take can be compared.
fn two_identical_accumulators(
    store: &mut TierFlowIndexStore,
    observations: &[(u64, u8, u64)],
) -> (TierAccumulator, TierAccumulator) {
    let mut left = TierAccumulator::new(TierKind::Minute1);
    let mut right = TierAccumulator::new(TierKind::Minute1);
    for &(ts, protocol, bytes) in observations {
        let mut rec = FlowRecord::default();
        rec.protocol = protocol;
        rec.bytes = bytes;
        rec.packets = 1;
        let flow_ref = store
            .get_or_insert_record_flow(ts, &rec)
            .expect("intern flow");
        let metrics = FlowMetrics::from_record(&rec);
        left.observe_flow(ts, flow_ref, metrics);
        right.observe_flow(ts, flow_ref, metrics);
    }
    (left, right)
}

#[test]
fn take_closed_buckets_matches_flush_closed_rows() {
    let mut store = TierFlowIndexStore::default();
    let minute = 60_000_000_u64;
    // Two closed buckets (starts 60s and 120s) and one open (180s) at now=240s-1.
    let observations = [
        (minute + 1, 6, 100),
        (minute + 2, 17, 200),
        (2 * minute + 1, 6, 300),
        (3 * minute + 1, 6, 400),
    ];
    let (mut legacy, mut taken_side) = two_identical_accumulators(&mut store, &observations);
    let now = 4 * minute - 1;

    let row_key = |ts: u64, flow_ref: TierFlowRef, m: FlowMetrics| {
        (
            ts,
            flow_ref.hour_start_usec,
            flow_ref.flow_id,
            m.bytes,
            m.packets,
        )
    };
    let mut legacy_rows: Vec<_> = legacy
        .flush_closed_rows(now)
        .into_iter()
        .map(|row| row_key(row.timestamp_usec, row.flow_ref, row.metrics))
        .collect();
    legacy_rows.sort();

    let bucket_usec = taken_side.bucket_usec();
    let taken = taken_side.take_closed_buckets(now);
    assert_eq!(
        taken.iter().map(|(start, _)| *start).collect::<Vec<_>>(),
        vec![minute, 2 * minute],
        "take must return exactly the closed buckets, in order"
    );
    let mut taken_rows: Vec<_> = taken
        .iter()
        .flat_map(|(start, bucket)| {
            let row_ts = start + bucket_usec - 1;
            bucket
                .iter()
                .map(move |(&flow_ref, &m)| row_key(row_ts, flow_ref, m))
        })
        .collect();
    taken_rows.sort();

    assert_eq!(legacy_rows, taken_rows);

    // The open bucket must remain in both.
    assert_eq!(legacy.snapshot_open_rows(now).len(), 1);
    assert_eq!(taken_side.snapshot_open_rows(now).len(), 1);
}

#[test]
fn take_closed_buckets_closes_exactly_at_bucket_end() {
    let mut store = TierFlowIndexStore::default();
    let minute = 60_000_000_u64;
    let (_, mut acc) =
        two_identical_accumulators(&mut store, &[(minute + 1, 6, 1), (2 * minute + 1, 6, 1)]);

    // now == end of the first bucket: first closes, second stays open.
    let taken = acc.take_closed_buckets(2 * minute);
    assert_eq!(
        taken.iter().map(|(s, _)| *s).collect::<Vec<_>>(),
        vec![minute]
    );
    assert_eq!(acc.snapshot_open_rows(2 * minute).len(), 1);
}

#[test]
fn snapshot_open_rows_into_clears_destination_and_reuses_capacity() {
    let mut store = TierFlowIndexStore::default();
    let mut acc = TierAccumulator::new(TierKind::Minute1);
    let minute = 60_000_000_u64;
    let now = minute + 10_000_000;

    let mut tcp = FlowRecord::default();
    tcp.protocol = 6;
    tcp.bytes = 100;
    let tcp_ref = store
        .get_or_insert_record_flow(minute + 1, &tcp)
        .expect("intern tcp flow");
    acc.observe_flow(minute + 1, tcp_ref, FlowMetrics::from_record(&tcp));

    let mut udp = FlowRecord::default();
    udp.protocol = 17;
    udp.bytes = 200;
    let udp_ref = store
        .get_or_insert_record_flow(minute + 2, &udp)
        .expect("intern udp flow");
    acc.observe_flow(minute + 2, udp_ref, FlowMetrics::from_record(&udp));

    let mut rows = Vec::with_capacity(8);
    rows.push(OpenTierRow {
        timestamp_usec: 1,
        flow_ref: tcp_ref,
        metrics: FlowMetrics {
            bytes: 999,
            packets: 999,
        },
    });
    let capacity = rows.capacity();

    acc.snapshot_open_rows_into(now, &mut rows);

    assert_eq!(rows.capacity(), capacity);
    assert_eq!(rows.len(), 2);
    assert!(
        rows.iter().all(|row| row.timestamp_usec == now),
        "stale destination rows must be cleared before open rows are appended"
    );
    assert_eq!(rows.iter().map(|row| row.metrics.bytes).sum::<u64>(), 300);
}

#[test]
fn snapshot_open_rows_into_handles_empty_closed_and_zero_capacity_destinations() {
    let mut store = TierFlowIndexStore::default();
    let mut acc = TierAccumulator::new(TierKind::Minute1);
    let minute = 60_000_000_u64;
    let mut rows = Vec::new();

    acc.snapshot_open_rows_into(minute + 1, &mut rows);
    assert!(rows.is_empty());
    assert_eq!(rows.capacity(), 0);

    let mut closed = FlowRecord::default();
    closed.protocol = 6;
    closed.bytes = 100;
    let closed_ref = store
        .get_or_insert_record_flow(minute + 1, &closed)
        .expect("intern closed flow");
    acc.observe_flow(minute + 1, closed_ref, FlowMetrics::from_record(&closed));

    acc.snapshot_open_rows_into(2 * minute, &mut rows);
    assert!(rows.is_empty(), "closed buckets must not be snapshotted");

    let mut open = FlowRecord::default();
    open.protocol = 17;
    open.bytes = 200;
    let open_ref = store
        .get_or_insert_record_flow(2 * minute + 1, &open)
        .expect("intern open flow");
    acc.observe_flow(2 * minute + 1, open_ref, FlowMetrics::from_record(&open));

    acc.snapshot_open_rows_into(2 * minute + 1, &mut rows);
    assert_eq!(rows.len(), 1);
    assert_eq!(rows[0].flow_ref, open_ref);
    assert!(rows.capacity() >= 1);
}

#[test]
fn extend_active_hours_preserves_existing_hours_and_adds_accumulator_hours() {
    let mut store = TierFlowIndexStore::default();
    let mut acc = TierAccumulator::new(TierKind::Minute1);
    let hour = 3_600_000_000_u64;
    let existing_in_flight_hour = 42 * hour;
    let first_hour = 100 * hour;
    let second_hour = 101 * hour;

    let mut hours = BTreeSet::from([existing_in_flight_hour]);
    acc.extend_active_hours(&mut hours);
    assert_eq!(hours, BTreeSet::from([existing_in_flight_hour]));

    for (timestamp, protocol) in [(first_hour + 1, 6_u8), (second_hour + 1, 17_u8)] {
        let mut rec = FlowRecord::default();
        rec.protocol = protocol;
        let flow_ref = store
            .get_or_insert_record_flow(timestamp, &rec)
            .expect("intern flow");
        acc.observe_flow(timestamp, flow_ref, FlowMetrics::from_record(&rec));
    }

    acc.extend_active_hours(&mut hours);

    assert_eq!(
        hours,
        BTreeSet::from([existing_in_flight_hour, first_hour, second_hour])
    );
}

#[test]
fn open_tier_state_clear_retain_capacity_keeps_row_buffers() {
    let mut state = OpenTierState {
        generation: 42,
        minute_1: Vec::with_capacity(8),
        minute_5: Vec::with_capacity(4),
        hour_1: Vec::with_capacity(2),
    };
    state.minute_1.push(OpenTierRow {
        timestamp_usec: 1,
        flow_ref: TierFlowRef {
            hour_start_usec: 0,
            flow_id: 1,
        },
        metrics: FlowMetrics {
            bytes: 1,
            packets: 1,
        },
    });

    let capacities = (
        state.minute_1.capacity(),
        state.minute_5.capacity(),
        state.hour_1.capacity(),
    );

    state.clear_retain_capacity();

    assert_eq!(state.generation, 0);
    assert!(state.minute_1.is_empty());
    assert!(state.minute_5.is_empty());
    assert!(state.hour_1.is_empty());
    assert_eq!(
        state.estimated_heap_bytes(),
        (capacities.0 + capacities.1 + capacities.2) * size_of::<OpenTierRow>(),
        "retained row buffers must remain exactly visible to memory diagnostics"
    );
    assert_eq!(
        (
            state.minute_1.capacity(),
            state.minute_5.capacity(),
            state.hour_1.capacity()
        ),
        capacities
    );
}

#[test]
fn recycled_container_capacity_is_reused_without_allocation() {
    let mut store = TierFlowIndexStore::default();
    let mut acc = TierAccumulator::new(TierKind::Minute1);
    let minute = 60_000_000_u64;

    // Fill one bucket with many unique flows to grow the container.
    for i in 0..512_u32 {
        let mut rec = FlowRecord::default();
        rec.protocol = 6;
        rec.src_as = 64_000 + i;
        rec.bytes = 1;
        rec.packets = 1;
        let flow_ref = store
            .get_or_insert_record_flow(minute + 1, &rec)
            .expect("intern flow");
        acc.observe_flow(minute + 1, flow_ref, FlowMetrics::from_record(&rec));
    }

    let taken = acc.take_closed_buckets(2 * minute);
    assert_eq!(taken.len(), 1);
    let (_, container) = taken.into_iter().next().expect("one bucket");
    let grown_capacity = container.capacity();
    assert!(grown_capacity >= 512);

    acc.recycle(container);
    assert_eq!(acc.free_pool_len(), 1);

    // The next opened bucket must consume the recycled container and start at
    // the previous high-water capacity.
    let mut rec = FlowRecord::default();
    rec.protocol = 17;
    rec.bytes = 1;
    rec.packets = 1;
    let flow_ref = store
        .get_or_insert_record_flow(2 * minute + 1, &rec)
        .expect("intern flow");
    acc.observe_flow(2 * minute + 1, flow_ref, FlowMetrics::from_record(&rec));
    assert_eq!(acc.free_pool_len(), 0, "new bucket must reuse the pool");
    assert!(
        acc.bucket_capacity(2 * minute).expect("open bucket") >= grown_capacity,
        "recycled capacity must be retained"
    );
}
