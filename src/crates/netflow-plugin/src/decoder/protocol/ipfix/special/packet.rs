use super::*;

pub(crate) fn decode_ipfix_special_from_raw_payload(
    source: SocketAddr,
    payload: &[u8],
    sampling: &SamplingState,
    decapsulation_mode: DecapsulationMode,
    timestamp_source: TimestampSource,
    input_realtime_usec: u64,
) -> Vec<DecodedFlow> {
    if payload.len() < 16 {
        return Vec::new();
    }
    if u16::from_be_bytes([payload[0], payload[1]]) != 10 {
        return Vec::new();
    }

    let export_time = u32::from_be_bytes([payload[4], payload[5], payload[6], payload[7]]) as u64;
    let packet_realtime_usec = Some(unix_timestamp_to_usec(export_time, 0));
    let exporter_ip = canonicalize_ip_addr(source.ip());
    let observation_domain_id =
        u32::from_be_bytes([payload[12], payload[13], payload[14], payload[15]]);
    let packet_length = u16::from_be_bytes([payload[2], payload[3]]) as usize;
    let end_limit = payload.len().min(packet_length);
    let mut offset = 16_usize;
    let mut out = Vec::new();

    while offset.saturating_add(4) <= end_limit {
        let flowset_id = u16::from_be_bytes([payload[offset], payload[offset + 1]]);
        let flowset_len = u16::from_be_bytes([payload[offset + 2], payload[offset + 3]]) as usize;
        if flowset_len < 4 {
            return out;
        }
        let end = offset.saturating_add(flowset_len);
        if end > end_limit {
            return out;
        }
        let body = &payload[offset + 4..end];

        if flowset_id >= 256
            && let Some(template) =
                sampling.get_ipfix_datalink_template(exporter_ip, observation_domain_id, flowset_id)
        {
            let mut cursor = body;
            while !cursor.is_empty() {
                let Some((record_values, consumed)) =
                    parse_ipfix_record_from_template(cursor, &template.fields)
                else {
                    break;
                };
                if consumed == 0 {
                    break;
                }
                if let Some(flow) = decode_ipfix_special_record(
                    source,
                    timestamp_source,
                    input_realtime_usec,
                    packet_realtime_usec,
                    sampling,
                    exporter_ip,
                    observation_domain_id,
                    &template,
                    &record_values,
                    decapsulation_mode,
                ) {
                    out.push(flow);
                }
                cursor = &cursor[consumed..];
            }
        }

        offset = end;
    }

    out
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::net::{IpAddr, Ipv4Addr};

    #[test]
    fn zero_length_ipfix_special_records_do_not_loop_forever() {
        let mut sampling = SamplingState::default();
        sampling.set_ipfix_datalink_template(
            "192.0.2.10".parse().unwrap(),
            7,
            256,
            vec![IPFixTemplateField {
                field_type: IPFIX_FIELD_DATALINK_FRAME_SECTION,
                field_length: 0,
                enterprise_number: None,
            }],
        );

        let payload = [
            0x00, 0x0a, // version 10
            0x00, 0x15, // packet length 21
            0x00, 0x00, 0x00, 0x01, // export time
            0x00, 0x00, 0x00, 0x01, // sequence number
            0x00, 0x00, 0x00, 0x07, // observation domain id
            0x01, 0x00, // flowset id 256
            0x00, 0x05, // flowset length 5 => 1 byte body
            0x00, // body byte to enter the loop even though record consumes 0
        ];

        let out = decode_ipfix_special_from_raw_payload(
            SocketAddr::new(IpAddr::V4(Ipv4Addr::new(192, 0, 2, 10)), 9995),
            &payload,
            &sampling,
            DecapsulationMode::None,
            TimestampSource::Input,
            1_700_000_000_000_000,
        );

        assert!(out.is_empty());
    }
}
