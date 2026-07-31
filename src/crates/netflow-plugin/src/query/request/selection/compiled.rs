use crate::facet_catalog::{FacetValueKind, facet_field_spec};
use crate::query::is_virtual_flow_field;
use ipnet::IpNet;
use ipnet_trie::IpnetTrie;
use std::borrow::Cow;
use std::collections::{BTreeSet, HashMap, HashSet};
use std::net::{IpAddr, Ipv4Addr, Ipv6Addr};

const IP_PREFILTER_EXPANSION_LIMIT: usize = 256;

#[derive(Default)]
pub(crate) struct CompiledSelections {
    fields: Vec<CompiledSelectionField>,
    prefilter_pairs: Vec<(String, String)>,
}

struct CompiledSelectionField {
    name: String,
    matcher: SelectionMatcher,
}

enum SelectionMatcher {
    Exact(HashSet<String>),
    Ip(IpSelectionMatcher),
}

struct IpSelectionMatcher {
    prefixes: IpnetTrie<()>,
    fallback_exact: HashSet<String>,
}

impl CompiledSelections {
    pub(crate) fn compile(selections: &HashMap<String, Vec<String>>) -> Result<Self, String> {
        let mut selected_fields = selections.iter().collect::<Vec<_>>();
        selected_fields.sort_unstable_by(|(left, _), (right, _)| left.cmp(right));

        let mut fields = Vec::with_capacity(selected_fields.len());
        let mut prefilter_pairs = Vec::new();
        for (field, values) in selected_fields {
            let values = values
                .iter()
                .filter(|value| !value.is_empty())
                .cloned()
                .collect::<Vec<_>>();
            if values.is_empty() {
                continue;
            }

            let normalized = field.to_ascii_uppercase();
            if is_ip_address_facet(&normalized) {
                let (matcher, exact_prefilter_values) = compile_ip_selection(&normalized, &values)?;
                if let Some(values) = exact_prefilter_values {
                    prefilter_pairs
                        .extend(values.into_iter().map(|value| (normalized.clone(), value)));
                }
                fields.push(CompiledSelectionField {
                    name: normalized,
                    matcher: SelectionMatcher::Ip(matcher),
                });
            } else {
                let exact = values.into_iter().collect::<HashSet<_>>();
                if !is_virtual_flow_field(&normalized) {
                    prefilter_pairs.extend(
                        exact
                            .iter()
                            .cloned()
                            .map(|value| (normalized.clone(), value)),
                    );
                }
                fields.push(CompiledSelectionField {
                    name: normalized,
                    matcher: SelectionMatcher::Exact(exact),
                });
            }
        }

        prefilter_pairs.sort_unstable();
        Ok(Self {
            fields,
            prefilter_pairs,
        })
    }

    pub(crate) fn is_empty(&self) -> bool {
        self.fields.is_empty()
    }

    pub(crate) fn field_names(&self) -> impl Iterator<Item = &str> {
        self.fields.iter().map(|field| field.name.as_str())
    }

    pub(crate) fn prefilter_pairs(&self) -> &[(String, String)] {
        &self.prefilter_pairs
    }

    pub(crate) fn matches<'a>(
        &self,
        ignored_field: Option<&str>,
        mut field_value: impl FnMut(&str) -> Option<Cow<'a, str>>,
    ) -> bool {
        self.fields.iter().all(|field| {
            if ignored_field.is_some_and(|ignored| field.name.eq_ignore_ascii_case(ignored)) {
                return true;
            }

            let Some(value) = field_value(&field.name) else {
                return false;
            };
            field.matcher.matches(value.as_ref())
        })
    }
}

impl SelectionMatcher {
    fn matches(&self, value: &str) -> bool {
        match self {
            Self::Exact(values) => values.contains(value),
            Self::Ip(matcher) => matcher.matches(value),
        }
    }
}

impl IpSelectionMatcher {
    fn matches(&self, value: &str) -> bool {
        if self.fallback_exact.contains(value) {
            return true;
        }

        let Ok(address) = value.parse::<IpAddr>() else {
            return false;
        };
        self.prefixes.longest_match(&IpNet::from(address)).is_some()
    }
}

