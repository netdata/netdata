//! Shared, pure mapping helpers for the file-lifecycle substrate: the
//! conversions between the config framework's types (`otel_bridge::config`) and
//! the format/catalog crates' plain structs, plus the single home of the
//! summary → date and registry → upload-request derivations. Every function
//! is a stateless computation over a resolved config or already-tracked
//! registry state — no I/O, no locks, no registry mutation.

use file_registry::{FileId, TenantId, TimestampNs};

use crate::ipc::UploaderRequest;
use crate::registry::Registry;

/// The calendar date an SFST's records belong to, from the summary's
/// `min_timestamp_s` — the date bucket the catalog layout and the SFST
/// remote key are partitioned by. A UTC date; the conversion applies no
/// offset.
///
/// Returns `None` only for an empty SFST (`record_count == 0`): the summary
/// carries no timestamp to derive a date from. The chrono conversion is
/// total over the field's `u32` range (chrono's representable span reaches
/// centuries past 1970 + `u32::MAX` seconds), so there is no second `None`
/// path.
///
/// Callers fall back to the current UTC date on `None` — written out at
/// each call site rather than hidden in this helper, so the fallback stays
/// visible.
pub fn date_from_summary(summary: &sfst::Summary) -> Option<chrono::NaiveDate> {
    if summary.record_count == 0 {
        return None;
    }
    chrono::DateTime::from_timestamp(summary.min_timestamp_s as i64, 0).map(|dt| dt.date_naive())
}

/// The catalog retention window, in whole days, from the tenant's
/// remote-archive horizon (`retention.horizon`, decoupled from SFST
/// `max_age`). Ceiling division so a horizon that is not a whole number of
/// days rounds up and never trims coverage; saturates at `u32::MAX` (the
/// `u32` the consumer signature takes).
///
/// The `horizon > max_age` day-unit invariant is enforced at config load
/// ([`otel_bridge::config::RetentionPolicy::validate`]): catalog eviction is
/// date-partition based (a file's earliest timestamp) while SFST eviction is
/// age based (its latest timestamp), so the catalog gating an SFST's
/// eviction always outlives that SFST.
pub fn catalog_retention_days(retention: &otel_bridge::config::RetentionConfig) -> u32 {
    retention
        .horizon
        .as_secs()
        .div_ceil(86_400)
        .try_into()
        .unwrap_or(u32::MAX)
}

/// Lower a resolved per-tenant retention config onto sfst's plain
/// [`RetentionPolicy`](sfst::RetentionPolicy): `max_files` and `max_age` map
/// 1:1; `max_total_size` is re-wrapped from `otel-bridge`'s `bytesize::ByteSize`
/// into [`file_registry::ByteSize`]. The
/// `retention_policy_maps_fields_one_to_one` test pins the mapping.
pub fn sfst_retention_policy(
    retention: &otel_bridge::config::RetentionConfig,
) -> sfst::RetentionPolicy {
    sfst::RetentionPolicy {
        max_files: retention.max_files,
        max_total_size: file_registry::ByteSize(retention.max_total_size.as_u64()),
        max_age: retention.max_age,
    }
}

/// Build a [`otel_catalog::CatalogEntry`] from a registered SFST file: the
/// file's id and size plus its summary fields, copied from registry state.
///
/// `sfst_file.summary` was populated when the registry learned the file, so
/// there are no reads against the SFST file itself. `uploaded_at_ns` and
/// `remote_etag` are the caller's upload metadata.
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

/// Build the SFST upload request for a tracked file, or `None` if the tenant
/// registry no longer tracks that seq. The summary → date → remote-key
/// derivation lives here so callers cannot drift.
///
/// `signal` is the owning signal's remote-key segment (`logs` / `traces`,
/// from `otel_bridge::signals::Signal::segment`); the request's `pipeline_id`
/// comes from the file's own [`FileId`], the single source of truth for
/// which pipeline owns the file.
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
        seq: file_registry::SeqKey::from(&id),
        local_path: registry.sfst.file_path(id),
        remote_key: crate::remote_keys::sfst(signal, tenant_id, date, id),
    })
}

#[cfg(test)]
mod tests {
    use super::*;

    /// Pin the config→policy field mapping 1:1. The evict-all recovery tests
    /// (`recovery/tests.rs`, the `evict_all_retention` fixture's
    /// `max_files: 0`) evict everything regardless of the other two limits,
    /// so they cannot catch a cross-wired field; a literal swap would not
    /// even compile, since the three fields have distinct types.
    #[test]
    fn retention_policy_maps_fields_one_to_one() {
        let cfg = otel_bridge::config::RetentionConfig {
            max_files: 7,
            max_total_size: bytesize::ByteSize::gib(10),
            max_age: std::time::Duration::from_secs(86_400),
            horizon: std::time::Duration::from_secs(2 * 365 * 24 * 3600),
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
