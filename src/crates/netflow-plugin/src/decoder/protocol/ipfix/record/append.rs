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
        account_ipfix_flowset(&flowset.body, stats);
        match flowset.body {
            IPFixFlowSetBody::Data(data) => {
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

                    let reverse_present = state.reverse_present;
                    let projection = finalize_ipfix_record(
                        rec,
                        state,
                        source,
                        version,
                        observation_domain_id,
                        sampling,
                        export_usec,
                        timestamp_source,
                        input_realtime_usec,
                    );
                    let (forward, reverse, partial_counter_record) = match projection {
                        Ok(projected) => projected,
                        Err(IPFixRecordRejection::SamplingOption) => {
                            stats.sampling_option_records += 1;
                            continue;
                        }
                        Err(IPFixRecordRejection::DecapsulationFailed) => {
                            stats.decapsulation_failed_records += 1;
                            continue;
                        }
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
            _ => {}
        }
    }
}
