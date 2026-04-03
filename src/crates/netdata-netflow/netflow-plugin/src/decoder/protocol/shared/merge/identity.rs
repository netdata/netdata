use super::*;

pub(crate) fn append_unique_flows(dst: &mut Vec<DecodedFlow>, incoming: Vec<DecodedFlow>) {
    let incoming_len = incoming.len();
    if incoming_len == 0 {
        return;
    }

    if dst.is_empty() && incoming_len == 1 {
        dst.extend(incoming);
        return;
    }

    dst.reserve(incoming_len);

    // Build a hash index over existing flows for O(1) identity lookups instead of O(n) scans.
    // The index also tracks flows appended from the current incoming batch so first-batch
    // dedup/merge keeps working when dst starts empty.
    let mut identity_index: HashMap<u64, Vec<usize>> =
        HashMap::with_capacity(dst.len().saturating_add(incoming_len));
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
                    // Merge found nothing to update. If the full records are
                    // identical, it's an exact duplicate — drop it.
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
/// observation_time_millis is intentionally excluded because it derives from
/// packet arrival time, not stable flow identity. Records that differ only in
/// observation timestamp still reach identity-based merge handling below, but
/// they are only dropped when the full records are otherwise identical.
pub(crate) fn flow_identity_hash(flow: &DecodedFlow) -> u64 {
    use std::hash::Hasher;
    let mut hasher = twox_hash::XxHash64::default();
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

pub(crate) fn same_flow_identity(existing: &DecodedFlow, incoming: &DecodedFlow) -> bool {
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

pub(crate) fn identity_counter_value(raw: u64, scaled: u64) -> u64 {
    if raw != 0 { raw } else { scaled }
}
