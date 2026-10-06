//! The OTLP/HTTP receiver: a second listener (config
//! `receivers.otlp.protocols.http`, stock address `127.0.0.1:4318`, off by
//! default) that serves `POST /v1/{logs,traces,metrics}` with
//! collector-parity semantics, so SDKs and exporters defaulting to
//! `http/protobuf` or `http/json` reach the same ingestion cores as gRPC.
//!
//! Contract (follows open-telemetry/opentelemetry-collector
//! `receiver/otlpreceiver/otlphttp.go`, except where marked "local"):
//!
//! - method mismatch → `405 text/plain` ("405 method not allowed, supported:
//!   [POST]"); unmatched path → `404 text/plain`;
//! - `Content-Type`: the media type before any `;` is matched
//!   case-insensitively; exactly `application/x-protobuf` and
//!   `application/json` are accepted, anything else → `415 text/plain`
//!   (local: parameters are ignored, not validated — upstream's
//!   `mime.ParseMediaType` 415s a malformed one);
//! - `Content-Encoding`: absent, `identity` and `gzip` (flate2) are
//!   accepted, anything else → `400` with a `google.rpc.Status` (local: the
//!   set — upstream also decompresses `zstd`, `zlib`, `deflate`, `snappy`,
//!   `lz4` and `x-snappy-framed`, and 400s an explicit `identity`, which its
//!   own tests flag as a bug; and the order — upstream's decompressor runs
//!   before the Content-Type check, so a request bad in both gets its 400
//!   where this receiver answers the Content-Type 415);
//! - bodies are capped (see [`MAX_BODY_BYTES`], applied to the wire body AND
//!   the gzip-expanded body — the second cap is the decompression-bomb guard)
//!   → over-limit `400` with a `google.rpc.Status` (local: the limit —
//!   upstream caps at 20 MiB);
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
//! JSON bodies are OTLP/JSON (hex ids, integer enums, 64-bit integers as
//! decimal strings or numbers, omitted fields), decoded by
//! [`crate::otlp_json`], which repairs the `opentelemetry-proto` derives'
//! narrower JSON forms first. Deviations from the collector's decoder: enum
//! names and the `NaN`/`Infinity` doubles are rejected with a `400`, never
//! dropped silently. Metric losses and the rewrite's own rejections name the
//! offending JSON field path; errors from the derives carry serde's message.
//!
//! The listener binds during startup with the same strict fail-fast rule as
//! the gRPC endpoint: a bind or TLS failure aborts the worker before `Ready`,
//! rather than advertising an endpoint that cannot receive. TLS terminates via `tokio-rustls`, configured independently of
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
use bridge::config::{AuthConfig, ProtocolConfig};
use file_registry::TenantId;
use opentelemetry_proto::tonic::collector::logs::v1::ExportLogsServiceRequest;
use opentelemetry_proto::tonic::collector::metrics::v1::ExportMetricsServiceRequest;
use opentelemetry_proto::tonic::collector::trace::v1::ExportTraceServiceRequest;
use prost::Message as _;
use tonic::{Code, Status};

