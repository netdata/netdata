//! Tenant extraction shared by every ingestion gRPC service.

use bridge::config::AuthConfig;
use file_registry::TenantId;
use tonic::Status;

/// Resolve the request's tenant from the `X-Scope-OrgID` header, or the
/// default tenant when auth is disabled.
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
    // The id policy (strict: becomes a directory name) lives on the
    // type; this layer only maps the reason onto the transport error.
    TenantId::validate_ingest(tenant).map_err(Status::invalid_argument)
}
