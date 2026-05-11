use super::decode::decode_remote_records;
use super::fetch::{build_client, fetch_source_once, parse_source_method};
use super::transform::{compile_transform, run_transform};
use super::*;
use std::net::IpAddr;
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
