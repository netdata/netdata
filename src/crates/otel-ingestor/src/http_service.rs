//! The OTLP/HTTP receiver: a second listener (config `endpoint.http_path`,
//! stock `127.0.0.1:4318`) that serves `POST /v1/{logs,traces,metrics}` with
//! collector-parity semantics, so SDKs and exporters defaulting to
//! `http/protobuf` or `http/json` reach the same ingestion cores as gRPC.
//!
//! Contract (mirrors open-telemetry/opentelemetry-collector
//! `receiver/otlpreceiver/otlphttp.go`):
//!
//! - method mismatch → `405 text/plain` ("405 method not allowed, supported:
//!   [POST]"); unmatched path → `404 text/plain`;
//! - `Content-Type` is MIME-parsed (parameters stripped, case-insensitive):
//!   exactly `application/x-protobuf` and `application/json` are accepted,
//!   anything else → `415 text/plain`;
//! - `Content-Encoding`: `gzip` (flate2), `identity`/absent → raw, anything
//!   else → `415 text/plain`;
//! - bodies are capped (see [`MAX_BODY_BYTES`], applied to the wire body AND
//!   the gzip-expanded body — the second cap is the decompression-bomb guard)
//!   → over-limit `413 text/plain`;
//! - a body that fails to decompress or decode → `400` with a
//!   `google.rpc.Status` body in the request's encoding;
//! - tenant policy (`X-Scope-OrgID`) is identical to gRPC via
//!   [`crate::tenant::extract_tenant_id_from_headers`], with the `tonic`
//!   `Status` mapped onto HTTP codes;
//! - export errors are returned as `google.rpc.Status` in the request's
//!   encoding (locally defined: we never attach `details`, so the wire format
//!   is byte-identical to upstream's two populated fields);
//! - success replies `200` with the response encoded in the SAME encoding and
//!   a mirrored `Content-Type`.
//!
//! The listener binds during startup with the same strict fail-fast rule as
//! the gRPC endpoint (user decision D4): a bind or TLS failure aborts the
//! worker before `Ready`, rather than advertising an endpoint that cannot
//! receive. TLS terminates via `tokio-rustls`, configured independently of
//! the gRPC endpoint's tonic TLS.

use std::io::Read;
use std::net::SocketAddr;
use std::sync::Arc;

use anyhow::{Context, Result, bail};
use axum::Router;
use axum::body::Body;
use axum::extract::State;
use axum::http::{HeaderMap, Response, StatusCode};
use axum::routing::post;
use bridge::config::{AuthConfig, EndpointConfig};
use file_registry::TenantId;
use opentelemetry_proto::tonic::collector::logs::v1::ExportLogsServiceRequest;
use opentelemetry_proto::tonic::collector::metrics::v1::ExportMetricsServiceRequest;
use opentelemetry_proto::tonic::collector::trace::v1::ExportTraceServiceRequest;
use prost::Message as _;
use serde::Serialize;
use serde::de::DeserializeOwned;
use tonic::{Code, Status};

use crate::logs_service::NetdataLogsService;
use crate::metrics_service::NetdataMetricsService;
use crate::tenant::extract_tenant_id_from_headers;
use crate::trace_service::NetdataTracesService;

/// The maximum accepted request body, on the wire AND after gzip expansion.
///
/// This mirrors tonic's default `max_decoding_message_size` (4 MiB), so the
/// two transports reject the same requests; capping the expanded body too is
/// the decompression-bomb guard (a 4 MiB gzip stream can otherwise expand
/// unboundedly before prost ever sees a length).
const MAX_BODY_BYTES: usize = 4 * 1024 * 1024;

/// The three signals' service handles plus the shared tenant policy. Cheap to
/// clone (all fields are `Arc`-backed or `Copy`).
#[derive(Clone)]
pub(crate) struct HttpState {
    pub(crate) logs: Arc<NetdataLogsService>,
    pub(crate) traces: Arc<NetdataTracesService>,
    pub(crate) metrics: Arc<NetdataMetricsService>,
    pub(crate) auth: AuthConfig,
}

/// One of the two wire codecs an OTLP/HTTP request/response can use, chosen
/// from the request's `Content-Type` MIME type.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
enum Codec {
    Protobuf,
    Json,
}

impl Codec {
    /// The canonical `Content-Type` this codec stamps on responses — the
    /// success path mirrors the request's encoding, per the OTLP/HTTP spec.
    fn content_type(self) -> &'static str {
        match self {
            Codec::Protobuf => "application/x-protobuf",
            Codec::Json => "application/json",
        }
    }

    /// Bytes for a successful export response, in this codec. JSON output is
    /// normalized to omit null-valued fields: the `with-serde` derives emit
    /// `"partialSuccess": null` where proto3 JSON (and the collector) omit
    /// unset fields entirely, and senders' protojson parsers are entitled to
    /// the canonical shape.
    fn encode_response<R: prost::Message + Serialize>(self, resp: &R) -> Result<Vec<u8>> {
        match self {
            Codec::Protobuf => Ok(resp.encode_to_vec()),
            Codec::Json => {
                let value =
                    serde_json::to_value(resp).context("serializing OTLP/HTTP response as JSON")?;
                serde_json::to_vec(&without_nulls(value))
                    .context("serializing OTLP/HTTP response as JSON")
            }
        }
    }
}

/// Recursively drop object entries whose value is JSON `null` (proto3 JSON's
/// spelling of an unset field). Arrays are mapped; scalars pass through.
fn without_nulls(value: serde_json::Value) -> serde_json::Value {
    match value {
        serde_json::Value::Object(map) => serde_json::Value::Object(
            map.into_iter()
                .filter(|(_, v)| !v.is_null())
                .map(|(k, v)| (k, without_nulls(v)))
                .collect(),
        ),
        serde_json::Value::Array(items) => {
            serde_json::Value::Array(items.into_iter().map(without_nulls).collect())
        }
        other => other,
    }
}

