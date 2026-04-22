use super::schema::{ROLLUP_FIELD_DEFS, is_internal_rollup_presence_field};
use super::*;
use crate::facet_runtime::FacetFileContribution;
use crate::ingest::JournalEncodeBuffer;
use crate::tiering::FlowMetrics;

pub(crate) fn emit_rollup_row(
    index: &FlowIndex,
    flow_id: IndexedFlowId,
    metrics: FlowMetrics,
    encode_buf: &mut JournalEncodeBuffer,
) -> Option<FacetFileContribution> {
    let mut contribution = FacetFileContribution::default();
    let presence = load_presence_state(index, flow_id)?;
    let mut virtual_icmp = VirtualIcmpInputs::default();

    encode_buf.clear();

    for (field_index, def) in ROLLUP_FIELD_DEFS.iter().enumerate() {
        let field_id = index.flow_field_id(flow_id, field_index)?;
        let value = index.field_value(field_index, field_id)?;

        match def.name {
            name if is_internal_rollup_presence_field(name) => {}
            "DIRECTION" => {
                if let IndexFieldValue::U8(direction) = value {
                    let direction_text = direction_from_u8(direction).as_str();
                    if presence.direction {
                        encode_buf.push_str("DIRECTION", direction_text);
                        contribution.insert_text_static("DIRECTION", direction_text);
                    } else {
                        encode_buf.push_str("DIRECTION", "");
                    }
                }
            }
            "PROTOCOL" => {
                if let IndexFieldValue::U8(protocol) = value {
                    encode_buf.push_u8("PROTOCOL", protocol);
                    contribution.insert_u8_present_static("PROTOCOL", protocol);
                    virtual_icmp.protocol = Some(protocol);
                }
            }
            "ETYPE" => {
                if let IndexFieldValue::U16(etype) = value {
                    push_optional_u16(
                        encode_buf,
                        &mut contribution,
                        "ETYPE",
                        presence.etype,
                        etype,
                    );
                }
            }
            "FORWARDING_STATUS" => {
                if let IndexFieldValue::U8(status) = value {
                    push_optional_u8(
                        encode_buf,
                        &mut contribution,
                        "FORWARDING_STATUS",
                        presence.forwarding_status,
                        status,
                    );
                }
            }
            "FLOW_VERSION" => {
                if let IndexFieldValue::Text(text) = value {
                    push_text(encode_buf, &mut contribution, "FLOW_VERSION", text);
                }
            }
            "IPTOS" => {
                if let IndexFieldValue::U8(iptos) = value {
                    push_optional_u8(
                        encode_buf,
                        &mut contribution,
                        "IPTOS",
                        presence.iptos,
                        iptos,
                    );
                }
            }
            "TCP_FLAGS" => {
                if let IndexFieldValue::U8(flags) = value {
                    push_optional_u8(
                        encode_buf,
                        &mut contribution,
                        "TCP_FLAGS",
                        presence.tcp_flags,
                        flags,
                    );
                }
            }
            "ICMPV4_TYPE" => {
                if let IndexFieldValue::U8(value) = value {
                    push_optional_u8(
                        encode_buf,
                        &mut contribution,
                        "ICMPV4_TYPE",
                        presence.icmpv4_type,
                        value,
                    );
                    if presence.icmpv4_type {
                        virtual_icmp.icmpv4_type = Some(value);
                    }
                }
            }
            "ICMPV4_CODE" => {
                if let IndexFieldValue::U8(value) = value {
                    push_optional_u8(
                        encode_buf,
                        &mut contribution,
                        "ICMPV4_CODE",
                        presence.icmpv4_code,
                        value,
                    );
                    if presence.icmpv4_code {
                        virtual_icmp.icmpv4_code = Some(value);
                    }
                }
            }
            "ICMPV6_TYPE" => {
                if let IndexFieldValue::U8(value) = value {
                    push_optional_u8(
                        encode_buf,
                        &mut contribution,
                        "ICMPV6_TYPE",
                        presence.icmpv6_type,
                        value,
                    );
                    if presence.icmpv6_type {
                        virtual_icmp.icmpv6_type = Some(value);
                    }
                }
            }
            "ICMPV6_CODE" => {
                if let IndexFieldValue::U8(value) = value {
                    push_optional_u8(
                        encode_buf,
                        &mut contribution,
                        "ICMPV6_CODE",
                        presence.icmpv6_code,
                        value,
                    );
                    if presence.icmpv6_code {
                        virtual_icmp.icmpv6_code = Some(value);
                    }
                }
            }
            "SRC_AS" => {
                if let IndexFieldValue::U32(value) = value {
                    push_u32(encode_buf, &mut contribution, "SRC_AS", value);
                }
            }
            "DST_AS" => {
                if let IndexFieldValue::U32(value) = value {
                    push_u32(encode_buf, &mut contribution, "DST_AS", value);
                }
            }
            "SRC_AS_NAME" => {
                if let IndexFieldValue::Text(text) = value {
                    push_text(encode_buf, &mut contribution, "SRC_AS_NAME", text);
                }
            }
            "DST_AS_NAME" => {
                if let IndexFieldValue::Text(text) = value {
                    push_text(encode_buf, &mut contribution, "DST_AS_NAME", text);
                }
            }
            "EXPORTER_IP" => {
                if let IndexFieldValue::IpAddr(ip) = value {
                    push_optional_ip(
                        encode_buf,
                        &mut contribution,
                        "EXPORTER_IP",
                        presence.exporter_ip,
                        ip,
                    );
                }
            }
            "EXPORTER_PORT" => {
                if let IndexFieldValue::U16(value) = value {
                    push_u16(encode_buf, &mut contribution, "EXPORTER_PORT", value);
                }
            }
            "EXPORTER_NAME" => {
                if let IndexFieldValue::Text(text) = value {
                    push_text(encode_buf, &mut contribution, "EXPORTER_NAME", text);
                }
            }
            "EXPORTER_GROUP" => {
                if let IndexFieldValue::Text(text) = value {
                    push_text(encode_buf, &mut contribution, "EXPORTER_GROUP", text);
                }
            }
            "EXPORTER_ROLE" => {
                if let IndexFieldValue::Text(text) = value {
                    push_text(encode_buf, &mut contribution, "EXPORTER_ROLE", text);
                }
            }
            "EXPORTER_SITE" => {
                if let IndexFieldValue::Text(text) = value {
                    push_text(encode_buf, &mut contribution, "EXPORTER_SITE", text);
                }
            }
            "EXPORTER_REGION" => {
                if let IndexFieldValue::Text(text) = value {
                    push_text(encode_buf, &mut contribution, "EXPORTER_REGION", text);
                }
            }
            "EXPORTER_TENANT" => {
                if let IndexFieldValue::Text(text) = value {
                    push_text(encode_buf, &mut contribution, "EXPORTER_TENANT", text);
                }
            }
            "IN_IF" => {
                if let IndexFieldValue::U32(value) = value {
                    push_u32(encode_buf, &mut contribution, "IN_IF", value);
                }
            }
            "OUT_IF" => {
                if let IndexFieldValue::U32(value) = value {
                    push_u32(encode_buf, &mut contribution, "OUT_IF", value);
                }
            }
            "IN_IF_NAME" => {
                if let IndexFieldValue::Text(text) = value {
                    push_text(encode_buf, &mut contribution, "IN_IF_NAME", text);
                }
            }
            "OUT_IF_NAME" => {
                if let IndexFieldValue::Text(text) = value {
                    push_text(encode_buf, &mut contribution, "OUT_IF_NAME", text);
                }
            }
            "IN_IF_DESCRIPTION" => {
                if let IndexFieldValue::Text(text) = value {
                    push_text(encode_buf, &mut contribution, "IN_IF_DESCRIPTION", text);
                }
            }
            "OUT_IF_DESCRIPTION" => {
                if let IndexFieldValue::Text(text) = value {
                    push_text(encode_buf, &mut contribution, "OUT_IF_DESCRIPTION", text);
                }
            }
            "IN_IF_SPEED" => {
                if let IndexFieldValue::U64(value) = value {
                    push_optional_u64(
                        encode_buf,
                        &mut contribution,
                        "IN_IF_SPEED",
                        presence.in_if_speed,
                        value,
                    );
                }
            }
            "OUT_IF_SPEED" => {
                if let IndexFieldValue::U64(value) = value {
                    push_optional_u64(
                        encode_buf,
                        &mut contribution,
                        "OUT_IF_SPEED",
                        presence.out_if_speed,
                        value,
                    );
                }
            }
            "IN_IF_PROVIDER" => {
                if let IndexFieldValue::Text(text) = value {
                    push_text(encode_buf, &mut contribution, "IN_IF_PROVIDER", text);
                }
            }
            "OUT_IF_PROVIDER" => {
                if let IndexFieldValue::Text(text) = value {
                    push_text(encode_buf, &mut contribution, "OUT_IF_PROVIDER", text);
                }
            }
            "IN_IF_CONNECTIVITY" => {
                if let IndexFieldValue::Text(text) = value {
                    push_text(encode_buf, &mut contribution, "IN_IF_CONNECTIVITY", text);
                }
            }
            "OUT_IF_CONNECTIVITY" => {
                if let IndexFieldValue::Text(text) = value {
                    push_text(encode_buf, &mut contribution, "OUT_IF_CONNECTIVITY", text);
                }
            }
            "IN_IF_BOUNDARY" => {
                if let IndexFieldValue::U8(value) = value {
                    push_optional_u8(
                        encode_buf,
                        &mut contribution,
                        "IN_IF_BOUNDARY",
                        presence.in_if_boundary,
                        value,
                    );
                }
            }
            "OUT_IF_BOUNDARY" => {
                if let IndexFieldValue::U8(value) = value {
                    push_optional_u8(
                        encode_buf,
                        &mut contribution,
                        "OUT_IF_BOUNDARY",
                        presence.out_if_boundary,
                        value,
                    );
                }
            }
            "SRC_NET_NAME" => {
                if let IndexFieldValue::Text(text) = value {
                    push_text(encode_buf, &mut contribution, "SRC_NET_NAME", text);
                }
            }
            "DST_NET_NAME" => {
                if let IndexFieldValue::Text(text) = value {
                    push_text(encode_buf, &mut contribution, "DST_NET_NAME", text);
                }
            }
            "SRC_NET_ROLE" => {
                if let IndexFieldValue::Text(text) = value {
                    push_text(encode_buf, &mut contribution, "SRC_NET_ROLE", text);
                }
            }
            "DST_NET_ROLE" => {
                if let IndexFieldValue::Text(text) = value {
                    push_text(encode_buf, &mut contribution, "DST_NET_ROLE", text);
                }
            }
            "SRC_NET_SITE" => {
                if let IndexFieldValue::Text(text) = value {
                    push_text(encode_buf, &mut contribution, "SRC_NET_SITE", text);
                }
            }
            "DST_NET_SITE" => {
                if let IndexFieldValue::Text(text) = value {
                    push_text(encode_buf, &mut contribution, "DST_NET_SITE", text);
                }
            }
            "SRC_NET_REGION" => {
                if let IndexFieldValue::Text(text) = value {
                    push_text(encode_buf, &mut contribution, "SRC_NET_REGION", text);
                }
            }
            "DST_NET_REGION" => {
                if let IndexFieldValue::Text(text) = value {
                    push_text(encode_buf, &mut contribution, "DST_NET_REGION", text);
                }
            }
            "SRC_NET_TENANT" => {
                if let IndexFieldValue::Text(text) = value {
                    push_text(encode_buf, &mut contribution, "SRC_NET_TENANT", text);
                }
            }
            "DST_NET_TENANT" => {
                if let IndexFieldValue::Text(text) = value {
                    push_text(encode_buf, &mut contribution, "DST_NET_TENANT", text);
                }
            }
            "SRC_COUNTRY" => {
                if let IndexFieldValue::Text(text) = value {
                    push_text(encode_buf, &mut contribution, "SRC_COUNTRY", text);
                }
            }
            "DST_COUNTRY" => {
                if let IndexFieldValue::Text(text) = value {
                    push_text(encode_buf, &mut contribution, "DST_COUNTRY", text);
                }
            }
            "SRC_GEO_STATE" => {
                if let IndexFieldValue::Text(text) = value {
                    push_text(encode_buf, &mut contribution, "SRC_GEO_STATE", text);
                }
            }
            "DST_GEO_STATE" => {
                if let IndexFieldValue::Text(text) = value {
                    push_text(encode_buf, &mut contribution, "DST_GEO_STATE", text);
                }
            }
            "NEXT_HOP" => {
                if let IndexFieldValue::IpAddr(ip) = value {
                    push_optional_ip(
                        encode_buf,
                        &mut contribution,
                        "NEXT_HOP",
                        presence.next_hop,
                        ip,
                    );
                }
            }
            "SRC_VLAN" => {
                if let IndexFieldValue::U16(value) = value {
                    push_optional_u16(
                        encode_buf,
                        &mut contribution,
                        "SRC_VLAN",
                        presence.src_vlan,
                        value,
                    );
                }
            }
            "DST_VLAN" => {
                if let IndexFieldValue::U16(value) = value {
                    push_optional_u16(
                        encode_buf,
                        &mut contribution,
                        "DST_VLAN",
                        presence.dst_vlan,
                        value,
                    );
                }
            }
            _ => {}
        }
    }

    encode_buf.push_u64("BYTES", metrics.bytes);
    encode_buf.push_u64("PACKETS", metrics.packets);

    append_virtual_icmp_fields(&mut contribution, virtual_icmp);

    Some(contribution)
}

