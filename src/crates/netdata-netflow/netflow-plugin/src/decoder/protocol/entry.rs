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

pub(crate) fn decode_netflow(
    parser: &mut AutoScopedParser,
    sampling: &mut SamplingState,
    source: SocketAddr,
    payload: &[u8],
    decapsulation_mode: DecapsulationMode,
    timestamp_source: TimestampSource,
    input_realtime_usec: u64,
    enable_v5: bool,
    enable_v7: bool,
    enable_v9: bool,
    enable_ipfix: bool,
) -> DecodedBatch {
    let mut batch = DecodedBatch {
        stats: DecodeStats {
            parse_attempts: 1,
            ..Default::default()
        },
        ..Default::default()
    };

    // Skip special datalink-frame decode paths when no datalink templates are registered.
    // These functions parse the raw payload looking for template-matched records — pointless
    // when no templates exist, and they are a significant fraction of per-packet CPU cost.
    let raw_v9_flows = if enable_v9 && sampling.has_any_v9_datalink_templates() {
        decode_v9_special_from_raw_payload(
            source,
            payload,
            sampling,
            decapsulation_mode,
            timestamp_source,
            input_realtime_usec,
        )
    } else {
        Vec::new()
    };

    let raw_ipfix_flows = if enable_ipfix && sampling.has_any_ipfix_datalink_templates() {
        decode_ipfix_special_from_raw_payload(
            source,
            payload,
            sampling,
            decapsulation_mode,
            timestamp_source,
            input_realtime_usec,
        )
    } else {
        Vec::new()
    };

    match parser.parse_from_source(source, payload) {
        Ok(packets) => {
            batch.stats.parsed_packets = packets.len() as u64;
            for packet in packets {
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
                                decapsulation_mode,
                                timestamp_source,
                                input_realtime_usec,
                            );
                        }
                    }
                    NetflowPacket::IPFix(ipfix) => {
                        if enable_ipfix {
                            batch.stats.ipfix_packets += 1;
                            append_ipfix_records(
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
                }
            }
        }
        Err(err) => {
            if is_template_error(&err.to_string()) {
                batch.stats.template_errors = 1;
            } else {
                batch.stats.parse_errors = 1;
            }
        }
    }

    let mut raw_flows = raw_v9_flows;
    raw_flows.extend(raw_ipfix_flows);
    append_unique_flows(&mut batch.flows, raw_flows);

    batch
}
