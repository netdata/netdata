use super::decode::decode_remote_records;
use super::fetch::{build_client, fetch_source_once, parse_source_method};
use super::transform::{compile_transform, run_transform};
use super::*;
use std::hint::black_box;
use std::net::IpAddr;
use std::time::{Duration, Instant};
use tokio::io::{AsyncReadExt, AsyncWriteExt};
use tokio::net::TcpListener;
use tokio::task::JoinHandle;

#[test]
fn decode_remote_records_supports_results_transform() {
    let transform = compile_transform(".results[]").expect("compile transform");
    let payload = serde_json::json!({
        "results": [
            {"prefix": "198.51.100.0/24", "tenant": "t1", "asn": "AS64512"}
        ]
    });
    let rows = decode_remote_records(payload, &transform).expect("decode rows");
    assert_eq!(rows.len(), 1);
    assert_eq!(
        rows[0].prefix,
        "198.51.100.0/24".parse().expect("parse prefix")
    );
    assert_eq!(rows[0].attrs.tenant, "t1");
    assert_eq!(rows[0].attrs.asn, 64_512);
}

#[test]
fn decode_remote_records_supports_akvorado_style_transform() {
    let transform = compile_transform(
        r#"(.prefixes + .ipv6_prefixes)[] |
{ prefix: (.ip_prefix // .ipv6_prefix), tenant: "amazon", region: .region, role: .service|ascii_downcase }"#,
    )
    .expect("compile transform");
    let payload = serde_json::json!({
        "prefixes": [
            {"ip_prefix": "3.2.34.0/26", "region": "af-south-1", "service": "AMAZON"}
        ],
        "ipv6_prefixes": [
            {"ipv6_prefix": "2600:1ff2:4000::/40", "region": "us-west-2", "service": "ROUTE53_HEALTHCHECKS"}
        ]
    });
    let rows = decode_remote_records(payload, &transform).expect("decode rows");
    assert_eq!(rows.len(), 2);
    assert_eq!(
        rows[0].prefix,
        "3.2.34.0/26".parse::<IpNet>().expect("parse prefix")
    );
    assert_eq!(rows[0].attrs.tenant, "amazon");
    assert_eq!(rows[0].attrs.region, "af-south-1");
    assert_eq!(rows[0].attrs.role, "amazon");
    assert_eq!(
        rows[1].prefix,
        "2600:1ff2:4000::/40"
            .parse::<IpNet>()
            .expect("parse prefix")
    );
    assert_eq!(rows[1].attrs.role, "route53_healthchecks");
}

#[test]
fn documented_cloud_and_ipam_transforms_decode_provider_payloads() {
    let cases = [
        (
            "aws",
            r#"(.prefixes + .ipv6_prefixes)[] | {
  prefix: (.ip_prefix // .ipv6_prefix),
  tenant: "amazon",
  region: .region,
  role: (.service | ascii_downcase)
}"#,
            serde_json::json!({
                "prefixes": [
                    {"ip_prefix": "198.51.100.0/24", "region": "us-east-1", "service": "EC2"}
                ],
                "ipv6_prefixes": [
                    {"ipv6_prefix": "2001:db8:1::/48", "region": "eu-west-1", "service": "AMAZON"}
                ]
            }),
            vec![
                ("198.51.100.0/24", "amazon", "us-east-1", "ec2", "", ""),
                ("2001:db8:1::/48", "amazon", "eu-west-1", "amazon", "", ""),
            ],
        ),
        (
            "azure",
            r#".values[]
| .properties as $p
| $p.addressPrefixes[]
| {
    prefix: .,
    tenant: "azure",
    region: ($p.region // ""),
    role:   (($p.systemService // "") | ascii_downcase)
  }"#,
            serde_json::json!({
                "values": [
                    {
                        "name": "AzureCloud.eastus",
                        "properties": {
                            "region": "eastus",
                            "systemService": "AzureStorage",
                            "addressPrefixes": ["203.0.113.0/24"]
                        }
                    }
                ]
            }),
            vec![("203.0.113.0/24", "azure", "eastus", "azurestorage", "", "")],
        ),
        (
            "gcp",
            r#".prefixes[] | {
  prefix: (.ipv4Prefix // .ipv6Prefix),
  tenant: "gcp",
  role: "google-cloud",
  region: .scope
}"#,
            serde_json::json!({
                "prefixes": [
                    {"ipv4Prefix": "192.0.2.0/24", "scope": "us-central1"},
                    {"ipv6Prefix": "2001:db8:2::/48", "scope": "europe-west1"}
                ]
            }),
            vec![
                ("192.0.2.0/24", "gcp", "us-central1", "google-cloud", "", ""),
                (
                    "2001:db8:2::/48",
                    "gcp",
                    "europe-west1",
                    "google-cloud",
                    "",
                    "",
                ),
            ],
        ),
        (
            "netbox",
            r#".results[] | {
  prefix: .prefix,
  tenant: (.tenant.name // ""),
  role:   (.role.name // ""),
  site:   (.scope.name // ""),
  name:   (.description // "")
}"#,
            serde_json::json!({
                "results": [
                    {
                        "prefix": "198.51.100.0/25",
                        "tenant": {"name": "tenant-a"},
                        "role": {"name": "edge"},
                        "scope": {"name": "dc1"},
                        "description": "edge subnet"
                    }
                ]
            }),
            vec![(
                "198.51.100.0/25",
                "tenant-a",
                "",
                "edge",
                "dc1",
                "edge subnet",
            )],
        ),
        (
            "generic",
            r#".[] | {
  prefix: .prefix,
  name: .name,
  tenant: .env
}"#,
            serde_json::json!([
                {"prefix": "203.0.113.0/25", "name": "app subnet", "env": "prod"}
            ]),
            vec![("203.0.113.0/25", "prod", "", "", "", "app subnet")],
        ),
    ];

    for (name, expression, payload, expected) in cases {
        let transform = compile_transform(expression)
            .unwrap_or_else(|err| panic!("{name} transform should compile: {err}"));
        let rows = decode_remote_records(payload, &transform)
            .unwrap_or_else(|err| panic!("{name} transform should decode: {err}"));
        assert_eq!(rows.len(), expected.len(), "{name} row count mismatch");
        for (idx, (prefix, tenant, region, role, site, net_name)) in expected.iter().enumerate() {
            assert_eq!(
                rows[idx].prefix,
                prefix.parse::<IpNet>().expect("parse expected prefix"),
                "{name} prefix mismatch at row {idx}"
            );
            assert_eq!(rows[idx].attrs.tenant, *tenant, "{name} tenant mismatch");
            assert_eq!(rows[idx].attrs.region, *region, "{name} region mismatch");
            assert_eq!(rows[idx].attrs.role, *role, "{name} role mismatch");
            assert_eq!(rows[idx].attrs.site, *site, "{name} site mismatch");
            assert_eq!(rows[idx].attrs.name, *net_name, "{name} name mismatch");
        }
    }
}

#[test]
fn decode_remote_records_preserves_optional_asn_name() {
    let transform = compile_transform(".results[]").expect("compile transform");
    let payload = serde_json::json!({
        "results": [
            {
                "prefix": "203.0.113.0/24",
                "asn": 64500,
                "asn_name": "Example Transit"
            }
        ]
    });

    let rows = decode_remote_records(payload, &transform).expect("decode rows");
    assert_eq!(rows.len(), 1);
    assert_eq!(rows[0].attrs.asn, 64_500);
    assert_eq!(rows[0].attrs.asn_name, "Example Transit");
}

#[test]
fn compile_transform_rejects_invalid_syntax() {
    let err = compile_transform("(.foo[] |").expect_err("expected compile error");
    assert!(err.to_string().contains("transform"));
}

#[test]
fn decode_remote_records_empty_transform_defaults_to_identity() {
    let transform = compile_transform("").expect("compile transform");
    let payload = serde_json::json!({
        "prefix": "198.51.100.0/24",
        "tenant": "identity-default",
        "asn": "AS64512"
    });

    let rows = decode_remote_records(payload, &transform).expect("decode rows");
    assert_eq!(rows.len(), 1);
    assert_eq!(
        rows[0].prefix,
        "198.51.100.0/24".parse::<IpNet>().expect("parse prefix")
    );
    assert_eq!(rows[0].attrs.tenant, "identity-default");
    assert_eq!(rows[0].attrs.asn, 64_512);
}

#[test]
fn parse_source_method_accepts_get_and_post_only() {
    assert_eq!(
        parse_source_method("get").expect("GET should be accepted"),
        Method::GET
    );
    assert_eq!(
        parse_source_method("POST").expect("POST should be accepted"),
        Method::POST
    );

    let err = parse_source_method("PUT").expect_err("PUT should be rejected");
    assert!(err.to_string().contains("expected GET or POST"));
}

#[test]
fn run_transform_runtime_error_is_reported() {
    let transform = compile_transform(r#"error("boom")"#).expect("compile transform");
    let payload = serde_json::json!({
        "prefix": "198.51.100.0/24"
    });

    let err = run_transform(payload, &transform).expect_err("expected runtime jq error");
    assert!(err.to_string().contains("failed to execute transform"));
}

#[test]
fn decode_remote_records_empty_transform_result_is_rejected() {
    let transform = compile_transform(r#".results[] | select(false)"#).expect("compile transform");
    let payload = serde_json::json!({
        "results": [
            {"prefix": "198.51.100.0/24", "tenant": "t1"}
        ]
    });

    let err = decode_remote_records(payload, &transform).expect_err("expected empty result");
    assert!(err.to_string().contains("produced empty result"));
}

#[test]
fn decode_remote_records_non_object_row_is_rejected() {
    let transform = compile_transform(r#".results[] | .prefix"#).expect("compile transform");
    let payload = serde_json::json!({
        "results": [
            {"prefix": "198.51.100.0/24", "tenant": "t1"}
        ]
    });

    let err = decode_remote_records(payload, &transform).expect_err("expected map error");
    assert!(
        err.to_string()
            .contains("cannot map remote network source row")
    );
}

#[test]
fn network_sources_runtime_matches_only_containing_prefixes() {
    let runtime = NetworkSourcesRuntime::default();
    runtime.replace_records(vec![
        network_source_record("10.0.0.0/8", "root", "", ""),
        network_source_record("10.1.0.0/16", "match", "", ""),
        network_source_record("10.2.0.0/16", "sibling", "", ""),
    ]);

    let matches = runtime.matching_attributes_ascending("10.1.1.1".parse().expect("address"));
    let names = matches
        .iter()
        .map(|(_, attrs)| attrs.name.as_str())
        .collect::<Vec<_>>();

    assert_prefix_lengths_ascending(&matches);
    assert_eq!(names, vec!["root", "match"]);
}

#[test]
fn network_sources_runtime_indexed_matches_only_containing_prefixes() {
    let runtime = NetworkSourcesRuntime::default();
    let mut records = vec![
        network_source_record("10.0.0.0/8", "root", "", ""),
        network_source_record("10.1.0.0/16", "match", "", ""),
        network_source_record("10.2.0.0/16", "sibling", "", ""),
    ];
    add_ipv4_indexed_lookup_fillers(&mut records);
    runtime.replace_records(records);

    let matches = runtime.matching_attributes_ascending("10.1.1.1".parse().expect("address"));
    let names = matches
        .iter()
        .map(|(_, attrs)| attrs.name.as_str())
        .collect::<Vec<_>>();

    assert_prefix_lengths_ascending(&matches);
    assert_eq!(names, vec!["root", "match"]);
}

#[test]
fn network_sources_runtime_indexed_lookup_preserves_match_semantics() {
    let runtime = NetworkSourcesRuntime::default();
    let mut records = vec![
        network_source_record("10.0.0.0/8", "root", "", ""),
        network_source_record("10.1.0.0/16", "match-first", "", ""),
        network_source_record("10.1.0.0/16", "match-second", "", ""),
        network_source_record("10.2.0.0/16", "sibling", "", ""),
    ];
    add_ipv4_indexed_lookup_fillers(&mut records);
    runtime.replace_records(records);

    let matches = runtime.matching_attributes_ascending("10.1.1.1".parse().expect("address"));
    let names = matches
        .iter()
        .map(|(_, attrs)| attrs.name.as_str())
        .collect::<Vec<_>>();

    assert_prefix_lengths_ascending(&matches);
    assert_eq!(names, vec!["root", "match-first", "match-second"]);
}

#[test]
fn network_sources_runtime_indexed_ipv6_lookup_preserves_match_semantics() {
    let runtime = NetworkSourcesRuntime::default();
    let mut records = vec![
        network_source_record("2001:db8::/32", "root", "", ""),
        network_source_record("2001:db8:1::/48", "match-first", "", ""),
        network_source_record("2001:db8:1::/48", "match-second", "", ""),
        network_source_record("2001:db8:2::/48", "sibling", "", ""),
    ];
    add_ipv6_indexed_lookup_fillers(&mut records);
    runtime.replace_records(records);

    let matches = runtime.matching_attributes_ascending("2001:db8:1::42".parse().expect("address"));
    let names = matches
        .iter()
        .map(|(_, attrs)| attrs.name.as_str())
        .collect::<Vec<_>>();

    assert_prefix_lengths_ascending(&matches);
    assert_eq!(names, vec!["root", "match-first", "match-second"]);
}

#[test]
fn network_sources_runtime_preserves_same_prefix_publish_order() {
    let runtime = NetworkSourcesRuntime::default();
    runtime.replace_records(vec![
        network_source_record("198.51.100.0/24", "first", "", "tenant-a"),
        network_source_record("198.51.100.0/24", "second", "edge", ""),
    ]);

    let matches = runtime.matching_attributes_ascending("198.51.100.42".parse().expect("address"));
    let values = matches
        .iter()
        .map(|(prefix_len, attrs)| (*prefix_len, attrs.name.as_str(), attrs.role.as_str()))
        .collect::<Vec<_>>();

    assert_prefix_lengths_ascending(&matches);
    assert_eq!(values, vec![(24, "first", ""), (24, "second", "edge")]);
}

#[test]
fn network_sources_runtime_indexed_preserves_same_prefix_publish_order() {
    let runtime = NetworkSourcesRuntime::default();
    let mut records = vec![
        network_source_record("198.51.100.0/24", "first", "", "tenant-a"),
        network_source_record("198.51.100.0/24", "second", "edge", ""),
    ];
    add_ipv4_indexed_lookup_fillers(&mut records);
    runtime.replace_records(records);

    let matches = runtime.matching_attributes_ascending("198.51.100.42".parse().expect("address"));
    let values = matches
        .iter()
        .map(|(prefix_len, attrs)| (*prefix_len, attrs.name.as_str(), attrs.role.as_str()))
        .collect::<Vec<_>>();

    assert_prefix_lengths_ascending(&matches);
    assert_eq!(values, vec![(24, "first", ""), (24, "second", "edge")]);
}

#[test]
fn network_sources_runtime_keeps_noncanonical_prefix_behavior() {
    let runtime = NetworkSourcesRuntime::default();
    runtime.replace_records(vec![network_source_record(
        "10.1.1.1/8",
        "noncanonical-root",
        "",
        "",
    )]);

    let matches = runtime.matching_attributes_ascending("10.2.2.2".parse().expect("address"));

    assert_prefix_lengths_ascending(&matches);
    assert_eq!(matches.len(), 1);
    assert_eq!(matches[0].0, 8);
    assert_eq!(matches[0].1.name, "noncanonical-root");
}

#[test]
fn network_sources_runtime_indexed_keeps_noncanonical_prefix_behavior() {
    let runtime = NetworkSourcesRuntime::default();
    let mut records = vec![network_source_record(
        "10.1.1.1/8",
        "noncanonical-root",
        "",
        "",
    )];
    add_ipv4_indexed_lookup_fillers(&mut records);
    runtime.replace_records(records);

    let matches = runtime.matching_attributes_ascending("10.2.2.2".parse().expect("address"));

    assert_prefix_lengths_ascending(&matches);
    assert_eq!(matches.len(), 1);
    assert_eq!(matches[0].0, 8);
    assert_eq!(matches[0].1.name, "noncanonical-root");
}

#[test]
fn network_sources_runtime_indexed_ipv6_keeps_noncanonical_prefix_behavior() {
    let runtime = NetworkSourcesRuntime::default();
    let mut records = vec![network_source_record(
        "2001:db8:1::42/32",
        "noncanonical-root",
        "",
        "",
    )];
    add_ipv6_indexed_lookup_fillers(&mut records);
    runtime.replace_records(records);

    let matches = runtime.matching_attributes_ascending("2001:db8:2::1".parse().expect("address"));

    assert_prefix_lengths_ascending(&matches);
    assert_eq!(matches.len(), 1);
    assert_eq!(matches[0].0, 32);
    assert_eq!(matches[0].1.name, "noncanonical-root");
}

#[test]
fn network_sources_runtime_indexed_matches_default_route() {
    let runtime = NetworkSourcesRuntime::default();
    let mut records = vec![network_source_record("0.0.0.0/0", "default", "", "")];
    add_ipv4_indexed_lookup_fillers(&mut records);
    runtime.replace_records(records);

    let matches = runtime.matching_attributes_ascending("198.51.100.42".parse().expect("address"));

    assert_prefix_lengths_ascending(&matches);
    assert_eq!(matches.len(), 1);
    assert_eq!(matches[0].0, 0);
    assert_eq!(matches[0].1.name, "default");
}

#[test]
fn network_sources_runtime_indexed_ipv6_matches_default_route() {
    let runtime = NetworkSourcesRuntime::default();
    let mut records = vec![network_source_record("::/0", "default", "", "")];
    add_ipv6_indexed_lookup_fillers(&mut records);
    runtime.replace_records(records);

    let matches = runtime.matching_attributes_ascending("2001:db8:1::42".parse().expect("address"));

    assert_prefix_lengths_ascending(&matches);
    assert_eq!(matches.len(), 1);
    assert_eq!(matches[0].0, 0);
    assert_eq!(matches[0].1.name, "default");
}

#[test]
fn network_sources_runtime_mixed_family_small_lookup_preserves_match_semantics() {
    let runtime = NetworkSourcesRuntime::default();
    runtime.replace_records(vec![
        network_source_record("2001:db8::/32", "v6-root", "", ""),
        network_source_record("198.51.0.0/16", "v4-root", "", ""),
        network_source_record("2001:db8:1::/48", "v6-match", "", ""),
        network_source_record("198.51.100.0/24", "v4-match", "", ""),
    ]);

    let ipv4_matches =
        runtime.matching_attributes_ascending("198.51.100.42".parse().expect("address"));
    let ipv6_matches =
        runtime.matching_attributes_ascending("2001:db8:1::42".parse().expect("address"));
    let ipv4_names = ipv4_matches
        .iter()
        .map(|(_, attrs)| attrs.name.as_str())
        .collect::<Vec<_>>();
    let ipv6_names = ipv6_matches
        .iter()
        .map(|(_, attrs)| attrs.name.as_str())
        .collect::<Vec<_>>();

    assert_prefix_lengths_ascending(&ipv4_matches);
    assert_prefix_lengths_ascending(&ipv6_matches);
    assert_eq!(ipv4_names, vec!["v4-root", "v4-match"]);
    assert_eq!(ipv6_names, vec!["v6-root", "v6-match"]);
}

#[test]
fn network_sources_runtime_mixed_family_large_linear_lookup_preserves_match_semantics() {
    let runtime = NetworkSourcesRuntime::default();
    let mut records = vec![
        network_source_record("198.51.0.0/16", "v4-root", "", ""),
        network_source_record("198.51.100.0/24", "v4-match", "", ""),
    ];
    for index in 0..IPV4_INDEX_MIN_FAMILY_RECORDS {
        records.push(network_source_record(
            &format!("2001:db8:{:x}::/48", 0x1000 + index),
            "v6-filler",
            "",
            "",
        ));
    }
    runtime.replace_records(records);

    let matches = runtime.matching_attributes_ascending("198.51.100.42".parse().expect("address"));
    let names = matches
        .iter()
        .map(|(_, attrs)| attrs.name.as_str())
        .collect::<Vec<_>>();

    assert_prefix_lengths_ascending(&matches);
    assert_eq!(names, vec!["v4-root", "v4-match"]);
}

#[test]
fn network_sources_runtime_replaces_records_without_stale_index_matches() {
    let runtime = NetworkSourcesRuntime::default();
    let mut first = vec![network_source_record("10.0.0.0/8", "old", "", "")];
    add_ipv4_indexed_lookup_fillers(&mut first);
    runtime.replace_records(first);

    let mut second = vec![network_source_record("192.0.2.0/24", "new", "", "")];
    add_ipv4_indexed_lookup_fillers(&mut second);
    runtime.replace_records(second);

    let stale_matches = runtime.matching_attributes_ascending("10.1.1.1".parse().expect("address"));
    let current_matches =
        runtime.matching_attributes_ascending("192.0.2.10".parse().expect("address"));

    assert!(stale_matches.is_empty());
    assert_prefix_lengths_ascending(&current_matches);
    assert_eq!(current_matches.len(), 1);
    assert_eq!(current_matches[0].1.name, "new");
}

#[test]
#[ignore = "manual network-source runtime lookup benchmark"]
fn bench_network_sources_runtime_lookup_matrix() {
    let cases = [
        ("ipv4", 32, "198.51.100.42"),
        ("ipv4", 33, "198.51.100.42"),
        ("ipv4", 34, "198.51.100.42"),
        ("ipv4", 128, "198.51.100.42"),
        ("ipv4", 129, "198.51.100.42"),
        ("ipv4", 500, "198.51.100.42"),
        ("ipv4", 2_000, "198.51.100.42"),
        ("ipv4", 10_000, "198.51.100.42"),
        ("ipv6", 128, "2001:db8:1::42"),
        ("ipv6", 129, "2001:db8:1::42"),
        ("ipv6", 130, "2001:db8:1::42"),
        ("ipv6", 500, "2001:db8:1::42"),
        ("ipv6", 2_000, "2001:db8:1::42"),
        ("ipv6", 10_000, "2001:db8:1::42"),
    ];

    eprintln!();
    eprintln!("=== Network Source Runtime Lookup Matrix ===");
    eprintln!("family records rounds runtime_ns linear_ns speedup");
    for (family, record_count, address) in cases {
        let address = address.parse().expect("address");
        let records = match family {
            "ipv4" => benchmark_records_ipv4(record_count),
            "ipv6" => benchmark_records_ipv6(record_count),
            _ => unreachable!("benchmark family"),
        };
        let runtime = NetworkSourcesRuntime::default();
        runtime.replace_records(records.clone());
        let rounds = benchmark_lookup_rounds(record_count);

        let runtime_elapsed =
            time_lookup_rounds(rounds, || runtime.matching_attributes_ascending(address));
        let linear_elapsed =
            time_lookup_rounds(rounds, || benchmark_linear_matches(&records, address));

        let runtime_ns = elapsed_nanos_per_round(runtime_elapsed, rounds);
        let linear_ns = elapsed_nanos_per_round(linear_elapsed, rounds);
        let speedup = linear_ns / runtime_ns.max(f64::EPSILON);
        eprintln!("{family} {record_count} {rounds} {runtime_ns:.1} {linear_ns:.1} {speedup:.2}x");
    }

    eprintln!();
    eprintln!("case ipv4_records ipv6_records rounds runtime_ns linear_ns speedup");
    for (label, ipv4_count, ipv6_count, address) in [
        ("mixed-ipv4-linear", 128, 128, "198.51.100.42"),
        ("mixed-ipv6-linear", 128, 128, "2001:db8:1::42"),
        ("mixed-ipv4-indexed", 500, 500, "198.51.100.42"),
        ("mixed-ipv6-indexed", 500, 2_000, "2001:db8:1::42"),
    ] {
        let address = address.parse().expect("address");
        let records = benchmark_records_mixed(ipv4_count, ipv6_count);
        let runtime = NetworkSourcesRuntime::default();
        runtime.replace_records(records.clone());
        let rounds = benchmark_lookup_rounds(ipv4_count + ipv6_count);

        let runtime_elapsed =
            time_lookup_rounds(rounds, || runtime.matching_attributes_ascending(address));
        let linear_elapsed =
            time_lookup_rounds(rounds, || benchmark_linear_matches(&records, address));

        let runtime_ns = elapsed_nanos_per_round(runtime_elapsed, rounds);
        let linear_ns = elapsed_nanos_per_round(linear_elapsed, rounds);
        let speedup = linear_ns / runtime_ns.max(f64::EPSILON);
        eprintln!(
            "{label} {ipv4_count} {ipv6_count} {rounds} {runtime_ns:.1} {linear_ns:.1} {speedup:.2}x"
        );
    }
}

fn add_ipv4_indexed_lookup_fillers(records: &mut Vec<NetworkSourceRecord>) {
    for index in 0..(IPV4_INDEX_MIN_FAMILY_RECORDS * 2) {
        records.push(network_source_record(
            &format!("172.{}.{}.0/24", 16 + index / 256, index % 256),
            "filler",
            "",
            "",
        ));
    }
}

fn add_ipv6_indexed_lookup_fillers(records: &mut Vec<NetworkSourceRecord>) {
    for index in 0..(IPV6_INDEX_MIN_FAMILY_RECORDS * 2) {
        records.push(network_source_record(
            &format!("2001:db8:{:x}::/48", 0x1000 + index),
            "filler",
            "",
            "",
        ));
    }
}

fn benchmark_records_ipv4(record_count: usize) -> Vec<NetworkSourceRecord> {
    let mut records = vec![
        network_source_record("198.51.0.0/16", "root", "", ""),
        network_source_record("198.51.100.0/24", "match", "", ""),
        network_source_record("198.51.200.0/24", "sibling", "", ""),
    ];
    while records.len() < record_count {
        let index = records.len();
        let octet2 = (index / 256) % 256;
        let octet3 = index % 256;
        records.push(network_source_record(
            &format!("10.{octet2}.{octet3}.0/24"),
            "filler",
            "",
            "",
        ));
    }
    records
}

fn benchmark_records_ipv6(record_count: usize) -> Vec<NetworkSourceRecord> {
    let mut records = vec![
        network_source_record("2001:db8::/32", "root", "", ""),
        network_source_record("2001:db8:1::/48", "match", "", ""),
        network_source_record("2001:db8:2::/48", "sibling", "", ""),
    ];
    while records.len() < record_count {
        let index = records.len();
        records.push(network_source_record(
            &format!("2001:db8:{:x}::/48", 0x1000 + index),
            "filler",
            "",
            "",
        ));
    }
    records
}

fn benchmark_records_mixed(ipv4_count: usize, ipv6_count: usize) -> Vec<NetworkSourceRecord> {
    let mut records = benchmark_records_ipv4(ipv4_count);
    records.extend(benchmark_records_ipv6(ipv6_count));
    records
}

fn benchmark_lookup_rounds(record_count: usize) -> usize {
    if let Ok(value) = std::env::var("NETFLOW_NETWORK_SOURCE_BENCH_ROUNDS") {
        return value.parse().expect("valid benchmark rounds");
    }
    (1_000_000 / record_count.max(1)).clamp(100, 20_000)
}

fn time_lookup_rounds<F>(rounds: usize, mut lookup: F) -> Duration
where
    F: FnMut() -> Vec<(u8, crate::enrichment::NetworkAttributes)>,
{
    let mut checksum = 0_usize;
    let started = Instant::now();
    for _ in 0..rounds {
        checksum = checksum.wrapping_add(black_box(lookup()).len());
    }
    black_box(checksum);
    started.elapsed()
}

fn elapsed_nanos_per_round(elapsed: Duration, rounds: usize) -> f64 {
    elapsed.as_nanos() as f64 / rounds.max(1) as f64
}

fn benchmark_linear_matches(
    records: &[NetworkSourceRecord],
    address: IpAddr,
) -> Vec<(u8, crate::enrichment::NetworkAttributes)> {
    let mut matches = Vec::new();
    for record in records {
        if record.prefix.contains(&address) {
            matches.push((record.prefix.prefix_len(), record.attrs.clone()));
        }
    }
    matches.sort_by_key(|(prefix_len, _)| *prefix_len);
    matches
}

fn assert_prefix_lengths_ascending(matches: &[(u8, crate::enrichment::NetworkAttributes)]) {
    assert!(
        matches.windows(2).all(|items| items[0].0 <= items[1].0),
        "network-source matches must be ascending by prefix length"
    );
}

#[test]
fn build_client_accepts_default_tls_config() {
    let tls = RemoteNetworkSourceTlsConfig::default();
    build_client(true, &tls).expect("default client should initialize");
}

#[test]
fn build_client_fails_on_missing_ca_file_when_tls_enabled() {
    let tls = RemoteNetworkSourceTlsConfig {
        enable: true,
        ca_file: "/missing/ca.pem".to_string(),
        ..Default::default()
    };
    let err = build_client(true, &tls).expect_err("expected missing CA file error");
    assert!(err.to_string().contains("network source CA file"));
}

#[test]
fn build_client_rejects_explicit_insecure_tls_opt_out() {
    let tls = RemoteNetworkSourceTlsConfig {
        enable: true,
        verify: false,
        skip_verify: true,
        ..Default::default()
    };
    let err = build_client(true, &tls).expect_err("expected insecure TLS error");
    assert!(
        err.to_string()
            .contains("certificate verification cannot be disabled")
    );
}

#[tokio::test]
async fn fetch_source_once_performs_http_get_and_decodes_records() {
    let (url, request_task) = serve_json_once(
        "200 OK",
        r#"{"results":[{"prefix":"198.51.100.0/24","tenant":"runtime","asn":"AS64512"}]}"#,
    )
    .await;
    let source = RemoteNetworkSourceConfig {
        url,
        headers: BTreeMap::from([("x-netdata-test".to_string(), "present".to_string())]),
        proxy: false,
        transform: ".results[]".to_string(),
        ..Default::default()
    };
    let client = build_client(false, &source.tls).expect("build client");
    let transform = compile_transform(&source.transform).expect("compile transform");

    let records = fetch_source_once(&client, &source, &transform)
        .await
        .expect("fetch source");

    assert_eq!(records.len(), 1);
    assert_eq!(
        records[0].prefix,
        "198.51.100.0/24"
            .parse::<IpNet>()
            .expect("parse expected prefix")
    );
    assert_eq!(records[0].attrs.tenant, "runtime");
    assert_eq!(records[0].attrs.asn, 64_512);

    let request = request_task.await.expect("join HTTP server");
    assert!(request.starts_with("GET / HTTP/1.1"), "{request}");
    assert!(
        request
            .to_ascii_lowercase()
            .contains("x-netdata-test: present"),
        "{request}"
    );
}

#[tokio::test]
async fn fetch_source_once_rejects_http_error_status() {
    let (url, request_task) = serve_json_once("503 Service Unavailable", r#"{}"#).await;
    let source = RemoteNetworkSourceConfig {
        url,
        proxy: false,
        transform: ".".to_string(),
        ..Default::default()
    };
    let client = build_client(false, &source.tls).expect("build client");
    let transform = compile_transform(&source.transform).expect("compile transform");

    let err = fetch_source_once(&client, &source, &transform)
        .await
        .expect_err("HTTP error should fail");

    assert!(err.to_string().contains("unexpected HTTP status 503"));
    let _ = request_task.await.expect("join HTTP server");
}

#[tokio::test]
async fn network_sources_refresher_publishes_first_successful_fetch_to_runtime() {
    let (url, request_task) = serve_json_once(
        "200 OK",
        r#"[{"prefix":"198.51.100.0/24","name":"runtime-net","tenant":"runtime"}]"#,
    )
    .await;
    let runtime = NetworkSourcesRuntime::default();
    let shutdown = CancellationToken::new();
    let source = RemoteNetworkSourceConfig {
        url,
        proxy: false,
        transform: ".[]".to_string(),
        ..Default::default()
    };
    let refresher_runtime = runtime.clone();
    let refresher_shutdown = shutdown.clone();
    let task = tokio::spawn(async move {
        run_network_sources_refresher(
            BTreeMap::from([("ipam".to_string(), source)]),
            refresher_runtime,
            refresher_shutdown,
        )
        .await
    });

    tokio::time::timeout(Duration::from_secs(3), async {
        let address: IpAddr = "198.51.100.42".parse().expect("parse address");
        loop {
            let matches = runtime.matching_attributes_ascending(address);
            if let Some((prefix_len, attrs)) = matches.first() {
                assert_eq!(*prefix_len, 24);
                assert_eq!(attrs.name, "runtime-net");
                assert_eq!(attrs.tenant, "runtime");
                break;
            }
            tokio::time::sleep(Duration::from_millis(25)).await;
        }
    })
    .await
    .expect("runtime did not receive network-source records");

    let _ = request_task.await.expect("join HTTP server");
    shutdown.cancel();
    task.await
        .expect("join refresher")
        .expect("refresher should stop cleanly");
}

fn network_source_record(
    prefix: &str,
    name: &str,
    role: &str,
    tenant: &str,
) -> NetworkSourceRecord {
    NetworkSourceRecord {
        prefix: prefix.parse().expect("parse network-source prefix"),
        attrs: crate::enrichment::NetworkAttributes {
            name: name.to_string(),
            role: role.to_string(),
            tenant: tenant.to_string(),
            ..Default::default()
        },
    }
}

async fn serve_json_once(status: &'static str, body: &'static str) -> (String, JoinHandle<String>) {
    let listener = TcpListener::bind("127.0.0.1:0")
        .await
        .expect("bind HTTP fixture server");
    let url = format!(
        "http://{}",
        listener.local_addr().expect("fixture server addr")
    );
    let task = tokio::spawn(async move {
        let (mut stream, _) = listener.accept().await.expect("accept HTTP request");
        let mut buffer = vec![0_u8; 4096];
        let n = stream.read(&mut buffer).await.expect("read HTTP request");
        let request = String::from_utf8_lossy(&buffer[..n]).into_owned();
        let response = format!(
            "HTTP/1.1 {status}\r\ncontent-type: application/json\r\ncontent-length: {}\r\nconnection: close\r\n\r\n{body}",
            body.len()
        );
        stream
            .write_all(response.as_bytes())
            .await
            .expect("write HTTP response");
        request
    });

    (url, task)
}