pub(crate) fn validate_ip_selection_values(
    selections: &HashMap<String, Vec<String>>,
) -> Result<(), String> {
    for (field, values) in selections {
        if !is_ip_address_facet(field) {
            continue;
        }
        for value in values {
            if value.contains('/') {
                parse_selected_cidr(field, value)?;
            }
        }
    }
    Ok(())
}

fn is_ip_address_facet(field: &str) -> bool {
    facet_field_spec(field).is_some_and(|spec| spec.kind == FacetValueKind::IpAddr)
}

fn compile_ip_selection(
    field: &str,
    values: &[String],
) -> Result<(IpSelectionMatcher, Option<Vec<String>>), String> {
    let mut prefixes = IpnetTrie::new();
    let mut fallback_exact = HashSet::new();
    let mut prefilter_networks = Vec::new();

    for value in values {
        if value.contains('/') {
            let network = parse_selected_cidr(field, value)?;
            prefixes.insert(network, ());
            prefilter_networks.push(network);
        } else if let Ok(address) = value.parse::<IpAddr>() {
            let network = IpNet::from(address);
            prefixes.insert(network, ());
            prefilter_networks.push(network);
        } else {
            fallback_exact.insert(value.clone());
        }
    }

    let exact_prefilter_values = bounded_exact_ip_values(&prefilter_networks, &fallback_exact);
    Ok((
        IpSelectionMatcher {
            prefixes,
            fallback_exact,
        },
        exact_prefilter_values,
    ))
}

fn parse_selected_cidr(field: &str, value: &str) -> Result<IpNet, String> {
    value.parse::<IpNet>().map(|network| network.trunc()).map_err(|_| {
        format!(
            "invalid CIDR selection `{value}` for field `{field}`; expected an IPv4 or IPv6 address with a valid prefix length"
        )
    })
}

fn bounded_exact_ip_values(
    networks: &[IpNet],
    fallback_exact: &HashSet<String>,
) -> Option<Vec<String>> {
    let mut values = fallback_exact.iter().cloned().collect::<BTreeSet<_>>();
    if values.len() > IP_PREFILTER_EXPANSION_LIMIT {
        return None;
    }

    for network in networks {
        let count = network_address_count(network)?;
        if count > IP_PREFILTER_EXPANSION_LIMIT {
            return None;
        }
        extend_network_addresses(&mut values, network, count);
        if values.len() > IP_PREFILTER_EXPANSION_LIMIT {
            return None;
        }
    }

    Some(values.into_iter().collect())
}

fn network_address_count(network: &IpNet) -> Option<usize> {
    let address_bits = match network {
        IpNet::V4(_) => 32,
        IpNet::V6(_) => 128,
    };
    let host_bits = address_bits - usize::from(network.prefix_len());
    (host_bits <= 8).then(|| 1_usize << host_bits)
}

