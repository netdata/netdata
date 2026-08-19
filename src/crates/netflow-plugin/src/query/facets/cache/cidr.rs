use crate::facet_catalog::{FacetValueKind, facet_field_spec};
use ipnet::IpNet;
use std::collections::HashSet;
use std::net::{IpAddr, Ipv4Addr, Ipv6Addr};

use crate::facet_runtime::FACET_AUTOCOMPLETE_LIMIT;

struct LooseIpAddress {
    address: IpAddr,
    natural_prefix: u8,
    max_prefix: u8,
}

pub(crate) fn cidr_autocomplete_networks(field: &str, term: &str) -> Option<Vec<IpNet>> {
    let spec = facet_field_spec(field)?;
    if spec.kind != FacetValueKind::IpAddr || !term.contains('/') {
        return None;
    }

    Some(generate_cidr_candidate_networks(term))
}

fn generate_cidr_candidate_networks(term: &str) -> Vec<IpNet> {
    let Some((address_fragment, prefix_fragment)) = term.split_once('/') else {
        return Vec::new();
    };
    if address_fragment.is_empty()
        || prefix_fragment.contains('/')
        || (!prefix_fragment.is_empty()
            && !prefix_fragment.bytes().all(|byte| byte.is_ascii_digit()))
    {
        return Vec::new();
    }

    let Some(loose) = parse_loose_ip_address(address_fragment) else {
        return Vec::new();
    };
    let prefixes = matching_prefix_lengths(prefix_fragment, loose.natural_prefix, loose.max_prefix);
    let mut seen = HashSet::with_capacity(prefixes.len());
    let mut values = Vec::with_capacity(prefixes.len().min(FACET_AUTOCOMPLETE_LIMIT));

    for prefix in prefixes {
        let Ok(network) = IpNet::new(loose.address, prefix) else {
            continue;
        };
        let network = network.trunc();
        if seen.insert(network) {
            values.push(network);
            if values.len() >= FACET_AUTOCOMPLETE_LIMIT {
                break;
            }
        }
    }
    values
}

fn parse_loose_ip_address(fragment: &str) -> Option<LooseIpAddress> {
    if fragment.contains(':') {
        parse_loose_ipv6(fragment)
    } else {
        parse_loose_ipv4(fragment)
    }
}

fn parse_loose_ipv4(fragment: &str) -> Option<LooseIpAddress> {
    let (fragment, trailing_dot) = match fragment.strip_suffix('.') {
        Some(fragment) => (fragment, true),
        None => (fragment, false),
    };
    let octets = fragment.split('.').collect::<Vec<_>>();
    if octets.is_empty() || octets.len() > 4 || (trailing_dot && octets.len() == 4) {
        return None;
    }

    let mut address = [0_u8; 4];
    for (index, octet) in octets.iter().enumerate() {
        if octet.is_empty() || !octet.bytes().all(|byte| byte.is_ascii_digit()) {
            return None;
        }
        address[index] = octet.parse().ok()?;
    }

    Some(LooseIpAddress {
        address: IpAddr::V4(Ipv4Addr::from(address)),
        natural_prefix: (octets.len() * 8) as u8,
        max_prefix: 32,
    })
}

fn parse_loose_ipv6(fragment: &str) -> Option<LooseIpAddress> {
    if let Ok(address) = fragment.parse::<Ipv6Addr>() {
        let natural_prefix = if fragment.ends_with("::") {
            fragment
                .trim_end_matches("::")
                .split(':')
                .filter(|component| !component.is_empty())
                .count()
                .saturating_mul(16) as u8
        } else {
            128
        };
        return Some(LooseIpAddress {
            address: IpAddr::V6(address),
            natural_prefix,
            max_prefix: 128,
        });
    }

    if fragment.contains("::") {
        return None;
    }
    let hextets = fragment.split(':').collect::<Vec<_>>();
    if hextets.is_empty() || hextets.len() >= 8 {
        return None;
    }
    if hextets.iter().any(|hextet| {
        hextet.is_empty()
            || hextet.len() > 4
            || !hextet.bytes().all(|byte| byte.is_ascii_hexdigit())
    }) {
        return None;
    }

    let address = format!("{fragment}::").parse::<Ipv6Addr>().ok()?;
    Some(LooseIpAddress {
        address: IpAddr::V6(address),
        natural_prefix: (hextets.len() * 16) as u8,
        max_prefix: 128,
    })
}

