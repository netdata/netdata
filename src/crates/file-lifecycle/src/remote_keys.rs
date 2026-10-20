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
//! segment — none is implicit, and a prefix-scoped bucket rule (retention or
//! access) can target one signal without touching another. The substrate
//! ascribes the segment no meaning beyond the path: each pipeline passes its
//! own segment — sourced from `bridge::signals::Signal::segment` (the single
//! signal ↔ segment map) — as an opaque `&str`. The `sfst` segment is the
//! artifact *type* (the SFST container), shared by every signal that seals
//! into it — the `{signal}` segment, not the extension, distinguishes one
//! signal's SFSTs from another's.
//!
//! Within a signal the layout stays artifact-first:
//!
//! - **`.../catalog/{date}/{tenant}/...`** — date-first under the catalog
//!   umbrella. Enumerated once per signal by the startup diff-sync
//!   (`recovery::startup::startup_catalog_sync`): one recursive LIST on
//!   [`catalog_prefix`], every key gated by [`parse_catalog_key`], then the
//!   own-machine filter, remote-tenant discovery, the seq-highwater seed,
//!   and diff-download of the bodies missing locally. The query path never
//!   LISTs the remote for catalogs — it reads the locally installed files.
//!   The tenant segment is not redundant with the body's `tenant_id` field:
//!   `recovery::startup::validate_catalog` cross-checks the body against it,
//!   the sync discovers remote-only tenants from it, and recovery rebuilds
//!   the local catalog path from it.
//!
//! - **`.../tenants/{tenant}/sfst/{date}/...`** — tenant-first under the
//!   tenants umbrella. SFSTs are fetched by known key — a catalog entry's
//!   `remote_key` field, read back by `remote_read` — so query discovery
//!   never enumerates them. The only LIST is startup recovery's pre-mark
//!   pass (`recovery::remote::reconcile_remote_uploads`), one [`sfst_prefix`]
//!   LIST per day of its bounded reconcile window; the prefix carries tenant
//!   and date, and the caller filters to its own machine via the parsed
//!   `FileId` (D6).
//!
//! Date bucketing is what remote retention rests on: the `Storage` trait
//! has no delete (`write`/`list`/`read`/`stat` only), so aging objects out
//! is left to bucket-side lifecycle rules, which key on prefixes. WAL files
//! are absent: they are deleted locally after indexing, and the uploader
//! only ever receives SFST/catalog requests — a WAL never reaches the
//! remote.
//!
//! All layout decisions live in this module — constructors and the inverse
//! `parse_*` functions sit together so a shape change must touch both. The
//! full-shape matchers are [`parse_catalog_key`] and [`parse_sfst_key`]:
//! exact segment count, the literal version/umbrella/signal segments, and
//! the expected extension (stem parsing strips any extension, so the explicit
//! check is what keeps an `.sfst` key from parsing as a catalog and vice
//! versa). Any deviation returns `None`
//! and the caller warns and skips — garbage keys never reach an install or
//! a fetch. The filename vocabulary is not defined here: catalog filenames
//! come from `otel_catalog::filename`, SFST filenames from
//! `FileId::to_filename` (the file-registry `FileId` contract).

use chrono::NaiveDate;
use file_registry::{FileId, Identity, TenantId};

/// Schema version prefix. The `v1` generation — the former plugin's
/// artifacts, without the signal segment — is rejected by the version check
/// in every parser (pinned in this module's tests). Builders and parsers
/// share this one const, so bumping it moves both in one build:
/// old-generation keys stop parsing, and a real migration needs code on each
/// side, not just the bump.
const SCHEMA_VERSION: &str = "v2";

/// Object extensions, matching the filename builders (`otel_catalog::filename`
/// stamps `.catalog`; SFST keys use `FileId::to_filename("sfst")`).
const CATALOG_EXT: &str = "catalog";
/// The SFST extension. Also what `remote_read` stamps on the download-cache
/// filenames it derives from a `FileId`, so a cached file's name equals the
/// remote key's filename half.
pub(crate) const SFST_EXT: &str = "sfst";

/// Remote key for an uploaded SFST file, scoped to `signal`.
///
/// Sole producer: `helpers::sfst_upload_request`, shared by the indexer's
/// upload path and recovery. The uploader writes the object at this key, and
/// its success handler stamps the key into the catalog entry's `remote_key`
/// — the field the query path later fetches by (`remote_read`).
pub fn sfst(signal: &str, tenant_id: &TenantId, date: NaiveDate, id: FileId) -> String {
    format!(
        "{SCHEMA_VERSION}/{signal}/tenants/{}/sfst/{}/{}",
        tenant_id,
        date.format("%Y-%m-%d"),
        id.to_filename("sfst"),
    )
}

