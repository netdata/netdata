use super::*;

pub(crate) fn append_ipfix_records(
    source: SocketAddr,
    out: &mut Vec<DecodedFlow>,
    packet: IPFix,
    sampling: &mut SamplingState,
    decapsulation_mode: DecapsulationMode,
    timestamp_source: TimestampSource,
    input_realtime_usec: u64,
) -> u64 {
    let mut partial_counter_records = 0_u64;
    let export_usec = unix_timestamp_to_usec(packet.header.export_time as u64, 0);
    let observation_domain_id = packet.header.observation_domain_id;
    let version = 10_u16;

    for flowset in packet.flowsets {
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
                        continue;
                    };

                    if partial_counter_record {
                        partial_counter_records += 1;
                    }

                    out.push(forward);
                    if let Some(reverse) = reverse {
                        out.push(reverse);
                    }
                }
            }
            _ => continue,
        }
    }

    partial_counter_records
}
