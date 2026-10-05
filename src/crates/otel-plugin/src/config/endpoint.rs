//! Partial overrides for the OTLP listener endpoints: the addresses the
//! ingestor binds for OTLP exporters plus their TLS certificate material
//! (`otel-ingestor` serves them; not the ferryboat IPC sockets between
//! supervisor and workers). The gRPC listener is mandatory (`grpc_path` +
//! `grpc_tls_*` trio); the OTLP/HTTP listener is optional (`http_path` +
//! `http_tls_*` trio, clearable to disable).
//!
//! Two of the layers `ConfigResolver` merges with stock < user < env
//! precedence surface this struct: the user YAML's `endpoint:` section
//! (parsed here) and the `NETDATA_OTEL_CFG_ENDPOINT_*` env vars (built in
//! `env.rs`). [`apply`] merges one layer into the effective
//! [`EndpointConfig`]; the merged result is validated in `mod.rs`
//! (`validate`: host:port shape, TLS cert/key pairing, distinct listener
//! addresses).
use anyhow::Result;
use bridge::config::EndpointConfig;
use serde::Deserialize;

/// The gRPC listener keys before they took the `grpc_` prefix (symmetric
/// with `http_path`/`http_tls_*`), each paired with its current name. Existing user files and
/// env vars keep working: an old name is accepted with a deprecation warning,
/// and setting both names in one layer is an error, since neither can be
/// silently preferred. `env.rs` derives the env var names from this table.
pub(super) const RENAMED_KEYS: [(&str, &str); 4] = [
    ("path", "grpc_path"),
    ("tls_cert_path", "grpc_tls_cert_path"),
    ("tls_key_path", "grpc_tls_key_path"),
    ("tls_ca_cert_path", "grpc_tls_ca_cert_path"),
];

/// Serde for the double-`Option` clear semantics of `http_path`. Without
/// this, serde's `Option` handling collapses YAML `null` into the OUTER
/// `None` ("no override"), making an explicit disable indistinguishable
/// from an absent key. Deserializing the inner `Option<String>` directly
/// and wrapping it in `Some` preserves all three states: absent key →
/// `None` (via `serde(default)`), `null` → `Some(None)` (clear/disable),
/// string → `Some(Some(v))` (set/replace).
mod double_option {
    use serde::{Deserialize, Deserializer};

    pub fn deserialize<'de, T, D>(de: D) -> Result<Option<T>, D::Error>
    where
        T: Deserialize<'de>,
        D: Deserializer<'de>,
    {
        T::deserialize(de).map(Some)
    }
}

/// Partial form of [`EndpointConfig`]: every field optional, so a config
/// layer overrides only the fields it names. Unknown keys in the YAML
/// section are a parse error (`deny_unknown_fields`), not silently ignored.
#[derive(Debug, Default, Deserialize)]
#[serde(deny_unknown_fields)]
pub(super) struct EndpointOverride {
    /// Bind address (`host:port`), not a filesystem path.
    #[serde(default)]
    pub(super) grpc_path: Option<String>,
    #[serde(default)]
    pub(super) grpc_tls_cert_path: Option<String>,
    #[serde(default)]
    pub(super) grpc_tls_key_path: Option<String>,
    #[serde(default)]
    pub(super) grpc_tls_ca_cert_path: Option<String>,
    /// Deprecated names of `grpc_path` and the `grpc_tls_*` trio
    /// ([`RENAMED_KEYS`]), read only from YAML;
    /// [`EndpointOverride::migrate_renamed_keys`] moves them onto the current
    /// fields before the layer is applied.
    #[serde(default)]
    pub(super) path: Option<String>,
    #[serde(default)]
    pub(super) tls_cert_path: Option<String>,
    #[serde(default)]
    pub(super) tls_key_path: Option<String>,
    #[serde(default)]
    pub(super) tls_ca_cert_path: Option<String>,
    /// OTLP/HTTP listener address, double-`Option` so an override layer can
    /// express all three states serde sees: absent key → `None` (no
    /// override, the stock value stands), YAML `null` → `Some(None)`
    /// (explicitly disable the HTTP listener), string → `Some(Some(v))`
    /// (set/replace). The listener is optional, so unlike the mandatory
    /// gRPC `grpc_path` it must be clearable — a plain `Option<String>` could not
    /// tell "unset" apart from "set to null" (see `double_option`).
    #[serde(default, deserialize_with = "double_option::deserialize")]
    pub(super) http_path: Option<Option<String>>,
    #[serde(default)]
    pub(super) http_tls_cert_path: Option<String>,
    #[serde(default)]
    pub(super) http_tls_key_path: Option<String>,
    #[serde(default)]
    pub(super) http_tls_ca_cert_path: Option<String>,
}

