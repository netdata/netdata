use super::*;

/// Merge non-default field values from incoming record into existing record.
/// Used to fill in enrichment data when two decode passes produce overlapping flows.
pub(crate) fn merge_enriched_records(existing: &mut DecodedFlow, incoming: &DecodedFlow) -> bool {
    let mut changed = false;
    let default = FlowRecord::default();
    let src = &incoming.record;
    let dst = &mut existing.record;

    // Merge Copy fields: replace existing default with incoming non-default.
    macro_rules! merge_copy {
        ($field:ident) => {
            if src.$field != default.$field && dst.$field == default.$field {
                dst.$field = src.$field;
                changed = true;
            }
        };
    }
    // Merge String fields: replace existing empty with incoming non-empty.
    macro_rules! merge_str {
        ($field:ident) => {
            if !src.$field.is_empty() && dst.$field.is_empty() {
                dst.$field = src.$field.clone();
                changed = true;
            }
        };
    }
    macro_rules! merge_present_copy {
        ($has:ident, $set:ident, $field:ident) => {
            if src.$has() && !dst.$has() {
                dst.$set(src.$field);
                changed = true;
            }
        };
    }
    fn merge_asn_with_name(
        dst_as: &mut u32,
        dst_name: &mut String,
        src_as: u32,
        src_name: &str,
        changed: &mut bool,
    ) {
        if src_as != 0 && *dst_as == 0 {
            *dst_as = src_as;
            *changed = true;

            if src_name.is_empty() {
                if !dst_name.is_empty() {
                    dst_name.clear();
                    *changed = true;
                }
            } else if dst_name.as_str() != src_name {
                *dst_name = src_name.to_string();
                *changed = true;
            }
        }

        if !src_name.is_empty() && dst_name.is_empty() {
            *dst_name = src_name.to_string();
            *changed = true;
        }
    }

    // Protocol version / exporter identity
    merge_copy!(flow_version);
    merge_copy!(exporter_ip);
    merge_copy!(exporter_port);
    merge_str!(exporter_name);
    merge_str!(exporter_group);
    merge_str!(exporter_role);
    merge_str!(exporter_site);
    merge_str!(exporter_region);
    merge_str!(exporter_tenant);

    // Sampling
    merge_present_copy!(has_sampling_rate, set_sampling_rate, sampling_rate);

    // L2/L3 identity
    merge_present_copy!(has_etype, set_etype, etype);
    merge_copy!(protocol);
    merge_present_copy!(has_direction, set_direction, direction);

    // Counters
    merge_copy!(bytes);
    merge_copy!(packets);
    merge_copy!(flows);
    merge_copy!(raw_bytes);
    merge_copy!(raw_packets);
    merge_present_copy!(
        has_forwarding_status,
        set_forwarding_status,
        forwarding_status
    );

    // Endpoints
    merge_copy!(src_addr);
    merge_copy!(dst_addr);
    merge_copy!(src_prefix);
    merge_copy!(dst_prefix);
    merge_copy!(src_mask);
    merge_copy!(dst_mask);
    merge_asn_with_name(
        &mut dst.src_as,
        &mut dst.src_as_name,
        src.src_as,
        &src.src_as_name,
        &mut changed,
    );
    merge_asn_with_name(
        &mut dst.dst_as,
        &mut dst.dst_as_name,
        src.dst_as,
        &src.dst_as_name,
        &mut changed,
    );

    // Network attributes
    merge_str!(src_net_name);
    merge_str!(dst_net_name);
    merge_str!(src_net_role);
    merge_str!(dst_net_role);
    merge_str!(src_net_site);
    merge_str!(dst_net_site);
    merge_str!(src_net_region);
    merge_str!(dst_net_region);
    merge_str!(src_net_tenant);
    merge_str!(dst_net_tenant);
    merge_str!(src_country);
    merge_str!(dst_country);
    merge_str!(src_geo_city);
    merge_str!(dst_geo_city);
    merge_str!(src_geo_state);
    merge_str!(dst_geo_state);
    merge_str!(src_geo_latitude);
    merge_str!(dst_geo_latitude);
    merge_str!(src_geo_longitude);
    merge_str!(dst_geo_longitude);

    // BGP routing
    merge_str!(dst_as_path);
    merge_str!(dst_communities);
    merge_str!(dst_large_communities);

    // Interfaces
    merge_copy!(in_if);
    merge_copy!(out_if);
    merge_str!(in_if_name);
    merge_str!(out_if_name);
    merge_str!(in_if_description);
    merge_str!(out_if_description);
    merge_present_copy!(has_in_if_speed, set_in_if_speed, in_if_speed);
    merge_present_copy!(has_out_if_speed, set_out_if_speed, out_if_speed);
    merge_str!(in_if_provider);
    merge_str!(out_if_provider);
    merge_str!(in_if_connectivity);
    merge_str!(out_if_connectivity);
    merge_present_copy!(has_in_if_boundary, set_in_if_boundary, in_if_boundary);
    merge_present_copy!(has_out_if_boundary, set_out_if_boundary, out_if_boundary);

    // Next hop / ports
    merge_copy!(next_hop);
    merge_copy!(src_port);
    merge_copy!(dst_port);

    // Timestamps
    merge_copy!(flow_start_usec);
    merge_copy!(flow_end_usec);
    merge_copy!(observation_time_millis);

    // NAT
    merge_copy!(src_addr_nat);
    merge_copy!(dst_addr_nat);
    merge_copy!(src_port_nat);
    merge_copy!(dst_port_nat);

    // VLAN
    merge_present_copy!(has_src_vlan, set_src_vlan, src_vlan);
    merge_present_copy!(has_dst_vlan, set_dst_vlan, dst_vlan);

    // MAC addresses
    merge_copy!(src_mac);
    merge_copy!(dst_mac);

    // IP header fields
    merge_copy!(ipttl);
    merge_present_copy!(has_iptos, set_iptos, iptos);
    merge_copy!(ipv6_flow_label);
    merge_present_copy!(has_tcp_flags, set_tcp_flags, tcp_flags);
    merge_copy!(ip_fragment_id);
    merge_copy!(ip_fragment_offset);

    // ICMP
    merge_present_copy!(has_icmpv4_type, set_icmpv4_type, icmpv4_type);
    merge_present_copy!(has_icmpv4_code, set_icmpv4_code, icmpv4_code);
    merge_present_copy!(has_icmpv6_type, set_icmpv6_type, icmpv6_type);
    merge_present_copy!(has_icmpv6_code, set_icmpv6_code, icmpv6_code);

    // MPLS
    merge_str!(mpls_labels);

    if existing.source_realtime_usec.is_none() {
        existing.source_realtime_usec = incoming.source_realtime_usec;
        changed = incoming.source_realtime_usec.is_some() || changed;
    }

    changed
}
