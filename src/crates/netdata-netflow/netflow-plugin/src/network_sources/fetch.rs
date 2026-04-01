use super::decode::decode_remote_records;
use super::types::CompiledTransform;
use super::*;

pub(super) async fn fetch_source_once(
    client: &Client,
    source: &RemoteNetworkSourceConfig,
    transform: &CompiledTransform,
) -> Result<Vec<NetworkSourceRecord>> {
    let method = parse_source_method(&source.method)?;
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

pub(super) fn parse_source_method(method: &str) -> Result<Method> {
    let method = Method::from_str(&method.trim().to_ascii_uppercase())
        .with_context(|| format!("invalid HTTP method name '{}'", method))?;

    if method != Method::GET && method != Method::POST {
        anyhow::bail!("unsupported method '{}' (expected GET or POST)", method);
    }

    Ok(method)
}

pub(super) fn build_client(use_proxy: bool, tls: &RemoteNetworkSourceTlsConfig) -> Result<Client> {
    let mut builder = if use_proxy {
        Client::builder()
    } else {
        Client::builder().no_proxy()
    };

    if tls.enable {
        let skip_verify = !tls.verify && tls.skip_verify;
        if skip_verify {
            // Only disable peer verification when the config explicitly opts out.
            tracing::warn!(
                "network-sources TLS certificate verification is disabled for a remote source; this is insecure"
            );
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
