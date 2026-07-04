//! Construction and parsing of remote object-storage keys.
//!
//! ## Bucket layout (versioned + signal-scoped at the root)
//!
//! ```text
//! v2/{signal}/catalog/{YYYY-MM-DD}/{tenant_id}/{machine}-{instance}-{max_seq}-{min_ts}-{max_ts}.catalog
//! v2/{signal}/tenants/{tenant_id}/sfst/{YYYY-MM-DD}/{file_id}.sfst
//! ```
//!
//! The `{signal}` segment (e.g. `logs`, `traces`) is the top-level
//! discriminator under the schema version: every signal carries its own
//! segment — none is implicit. A console browse / LIST `v2/` shows the
//! signals; per-signal lifecycle and IAM rules attach to a single
//! `v2/{signal}/` prefix. The substrate ascribes the segment no meaning
//! beyond the path; each pipeline supplies its own signal name. The
//! `sfst` segment is the artifact *type* (the SFST container), shared by
//! every signal that seals into it — the `{signal}` segment, not the
//! extension, distinguishes one signal's SFSTs from another's.
//!
//! Within a signal the layout stays artifact-first:
//!
//! - **`.../catalog/{date}/{tenant}/...`** — date-first under the catalog
//!   umbrella. Catalogs are LIST-enumerated per-(date, tenant) for query
//!   discovery, and bucket-level lifecycle rules attach naturally to a
//!   single date prefix. The tenant segment is redundant with the body's
//!   `tenant_id` field but scopes per-tenant LISTs and IAM policies.
//!
//! - **`.../tenants/{tenant}/sfst/{date}/...`** — tenant-first under the
//!   tenants umbrella. SFSTs are fetched by known key (drawn from a
//!   catalog entry's `remote_key`), never LIST-enumerated by date, so the
//!   prefix shape doesn't affect query discovery — only IAM policies
//!   (per-tenant scope) and lifecycle rules (date-bucketed under the
//!   tenant).
//!
//! - **WAL is absent.** WAL files are deleted post-index; they're
//!   ephemeral by design and never reach the remote.
//!
//! All layout decisions live in this module — constructors and the
//! inverse `parse_*` functions sit together so they stay in sync.

use chrono::NaiveDate;
use file_registry::{FileId, Identity, TenantId};

/// Schema version prefix. `v2` matches the plugin's namespace (the former
/// plugin's artifacts were the `v1` generation); bumping this enables
/// side-by-side migrations (write `v3/...` while readers still handle
/// `v2/...`).
const SCHEMA_VERSION: &str = "v2";

/// Object extensions, matching the filename builders (`otel_catalog::filename`
/// stamps `.catalog`; SFST keys use `FileId::to_filename("sfst")`).
const CATALOG_EXT: &str = "catalog";
const SFST_EXT: &str = "sfst";

/// Remote key for an uploaded SFST file, scoped to `signal`.
pub fn sfst(signal: &str, tenant_id: &TenantId, date: NaiveDate, id: FileId) -> String {
    format!(
        "{SCHEMA_VERSION}/{signal}/tenants/{}/sfst/{}/{}",
        tenant_id,
        date.format("%Y-%m-%d"),
        id.to_filename("sfst"),
    )
}

/// LIST prefix for every SFST uploaded for `signal`/`tenant_id` on `date`.
pub fn sfst_prefix(signal: &str, tenant_id: &TenantId, date: NaiveDate) -> String {
    format!(
        "{SCHEMA_VERSION}/{signal}/tenants/{}/sfst/{}/",
        tenant_id,
        date.format("%Y-%m-%d"),
    )
}

/// Remote key for a rotated catalog file, scoped to `signal`.
// The key is built from the signal, the date, the tenant, and the five
// catalog-file identity components — all distinct primitives that belong in the
// key, so grouping them into a one-off struct would add indirection, not clarity.
#[allow(clippy::too_many_arguments)]
pub fn catalog(
    signal: &str,
    date: NaiveDate,
    tenant_id: &TenantId,
    identity: Identity,
    max_seq: u64,
    min_timestamp_s: u32,
    max_timestamp_s: u32,
) -> String {
    format!(
        "{SCHEMA_VERSION}/{signal}/catalog/{}/{}/{}",
        date.format("%Y-%m-%d"),
        tenant_id,
        otel_catalog::filename(identity, max_seq, min_timestamp_s, max_timestamp_s),
    )
}

/// LIST prefix for every catalog uploaded for `signal` (all dates/tenants).
/// The startup diff-sync issues one recursive LIST against this prefix.
pub fn catalog_prefix(signal: &str) -> String {
    format!("{SCHEMA_VERSION}/{signal}/catalog/")
}