/// A `google.rpc.Status` (rpc/status.proto), locally defined because no
/// dependency in this workspace ships it. Only `code` and `message` are ever
/// populated (the collector's OTLP/HTTP errors carry no `details` either), so
/// the wire format is identical to upstream's; adding `details` later under
/// field 3 stays wire-compatible.
#[derive(Clone, PartialEq, ::prost::Message)]
struct RpcStatus {
    /// The `google.rpc.Code` value (carried as the HTTP status analog).
    #[prost(int32, tag = "1")]
    code: i32,
    #[prost(string, tag = "2")]
    message: String,
}

impl RpcStatus {
    fn new(code: Code, message: impl Into<String>) -> Self {
        Self {
            code: code as i32,
            message: message.into(),
        }
    }

    /// Body bytes in the request's codec. The JSON form follows proto3 JSON
    /// for `google.rpc.Status` (`code` as a number, `message` as a string;
    /// absent `details` is omitted).
    fn encode(self, codec: Codec) -> Vec<u8> {
        match codec {
            Codec::Protobuf => self.encode_to_vec(),
            Codec::Json => {
                // `json!` + `to_vec` cannot fail on two plain scalars.
                serde_json::to_vec(&serde_json::json!({
                    "code": self.code,
                    "message": self.message,
                }))
                .expect("serializing a two-scalar JSON object cannot fail")
            }
        }
    }
}

/// Build the OTLP/HTTP router. Exposed separately from [`serve`] so the
/// handler tests can drive it with `tower::ServiceExt::oneshot` without a
/// listener.
pub(crate) fn router(state: HttpState) -> Router {
    // Method mismatch on a known path answers in the collector's own words;
    // axum's built-in 405 would be an empty body.
    Router::new()
        .route("/v1/logs", post(export_logs).fallback(method_not_allowed))
        .route(
            "/v1/traces",
            post(export_traces).fallback(method_not_allowed),
        )
        .route(
            "/v1/metrics",
            post(export_metrics).fallback(method_not_allowed),
        )
        // Unmatched paths mirror net/http's plain 404 (the collector is
        // served by a stdlib mux, so this is the parity body).
        .fallback(not_found)
        .with_state(state)
}

/// The OTLP/HTTP listener: a bound TCP listener, wrapped for TLS when the
/// `http_tls_*` trio is configured.
pub(crate) enum HttpListener {
    Plain(tokio::net::TcpListener),
    Tls(TlsListener),
}

/// Bind the OTLP/HTTP listener (and build its TLS acceptor when configured).
///
/// Returns `Ok(None)` when the receiver is disabled (`http_path: null`).
/// Every failure is fatal per D4 — strict fail-fast, symmetric with the gRPC
/// bind — which is why this runs before the worker touches its WAL dirs and
/// before `Ready` is sent.
pub(crate) async fn bind_http(endpoint: &EndpointConfig) -> Result<Option<HttpListener>> {
    let Some(path) = endpoint.http_path.as_ref() else {
        tracing::info!(
            "OTLP/HTTP receiver disabled (endpoint.http_path is null); serving gRPC only"
        );
        return Ok(None);
    };

    let addr: SocketAddr = path
        .parse()
        .with_context(|| format!("failed to parse OTLP/HTTP endpoint address: {path}"))?;
    let listener = tokio::net::TcpListener::bind(addr)
        .await
        .with_context(|| format!("failed to bind OTLP/HTTP endpoint {path}"))?;
    tracing::info!(endpoint = %path, "OTLP/HTTP endpoint bound");

    match (
        endpoint.http_tls_cert_path.as_ref(),
        endpoint.http_tls_key_path.as_ref(),
    ) {
        (Some(cert_path), Some(key_path)) => {
            let acceptor =
                build_tls_acceptor(cert_path, key_path, endpoint.http_tls_ca_cert_path.as_ref())
                    .context("failed to configure OTLP/HTTP TLS")?;
            Ok(Some(HttpListener::Tls(TlsListener { listener, acceptor })))
        }
        (None, None) => {
            tracing::warn!("TLS disabled, using insecure connection on OTLP/HTTP endpoint: {path}");
            Ok(Some(HttpListener::Plain(listener)))
        }
        // `validate()` rejects this pairing at config time; reaching here
        // means the supervisor handed us an unvalidated config.
        _ => bail!(
            "OTLP/HTTP TLS requires both a certificate and a private key (http_tls_cert_path / http_tls_key_path)"
        ),
    }
}

/// Serve the router on the bound listener until the server future is dropped.
/// Errors are fatal and surface to the worker's main loop like the gRPC
/// server's own.
pub(crate) async fn serve(listener: HttpListener, app: Router) -> Result<()> {
    match listener {
        HttpListener::Plain(listener) => axum::serve(listener, app)
            .await
            .context("OTLP/HTTP server error"),
        HttpListener::Tls(listener) => axum::serve(listener, app)
            .await
            .context("OTLP/HTTP TLS server error"),
    }
}

/// A `tokio-rustls` acceptor wrapper implementing axum's `Listener` contract,
/// per the axum TLS recipe: `axum::serve` is generic over listeners whose
/// IO types are async read/write, so TLS termination sits in front of the
/// plain HTTP serving machinery instead of a second server stack.
pub(crate) struct TlsListener {
    listener: tokio::net::TcpListener,
    acceptor: tokio_rustls::TlsAcceptor,
}

impl axum::serve::Listener for TlsListener {
    type Io = tokio_rustls::server::TlsStream<tokio::net::TcpStream>;
    type Addr = std::net::SocketAddr;

