use super::*;

pub(crate) fn build_sflow_flow(
    source: SocketAddr,
    exporter_ip_override: Option<IpAddr>,
    sampling_rate: u32,
    in_if: Option<u32>,
    out_if: Option<u32>,
    forwarding_status: u32,
    flow_records: &[SFlowRecord],
    source_realtime_usec: Option<u64>,
    decapsulation_mode: DecapsulationMode,
    need_decap: bool,
) -> Option<DecodedFlow> {
    let mut rec = base_record("sflow", source);
    if let Some(ip) = exporter_ip_override {
        rec.exporter_ip = Some(ip);
    }
    rec.set_sampling_rate(sampling_rate as u64);
    rec.set_forwarding_status(forwarding_status as u8);
    if let Some(value) = in_if {
        rec.in_if = value;
    }
    if let Some(value) = out_if {
        rec.out_if = value;
    }

    let has_sampled_ipv4 = flow_records
        .iter()
        .any(|record| matches!(&record.flow_data, FlowData::SampledIpv4(_)));
    let has_sampled_ipv6 = flow_records
        .iter()
        .any(|record| matches!(&record.flow_data, FlowData::SampledIpv6(_)));
    let has_sampled_ethernet = flow_records
        .iter()
        .any(|record| matches!(&record.flow_data, FlowData::SampledEthernet(_)));
    let has_extended_switch = flow_records
        .iter()
        .any(|record| matches!(&record.flow_data, FlowData::ExtendedSwitch(_)));

    let mut l3_length = 0_u64;
    for flow_record in flow_records {
        match &flow_record.flow_data {
            FlowData::SampledHeader(sampled) => {
                let needs_ip_data = !(has_sampled_ipv4 || has_sampled_ipv6);
                let needs_l2_data = !(has_sampled_ethernet && has_extended_switch);
                let needs_l3_l4_data = true;
                if needs_ip_data || needs_l2_data || needs_l3_l4_data || need_decap {
                    let parsed_len = match sampled.protocol {
                        HeaderProtocol::EthernetIso88023 => parse_datalink_frame_section_record(
                            &sampled.header,
                            &mut rec,
                            decapsulation_mode,
                        ),
                        HeaderProtocol::Ipv4 => {
                            parse_ipv4_packet_record(&sampled.header, &mut rec, decapsulation_mode)
                        }
                        HeaderProtocol::Ipv6 => {
                            parse_ipv6_packet_record(&sampled.header, &mut rec, decapsulation_mode)
                        }
                        _ => None,
                    };
                    if let Some(length) = parsed_len
                        && length > 0
                    {
                        l3_length = length;
                    }
                }
            }
            FlowData::SampledIpv4(sampled) => {
                if need_decap {
                    continue;
                }
                rec.src_addr = Some(IpAddr::V4(sampled.src_ip));
                rec.dst_addr = Some(IpAddr::V4(sampled.dst_ip));
                rec.set_src_port(sampled.src_port as u16);
                rec.set_dst_port(sampled.dst_port as u16);
                rec.protocol = sampled.protocol as u8;
                rec.set_etype(2048);
                rec.set_iptos(sampled.tos as u8);
                l3_length = sampled.length as u64;
            }
            FlowData::SampledIpv6(sampled) => {
                if need_decap {
                    continue;
                }
                rec.src_addr = Some(IpAddr::V6(sampled.src_ip));
                rec.dst_addr = Some(IpAddr::V6(sampled.dst_ip));
                rec.set_src_port(sampled.src_port as u16);
                rec.set_dst_port(sampled.dst_port as u16);
                rec.protocol = sampled.protocol as u8;
                rec.set_etype(34525);
                rec.set_iptos(sampled.priority as u8);
                l3_length = sampled.length as u64;
            }
            FlowData::SampledEthernet(sampled) => {
                if need_decap {
                    continue;
                }
                if l3_length == 0 {
                    l3_length = sampled.length.saturating_sub(16) as u64;
                }
                rec.src_mac = parse_mac(&sampled.src_mac.to_string());
                rec.dst_mac = parse_mac(&sampled.dst_mac.to_string());
            }
            FlowData::ExtendedSwitch(record) => {
                if need_decap {
                    continue;
                }
                if record.src_vlan < 4096 {
                    rec.set_src_vlan(record.src_vlan as u16);
                }
                if record.dst_vlan < 4096 {
                    rec.set_dst_vlan(record.dst_vlan as u16);
                }
            }
            FlowData::ExtendedRouter(record) => {
                if need_decap {
                    continue;
                }
                rec.src_mask = record.src_mask_len as u8;
                rec.dst_mask = record.dst_mask_len as u8;
                if let Some(next_hop) = sflow_agent_ip_addr(&record.next_hop) {
                    rec.next_hop = Some(next_hop);
                }
            }
            FlowData::ExtendedGateway(record) => {
                if need_decap {
                    continue;
                }
                if let Some(next_hop) = sflow_agent_ip_addr(&record.next_hop) {
                    rec.next_hop = Some(next_hop);
                }

                rec.dst_as = record.as_number;
                rec.src_as = record.as_number;
                if record.src_as > 0 {
                    rec.src_as = record.src_as;
                }

                let mut dst_path = Vec::new();
                for segment in &record.dst_as_path {
                    dst_path.extend(segment.path.iter().copied());
                }
                if let Some(&last_asn) = dst_path.last() {
                    rec.dst_as = last_asn;
                }
                if !dst_path.is_empty() {
                    rec.dst_as_path = dst_path
                        .iter()
                        .map(u32::to_string)
                        .collect::<Vec<_>>()
                        .join(",");
                }
                if !record.communities.is_empty() {
                    rec.dst_communities = record
                        .communities
                        .iter()
                        .map(u32::to_string)
                        .collect::<Vec<_>>()
                        .join(",");
                }
            }
            _ => {}
        }
    }

    if l3_length > 0 {
        rec.bytes = l3_length;
    } else if need_decap {
        return None;
    }

    rec.packets = 1;
    rec.flows = 1;
    finalize_record(&mut rec);

    Some(DecodedFlow {
        record: rec,
        source_realtime_usec,
    })
}
