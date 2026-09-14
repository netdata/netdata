use crate::flow::FlowFields;
use crate::presentation;
use crate::tiering::{OpenTierRow, TierFlowIndexStore, dimensions_for_rollup};

pub(crate) fn open_tier_row_field_value(
    row: &OpenTierRow,
    tier_flow_indexes: &TierFlowIndexStore,
    field: &str,
) -> Option<String> {
    match field.to_ascii_uppercase().as_str() {
        "BYTES" => Some(row.metrics.bytes.to_string()),
        "PACKETS" => Some(row.metrics.packets.to_string()),
        "ICMPV4" => presentation::icmp_virtual_value(
            "ICMPV4",
            tier_flow_indexes
                .field_value_string(row.flow_ref, "PROTOCOL")
                .as_deref(),
            tier_flow_indexes
                .field_value_string(row.flow_ref, "ICMPV4_TYPE")
                .as_deref(),
            tier_flow_indexes
                .field_value_string(row.flow_ref, "ICMPV4_CODE")
                .as_deref(),
        ),
        "ICMPV6" => presentation::icmp_virtual_value(
            "ICMPV6",
            tier_flow_indexes
                .field_value_string(row.flow_ref, "PROTOCOL")
                .as_deref(),
            tier_flow_indexes
                .field_value_string(row.flow_ref, "ICMPV6_TYPE")
                .as_deref(),
            tier_flow_indexes
                .field_value_string(row.flow_ref, "ICMPV6_CODE")
                .as_deref(),
        ),
        _ => tier_flow_indexes.field_value_string(row.flow_ref, field),
    }
}

pub(crate) fn dimensions_from_fields(fields: &FlowFields) -> FlowFields {
    dimensions_for_rollup(fields)
}
