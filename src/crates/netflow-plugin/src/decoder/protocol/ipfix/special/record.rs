use super::*;

pub(crate) fn decode_ipfix_special_record(
    source: SocketAddr,
    timestamp_source: TimestampSource,
    input_realtime_usec: u64,
    packet_realtime_usec: Option<u64>,
    sampling: &SamplingState,
    exporter_ip: IpAddr,
    observation_domain_id: u32,
    template: &IPFixDataLinkTemplate,
    values: &[&[u8]],
    decapsulation_mode: DecapsulationMode,
) -> Option<DecodedFlow> {
    let mut fields = base_fields("ipfix", source);
    let mut has_datalink_section = false;
    let mut has_decoded_datalink = false;
    let mut has_mpls_labels = false;
    let mut flow_start_usec: Option<u64> = None;
    let mut sampler_id: Option<u64> = None;
    let mut observed_sampling_rate: Option<u64> = None;
    let mut sampling_packet_interval: Option<u64> = None;
    let mut sampling_packet_space: Option<u64> = None;

    for (template_field, raw_value) in template.fields.iter().zip(values.iter()) {
        if let Some(pen) = template_field.enterprise_number {
            if pen == JUNIPER_PEN
                && template_field.field_type == JUNIPER_COMMON_PROPERTIES_ID
                && raw_value.len() == 2
                && ((raw_value[0] & 0xfc) >> 2) == 0x02
            {
                let Some(value) = decode_akvorado_unsigned(raw_value) else {
                    continue;
                };
                let status = if value & 0x03ff == 0 {
                    "64"
                } else {
                    "128"
                };
                fields.insert("FORWARDING_STATUS", status.to_string());
            }
            continue;
        }

        match template_field.field_type {
            IPFIX_FIELD_OCTET_DELTA_COUNT => {
                if let Some(value) = decode_akvorado_unsigned(raw_value) {
                    fields.insert("BYTES", value.to_string());
                }
            }
            IPFIX_FIELD_PACKET_DELTA_COUNT => {
                if let Some(value) = decode_akvorado_unsigned(raw_value) {
                    fields.insert("PACKETS", value.to_string());
                }
            }
            IPFIX_FIELD_PROTOCOL_IDENTIFIER => {
                if let Some(value) = decode_akvorado_unsigned(raw_value) {
                    fields.insert("PROTOCOL", value.to_string());
                }
            }
            IPFIX_FIELD_SAMPLER_ID | IPFIX_FIELD_SELECTOR_ID => {
                sampler_id = decode_akvorado_unsigned(raw_value);
            }
            IPFIX_FIELD_SAMPLING_INTERVAL | IPFIX_FIELD_SAMPLER_RANDOM_INTERVAL => {
                observed_sampling_rate = decode_akvorado_unsigned(raw_value);
            }
            IPFIX_FIELD_SAMPLING_PACKET_INTERVAL => {
                sampling_packet_interval = decode_akvorado_unsigned(raw_value);
            }
            IPFIX_FIELD_SAMPLING_PACKET_SPACE => {
                sampling_packet_space = decode_akvorado_unsigned(raw_value);
            }
            IPFIX_FIELD_SOURCE_TRANSPORT_PORT => {
                if let Some(value) = decode_akvorado_unsigned(raw_value) {
                    fields.insert("SRC_PORT", value.to_string());
                }
            }
            IPFIX_FIELD_DESTINATION_TRANSPORT_PORT => {
                if let Some(value) = decode_akvorado_unsigned(raw_value) {
                    fields.insert("DST_PORT", value.to_string());
                }
            }
            IPFIX_FIELD_SOURCE_IPV4_ADDRESS | IPFIX_FIELD_SOURCE_IPV6_ADDRESS => {
                if let Some(ip) = parse_ip_value(raw_value) {
                    if !is_zero_ip_value(&ip) {
                        fields.insert("SRC_ADDR", ip);
                    }
                }
            }
            IPFIX_FIELD_DESTINATION_IPV4_ADDRESS | IPFIX_FIELD_DESTINATION_IPV6_ADDRESS => {
                if let Some(ip) = parse_ip_value(raw_value) {
                    if !is_zero_ip_value(&ip) {
                        fields.insert("DST_ADDR", ip);
                    }
                }
            }
            IPFIX_FIELD_IP_VERSION => {
                if let Some(etype) = decode_akvorado_unsigned(raw_value)
                    .map(|value| value.to_string())
                    .and_then(|value| etype_from_ip_version(&value).map(str::to_string))
                {
                    fields.insert("ETYPE", etype);
                }
            }
            IPFIX_FIELD_INPUT_SNMP => {
                if let Some(value) = decode_akvorado_unsigned(raw_value) {
                    fields.insert("IN_IF", value.to_string());
                }
            }
            IPFIX_FIELD_OUTPUT_SNMP => {
                if let Some(value) = decode_akvorado_unsigned(raw_value) {
                    fields.insert("OUT_IF", value.to_string());
                }
            }
            IPFIX_FIELD_DIRECTION => {
                if let Some(value) = decode_akvorado_unsigned(raw_value) {
                    fields.insert("DIRECTION", value.to_string());
                }
            }
            IPFIX_FIELD_FORWARDING_STATUS => {
                if let Some(value) = decode_akvorado_unsigned(raw_value) {
                    fields.insert("FORWARDING_STATUS", value.to_string());
                }
            }
            IPFIX_FIELD_FLOW_START_MILLISECONDS => {
                if let Some(value) = decode_akvorado_unsigned(raw_value) {
                    flow_start_usec = Some(value.saturating_mul(USEC_PER_MILLISECOND));
                    fields.insert(
                        "FLOW_START_USEC",
                        value.saturating_mul(USEC_PER_MILLISECOND).to_string(),
                    );
                }
            }
            IPFIX_FIELD_FLOW_END_MILLISECONDS => {
                if let Some(value) = decode_akvorado_unsigned(raw_value) {
                    fields.insert(
                        "FLOW_END_USEC",
                        value.saturating_mul(USEC_PER_MILLISECOND).to_string(),
                    );
                }
            }
            IPFIX_FIELD_MINIMUM_TTL | IPFIX_FIELD_MAXIMUM_TTL => {
                if let Some(value) = decode_akvorado_unsigned(raw_value) {
                    fields.entry("IPTTL").or_insert_with(|| value.to_string());
                }
            }
            field_type if is_ipfix_mpls_label_field(field_type) => {
                if let Some(label) = decode_akvorado_unsigned(raw_value).map(|value| value >> 4)
                    && label > 0
                {
                    append_mpls_label_value(&mut fields, label);
                    has_mpls_labels = true;
                }
            }
            IPFIX_FIELD_DATALINK_FRAME_SIZE => {
                // Akvorado derives bytes from decoded L3 payload for field 315 path.
            }
            IPFIX_FIELD_DATALINK_FRAME_SECTION => {
                has_datalink_section = true;
                if let Some(l3_len) =
                    parse_datalink_frame_section(raw_value, &mut fields, decapsulation_mode)
                {
                    fields.insert("BYTES", l3_len.to_string());
                    fields.insert("PACKETS", "1".to_string());
                    has_decoded_datalink = true;
                }
            }
            _ => {}
        }
    }

    if let (Some(interval), Some(space)) = (sampling_packet_interval, sampling_packet_space)
        && interval > 0
    {
        observed_sampling_rate = Some((interval.saturating_add(space)) / interval);
    }

    if has_datalink_section && !has_decoded_datalink {
        return None;
    }
    if !has_datalink_section && !has_mpls_labels {
        return None;
    }

    fields.entry("FLOWS").or_insert_with(|| "1".to_string());
    apply_sampling_state_fields(
        &mut fields,
        exporter_ip,
        10,
        observation_domain_id,
        sampler_id,
        observed_sampling_rate,
        sampling,
    );
    finalize_canonical_flow_fields(&mut fields);

    Some(DecodedFlow {
        record: FlowRecord::from_fields(&fields),
        source_realtime_usec: timestamp_source.select(
            input_realtime_usec,
            packet_realtime_usec,
            flow_start_usec,
        ),
    })
}
