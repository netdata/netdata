use crate::presentation;
use crate::query::{FACET_ALLOWED_OPTIONS, FlowsRequest};
use hashbrown::HashMap as FastHashMap;
use std::borrow::Cow;
use std::collections::HashMap;

pub(crate) fn captured_stored_facet_field_value<'a>(
    field: &str,
    capture_positions: &FastHashMap<String, usize>,
    captured_values: &'a [Option<String>],
) -> Option<&'a str> {
    let slot = capture_positions.get(field).copied()?;
    captured_values.get(slot)?.as_deref()
}

pub(crate) fn captured_facet_field_value<'a>(
    field: &str,
    capture_positions: &FastHashMap<String, usize>,
    captured_values: &'a [Option<String>],
) -> Option<Cow<'a, str>> {
    match field.to_ascii_uppercase().as_str() {
        "ICMPV4" => presentation::icmp_virtual_value(
            "ICMPV4",
            captured_stored_facet_field_value("PROTOCOL", capture_positions, captured_values),
            captured_stored_facet_field_value("ICMPV4_TYPE", capture_positions, captured_values),
            captured_stored_facet_field_value("ICMPV4_CODE", capture_positions, captured_values),
        )
        .map(Cow::Owned),
        "ICMPV6" => presentation::icmp_virtual_value(
            "ICMPV6",
            captured_stored_facet_field_value("PROTOCOL", capture_positions, captured_values),
            captured_stored_facet_field_value("ICMPV6_TYPE", capture_positions, captured_values),
            captured_stored_facet_field_value("ICMPV6_CODE", capture_positions, captured_values),
        )
        .map(Cow::Owned),
        _ => captured_stored_facet_field_value(field, capture_positions, captured_values)
            .map(Cow::Borrowed),
    }
}

pub(crate) fn captured_facet_matches_selections_except(
    ignored_field: Option<&str>,
    selections: &HashMap<String, Vec<String>>,
    capture_positions: &FastHashMap<String, usize>,
    captured_values: &[Option<String>],
) -> bool {
    selections.iter().all(|(field, values)| {
        if ignored_field.is_some_and(|ignored| ignored.eq_ignore_ascii_case(field)) {
            return true;
        }
        if values.is_empty() {
            return true;
        }
        let Some(record_value) =
            captured_facet_field_value(field, capture_positions, captured_values)
        else {
            return false;
        };
        values.iter().any(|value| value == record_value.as_ref())
    })
}

pub(crate) fn requested_facet_fields(request: &FlowsRequest) -> Vec<String> {
    request
        .normalized_facets()
        .unwrap_or_else(|| FACET_ALLOWED_OPTIONS.clone())
}
