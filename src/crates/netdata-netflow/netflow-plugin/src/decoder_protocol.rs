fn is_sflow_payload(payload: &[u8]) -> bool {
    if payload.len() < 4 {
        return false;
    }
    u32::from_be_bytes([payload[0], payload[1], payload[2], payload[3]]) == 5
}

fn decode_sflow(
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

fn decode_netflow(
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

    append_unique_flows(&mut batch.flows, raw_v9_flows);
    append_unique_flows(&mut batch.flows, raw_ipfix_flows);

    batch
}

fn append_unique_flows(dst: &mut Vec<DecodedFlow>, incoming: Vec<DecodedFlow>) {
    if incoming.is_empty() {
        return;
    }

    // Build a hash index over existing flows for O(1) identity lookups instead of O(n) scans.
    let mut identity_index: HashMap<u64, Vec<usize>> = HashMap::new();
    for (idx, flow) in dst.iter().enumerate() {
        identity_index
            .entry(flow_identity_hash(flow))
            .or_default()
            .push(idx);
    }

    for flow in incoming {
        let hash = flow_identity_hash(&flow);

        // Try identity-based merge first (most common case for special decode overlap).
        if let Some(candidates) = identity_index.get(&hash) {
            let mut handled = false;
            for &idx in candidates {
                if same_flow_identity(&dst[idx], &flow) {
                    let merged = merge_enriched_records(&mut dst[idx], &flow);
                    if merged {
                        // Incoming enrichment data was merged into existing flow.
                        handled = true;
                        break;
                    }
                    // Merge found nothing to update. If the records are identical,
                    // it's an exact duplicate — drop regardless of timestamp
                    // (different decode paths can compute different timestamps
                    // for the same logical flow).
                    if dst[idx].record == flow.record {
                        handled = true;
                        break;
                    }
                    // Records have same identity but different non-identity data
                    // (e.g., different TTL values). Keep both as distinct flows.
                }
            }
            if handled {
                continue;
            }
        }

        let new_idx = dst.len();
        identity_index.entry(hash).or_default().push(new_idx);
        dst.push(flow);
    }
}

/// Hash identity fields directly from FlowRecord — zero string allocation.
/// observation_time_millis is intentionally excluded: it derives from packet
/// arrival time, not flow identity, so two records for the same logical flow
/// from different decode paths may have different observation timestamps.
fn flow_identity_hash(flow: &DecodedFlow) -> u64 {
    use std::hash::Hasher;
    let mut hasher = std::collections::hash_map::DefaultHasher::new();
    let r = &flow.record;
    let identity_bytes = identity_counter_value(r.raw_bytes, r.bytes);
    let identity_packets = identity_counter_value(r.raw_packets, r.packets);
    r.flow_version.hash(&mut hasher);
    r.exporter_ip.hash(&mut hasher);
    r.exporter_port.hash(&mut hasher);
    r.src_addr.hash(&mut hasher);
    r.dst_addr.hash(&mut hasher);
    r.protocol.hash(&mut hasher);
    r.src_port.hash(&mut hasher);
    r.dst_port.hash(&mut hasher);
    r.in_if.hash(&mut hasher);
    r.out_if.hash(&mut hasher);
    identity_bytes.hash(&mut hasher);
    identity_packets.hash(&mut hasher);
    r.flow_start_usec.hash(&mut hasher);
    r.flow_end_usec.hash(&mut hasher);
    r.direction.hash(&mut hasher);
    hasher.finish()
}

fn same_flow_identity(existing: &DecodedFlow, incoming: &DecodedFlow) -> bool {
    let a = &existing.record;
    let b = &incoming.record;
    let a_bytes = identity_counter_value(a.raw_bytes, a.bytes);
    let a_packets = identity_counter_value(a.raw_packets, a.packets);
    let b_bytes = identity_counter_value(b.raw_bytes, b.bytes);
    let b_packets = identity_counter_value(b.raw_packets, b.packets);
    a.flow_version == b.flow_version
        && a.exporter_ip == b.exporter_ip
        && a.exporter_port == b.exporter_port
        && a.src_addr == b.src_addr
        && a.dst_addr == b.dst_addr
        && a.protocol == b.protocol
        && a.src_port == b.src_port
        && a.dst_port == b.dst_port
        && a.in_if == b.in_if
        && a.out_if == b.out_if
        && a_bytes == b_bytes
        && a_packets == b_packets
        && a.flow_start_usec == b.flow_start_usec
        && a.flow_end_usec == b.flow_end_usec
        && a.direction == b.direction
}

fn identity_counter_value(raw: u64, scaled: u64) -> u64 {
    if raw != 0 { raw } else { scaled }
}

/// Merge non-default field values from incoming record into existing record.
/// Used to fill in enrichment data when two decode passes produce overlapping flows.
fn merge_enriched_records(existing: &mut DecodedFlow, incoming: &DecodedFlow) -> bool {
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
    merge_copy!(src_as);
    merge_copy!(dst_as);

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

fn is_ipfix_mpls_label_field(field_type: u16) -> bool {
    (IPFIX_FIELD_MPLS_LABEL_1..=IPFIX_FIELD_MPLS_LABEL_10).contains(&field_type)
}

fn observe_v9_decoder_state_from_raw_payload(
    source: SocketAddr,
    payload: &[u8],
    sampling: &mut SamplingState,
    namespace: &mut DecoderStateNamespace,
) -> DecoderStateObservation {
    let template_state_changed =
        observe_v9_templates_from_raw_payload(source, payload, sampling, namespace);
    let sampling_state_changed =
        observe_v9_sampling_from_raw_payload(source, payload, sampling, namespace);
    DecoderStateObservation {
        namespace_state_changed: template_state_changed || sampling_state_changed,
        template_state_changed,
    }
}

fn observe_v9_sampling_from_raw_payload(
    source: SocketAddr,
    payload: &[u8],
    sampling: &mut SamplingState,
    namespace: &mut DecoderStateNamespace,
) -> bool {
    // netflow_parser currently decodes some options data flowsets as empty records
    // when they contain unsupported field widths (for example SAMPLER_NAME=32 bytes).
    // Parse v9 options templates/data minimally here to preserve Akvorado sampling parity.
    if payload.len() < 20 {
        return false;
    }
    if u16::from_be_bytes([payload[0], payload[1]]) != 9 {
        return false;
    }

    let exporter_ip = source.ip().to_string();
    let observation_domain_id =
        u32::from_be_bytes([payload[16], payload[17], payload[18], payload[19]]);
    let mut offset = 20_usize;
    let mut changed = false;

    while offset.saturating_add(4) <= payload.len() {
        let flowset_id = u16::from_be_bytes([payload[offset], payload[offset + 1]]);
        let flowset_len = u16::from_be_bytes([payload[offset + 2], payload[offset + 3]]) as usize;
        if flowset_len < 4 {
            return changed;
        }
        let end = offset.saturating_add(flowset_len);
        if end > payload.len() {
            return changed;
        }
        let body = &payload[offset + 4..end];

        if flowset_id == 1 {
            changed |= observe_v9_sampling_templates(
                &exporter_ip,
                observation_domain_id,
                body,
                sampling,
                namespace,
            );
        } else if flowset_id >= 256
            && let Some(template) =
                sampling.get_v9_sampling_template(&exporter_ip, observation_domain_id, flowset_id)
        {
            changed |= observe_v9_sampling_data(
                &exporter_ip,
                observation_domain_id,
                &template,
                body,
                sampling,
                namespace,
            );
        }

        offset = end;
    }

    changed
}

fn observe_v9_templates_from_raw_payload(
    source: SocketAddr,
    payload: &[u8],
    sampling: &mut SamplingState,
    namespace: &mut DecoderStateNamespace,
) -> bool {
    if payload.len() < 20 {
        return false;
    }
    if u16::from_be_bytes([payload[0], payload[1]]) != 9 {
        return false;
    }

    let exporter_ip = source.ip().to_string();
    let observation_domain_id =
        u32::from_be_bytes([payload[16], payload[17], payload[18], payload[19]]);
    let mut offset = 20_usize;
    let mut changed = false;

    while offset.saturating_add(4) <= payload.len() {
        let flowset_id = u16::from_be_bytes([payload[offset], payload[offset + 1]]);
        let flowset_len = u16::from_be_bytes([payload[offset + 2], payload[offset + 3]]) as usize;
        if flowset_len < 4 {
            return changed;
        }
        let end = offset.saturating_add(flowset_len);
        if end > payload.len() {
            return changed;
        }
        let body = &payload[offset + 4..end];

        if flowset_id == 0 {
            changed |= observe_v9_data_templates(
                &exporter_ip,
                observation_domain_id,
                body,
                sampling,
                namespace,
            );
        }

        offset = end;
    }

    changed
}

fn observe_v9_data_templates(
    exporter_ip: &str,
    observation_domain_id: u32,
    body: &[u8],
    sampling: &mut SamplingState,
    namespace: &mut DecoderStateNamespace,
) -> bool {
    let mut cursor = body;
    let mut changed = false;
    while cursor.len() >= 4 {
        let template_id = u16::from_be_bytes([cursor[0], cursor[1]]);
        let field_count = u16::from_be_bytes([cursor[2], cursor[3]]) as usize;
        if field_count == 0 {
            cursor = &cursor[4..];
            continue;
        }

        let record_len = 4_usize.saturating_add(field_count.saturating_mul(4));
        if record_len > cursor.len() {
            return changed;
        }

        let mut fields = Vec::with_capacity(field_count);
        let mut persisted_fields = Vec::with_capacity(field_count);
        let mut field_cursor = &cursor[4..record_len];
        for _ in 0..field_count {
            let field_type = u16::from_be_bytes([field_cursor[0], field_cursor[1]]);
            let field_length = u16::from_be_bytes([field_cursor[2], field_cursor[3]]);
            fields.push(V9TemplateField {
                field_type,
                field_length: usize::from(field_length),
            });
            persisted_fields.push(PersistedV9TemplateField {
                field_type,
                field_length,
            });
            field_cursor = &field_cursor[4..];
        }

        sampling.set_v9_datalink_template(exporter_ip, observation_domain_id, template_id, fields);
        changed |= namespace.set_v9_template(template_id, persisted_fields);
        cursor = &cursor[record_len..];
    }

    changed
}

fn observe_ipfix_decoder_state_from_raw_payload(
    source: SocketAddr,
    payload: &[u8],
    sampling: &mut SamplingState,
    namespace: &mut DecoderStateNamespace,
) -> DecoderStateObservation {
    let template_state_changed =
        observe_ipfix_templates_from_raw_payload(source, payload, sampling, namespace);
    DecoderStateObservation {
        namespace_state_changed: template_state_changed,
        template_state_changed,
    }
}

fn observe_ipfix_templates_from_raw_payload(
    source: SocketAddr,
    payload: &[u8],
    sampling: &mut SamplingState,
    namespace: &mut DecoderStateNamespace,
) -> bool {
    if payload.len() < 16 {
        return false;
    }
    if u16::from_be_bytes([payload[0], payload[1]]) != 10 {
        return false;
    }

    let exporter_ip = source.ip().to_string();
    let observation_domain_id =
        u32::from_be_bytes([payload[12], payload[13], payload[14], payload[15]]);
    let packet_length = u16::from_be_bytes([payload[2], payload[3]]) as usize;
    let end_limit = payload.len().min(packet_length);
    let mut offset = 16_usize;
    let mut changed = false;

    while offset.saturating_add(4) <= end_limit {
        let flowset_id = u16::from_be_bytes([payload[offset], payload[offset + 1]]);
        let flowset_len = u16::from_be_bytes([payload[offset + 2], payload[offset + 3]]) as usize;
        if flowset_len < 4 {
            return changed;
        }
        let end = offset.saturating_add(flowset_len);
        if end > end_limit {
            return changed;
        }
        let body = &payload[offset + 4..end];

        if flowset_id == IPFIX_SET_ID_TEMPLATE {
            changed |= observe_ipfix_data_templates(
                &exporter_ip,
                observation_domain_id,
                body,
                sampling,
                namespace,
            );
        } else if flowset_id == 3 {
            changed |= observe_ipfix_options_templates(body, namespace);
        } else if flowset_id == 0 {
            changed |= observe_ipfix_v9_templates(body, namespace);
        } else if flowset_id == 1 {
            changed |= observe_ipfix_v9_options_templates(body, namespace);
        }

        offset = end;
    }

    changed
}

fn observe_ipfix_data_templates(
    exporter_ip: &str,
    observation_domain_id: u32,
    body: &[u8],
    sampling: &mut SamplingState,
    namespace: &mut DecoderStateNamespace,
) -> bool {
    let mut cursor = body;
    let mut changed = false;
    while cursor.len() >= 4 {
        let template_id = u16::from_be_bytes([cursor[0], cursor[1]]);
        let field_count = u16::from_be_bytes([cursor[2], cursor[3]]) as usize;
        cursor = &cursor[4..];

        let mut fields = Vec::with_capacity(field_count);
        let mut persisted_fields = Vec::with_capacity(field_count);
        for _ in 0..field_count {
            if cursor.len() < 4 {
                return changed;
            }

            let raw_type = u16::from_be_bytes([cursor[0], cursor[1]]);
            let field_length = u16::from_be_bytes([cursor[2], cursor[3]]);
            cursor = &cursor[4..];

            let pen_provided = (raw_type & 0x8000) != 0;
            let field_type = raw_type & 0x7fff;
            let enterprise_number = if pen_provided {
                if cursor.len() < 4 {
                    return changed;
                }
                let pen = u32::from_be_bytes([cursor[0], cursor[1], cursor[2], cursor[3]]);
                cursor = &cursor[4..];
                Some(pen)
            } else {
                None
            };

            fields.push(IPFixTemplateField {
                field_type,
                field_length,
                enterprise_number,
            });
            persisted_fields.push(PersistedIPFixTemplateField {
                field_type,
                field_length,
                enterprise_number,
            });
        }

        sampling.set_ipfix_datalink_template(
            exporter_ip,
            observation_domain_id,
            template_id,
            fields,
        );
        changed |= namespace.set_ipfix_template(template_id, persisted_fields);
    }

    changed
}

fn decode_ipfix_special_from_raw_payload(
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
    let exporter_ip = source.ip().to_string();
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
            && let Some(template) = sampling.get_ipfix_datalink_template(
                &exporter_ip,
                observation_domain_id,
                flowset_id,
            )
        {
            let mut cursor = body;
            while !cursor.is_empty() {
                let Some((record_values, consumed)) =
                    parse_ipfix_record_from_template(cursor, &template.fields)
                else {
                    break;
                };
                if let Some(flow) = decode_ipfix_special_record(
                    source,
                    timestamp_source,
                    input_realtime_usec,
                    packet_realtime_usec,
                    sampling,
                    &exporter_ip,
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

fn decode_v9_special_from_raw_payload(
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
    let exporter_ip = source.ip().to_string();
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
                sampling.get_v9_datalink_template(&exporter_ip, observation_domain_id, flowset_id)
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
                    &exporter_ip,
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

fn parse_v9_record_from_template<'a>(
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

fn decode_v9_special_record(
    source: SocketAddr,
    timestamp_source: TimestampSource,
    input_realtime_usec: u64,
    packet_realtime_usec: Option<u64>,
    export_time_seconds: u64,
    sys_uptime_millis: u64,
    sampling: &SamplingState,
    exporter_ip: &str,
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
            V9Field::FlowSamplerId => {
                sampler_id = value.parse::<u64>().ok();
            }
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

fn parse_ipfix_record_from_template<'a>(
    body: &'a [u8],
    fields: &[IPFixTemplateField],
) -> Option<(Vec<&'a [u8]>, usize)> {
    let mut consumed = 0_usize;
    let mut values = Vec::with_capacity(fields.len());

    for field in fields {
        let value_len = if field.field_length == u16::MAX {
            if consumed >= body.len() {
                return None;
            }
            let first = body[consumed] as usize;
            consumed = consumed.saturating_add(1);
            if first < 255 {
                first
            } else {
                if consumed.saturating_add(2) > body.len() {
                    return None;
                }
                let extended = u16::from_be_bytes([body[consumed], body[consumed + 1]]) as usize;
                consumed = consumed.saturating_add(2);
                extended
            }
        } else {
            field.field_length as usize
        };

        if consumed.saturating_add(value_len) > body.len() {
            return None;
        }
        values.push(&body[consumed..consumed + value_len]);
        consumed = consumed.saturating_add(value_len);
    }

    Some((values, consumed))
}

fn decode_ipfix_special_record(
    source: SocketAddr,
    timestamp_source: TimestampSource,
    input_realtime_usec: u64,
    packet_realtime_usec: Option<u64>,
    sampling: &SamplingState,
    exporter_ip: &str,
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
                let status = if decode_akvorado_unsigned(raw_value) & 0x03ff == 0 {
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
                fields.insert("BYTES", decode_akvorado_unsigned(raw_value).to_string());
            }
            IPFIX_FIELD_PACKET_DELTA_COUNT => {
                fields.insert("PACKETS", decode_akvorado_unsigned(raw_value).to_string());
            }
            IPFIX_FIELD_PROTOCOL_IDENTIFIER => {
                fields.insert("PROTOCOL", decode_akvorado_unsigned(raw_value).to_string());
            }
            IPFIX_FIELD_SAMPLER_ID | IPFIX_FIELD_SELECTOR_ID => {
                sampler_id = Some(decode_akvorado_unsigned(raw_value));
            }
            IPFIX_FIELD_SAMPLING_INTERVAL | IPFIX_FIELD_SAMPLER_RANDOM_INTERVAL => {
                observed_sampling_rate = Some(decode_akvorado_unsigned(raw_value));
            }
            IPFIX_FIELD_SAMPLING_PACKET_INTERVAL => {
                sampling_packet_interval = Some(decode_akvorado_unsigned(raw_value));
            }
            IPFIX_FIELD_SAMPLING_PACKET_SPACE => {
                sampling_packet_space = Some(decode_akvorado_unsigned(raw_value));
            }
            IPFIX_FIELD_SOURCE_TRANSPORT_PORT => {
                fields.insert("SRC_PORT", decode_akvorado_unsigned(raw_value).to_string());
            }
            IPFIX_FIELD_DESTINATION_TRANSPORT_PORT => {
                fields.insert("DST_PORT", decode_akvorado_unsigned(raw_value).to_string());
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
                if let Some(etype) =
                    etype_from_ip_version(&decode_akvorado_unsigned(raw_value).to_string())
                {
                    fields.insert("ETYPE", etype.to_string());
                }
            }
            IPFIX_FIELD_INPUT_SNMP => {
                fields.insert("IN_IF", decode_akvorado_unsigned(raw_value).to_string());
            }
            IPFIX_FIELD_OUTPUT_SNMP => {
                fields.insert("OUT_IF", decode_akvorado_unsigned(raw_value).to_string());
            }
            IPFIX_FIELD_DIRECTION => {
                fields.insert("DIRECTION", decode_akvorado_unsigned(raw_value).to_string());
            }
            IPFIX_FIELD_FORWARDING_STATUS => {
                fields.insert(
                    "FORWARDING_STATUS",
                    decode_akvorado_unsigned(raw_value).to_string(),
                );
            }
            IPFIX_FIELD_FLOW_START_MILLISECONDS => {
                let value = decode_akvorado_unsigned(raw_value);
                flow_start_usec = Some(value.saturating_mul(USEC_PER_MILLISECOND));
                fields.insert(
                    "FLOW_START_USEC",
                    value.saturating_mul(USEC_PER_MILLISECOND).to_string(),
                );
            }
            IPFIX_FIELD_FLOW_END_MILLISECONDS => {
                fields.insert(
                    "FLOW_END_USEC",
                    decode_akvorado_unsigned(raw_value)
                        .saturating_mul(USEC_PER_MILLISECOND)
                        .to_string(),
                );
            }
            IPFIX_FIELD_MINIMUM_TTL | IPFIX_FIELD_MAXIMUM_TTL => {
                fields
                    .entry("IPTTL")
                    .or_insert_with(|| decode_akvorado_unsigned(raw_value).to_string());
            }
            field_type if is_ipfix_mpls_label_field(field_type) => {
                let label = decode_akvorado_unsigned(raw_value) >> 4;
                if label > 0 {
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

fn parse_datalink_frame_section(
    data: &[u8],
    fields: &mut FlowFields,
    decapsulation_mode: DecapsulationMode,
) -> Option<u64> {
    if data.len() < 14 {
        return None;
    }

    fields.insert("DST_MAC", mac_to_string(&data[0..6]));
    fields.insert("SRC_MAC", mac_to_string(&data[6..12]));

    let mut etype = u16::from_be_bytes([data[12], data[13]]);
    let mut cursor = &data[14..];

    while etype == ETYPE_VLAN {
        if cursor.len() < 4 {
            return None;
        }
        let vlan = ((u16::from(cursor[0] & 0x0f)) << 8) | u16::from(cursor[1]);
        if vlan > 0 && !field_present_in_map(fields, "SRC_VLAN") {
            fields.insert("SRC_VLAN", vlan.to_string());
        }
        etype = u16::from_be_bytes([cursor[2], cursor[3]]);
        cursor = &cursor[4..];
    }

    if etype == ETYPE_MPLS_UNICAST {
        let mut labels = Vec::new();
        loop {
            if cursor.len() < 4 {
                return None;
            }
            let raw =
                (u32::from(cursor[0]) << 16) | (u32::from(cursor[1]) << 8) | u32::from(cursor[2]);
            let label = raw >> 4;
            let bottom = cursor[2] & 0x01;
            cursor = &cursor[4..];
            if label > 0 {
                labels.push(label.to_string());
            }
            if bottom == 1 || label <= 15 {
                if cursor.is_empty() {
                    return None;
                }
                etype = match (cursor[0] & 0xf0) >> 4 {
                    4 => 0x0800,
                    6 => 0x86dd,
                    _ => return None,
                };
                break;
            }
        }
        if !labels.is_empty() {
            fields.insert("MPLS_LABELS", labels.join(","));
        }
    }

    match etype {
        0x0800 => parse_ipv4_packet(cursor, fields, decapsulation_mode),
        0x86dd => parse_ipv6_packet(cursor, fields, decapsulation_mode),
        _ => None,
    }
}

fn parse_ipv4_packet(
    data: &[u8],
    fields: &mut FlowFields,
    decapsulation_mode: DecapsulationMode,
) -> Option<u64> {
    if data.len() < 20 {
        return None;
    }
    let ihl = ((data[0] & 0x0f) as usize).saturating_mul(4);
    if ihl < 20 || ihl > data.len() {
        return None;
    }

    let total_length = u16::from_be_bytes([data[2], data[3]]) as u64;
    let fragment_id = u16::from_be_bytes([data[4], data[5]]);
    let fragment_offset = u16::from_be_bytes([data[6], data[7]]) & 0x1fff;
    let proto = data[9];
    let src = Ipv4Addr::new(data[12], data[13], data[14], data[15]);
    let dst = Ipv4Addr::new(data[16], data[17], data[18], data[19]);

    if decapsulation_mode.is_none() {
        fields.insert("ETYPE", ETYPE_IPV4.to_string());
        fields.insert("SRC_ADDR", src.to_string());
        fields.insert("DST_ADDR", dst.to_string());
        fields.insert("PROTOCOL", proto.to_string());
        fields.insert("IPTOS", data[1].to_string());
        fields.insert("IPTTL", data[8].to_string());
        fields.insert("IP_FRAGMENT_ID", fragment_id.to_string());
        fields.insert("IP_FRAGMENT_OFFSET", fragment_offset.to_string());
    }

    if fragment_offset == 0 {
        let inner_l3_length = parse_transport(proto, &data[ihl..], fields, decapsulation_mode);
        if decapsulation_mode.is_none() {
            return Some(total_length);
        }
        return if inner_l3_length > 0 {
            Some(inner_l3_length)
        } else {
            None
        };
    }

    if decapsulation_mode.is_none() {
        Some(total_length)
    } else {
        None
    }
}

fn parse_ipv6_packet(
    data: &[u8],
    fields: &mut FlowFields,
    decapsulation_mode: DecapsulationMode,
) -> Option<u64> {
    if data.len() < 40 {
        return None;
    }

    let payload_length = u16::from_be_bytes([data[4], data[5]]) as u64;
    let next_header = data[6];
    let hop_limit = data[7];
    let mut src_bytes = [0_u8; 16];
    let mut dst_bytes = [0_u8; 16];
    src_bytes.copy_from_slice(&data[8..24]);
    dst_bytes.copy_from_slice(&data[24..40]);
    let src = Ipv6Addr::from(src_bytes);
    let dst = Ipv6Addr::from(dst_bytes);

    if decapsulation_mode.is_none() {
        let traffic_class = (u16::from_be_bytes([data[0], data[1]]) & 0x0ff0) >> 4;
        let flow_label = u32::from_be_bytes([data[0], data[1], data[2], data[3]]) & 0x000f_ffff;

        fields.insert("ETYPE", ETYPE_IPV6.to_string());
        fields.insert("SRC_ADDR", src.to_string());
        fields.insert("DST_ADDR", dst.to_string());
        fields.insert("PROTOCOL", next_header.to_string());
        fields.insert("IPTOS", traffic_class.to_string());
        fields.insert("IPTTL", hop_limit.to_string());
        fields.insert("IPV6_FLOW_LABEL", flow_label.to_string());
    }
    let inner_l3_length = parse_transport(next_header, &data[40..], fields, decapsulation_mode);
    if decapsulation_mode.is_none() {
        Some(payload_length.saturating_add(40))
    } else if inner_l3_length > 0 {
        Some(inner_l3_length)
    } else {
        None
    }
}

fn parse_srv6_inner(proto: u8, data: &[u8], fields: &mut FlowFields) -> Option<u64> {
    let mut next = proto;
    let mut cursor = data;

    loop {
        match next {
            4 => return parse_ipv4_packet(cursor, fields, DecapsulationMode::None),
            41 => return parse_ipv6_packet(cursor, fields, DecapsulationMode::None),
            43 => {
                if cursor.len() < 8 || cursor[2] != 4 {
                    return None;
                }
                let skip = 8_usize.saturating_add((cursor[1] as usize).saturating_mul(8));
                if cursor.len() < skip {
                    return None;
                }
                next = cursor[0];
                cursor = &cursor[skip..];
            }
            _ => return None,
        }
    }
}

fn parse_transport(
    proto: u8,
    data: &[u8],
    fields: &mut FlowFields,
    decapsulation_mode: DecapsulationMode,
) -> u64 {
    if !decapsulation_mode.is_none() {
        return match decapsulation_mode {
            DecapsulationMode::Vxlan => {
                if proto == 17
                    && data.len() > 16
                    && u16::from_be_bytes([data[2], data[3]]) == VXLAN_UDP_PORT
                {
                    parse_datalink_frame_section(&data[16..], fields, DecapsulationMode::None)
                        .unwrap_or(0)
                } else {
                    0
                }
            }
            DecapsulationMode::Srv6 => parse_srv6_inner(proto, data, fields).unwrap_or(0),
            DecapsulationMode::None => 0,
        };
    }

    match proto {
        6 | 17 => {
            if data.len() >= 4 {
                fields.insert(
                    "SRC_PORT",
                    u16::from_be_bytes([data[0], data[1]]).to_string(),
                );
                fields.insert(
                    "DST_PORT",
                    u16::from_be_bytes([data[2], data[3]]).to_string(),
                );
            }
            if proto == 6 && data.len() >= 14 {
                fields.insert("TCP_FLAGS", data[13].to_string());
            }
        }
        1 => {
            if data.len() >= 2 {
                fields.insert("ICMPV4_TYPE", data[0].to_string());
                fields.insert("ICMPV4_CODE", data[1].to_string());
            }
        }
        58 => {
            if data.len() >= 2 {
                fields.insert("ICMPV6_TYPE", data[0].to_string());
                fields.insert("ICMPV6_CODE", data[1].to_string());
            }
        }
        _ => {}
    }

    0
}

fn mac_to_string(bytes: &[u8]) -> String {
    if bytes.len() != 6 {
        return String::new();
    }
    format!(
        "{:02x}:{:02x}:{:02x}:{:02x}:{:02x}:{:02x}",
        bytes[0], bytes[1], bytes[2], bytes[3], bytes[4], bytes[5]
    )
}

fn observe_v9_sampling_templates(
    exporter_ip: &str,
    observation_domain_id: u32,
    body: &[u8],
    sampling: &mut SamplingState,
    namespace: &mut DecoderStateNamespace,
) -> bool {
    let mut cursor = body;
    let mut changed = false;
    while cursor.len() >= 6 {
        let template_id = u16::from_be_bytes([cursor[0], cursor[1]]);
        let scope_length = u16::from_be_bytes([cursor[2], cursor[3]]) as usize;
        let option_length = u16::from_be_bytes([cursor[4], cursor[5]]) as usize;
        let scope_count = scope_length / 4;
        let option_count = option_length / 4;
        let fields_block_len = scope_count.saturating_add(option_count).saturating_mul(4);
        let record_len = 6_usize.saturating_add(fields_block_len);
        if record_len > cursor.len() {
            return changed;
        }

        let mut fields = &cursor[6..record_len];
        let mut scope_fields = Vec::with_capacity(scope_count);
        let mut option_fields = Vec::with_capacity(option_count);
        let mut persisted_scope_fields = Vec::with_capacity(scope_count);
        let mut persisted_option_fields = Vec::with_capacity(option_count);

        for _ in 0..scope_count {
            let field_type = u16::from_be_bytes([fields[0], fields[1]]);
            let field_length = u16::from_be_bytes([fields[2], fields[3]]);
            scope_fields.push(V9TemplateField {
                field_type,
                field_length: usize::from(field_length),
            });
            persisted_scope_fields.push(PersistedV9TemplateField {
                field_type,
                field_length,
            });
            fields = &fields[4..];
        }
        for _ in 0..option_count {
            let field_type = u16::from_be_bytes([fields[0], fields[1]]);
            let field_length = u16::from_be_bytes([fields[2], fields[3]]);
            option_fields.push(V9TemplateField {
                field_type,
                field_length: usize::from(field_length),
            });
            persisted_option_fields.push(PersistedV9TemplateField {
                field_type,
                field_length,
            });
            fields = &fields[4..];
        }

        sampling.set_v9_sampling_template(
            exporter_ip,
            observation_domain_id,
            template_id,
            scope_fields,
            option_fields,
        );
        changed |= namespace.set_v9_options_template(
            template_id,
            persisted_scope_fields,
            persisted_option_fields,
        );
        cursor = &cursor[record_len..];
    }

    changed
}

fn observe_v9_sampling_data(
    exporter_ip: &str,
    observation_domain_id: u32,
    template: &V9SamplingTemplate,
    body: &[u8],
    sampling: &mut SamplingState,
    namespace: &mut DecoderStateNamespace,
) -> bool {
    if template.record_length == 0 {
        return false;
    }

    let mut cursor = body;
    let mut changed = false;
    while cursor.len() >= template.record_length {
        let mut record = &cursor[..template.record_length];
        let mut sampler_id = 0_u64;
        let mut rate = 0_u64;

        for field in template
            .scope_fields
            .iter()
            .chain(template.option_fields.iter())
        {
            if field.field_length > record.len() {
                return changed;
            }
            let raw = &record[..field.field_length];
            record = &record[field.field_length..];

            match field.field_type {
                48 => {
                    sampler_id = decode_akvorado_unsigned(raw);
                }
                34 | 50 => {
                    let parsed = decode_akvorado_unsigned(raw);
                    if parsed > 0 {
                        rate = parsed;
                    }
                }
                _ => {}
            }
        }

        if rate > 0 {
            sampling.set(exporter_ip, 9, observation_domain_id, sampler_id, rate);
            changed |= namespace.set_sampling_rate(9, sampler_id, rate);
        }
        cursor = &cursor[template.record_length..];
    }

    changed
}

fn observe_ipfix_options_templates(body: &[u8], namespace: &mut DecoderStateNamespace) -> bool {
    let mut cursor = body;
    let mut changed = false;
    while cursor.len() >= 6 {
        let template_id = u16::from_be_bytes([cursor[0], cursor[1]]);
        let field_count = u16::from_be_bytes([cursor[2], cursor[3]]) as usize;
        let scope_field_count = u16::from_be_bytes([cursor[4], cursor[5]]);
        cursor = &cursor[6..];

        let mut fields = Vec::with_capacity(field_count);
        for _ in 0..field_count {
            if cursor.len() < 4 {
                return changed;
            }

            let raw_type = u16::from_be_bytes([cursor[0], cursor[1]]);
            let field_length = u16::from_be_bytes([cursor[2], cursor[3]]);
            cursor = &cursor[4..];

            let pen_provided = (raw_type & 0x8000) != 0;
            let field_type = raw_type & 0x7fff;
            let enterprise_number = if pen_provided {
                if cursor.len() < 4 {
                    return changed;
                }
                let pen = u32::from_be_bytes([cursor[0], cursor[1], cursor[2], cursor[3]]);
                cursor = &cursor[4..];
                Some(pen)
            } else {
                None
            };

            fields.push(PersistedIPFixTemplateField {
                field_type,
                field_length,
                enterprise_number,
            });
        }

        changed |= namespace.set_ipfix_options_template(template_id, scope_field_count, fields);
    }

    changed
}

fn observe_ipfix_v9_templates(body: &[u8], namespace: &mut DecoderStateNamespace) -> bool {
    let mut cursor = body;
    let mut changed = false;
    while cursor.len() >= 4 {
        let template_id = u16::from_be_bytes([cursor[0], cursor[1]]);
        let field_count = u16::from_be_bytes([cursor[2], cursor[3]]) as usize;
        if field_count == 0 {
            cursor = &cursor[4..];
            continue;
        }

        let record_len = 4_usize.saturating_add(field_count.saturating_mul(4));
        if record_len > cursor.len() {
            return changed;
        }

        let mut field_cursor = &cursor[4..record_len];
        let mut fields = Vec::with_capacity(field_count);
        for _ in 0..field_count {
            let field_type = u16::from_be_bytes([field_cursor[0], field_cursor[1]]);
            let field_length = u16::from_be_bytes([field_cursor[2], field_cursor[3]]);
            fields.push(PersistedV9TemplateField {
                field_type,
                field_length,
            });
            field_cursor = &field_cursor[4..];
        }

        changed |= namespace.set_ipfix_v9_template(template_id, fields);
        cursor = &cursor[record_len..];
    }

    changed
}

fn observe_ipfix_v9_options_templates(body: &[u8], namespace: &mut DecoderStateNamespace) -> bool {
    let mut cursor = body;
    let mut changed = false;
    while cursor.len() >= 6 {
        let template_id = u16::from_be_bytes([cursor[0], cursor[1]]);
        let scope_length = u16::from_be_bytes([cursor[2], cursor[3]]) as usize;
        let option_length = u16::from_be_bytes([cursor[4], cursor[5]]) as usize;
        let scope_count = scope_length / 4;
        let option_count = option_length / 4;
        let fields_block_len = scope_count.saturating_add(option_count).saturating_mul(4);
        let record_len = 6_usize.saturating_add(fields_block_len);
        if record_len > cursor.len() {
            return changed;
        }

        let mut fields = &cursor[6..record_len];
        let mut scope_fields = Vec::with_capacity(scope_count);
        let mut option_fields = Vec::with_capacity(option_count);

        for _ in 0..scope_count {
            scope_fields.push(PersistedV9TemplateField {
                field_type: u16::from_be_bytes([fields[0], fields[1]]),
                field_length: u16::from_be_bytes([fields[2], fields[3]]),
            });
            fields = &fields[4..];
        }
        for _ in 0..option_count {
            option_fields.push(PersistedV9TemplateField {
                field_type: u16::from_be_bytes([fields[0], fields[1]]),
                field_length: u16::from_be_bytes([fields[2], fields[3]]),
            });
            fields = &fields[4..];
        }

        changed |=
            namespace.set_ipfix_v9_options_template(template_id, scope_fields, option_fields);
        cursor = &cursor[record_len..];
    }

    changed
}

fn decode_akvorado_unsigned(bytes: &[u8]) -> u64 {
    match bytes.len() {
        1 => u64::from(bytes[0]),
        2 => u64::from(bytes[1]) | (u64::from(bytes[0]) << 8),
        3 => u64::from(bytes[2]) | (u64::from(bytes[1]) << 8) | (u64::from(bytes[0]) << 16),
        4 => {
            u64::from(bytes[3])
                | (u64::from(bytes[2]) << 8)
                | (u64::from(bytes[1]) << 16)
                | (u64::from(bytes[0]) << 24)
        }
        5 => {
            u64::from(bytes[4])
                | (u64::from(bytes[3]) << 8)
                | (u64::from(bytes[2]) << 16)
                | (u64::from(bytes[1]) << 24)
                | (u64::from(bytes[0]) << 32)
        }
        6 => {
            u64::from(bytes[5])
                | (u64::from(bytes[4]) << 8)
                | (u64::from(bytes[3]) << 16)
                | (u64::from(bytes[2]) << 24)
                | (u64::from(bytes[1]) << 32)
                | (u64::from(bytes[0]) << 40)
        }
        7 => {
            u64::from(bytes[6])
                | (u64::from(bytes[5]) << 8)
                | (u64::from(bytes[4]) << 16)
                | (u64::from(bytes[3]) << 24)
                | (u64::from(bytes[2]) << 32)
                | (u64::from(bytes[1]) << 40)
                | (u64::from(bytes[0]) << 48)
        }
        8 => u64::from_be_bytes([
            bytes[0], bytes[1], bytes[2], bytes[3], bytes[4], bytes[5], bytes[6], bytes[7],
        ]),
        _ => 0,
    }
}

fn is_template_error(message: &str) -> bool {
    let msg = message.to_ascii_lowercase();
    msg.contains("template") && msg.contains("not found")
}

fn append_v5_records(
    source: SocketAddr,
    out: &mut Vec<DecodedFlow>,
    packet: V5,
    timestamp_source: TimestampSource,
    input_realtime_usec: u64,
) {
    let export_usec = unix_timestamp_to_usec(
        packet.header.unix_secs as u64,
        packet.header.unix_nsecs as u64,
    );
    let sampling = decode_sampling_interval(packet.header.sampling_interval);
    let boot_millis = (packet.header.unix_secs as u64)
        .saturating_mul(1000)
        .saturating_sub(packet.header.sys_up_time as u64);

    for flow in packet.flowsets {
        let flow_start_usec = boot_millis
            .saturating_add(flow.first as u64)
            .saturating_mul(1000);
        let flow_end_usec = boot_millis
            .saturating_add(flow.last as u64)
            .saturating_mul(1000);

        let mut rec = base_record("v5", source);
        rec.src_addr = Some(IpAddr::V4(flow.src_addr));
        rec.dst_addr = Some(IpAddr::V4(flow.dst_addr));
        rec.src_prefix = Some(IpAddr::V4(flow.src_addr));
        rec.dst_prefix = Some(IpAddr::V4(flow.dst_addr));
        rec.src_mask = flow.src_mask;
        rec.dst_mask = flow.dst_mask;
        rec.src_port = flow.src_port;
        rec.dst_port = flow.dst_port;
        rec.protocol = flow.protocol_number;
        rec.src_as = flow.src_as as u32;
        rec.dst_as = flow.dst_as as u32;
        rec.in_if = flow.input as u32;
        rec.out_if = flow.output as u32;
        rec.next_hop = Some(IpAddr::V4(flow.next_hop));
        rec.set_etype(2048); // IPv4
        rec.set_iptos(flow.tos);
        rec.set_tcp_flags(flow.tcp_flags);
        rec.bytes = flow.d_octets as u64;
        rec.packets = flow.d_pkts as u64;
        rec.flows = 1;
        rec.raw_bytes = flow.d_octets as u64;
        rec.raw_packets = flow.d_pkts as u64;
        rec.set_sampling_rate(sampling as u64);
        finalize_record(&mut rec);

        out.push(DecodedFlow {
            record: rec,
            source_realtime_usec: timestamp_source.select(
                input_realtime_usec,
                Some(export_usec),
                Some(if flow_start_usec > 0 {
                    flow_start_usec
                } else {
                    flow_end_usec
                }),
            ),
        });
    }
}

fn append_v7_records(
    source: SocketAddr,
    out: &mut Vec<DecodedFlow>,
    packet: V7,
    timestamp_source: TimestampSource,
    input_realtime_usec: u64,
) {
    let export_usec = unix_timestamp_to_usec(
        packet.header.unix_secs as u64,
        packet.header.unix_nsecs as u64,
    );
    let boot_millis = (packet.header.unix_secs as u64)
        .saturating_mul(1000)
        .saturating_sub(packet.header.sys_up_time as u64);

    for flow in packet.flowsets {
        let flow_start_usec = boot_millis
            .saturating_add(flow.first as u64)
            .saturating_mul(1000);
        let flow_end_usec = boot_millis
            .saturating_add(flow.last as u64)
            .saturating_mul(1000);

        let mut rec = base_record("v7", source);
        rec.src_addr = Some(IpAddr::V4(flow.src_addr));
        rec.dst_addr = Some(IpAddr::V4(flow.dst_addr));
        rec.src_prefix = Some(IpAddr::V4(flow.src_addr));
        rec.dst_prefix = Some(IpAddr::V4(flow.dst_addr));
        rec.src_mask = flow.src_mask;
        rec.dst_mask = flow.dst_mask;
        rec.src_port = flow.src_port;
        rec.dst_port = flow.dst_port;
        rec.protocol = flow.protocol_number;
        rec.src_as = flow.src_as as u32;
        rec.dst_as = flow.dst_as as u32;
        rec.in_if = flow.input as u32;
        rec.out_if = flow.output as u32;
        rec.next_hop = Some(IpAddr::V4(flow.next_hop));
        rec.set_etype(2048); // IPv4
        rec.set_iptos(flow.tos);
        rec.set_tcp_flags(flow.tcp_flags);
        rec.bytes = flow.d_octets as u64;
        rec.packets = flow.d_pkts as u64;
        rec.flows = 1;
        rec.raw_bytes = flow.d_octets as u64;
        rec.raw_packets = flow.d_pkts as u64;
        // V7 has no sampling_interval in header (unlike V5)
        finalize_record(&mut rec);

        out.push(DecodedFlow {
            record: rec,
            source_realtime_usec: timestamp_source.select(
                input_realtime_usec,
                Some(export_usec),
                Some(if flow_start_usec > 0 {
                    flow_start_usec
                } else {
                    flow_end_usec
                }),
            ),
        });
    }
}

fn append_v9_records(
    source: SocketAddr,
    out: &mut Vec<DecodedFlow>,
    packet: V9,
    sampling: &mut SamplingState,
    decapsulation_mode: DecapsulationMode,
    timestamp_source: TimestampSource,
    input_realtime_usec: u64,
) {
    let export_usec = unix_timestamp_to_usec(packet.header.unix_secs as u64, 0);
    let exporter_ip = source.ip().to_string();
    let observation_domain_id = packet.header.source_id;
    let version = 9_u16;

    for flowset in packet.flowsets {
        match flowset.body {
            V9FlowSetBody::Data(data) => {
                for record in data.fields {
                    let mut rec = base_record("v9", source);
                    let mut sampler_id: Option<u64> = None;
                    let mut observed_sampling_rate: Option<u64> = None;
                    let mut first_switched_millis: Option<u64> = None;
                    let mut last_switched_millis: Option<u64> = None;
                    let system_init_usec = netflow_v9_system_init_usec(
                        packet.header.unix_secs as u64,
                        packet.header.sys_up_time as u64,
                    );

                    for (field, value) in record {
                        let value_str = field_value_to_string(&value);
                        apply_v9_special_mappings_record(&mut rec, field, &value_str);
                        match field {
                            V9Field::FlowSamplerId => {
                                sampler_id = value_str.parse::<u64>().ok();
                            }
                            V9Field::SamplingInterval | V9Field::FlowSamplerRandomInterval => {
                                observed_sampling_rate = value_str.parse::<u64>().ok();
                            }
                            V9Field::FirstSwitched => {
                                first_switched_millis = value_str.parse::<u64>().ok();
                            }
                            V9Field::LastSwitched => {
                                last_switched_millis = value_str.parse::<u64>().ok();
                            }
                            _ => {}
                        }
                        if let Some(canonical) = v9_canonical_key(field) {
                            if should_skip_zero_ip(canonical, &value_str) {
                                continue;
                            }
                            // IpProtocolVersion is fully handled by special mappings
                            // (raw "6" → etype 34525). Skip to avoid overwriting.
                            if matches!(field, V9Field::IpProtocolVersion) {
                                continue;
                            }
                            set_record_field(&mut rec, canonical, &value_str);
                        }
                    }

                    apply_sampling_state_record(
                        &mut rec,
                        &exporter_ip,
                        version,
                        observation_domain_id,
                        sampler_id,
                        observed_sampling_rate,
                        sampling,
                    );

                    if looks_like_sampling_option_record_from_rec(&rec, observed_sampling_rate) {
                        continue;
                    }
                    if !decapsulation_mode.is_none() {
                        continue;
                    }

                    if rec.flows == 0 {
                        rec.flows = 1;
                    }
                    rec.flow_start_usec = first_switched_millis
                        .map(|value| {
                            netflow_v9_uptime_millis_to_absolute_usec(system_init_usec, value)
                        })
                        .unwrap_or(0);
                    rec.flow_end_usec = last_switched_millis
                        .map(|value| {
                            netflow_v9_uptime_millis_to_absolute_usec(system_init_usec, value)
                        })
                        .unwrap_or(0);
                    finalize_record(&mut rec);
                    let first_switched_usec =
                        (rec.flow_start_usec != 0).then_some(rec.flow_start_usec);
                    out.push(DecodedFlow {
                        record: rec,
                        source_realtime_usec: timestamp_source.select(
                            input_realtime_usec,
                            Some(export_usec),
                            first_switched_usec,
                        ),
                    });
                }
            }
            V9FlowSetBody::OptionsData(options_data) => {
                observe_v9_sampling_options(
                    &exporter_ip,
                    version,
                    observation_domain_id,
                    sampling,
                    options_data,
                );
            }
            _ => {
                continue;
            }
        }
    }
}

#[derive(Default)]
struct IPFixRecordBuildState {
    reverse_overrides: FlowFields,
    reverse_present: bool,
    decap_ok: bool,
    sampler_id: Option<u64>,
    observed_sampling_rate: Option<u64>,
    sampling_packet_interval: Option<u64>,
    sampling_packet_space: Option<u64>,
    system_init_millis: Option<u64>,
    flow_start_seconds: Option<u64>,
    flow_end_seconds: Option<u64>,
    flow_start_millis: Option<u64>,
    flow_end_millis: Option<u64>,
    flow_start_micros: Option<u64>,
    flow_end_micros: Option<u64>,
    flow_start_nanos: Option<u64>,
    flow_end_nanos: Option<u64>,
    flow_start_delta_micros: Option<u64>,
    flow_end_delta_micros: Option<u64>,
    flow_start_sysuptime_millis: Option<u64>,
    flow_end_sysuptime_millis: Option<u64>,
    reverse_flow_start_usec: Option<u64>,
    reverse_flow_end_usec: Option<u64>,
}

impl IPFixRecordBuildState {
    fn apply_sampling_packet_ratio(&mut self) {
        if let (Some(interval), Some(space)) =
            (self.sampling_packet_interval, self.sampling_packet_space)
            && interval > 0
        {
            self.observed_sampling_rate = Some((interval.saturating_add(space)) / interval);
        }
    }

    fn resolve_flow_times(&self, rec: &mut FlowRecord, export_usec: u64) {
        rec.flow_start_usec = resolve_ipfix_time_usec(
            self.flow_start_seconds,
            self.flow_start_millis,
            self.flow_start_micros,
            self.flow_start_nanos,
            self.flow_start_delta_micros,
            self.flow_start_sysuptime_millis,
            self.system_init_millis,
            export_usec,
        )
        .unwrap_or(0);
        rec.flow_end_usec = resolve_ipfix_time_usec(
            self.flow_end_seconds,
            self.flow_end_millis,
            self.flow_end_micros,
            self.flow_end_nanos,
            self.flow_end_delta_micros,
            self.flow_end_sysuptime_millis,
            self.system_init_millis,
            export_usec,
        )
        .unwrap_or(0);
    }

    fn apply_reverse_time_overrides(&mut self) {
        if let Some(start_usec) = self.reverse_flow_start_usec {
            self.reverse_overrides
                .insert("FLOW_START_USEC", start_usec.to_string());
        }
        if let Some(end_usec) = self.reverse_flow_end_usec {
            self.reverse_overrides
                .insert("FLOW_END_USEC", end_usec.to_string());
        }
    }
}

fn track_reverse_ipfix_time(
    state: &mut IPFixRecordBuildState,
    reverse_field: &ReverseInformationElement,
    value: &FieldValue,
    export_usec: u64,
) {
    let Some(usec) =
        reverse_ipfix_timestamp_to_usec(reverse_field, value, export_usec, state.system_init_millis)
    else {
        return;
    };

    match reverse_field {
        ReverseInformationElement::ReverseFlowStartSeconds
        | ReverseInformationElement::ReverseFlowStartMilliseconds
        | ReverseInformationElement::ReverseFlowStartMicroseconds
        | ReverseInformationElement::ReverseFlowStartNanoseconds
        | ReverseInformationElement::ReverseFlowStartDeltaMicroseconds
        | ReverseInformationElement::ReverseFlowStartSysUpTime
        | ReverseInformationElement::ReverseMinFlowStartMicroseconds
        | ReverseInformationElement::ReverseMinFlowStartNanoseconds => {
            state.reverse_flow_start_usec = Some(usec);
        }
        ReverseInformationElement::ReverseFlowEndSeconds
        | ReverseInformationElement::ReverseFlowEndMilliseconds
        | ReverseInformationElement::ReverseFlowEndMicroseconds
        | ReverseInformationElement::ReverseFlowEndNanoseconds
        | ReverseInformationElement::ReverseFlowEndDeltaMicroseconds
        | ReverseInformationElement::ReverseFlowEndSysUpTime
        | ReverseInformationElement::ReverseMaxFlowEndMicroseconds
        | ReverseInformationElement::ReverseMaxFlowEndNanoseconds => {
            state.reverse_flow_end_usec = Some(usec);
        }
        _ => {}
    }
}

fn observe_ipfix_record_value(
    state: &mut IPFixRecordBuildState,
    field: &IPFixField,
    value: &FieldValue,
    value_str: &str,
) {
    match field {
        IPFixField::IANA(IANAIPFixField::SamplerId)
        | IPFixField::IANA(IANAIPFixField::SelectorId) => {
            state.sampler_id = value_str.parse::<u64>().ok();
        }
        IPFixField::IANA(IANAIPFixField::SamplingInterval)
        | IPFixField::IANA(IANAIPFixField::SamplerRandomInterval) => {
            state.observed_sampling_rate = value_str.parse::<u64>().ok();
        }
        IPFixField::IANA(IANAIPFixField::SamplingPacketInterval) => {
            state.sampling_packet_interval = value_str.parse::<u64>().ok();
        }
        IPFixField::IANA(IANAIPFixField::SamplingPacketSpace) => {
            state.sampling_packet_space = value_str.parse::<u64>().ok();
        }
        IPFixField::IANA(IANAIPFixField::FlowStartSeconds) => {
            state.flow_start_seconds = value_str.parse::<u64>().ok();
        }
        IPFixField::IANA(IANAIPFixField::FlowEndSeconds) => {
            state.flow_end_seconds = value_str.parse::<u64>().ok();
        }
        IPFixField::IANA(IANAIPFixField::FlowStartMilliseconds)
        | IPFixField::IANA(IANAIPFixField::MinFlowStartMilliseconds) => {
            state.flow_start_millis = value_str.parse::<u64>().ok();
        }
        IPFixField::IANA(IANAIPFixField::FlowEndMilliseconds)
        | IPFixField::IANA(IANAIPFixField::MaxFlowEndMilliseconds) => {
            state.flow_end_millis = value_str.parse::<u64>().ok();
        }
        IPFixField::IANA(IANAIPFixField::FlowStartMicroseconds)
        | IPFixField::IANA(IANAIPFixField::MinFlowStartMicroseconds) => {
            state.flow_start_micros = field_value_duration_usec(value);
        }
        IPFixField::IANA(IANAIPFixField::FlowEndMicroseconds)
        | IPFixField::IANA(IANAIPFixField::MaxFlowEndMicroseconds) => {
            state.flow_end_micros = field_value_duration_usec(value);
        }
        IPFixField::IANA(IANAIPFixField::FlowStartNanoseconds)
        | IPFixField::IANA(IANAIPFixField::MinFlowStartNanoseconds) => {
            state.flow_start_nanos = field_value_duration_usec(value);
        }
        IPFixField::IANA(IANAIPFixField::FlowEndNanoseconds)
        | IPFixField::IANA(IANAIPFixField::MaxFlowEndNanoseconds) => {
            state.flow_end_nanos = field_value_duration_usec(value);
        }
        IPFixField::IANA(IANAIPFixField::FlowStartDeltaMicroseconds) => {
            state.flow_start_delta_micros = field_value_unsigned(value);
        }
        IPFixField::IANA(IANAIPFixField::FlowEndDeltaMicroseconds) => {
            state.flow_end_delta_micros = field_value_unsigned(value);
        }
        IPFixField::IANA(IANAIPFixField::FlowStartSysUpTime) => {
            state.flow_start_sysuptime_millis = field_value_unsigned(value);
        }
        IPFixField::IANA(IANAIPFixField::FlowEndSysUpTime) => {
            state.flow_end_sysuptime_millis = field_value_unsigned(value);
        }
        _ => {}
    }
}

fn apply_ipfix_record_field(
    rec: &mut FlowRecord,
    state: &mut IPFixRecordBuildState,
    field: &IPFixField,
    value: &FieldValue,
    decapsulation_mode: DecapsulationMode,
    export_usec: u64,
) {
    if let IPFixField::IANA(IANAIPFixField::DataLinkFrameSection) = field {
        if let FieldValue::Vec(raw_value) | FieldValue::Unknown(raw_value) = value
            && let Some(l3_len) =
                parse_datalink_frame_section_record(raw_value, rec, decapsulation_mode)
        {
            rec.bytes = l3_len;
            rec.packets = 1;
            state.decap_ok = true;
        }
        return;
    }

    if let IPFixField::ReverseInformationElement(reverse_field) = field {
        state.reverse_present = true;
        track_reverse_ipfix_time(state, reverse_field, value, export_usec);

        let value_str = field_value_to_string(value);
        apply_reverse_ipfix_special_mappings(&mut state.reverse_overrides, reverse_field, &value_str);
        if let Some(canonical) = reverse_ipfix_canonical_key(reverse_field) {
            if should_skip_zero_ip(canonical, &value_str) {
                return;
            }
            state
                .reverse_overrides
                .insert(canonical, canonical_value(canonical, &value_str).to_string());
        }
        return;
    }

    if let IPFixField::IANA(IANAIPFixField::SystemInitTimeMilliseconds) = field {
        state.system_init_millis = field_value_unsigned(value);
    }

    let value_str = field_value_to_string(value);
    if let IPFixField::IANA(IANAIPFixField::ResponderOctets) = field {
        state.reverse_present = true;
        state.reverse_overrides.insert("BYTES", value_str);
        return;
    }
    if let IPFixField::IANA(IANAIPFixField::ResponderPackets) = field {
        state.reverse_present = true;
        state.reverse_overrides.insert("PACKETS", value_str);
        return;
    }

    apply_ipfix_special_mappings_record(rec, field, &value_str);
    observe_ipfix_record_value(state, field, value, &value_str);

    if let Some(canonical) = ipfix_canonical_key(field) {
        if should_skip_zero_ip(canonical, &value_str) {
            return;
        }
        if matches!(field, IPFixField::IANA(IANAIPFixField::IpVersion)) {
            return;
        }
        set_record_field(rec, canonical, &value_str);
    }
}

fn build_reverse_ipfix_flow(
    forward: &FlowRecord,
    reverse_overrides: &FlowFields,
    source_ts: Option<u64>,
) -> Option<DecodedFlow> {
    let reverse_packets = reverse_overrides
        .get("PACKETS")
        .and_then(|value| value.parse::<u64>().ok())
        .unwrap_or(0);
    if reverse_packets == 0 {
        return None;
    }

    let mut reverse = forward.clone();
    swap_directional_record_fields(&mut reverse);
    for (&key, value) in reverse_overrides {
        override_record_field(&mut reverse, key, value);
    }
    sync_raw_metrics_record(&mut reverse);
    finalize_record(&mut reverse);
    Some(DecodedFlow {
        record: reverse,
        source_realtime_usec: source_ts,
    })
}

fn finalize_ipfix_record(
    mut rec: FlowRecord,
    mut state: IPFixRecordBuildState,
    exporter_ip: &str,
    version: u16,
    observation_domain_id: u32,
    sampling: &mut SamplingState,
    need_decap: bool,
    export_usec: u64,
    timestamp_source: TimestampSource,
    input_realtime_usec: u64,
) -> Option<(DecodedFlow, Option<DecodedFlow>)> {
    state.apply_sampling_packet_ratio();
    apply_sampling_state_record(
        &mut rec,
        exporter_ip,
        version,
        observation_domain_id,
        state.sampler_id,
        state.observed_sampling_rate,
        sampling,
    );

    if looks_like_sampling_option_record_from_rec(&rec, state.observed_sampling_rate) {
        return None;
    }
    if need_decap && !state.decap_ok {
        return None;
    }

    if rec.flows == 0 {
        rec.flows = 1;
    }
    state.resolve_flow_times(&mut rec, export_usec);
    state.apply_reverse_time_overrides();
    finalize_record(&mut rec);

    let first_switched_usec = (rec.flow_start_usec != 0).then_some(rec.flow_start_usec);
    let source_ts = timestamp_source.select(
        input_realtime_usec,
        Some(export_usec),
        first_switched_usec,
    );
    let reverse = state
        .reverse_present
        .then(|| build_reverse_ipfix_flow(&rec, &state.reverse_overrides, source_ts))
        .flatten();

    Some((
        DecodedFlow {
            record: rec,
            source_realtime_usec: source_ts,
        },
        reverse,
    ))
}

fn append_ipfix_records(
    source: SocketAddr,
    out: &mut Vec<DecodedFlow>,
    packet: IPFix,
    sampling: &mut SamplingState,
    decapsulation_mode: DecapsulationMode,
    timestamp_source: TimestampSource,
    input_realtime_usec: u64,
) {
    let export_usec = unix_timestamp_to_usec(packet.header.export_time as u64, 0);
    let exporter_ip = source.ip().to_string();
    let observation_domain_id = packet.header.observation_domain_id;
    let version = 10_u16;

    for flowset in packet.flowsets {
        match flowset.body {
            IPFixFlowSetBody::Data(data) => {
                for record in data.fields {
                    let mut rec = base_record("ipfix", source);
                    let mut state = IPFixRecordBuildState::default();
                    let need_decap = !decapsulation_mode.is_none();

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
                        &exporter_ip,
                        version,
                        observation_domain_id,
                        sampling,
                        need_decap,
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
                    &exporter_ip,
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

fn extract_sflow_flows(
    source: SocketAddr,
    datagram: SFlowDatagram,
    decapsulation_mode: DecapsulationMode,
    timestamp_source: TimestampSource,
    input_realtime_usec: u64,
) -> Vec<DecodedFlow> {
    let exporter_ip_override = sflow_agent_ip_addr(&datagram.agent_address);
    let source_realtime_usec = timestamp_source.select(input_realtime_usec, None, None);
    let need_decap = !decapsulation_mode.is_none();

    let mut flows = Vec::new();
    for sample in datagram.samples {
        match sample.sample_data {
            SampleData::FlowSample(sample_data) => {
                let mut in_if = if sample_data.input.is_single() {
                    Some(sample_data.input.value())
                } else {
                    None
                };
                let mut out_if = if sample_data.output.is_single() {
                    Some(sample_data.output.value())
                } else {
                    None
                };
                let forwarding_status = if sample_data.output.is_discarded() {
                    128
                } else {
                    0
                };

                if in_if == Some(SFLOW_INTERFACE_LOCAL) {
                    in_if = Some(0);
                }
                if out_if == Some(SFLOW_INTERFACE_LOCAL) {
                    out_if = Some(0);
                }

                let flow_records: Vec<FlowData> = sample_data
                    .flow_records
                    .into_iter()
                    .map(|record| record.flow_data)
                    .collect();

                if let Some(flow) = build_sflow_flow(
                    source,
                    exporter_ip_override,
                    sample_data.sampling_rate,
                    in_if,
                    out_if,
                    forwarding_status,
                    &flow_records,
                    source_realtime_usec,
                    decapsulation_mode,
                    need_decap,
                ) {
                    flows.push(flow);
                }
            }
            SampleData::FlowSampleExpanded(sample_data) => {
                let mut in_if = if sample_data.input.format == SFLOW_INTERFACE_FORMAT_INDEX {
                    Some(sample_data.input.value)
                } else {
                    None
                };
                let mut out_if = if sample_data.output.format == SFLOW_INTERFACE_FORMAT_INDEX {
                    Some(sample_data.output.value)
                } else {
                    None
                };
                let forwarding_status =
                    if sample_data.output.format == SFLOW_INTERFACE_FORMAT_DISCARD {
                        128
                    } else {
                        0
                    };

                if in_if == Some(SFLOW_INTERFACE_LOCAL) {
                    in_if = Some(0);
                }
                if out_if == Some(SFLOW_INTERFACE_LOCAL) {
                    out_if = Some(0);
                }

                let flow_records: Vec<FlowData> = sample_data
                    .flow_records
                    .into_iter()
                    .map(|record| record.flow_data)
                    .collect();

                if let Some(flow) = build_sflow_flow(
                    source,
                    exporter_ip_override,
                    sample_data.sampling_rate,
                    in_if,
                    out_if,
                    forwarding_status,
                    &flow_records,
                    source_realtime_usec,
                    decapsulation_mode,
                    need_decap,
                ) {
                    flows.push(flow);
                }
            }
            _ => {}
        }
    }

    flows
}

fn build_sflow_flow(
    source: SocketAddr,
    exporter_ip_override: Option<IpAddr>,
    sampling_rate: u32,
    in_if: Option<u32>,
    out_if: Option<u32>,
    forwarding_status: u32,
    flow_records: &[FlowData],
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
        .any(|record| matches!(record, FlowData::SampledIpv4(_)));
    let has_sampled_ipv6 = flow_records
        .iter()
        .any(|record| matches!(record, FlowData::SampledIpv6(_)));
    let has_sampled_ethernet = flow_records
        .iter()
        .any(|record| matches!(record, FlowData::SampledEthernet(_)));
    let has_extended_switch = flow_records
        .iter()
        .any(|record| matches!(record, FlowData::ExtendedSwitch(_)));

    let mut l3_length = 0_u64;
    for flow_data in flow_records {
        match flow_data {
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
                rec.src_port = sampled.src_port as u16;
                rec.dst_port = sampled.dst_port as u16;
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
                rec.src_port = sampled.src_port as u16;
                rec.dst_port = sampled.dst_port as u16;
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
