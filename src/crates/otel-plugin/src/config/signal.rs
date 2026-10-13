//! Override types for the per-signal tuning sections (`logs`, `traces`) and the
//! global `remote_storage`/`auth` sections of otel.yaml — the shapes both
//! override layers fill: the user YAML (parsed in `mod.rs`) and the
//! `NETDATA_OTEL_CFG_*` environment (built in `env.rs`). The target types come
//! from `bridge::config`, the shared `netdata-plugin/bridge` crate.
//!
//! Per-signal sections carry tuning only — no dirs (derived from `base_dir`) and
//! no storage (global) — and there is no per-signal on/off. The same
//! [`SignalOverride`] shape applies to every signal; [`apply_signal`] merges it
//! onto a [`SignalConfig`]. Storage and auth are global, merged by
//! [`apply_remote_storage`] / [`apply_auth`]. Every field is an `Option`, so a
//! layer tunes only what it names, and `deny_unknown_fields` makes an
//! unrecognized key a parse error rather than a silent default.

use std::collections::HashMap;
use std::path::PathBuf;
use std::time::Duration;

use bridge::config::{AuthConfig, RemoteStorageConfig, RetentionEntry, SignalConfig};
use bytesize::ByteSize;
use serde::Deserialize;

/// Partial form of [`SignalConfig`]. `rotation`/`retention` hold the raw
/// per-tenant entry maps — not the validated
/// [`bridge::config::RotationPolicy`] / [`bridge::config::RetentionPolicy`] —
/// because merging patches an already-valid policy through their
/// `apply_overrides`: only `Some` fields replace, so the policy's `default`
/// stays complete.
#[derive(Debug, Default, Deserialize)]
#[serde(deny_unknown_fields)]
pub(super) struct SignalOverride {
    #[serde(default)]
    pub(super) crc_enabled: Option<bool>,
    #[serde(default)]
    pub(super) compression_enabled: Option<bool>,
    #[serde(default)]
    pub(super) rotation: Option<HashMap<String, bridge::config::RotationEntry>>,
    #[serde(default)]
    pub(super) retention: Option<HashMap<String, RetentionEntry>>,
    #[serde(default)]
    pub(super) catalog: Option<CatalogOverride>,
    #[serde(default)]
    pub(super) ingest: Option<IngestOverride>,
    /// Where the former plugin's journal files live, for the read-only
    /// legacy-logs viewer. Declared so strict parsing accepts it; valid under
    /// `logs:` only — `traces.journal_dir` is rejected at resolve by
    /// `ConfigOverride::validate` (mod.rs). Never merged: the value is consumed
    /// by `resolve_legacy_journal_dir` re-reading the raw file, and no env var
    /// exists for it (`env.rs` leaves it `None`).
    #[serde(default)]
    pub(super) journal_dir: Option<PathBuf>,
}

/// Partial [`bridge::config::CatalogTuning`] (its dir is derived, not configured).
#[derive(Debug, Default, Deserialize)]
#[serde(deny_unknown_fields)]
pub(super) struct CatalogOverride {
    #[serde(default)]
    pub(super) rotation_count: Option<usize>,
    #[serde(default, deserialize_with = "opt_humantime")]
    pub(super) rotation_period: Option<Duration>,
}

/// Partial [`bridge::config::IngestConfig`]: per-record ingestion time-bounds,
/// global for the signal (not per-tenant).
#[derive(Debug, Default, Deserialize)]
#[serde(deny_unknown_fields)]
pub(super) struct IngestOverride {
    #[serde(default, deserialize_with = "opt_humantime")]
    pub(super) max_age: Option<Duration>,
    #[serde(default, deserialize_with = "opt_humantime")]
    pub(super) future_skew: Option<Duration>,
}

/// Deserialize an optional humantime `Duration` (e.g. `"24h"`, `"10m"`) from
/// the YAML override file. The override types are never serialized — only the
/// merged `PluginConfig` crosses the bincode IPC, via `bridge::config`'s
/// duration module — so a plain humantime parse is correct here.
fn opt_humantime<'de, D: serde::Deserializer<'de>>(d: D) -> Result<Option<Duration>, D::Error> {
    match Option::<String>::deserialize(d)? {
        Some(s) => humantime::parse_duration(&s)
            .map(Some)
            .map_err(serde::de::Error::custom),
        None => Ok(None),
    }
}

