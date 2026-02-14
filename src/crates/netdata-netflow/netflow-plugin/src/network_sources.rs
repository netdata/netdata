use crate::enrichment::{NetworkAttributes, NetworkSourceRecord, NetworkSourcesRuntime};
use crate::plugin_config::{RemoteNetworkSourceConfig, RemoteNetworkSourceTlsConfig};
use anyhow::{Context, Result};
use ipnet::IpNet;
use jaq_interpret::{Ctx, Filter, FilterT, ParseCtx, RcIter, Val};
use reqwest::{Certificate, Client, Identity, Method};
use serde::Deserialize;
use serde_json::Value;
use std::collections::BTreeMap;
use std::fs;
use std::str::FromStr;
use std::sync::{Arc, RwLock};
use std::time::Duration;
use tokio::task::JoinSet;
use tokio::time::MissedTickBehavior;
use tokio_util::sync::CancellationToken;

#[derive(Debug, Default, Deserialize)]
struct RemoteRecord {
    prefix: String,
    #[serde(default)]
    name: String,
    #[serde(default)]
    role: String,
    #[serde(default)]
    site: String,
    #[serde(default)]
    region: String,
    #[serde(default)]
    country: String,
    #[serde(default)]
    state: String,
    #[serde(default)]
    city: String,
    #[serde(default)]
    tenant: String,
    #[serde(default)]
    asn: AsnValue,
}

#[derive(Debug, Default, Deserialize)]
#[serde(untagged)]
enum AsnValue {
    #[default]
    Empty,
    Number(u32),
    Text(String),
}

#[derive(Debug, Clone)]
struct SourceRecordState {
    by_source: Arc<RwLock<BTreeMap<String, Vec<NetworkSourceRecord>>>>,
}

#[derive(Debug, Clone)]
struct CompiledTransform {
    expression: String,
    filter: Filter,
}

pub(crate) async fn run_network_sources_refresher(
    sources: BTreeMap<String, RemoteNetworkSourceConfig>,
    runtime: NetworkSourcesRuntime,
    shutdown: CancellationToken,
) -> Result<()> {
    if sources.is_empty() {
        return Ok(());
    }

    let state = SourceRecordState {
        by_source: Arc::new(RwLock::new(BTreeMap::new())),
    };

    let mut tasks = JoinSet::new();
    for (name, source) in sources {
        let runtime = runtime.clone();
        let state = state.clone();
        let shutdown = shutdown.clone();
        tasks.spawn(async move {
            run_source_loop(name, source, runtime, state, shutdown).await;
        });
    }

    shutdown.cancelled().await;
    tasks.abort_all();
    while let Some(result) = tasks.join_next().await {
        if let Err(err) = result
            && !err.is_cancelled()
        {
            tracing::warn!("network-sources task join error: {}", err);
        }
    }

    Ok(())
}

async fn run_source_loop(
    name: String,
    source: RemoteNetworkSourceConfig,
    runtime: NetworkSourcesRuntime,
    state: SourceRecordState,
    shutdown: CancellationToken,
) {
    let transform = match compile_transform(&source.transform) {
        Ok(compiled) => compiled,
        Err(err) => {
            tracing::warn!(
                "network-sources source '{}' disabled: invalid transform: {:#}",
                name,
                err
            );
            return;
        }
    };

    let client = match build_client(source.proxy, &source.tls) {
        Ok(client) => client,
        Err(err) => {
            tracing::warn!(
                "network-sources source '{}' disabled: cannot initialize HTTP client: {:#}",
                name,
                err
            );
            return;
        }
    };

    let regular_interval = source.interval.max(Duration::from_secs(60));
    let mut retry_interval = regular_interval / 10;
    if retry_interval < Duration::from_secs(1) {
        retry_interval = Duration::from_secs(1);
    }

    let mut next_wait = Duration::ZERO;
    let mut ticker = tokio::time::interval(regular_interval);
    ticker.set_missed_tick_behavior(MissedTickBehavior::Skip);

    loop {
        if next_wait.is_zero() {
            match fetch_source_once(&client, &source, &transform).await {
                Ok(records) => {
                    publish_source_records(&name, records, &runtime, &state);
                    next_wait = regular_interval;
                    retry_interval = (regular_interval / 10).max(Duration::from_secs(1));
                }
                Err(err) => {
                    tracing::warn!(
                        "network-sources source '{}' refresh failed: {:#}",
                        name,
                        err
                    );
                    next_wait = retry_interval;
                    retry_interval = std::cmp::min(retry_interval * 2, regular_interval);
                }
            }
        }

        tokio::select! {
            _ = shutdown.cancelled() => return,
            _ = tokio::time::sleep(next_wait) => {
                next_wait = Duration::ZERO;
            }
            _ = ticker.tick() => {
                if next_wait > regular_interval {
                    next_wait = regular_interval;
                }
            }
        }
    }
}

