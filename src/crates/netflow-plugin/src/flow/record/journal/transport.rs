use super::*;

pub(super) fn encode_transport_journal_fields(
    record: &FlowRecord,
    writer: &mut JournalBufWriter<'_>,
) {
    writer.push_opt_ip("NEXT_HOP", record.next_hop);
    writer.push_u16("SRC_PORT", record.src_port);
    writer.push_u16("DST_PORT", record.dst_port);
    writer.push_u64("FLOW_START_USEC", record.flow_start_usec);
    writer.push_u64("FLOW_END_USEC", record.flow_end_usec);
    writer.push_u64("OBSERVATION_TIME_MILLIS", record.observation_time_millis);
    writer.push_opt_ip("SRC_ADDR_NAT", record.src_addr_nat);
    writer.push_opt_ip("DST_ADDR_NAT", record.dst_addr_nat);
    writer.push_u16("SRC_PORT_NAT", record.src_port_nat);
    writer.push_u16("DST_PORT_NAT", record.dst_port_nat);
    writer.push_u16_when(record.has_src_vlan(), "SRC_VLAN", record.src_vlan);
    writer.push_u16_when(record.has_dst_vlan(), "DST_VLAN", record.dst_vlan);
    writer.push_mac("SRC_MAC", record.src_mac);
    writer.push_mac("DST_MAC", record.dst_mac);
}
