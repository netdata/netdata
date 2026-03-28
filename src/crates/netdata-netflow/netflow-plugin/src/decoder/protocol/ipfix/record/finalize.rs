use super::state::IPFixRecordBuildState;
use super::*;

fn reverse_metric_value(reverse_overrides: &FlowFields, key: &'static str) -> u64 {
    reverse_overrides
        .get(key)
        .and_then(|value| value.parse::<u64>().ok())
        .unwrap_or(0)
}

fn reset_reverse_metrics(reverse: &mut FlowRecord) {
    reverse.bytes = 0;
    reverse.packets = 0;
    reverse.raw_bytes = 0;
    reverse.raw_packets = 0;
}

fn build_reverse_ipfix_flow(
    forward: &FlowRecord,
    reverse_overrides: &FlowFields,
    source_ts: Option<u64>,
) -> Option<DecodedFlow> {
    let reverse_packets = reverse_metric_value(reverse_overrides, "PACKETS");
    let reverse_bytes = reverse_metric_value(reverse_overrides, "BYTES");
    if reverse_packets == 0 && reverse_bytes == 0 {
        return None;
    }

    let mut reverse = forward.clone();
    swap_directional_record_fields(&mut reverse);
    reset_reverse_metrics(&mut reverse);
    for (&key, value) in reverse_overrides {
        override_record_field(&mut reverse, key, value);
    }
    sync_raw_metrics_record(&mut reverse);
    finalize_record(&mut reverse);
    Some(DecodedFlow {
        record: reverse,
        source_realtime_usec: source_ts,
    })
}

pub(crate) fn finalize_ipfix_record(
    mut rec: FlowRecord,
    mut state: IPFixRecordBuildState,
    exporter_ip: &str,
    version: u16,
    observation_domain_id: u32,
    sampling: &mut SamplingState,
    need_decap: bool,
    export_usec: u64,
    timestamp_source: TimestampSource,
    input_realtime_usec: u64,
) -> Option<(DecodedFlow, Option<DecodedFlow>)> {
    state.apply_sampling_packet_ratio();
    apply_sampling_state_record(
        &mut rec,
        exporter_ip,
        version,
        observation_domain_id,
        state.sampler_id,
        state.observed_sampling_rate,
        sampling,
    );

    if looks_like_sampling_option_record_from_rec(&rec, state.observed_sampling_rate) {
        return None;
    }
    if need_decap && !state.decap_ok {
        return None;
    }

    if rec.flows == 0 {
        rec.flows = 1;
    }
    state.resolve_flow_times(&mut rec, export_usec);
    state.apply_reverse_time_overrides();
    finalize_record(&mut rec);

    let first_switched_usec = (rec.flow_start_usec != 0).then_some(rec.flow_start_usec);
    let source_ts =
        timestamp_source.select(input_realtime_usec, Some(export_usec), first_switched_usec);
    let reverse = state
        .reverse_present
        .then(|| build_reverse_ipfix_flow(&rec, &state.reverse_overrides, source_ts))
        .flatten();

    Some((
        DecodedFlow {
            record: rec,
            source_realtime_usec: source_ts,
        },
        reverse,
    ))
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::net::{IpAddr, Ipv4Addr};

    fn forward_record() -> FlowRecord {
        FlowRecord {
            bytes: 111,
            packets: 7,
            raw_bytes: 111,
            raw_packets: 7,
            src_addr: Some(IpAddr::V4(Ipv4Addr::new(10, 0, 0, 1))),
            dst_addr: Some(IpAddr::V4(Ipv4Addr::new(10, 0, 0, 2))),
            src_port: 1234,
            dst_port: 4321,
            ..FlowRecord::default()
        }
    }

    #[test]
    fn reverse_ipfix_flow_keeps_bytes_only_reverse_records() {
        let forward = forward_record();
        let overrides = FlowFields::from([
            ("BYTES", "42".to_string()),
            ("FLOW_END_USEC", "5".to_string()),
        ]);

        let reverse =
            build_reverse_ipfix_flow(&forward, &overrides, Some(77)).expect("reverse flow");

        assert_eq!(reverse.record.bytes, 42);
        assert_eq!(reverse.record.raw_bytes, 42);
        assert_eq!(reverse.record.packets, 0);
        assert_eq!(reverse.record.raw_packets, 0);
        assert_eq!(reverse.record.src_port, 4321);
        assert_eq!(reverse.record.dst_port, 1234);
        assert_eq!(reverse.source_realtime_usec, Some(77));
    }

    #[test]
    fn reverse_ipfix_flow_does_not_leak_forward_bytes_into_packets_only_reverse_records() {
        let forward = forward_record();
        let overrides = FlowFields::from([("PACKETS", "3".to_string())]);

        let reverse = build_reverse_ipfix_flow(&forward, &overrides, None).expect("reverse flow");

        assert_eq!(reverse.record.bytes, 0);
        assert_eq!(reverse.record.raw_bytes, 0);
        assert_eq!(reverse.record.packets, 3);
        assert_eq!(reverse.record.raw_packets, 3);
    }

    #[test]
    fn reverse_ipfix_flow_skips_zero_metric_reverse_records() {
        let forward = forward_record();
        let overrides =
            FlowFields::from([("PACKETS", "0".to_string()), ("BYTES", "0".to_string())]);

        assert!(build_reverse_ipfix_flow(&forward, &overrides, None).is_none());
    }
}