impl EndpointOverride {
    /// True when the override sets any field. `env.rs` uses this to collapse
    /// an all-empty env override into `None`.
    pub(super) fn has_any(&self) -> bool {
        self.grpc_path.is_some()
            || self.grpc_tls_cert_path.is_some()
            || self.grpc_tls_key_path.is_some()
            || self.grpc_tls_ca_cert_path.is_some()
            || self.http_path.is_some()
            || self.http_tls_cert_path.is_some()
            || self.http_tls_key_path.is_some()
            || self.http_tls_ca_cert_path.is_some()
    }

    /// Move each deprecated key's value onto its `grpc_` name, logging a
    /// warning that names `source` (the user file). Both names set is an
    /// error. A deprecated key set to `null` carries no value and is ignored,
    /// so a copy of the former stock file needs no edit.
    pub(super) fn migrate_renamed_keys(&mut self, source: &str) -> Result<()> {
        let pairs = [
            (&mut self.path, &mut self.grpc_path),
            (&mut self.tls_cert_path, &mut self.grpc_tls_cert_path),
            (&mut self.tls_key_path, &mut self.grpc_tls_key_path),
            (&mut self.tls_ca_cert_path, &mut self.grpc_tls_ca_cert_path),
        ];
        for ((old_slot, new_slot), (old, new)) in pairs.into_iter().zip(RENAMED_KEYS) {
            let Some(value) = old_slot.take() else {
                continue;
            };
            if new_slot.is_some() {
                anyhow::bail!(
                    "endpoint.{old} and endpoint.{new} are both set; endpoint.{old} is the \
                     deprecated name of endpoint.{new}, remove it"
                );
            }
            tracing::warn!("{source}: endpoint.{old} is deprecated, use endpoint.{new}");
            *new_slot = Some(value);
        }
        Ok(())
    }
}

/// Merge `o` into `config` field-wise: a `Some` field replaces the current
/// value, `None` keeps whatever the stock file or an earlier layer supplied.
/// `apply_overrides` in `mod.rs` calls this once per layer, user first and
/// env last, so env vars win. A TLS path can be set or replaced but never
/// cleared — the target field is only ever assigned `Some` — while
/// `http_path` carries its own `Some(None)` clear state (the HTTP listener
/// is optional, so an override may deliberately turn it off).
pub(super) fn apply(config: &mut EndpointConfig, o: &EndpointOverride) {
    if let Some(v) = &o.grpc_path {
        config.grpc_path = v.clone();
    }
    if let Some(v) = &o.grpc_tls_cert_path {
        config.grpc_tls_cert_path = Some(v.clone());
    }
    if let Some(v) = &o.grpc_tls_key_path {
        config.grpc_tls_key_path = Some(v.clone());
    }
    if let Some(v) = &o.grpc_tls_ca_cert_path {
        config.grpc_tls_ca_cert_path = Some(v.clone());
    }
    if let Some(v) = &o.http_path {
        config.http_path = v.clone();
    }
    if let Some(v) = &o.http_tls_cert_path {
        config.http_tls_cert_path = Some(v.clone());
    }
    if let Some(v) = &o.http_tls_key_path {
        config.http_tls_key_path = Some(v.clone());
    }
    if let Some(v) = &o.http_tls_ca_cert_path {
        config.http_tls_ca_cert_path = Some(v.clone());
    }
}