/// The identity + fold fields recovered from a catalog remote key.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct ParsedCatalogKey {
    pub date: NaiveDate,
    pub tenant_id: TenantId,
    pub identity: Identity,
    pub max_seq: u64,
    pub min_timestamp_s: u32,
    pub max_timestamp_s: u32,
}

/// Inverse of [`catalog`]: parse + sanitize a listed catalog key. Returns
/// `None` for any deviation from the exact shape
/// `v2/{expected_signal}/catalog/{YYYY-MM-DD}/{tenant}/{filename}.catalog` — the
/// caller warns and skips (garbage keys never reach the install path). The
/// `{signal}` segment must equal `expected_signal` (the prefix the caller
/// LISTed). The tenant segment goes through [`TenantId::validate_path_segment`]
/// whose charset excludes `/` (path-traversal dies) but admits the stored
/// `default` tenant.
pub fn parse_catalog_key(key: &str, expected_signal: &str) -> Option<ParsedCatalogKey> {
    let parts: Vec<&str> = key.split('/').collect();
    if parts.len() != 6
        || parts[0] != SCHEMA_VERSION
        || parts[1] != expected_signal
        || parts[2] != "catalog"
    {
        return None;
    }
    let date = NaiveDate::parse_from_str(parts[3], "%Y-%m-%d").ok()?;
    let tenant_id = TenantId::validate_path_segment(parts[4]).ok()?;
    // Require the catalog extension (`file_stem`/`parse_stem` would otherwise
    // strip ANY extension, so an `.sfst` key would parse as a catalog).
    let filename = std::path::Path::new(parts[5]);
    if filename.extension()?.to_str()? != CATALOG_EXT {
        return None;
    }
    let stem = filename.file_stem()?.to_str()?;
    let (identity, max_seq, min_timestamp_s, max_timestamp_s) = otel_catalog::parse_stem(stem)?;
    Some(ParsedCatalogKey {
        date,
        tenant_id,
        identity,
        max_seq,
        min_timestamp_s,
        max_timestamp_s,
    })
}

/// Parse an SFST remote key into its [`FileId`] and tenant. Used to validate a
/// catalog entry's `remote_key`: the caller checks the `FileId.machine_id`
/// belongs to this machine and the returned tenant matches the catalog's tenant.
/// The `{signal}` segment must equal `expected_signal` (mirroring
/// [`parse_catalog_key`]), so a tampered catalog body can't redirect a fetch
/// into another signal's object path. Returns `None` for any shape other than
/// `v2/{expected_signal}/tenants/{tenant}/sfst/{YYYY-MM-DD}/{file_id}.sfst`.
pub fn parse_sfst_key(key: &str, expected_signal: &str) -> Option<(FileId, TenantId)> {
    let parts: Vec<&str> = key.split('/').collect();
    if parts.len() != 7
        || parts[0] != SCHEMA_VERSION
        || parts[1] != expected_signal
        || parts[2] != "tenants"
        || parts[4] != "sfst"
    {
        return None;
    }
    let tenant_id = TenantId::validate_path_segment(parts[3]).ok()?;
    NaiveDate::parse_from_str(parts[5], "%Y-%m-%d").ok()?;
    // Require the SFST extension (`FileId::parse` would otherwise strip any).
    let filename = std::path::Path::new(parts[6]);
    if filename.extension()?.to_str()? != SFST_EXT {
        return None;
    }
    let id = FileId::parse(filename)?;
    Some((id, tenant_id))
}

/// Extract the date from an SFST remote key.
///
/// Expected shape: `v2/{signal}/tenants/{tenant_id}/sfst/{YYYY-MM-DD}/{file_id}.sfst`.
/// Returns `None` if the key doesn't match this shape. The `{signal}`
/// segment is skipped — callers already know the signal from the LIST
/// prefix they issued.
///
/// This is the tested inverse of [`sfst`], kept in sync with it per this
/// module's contract. The live recovery LIST path extracts the date via
/// `FileId::parse` on the trailing filename instead, so this helper currently
/// has no production caller; it remains the canonical, format-pinned inverse
/// (and its tests pin the layout, including rejection of the old segment-less
/// shape).
pub fn parse_sfst_date(key: &str) -> Option<NaiveDate> {
    let mut parts = key.split('/');
    if parts.next()? != SCHEMA_VERSION {
        return None;
    }
    let _signal = parts.next()?;
    if parts.next()? != "tenants" {
        return None;
    }
    let _tenant = parts.next()?;
    if parts.next()? != "sfst" {
        return None;
    }
    let date_str = parts.next()?;
    NaiveDate::parse_from_str(date_str, "%Y-%m-%d").ok()
}

#[cfg(test)]
mod tests;
