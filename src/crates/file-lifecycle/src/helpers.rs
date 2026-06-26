//! Shared helpers.

use file_registry::{FileId, TenantId, TimestampNs};

use crate::ipc::UploaderRequest;
use crate::registry::Registry;

/// Convert a summary's `min_timestamp_s` to the calendar date used for
/// catalog partitioning.
///
/// Returns `None` for an empty SFST (`record_count == 0`) or a timestamp
/// outside the representable chrono range. Callers fall back to the
/// current date when `None` — encoded once at each call site rather than
/// hidden inside this helper, so the fallback is visible.
pub fn date_from_summary(summary: &sfst::Summary) -> Option<chrono::NaiveDate> {
    if summary.record_count == 0 {
        return None;
    }
    chrono::DateTime::from_timestamp(summary.min_timestamp_s as i64, 0).map(|dt| dt.date_naive())
}

/// Catalog retention window (whole days) derived from a tenant's SFST
/// retention policy. Ceiling division so a non-integer `max_age` in days
/// doesn't trim catalog coverage below SFST coverage. There is no
/// independent knob — this is the single source of truth.
pub fn catalog_retention_days(retention: &bridge::config::RetentionConfig) -> u32 {
    retention
        .max_age
        .as_secs()
        .div_ceil(86_400)
        .try_into()
        .unwrap_or(u32::MAX)
}

/// Lower a resolved per-tenant retention config onto sfst's plain
/// [`RetentionPolicy`](sfst::RetentionPolicy) — the boundary where the
/// config framework's types stop and the format crate's begin.
pub fn sfst_retention_policy(retention: &bridge::config::RetentionConfig) -> sfst::RetentionPolicy {
    sfst::RetentionPolicy {
        max_files: retention.max_files,
        max_total_size: file_registry::ByteSize(retention.max_total_size.as_u64()),
        max_age: retention.max_age,
    }
}

/// Build a [`otel_catalog::CatalogEntry`] from a registered SFST file.
///
/// All summary fields come from `sfst_file.summary`, which the registry
/// populated either at indexing time (`Registry::track`) or at recovery time
/// (`Registry::recover`). No reads against the SFST file itself.
pub fn build_catalog_entry(
    sfst_file: &sfst::File,
    remote_key: String,
    uploaded_at_ns: TimestampNs,
    remote_etag: Option<String>,
) -> otel_catalog::CatalogEntry {
    let summary = &sfst_file.summary;
    otel_catalog::CatalogEntry {
        id: sfst_file.id,
        remote_key,
        min_timestamp_s: summary.min_timestamp_s,
        max_timestamp_s: summary.max_timestamp_s,
        record_count: summary.record_count,
        content_meta: summary.content_meta.clone(),
        size: sfst_file.size,
        uploaded_at_ns,
        remote_etag,
    }
}

/// Build the SFST upload request for a tracked file, or `None` if the registry
/// no longer has it. Shared by the indexer-response path and recovery so the
/// date → remote-key derivation lives in one place.
///
/// `signal` is the owning pipeline's remote-key segment (`logs`, later
/// `traces`); the request's `pipeline_id` comes from the file's own `FileId`,
/// the single source of truth for which pipeline owns the file.
pub fn sfst_upload_request(
    registry: &Registry,
    signal: &str,
    tenant_id: &TenantId,
    id: FileId,
) -> Option<UploaderRequest> {
    let sfst_file = registry.sfst.get(id.seq)?;
    let date =
        date_from_summary(&sfst_file.summary).unwrap_or_else(|| chrono::Utc::now().date_naive());
    Some(UploaderRequest::Upload {
        pipeline_id: id.pipeline_id,
        seq: id.seq,
        local_path: registry.sfst.file_path(id),
        remote_key: crate::remote_keys::sfst(signal, tenant_id, date, id),
    })
}

#[cfg(test)]
mod tests {
    use super::*;

    /// Pin the field mapping: a swap (e.g. max_files <-> max_total_size)
    /// would compile and still pass the evict-all recovery tests, since
    /// `max_files: 0` evicts everything regardless of the other limits.
    #[test]
    fn retention_policy_maps_fields_one_to_one() {
        let cfg = bridge::config::RetentionConfig {
            max_files: 7,
            max_total_size: bytesize::ByteSize::gib(10),
            max_age: std::time::Duration::from_secs(86_400),
        };
        let policy = sfst_retention_policy(&cfg);
        assert_eq!(policy.max_files, 7);
        assert_eq!(
            policy.max_total_size,
            file_registry::ByteSize(10 * 1024 * 1024 * 1024)
        );
        assert_eq!(policy.max_age, std::time::Duration::from_secs(86_400));
    }
}
