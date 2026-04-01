use super::decode::decode_remote_records;
use super::fetch::{build_client, parse_source_method};
use super::transform::{compile_transform, run_transform};
use super::*;

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
fn build_client_accepts_explicit_insecure_tls_opt_out() {
    let tls = RemoteNetworkSourceTlsConfig {
        enable: true,
        verify: false,
        skip_verify: true,
        ..Default::default()
    };
    build_client(true, &tls).expect("explicit insecure client should initialize");
}
