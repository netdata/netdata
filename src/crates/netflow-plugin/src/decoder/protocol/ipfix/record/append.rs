use super::*;

pub(crate) fn append_ipfix_records(
    source: SocketAddr,
    out: &mut Vec<DecodedFlow>,
    packet: IPFix,
    sampling: &mut SamplingState,
    decapsulation_mode: DecapsulationMode,
    timestamp_source: TimestampSource,
    input_realtime_usec: u64,
) {
    let export_usec = unix_timestamp_to_usec(packet.header.export_time as u64, 0);
    let exporter_ip = canonicalize_ip_addr(source.ip());
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

                    let Some((forward, reverse)) = finalize_ipfix_record(
                        rec,
                        state,
                        exporter_ip,
                        version,
                        observation_domain_id,
                        sampling,
                        export_usec,
                        timestamp_source,
                        input_realtime_usec,
                    ) else {
                        continue;
                    };

                    out.push(forward);
                    if let Some(reverse) = reverse {
                        out.push(reverse);
                    }
                }
            }
            IPFixFlowSetBody::OptionsData(options_data) => {
                observe_ipfix_sampling_options(
                    exporter_ip,
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