#[derive(Default)]
struct RollupPresenceState {
    exporter_ip: bool,
    next_hop: bool,
    direction: bool,
    etype: bool,
    forwarding_status: bool,
    iptos: bool,
    tcp_flags: bool,
    icmpv4_type: bool,
    icmpv4_code: bool,
    icmpv6_type: bool,
    icmpv6_code: bool,
    in_if_speed: bool,
    out_if_speed: bool,
    in_if_boundary: bool,
    out_if_boundary: bool,
    src_vlan: bool,
    dst_vlan: bool,
}

#[derive(Default)]
struct VirtualIcmpInputs {
    protocol: Option<u8>,
    icmpv4_type: Option<u8>,
    icmpv4_code: Option<u8>,
    icmpv6_type: Option<u8>,
    icmpv6_code: Option<u8>,
}

fn load_presence_state(index: &FlowIndex, flow_id: IndexedFlowId) -> Option<RollupPresenceState> {
    Some(RollupPresenceState {
        exporter_ip: presence_flag(index, flow_id, INTERNAL_EXPORTER_IP_PRESENT)?,
        next_hop: presence_flag(index, flow_id, INTERNAL_NEXT_HOP_PRESENT)?,
        direction: presence_flag(index, flow_id, INTERNAL_DIRECTION_PRESENT)?,
        etype: presence_flag(index, flow_id, "_ETYPE_PRESENT")?,
        forwarding_status: presence_flag(index, flow_id, "_FORWARDING_STATUS_PRESENT")?,
        iptos: presence_flag(index, flow_id, "_IPTOS_PRESENT")?,
        tcp_flags: presence_flag(index, flow_id, "_TCP_FLAGS_PRESENT")?,
        icmpv4_type: presence_flag(index, flow_id, "_ICMPV4_TYPE_PRESENT")?,
        icmpv4_code: presence_flag(index, flow_id, "_ICMPV4_CODE_PRESENT")?,
        icmpv6_type: presence_flag(index, flow_id, "_ICMPV6_TYPE_PRESENT")?,
        icmpv6_code: presence_flag(index, flow_id, "_ICMPV6_CODE_PRESENT")?,
        in_if_speed: presence_flag(index, flow_id, "_IN_IF_SPEED_PRESENT")?,
        out_if_speed: presence_flag(index, flow_id, "_OUT_IF_SPEED_PRESENT")?,
        in_if_boundary: presence_flag(index, flow_id, "_IN_IF_BOUNDARY_PRESENT")?,
        out_if_boundary: presence_flag(index, flow_id, "_OUT_IF_BOUNDARY_PRESENT")?,
        src_vlan: presence_flag(index, flow_id, "_SRC_VLAN_PRESENT")?,
        dst_vlan: presence_flag(index, flow_id, "_DST_VLAN_PRESENT")?,
    })
}