    async fn accept(&mut self) -> (Self::Io, Self::Addr) {
        loop {
            match self.listener.accept().await {
                Ok((stream, peer)) => {
                    // Same default the gRPC path restores on its sockets.
                    if let Err(e) = stream.set_nodelay(true) {
                        tracing::warn!(%e, "failed to set TCP_NODELAY on OTLP/HTTP connection");
                    }
                    // Handshake failures (untrusted client, bad ALPN) kill one
                    // connection, never the listener: log and keep accepting.
                    match self.acceptor.accept(stream).await {
                        Ok(io) => return (io, peer),
                        Err(e) => {
                            tracing::debug!(%e, "OTLP/HTTP TLS handshake failed");
                        }
                    }
                }
                // Transient accept failures (EMFILE under fd pressure, a
                // reset connection) must not tear the receiver down either;
                // axum's own TcpListener impl applies the same policy.
                Err(e) => {
                    tracing::warn!(%e, "OTLP/HTTP accept failed");
                    tokio::time::sleep(std::time::Duration::from_millis(100)).await;
                }
            }
        }
    }

    fn local_addr(&self) -> std::io::Result<Self::Addr> {
        self.listener.local_addr()
    }
}

/// Build the TLS acceptor from the configured PEM files.
///
/// ALPN advertises `h2` then `http/1.1`, the same preference the collector's
/// confighttp server expresses; `axum::serve`'s auto connection builder then
/// serves either protocol on the accepted stream.
fn build_tls_acceptor(
    cert_path: &str,
    key_path: &str,
    ca_path: Option<&String>,
) -> Result<tokio_rustls::TlsAcceptor> {
    use tokio_rustls::rustls::pki_types::pem::PemObject;
    use tokio_rustls::rustls::server::WebPkiClientVerifier;
    use tokio_rustls::rustls::{RootCertStore, ServerConfig};

    let certs = tokio_rustls::rustls::pki_types::CertificateDer::pem_file_iter(cert_path)
        .with_context(|| format!("failed to read TLS certificates from: {cert_path}"))?
        .collect::<Result<Vec<_>, _>>()
        .with_context(|| format!("failed to parse TLS certificates from: {cert_path}"))?;
    let key = tokio_rustls::rustls::pki_types::PrivateKeyDer::pem_file_iter(key_path)
        .with_context(|| format!("failed to read TLS private key from: {key_path}"))?
        .next()
        .with_context(|| format!("no private key found in: {key_path}"))?
        .with_context(|| format!("failed to parse TLS private key from: {key_path}"))?;

    // With a client CA the listener requires client certificates (mTLS),
    // mirroring `tls_ca_cert_path` on the gRPC endpoint; without one it serves
    // plain server-TLS.
    let builder = ServerConfig::builder();
    let config = match ca_path {
        Some(ca_cert_path) => {
            let ca_certs =
                tokio_rustls::rustls::pki_types::CertificateDer::pem_file_iter(ca_cert_path)
                    .with_context(|| format!("failed to read CA certificate from: {ca_cert_path}"))?
                    .collect::<Result<Vec<_>, _>>()
                    .with_context(|| {
                        format!("failed to parse CA certificate from: {ca_cert_path}")
                    })?;
            let mut roots = RootCertStore::empty();
            let (added, ignored) = roots.add_parsable_certificates(ca_certs);
            if added == 0 {
                bail!("no usable trust anchors in CA certificate file: {ca_cert_path}");
            }
            if ignored > 0 {
                tracing::warn!(
                    ignored,
                    "skipped unparseable CA certificates from: {ca_cert_path}"
                );
            }
            let verifier = WebPkiClientVerifier::builder(Arc::new(roots))
                .build()
                .context("failed to build the OTLP/HTTP client-certificate verifier")?;
            builder
                .with_client_cert_verifier(verifier)
                .with_single_cert(certs, key)
        }
        None => builder.with_no_client_auth().with_single_cert(certs, key),
    }
    .context("failed to assemble the OTLP/HTTP TLS identity")?;

    let mut config = config;
    config.alpn_protocols = vec![b"h2".to_vec(), b"http/1.1".to_vec()];
    Ok(tokio_rustls::TlsAcceptor::from(Arc::new(config)))
}

// ---------------------------------------------------------------------------
// Handlers
// ---------------------------------------------------------------------------

async fn export_logs(
    State(state): State<HttpState>,
    headers: HeaderMap,
    body: Body,
) -> Response<Body> {
    // The closure needs the headers after `serve_export` is done reading
    // content negotiation off them, so it takes its own copy (cheap:
    // `HeaderMap` is Arc-backed).
    let hdrs = headers.clone();
    serve_export::<ExportLogsServiceRequest, _, _, _>(headers, body, move |req| async move {
        let tenant: TenantId = extract_tenant_id_from_headers(&hdrs, &state.auth)?;
        state.logs.export_logs(&tenant, req).await
    })
    .await
}

async fn export_traces(
    State(state): State<HttpState>,
    headers: HeaderMap,
    body: Body,
) -> Response<Body> {
    let hdrs = headers.clone();
    serve_export::<ExportTraceServiceRequest, _, _, _>(headers, body, move |req| async move {
        let tenant: TenantId = extract_tenant_id_from_headers(&hdrs, &state.auth)?;
        state.traces.export_traces(&tenant, req).await
    })
    .await
}

async fn export_metrics(
    State(state): State<HttpState>,
    headers: HeaderMap,
    body: Body,
) -> Response<Body> {
    serve_export::<ExportMetricsServiceRequest, _, _, _>(headers, body, move |req| async move {
        // Metrics carry no tenant (charts are agent-wide); the core's own
        // signature says so.
        state.metrics.export_metrics(req).await
    })
    .await
}

