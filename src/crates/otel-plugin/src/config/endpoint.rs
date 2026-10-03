//! Partial overrides for the OTLP listener endpoints: the addresses the
//! ingestor binds for OTLP exporters plus their TLS certificate material
//! (`otel-ingestor` serves them; not the ferryboat IPC sockets between
//! supervisor and workers). The gRPC listener is mandatory (`path` + TLS
//! trio); the OTLP/HTTP listener is optional (`http_path` + its own trio,
//! clearable to disable).
//!
//! Two of the layers `ConfigResolver` merges with stock < user < env
//! precedence surface this struct: the user YAML's `endpoint:` section
//! (parsed here) and the `NETDATA_OTEL_CFG_ENDPOINT_*` env vars (built in
//! `env.rs`). [`apply`] merges one layer into the effective
//! [`EndpointConfig`]; the merged result is validated in `mod.rs`
//! (`validate`: host:port shape, TLS cert/key pairing, distinct listener
//! addresses).
use bridge::config::EndpointConfig;
use serde::Deserialize;

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
    /// gRPC `path` it must be clearable — a plain `Option<String>` could not
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
        self.path.is_some()
            || self.tls_cert_path.is_some()
            || self.tls_key_path.is_some()
            || self.tls_ca_cert_path.is_some()
            || self.http_path.is_some()
            || self.http_tls_cert_path.is_some()
            || self.http_tls_key_path.is_some()
            || self.http_tls_ca_cert_path.is_some()
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
    if let Some(v) = &o.path {
        config.path = v.clone();
    }
    if let Some(v) = &o.tls_cert_path {
        config.tls_cert_path = Some(v.clone());
    }
    if let Some(v) = &o.tls_key_path {
        config.tls_key_path = Some(v.clone());
    }
    if let Some(v) = &o.tls_ca_cert_path {
        config.tls_ca_cert_path = Some(v.clone());
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