fn publish_source_records(
    source_name: &str,
    records: Vec<NetworkSourceRecord>,
    runtime: &NetworkSourcesRuntime,
    state: &SourceRecordState,
) {
    match state.by_source.write() {
        Ok(mut guard) => {
            guard.insert(source_name.to_string(), records);
            let merged = guard
                .values()
                .flat_map(|items| items.iter().cloned())
                .collect::<Vec<_>>();
            runtime.replace_records(merged);
        }
        Err(err) => {
            tracing::warn!(
                "network-sources source '{}' failed to publish records due to poisoned lock: {}",
                source_name,
                err
            );
        }
    }
}

async fn fetch_source_once(
    client: &Client,
    source: &RemoteNetworkSourceConfig,
    transform: &CompiledTransform,
) -> Result<Vec<NetworkSourceRecord>> {
    let method =
        Method::from_str(&source.method.trim().to_ascii_uppercase()).with_context(|| {
            format!(
                "unsupported method '{}' (expected GET or POST)",
                source.method
            )
        })?;
    let mut request = client
        .request(method, source.url.trim())
        .timeout(source.timeout.max(Duration::from_secs(1)))
        .header("accept", "application/json");
    for (name, value) in &source.headers {
        request = request.header(name, value);
    }

    let response = request
        .send()
        .await
        .with_context(|| format!("failed HTTP request to {}", source.url))?;
    if !response.status().is_success() {
        anyhow::bail!(
            "unexpected HTTP status {} for {}",
            response.status(),
            source.url
        );
    }

    let payload: Value = response
        .json()
        .await
        .with_context(|| format!("invalid JSON response from {}", source.url))?;
    decode_remote_records(payload, transform)
}

fn decode_remote_records(
    payload: Value,
    transform: &CompiledTransform,
) -> Result<Vec<NetworkSourceRecord>> {
    let items = run_transform(payload, transform)?;

    let mut out = Vec::with_capacity(items.len());
    for item in items {
        let remote: RemoteRecord =
            serde_json::from_value(item).context("cannot map remote network source row")?;
        let prefix = parse_remote_prefix(&remote.prefix)?;
        let attrs = NetworkAttributes {
            name: remote.name,
            role: remote.role,
            site: remote.site,
            region: remote.region,
            country: remote.country,
            state: remote.state,
            city: remote.city,
            tenant: remote.tenant,
            asn: decode_remote_asn(remote.asn),
        };
        out.push(NetworkSourceRecord { prefix, attrs });
    }

    if out.is_empty() {
        anyhow::bail!("empty result");
    }

    Ok(out)
}

