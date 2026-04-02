use super::super::*;

pub(crate) fn append_v9_records(
    source: SocketAddr,
    out: &mut Vec<DecodedFlow>,
    packet: V9,
    sampling: &mut SamplingState,
    _decapsulation_mode: DecapsulationMode,
    timestamp_source: TimestampSource,
    input_realtime_usec: u64,
) {
    let export_usec = unix_timestamp_to_usec(packet.header.unix_secs as u64, 0);
    let exporter_ip = source.ip().to_string();
    let observation_domain_id = packet.header.source_id;
    let version = 9_u16;
    let system_init_usec = netflow_v9_system_init_usec(
        packet.header.unix_secs as u64,
        packet.header.sys_up_time as u64,
    );

    for flowset in packet.flowsets {
        match flowset.body {
            V9FlowSetBody::Data(data) => {
                for record in data.fields {
                    let mut rec = base_record("v9", source);
                    let mut sampler_id: Option<u64> = None;
                    let mut observed_sampling_rate: Option<u64> = None;
                    let mut first_switched_millis: Option<u64> = None;
                    let mut last_switched_millis: Option<u64> = None;

                    for (field, value) in record {
                        let value_str = field_value_to_string(&value);
                        apply_v9_special_mappings_record(&mut rec, field, &value_str);
                        match field {
                            V9Field::FlowSamplerId => {
                                sampler_id = value_str.parse::<u64>().ok();
                            }
                            V9Field::SamplingInterval | V9Field::FlowSamplerRandomInterval => {
                                observed_sampling_rate = value_str.parse::<u64>().ok();
                            }
                            V9Field::FirstSwitched => {
                                first_switched_millis = value_str.parse::<u64>().ok();
                            }
                            V9Field::LastSwitched => {
                                last_switched_millis = value_str.parse::<u64>().ok();
                            }
                            _ => {}
                        }
                        if let Some(canonical) = v9_canonical_key(field) {
                            if should_skip_zero_ip(canonical, &value_str) {
                                continue;
                            }
                            // IpProtocolVersion is fully handled by special mappings
                            // (raw "6" -> etype 34525). Skip to avoid overwriting.
                            if matches!(field, V9Field::IpProtocolVersion) {
                                continue;
                            }
                            set_record_field(&mut rec, canonical, &value_str);
                        }
                    }

                    apply_sampling_state_record(
                        &mut rec,
                        &exporter_ip,
                        version,
                        observation_domain_id,
                        sampler_id,
                        observed_sampling_rate,
                        sampling,
                    );

                    if looks_like_sampling_option_record_from_rec(&rec, observed_sampling_rate) {
                        continue;
                    }

                    if rec.flows == 0 {
                        rec.flows = 1;
                    }
                    rec.flow_start_usec = first_switched_millis
                        .map(|value| {
                            netflow_v9_uptime_millis_to_absolute_usec(system_init_usec, value)
                        })
                        .unwrap_or(0);
                    rec.flow_end_usec = last_switched_millis
                        .map(|value| {
                            netflow_v9_uptime_millis_to_absolute_usec(system_init_usec, value)
                        })
                        .unwrap_or(0);
                    finalize_record(&mut rec);
                    let first_switched_usec =
                        (rec.flow_start_usec != 0).then_some(rec.flow_start_usec);
                    out.push(DecodedFlow {
                        record: rec,
                        source_realtime_usec: timestamp_source.select(
                            input_realtime_usec,
                            Some(export_usec),
                            first_switched_usec,
                        ),
                    });
                }
            }
            V9FlowSetBody::OptionsData(options_data) => {
                observe_v9_sampling_options(
                    &exporter_ip,
                    version,
                    observation_domain_id,
                    sampling,
                    options_data,
                );
            }
            _ => continue,
        }
    }
}
