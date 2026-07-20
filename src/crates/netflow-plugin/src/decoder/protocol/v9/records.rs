use super::super::*;

pub(crate) fn append_v9_records(
    source: SocketAddr,
    out: &mut Vec<DecodedFlow>,
    packet: V9,
    sampling: &mut SamplingState,
    nsel_flowsets: Option<&[bool]>,
    decapsulation_mode: DecapsulationMode,
    timestamp_source: TimestampSource,
    input_realtime_usec: u64,
    stats: &mut DecodeStats,
) {
    let export_usec = unix_timestamp_to_usec(packet.header.unix_secs as u64, 0);
    let observation_domain_id = packet.header.source_id;
    let version = 9_u16;
    let system_init_usec = netflow_v9_system_init_usec(
        packet.header.unix_secs as u64,
        packet.header.sys_up_time as u64,
    );

    for (flowset_index, flowset) in packet.flowsets.into_iter().enumerate() {
        let nsel = nsel_flowsets
            .and_then(|flowsets| flowsets.get(flowset_index))
            .copied()
            .unwrap_or(false);
        match flowset.body {
            V9FlowSetBody::Data(data) => {
                for record in data.fields {
                    let mut rec = base_record("v9", source);
                    let mut sampler_id: Option<u64> = None;
                    let mut observed_sampling_rate: Option<u64> = None;
                    let mut flow_start_usec: Option<u64> = None;
                    let mut flow_end_usec: Option<u64> = None;
                    let mut decap_required = false;
                    let mut decap_ok = false;
                    let mut counters = OrdinaryCounterSelector::default();
                    let mut nsel_state = NselRecordState::default();

                    for (field, value) in record {
                        if nsel && nsel_state.observe(field, &value) {
                            continue;
                        }
                        if field == V9Field::Layer2packetSectionData {
                            decap_required = true;
                            if let FieldValue::Vec(raw_value) = &value
                                && let Some(l3_len) = parse_datalink_frame_section_record(
                                    raw_value,
                                    &mut rec,
                                    decapsulation_mode,
                                )
                            {
                                counters.observe_sampled_frame(l3_len);
                                decap_ok = true;
                            }
                            continue;
                        }

                        if counters.observe_v9(field, &value) {
                            continue;
                        }

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
                                flow_start_usec = value_str.parse::<u64>().ok().map(|value| {
                                    netflow_v9_uptime_millis_to_absolute_usec(
                                        system_init_usec,
                                        value,
                                    )
                                });
                            }
                            V9Field::LastSwitched => {
                                flow_end_usec = value_str.parse::<u64>().ok().map(|value| {
                                    netflow_v9_uptime_millis_to_absolute_usec(
                                        system_init_usec,
                                        value,
                                    )
                                });
                            }
                            V9Field::FlowStartMilliseconds => {
                                flow_start_usec = value_str
                                    .parse::<u64>()
                                    .ok()
                                    .map(|value| value.saturating_mul(USEC_PER_MILLISECOND));
                            }
                            V9Field::FlowEndMilliseconds => {
                                flow_end_usec = value_str
                                    .parse::<u64>()
                                    .ok()
                                    .map(|value| value.saturating_mul(USEC_PER_MILLISECOND));
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

                    rec.flow_start_usec = flow_start_usec.unwrap_or(0);
                    rec.flow_end_usec = flow_end_usec.unwrap_or(0);
                    if nsel {
                        let projection = project_nsel_record(rec, nsel_state, input_realtime_usec);
                        stats.partial_counter_records = stats
                            .partial_counter_records
                            .saturating_add(projection.stats.partial_counter_records);
                        projection.stats.merge_into(stats);
                        if let Some(flow) = projection.forward {
                            out.push(flow);
                        }
                        if let Some(flow) = projection.reverse {
                            out.push(flow);
                        }
                        continue;
                    }

                    apply_sampling_state_record(
                        &mut rec,
                        source,
                        version,
                        observation_domain_id,
                        sampler_id,
                        observed_sampling_rate,
                        sampling,
                    );

                    if looks_like_sampling_option_record_from_rec(&rec, observed_sampling_rate) {
                        continue;
                    }
                    if decap_required && !decap_ok {
                        continue;
                    }

                    if counters.apply_to_record(&mut rec) {
                        stats.partial_counter_records += 1;
                    }

                    if rec.flows == 0 {
                        rec.flows = 1;
                    }
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
            _ => continue,
        }
    }
}