fn compile_transform(expression: &str) -> Result<CompiledTransform> {
    let normalized = if expression.trim().is_empty() {
        ".".to_string()
    } else {
        expression.trim().to_string()
    };

    let tokens = jaq_syn::Lexer::new(&normalized)
        .lex()
        .map_err(|errs| anyhow::anyhow!("failed to lex transform '{}': {:?}", normalized, errs))?;
    let main = jaq_syn::Parser::new(&tokens)
        .parse(|parser| parser.module(|module| module.term()))
        .map_err(|errs| anyhow::anyhow!("failed to parse transform '{}': {:?}", normalized, errs))?
        .conv(&normalized);

    let mut ctx = ParseCtx::new(Vec::new());
    ctx.insert_natives(jaq_core::core());
    ctx.insert_defs(jaq_std::std());
    let filter = ctx.compile(main);
    if !ctx.errs.is_empty() {
        let errors = ctx
            .errs
            .into_iter()
            .map(|err| err.0.to_string())
            .collect::<Vec<_>>()
            .join("; ");
        anyhow::bail!("failed to compile transform '{}': {}", normalized, errors);
    }

    Ok(CompiledTransform {
        expression: normalized,
        filter,
    })
}

fn run_transform(payload: Value, transform: &CompiledTransform) -> Result<Vec<Value>> {
    let input = Val::from(payload);
    let inputs = RcIter::new(core::iter::empty());
    let mut output = transform.filter.run((Ctx::new([], &inputs), input));
    let mut rows = Vec::new();
    while let Some(next) = output.next() {
        let value = next.map_err(|err| {
            anyhow::anyhow!(
                "failed to execute transform '{}': {}",
                transform.expression,
                err
            )
        })?;
        rows.push(Value::from(value));
    }
    if rows.is_empty() {
        anyhow::bail!("transform '{}' produced empty result", transform.expression);
    }
    Ok(rows)
}

fn parse_remote_prefix(value: &str) -> Result<IpNet> {
    IpNet::from_str(value.trim())
        .with_context(|| format!("invalid remote network prefix '{}'", value))
}

fn decode_remote_asn(value: AsnValue) -> u32 {
    match value {
        AsnValue::Empty => 0,
        AsnValue::Number(v) => v,
        AsnValue::Text(text) => text
            .strip_prefix("AS")
            .or_else(|| text.strip_prefix("as"))
            .unwrap_or(&text)
            .trim()
            .parse::<u32>()
            .unwrap_or(0),
    }
}

fn build_client(use_proxy: bool, tls: &RemoteNetworkSourceTlsConfig) -> Result<Client> {
    let mut builder = if use_proxy {
        Client::builder()
    } else {
        Client::builder().no_proxy()
    };

    if tls.enable {
        let skip_verify = if tls.skip_verify { true } else { !tls.verify };
        if skip_verify {
            // Matches Akvorado's SkipVerify behavior when explicitly enabled.
            builder = builder.danger_accept_invalid_certs(true);
        }

        if !tls.ca_file.trim().is_empty() {
            let data = fs::read(tls.ca_file.trim()).with_context(|| {
                format!(
                    "failed to read network source CA file '{}'",
                    tls.ca_file.trim()
                )
            })?;
            let cert = Certificate::from_pem(&data).with_context(|| {
                format!(
                    "failed to parse network source CA file '{}'",
                    tls.ca_file.trim()
                )
            })?;
            builder = builder.add_root_certificate(cert);
        }

        if !tls.cert_file.trim().is_empty() {
            let cert_path = tls.cert_file.trim();
            let key_path = if tls.key_file.trim().is_empty() {
                cert_path
            } else {
                tls.key_file.trim()
            };

            let cert_pem = fs::read(cert_path).with_context(|| {
                format!(
                    "failed to read network source client certificate '{}'",
                    cert_path
                )
            })?;
            let mut identity_pem = cert_pem;
            if key_path != cert_path {
                let key_pem = fs::read(key_path).with_context(|| {
                    format!("failed to read network source client key '{}'", key_path)
                })?;
                identity_pem.extend_from_slice(b"\n");
                identity_pem.extend_from_slice(&key_pem);
            }

            let identity = Identity::from_pem(&identity_pem).with_context(|| {
                format!(
                    "failed to parse network source client identity from cert '{}' and key '{}'",
                    cert_path, key_path
                )
            })?;
            builder = builder.identity(identity);
        }
    }

    builder
        .build()
        .context("failed to initialize network source HTTP client")
}

#[cfg(test)]
mod tests {
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
        let transform =
            compile_transform(r#".results[] | select(false)"#).expect("compile transform");
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
}