/// The transport pipeline shared by all three signals: content negotiation,
/// body limits, decompression, decode, then the signal's export.
///
/// `run` receives the decoded request and answers with the core's
/// `Result<Response, Status>` — the same cores the gRPC wrappers call — so
/// the two transports cannot drift apart.
async fn serve_export<Req, Resp, F, Fut>(headers: HeaderMap, body: Body, run: F) -> Response<Body>
where
    Req: prost::Message + DeserializeOwned + Default,
    Resp: prost::Message + Serialize,
    F: FnOnce(Req) -> Fut + Send,
    Fut: Future<Output = Result<Resp, Status>> + Send,
{
    // Content-Type first, exactly like the collector's `readContentType`.
    let Some(codec) = content_type_codec(headers.get(axum::http::header::CONTENT_TYPE)) else {
        return plain_text(
            StatusCode::UNSUPPORTED_MEDIA_TYPE,
            "415 unsupported media type, supported: [application/json, application/x-protobuf]",
        );
    };

    let Some(coding) = content_encoding(headers.get(axum::http::header::CONTENT_ENCODING)) else {
        return plain_text(
            StatusCode::UNSUPPORTED_MEDIA_TYPE,
            "415 unsupported content coding, supported: [gzip, identity]",
        );
    };

    // Wire-body cap. `to_bytes` fails either on the limit or on a transport
    // error mid-body; the limit sits in the error's source chain.
    let wire = match axum::body::to_bytes(body, MAX_BODY_BYTES).await {
        Ok(bytes) => bytes,
        Err(err) => {
            if std::error::Error::source(&err)
                .is_some_and(|s| s.is::<http_body_util::LengthLimitError>())
            {
                return plain_text(StatusCode::PAYLOAD_TOO_LARGE, "413 request body too large");
            }
            return rpc_error(
                codec,
                Status::invalid_argument(format!("failed to read request body: {err}")),
            );
        }
    };

    let body_bytes = match coding {
        BodyCoding::Identity => wire,
        BodyCoding::Gzip => {
            let mut out = Vec::new();
            // `take` caps the EXPANDED size (the wire cap alone does not bound
            // a gzip stream), reading one extra byte to notice the overflow.
            let mut reader =
                flate2::read::GzDecoder::new(wire.as_ref()).take((MAX_BODY_BYTES + 1) as u64);
            match reader.read_to_end(&mut out) {
                // Ok(n) with n <= cap: EOF ended the stream inside the limit.
                Ok(_) if out.len() <= MAX_BODY_BYTES => out.into(),
                // Ok with more than the cap: the `take` bound, not EOF, ended
                // the read — the expanded body is over the limit.
                Ok(_) => {
                    return plain_text(StatusCode::PAYLOAD_TOO_LARGE, "413 request body too large");
                }
                Err(e) => {
                    return rpc_error(
                        codec,
                        Status::invalid_argument(format!("failed to decompress gzip body: {e}")),
                    );
                }
            }
        }
    };

    let req = match codec {
        Codec::Protobuf => match Req::decode(body_bytes.as_ref()) {
            Ok(req) => req,
            Err(e) => {
                return rpc_error(
                    codec,
                    Status::invalid_argument(format!("failed to decode protobuf body: {e}")),
                );
            }
        },
        Codec::Json => match serde_json::from_slice(&body_bytes) {
            Ok(req) => req,
            Err(e) => {
                return rpc_error(
                    codec,
                    Status::invalid_argument(format!("failed to decode JSON body: {e}")),
                );
            }
        },
    };

    match run(req).await {
        Ok(resp) => {
            // A response-encoding failure is a server bug, not a sender
            // error: 500 with the collector's fallback body shape.
            match codec.encode_response(&resp) {
                Ok(bytes) => response_bytes(StatusCode::OK, codec.content_type(), bytes),
                Err(e) => rpc_error(
                    codec,
                    Status::internal(format!("failed to marshal response: {e:#}")),
                ),
            }
        }
        Err(status) => rpc_error(codec, status),
    }
}

/// The request's MIME type → codec. Parameters are stripped and the type is
/// matched case-insensitively (`application/json; charset=utf-8` is JSON),
/// the same normalization `mime.ParseMediaType` applies upstream. A missing
/// header is unsupported, matching the collector.
fn content_type_codec(value: Option<&axum::http::HeaderValue>) -> Option<Codec> {
    let value = value?;
    let mime = value
        .to_str()
        .ok()?
        .split(';')
        .next()
        .unwrap_or("")
        .trim()
        .to_ascii_lowercase();
    match mime.as_str() {
        "application/x-protobuf" => Some(Codec::Protobuf),
        "application/json" => Some(Codec::Json),
        _ => None,
    }
}

enum BodyCoding {
    Identity,
    Gzip,
}

/// The request's `Content-Encoding` (case-insensitive). Absent means
/// identity (the collector's decompression middleware treats a missing
/// header the same as `identity`); anything but `gzip`/`identity` is
/// unsupported.
fn content_encoding(value: Option<&axum::http::HeaderValue>) -> Option<BodyCoding> {
    let Some(value) = value else {
        return Some(BodyCoding::Identity);
    };
    let value = value.to_str().ok()?.trim().to_ascii_lowercase();
    match value.as_str() {
        "" | "identity" => Some(BodyCoding::Identity),
        "gzip" => Some(BodyCoding::Gzip),
        _ => None,
    }
}

/// Map a transport-agnostic `tonic::Status` onto the HTTP response the OTLP
/// spec expects: the mapped status code plus a `google.rpc.Status` body in
/// the request's own encoding. The mapping matches the collector's
/// gRPC→HTTP table (`errors.GetHTTPStatusCodeFromStatus`).
fn rpc_error(codec: Codec, status: Status) -> Response<Body> {
    let code = status.code();
    let http = http_status_for(code);
    let body = RpcStatus::new(code, status.message().to_string()).encode(codec);
    response_bytes(http, codec.content_type(), body)
}

fn http_status_for(code: Code) -> StatusCode {
    match code {
        Code::InvalidArgument | Code::OutOfRange | Code::FailedPrecondition => {
            StatusCode::BAD_REQUEST
        }
        Code::Unauthenticated => StatusCode::UNAUTHORIZED,
        Code::PermissionDenied => StatusCode::FORBIDDEN,
        Code::NotFound => StatusCode::NOT_FOUND,
        Code::ResourceExhausted => StatusCode::TOO_MANY_REQUESTS,
        Code::Unimplemented => StatusCode::NOT_IMPLEMENTED,
        Code::Unavailable | Code::DeadlineExceeded => StatusCode::SERVICE_UNAVAILABLE,
        _ => StatusCode::INTERNAL_SERVER_ERROR,
    }
}

