use super::*;

pub(super) fn encode_header_journal_fields(record: &FlowRecord, writer: &mut JournalBufWriter<'_>) {
    writer.push_u8("IPTTL", record.ipttl);
    writer.push_u8_when(record.has_iptos(), "IPTOS", record.iptos);
    writer.push_u32("IPV6_FLOW_LABEL", record.ipv6_flow_label);
    writer.push_u8_when(record.has_tcp_flags(), "TCP_FLAGS", record.tcp_flags);
    writer.push_u32("IP_FRAGMENT_ID", record.ip_fragment_id);
    writer.push_u16("IP_FRAGMENT_OFFSET", record.ip_fragment_offset);
    writer.push_u8_when(record.has_icmpv4_type(), "ICMPV4_TYPE", record.icmpv4_type);
    writer.push_u8_when(record.has_icmpv4_code(), "ICMPV4_CODE", record.icmpv4_code);
    writer.push_u8_when(record.has_icmpv6_type(), "ICMPV6_TYPE", record.icmpv6_type);
    writer.push_u8_when(record.has_icmpv6_code(), "ICMPV6_CODE", record.icmpv6_code);
    writer.push_str("MPLS_LABELS", &record.mpls_labels);
    writer.push_u64("RAW_BYTES", record.raw_bytes);
    writer.push_u64("RAW_PACKETS", record.raw_packets);
    writer.push_u64_when(
        record.has_sampling_rate(),
        "SAMPLING_RATE",
        record.sampling_rate,
    );
}
