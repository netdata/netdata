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

/// How the autocomplete dropdown matches user input against stored values.
///
/// This is exclusively about the autocomplete/dropdown UX. Regular facet
/// matching (selections / `key in [values]`) is always exact equality and
/// uses indexes — never a substring scan.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) enum AutocompleteMatchKind {
    /// `value.starts_with(term)`. Cheap on FST sidecars (automaton-driven).
    /// Right for IPs (typing `10.0.` to narrow), short numeric values.
    Prefix,
    /// `value.contains(term)`. Linear scan. Right for free-form labels
    /// (e.g. `AS20940 Akamai International` matching a search for `Akamai`).
    Substring,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) struct FacetFieldSpec {
    pub(crate) name: &'static str,
    pub(crate) kind: FacetValueKind,
    pub(crate) supports_autocomplete: bool,
    pub(crate) uses_sidecar: bool,
    pub(crate) autocomplete_match: AutocompleteMatchKind,
}

const VIRTUAL_FACET_FIELDS: &[FacetFieldSpec] = &[
    FacetFieldSpec {
        name: "ICMPV4",
        kind: FacetValueKind::Text,
        supports_autocomplete: true,
        uses_sidecar: false,
        autocomplete_match: AutocompleteMatchKind::Substring,
    },
    FacetFieldSpec {
        name: "ICMPV6",
        kind: FacetValueKind::Text,
        supports_autocomplete: true,
        uses_sidecar: false,
        autocomplete_match: AutocompleteMatchKind::Substring,
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

const FACET_FIELD_STACK_BUF_CAP: usize = 64;

fn facet_field_spec_lookup(trimmed: &str) -> Option<&'static FacetFieldSpec> {
    if trimmed.len() <= FACET_FIELD_STACK_BUF_CAP {
        let mut buf = [0_u8; FACET_FIELD_STACK_BUF_CAP];
        let bytes = trimmed.as_bytes();
        for (idx, &byte) in bytes.iter().enumerate() {
            buf[idx] = byte.to_ascii_uppercase();
        }

        let normalized = std::str::from_utf8(&buf[..bytes.len()])
            .expect("ASCII uppercasing preserves UTF-8 validity");
        return FACET_FIELD_SPEC_INDEX.get(normalized);
    }

    let normalized = trimmed.to_ascii_uppercase();
    FACET_FIELD_SPEC_INDEX.get(normalized.as_str())
}

pub(crate) fn facet_field_spec(field: &str) -> Option<&'static FacetFieldSpec> {
    facet_field_spec_lookup(field.trim())
}

#[inline]
pub(crate) fn facet_field_spec_static(field: &'static str) -> Option<FacetFieldSpec> {
    facet_field_spec_for_name(field).or_else(|| match field {
        "ICMPV4" => Some(VIRTUAL_FACET_FIELDS[0]),
        "ICMPV6" => Some(VIRTUAL_FACET_FIELDS[1]),
        _ => None,
    })
}

pub(crate) fn facet_field_enabled(field: &str) -> bool {
    facet_field_spec(field).is_some()
}

fn facet_field_spec_for_name(field: &'static str) -> Option<FacetFieldSpec> {
    let kind = match field {
        "BYTES"
        | "PACKETS"
        | "FLOWS"
        | "RAW_BYTES"
        | "RAW_PACKETS"
        | "SAMPLING_RATE"
        | "FLOW_START_USEC"
        | "FLOW_END_USEC"
        | "OBSERVATION_TIME_MILLIS" => {
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
        "IN_IF_SPEED" | "OUT_IF_SPEED" => FacetValueKind::SparseU64,
        _ => FacetValueKind::Text,
    };

    let autocomplete_match = match kind {
        FacetValueKind::Text => AutocompleteMatchKind::Substring,
        _ => AutocompleteMatchKind::Prefix,
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
        autocomplete_match,
    })
}

#[cfg(test)]
mod tests {
    use super::facet_field_spec;

    #[test]
    fn facet_field_spec_trims_and_matches_case_insensitively() {
        assert_eq!(
            facet_field_spec(" src_as ").map(|spec| spec.name),
            Some("SRC_AS")
        );
    }

    #[test]
    fn facet_field_spec_rejects_unknown_fields() {
        assert!(facet_field_spec(" definitely_not_a_facet ").is_none());
    }

    #[test]
    fn facet_field_spec_rejects_internal_timestamp_fields() {
        assert!(facet_field_spec("FLOW_START_USEC").is_none());
        assert!(facet_field_spec("FLOW_END_USEC").is_none());
        assert!(facet_field_spec("OBSERVATION_TIME_MILLIS").is_none());
    }
}