fn presence_flag(index: &FlowIndex, flow_id: IndexedFlowId, field: &str) -> Option<bool> {
    Some(matches!(
        rollup_field_value(index, flow_id, field)?,
        IndexFieldValue::U8(1)
    ))
}

fn push_text(
    encode_buf: &mut JournalEncodeBuffer,
    contribution: &mut FacetFileContribution,
    field: &'static str,
    value: &str,
) {
    encode_buf.push_str(field, value);
    contribution.insert_text_static(field, value);
}

fn push_u32(
    encode_buf: &mut JournalEncodeBuffer,
    contribution: &mut FacetFileContribution,
    field: &'static str,
    value: u32,
) {
    encode_buf.push_u32(field, value);
    contribution.insert_u32_present_static(field, value);
}

fn push_u16(
    encode_buf: &mut JournalEncodeBuffer,
    contribution: &mut FacetFileContribution,
    field: &'static str,
    value: u16,
) {
    encode_buf.push_u16(field, value);
    contribution.insert_u16_present_static(field, value);
}

fn push_optional_u8(
    encode_buf: &mut JournalEncodeBuffer,
    contribution: &mut FacetFileContribution,
    field: &'static str,
    present: bool,
    value: u8,
) {
    if present {
        encode_buf.push_u8(field, value);
        contribution.insert_u8_present_static(field, value);
    } else {
        encode_buf.push_str(field, "");
    }
}

