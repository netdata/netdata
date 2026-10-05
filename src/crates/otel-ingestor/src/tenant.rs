//! Tenant routing for the two multi-tenant OTLP export services (logs +
//! traces): resolves the `TenantId` that keys a request's per-tenant WAL
//! and is forwarded to the ledger. The metrics service shares the gRPC
//! endpoint but is not tenant-scoped and does not use this path.
//!
//! The id's validation policy lives on `TenantId::validate_ingest`
//! (file-registry); this module only extracts the header and maps the
//! outcomes onto transport errors.

use bridge::config::AuthConfig;
use file_registry::TenantId;
use tonic::Status;

/// Resolve the tenant an OTLP export request belongs to.
///
/// With auth disabled every request routes to the default tenant without
/// examining headers. Otherwise the `x-scope-orgid` header is required:
/// missing → `unauthenticated`; non-UTF-8 header value → `invalid_argument`;
/// an id rejected by the ingest policy → `invalid_argument` carrying the
/// policy's reason.
pub(crate) fn extract_tenant_id(
    metadata: &tonic::metadata::MetadataMap,
    auth: &AuthConfig,
) -> Result<TenantId, Status> {
    if !auth.enabled {
        return Ok(TenantId::default_tenant());
    }
    let value = metadata
        .get(AuthConfig::TENANT_HEADER)
        .ok_or_else(|| Status::unauthenticated("missing tenant header"))?;
    let tenant = value
        .to_str()
        .map_err(|_| Status::invalid_argument("tenant header must be valid UTF-8"))?;
    // The strict id policy (it becomes a per-tenant directory name) lives on
    // `TenantId::validate_ingest`; this layer only wraps its reason as the
    // transport error.
    TenantId::validate_ingest(tenant).map_err(Status::invalid_argument)
}