/// A `text/plain` response with a body that names its own status code, the
/// shape the collector uses for transport-level rejections.
fn plain_text(status: StatusCode, body: &str) -> Response<Body> {
    response_bytes(status, "text/plain", body.as_bytes().to_vec())
}

fn response_bytes(status: StatusCode, content_type: &'static str, body: Vec<u8>) -> Response<Body> {
    Response::builder()
        .status(status)
        .header(axum::http::header::CONTENT_TYPE, content_type)
        .body(Body::from(body))
        .expect("statically valid response parts")
}

async fn method_not_allowed() -> Response<Body> {
    plain_text(
        StatusCode::METHOD_NOT_ALLOWED,
        "405 method not allowed, supported: [POST]",
    )
}

async fn not_found() -> Response<Body> {
    plain_text(StatusCode::NOT_FOUND, "404 page not found")
}

#[cfg(test)]
mod tests {
    use std::collections::HashMap;
    use std::io::Write as _;
    use std::path::Path;
    use std::sync::{Arc, Mutex};

    use axum::body::Body;
    use axum::http::{Request, StatusCode, header};
    use bridge::config::{AuthConfig, IngestConfig, RotationEntry, WalConfig};
    use file_registry::{Identity, InstanceId, MachineId, MonotonicClock};
    use opentelemetry_proto::tonic::collector::logs::v1::ExportLogsServiceRequest;
    use opentelemetry_proto::tonic::collector::metrics::v1::ExportMetricsServiceRequest;
    use opentelemetry_proto::tonic::collector::trace::v1::ExportTraceServiceRequest;
    use opentelemetry_proto::tonic::common::v1::KeyValue;
    use opentelemetry_proto::tonic::common::v1::any_value::Value;
    use opentelemetry_proto::tonic::logs::v1::{LogRecord, ResourceLogs, ScopeLogs};
    use opentelemetry_proto::tonic::resource::v1::Resource;
    use opentelemetry_proto::tonic::trace::v1::{ResourceSpans, ScopeSpans, Span};
    use tower::ServiceExt;

    use super::*;
    use crate::ledger_sender::LedgerSender;
    use crate::logs_service::NetdataLogsService;
    use crate::trace_service::NetdataTracesService;

    /// One request-scoped log record, attributed to `service.name` so the
    /// stream identity is realistic. Timestamps use a fixed instant inside
    /// any sane ingest window (the test window is effectively unbounded).
    fn logs_req(n_records: usize) -> ExportLogsServiceRequest {
        let records = (0..n_records)
            .map(|i| LogRecord {
                time_unix_nano: 1_700_000_000_000_000_000 + i as u64,
                observed_time_unix_nano: 1_700_000_000_000_000_000 + i as u64,
                body: Some(opentelemetry_proto::tonic::common::v1::AnyValue {
                    value: Some(Value::StringValue(format!("http-test-{i}"))),
                }),
                ..Default::default()
            })
            .collect();
        ExportLogsServiceRequest {
            resource_logs: vec![ResourceLogs {
                resource: Some(Resource {
                    attributes: vec![KeyValue {
                        key: "service.name".into(),
                        value: Some(opentelemetry_proto::tonic::common::v1::AnyValue {
                            value: Some(Value::StringValue("http-tests".into())),
                        }),
                    }],
                    ..Default::default()
                }),
                scope_logs: vec![ScopeLogs {
                    log_records: records,
                    ..Default::default()
                }],
                ..Default::default()
            }],
        }
    }

    fn traces_req() -> ExportTraceServiceRequest {
        ExportTraceServiceRequest {
            resource_spans: vec![ResourceSpans {
                scope_spans: vec![ScopeSpans {
                    spans: vec![Span {
                        trace_id: vec![0x22; 16],
                        span_id: vec![1, 2, 3, 4, 5, 6, 7, 8],
                        name: "http-test-span".into(),
                        start_time_unix_nano: 1_700_000_000_000_000_000,
                        end_time_unix_nano: 1_700_000_000_000_001_000,
                        ..Default::default()
                    }],
                    ..Default::default()
                }],
                ..Default::default()
            }],
        }
    }

    fn metrics_req() -> ExportMetricsServiceRequest {
        // An empty request is a valid export for the metrics core (it always
        // answers Ok); these tests exercise the transport, not chart logic.
        ExportMetricsServiceRequest::default()
    }

    /// Test WAL config in the shape the service tests use elsewhere.
    fn wal_config(dir: &Path) -> WalConfig {
        let mut rotation = HashMap::new();
        rotation.insert(
            "default".to_string(),
            RotationEntry {
                max_file_size: Some(bytesize::ByteSize::mb(64)),
                max_entries: Some(100_000),
                max_file_duration: Some(std::time::Duration::from_secs(3600)),
            },
        );
        WalConfig {
            dir: dir.to_path_buf(),
            crc_enabled: true,
            compression_enabled: true,
            rotation: bridge::config::RotationPolicy::try_from(rotation)
                .expect("test rotation has a complete default"),
        }
    }

    /// Effectively-unbounded ingest window, like the sibling service tests.
    fn unbounded_ingest() -> IngestConfig {
        IngestConfig {
            max_age: std::time::Duration::from_secs(u64::MAX),
            future_skew: std::time::Duration::from_secs(u64::MAX),
        }
    }

    /// The full three-service state over a tempdir-backed WAL, with the
    /// `LedgerSender` pointed at a socket that accepts no connection
    /// (`send_events` is fire-and-forget, so this never blocks a test).
    fn test_state(auth: AuthConfig) -> (HttpState, tempfile::TempDir) {
        test_state_with_ingest(auth, unbounded_ingest())
    }

