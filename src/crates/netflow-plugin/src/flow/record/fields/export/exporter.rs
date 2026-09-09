use super::super::super::*;

pub(super) fn insert_exporter_fields(record: &FlowRecord, fields: &mut FlowFields) {
    // Version / exporter
    fields.insert("FLOW_VERSION", record.flow_version.to_string());
    fields.insert(
        "EXPORTER_IP",
        super::helpers::opt_ip_to_string(record.exporter_ip),
    );
    fields.insert("EXPORTER_PORT", record.exporter_port.to_string());
    fields.insert("EXPORTER_NAME", record.exporter_name.clone());
    fields.insert("EXPORTER_GROUP", record.exporter_group.clone());
    fields.insert("EXPORTER_ROLE", record.exporter_role.clone());
    fields.insert("EXPORTER_SITE", record.exporter_site.clone());
    fields.insert("EXPORTER_REGION", record.exporter_region.clone());
    fields.insert("EXPORTER_TENANT", record.exporter_tenant.clone());

    // Sampling
    fields.insert(
        "SAMPLING_RATE",
        if record.has_sampling_rate() {
            record.sampling_rate.to_string()
        } else {
            String::new()
        },
    );

    // L2/L3
    fields.insert(
        "ETYPE",
        if record.has_etype() {
            record.etype.to_string()
        } else {
            String::new()
        },
    );
    fields.insert("PROTOCOL", record.protocol.to_string());

    // Counters
    fields.insert("BYTES", record.bytes.to_string());
    fields.insert("PACKETS", record.packets.to_string());
    fields.insert("FLOWS", record.flows.to_string());
    fields.insert("RAW_BYTES", record.raw_bytes.to_string());
    fields.insert("RAW_PACKETS", record.raw_packets.to_string());
    fields.insert(
        "FORWARDING_STATUS",
        if record.has_forwarding_status() {
            record.forwarding_status.to_string()
        } else {
            String::new()
        },
    );
    fields.insert(
        "DIRECTION",
        if record.has_direction() {
            record.direction.as_str().to_string()
        } else {
            String::new()
        },
    );
}
