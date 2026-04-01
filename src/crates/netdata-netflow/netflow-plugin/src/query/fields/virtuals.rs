use crate::presentation;
use crate::query::VIRTUAL_FLOW_FIELDS;
use std::collections::BTreeMap;
#[cfg(test)]
use std::collections::HashSet;

pub(crate) fn virtual_flow_field_dependencies(field: &str) -> &'static [&'static str] {
    match field.to_ascii_uppercase().as_str() {
        "ICMPV4" => &["PROTOCOL", "ICMPV4_TYPE", "ICMPV4_CODE"],
        "ICMPV6" => &["PROTOCOL", "ICMPV6_TYPE", "ICMPV6_CODE"],
        _ => &[],
    }
}

#[cfg(test)]
pub(crate) fn expand_virtual_flow_field_dependencies(fields: &mut HashSet<String>) {
    let requested = fields.iter().cloned().collect::<Vec<_>>();
    for field in requested {
        fields.extend(
            virtual_flow_field_dependencies(field.as_str())
                .iter()
                .map(|dependency| (*dependency).to_string()),
        );
    }
}

pub(crate) fn populate_virtual_fields(fields: &mut BTreeMap<String, String>) {
    for field in VIRTUAL_FLOW_FIELDS {
        let value = match *field {
            "ICMPV4" => presentation::icmp_virtual_value(
                field,
                fields.get("PROTOCOL").map(String::as_str),
                fields.get("ICMPV4_TYPE").map(String::as_str),
                fields.get("ICMPV4_CODE").map(String::as_str),
            ),
            "ICMPV6" => presentation::icmp_virtual_value(
                field,
                fields.get("PROTOCOL").map(String::as_str),
                fields.get("ICMPV6_TYPE").map(String::as_str),
                fields.get("ICMPV6_CODE").map(String::as_str),
            ),
            _ => None,
        };

        match value {
            Some(value) => {
                fields.insert((*field).to_string(), value);
            }
            None => {
                fields.remove(*field);
            }
        }
    }
}