    fn test_state_with_ingest(
        auth: AuthConfig,
        ingest: IngestConfig,
    ) -> (HttpState, tempfile::TempDir) {
        let tmp = tempfile::tempdir().unwrap();
        let socket = format!("/tmp/netdata-http-service-test-{}.sock", std::process::id());
        let sender = Arc::new(LedgerSender::new(&socket));
        let seq = Arc::new(wal::SeqAllocator::ephemeral(0));
        let clock = Arc::new(Mutex::new(MonotonicClock::new()));
        let identity = Identity::new(
            MachineId::new(uuid::Uuid::from_u128(1)).unwrap(),
            InstanceId::new(uuid::Uuid::from_u128(2)).unwrap(),
        );

        let logs = Arc::new(NetdataLogsService::new(
            Arc::clone(&sender),
            tmp.path().join("logs/wal"),
            wal_config(&tmp.path().join("logs/wal")),
            ingest.clone(),
            Arc::clone(&seq),
            Arc::clone(&clock),
            auth.clone(),
            identity,
        ));

        let traces = Arc::new(NetdataTracesService::new(
            sender,
            tmp.path().join("traces/wal"),
            wal_config(&tmp.path().join("traces/wal")),
            ingest,
            seq,
            clock,
            auth.clone(),
            identity,
        ));

        (
            HttpState {
                logs,
                traces,
                metrics: Arc::new(NetdataMetricsService::default()),
                // The router's tenant policy must be the SAME auth config the
                // services carry — the test builds both from one value.
                auth,
            },
            tmp,
        )
    }

    /// POST a request at the router through `oneshot`, with headers.
    async fn post(
        state: HttpState,
        uri: &str,
        content_type: Option<&str>,
        content_encoding: Option<&str>,
        tenant: Option<&str>,
        body: Vec<u8>,
    ) -> axum::response::Response {
        let mut builder = Request::builder().method("POST").uri(uri);
        if let Some(ct) = content_type {
            builder = builder.header(header::CONTENT_TYPE, ct);
        }
        if let Some(ce) = content_encoding {
            builder = builder.header(header::CONTENT_ENCODING, ce);
        }
        if let Some(t) = tenant {
            builder = builder.header(AuthConfig::TENANT_HEADER, t);
        }
        let request = builder.body(Body::from(body)).unwrap();
        router(state).oneshot(request).await.unwrap()
    }

    /// The response body collected to bytes.
    async fn body_bytes(response: axum::response::Response) -> Vec<u8> {
        axum::body::to_bytes(response.into_body(), usize::MAX)
            .await
            .unwrap()
            .to_vec()
    }

    /// True when any regular file exists under `dir` (recursively) — the
    /// cheap "the WAL got a frame" assertion for e2e/happy-path tests.
    fn any_file_under(dir: &Path) -> bool {
        fn walk(dir: &Path) -> bool {
            let Ok(entries) = std::fs::read_dir(dir) else {
                return false;
            };
            for entry in entries.flatten() {
                let Ok(ft) = entry.file_type() else { continue };
                if ft.is_file() {
                    return true;
                }
                if ft.is_dir() && walk(&entry.path()) {
                    return true;
                }
            }
            false
        }
        walk(dir)
    }

    fn gzip(data: &[u8]) -> Vec<u8> {
        let mut enc = flate2::write::GzEncoder::new(Vec::new(), flate2::Compression::default());
        enc.write_all(data).unwrap();
        enc.finish().unwrap()
    }

    #[tokio::test]
    async fn logs_export_protobuf_round_trip() {
        let (state, tmp) = test_state(AuthConfig::default());
        let body = logs_req(2).encode_to_vec();
        let resp = post(
            state,
            "/v1/logs",
            Some("application/x-protobuf"),
            None,
            None,
            body,
        )
        .await;
        assert_eq!(resp.status(), StatusCode::OK);
        assert_eq!(
            resp.headers().get(header::CONTENT_TYPE).unwrap(),
            "application/x-protobuf"
        );
        let bytes = body_bytes(resp).await;
        let decoded =
            opentelemetry_proto::tonic::collector::logs::v1::ExportLogsServiceResponse::decode(
                bytes.as_ref(),
            )
            .unwrap();
        assert!(decoded.partial_success.is_none());
        // The ack follows the WAL sync, so by the time 200 returns the frame
        // is on disk.
        assert!(any_file_under(&tmp.path().join("logs/wal")));
    }

    #[tokio::test]
    async fn logs_export_json_round_trip() {
        let (state, _tmp) = test_state(AuthConfig::default());
        let body = serde_json::to_vec(&logs_req(1)).unwrap();
        let resp = post(
            state,
            "/v1/logs",
            Some("application/json"),
            None,
            None,
            body,
        )
        .await;
        assert_eq!(resp.status(), StatusCode::OK);
        assert_eq!(
            resp.headers().get(header::CONTENT_TYPE).unwrap(),
            "application/json"
        );
        let bytes = body_bytes(resp).await;
        // Full success serializes with no fields set (proto3 JSON omits
        // defaults), and `partialSuccess` must be absent, not null.
        let v: serde_json::Value = serde_json::from_slice(&bytes).unwrap();
        assert!(v.get("partialSuccess").is_none());
    }

    #[tokio::test]
    async fn traces_export_both_codecs() {
        for (ct, json) in [
            ("application/x-protobuf", false),
            ("application/json", true),
        ] {
            let (state, tmp) = test_state(AuthConfig::default());
            let req = traces_req();
            let body = if json {
                serde_json::to_vec(&req).unwrap()
            } else {
                req.encode_to_vec()
            };
            let resp = post(state, "/v1/traces", Some(ct), None, None, body).await;
            assert_eq!(resp.status(), StatusCode::OK, "content-type {ct}");
            assert_eq!(resp.headers().get(header::CONTENT_TYPE).unwrap(), ct);
            let bytes = body_bytes(resp).await;
            if json {
                assert_eq!(bytes, b"{}");
            } else {
                assert!(bytes.is_empty());
            }
            assert!(any_file_under(&tmp.path().join("traces/wal")));
        }
    }