fn matching_prefix_lengths(fragment: &str, natural: u8, max: u8) -> Vec<u8> {
    if fragment.is_empty() {
        return vec![natural];
    }

    let mut matching = (0..=max)
        .filter(|prefix| prefix.to_string().starts_with(fragment))
        .collect::<Vec<_>>();
    if matching.is_empty() {
        return matching;
    }

    let mut ranked = Vec::with_capacity(matching.len());
    if matching.contains(&natural) {
        ranked.push(natural);
    }
    if let Ok(exact) = fragment.parse::<u8>()
        && matching.contains(&exact)
        && !ranked.contains(&exact)
    {
        ranked.push(exact);
    }
    for prefix in matching.drain(..) {
        if !ranked.contains(&prefix) {
            ranked.push(prefix);
        }
    }
    ranked
}

#[cfg(test)]
mod tests {
    use super::*;

    fn generate_cidr_candidates(term: &str) -> Vec<String> {
        generate_cidr_candidate_networks(term)
            .into_iter()
            .map(|network| network.to_string())
            .collect()
    }

    #[test]
    fn slash_is_required_and_only_ip_fields_are_synthetic() {
        assert_eq!(cidr_autocomplete_networks("SRC_ADDR", "10"), None);
        assert_eq!(cidr_autocomplete_networks("PROTOCOL", "10/8"), None);
        assert_eq!(cidr_autocomplete_networks("SRC_PREFIX", "10/8"), None);
        for field in [
            "EXPORTER_IP",
            "SRC_ADDR",
            "DST_ADDR",
            "NEXT_HOP",
            "SRC_ADDR_NAT",
            "DST_ADDR_NAT",
        ] {
            assert_eq!(
                cidr_autocomplete_networks(field, "10/8"),
                Some(vec!["10.0.0.0/8".parse().expect("valid network")]),
                "{field}"
            );
        }
    }

    #[test]
    fn loose_ipv4_completion_returns_canonical_networks() {
        for (term, expected) in [
            ("10/", "10.0.0.0/8"),
            ("10./", "10.0.0.0/8"),
            ("10/8", "10.0.0.0/8"),
            ("10.1/", "10.1.0.0/16"),
            ("10.1./", "10.1.0.0/16"),
            ("10.1.2./", "10.1.2.0/24"),
            ("10.1.2.3/24", "10.1.2.0/24"),
        ] {
            assert_eq!(
                generate_cidr_candidates(term).first().map(String::as_str),
                Some(expected),
                "{term}"
            );
        }
    }

    #[test]
    fn prefix_fragments_rank_natural_then_exact_and_only_return_canonical_values() {
        let values = generate_cidr_candidates("10.1/1");
        assert_eq!(values.first().map(String::as_str), Some("10.1.0.0/16"));
        assert_eq!(values.get(1).map(String::as_str), Some("0.0.0.0/1"));
        assert!(values.contains(&"10.0.0.0/10".to_string()));
        assert!(values.contains(&"10.1.0.0/19".to_string()));
        assert!(!values.contains(&"10.1.0.0/1".to_string()));
    }

    #[test]
    fn loose_ipv6_completion_returns_canonical_networks() {
        for (term, expected) in [
            ("2001:db8/", "2001:db8::/32"),
            ("2001:db8/32", "2001:db8::/32"),
            ("2001:db8:1/48", "2001:db8:1::/48"),
            ("2001:db8::1/64", "2001:db8::/64"),
            ("2001:DB8::/32", "2001:db8::/32"),
        ] {
            assert_eq!(
                generate_cidr_candidates(term).first().map(String::as_str),
                Some(expected),
                "{term}"
            );
        }
    }

    #[test]
    fn invalid_or_incomplete_cidr_fragments_return_no_candidates() {
        for term in [
            "/8",
            "10../8",
            "10..1/8",
            "10.1.2.3./24",
            "10.300/8",
            "10/33",
            "10/a",
            "10/8/9",
            "2001::db8::/32",
            "2001:db8:/32",
            "2001:db8/129",
        ] {
            assert!(generate_cidr_candidates(term).is_empty(), "{term}");
        }
    }

    #[test]
    fn boundary_prefixes_are_supported_for_both_families() {
        assert_eq!(generate_cidr_candidates("10.1.2.3/0"), ["0.0.0.0/0"]);
        assert_eq!(generate_cidr_candidates("10.1.2.3/32"), ["10.1.2.3/32"]);
        assert_eq!(generate_cidr_candidates("2001:db8::1/0"), ["::/0"]);
        assert_eq!(
            generate_cidr_candidates("2001:db8::1/128"),
            ["2001:db8::1/128"]
        );
    }
}