use crate::logs_service::NetdataLogsService;
use crate::metrics_service::NetdataMetricsService;
use crate::otlp_json::{OtlpJson, OtlpJsonResponse};
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
/// clone: three `Arc`s and a one-flag `AuthConfig`.
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

    /// Bytes for a successful export response, in this codec.
    fn encode_response<R: prost::Message + OtlpJsonResponse>(self, resp: &R) -> Vec<u8> {
        match self {
            Codec::Protobuf => resp.encode_to_vec(),
            Codec::Json => resp.encode_json(),
        }
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
/// listener's `tls` cert and key are configured.
pub(crate) enum HttpListener {
    Plain(tokio::net::TcpListener),
    Tls(TlsListener),
}

/// Bind the OTLP/HTTP listener (and build its TLS acceptor when configured).
///
/// Returns `Ok(None)` when the receiver is disabled (`enabled: false`).
/// Every failure is fatal — strict fail-fast, symmetric with the gRPC
/// bind — which is why this runs before the worker touches its WAL dirs and
/// before `Ready` is sent.
pub(crate) async fn bind_http(listener: &ProtocolConfig) -> Result<Option<HttpListener>> {
    if !listener.enabled {
        tracing::info!(
            "OTLP/HTTP receiver disabled (receivers.otlp.protocols.http.enabled is false)"
        );
        return Ok(None);
    }
    let path = &listener.endpoint;
    let tls = &listener.tls;

    let addr: SocketAddr = path
        .parse()
        .with_context(|| format!("failed to parse OTLP/HTTP endpoint address: {path}"))?;
    let listener = tokio::net::TcpListener::bind(addr)
        .await
        .with_context(|| format!("failed to bind OTLP/HTTP endpoint {path}"))?;
    tracing::info!(endpoint = %path, "OTLP/HTTP endpoint bound");

    match (tls.cert_file.as_ref(), tls.key_file.as_ref()) {
        (Some(cert_path), Some(key_path)) => {
            let acceptor = build_tls_acceptor(cert_path, key_path, tls.client_ca_file.as_ref())
                .context("failed to configure OTLP/HTTP TLS")?;
            Ok(Some(HttpListener::Tls(TlsListener::new(
                listener, acceptor,
            ))))
        }
        (None, None) => {
            tracing::warn!("TLS disabled, using insecure connection on OTLP/HTTP endpoint: {path}");
            Ok(Some(HttpListener::Plain(listener)))
        }
        // `validate()` rejects this pairing at config time; reaching here
        // means the supervisor handed us an unvalidated config.
        _ => bail!(
            "OTLP/HTTP TLS requires both a certificate and a private key (tls.cert_file / tls.key_file)"
        ),
    }
}

/// Serve the router on the bound listener until the server future is dropped.
/// axum's serve loop never returns (it retries accept errors itself), so the
/// `Result` only satisfies axum's signature; the contexts name the transport
/// should that ever change.
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

/// How long a client may take to complete its TLS handshake before the
/// connection is dropped.
const TLS_HANDSHAKE_TIMEOUT: std::time::Duration = std::time::Duration::from_secs(10);

/// A `tokio-rustls` acceptor wrapper implementing axum's `Listener` contract:
/// `axum::serve` is generic over listeners whose IO types are async
/// read/write, so TLS termination sits in front of the plain HTTP serving
/// machinery instead of a second server stack.
///
/// axum awaits `accept` before serving each connection, so the handshake must
/// not run inline: one client that opens TCP and never sends a ClientHello
/// would stop every other sender from being accepted. Each handshake runs on
/// its own task with a timeout instead, and `accept` returns whichever
/// completes first (tonic's server pattern). Dropping the listener — server
/// shutdown — aborts the handshakes still in flight.
pub(crate) struct TlsListener {
    listener: tokio::net::TcpListener,
    acceptor: tokio_rustls::TlsAcceptor,
    /// In-flight handshakes; a task yields `None` when its handshake failed
    /// or timed out (already logged).
    handshakes: tokio::task::JoinSet<Option<TlsConnection>>,
}

type TlsConnection = (
    tokio_rustls::server::TlsStream<tokio::net::TcpStream>,
    SocketAddr,
);

impl TlsListener {
    fn new(listener: tokio::net::TcpListener, acceptor: tokio_rustls::TlsAcceptor) -> Self {
        Self {
            listener,
            acceptor,
            handshakes: tokio::task::JoinSet::new(),
        }
    }

    fn start_handshake(&mut self, stream: tokio::net::TcpStream, peer: SocketAddr) {
        // Same default the gRPC path restores on its sockets.
        if let Err(e) = stream.set_nodelay(true) {
            tracing::warn!(%e, "failed to set TCP_NODELAY on OTLP/HTTP connection");
        }
        let acceptor = self.acceptor.clone();
        self.handshakes.spawn(async move {
            match tokio::time::timeout(TLS_HANDSHAKE_TIMEOUT, acceptor.accept(stream)).await {
                Ok(Ok(io)) => Some((io, peer)),
                // An untrusted client or a bad ALPN costs one connection,
                // never the listener.
                Ok(Err(e)) => {
                    tracing::debug!(%e, %peer, "OTLP/HTTP TLS handshake failed");
                    None
                }
                Err(_) => {
                    tracing::debug!(%peer, "OTLP/HTTP TLS handshake timed out");
                    None
                }
            }
        });
    }
}

impl axum::serve::Listener for TlsListener {
    type Io = tokio_rustls::server::TlsStream<tokio::net::TcpStream>;
    type Addr = std::net::SocketAddr;

    async fn accept(&mut self) -> (Self::Io, Self::Addr) {
        loop {
            // Both branches are cancel-safe: the worker drops the server
            // future at shutdown, which can cancel `accept` mid-flight.
            tokio::select! {
                accepted = self.listener.accept() => match accepted {
                    Ok((stream, peer)) => self.start_handshake(stream, peer),
                    Err(e) => handle_accept_error(e).await,
                },
                // `join_next` on an empty set resolves at once; the guard
                // keeps the loop from spinning.
                Some(done) = self.handshakes.join_next(), if !self.handshakes.is_empty() => {
                    match done {
                        Ok(Some(connection)) => return connection,
                        Ok(None) => {}
                        Err(e) => {
                            tracing::debug!(%e, "OTLP/HTTP TLS handshake task ended abnormally");
                        }
                    }
                }
            }
        }
    }

    fn local_addr(&self) -> std::io::Result<Self::Addr> {
        self.listener.local_addr()
    }
}

/// axum's accept-error policy (axum `serve::listener`): a failure tied to one
/// connection is retried at once; anything else (e.g. `EMFILE` under fd
/// pressure) is logged and waited out for a second, since retrying a full
/// descriptor table immediately would spin.
async fn handle_accept_error(e: std::io::Error) {
    use std::io::ErrorKind;
    if matches!(
        e.kind(),
        ErrorKind::ConnectionRefused | ErrorKind::ConnectionAborted | ErrorKind::ConnectionReset
    ) {
        return;
    }
    tracing::error!(%e, "OTLP/HTTP accept failed");
    tokio::time::sleep(std::time::Duration::from_secs(1)).await;
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
    // mirroring `tls.client_ca_file` on the gRPC endpoint; without one it serves
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
    serve_export::<ExportLogsServiceRequest, _, _, _>(
        headers,
        body,
        move |headers, req| async move {
            let tenant: TenantId = extract_tenant_id_from_headers(&headers, &state.auth)?;
            state.logs.export_logs(&tenant, req).await
        },
    )
    .await
}

async fn export_traces(
    State(state): State<HttpState>,
    headers: HeaderMap,
    body: Body,
) -> Response<Body> {
    serve_export::<ExportTraceServiceRequest, _, _, _>(
        headers,
        body,
        move |headers, req| async move {
            let tenant: TenantId = extract_tenant_id_from_headers(&headers, &state.auth)?;
            state.traces.export_traces(&tenant, req).await
        },
    )
    .await
}

async fn export_metrics(
    State(state): State<HttpState>,
    headers: HeaderMap,
    body: Body,
) -> Response<Body> {
    serve_export::<ExportMetricsServiceRequest, _, _, _>(
        headers,
        body,
        move |_headers, req| async move {
            // Metrics carry no tenant (charts are agent-wide); the core's own
            // signature says so.
            state.metrics.export_metrics(req).await
        },
    )
    .await
}

/// The transport pipeline shared by all three signals: content negotiation,
/// body limits, decompression, decode, then the signal's export.
///
/// `run` receives the request headers (handed over, not copied, once content
/// negotiation is done with them) and the decoded request, and answers with
/// the core's `Result<Response, Status>` — the same cores the gRPC wrappers
/// call — so the two transports cannot drift apart.
async fn serve_export<Req, Resp, F, Fut>(headers: HeaderMap, body: Body, run: F) -> Response<Body>
where
    Req: prost::Message + OtlpJson + Default,
    Resp: prost::Message + OtlpJsonResponse,
    F: FnOnce(HeaderMap, Req) -> Fut + Send,
    Fut: Future<Output = Result<Resp, Status>> + Send,
{
    // Content-Type first, exactly like the collector's `readContentType`.
    let Some(codec) = content_type_codec(headers.get(axum::http::header::CONTENT_TYPE)) else {
        return plain_text(
            StatusCode::UNSUPPORTED_MEDIA_TYPE,
            "415 unsupported media type, supported: [application/json, application/x-protobuf]",
        );
    };

    // An unsupported coding and an oversized body get the collector's
    // answer: 400 with a google.rpc.Status (its decompressor middleware and
    // MaxBytesReader), not the plain HTTP 415/413.
    let encoding = headers.get(axum::http::header::CONTENT_ENCODING);
    let Some(coding) = content_encoding(encoding) else {
        let name = encoding.map(|v| String::from_utf8_lossy(v.as_bytes()).into_owned());
        return rpc_error(
            codec,
            Status::invalid_argument(format!(
                "unsupported Content-Encoding: {}, supported: [gzip, identity]",
                name.unwrap_or_default()
            )),
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
                return too_large(codec);
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
            // `MultiGzDecoder` (not `GzDecoder`) so a concatenated-members
            // stream — legal gzip that Go's compress/gzip also reads whole —
            // is not silently truncated after its first member.
            let mut reader =
                flate2::read::MultiGzDecoder::new(wire.as_ref()).take((MAX_BODY_BYTES + 1) as u64);
            match reader.read_to_end(&mut out) {
                // Ok(n) with n <= cap: EOF ended the stream inside the limit.
                Ok(_) if out.len() <= MAX_BODY_BYTES => out.into(),
                // Ok with more than the cap: the `take` bound, not EOF, ended
                // the read — the expanded body is over the limit.
                Ok(_) => return too_large(codec),
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
        Codec::Json => match Req::decode_json(&body_bytes) {
            Ok(req) => req,
            Err(e) => {
                return rpc_error(
                    codec,
                    Status::invalid_argument(format!("failed to decode JSON body: {e}")),
                );
            }
        },
    };

    match run(headers, req).await {
        Ok(resp) => response_bytes(
            StatusCode::OK,
            codec.content_type(),
            codec.encode_response(&resp),
        ),
        Err(status) => rpc_error(codec, status),
    }
}

/// The request's MIME type → codec. Parameters are stripped and the type is
/// matched case-insensitively (`application/json; charset=utf-8` is JSON), as
/// `mime.ParseMediaType` does upstream; unlike it, malformed parameters are
/// not rejected. A missing header is unsupported, matching the collector.
fn content_type_codec(value: Option<&axum::http::HeaderValue>) -> Option<Codec> {
    let mime = value?.to_str().ok()?.split(';').next().unwrap_or("").trim();
    if mime.eq_ignore_ascii_case("application/x-protobuf") {
        Some(Codec::Protobuf)
    } else if mime.eq_ignore_ascii_case("application/json") {
        Some(Codec::Json)
    } else {
        None
    }
}

enum BodyCoding {
    Identity,
    Gzip,
}

/// The over-limit answer, on the wire or after gzip expansion.
fn too_large(codec: Codec) -> Response<Body> {
    rpc_error(
        codec,
        Status::invalid_argument(format!(
            "request body exceeds the {} MiB limit",
            MAX_BODY_BYTES / (1024 * 1024)
        )),
    )
}

/// The request's `Content-Encoding` (case-insensitive). Absent or
/// `identity` means a raw body; anything but `gzip` is unsupported (see the
/// module doc for how this differs from the collector).
fn content_encoding(value: Option<&axum::http::HeaderValue>) -> Option<BodyCoding> {
    let Some(value) = value else {
        return Some(BodyCoding::Identity);
    };
    let coding = value.to_str().ok()?.trim();
    if coding.is_empty() || coding.eq_ignore_ascii_case("identity") {
        Some(BodyCoding::Identity)
    } else if coding.eq_ignore_ascii_case("gzip") {
        Some(BodyCoding::Gzip)
    } else {
        None
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

/// The collector's gRPC→HTTP table (otlpreceiver `GetHTTPStatusCodeFromStatus`).
/// Today only three codes reach it: `InvalidArgument` (undecodable body,
/// invalid tenant), `Unauthenticated` (missing tenant header) and `Internal`
/// (frame encoding and WAL failures). The other arms are kept so a future
/// core error maps correctly without touching the transport — e.g.
/// throttling's `ResourceExhausted` becomes the retryable 429. The 503 arm is
/// the spec's retryable set; `Unimplemented` → 404 is the collector's choice.
fn http_status_for(code: Code) -> StatusCode {
    match code {
        Code::Cancelled
        | Code::DeadlineExceeded
        | Code::Aborted
        | Code::OutOfRange
        | Code::Unavailable
        | Code::DataLoss => StatusCode::SERVICE_UNAVAILABLE,
        Code::ResourceExhausted => StatusCode::TOO_MANY_REQUESTS,
        Code::InvalidArgument => StatusCode::BAD_REQUEST,
        Code::Unauthenticated => StatusCode::UNAUTHORIZED,
        Code::PermissionDenied => StatusCode::FORBIDDEN,
        Code::Unimplemented => StatusCode::NOT_FOUND,
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
    // net/http's mux writes this body with a trailing newline; keep byte
    // parity with what the collector's 404 looks like on the wire.
    plain_text(StatusCode::NOT_FOUND, "404 page not found\n")
}

#[cfg(test)]
mod tests {
    use std::collections::HashMap;
    use std::io::Write as _;
    use std::path::Path;
    use std::sync::{Arc, Mutex};

    use axum::body::Body;
    use axum::http::{Request, StatusCode, header};
    use bridge::config::{AuthConfig, IngestConfig, RotationEntry, TlsServerConfig, WalConfig};
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
            serde_json::json!("2")
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
        assert_eq!(body_bytes(resp).await, b"404 page not found\n");
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
    async fn unsupported_content_encoding_is_400_rpc_status() {
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
        // Collector parity: its decompressor answers 400 with a
        // google.rpc.Status, in the request's encoding.
        assert_eq!(resp.status(), StatusCode::BAD_REQUEST);
        let v: serde_json::Value = serde_json::from_slice(&body_bytes(resp).await).unwrap();
        assert_eq!(v["code"], serde_json::json!(3));
        assert_eq!(
            v["message"],
            serde_json::json!("unsupported Content-Encoding: br, supported: [gzip, identity]")
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

    /// Every gRPC code, against the collector's table (otlpreceiver
    /// `internal/errors/errors.go`, `GetHTTPStatusCodeFromStatus`).
    #[test]
    fn grpc_codes_map_to_the_collector_http_statuses() {
        let cases = [
            (Code::Ok, StatusCode::INTERNAL_SERVER_ERROR),
            (Code::Cancelled, StatusCode::SERVICE_UNAVAILABLE),
            (Code::Unknown, StatusCode::INTERNAL_SERVER_ERROR),
            (Code::InvalidArgument, StatusCode::BAD_REQUEST),
            (Code::DeadlineExceeded, StatusCode::SERVICE_UNAVAILABLE),
            (Code::NotFound, StatusCode::INTERNAL_SERVER_ERROR),
            (Code::AlreadyExists, StatusCode::INTERNAL_SERVER_ERROR),
            (Code::PermissionDenied, StatusCode::FORBIDDEN),
            (Code::ResourceExhausted, StatusCode::TOO_MANY_REQUESTS),
            (Code::FailedPrecondition, StatusCode::INTERNAL_SERVER_ERROR),
            (Code::Aborted, StatusCode::SERVICE_UNAVAILABLE),
            (Code::OutOfRange, StatusCode::SERVICE_UNAVAILABLE),
            (Code::Unimplemented, StatusCode::NOT_FOUND),
            (Code::Internal, StatusCode::INTERNAL_SERVER_ERROR),
            (Code::Unavailable, StatusCode::SERVICE_UNAVAILABLE),
            (Code::DataLoss, StatusCode::SERVICE_UNAVAILABLE),
            (Code::Unauthenticated, StatusCode::UNAUTHORIZED),
        ];
        for (code, expected) in cases {
            assert_eq!(http_status_for(code), expected, "{code:?}");
        }
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
    async fn json_metric_the_codec_cannot_hold_is_400_not_a_silent_200() {
        // A `"NaN"` double is valid OTLP/JSON the codec cannot represent;
        // without the loss check the gauge would vanish behind a 200.
        let (state, _tmp) = test_state(AuthConfig::default());
        let body = br#"{"resourceMetrics":[{"scopeMetrics":[{"metrics":[
            {"name":"m","gauge":{"dataPoints":[{"timeUnixNano":"1","asDouble":"NaN"}]}}]}]}]}"#;
        let resp = post(
            state,
            "/v1/metrics",
            Some("application/json"),
            None,
            None,
            body.to_vec(),
        )
        .await;
        assert_eq!(resp.status(), StatusCode::BAD_REQUEST);
        let v: serde_json::Value = serde_json::from_slice(&body_bytes(resp).await).unwrap();
        assert_eq!(v["code"], serde_json::json!(3));
        let message = v["message"].as_str().unwrap();
        assert!(
            message.contains("metrics[0].gauge.dataPoints[0]: value cannot be decoded"),
            "{message}"
        );
    }

    #[tokio::test]
    async fn body_over_wire_limit_is_400_rpc_status() {
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
        assert_too_large(resp).await;
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
    async fn concatenated_gzip_members_all_decode() {
        // `cat a.gz b.gz` is legal gzip with two members; Go's compress/gzip
        // reads every member. Split ONE valid request across two members so
        // the reassembled body is a single JSON document — a first-member-
        // only decoder would see truncated JSON and 400 it.
        let (state, _tmp) = test_state(AuthConfig::default());
        let json = serde_json::to_vec(&logs_req(1)).unwrap();
        let split = json.len() / 2;
        let body = [gzip(&json[..split]), gzip(&json[split..])].concat();
        let resp = post(
            state,
            "/v1/logs",
            Some("application/json"),
            Some("gzip"),
            None,
            body,
        )
        .await;
        assert_eq!(resp.status(), StatusCode::OK);
    }

    #[tokio::test]
    async fn gzip_expansion_over_limit_is_400_rpc_status() {
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
        assert_too_large(resp).await;
    }

    /// The over-limit answer for a protobuf request: collector parity, 400
    /// with a protobuf google.rpc.Status (its MaxBytesReader error).
    async fn assert_too_large(resp: Response<Body>) {
        assert_eq!(resp.status(), StatusCode::BAD_REQUEST);
        assert_eq!(
            resp.headers().get(header::CONTENT_TYPE).unwrap(),
            "application/x-protobuf"
        );
        let decoded = RpcStatus::decode(body_bytes(resp).await.as_ref()).unwrap();
        assert_eq!(decoded.code, Code::InvalidArgument as i32);
        assert_eq!(decoded.message, "request body exceeds the 4 MiB limit");
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

        let stream = tokio::net::TcpStream::connect(addr).await.unwrap();
        let response = post_json_logs_http11(stream, addr).await.unwrap();
        assert!(response.starts_with("HTTP/1.1 200"), "{response}");
        assert!(response.contains("content-type: application/json"));

        assert!(any_file_under(&tmp.path().join("logs/wal")));
        server.abort();
    }

    /// POST one JSON logs export as a hand-rolled HTTP/1.1 request (no client
    /// dependency) over any byte stream, plain or TLS, and return the raw
    /// response. Errors surface as `Err` so TLS tests can assert rejections.
    async fn post_json_logs_http11<S>(mut stream: S, addr: SocketAddr) -> std::io::Result<String>
    where
        S: tokio::io::AsyncRead + tokio::io::AsyncWrite + Unpin,
    {
        use tokio::io::{AsyncReadExt as _, AsyncWriteExt as _};

        let body = serde_json::to_vec(&logs_req(1)).unwrap();
        let request = format!(
            "POST /v1/logs HTTP/1.1\r\nHost: {addr}\r\nContent-Type: application/json\r\n\
             Content-Length: {}\r\nConnection: close\r\n\r\n",
            body.len(),
        );
        stream.write_all(request.as_bytes()).await?;
        stream.write_all(&body).await?;
        stream.flush().await?;

        let mut response = String::new();
        stream.read_to_string(&mut response).await?;
        Ok(response)
    }

    // -- TLS listener --------------------------------------------------------

    /// The process-level rustls provider the otel-plugin binary installs at
    /// startup; tests share one process, so a repeat install is expected.
    fn install_crypto_provider() {
        let _ = rustls::crypto::ring::default_provider().install_default();
    }

    /// A throwaway PKI generated per test, so no key material lives in the
    /// repository: a CA, a server certificate for `localhost`/`127.0.0.1`,
    /// and an mTLS client certificate. The listener reads PEM files, so the
    /// CA and server material are also written under `dir`.
    struct TestPki {
        dir: tempfile::TempDir,
        ca: rustls::pki_types::CertificateDer<'static>,
        client_cert: rustls::pki_types::CertificateDer<'static>,
        client_key: rustls::pki_types::PrivateKeyDer<'static>,
    }

    impl TestPki {
        fn new() -> Self {
            use rcgen::{
                BasicConstraints, CertificateParams, ExtendedKeyUsagePurpose, IsCa, Issuer,
                KeyPair, KeyUsagePurpose,
            };

            let ca_key = KeyPair::generate().unwrap();
            let mut ca_params = CertificateParams::new(Vec::<String>::new()).unwrap();
            ca_params.is_ca = IsCa::Ca(BasicConstraints::Unconstrained);
            ca_params.key_usages = vec![KeyUsagePurpose::KeyCertSign];
            let ca_cert = ca_params.self_signed(&ca_key).unwrap();
            let issuer = Issuer::new(ca_params, ca_key);

            let leaf = |names: Vec<String>, usage: ExtendedKeyUsagePurpose| {
                let key = KeyPair::generate().unwrap();
                let mut params = CertificateParams::new(names).unwrap();
                params.extended_key_usages = vec![usage];
                let cert = params.signed_by(&key, &issuer).unwrap();
                (cert, key)
            };
            let (server_cert, server_key) = leaf(
                vec!["localhost".into(), "127.0.0.1".into()],
                ExtendedKeyUsagePurpose::ServerAuth,
            );
            let (client_cert, client_key) = leaf(Vec::new(), ExtendedKeyUsagePurpose::ClientAuth);

            let dir = tempfile::tempdir().unwrap();
            std::fs::write(dir.path().join("ca.pem"), ca_cert.pem()).unwrap();
            std::fs::write(dir.path().join("server.pem"), server_cert.pem()).unwrap();
            std::fs::write(dir.path().join("server.key"), server_key.serialize_pem()).unwrap();
            Self {
                dir,
                ca: ca_cert.der().clone(),
                client_cert: client_cert.der().clone(),
                client_key: rustls::pki_types::PrivatePkcs8KeyDer::from(client_key.serialize_der())
                    .into(),
            }
        }

        fn path(&self, name: &str) -> String {
            self.dir.path().join(name).display().to_string()
        }

        fn endpoint(&self, mtls: bool) -> ProtocolConfig {
            ProtocolConfig {
                enabled: true,
                endpoint: "127.0.0.1:0".to_string(),
                tls: TlsServerConfig {
                    cert_file: Some(self.path("server.pem")),
                    key_file: Some(self.path("server.key")),
                    client_ca_file: mtls.then(|| self.path("ca.pem")),
                },
            }
        }

        /// A client trusting the CA, offering exactly `alpn`, optionally
        /// presenting the client certificate.
        fn client(&self, alpn: &[&[u8]], client_cert: bool) -> tokio_rustls::TlsConnector {
            let mut roots = rustls::RootCertStore::empty();
            roots.add(self.ca.clone()).unwrap();
            let builder = rustls::ClientConfig::builder().with_root_certificates(roots);
            let mut config = if client_cert {
                builder
                    .with_client_auth_cert(
                        vec![self.client_cert.clone()],
                        self.client_key.clone_key(),
                    )
                    .unwrap()
            } else {
                builder.with_no_client_auth()
            };
            config.alpn_protocols = alpn.iter().map(|p| p.to_vec()).collect();
            tokio_rustls::TlsConnector::from(Arc::new(config))
        }
    }

    /// Bind the TLS listener from `endpoint` and serve a test state on it.
    async fn spawn_tls_server(
        endpoint: &ProtocolConfig,
    ) -> (SocketAddr, tokio::task::JoinHandle<()>, tempfile::TempDir) {
        let Some(HttpListener::Tls(listener)) = bind_http(endpoint).await.unwrap() else {
            panic!("a configured cert/key pair must yield a TLS listener");
        };
        let addr = listener.listener.local_addr().unwrap();
        let (state, tmp) = test_state(AuthConfig::default());
        let server = tokio::spawn(async move {
            serve(HttpListener::Tls(listener), router(state))
                .await
                .unwrap();
        });
        (addr, server, tmp)
    }

    async fn tls_connect(
        connector: &tokio_rustls::TlsConnector,
        addr: SocketAddr,
    ) -> std::io::Result<tokio_rustls::client::TlsStream<tokio::net::TcpStream>> {
        let tcp = tokio::net::TcpStream::connect(addr).await?;
        let name = rustls::pki_types::ServerName::try_from("localhost").unwrap();
        connector.connect(name, tcp).await
    }

    #[tokio::test]
    async fn tls_listener_negotiates_http1_and_h2_and_lands_exports() {
        install_crypto_provider();
        let pki = TestPki::new();
        let (addr, server, tmp) = spawn_tls_server(&pki.endpoint(false)).await;

        // A client offering ONLY http/1.1 (Python requests/urllib3 does this)
        // must negotiate it, not fail the handshake.
        let stream = tls_connect(&pki.client(&[b"http/1.1"], false), addr)
            .await
            .unwrap();
        assert_eq!(stream.get_ref().1.alpn_protocol(), Some(&b"http/1.1"[..]));
        let response = post_json_logs_http11(stream, addr).await.unwrap();
        assert!(response.starts_with("HTTP/1.1 200"), "{response}");
        assert!(any_file_under(&tmp.path().join("logs/wal")));

        // A client preferring h2 gets h2.
        let stream = tls_connect(&pki.client(&[b"h2", b"http/1.1"], false), addr)
            .await
            .unwrap();
        assert_eq!(stream.get_ref().1.alpn_protocol(), Some(&b"h2"[..]));

        server.abort();
    }

    #[tokio::test]
    async fn mtls_listener_rejects_anonymous_clients_and_keeps_serving() {
        install_crypto_provider();
        let pki = TestPki::new();
        let (addr, server, _tmp) = spawn_tls_server(&pki.endpoint(true)).await;

        // Without a client certificate the request never gets a response.
        // TLS 1.3 completes the client side of the handshake before the server
        // checks the certificate, so the rejection may surface on connect or
        // on the first read; either way no HTTP response arrives.
        let anonymous = match tls_connect(&pki.client(&[b"http/1.1"], false), addr).await {
            Ok(stream) => post_json_logs_http11(stream, addr).await,
            Err(e) => Err(e),
        };
        assert!(
            !anonymous.as_deref().is_ok_and(|r| r.starts_with("HTTP/")),
            "an anonymous client must not be served: {anonymous:?}"
        );

        // The failed handshake cost one connection, not the listener.
        let stream = tls_connect(&pki.client(&[b"http/1.1"], true), addr)
            .await
            .unwrap();
        let response = post_json_logs_http11(stream, addr).await.unwrap();
        assert!(response.starts_with("HTTP/1.1 200"), "{response}");

        server.abort();
    }

    #[tokio::test]
    async fn tls_listener_serves_others_while_a_handshake_stalls() {
        // A client that opens TCP and never sends a ClientHello must not hold
        // the listener: each handshake runs on its own, so other senders are
        // still served while it hangs.
        install_crypto_provider();
        let pki = TestPki::new();
        let (addr, server, _tmp) = spawn_tls_server(&pki.endpoint(false)).await;

        let _stalled = tokio::net::TcpStream::connect(addr).await.unwrap();
        // Let the listener pick the stalled connection up first.
        tokio::time::sleep(std::time::Duration::from_millis(100)).await;

        let served = tokio::time::timeout(std::time::Duration::from_secs(5), async {
            let stream = tls_connect(&pki.client(&[b"http/1.1"], false), addr).await?;
            post_json_logs_http11(stream, addr).await
        })
        .await
        .expect("a stalled handshake must not block other clients")
        .unwrap();
        assert!(served.starts_with("HTTP/1.1 200"), "{served}");

        server.abort();
    }

    #[tokio::test]
    async fn unreadable_tls_material_fails_the_bind() {
        install_crypto_provider();
        let pki = TestPki::new();
        let mut endpoint = pki.endpoint(false);
        endpoint.tls.key_file = Some(pki.path("missing.key"));
        let err = bind_http(&endpoint)
            .await
            .err()
            .expect("missing key must fail");
        assert!(
            format!("{err:#}").contains("failed to configure OTLP/HTTP TLS"),
            "{err:#}"
        );
    }

    #[tokio::test]
    async fn disabled_http_listener_binds_nothing() {
        // Even an address that cannot bind is never touched when disabled.
        let listener = ProtocolConfig {
            enabled: false,
            endpoint: "not-an-address".to_string(),
            tls: TlsServerConfig::default(),
        };
        assert!(bind_http(&listener).await.unwrap().is_none());
    }

    #[tokio::test]
    async fn enabled_http_listener_binds_or_fails_fast() {
        let listener = |endpoint: &str| ProtocolConfig {
            enabled: true,
            endpoint: endpoint.to_string(),
            tls: TlsServerConfig::default(),
        };
        assert!(bind_http(&listener("127.0.0.1:0")).await.unwrap().is_some());

        let err = bind_http(&listener("not-an-address")).await.err().unwrap();
        assert!(
            format!("{err:#}").contains("failed to parse OTLP/HTTP endpoint address"),
            "{err:#}"
        );

        let taken = std::net::TcpListener::bind("127.0.0.1:0").unwrap();
        let addr = taken.local_addr().unwrap().to_string();
        let err = bind_http(&listener(&addr)).await.err().unwrap();
        assert!(
            format!("{err:#}").contains("failed to bind OTLP/HTTP endpoint"),
            "{err:#}"
        );
    }
}