    #[tokio::test]
    async fn metrics_export_both_codecs() {
        for ct in ["application/x-protobuf", "application/json"] {
            let (state, _tmp) = test_state(AuthConfig::default());
            let req = metrics_req();
            let body = if ct == "application/json" {
                serde_json::to_vec(&req).unwrap()
            } else {
                req.encode_to_vec()
            };
            let resp = post(state, "/v1/metrics", Some(ct), None, None, body).await;
            assert_eq!(resp.status(), StatusCode::OK, "content-type {ct}");
            assert_eq!(resp.headers().get(header::CONTENT_TYPE).unwrap(), ct);
            let bytes = body_bytes(resp).await;
            if ct == "application/json" {
                assert_eq!(bytes, b"{}");
            } else {
                assert!(bytes.is_empty());
            }
        }
    }

    #[tokio::test]
    async fn out_of_window_logs_report_partial_success() {
        // max_age = 0 rejects every record at arrival; the transport must
        // still answer 200 carrying OTLP partial_success (rejections are
        // data-level, not transport-level).
        let (state, _tmp) = test_state_with_ingest(
            AuthConfig::default(),
            IngestConfig {
                max_age: std::time::Duration::ZERO,
                future_skew: std::time::Duration::from_secs(u64::MAX),
            },
        );
        let body = serde_json::to_vec(&logs_req(2)).unwrap();
        let resp = post(
            state,
            "/v1/logs",
            Some("application/json"),
            None,
            None,
            body,
        )
        .await;
        assert_eq!(resp.status(), StatusCode::OK);
        let bytes = body_bytes(resp).await;
        let v: serde_json::Value = serde_json::from_slice(&bytes).unwrap();
        assert_eq!(
            v["partialSuccess"]["rejectedLogRecords"],
            serde_json::json!(2)
        );
    }

    #[tokio::test]
    async fn tenant_header_policy_matches_grpc() {
        // (name, auth enabled, tenant header, expected status)
        let cases: &[(&str, bool, Option<&str>, StatusCode)] = &[
            (
                "auth on, header missing",
                true,
                None,
                StatusCode::UNAUTHORIZED,
            ),
            (
                "auth on, invalid id",
                true,
                Some("bad/id"),
                StatusCode::BAD_REQUEST,
            ),
            ("auth on, valid id", true, Some("tenant-a"), StatusCode::OK),
            ("auth off, header missing", false, None, StatusCode::OK),
        ];
        for (name, enabled, tenant, expected) in cases {
            let auth = AuthConfig { enabled: *enabled };
            let (state, _tmp) = test_state(auth);
            let resp = post(
                state,
                "/v1/logs",
                Some("application/json"),
                None,
                *tenant,
                serde_json::to_vec(&logs_req(1)).unwrap(),
            )
            .await;
            assert_eq!(resp.status(), *expected, "{name}");
            if *expected != StatusCode::OK {
                // Error bodies are google.rpc.Status in the request's codec:
                // code 16 (UNAUTHENTICATED) or 3 (INVALID_ARGUMENT).
                let bytes = body_bytes(resp).await;
                let v: serde_json::Value = serde_json::from_slice(&bytes).unwrap();
                let code = match *expected {
                    StatusCode::UNAUTHORIZED => 16,
                    _ => 3,
                };
                assert_eq!(v["code"], serde_json::json!(code), "{name}");
                assert!(v["message"].is_string(), "{name}");
            }
        }
    }

    #[tokio::test]
    async fn method_mismatch_is_text_405() {
        let (state, _tmp) = test_state(AuthConfig::default());
        let request = Request::builder()
            .method("GET")
            .uri("/v1/logs")
            .body(Body::empty())
            .unwrap();
        let resp = router(state).oneshot(request).await.unwrap();
        assert_eq!(resp.status(), StatusCode::METHOD_NOT_ALLOWED);
        assert_eq!(
            resp.headers().get(header::CONTENT_TYPE).unwrap(),
            "text/plain"
        );
        let bytes = body_bytes(resp).await;
        assert_eq!(
            String::from_utf8(bytes).unwrap(),
            "405 method not allowed, supported: [POST]"
        );
    }

    #[tokio::test]
    async fn unknown_path_is_text_404() {
        let (state, _tmp) = test_state(AuthConfig::default());
        let request = Request::builder()
            .method("POST")
            .uri("/v1/profiles")
            .header(header::CONTENT_TYPE, "application/json")
            .body(Body::empty())
            .unwrap();
        let resp = router(state).oneshot(request).await.unwrap();
        assert_eq!(resp.status(), StatusCode::NOT_FOUND);
        assert_eq!(body_bytes(resp).await, b"404 page not found");
    }

    #[tokio::test]
    async fn unsupported_content_type_is_text_415() {
        for ct in [Some("text/plain"), None] {
            let (state, _tmp) = test_state(AuthConfig::default());
            let resp = post(state, "/v1/logs", ct, None, None, b"{}".to_vec()).await;
            assert_eq!(
                resp.status(),
                StatusCode::UNSUPPORTED_MEDIA_TYPE,
                "ct {ct:?}"
            );
            assert_eq!(
                resp.headers().get(header::CONTENT_TYPE).unwrap(),
                "text/plain"
            );
            assert_eq!(
                String::from_utf8(body_bytes(resp).await).unwrap(),
                "415 unsupported media type, supported: [application/json, application/x-protobuf]"
            );
        }
    }

    #[tokio::test]
    async fn content_type_parameters_and_case_are_ignored() {
        let (state, _tmp) = test_state(AuthConfig::default());
        let resp = post(
            state,
            "/v1/logs",
            Some("application/JSON; charset=utf-8"),
            None,
            None,
            serde_json::to_vec(&logs_req(1)).unwrap(),
        )
        .await;
        assert_eq!(resp.status(), StatusCode::OK);
        // The mirrored type is the canonical MIME, not the sender's spelling.
        assert_eq!(
            resp.headers().get(header::CONTENT_TYPE).unwrap(),
            "application/json"
        );
    }

