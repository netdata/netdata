use super::super::super::*;

pub(super) fn insert_interface_fields(record: &FlowRecord, fields: &mut FlowFields) {
    // Interfaces
    fields.insert("IN_IF", record.in_if.to_string());
    fields.insert("OUT_IF", record.out_if.to_string());
    fields.insert("IN_IF_NAME", record.in_if_name.clone());
    fields.insert("OUT_IF_NAME", record.out_if_name.clone());
    fields.insert("IN_IF_DESCRIPTION", record.in_if_description.clone());
    fields.insert("OUT_IF_DESCRIPTION", record.out_if_description.clone());
    fields.insert(
        "IN_IF_SPEED",
        if record.has_in_if_speed() {
            record.in_if_speed.to_string()
        } else {
            String::new()
        },
    );
    fields.insert(
        "OUT_IF_SPEED",
        if record.has_out_if_speed() {
            record.out_if_speed.to_string()
        } else {
            String::new()
        },
    );
    fields.insert("IN_IF_PROVIDER", record.in_if_provider.clone());
    fields.insert("OUT_IF_PROVIDER", record.out_if_provider.clone());
    fields.insert("IN_IF_CONNECTIVITY", record.in_if_connectivity.clone());
    fields.insert("OUT_IF_CONNECTIVITY", record.out_if_connectivity.clone());
    fields.insert(
        "IN_IF_BOUNDARY",
        if record.has_in_if_boundary() {
            record.in_if_boundary.to_string()
        } else {
            String::new()
        },
    );
    fields.insert(
        "OUT_IF_BOUNDARY",
        if record.has_out_if_boundary() {
            record.out_if_boundary.to_string()
        } else {
            String::new()
        },
    );

    // VLAN
    fields.insert(
        "SRC_VLAN",
        if record.has_src_vlan() {
            record.src_vlan.to_string()
        } else {
            String::new()
        },
    );
    fields.insert(
        "DST_VLAN",
        if record.has_dst_vlan() {
            record.dst_vlan.to_string()
        } else {
            String::new()
        },
    );

    // MAC
    fields.insert(
        "SRC_MAC",
        if record.src_mac == [0u8; 6] {
            String::new()
        } else {
            super::helpers::format_mac(&record.src_mac)
        },
    );
    fields.insert(
        "DST_MAC",
        if record.dst_mac == [0u8; 6] {
            String::new()
        } else {
            super::helpers::format_mac(&record.dst_mac)
        },
    );
}
