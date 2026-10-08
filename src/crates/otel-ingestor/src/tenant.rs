//! Tenant routing for the two multi-tenant OTLP export services (logs +
//! traces): resolves the `TenantId` that keys a request's per-tenant WAL
//! and is forwarded to the ledger. The metrics service shares the
//! transports but is not tenant-scoped and does not use this path.
//!
//! Both transports — the gRPC wrappers and the OTLP/HTTP front end —
//! resolve through one shared inner function, so the tenant policy exists
//! exactly once. The id's validation policy lives on
//! `TenantId::validate_ingest` (file-registry); this module only extracts
//! the header and maps the outcomes onto transport errors.

use bridge::config::AuthConfig;
use file_registry::TenantId;
use tonic::Status;

/// Resolve the tenant a gRPC OTLP export request belongs to.
///
/// Reads `x-scope-orgid` from the tonic metadata map (HTTP/2 headers). The
/// policy outcomes are owned by [`resolve_tenant`], shared with the
/// OTLP/HTTP path.
pub(crate) fn extract_tenant_id(
    metadata: &tonic::metadata::MetadataMap,
    auth: &AuthConfig,
) -> Result<TenantId, Status> {
    resolve_tenant(
        metadata
            .get(AuthConfig::TENANT_HEADER)
            .map(|value| value.as_bytes()),
        auth,
    )
}

/// Resolve the tenant an OTLP/HTTP export request belongs to.
///
/// Reads `x-scope-orgid` from the HTTP header map. The policy outcomes are
/// owned by [`resolve_tenant`], the same code the gRPC path runs, so both
/// transports enforce identical rules.
pub(crate) fn extract_tenant_id_from_headers(
    headers: &http::HeaderMap,
    auth: &AuthConfig,
) -> Result<TenantId, Status> {
    resolve_tenant(
        headers
            .get(AuthConfig::TENANT_HEADER)
            .map(|value| value.as_bytes()),
        auth,
    )
}

/// The single tenant-resolution policy, shared by both transports.
///
/// With auth disabled every request routes to the default tenant without
/// examining headers. Otherwise the `x-scope-orgid` header
/// (`AuthConfig::TENANT_HEADER`) is required: missing → `unauthenticated`;
/// non-UTF-8 bytes → `invalid_argument`; an id rejected by the ingest
/// policy → `invalid_argument` carrying the policy's reason.
///
/// Takes the raw header bytes because that is the one representation both
/// caller map types (tonic `MetadataMap` and `http::HeaderMap`) share, so
/// the policy is written once and cannot drift between transports.
fn resolve_tenant(raw: Option<&[u8]>, auth: &AuthConfig) -> Result<TenantId, Status> {
    if !auth.enabled {
        return Ok(TenantId::default_tenant());
    }
    let raw = raw.ok_or_else(|| Status::unauthenticated("missing tenant header"))?;
    let tenant = std::str::from_utf8(raw)
        .map_err(|_| Status::invalid_argument("tenant header must be valid UTF-8"))?;
    // The strict id policy (it becomes a per-tenant directory name) lives on
    // `TenantId::validate_ingest`; this layer only wraps its reason as the
    // transport error.
    TenantId::validate_ingest(tenant).map_err(Status::invalid_argument)
}
