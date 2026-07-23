use super::*;

pub(crate) fn is_sflow_payload(payload: &[u8]) -> bool {
    if payload.len() < 4 {
        return false;
    }
    u32::from_be_bytes([payload[0], payload[1], payload[2], payload[3]]) == 5
}

pub(crate) fn decode_sflow(
    source: SocketAddr,
    payload: &[u8],
    enabled: bool,
    decapsulation_mode: DecapsulationMode,
    timestamp_source: TimestampSource,
    input_realtime_usec: u64,
) -> DecodedBatch {
    let mut batch = DecodedBatch {
        stats: DecodeStats {
            parse_attempts: 1,
            ..Default::default()
        },
        ..Default::default()
    };

    match parse_datagram(payload) {
        Ok(datagram) => {
            batch.stats.parsed_packets = 1;
            batch.stats.sflow_datagrams = 1;
            if !enabled {
                batch.stats.disabled_protocol_packets = 1;
            }
            batch.flows = extract_sflow_flows(
                source,
                datagram,
                decapsulation_mode,
                timestamp_source,
                input_realtime_usec,
                enabled,
                &mut batch.stats,
            );
        }
        Err(_err) => {
            batch.stats.parse_errors = 1;
        }
    }

    batch
}

fn account_disabled_v9_packet(packet: &V9, stats: &mut DecodeStats) {
    for flowset in &packet.flowsets {
        match &flowset.body {
            V9FlowSetBody::Template(templates) => {
                stats.v9_template_sets += 1;
                stats.v9_data_templates += templates.templates.len() as u64;
            }
            V9FlowSetBody::OptionsTemplate(templates) => {
                stats.v9_options_template_sets += 1;
                stats.v9_options_templates += templates.templates.len() as u64;
            }
            V9FlowSetBody::Data(data) => {
                stats.v9_data_sets += 1;
                stats.netflow_v9_records += data.fields.len() as u64;
            }
            V9FlowSetBody::OptionsData(data) => {
                stats.v9_options_data_sets += 1;
                stats.v9_options_records += data.fields.len() as u64;
            }
            V9FlowSetBody::NoTemplate(_) => {
                stats.v9_missing_template_sets += 1;
                stats.missing_template_sets += 1;
            }
            V9FlowSetBody::Empty => stats.v9_ignored_sets += 1,
        }
    }
}

fn account_disabled_ipfix_packet(packet: &IPFix, stats: &mut DecodeStats) {
    for flowset in &packet.flowsets {
        match &flowset.body {
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
            IPFixFlowSetBody::Data(data) => {
                stats.ipfix_data_sets += 1;
                stats.ipfix_records += data.fields.len() as u64;
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
            IPFixFlowSetBody::NoTemplate(_) => {
                stats.ipfix_missing_template_sets += 1;
                stats.missing_template_sets += 1;
            }
            IPFixFlowSetBody::Empty => stats.ipfix_ignored_sets += 1,
        }
    }
}

pub(crate) fn decode_netflow_result(
    result: ParseResult,
    sampling: &mut SamplingState,
    v9_nsel_flowsets_by_packet: &[Option<Vec<bool>>],
    source: SocketAddr,
    decapsulation_mode: DecapsulationMode,
    timestamp_source: TimestampSource,
    input_realtime_usec: u64,
    enable_v5: bool,
    enable_v7: bool,
    enable_v9: bool,
    enable_ipfix: bool,
    parse_attempts: u64,
) -> DecodedBatch {
    let mut batch = DecodedBatch {
        stats: DecodeStats {
            parse_attempts,
            ..Default::default()
        },
        ..Default::default()
    };

    batch.stats.parsed_packets = result.packets.len() as u64;
    for (packet_index, packet) in result.packets.into_iter().enumerate() {
        match packet {
            NetflowPacket::V5(v5) => {
                batch.stats.netflow_v5_packets += 1;
                batch.stats.netflow_v5_records += v5.flowsets.len() as u64;
                if enable_v5 {
                    append_v5_records(
                        source,
                        &mut batch.flows,
                        v5,
                        timestamp_source,
                        input_realtime_usec,
                    );
                } else {
                    batch.stats.disabled_protocol_packets += 1;
                }
            }
            NetflowPacket::V7(v7) => {
                batch.stats.netflow_v7_packets += 1;
                batch.stats.netflow_v7_records += v7.flowsets.len() as u64;
                if enable_v7 {
                    append_v7_records(
                        source,
                        &mut batch.flows,
                        v7,
                        timestamp_source,
                        input_realtime_usec,
                    );
                } else {
                    batch.stats.disabled_protocol_packets += 1;
                }
            }
            NetflowPacket::V9(v9) => {
                batch.stats.netflow_v9_packets += 1;
                if enable_v9 {
                    append_v9_records(
                        source,
                        &mut batch.flows,
                        v9,
                        sampling,
                        v9_nsel_flowsets_by_packet
                            .get(packet_index)
                            .and_then(Option::as_deref),
                        decapsulation_mode,
                        timestamp_source,
                        input_realtime_usec,
                        &mut batch.stats,
                    );
                } else {
                    batch.stats.disabled_protocol_packets += 1;
                    account_disabled_v9_packet(&v9, &mut batch.stats);
                }
            }
            NetflowPacket::IPFix(ipfix) => {
                batch.stats.ipfix_packets += 1;
                if enable_ipfix {
                    append_ipfix_records(
                        source,
                        &mut batch,
                        ipfix,
                        sampling,
                        decapsulation_mode,
                        timestamp_source,
                        input_realtime_usec,
                    );
                } else {
                    batch.stats.disabled_protocol_packets += 1;
                    account_disabled_ipfix_packet(&ipfix, &mut batch.stats);
                }
            }
            _ => {}
        }
    }

    if let Some(err) = result.error {
        if is_template_error(&err.to_string()) {
            // Most missing templates are represented by an exact NoTemplate
            // Set above. A parser-level template error has no Set object, so
            // account for one unresolved Set only when none was returned.
            if batch.stats.missing_template_sets == 0 {
                batch.stats.missing_template_sets = 1;
            }
        } else {
            batch.stats.parse_errors = 1;
        }
    }

    batch
}
