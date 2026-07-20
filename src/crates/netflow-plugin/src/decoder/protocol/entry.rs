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
            batch.flows = extract_sflow_flows(
                source,
                datagram,
                decapsulation_mode,
                timestamp_source,
                input_realtime_usec,
            );
        }
        Err(_err) => {
            batch.stats.parse_errors = 1;
        }
    }

    batch
}

pub(crate) fn missing_template_ids(packets: &[NetflowPacket]) -> HashSet<u16> {
    let mut ids = HashSet::new();
    for packet in packets {
        match packet {
            NetflowPacket::V9(packet) => {
                for flowset in &packet.flowsets {
                    if let V9FlowSetBody::NoTemplate(info) = &flowset.body {
                        ids.insert(info.template_id);
                    }
                }
            }
            NetflowPacket::IPFix(packet) => {
                for flowset in &packet.flowsets {
                    if let IPFixFlowSetBody::NoTemplate(info) = &flowset.body {
                        ids.insert(info.template_id);
                    }
                }
            }
            _ => {}
        }
    }
    ids
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

    if !missing_template_ids(&result.packets).is_empty() {
        batch.stats.template_errors = 1;
    }
    batch.stats.parsed_packets = result.packets.len() as u64;
    for (packet_index, packet) in result.packets.into_iter().enumerate() {
        match packet {
            NetflowPacket::V5(v5) => {
                if enable_v5 {
                    batch.stats.netflow_v5_packets += 1;
                    append_v5_records(
                        source,
                        &mut batch.flows,
                        v5,
                        timestamp_source,
                        input_realtime_usec,
                    );
                }
            }
            NetflowPacket::V7(v7) => {
                if enable_v7 {
                    batch.stats.netflow_v7_packets += 1;
                    append_v7_records(
                        source,
                        &mut batch.flows,
                        v7,
                        timestamp_source,
                        input_realtime_usec,
                    );
                }
            }
            NetflowPacket::V9(v9) => {
                if enable_v9 {
                    batch.stats.netflow_v9_packets += 1;
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
                }
            }
            NetflowPacket::IPFix(ipfix) => {
                if enable_ipfix {
                    batch.stats.ipfix_packets += 1;
                    batch.stats.partial_counter_records += append_ipfix_records(
                        source,
                        &mut batch.flows,
                        ipfix,
                        sampling,
                        decapsulation_mode,
                        timestamp_source,
                        input_realtime_usec,
                    );
                }
            }
            _ => {}
        }
    }

    if let Some(err) = result.error {
        if is_template_error(&err.to_string()) {
            batch.stats.template_errors = 1;
        } else {
            batch.stats.parse_errors = 1;
        }
    }

    batch
}
