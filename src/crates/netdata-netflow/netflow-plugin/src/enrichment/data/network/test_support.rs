use super::*;

pub(crate) fn parse_exporter_ip(fields: &FlowFields) -> Option<IpAddr> {
    fields
        .get("EXPORTER_IP")
        .and_then(|value| value.parse::<IpAddr>().ok())
}

pub(crate) fn parse_u16_field(fields: &FlowFields, key: &str) -> u16 {
    fields
        .get(key)
        .and_then(|value| value.parse::<u16>().ok())
        .unwrap_or(0)
}

pub(crate) fn parse_u8_field(fields: &FlowFields, key: &str) -> u8 {
    fields
        .get(key)
        .and_then(|value| value.parse::<u8>().ok())
        .unwrap_or(0)
}

pub(crate) fn parse_u32_field(fields: &FlowFields, key: &str) -> u32 {
    fields
        .get(key)
        .and_then(|value| value.parse::<u32>().ok())
        .unwrap_or(0)
}

pub(crate) fn parse_u64_field(fields: &FlowFields, key: &str) -> u64 {
    fields
        .get(key)
        .and_then(|value| value.parse::<u64>().ok())
        .unwrap_or(0)
}

pub(crate) fn parse_ip_field(fields: &FlowFields, key: &str) -> Option<IpAddr> {
    fields
        .get(key)
        .and_then(|value| value.parse::<IpAddr>().ok())
}

pub(crate) fn append_u32_list_field(fields: &mut FlowFields, key: &'static str, values: &[u32]) {
    if values.is_empty() {
        return;
    }
    let serialized = values
        .iter()
        .map(u32::to_string)
        .collect::<Vec<_>>()
        .join(",");
    append_csv_field(fields, key, &serialized);
}

pub(crate) fn append_large_communities_field(
    fields: &mut FlowFields,
    key: &'static str,
    values: &[StaticRoutingLargeCommunity],
) {
    if values.is_empty() {
        return;
    }
    let serialized = values
        .iter()
        .map(StaticRoutingLargeCommunity::format)
        .collect::<Vec<_>>()
        .join(",");
    append_csv_field(fields, key, &serialized);
}

fn append_csv_field(fields: &mut FlowFields, key: &'static str, suffix: &str) {
    if suffix.is_empty() {
        return;
    }

    let entry = fields.entry(key).or_default();
    if entry.is_empty() {
        *entry = suffix.to_string();
    } else {
        entry.push(',');
        entry.push_str(suffix);
    }
}
