use std::collections::{BTreeMap, HashMap};

pub(crate) const DIRECTION_UNDEFINED: &str = "undefined";
pub(crate) const DIRECTION_INGRESS: &str = "ingress";
pub(crate) const DIRECTION_EGRESS: &str = "egress";

pub(crate) const CANONICAL_FLOW_DEFAULTS: &[(&str, &str)] = &[
    ("FLOW_VERSION", ""),
    ("EXPORTER_IP", ""),
    ("EXPORTER_PORT", "0"),
    ("EXPORTER_NAME", ""),
    ("EXPORTER_GROUP", ""),
    ("EXPORTER_ROLE", ""),
    ("EXPORTER_SITE", ""),
    ("EXPORTER_REGION", ""),
    ("EXPORTER_TENANT", ""),
    ("SAMPLING_RATE", "0"),
    ("ETYPE", "0"),
    ("PROTOCOL", "0"),
    ("BYTES", "0"),
    ("PACKETS", "0"),
    ("FLOWS", "1"),
    ("RAW_BYTES", "0"),
    ("RAW_PACKETS", "0"),
    ("FORWARDING_STATUS", "0"),
    ("DIRECTION", DIRECTION_UNDEFINED),
    ("SRC_ADDR", ""),
    ("DST_ADDR", ""),
    ("SRC_PREFIX", ""),
    ("DST_PREFIX", ""),
    ("SRC_MASK", "0"),
    ("DST_MASK", "0"),
    ("SRC_AS", "0"),
    ("DST_AS", "0"),
    ("SRC_AS_NAME", ""),
    ("DST_AS_NAME", ""),
    ("SRC_NET_NAME", ""),
    ("DST_NET_NAME", ""),
    ("SRC_NET_ROLE", ""),
    ("DST_NET_ROLE", ""),
    ("SRC_NET_SITE", ""),
    ("DST_NET_SITE", ""),
    ("SRC_NET_REGION", ""),
    ("DST_NET_REGION", ""),
    ("SRC_NET_TENANT", ""),
    ("DST_NET_TENANT", ""),
    ("SRC_COUNTRY", ""),
    ("DST_COUNTRY", ""),
    ("SRC_GEO_CITY", ""),
    ("DST_GEO_CITY", ""),
    ("SRC_GEO_STATE", ""),
    ("DST_GEO_STATE", ""),
    ("SRC_GEO_LATITUDE", ""),
    ("DST_GEO_LATITUDE", ""),
    ("SRC_GEO_LONGITUDE", ""),
    ("DST_GEO_LONGITUDE", ""),
    ("DST_AS_PATH", ""),
    ("DST_COMMUNITIES", ""),
    ("DST_LARGE_COMMUNITIES", ""),
    ("IN_IF", "0"),
    ("OUT_IF", "0"),
    ("IN_IF_NAME", ""),
    ("OUT_IF_NAME", ""),
    ("IN_IF_DESCRIPTION", ""),
    ("OUT_IF_DESCRIPTION", ""),
    ("IN_IF_SPEED", "0"),
    ("OUT_IF_SPEED", "0"),
    ("IN_IF_PROVIDER", ""),
    ("OUT_IF_PROVIDER", ""),
    ("IN_IF_CONNECTIVITY", ""),
    ("OUT_IF_CONNECTIVITY", ""),
    ("IN_IF_BOUNDARY", "0"),
    ("OUT_IF_BOUNDARY", "0"),
    ("NEXT_HOP", ""),
    ("SRC_PORT", "0"),
    ("DST_PORT", "0"),
    ("FLOW_START_USEC", "0"),
    ("FLOW_END_USEC", "0"),
    ("OBSERVATION_TIME_MILLIS", "0"),
    ("SRC_ADDR_NAT", ""),
    ("DST_ADDR_NAT", ""),
    ("SRC_PORT_NAT", "0"),
    ("DST_PORT_NAT", "0"),
    ("SRC_VLAN", "0"),
    ("DST_VLAN", "0"),
    ("SRC_MAC", ""),
    ("DST_MAC", ""),
    ("IPTTL", "0"),
    ("IPTOS", "0"),
    ("IPV6_FLOW_LABEL", "0"),
    ("TCP_FLAGS", "0"),
    ("IP_FRAGMENT_ID", "0"),
    ("IP_FRAGMENT_OFFSET", "0"),
    ("ICMPV4_TYPE", "0"),
    ("ICMPV4_CODE", "0"),
    ("ICMPV6_TYPE", "0"),
    ("ICMPV6_CODE", "0"),
    ("MPLS_LABELS", ""),
];

pub(crate) fn canonical_flow_field_names() -> impl Iterator<Item = &'static str> {
    CANONICAL_FLOW_DEFAULTS.iter().map(|&(name, _)| name)
}

pub(crate) fn field_tracks_presence(field: &str) -> bool {
    matches!(
        field,
        "SAMPLING_RATE"
            | "ETYPE"
            | "DIRECTION"
            | "FORWARDING_STATUS"
            | "IN_IF_SPEED"
            | "OUT_IF_SPEED"
            | "IN_IF_BOUNDARY"
            | "OUT_IF_BOUNDARY"
            | "SRC_VLAN"
            | "DST_VLAN"
            | "IPTOS"
            | "TCP_FLAGS"
            | "ICMPV4_TYPE"
            | "ICMPV4_CODE"
            | "ICMPV6_TYPE"
            | "ICMPV6_CODE"
    )
}

pub(crate) fn field_present_in_map(fields: &FlowFields, field: &'static str) -> bool {
    fields.get(field).is_some_and(|value| {
        !value.is_empty() && !(field == "DIRECTION" && value == DIRECTION_UNDEFINED)
    })
}

pub(crate) fn scalar_field_present_in_map(fields: &FlowFields, field: &'static str) -> bool {
    fields.get(field).is_some_and(|value| !value.is_empty())
}

/// Flow field map: keys are compile-time constant field names, values are string representations.
/// Using `&'static str` keys eliminates ~60 heap allocations per flow for field names.
pub(crate) type FlowFields = BTreeMap<&'static str, String>;

/// Intern a field name string to its `&'static str` equivalent if known.
/// Used on cold paths (journal deserialization) to convert dynamic String keys.
pub(crate) fn intern_field_name(name: &str) -> Option<&'static str> {
    static INTERNED: std::sync::LazyLock<HashMap<&'static str, &'static str>> =
        std::sync::LazyLock::new(|| {
            CANONICAL_FLOW_DEFAULTS
                .iter()
                .map(|&(k, _)| (k, k))
                .collect()
        });
    INTERNED.get(name).copied()
}