    #[tokio::test]
    async fn unsupported_content_encoding_is_text_415() {
        let (state, _tmp) = test_state(AuthConfig::default());
        let resp = post(
            state,
            "/v1/logs",
            Some("application/json"),
            Some("br"),
            None,
            b"{}".to_vec(),
        )
        .await;
        assert_eq!(resp.status(), StatusCode::UNSUPPORTED_MEDIA_TYPE);
        assert_eq!(
            String::from_utf8(body_bytes(resp).await).unwrap(),
            "415 unsupported content coding, supported: [gzip, identity]"
        );
    }

    #[tokio::test]
    async fn identity_encoding_name_is_accepted() {
        let (state, _tmp) = test_state(AuthConfig::default());
        let resp = post(
            state,
            "/v1/logs",
            Some("application/json"),
            Some("identity"),
            None,
            serde_json::to_vec(&logs_req(1)).unwrap(),
        )
        .await;
        assert_eq!(resp.status(), StatusCode::OK);
    }

    #[tokio::test]
    async fn bad_protobuf_body_is_400_rpc_status() {
        let (state, _tmp) = test_state(AuthConfig::default());
        let resp = post(
            state,
            "/v1/logs",
            Some("application/x-protobuf"),
            None,
            None,
            vec![0xff, 0xff, 0xff, 0xff],
        )
        .await;
        assert_eq!(resp.status(), StatusCode::BAD_REQUEST);
        assert_eq!(
            resp.headers().get(header::CONTENT_TYPE).unwrap(),
            "application/x-protobuf"
        );
        let decoded = RpcStatus::decode(body_bytes(resp).await.as_ref()).unwrap();
        assert_eq!(decoded.code, Code::InvalidArgument as i32);
        assert!(decoded.message.contains("protobuf"));
    }

    #[tokio::test]
    async fn bad_json_body_is_400_rpc_status() {
        let (state, _tmp) = test_state(AuthConfig::default());
        let resp = post(
            state,
            "/v1/logs",
            Some("application/json"),
            None,
            None,
            b"{oops".to_vec(),
        )
        .await;
        assert_eq!(resp.status(), StatusCode::BAD_REQUEST);
        let bytes = body_bytes(resp).await;
        let v: serde_json::Value = serde_json::from_slice(&bytes).unwrap();
        assert_eq!(v["code"], serde_json::json!(3));
        assert!(v["message"].as_str().unwrap().contains("JSON"));
    }

    #[tokio::test]
    async fn body_over_wire_limit_is_413() {
        let (state, _tmp) = test_state(AuthConfig::default());
        let over = vec![0u8; MAX_BODY_BYTES + 1];
        let resp = post(
            state,
            "/v1/logs",
            Some("application/x-protobuf"),
            None,
            None,
            over,
        )
        .await;
        assert_eq!(resp.status(), StatusCode::PAYLOAD_TOO_LARGE);
        assert_eq!(body_bytes(resp).await, b"413 request body too large");
    }

    #[tokio::test]
    async fn gzip_round_trip() {
        let (state, tmp) = test_state(AuthConfig::default());
        let body = gzip(&logs_req(2).encode_to_vec());
        let resp = post(
            state,
            "/v1/logs",
            Some("application/x-protobuf"),
            Some("gzip"),
            None,
            body,
        )
        .await;
        assert_eq!(resp.status(), StatusCode::OK);
        assert!(any_file_under(&tmp.path().join("logs/wal")));
    }

    #[tokio::test]
    async fn gzip_expansion_over_limit_is_413() {
        // A small gzip stream expanding past the cap must be refused, not
        // buffered into memory (decompression-bomb guard).
        let (state, _tmp) = test_state(AuthConfig::default());
        let body = gzip(&vec![0u8; MAX_BODY_BYTES + 1024]);
        assert!(body.len() < MAX_BODY_BYTES);
        let resp = post(
            state,
            "/v1/logs",
            Some("application/x-protobuf"),
            Some("gzip"),
            None,
            body,
        )
        .await;
        assert_eq!(resp.status(), StatusCode::PAYLOAD_TOO_LARGE);
    }

    #[tokio::test]
    async fn corrupt_gzip_body_is_400() {
        let (state, _tmp) = test_state(AuthConfig::default());
        let resp = post(
            state,
            "/v1/logs",
            Some("application/json"),
            Some("gzip"),
            None,
            b"definitely-not-gzip".to_vec(),
        )
        .await;
        assert_eq!(resp.status(), StatusCode::BAD_REQUEST);
        let bytes = body_bytes(resp).await;
        let v: serde_json::Value = serde_json::from_slice(&bytes).unwrap();
        assert_eq!(v["code"], serde_json::json!(3));
    }

    /// End-to-end over a real listener: a hand-rolled HTTP/1.1 request (no
    /// client dependencies) must land in the WAL and answer 200.
    #[tokio::test]
    async fn e2e_http11_json_export_lands_in_wal() {
        let (state, tmp) = test_state(AuthConfig::default());
        let listener = tokio::net::TcpListener::bind("127.0.0.1:0").await.unwrap();
        let addr = listener.local_addr().unwrap();
        let server = tokio::spawn(async move {
            serve(HttpListener::Plain(listener), router(state))
                .await
                .unwrap();
        });

        let body = serde_json::to_vec(&logs_req(1)).unwrap();
        let mut stream = tokio::net::TcpStream::connect(addr).await.unwrap();
        let request = format!(
            "POST /v1/logs HTTP/1.1\r\nHost: {addr}\r\nContent-Type: application/json\r\n\
             Content-Length: {}\r\nConnection: close\r\n\r\n",
            body.len(),
        );
        {
            use tokio::io::AsyncWriteExt as _;
            stream.write_all(request.as_bytes()).await.unwrap();
            stream.write_all(&body).await.unwrap();
        }

        let mut response = String::new();
        use tokio::io::AsyncReadExt as _;
        stream.read_to_string(&mut response).await.unwrap();
        assert!(response.starts_with("HTTP/1.1 200"), "{response}");
        assert!(response.contains("content-type: application/json"));

        assert!(any_file_under(&tmp.path().join("logs/wal")));
        server.abort();
    }
}
