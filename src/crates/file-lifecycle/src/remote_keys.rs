//! Construction and parsing of remote object-storage keys.
//!
//! ## Bucket layout (versioned + signal-scoped at the root)
//!
//! ```text
//! v2/{signal}/catalog/{YYYY-MM-DD}/{tenant_id}/{machine}-{boot}-{max_seq}-{min_ts}-{max_ts}.catalog
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
use file_registry::{FileId, TenantId};
use uuid::Uuid;

/// Schema version prefix. `v2` matches the plugin's namespace (the former
/// plugin's artifacts were the `v1` generation); bumping this enables
/// side-by-side migrations (write `v3/...` while readers still handle
/// `v2/...`).
const SCHEMA_VERSION: &str = "v2";

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
    machine_id: Uuid,
    boot_id: Uuid,
    max_seq: u64,
    min_timestamp_s: u32,
    max_timestamp_s: u32,
) -> String {
    format!(
        "{SCHEMA_VERSION}/{signal}/catalog/{}/{}/{}",
        date.format("%Y-%m-%d"),
        tenant_id,
        otel_catalog::filename(
            machine_id,
            boot_id,
            max_seq,
            min_timestamp_s,
            max_timestamp_s,
        ),
    )
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
