use super::schema::{
    ROLLUP_FIELD_DEFS, ROLLUP_PRESENCE_FIELDS, is_internal_rollup_presence_field,
    rollup_field_index,
};
use super::*;

pub(crate) fn materialize_rollup_fields(
    index: &FlowIndex,
    flow_id: IndexedFlowId,
) -> Option<FlowFields> {
    let field_ids = index.flow_field_ids(flow_id)?;
    let mut fields = FlowFields::new();
    let mut exporter_ip_present = false;
    let mut next_hop_present = false;
    let mut exporter_ip = IpAddr::V4(Ipv4Addr::UNSPECIFIED);
    let mut next_hop = IpAddr::V4(Ipv4Addr::UNSPECIFIED);

    for (field_index, field_id) in field_ids.iter().copied().enumerate() {
        let name = ROLLUP_FIELD_DEFS.get(field_index)?.name;
        let value = index.field_value(field_index, field_id)?;
        match name {
            INTERNAL_EXPORTER_IP_PRESENT => {
                exporter_ip_present = matches!(value, IndexFieldValue::U8(1));
            }
            INTERNAL_NEXT_HOP_PRESENT => {
                next_hop_present = matches!(value, IndexFieldValue::U8(1));
            }
            name if is_internal_rollup_presence_field(name) => {}
            "EXPORTER_IP" => {
                if let IndexFieldValue::IpAddr(ip) = value {
                    exporter_ip = ip;
                }
            }
            "NEXT_HOP" => {
                if let IndexFieldValue::IpAddr(ip) = value {
                    next_hop = ip;
                }
            }
            "DIRECTION" => {
                if let IndexFieldValue::U8(direction) = value {
                    fields.insert(name, direction_from_u8(direction).as_str().to_string());
                }
            }
            _ => {
                fields.insert(name, compact_index_value_to_string(value));
            }
        }
    }

    fields.insert(
        "EXPORTER_IP",
        if exporter_ip_present {
            exporter_ip.to_string()
        } else {
            String::new()
        },
    );
    fields.insert(
        "NEXT_HOP",
        if next_hop_present {
            next_hop.to_string()
        } else {
            String::new()
        },
    );
    for &(field, internal_field) in ROLLUP_PRESENCE_FIELDS {
        let present = rollup_field_value(index, field_ids, internal_field)
            .is_some_and(|value| matches!(value, IndexFieldValue::U8(1)));
        if !present {
            fields.insert(field, String::new());
        }
    }

    Some(fields)
}

pub(crate) fn rollup_field_value<'a>(
    index: &'a FlowIndex,
    field_ids: &[u32],
    field: &str,
) -> Option<IndexFieldValue<'a>> {
    let field_index = rollup_field_index(field)?;
    let field_id = *field_ids.get(field_index)?;
    index.field_value(field_index, field_id)
}

pub(crate) fn compact_index_value_to_string(value: IndexFieldValue<'_>) -> String {
    match value {
        IndexFieldValue::Text(text) => text.to_string(),
        IndexFieldValue::U8(number) => number.to_string(),
        IndexFieldValue::U16(number) => number.to_string(),
        IndexFieldValue::U32(number) => number.to_string(),
        IndexFieldValue::U64(number) => number.to_string(),
        IndexFieldValue::IpAddr(ip) => ip.to_string(),
    }
}

pub(crate) fn bucket_start_usec(timestamp_usec: u64, bucket_usec: u64) -> u64 {
    (timestamp_usec / bucket_usec).saturating_mul(bucket_usec)
}

#[cfg(test)]
pub(crate) fn dimensions_for_rollup(fields: &FlowFields) -> FlowFields {
    const METRIC_FIELDS: [&str; 4] = ["BYTES", "PACKETS", "RAW_BYTES", "RAW_PACKETS"];
    const DEBUG_PREFIXES: [&str; 2] = ["V9_", "IPFIX_"];

    fields
        .iter()
        .filter(|&(&name, _)| {
            !name.starts_with('_')
                && !METRIC_FIELDS.contains(&name)
                && !DEBUG_PREFIXES.iter().any(|p| name.starts_with(p))
        })
        .map(|(&name, value)| (name, value.clone()))
        .collect()
}