fn push_optional_u16(
    encode_buf: &mut JournalEncodeBuffer,
    contribution: &mut FacetFileContribution,
    field: &'static str,
    present: bool,
    value: u16,
) {
    if present {
        encode_buf.push_u16(field, value);
        contribution.insert_u16_present_static(field, value);
    } else {
        encode_buf.push_str(field, "");
    }
}

fn push_optional_u64(
    encode_buf: &mut JournalEncodeBuffer,
    contribution: &mut FacetFileContribution,
    field: &'static str,
    present: bool,
    value: u64,
) {
    if present {
        encode_buf.push_u64(field, value);
        contribution.insert_u64_present_static(field, value);
    } else {
        encode_buf.push_str(field, "");
    }
}

fn push_optional_ip(
    encode_buf: &mut JournalEncodeBuffer,
    contribution: &mut FacetFileContribution,
    field: &'static str,
    present: bool,
    value: IpAddr,
) {
    if present {
        encode_buf.push_ip_addr(field, value);
        contribution.insert_ip_present_static(field, value);
    } else {
        encode_buf.push_str(field, "");
    }
}

fn append_virtual_icmp_fields(contribution: &mut FacetFileContribution, inputs: VirtualIcmpInputs) {
    if let Some(value) = virtual_icmp_value(
        inputs.protocol,
        inputs.icmpv4_type,
        inputs.icmpv4_code,
        "ICMPV4",
    ) {
        contribution.insert_text_static("ICMPV4", &value);
    }
    if let Some(value) = virtual_icmp_value(
        inputs.protocol,
        inputs.icmpv6_type,
        inputs.icmpv6_code,
        "ICMPV6",
    ) {
        contribution.insert_text_static("ICMPV6", &value);
    }
}