/// Partial form of the process-global [`RemoteStorageConfig`] — one backend and
/// one download cache for the whole process. `mod.rs` validation requires a
/// non-empty `uri` when `enabled` is true.
#[derive(Debug, Default, Deserialize)]
#[serde(deny_unknown_fields)]
pub(super) struct RemoteStorageOverride {
    #[serde(default)]
    pub(super) enabled: Option<bool>,
    #[serde(default)]
    pub(super) uri: Option<String>,
    #[serde(default)]
    pub(super) read_cache_max_size: Option<ByteSize>,
    #[serde(default, deserialize_with = "opt_humantime")]
    pub(super) startup_op_timeout: Option<Duration>,
}

/// Partial form of the process-global [`AuthConfig`] (tenant authentication).
#[derive(Debug, Default, Deserialize)]
#[serde(deny_unknown_fields)]
pub(super) struct AuthOverride {
    #[serde(default)]
    pub(super) enabled: Option<bool>,
}

impl AuthOverride {
    /// True when `enabled` was set (see [`SignalOverride::has_any`]).
    pub(super) fn has_any(&self) -> bool {
        self.enabled.is_some()
    }
}

impl SignalOverride {
    /// True when the override sets anything, `journal_dir` included. `env.rs`
    /// collapses a section this returns `false` for back to `None`, so every
    /// field must be listed — a missed one silently drops a parsed override.
    pub(super) fn has_any(&self) -> bool {
        self.crc_enabled.is_some()
            || self.compression_enabled.is_some()
            || self.rotation.is_some()
            || self.retention.is_some()
            || self.catalog.as_ref().is_some_and(|c| c.has_any())
            || self.ingest.as_ref().is_some_and(|i| i.has_any())
            || self.journal_dir.is_some()
    }
}

impl RemoteStorageOverride {
    /// True when any field is set (see [`SignalOverride::has_any`]). Every
    /// field must be listed here — the `env_override_global_remote_storage`
    /// test in `mod.rs` pins that a missed one cannot silently drop a parsed
    /// override.
    pub(super) fn has_any(&self) -> bool {
        self.enabled.is_some()
            || self.uri.is_some()
            || self.read_cache_max_size.is_some()
            || self.startup_op_timeout.is_some()
    }
}

impl CatalogOverride {
    /// True when any field is set — feeds [`SignalOverride::has_any`].
    pub(super) fn has_any(&self) -> bool {
        self.rotation_count.is_some() || self.rotation_period.is_some()
    }
}

impl IngestOverride {
    /// True when any field is set — feeds [`SignalOverride::has_any`].
    pub(super) fn has_any(&self) -> bool {
        self.max_age.is_some() || self.future_skew.is_some()
    }
}

/// Merge `o` into `config` field-wise: a `Some` replaces the current value,
/// `None` keeps whatever stock or an earlier layer supplied. `apply_overrides`
/// (`mod.rs`) calls this once per layer for each signal — user first, env last,
/// so env wins. `journal_dir` is never merged (see its field doc).
pub(super) fn apply_signal(config: &mut SignalConfig, o: &SignalOverride) {
    if let Some(v) = o.crc_enabled {
        config.crc_enabled = v;
    }
    if let Some(v) = o.compression_enabled {
        config.compression_enabled = v;
    }
    if let Some(r) = &o.rotation {
        config.rotation.apply_overrides(r);
    }
    if let Some(r) = &o.retention {
        config.retention.apply_overrides(r);
    }
    if let Some(c) = &o.catalog {
        if let Some(v) = c.rotation_count {
            config.catalog.rotation_count = v;
        }
        if let Some(v) = c.rotation_period {
            config.catalog.rotation_period = v;
        }
    }
    if let Some(i) = &o.ingest {
        if let Some(v) = i.max_age {
            config.ingest.max_age = v;
        }
        if let Some(v) = i.future_skew {
            config.ingest.future_skew = v;
        }
    }
}

/// Merge the global remote-storage override onto [`RemoteStorageConfig`], with
/// the same field-wise semantics as [`apply_signal`] (user first, env last).
pub(super) fn apply_remote_storage(config: &mut RemoteStorageConfig, o: &RemoteStorageOverride) {
    if let Some(v) = o.enabled {
        config.enabled = v;
    }
    if let Some(v) = &o.uri {
        config.uri = v.clone();
    }
    if let Some(v) = o.read_cache_max_size {
        config.read_cache_max_size = v;
    }
    if let Some(v) = o.startup_op_timeout {
        config.startup_op_timeout = v;
    }
}

/// Merge the global auth override onto [`AuthConfig`], with the same field-wise
/// semantics as [`apply_signal`] (user first, env last). A single on/off: when
/// off, all data routes to the `default` tenant.
pub(super) fn apply_auth(config: &mut AuthConfig, o: &AuthOverride) {
    if let Some(v) = o.enabled {
        config.enabled = v;
    }
}
