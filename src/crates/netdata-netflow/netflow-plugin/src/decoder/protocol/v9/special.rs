use super::super::*;

pub(crate) fn decode_v9_special_from_raw_payload(
    source: SocketAddr,
    payload: &[u8],
    sampling: &SamplingState,
    decapsulation_mode: DecapsulationMode,
    timestamp_source: TimestampSource,
    input_realtime_usec: u64,
) -> Vec<DecodedFlow> {
    if payload.len() < 20 {
        return Vec::new();
    }
    if u16::from_be_bytes([payload[0], payload[1]]) != 9 {
        return Vec::new();
    }

    let sys_uptime_millis =
        u32::from_be_bytes([payload[4], payload[5], payload[6], payload[7]]) as u64;
    let export_time = u32::from_be_bytes([payload[8], payload[9], payload[10], payload[11]]) as u64;
    let packet_realtime_usec = Some(unix_timestamp_to_usec(export_time, 0));
    let exporter_ip = source.ip();
    let observation_domain_id =
        u32::from_be_bytes([payload[16], payload[17], payload[18], payload[19]]);
    let mut offset = 20_usize;
    let mut out = Vec::new();

    while offset.saturating_add(4) <= payload.len() {
        let flowset_id = u16::from_be_bytes([payload[offset], payload[offset + 1]]);
        let flowset_len = u16::from_be_bytes([payload[offset + 2], payload[offset + 3]]) as usize;
        if flowset_len < 4 {
            return out;
        }
        let end = offset.saturating_add(flowset_len);
        if end > payload.len() {
            return out;
        }
        let body = &payload[offset + 4..end];

        if flowset_id >= 256
            && let Some(template) =
                sampling.get_v9_datalink_template(exporter_ip, observation_domain_id, flowset_id)
        {
            let mut cursor = body;
            while !cursor.is_empty() {
                let Some((record_values, consumed)) =
                    parse_v9_record_from_template(cursor, &template.fields)
                else {
                    break;
                };
                if let Some(flow) = decode_v9_special_record(
                    source,
                    timestamp_source,
                    input_realtime_usec,
                    packet_realtime_usec,
                    export_time,
                    sys_uptime_millis,
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

pub(crate) fn parse_v9_record_from_template<'a>(
    body: &'a [u8],
    fields: &[V9TemplateField],
) -> Option<(Vec<&'a [u8]>, usize)> {
    let mut consumed = 0_usize;
    let mut values = Vec::with_capacity(fields.len());

    for field in fields {
        if field.field_length == 0 {
            return None;
        }
        if consumed.saturating_add(field.field_length) > body.len() {
            return None;
        }
        values.push(&body[consumed..consumed + field.field_length]);
        consumed = consumed.saturating_add(field.field_length);
    }

    Some((values, consumed))
}

pub(crate) fn decode_v9_special_record(
    source: SocketAddr,
    timestamp_source: TimestampSource,
    input_realtime_usec: u64,
    packet_realtime_usec: Option<u64>,
    export_time_seconds: u64,
    sys_uptime_millis: u64,
    sampling: &SamplingState,
    exporter_ip: IpAddr,
    observation_domain_id: u32,
    template: &V9DataLinkTemplate,
    values: &[&[u8]],
    decapsulation_mode: DecapsulationMode,
) -> Option<DecodedFlow> {
    let mut fields = base_fields("v9", source);
    let mut has_datalink_section = false;
    let mut has_decoded_datalink = false;
    let mut flow_start_usec: Option<u64> = None;
    let mut flow_end_usec: Option<u64> = None;
    let mut sampler_id: Option<u64> = None;
    let mut observed_sampling_rate: Option<u64> = None;
    let system_init_usec = netflow_v9_system_init_usec(export_time_seconds, sys_uptime_millis);

    for (template_field, raw_value) in template.fields.iter().zip(values.iter()) {
        let field = V9Field::from(template_field.field_type);
        if field == V9Field::Layer2packetSectionData {
            has_datalink_section = true;
            if let Some(l3_len) =
                parse_datalink_frame_section(raw_value, &mut fields, decapsulation_mode)
            {
                fields.insert("BYTES", l3_len.to_string());
                fields.insert("PACKETS", "1".to_string());
                has_decoded_datalink = true;
            }
            continue;
        }

        let value = match field {
            V9Field::Ipv4SrcAddr
            | V9Field::Ipv4DstAddr
            | V9Field::Ipv4NextHop
            | V9Field::BgpIpv4NextHop
            | V9Field::Ipv4SrcPrefix
            | V9Field::Ipv4DstPrefix
            | V9Field::Ipv6SrcAddr
            | V9Field::Ipv6DstAddr
            | V9Field::Ipv6NextHop
            | V9Field::BpgIpv6NextHop
            | V9Field::PostNATSourceIPv4Address
            | V9Field::PostNATDestinationIPv4Address
            | V9Field::PostNATSourceIpv6Address
            | V9Field::PostNATDestinationIpv6Address => parse_ip_value(raw_value)
                .unwrap_or_else(|| decode_akvorado_unsigned(raw_value).to_string()),
            V9Field::InSrcMac | V9Field::OutSrcMac | V9Field::InDstMac | V9Field::OutDstMac => {
                mac_to_string(raw_value)
            }
            _ => decode_akvorado_unsigned(raw_value).to_string(),
        };

        apply_v9_special_mappings(&mut fields, field, &value);
        match field {
            V9Field::FlowSamplerId => sampler_id = value.parse::<u64>().ok(),
            V9Field::SamplingInterval | V9Field::FlowSamplerRandomInterval => {
                observed_sampling_rate = value.parse::<u64>().ok();
            }
            _ => {}
        }
        if let Some(canonical) = v9_canonical_key(field) {
            if should_skip_zero_ip(canonical, &value) {
                continue;
            }
            fields
                .entry(canonical)
                .or_insert_with(|| canonical_value(canonical, &value).to_string());
        }

        if matches!(
            field,
            V9Field::FirstSwitched | V9Field::FlowStartMilliseconds
        ) {
            flow_start_usec = value.parse::<u64>().ok().map(|switched_millis| {
                netflow_v9_uptime_millis_to_absolute_usec(system_init_usec, switched_millis)
            });
            continue;
        }

        if matches!(field, V9Field::LastSwitched | V9Field::FlowEndMilliseconds) {
            flow_end_usec = value.parse::<u64>().ok().map(|switched_millis| {
                netflow_v9_uptime_millis_to_absolute_usec(system_init_usec, switched_millis)
            });
            continue;
        }
    }

    if !has_datalink_section || !has_decoded_datalink {
        return None;
    }

    fields.entry("FLOWS").or_insert_with(|| "1".to_string());
    apply_sampling_state_fields(
        &mut fields,
        exporter_ip,
        9,
        observation_domain_id,
        sampler_id,
        observed_sampling_rate,
        sampling,
    );
    if let Some(start_usec) = flow_start_usec {
        fields.insert("FLOW_START_USEC", start_usec.to_string());
    }
    if let Some(end_usec) = flow_end_usec {
        fields.insert("FLOW_END_USEC", end_usec.to_string());
    }
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