fn virtual_icmp_value(
    protocol: Option<u8>,
    icmp_type: Option<u8>,
    icmp_code: Option<u8>,
    field: &'static str,
) -> Option<String> {
    let mut protocol_buf = itoa::Buffer::new();
    let mut type_buf = itoa::Buffer::new();
    let mut code_buf = itoa::Buffer::new();

    crate::presentation::icmp_virtual_value(
        field,
        protocol.map(|value| protocol_buf.format(value)),
        icmp_type.map(|value| type_buf.format(value)),
        icmp_code.map(|value| code_buf.format(value)),
    )
}

#[cfg(test)]
mod tests {
    use super::emit_rollup_row;
    use crate::facet_runtime::{FacetFileContribution, facet_contribution_from_flow_fields};
    use crate::flow::{FlowDirection, FlowRecord};
    use crate::ingest::JournalEncodeBuffer;
    use crate::tiering::{FlowMetrics, TierFlowIndexStore};

    #[test]
    fn direct_emit_matches_materialized_row_semantics() {
        let mut store = TierFlowIndexStore::default();
        let mut record = FlowRecord::default();
        record.set_direction(FlowDirection::Ingress);
        record.protocol = 1;
        record.set_etype(2048);
        record.icmpv4_type = 8;
        record.icmpv4_code = 0;
        record.exporter_port = 9995;
        record.exporter_name = "edge-1".to_string();
        record.src_as = 64512;
        record.dst_as_name = "AS15169 Google LLC".to_string();
        record.in_if = 10;
        record.out_if = 0;
        record.set_in_if_speed(1_000_000_000);
        record.src_country = "US".to_string();
        record.next_hop = Some("192.0.2.1".parse().unwrap());
        record.set_src_vlan(12);
        record.bytes = 1234;
        record.packets = 9;

        let flow_ref = store
            .get_or_insert_record_flow(120_000_000, &record)
            .expect("intern tier flow");
        let mut fields = store
            .materialize_fields(flow_ref)
            .expect("materialize fields");
        let metrics = FlowMetrics::from_record(&record);
        metrics.write_fields(&mut fields);

        let mut expected_buf = JournalEncodeBuffer::new();
        expected_buf.encode(&fields);
        let expected_payloads = sorted_payloads(&expected_buf);
        let expected_contribution = facet_contribution_from_flow_fields(&fields);

        let mut actual_buf = JournalEncodeBuffer::new();
        let actual_contribution = emit_rollup_row(
            store.index_for_test(flow_ref).expect("rollup index"),
            flow_ref.flow_id,
            metrics,
            &mut actual_buf,
        )
        .expect("emit row");
        let actual_payloads = sorted_payloads(&actual_buf);

        assert_eq!(actual_payloads, expected_payloads);
        assert_eq!(
            contribution_debug_map(&actual_contribution),
            contribution_debug_map(&expected_contribution)
        );
    }

    fn sorted_payloads(buf: &JournalEncodeBuffer) -> Vec<Vec<u8>> {
        let mut payloads = buf
            .debug_field_slices()
            .into_iter()
            .map(|slice| slice.to_vec())
            .collect::<Vec<_>>();
        payloads.sort();
        payloads
    }

    fn contribution_debug_map(
        contribution: &FacetFileContribution,
    ) -> Vec<(&'static str, Vec<String>)> {
        let mut fields = contribution
            .debug_string_map()
            .into_iter()
            .collect::<Vec<_>>();
        fields.sort_by_key(|(field, _)| *field);
        fields
    }
}
