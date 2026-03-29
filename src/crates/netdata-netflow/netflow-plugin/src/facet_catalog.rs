use crate::flow::canonical_flow_field_names;
use std::collections::HashMap;
use std::sync::LazyLock;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) enum FacetValueKind {
    Text,
    DenseU8,
    DenseU16,
    SparseU32,
    SparseU64,
    IpAddr,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) struct FacetFieldSpec {
    pub(crate) name: &'static str,
    pub(crate) kind: FacetValueKind,
    pub(crate) supports_autocomplete: bool,
    pub(crate) uses_sidecar: bool,
}

const VIRTUAL_FACET_FIELDS: &[FacetFieldSpec] = &[
    FacetFieldSpec {
        name: "ICMPV4",
        kind: FacetValueKind::Text,
        supports_autocomplete: true,
        uses_sidecar: false,
    },
    FacetFieldSpec {
        name: "ICMPV6",
        kind: FacetValueKind::Text,
        supports_autocomplete: true,
        uses_sidecar: false,
    },
];

pub(crate) static FACET_FIELD_SPECS: LazyLock<Vec<FacetFieldSpec>> = LazyLock::new(|| {
    let mut specs = canonical_flow_field_names()
        .filter_map(facet_field_spec_for_name)
        .collect::<Vec<_>>();
    specs.extend_from_slice(VIRTUAL_FACET_FIELDS);
    specs
});

static FACET_FIELD_SPEC_INDEX: LazyLock<HashMap<&'static str, FacetFieldSpec>> =
    LazyLock::new(|| {
        FACET_FIELD_SPECS
            .iter()
            .copied()
            .map(|spec| (spec.name, spec))
            .collect()
    });

pub(crate) static FACET_ALLOWED_OPTIONS: LazyLock<Vec<String>> = LazyLock::new(|| {
    FACET_FIELD_SPECS
        .iter()
        .map(|spec| spec.name.to_string())
        .collect()
});

pub(crate) fn facet_field_spec(field: &str) -> Option<&'static FacetFieldSpec> {
    let normalized = field.trim().to_ascii_uppercase();
    FACET_FIELD_SPEC_INDEX.get(normalized.as_str())
}

pub(crate) fn facet_field_enabled(field: &str) -> bool {
    facet_field_spec(field).is_some()
}

fn facet_field_spec_for_name(field: &'static str) -> Option<FacetFieldSpec> {
    let kind = match field {
        "BYTES" | "PACKETS" | "FLOWS" | "RAW_BYTES" | "RAW_PACKETS" | "SAMPLING_RATE" => {
            return None;
        }
        "SRC_GEO_LATITUDE" | "DST_GEO_LATITUDE" | "SRC_GEO_LONGITUDE" | "DST_GEO_LONGITUDE" => {
            return None;
        }
        "EXPORTER_IP" | "SRC_ADDR" | "DST_ADDR" | "NEXT_HOP" | "SRC_ADDR_NAT" | "DST_ADDR_NAT" => {
            FacetValueKind::IpAddr
        }
        "EXPORTER_PORT" | "ETYPE" | "SRC_PORT" | "DST_PORT" | "SRC_PORT_NAT" | "DST_PORT_NAT"
        | "SRC_VLAN" | "DST_VLAN" | "IP_FRAGMENT_ID" | "IP_FRAGMENT_OFFSET" => {
            FacetValueKind::DenseU16
        }
        "PROTOCOL" | "FORWARDING_STATUS" | "DIRECTION" | "SRC_MASK" | "DST_MASK"
        | "IN_IF_BOUNDARY" | "OUT_IF_BOUNDARY" | "IPTTL" | "IPTOS" | "TCP_FLAGS"
        | "ICMPV4_TYPE" | "ICMPV4_CODE" | "ICMPV6_TYPE" | "ICMPV6_CODE" => FacetValueKind::DenseU8,
        "SRC_AS" | "DST_AS" | "IN_IF" | "OUT_IF" | "IPV6_FLOW_LABEL" => FacetValueKind::SparseU32,
        "FLOW_START_USEC"
        | "FLOW_END_USEC"
        | "OBSERVATION_TIME_MILLIS"
        | "IN_IF_SPEED"
        | "OUT_IF_SPEED" => FacetValueKind::SparseU64,
        _ => FacetValueKind::Text,
    };

    Some(FacetFieldSpec {
        name: field,
        kind,
        supports_autocomplete: true,
        uses_sidecar: matches!(
            kind,
            FacetValueKind::Text
                | FacetValueKind::IpAddr
                | FacetValueKind::SparseU32
                | FacetValueKind::SparseU64
                | FacetValueKind::DenseU16
        ),
    })
}
