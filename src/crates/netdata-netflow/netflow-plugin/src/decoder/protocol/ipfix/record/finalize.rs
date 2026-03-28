use super::state::IPFixRecordBuildState;
use super::*;

fn build_reverse_ipfix_flow(
    forward: &FlowRecord,
    reverse_overrides: &FlowFields,
    source_ts: Option<u64>,
) -> Option<DecodedFlow> {
    let reverse_packets = reverse_overrides
        .get("PACKETS")
        .and_then(|value| value.parse::<u64>().ok())
        .unwrap_or(0);
    if reverse_packets == 0 {
        return None;
    }

    let mut reverse = forward.clone();
    swap_directional_record_fields(&mut reverse);
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
