use super::*;

fn ipv4_prefix_addr(ip: Ipv4Addr, prefix_len: u8) -> Ipv4Addr {
    let prefix_len = prefix_len.min(32);
    let mask = if prefix_len == 0 {
        0
    } else {
        u32::MAX << (32 - prefix_len)
    };
    Ipv4Addr::from(u32::from(ip) & mask)
}

pub(crate) fn append_v5_records(
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
        rec.src_prefix = Some(IpAddr::V4(ipv4_prefix_addr(flow.src_addr, flow.src_mask)));
        rec.dst_prefix = Some(IpAddr::V4(ipv4_prefix_addr(flow.dst_addr, flow.dst_mask)));
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
        rec.flow_start_usec = flow_start_usec;
        rec.flow_end_usec = flow_end_usec;
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

pub(crate) fn append_v7_records(
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
        rec.src_prefix = Some(IpAddr::V4(ipv4_prefix_addr(flow.src_addr, flow.src_mask)));
        rec.dst_prefix = Some(IpAddr::V4(ipv4_prefix_addr(flow.dst_addr, flow.dst_mask)));
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
        rec.flow_start_usec = flow_start_usec;
        rec.flow_end_usec = flow_end_usec;
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