fn extend_network_addresses(values: &mut BTreeSet<String>, network: &IpNet, count: usize) {
    match network {
        IpNet::V4(network) => {
            let start = u32::from(network.network());
            values
                .extend((0..count).map(|offset| Ipv4Addr::from(start + offset as u32).to_string()));
        }
        IpNet::V6(network) => {
            let start = u128::from(network.network());
            values.extend(
                (0..count).map(|offset| Ipv6Addr::from(start + offset as u128).to_string()),
            );
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn selections(field: &str, values: &[&str]) -> HashMap<String, Vec<String>> {
        HashMap::from([(
            field.to_string(),
            values.iter().map(|value| (*value).to_string()).collect(),
        )])
    }

    #[test]
    fn cidr_selection_matches_ipv4_and_ipv6_by_family() {
        let compiled = CompiledSelections::compile(&HashMap::from([
            ("SRC_ADDR".to_string(), vec!["10.1.2.99/24".to_string()]),
            (
                "DST_ADDR".to_string(),
                vec!["2001:db8:1::abcd/48".to_string()],
            ),
        ]))
        .expect("compile selections");

        assert!(compiled.matches(None, |field| match field {
            "SRC_ADDR" => Some(Cow::Borrowed("10.1.2.3")),
            "DST_ADDR" => Some(Cow::Borrowed("2001:db8:1::5")),
            _ => None,
        }));
        assert!(!compiled.matches(None, |field| match field {
            "SRC_ADDR" => Some(Cow::Borrowed("10.1.2.3")),
            "DST_ADDR" => Some(Cow::Borrowed("10.1.2.3")),
            _ => None,
        }));
    }

    #[test]
    fn cidr_selection_ors_exact_addresses_and_networks() {
        let compiled =
            CompiledSelections::compile(&selections("SRC_ADDR", &["192.0.2.1", "10.0.0.0/8"]))
                .expect("compile selections");

        for address in ["192.0.2.1", "10.20.30.40"] {
            assert!(compiled.matches(None, |_| Some(Cow::Borrowed(address))));
        }
        assert!(!compiled.matches(None, |_| Some(Cow::Borrowed("192.0.2.2"))));
    }

    #[test]
    fn cidr_selection_supports_prefix_boundaries_without_crossing_families() {
        let any_ipv4 = CompiledSelections::compile(&selections("SRC_ADDR", &["0.0.0.0/0"]))
            .expect("compile IPv4 selection");
        assert!(any_ipv4.matches(None, |_| Some(Cow::Borrowed("203.0.113.7"))));
        assert!(!any_ipv4.matches(None, |_| Some(Cow::Borrowed("2001:db8::7"))));

        let ipv6_host = CompiledSelections::compile(&selections("SRC_ADDR", &["2001:db8::7/128"]))
            .expect("compile IPv6 selection");
        assert!(ipv6_host.matches(None, |_| Some(Cow::Borrowed("2001:db8::7"))));
        assert!(!ipv6_host.matches(None, |_| Some(Cow::Borrowed("2001:db8::8"))));
    }

    #[test]
    fn cidr_prefilter_expands_complete_small_field() {
        let compiled = CompiledSelections::compile(&selections("SRC_ADDR", &["192.0.2.0/24"]))
            .expect("compile selections");

        assert_eq!(compiled.prefilter_pairs().len(), 256);
        assert_eq!(
            compiled.prefilter_pairs().first(),
            Some(&("SRC_ADDR".to_string(), "192.0.2.0".to_string()))
        );
        assert!(
            compiled
                .prefilter_pairs()
                .contains(&("SRC_ADDR".to_string(), "192.0.2.255".to_string()))
        );

        let ipv6 = CompiledSelections::compile(&selections("DST_ADDR", &["2001:db8::/120"]))
            .expect("compile IPv6 selections");
        assert_eq!(ipv6.prefilter_pairs().len(), 256);
        assert!(
            ipv6.prefilter_pairs()
                .contains(&("DST_ADDR".to_string(), "2001:db8::ff".to_string()))
        );
    }

    #[test]
    fn cidr_prefilter_omits_whole_unbounded_field_but_keeps_other_fields() {
        let compiled = CompiledSelections::compile(&HashMap::from([
            ("SRC_ADDR".to_string(), vec!["10.0.0.0/8".to_string()]),
            ("PROTOCOL".to_string(), vec!["6".to_string()]),
        ]))
        .expect("compile selections");

        assert_eq!(
            compiled.prefilter_pairs(),
            &[("PROTOCOL".to_string(), "6".to_string())]
        );

        let ipv6 = CompiledSelections::compile(&selections("DST_ADDR", &["2001:db8::/119"]))
            .expect("compile IPv6 selections");
        assert!(ipv6.prefilter_pairs().is_empty());
    }

    #[test]
    fn cidr_prefilter_limit_applies_to_the_complete_ip_field_union() {
        let compiled = CompiledSelections::compile(&HashMap::from([
            (
                "SRC_ADDR".to_string(),
                vec!["192.0.2.0/24".to_string(), "198.51.100.7".to_string()],
            ),
            ("PROTOCOL".to_string(), vec!["17".to_string()]),
        ]))
        .expect("compile selections");

        assert_eq!(
            compiled.prefilter_pairs(),
            &[("PROTOCOL".to_string(), "17".to_string())]
        );
        assert!(compiled.matches(None, |field| match field {
            "SRC_ADDR" => Some(Cow::Borrowed("198.51.100.7")),
            "PROTOCOL" => Some(Cow::Borrowed("17")),
            _ => None,
        }));
    }

    #[test]
    fn invalid_slash_selection_is_rejected() {
        let error = CompiledSelections::compile(&selections("SRC_ADDR", &["10/8"]))
            .err()
            .expect("invalid CIDR");
        assert!(error.contains("invalid CIDR selection"));
        assert!(error.contains("SRC_ADDR"));
    }
}
