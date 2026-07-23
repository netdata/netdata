use super::*;

pub(crate) fn append_ipfix_records(
    source: SocketAddr,
    batch: &mut DecodedBatch,
    packet: IPFix,
    sampling: &mut SamplingState,
    decapsulation_mode: DecapsulationMode,
    timestamp_source: TimestampSource,
    input_realtime_usec: u64,
) {
    let DecodedBatch { flows: out, stats } = batch;
    let export_usec = unix_timestamp_to_usec(packet.header.export_time as u64, 0);
    let observation_domain_id = packet.header.observation_domain_id;
    let version = 10_u16;

    for flowset in packet.flowsets {
        match flowset.body {
            IPFixFlowSetBody::Data(data) => {
                stats.ipfix_data_sets += 1;
                stats.ipfix_records += data.fields.len() as u64;
                for record in data.fields {
                    let mut rec = base_record("ipfix", source);
                    let mut state = IPFixRecordBuildState::default();

                    for (field, value) in record {
                        apply_ipfix_record_field(
                            &mut rec,
                            &mut state,
                            &field,
                            &value,
                            decapsulation_mode,
                            export_usec,
                        );
                    }

                    let sampling_option_record = looks_like_sampling_option_record_from_rec(
                        &rec,
                        state.observed_sampling_rate,
                    );
                    let decapsulation_failed = state.decap_required && !state.decap_ok;
                    let reverse_present = state.reverse_present;
                    let Some((forward, reverse, partial_counter_record)) = finalize_ipfix_record(
                        rec,
                        state,
                        source,
                        version,
                        observation_domain_id,
                        sampling,
                        export_usec,
                        timestamp_source,
                        input_realtime_usec,
                    ) else {
                        if sampling_option_record {
                            stats.sampling_option_records += 1;
                        } else if decapsulation_failed {
                            stats.decapsulation_failed_records += 1;
                        }
                        continue;
                    };

                    if partial_counter_record {
                        stats.partial_counter_records += 1;
                    }
                    if reverse_present && reverse.is_none() {
                        stats.ipfix_zero_reverse_records += 1;
                    }

                    out.push(forward);
                    if let Some(reverse) = reverse {
                        out.push(reverse);
                    }
                }
            }
            IPFixFlowSetBody::V9Data(data) => {
                stats.ipfix_data_sets += 1;
                stats.ipfix_records += data.fields.len() as u64;
                stats.unsupported_data_sets += 1;
            }
            IPFixFlowSetBody::OptionsData(data) => {
                stats.ipfix_options_data_sets += 1;
                stats.ipfix_options_records += data.fields.len() as u64;
            }
            IPFixFlowSetBody::V9OptionsData(data) => {
                stats.ipfix_options_data_sets += 1;
                stats.ipfix_options_records += data.fields.len() as u64;
            }
            IPFixFlowSetBody::Template(_) | IPFixFlowSetBody::V9Template(_) => {
                stats.ipfix_template_sets += 1;
                stats.ipfix_data_templates += 1;
            }
            IPFixFlowSetBody::Templates(templates) => {
                stats.ipfix_template_sets += 1;
                stats.ipfix_data_templates += templates.len() as u64;
            }
            IPFixFlowSetBody::V9Templates(templates) => {
                stats.ipfix_template_sets += 1;
                stats.ipfix_data_templates += templates.len() as u64;
            }
            IPFixFlowSetBody::OptionsTemplate(_) | IPFixFlowSetBody::V9OptionsTemplate(_) => {
                stats.ipfix_options_template_sets += 1;
                stats.ipfix_options_templates += 1;
            }
            IPFixFlowSetBody::OptionsTemplates(templates) => {
                stats.ipfix_options_template_sets += 1;
                stats.ipfix_options_templates += templates.len() as u64;
            }
            IPFixFlowSetBody::V9OptionsTemplates(templates) => {
                stats.ipfix_options_template_sets += 1;
                stats.ipfix_options_templates += templates.len() as u64;
            }
            IPFixFlowSetBody::NoTemplate(_) => {
                stats.ipfix_missing_template_sets += 1;
                stats.missing_template_sets += 1;
            }
            IPFixFlowSetBody::Empty => {
                stats.ipfix_ignored_sets += 1;
            }
        }
    }
}