/// LIST prefix for every SFST uploaded for `signal`/`tenant_id` on `date`.
/// Sole caller: recovery's upload reconcile
/// (`recovery::remote::reconcile_remote_uploads`), which issues one LIST per
/// day of its bounded window to pre-mark recent uploads and spare them a
/// redundant re-upload. The query path never enumerates SFSTs.
pub fn sfst_prefix(signal: &str, tenant_id: &TenantId, date: NaiveDate) -> String {
    format!(
        "{SCHEMA_VERSION}/{signal}/tenants/{}/sfst/{}/",
        tenant_id,
        date.format("%Y-%m-%d"),
    )
}

/// Remote key for a rotated catalog file, scoped to `signal`. The filename
/// half is `otel_catalog::filename` — the same name the local rotation
/// wrote — so a re-uploaded or re-fetched catalog lands under the same local
/// tree. Built at every upload site: the ledger's catalog-builder response
/// handler (steady state), recovery's re-upload pass
/// (`recovery::remote::reconcile_local_catalog_uploads`), and the
/// corrupt-catalog re-fetch (`recovery::startup::heal_corrupt_catalog`).
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
/// The startup diff-sync issues one recursive LIST against this prefix
/// (`recovery::startup::startup_catalog_sync`); the result is the signal's
/// whole catalog cardinality, other machines' keys included — the
/// own-machine filter runs per parsed key, after materialization (D6).
pub fn catalog_prefix(signal: &str) -> String {
    format!("{SCHEMA_VERSION}/{signal}/catalog/")
}

/// The date, tenant, identity, and fold fields recovered from a catalog
/// remote key. [`parse_catalog_key`] builds it from a LISTed key; recovery
/// also rebuilds it from the registry's filename-derived fields
/// (`recovery::local::seed_from_catalog_files`), so the corrupt-catalog heal
/// never trusts a key drawn from a possibly-corrupt body. Recovery turns it
/// back into the local catalog path (`recovery::startup::local_catalog_path`)
/// and validates downloaded bodies against it
/// (`recovery::startup::validate_catalog`).
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
/// `v2/{expected_signal}/catalog/{YYYY-MM-DD}/{tenant}/{filename}.catalog` —
/// the sole caller, the startup diff-sync
/// (`recovery::startup::startup_catalog_sync`), warns and skips (garbage
/// keys never reach the install path). The `{signal}` segment must equal
/// `expected_signal` (the prefix the caller LISTed). The tenant segment goes
/// through `TenantId::validate_path_segment`, whose charset excludes `/`
/// (path traversal dies), which also rejects `.`/`..`, and which admits the
/// stored `default` tenant (the auth-off tenant a node itself wrote is a
/// legitimate object to parse).
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

/// Parse an SFST remote key into its [`FileId`], tenant, and date. Sole
/// caller: `recovery::startup::validate_catalog`, which checks the
/// `FileId.machine_id` belongs to this machine, the returned tenant matches
/// the catalog's tenant, and the returned date matches the catalog's date.
/// The `{signal}` segment must equal `expected_signal` (mirroring
/// [`parse_catalog_key`]), so a tampered catalog body can't redirect a fetch
/// into another signal's object path. Returns `None` for any shape other
/// than
/// `v2/{expected_signal}/tenants/{tenant}/sfst/{YYYY-MM-DD}/{file_id}.sfst`.
pub fn parse_sfst_key(key: &str, expected_signal: &str) -> Option<(FileId, TenantId, NaiveDate)> {
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
    let date = NaiveDate::parse_from_str(parts[5], "%Y-%m-%d").ok()?;
    // Require the SFST extension (`FileId::parse` would otherwise strip any).
    let filename = std::path::Path::new(parts[6]);
    if filename.extension()?.to_str()? != SFST_EXT {
        return None;
    }
    let id = FileId::parse(filename)?;
    Some((id, tenant_id, date))
}

/// Extract the date from an SFST remote key.
///
/// Expected shape:
/// `v2/{signal}/tenants/{tenant_id}/sfst/{YYYY-MM-DD}/{file_id}.sfst`.
/// Returns `None` if the segments up to the date don't match this shape;
/// anything after the date is ignored, and the `{signal}`/`{tenant_id}`
/// segments are accepted as-is — a caller listing a signal/tenant/date-scoped
/// prefix already knows them.
///
/// No production caller: the recovery LIST knows each date from the prefix
/// it issued (one LIST per day) and parses the trailing filename with
/// `FileId::parse` for the file identity only. This remains the
/// format-pinned inverse of `sfst` (same module, kept in sync by
/// construction), and its tests pin the layout, including rejection of the
/// old segment-less v1 shape.
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
