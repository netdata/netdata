use super::*;

pub(super) fn encode_core_journal_fields(record: &FlowRecord, writer: &mut JournalBufWriter<'_>) {
    writer.push_str("FLOW_VERSION", record.flow_version);
    writer.push_opt_ip("EXPORTER_IP", record.exporter_ip);
    writer.push_u16("EXPORTER_PORT", record.exporter_port);
    writer.push_str("EXPORTER_NAME", &record.exporter_name);
    writer.push_str("EXPORTER_GROUP", &record.exporter_group);
    writer.push_str("EXPORTER_ROLE", &record.exporter_role);
    writer.push_str("EXPORTER_SITE", &record.exporter_site);
    writer.push_str("EXPORTER_REGION", &record.exporter_region);
    writer.push_str("EXPORTER_TENANT", &record.exporter_tenant);
    writer.push_u16_when(record.has_etype(), "ETYPE", record.etype);
    writer.push_u8("PROTOCOL", record.protocol);
    writer.push_u64("BYTES", record.bytes);
    writer.push_u64("PACKETS", record.packets);
    writer.push_u64("FLOWS", record.flows);
    writer.push_u8_when(
        record.has_forwarding_status(),
        "FORWARDING_STATUS",
        record.forwarding_status,
    );
    writer.push_direction(record.has_direction(), record.direction.as_str());
}
