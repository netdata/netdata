//! Partial overrides for the OTLP gRPC endpoint config: the address the
//! ingestor binds for OTLP exporters plus its TLS certificate material
//! (`otel-ingestor` serves it; not the ferryboat IPC sockets between
//! supervisor and workers).
//!
//! Two of the layers `ConfigResolver` merges with stock < user < env
//! precedence surface this struct: the user YAML's `endpoint:` section
//! (parsed here) and the `NETDATA_OTEL_CFG_ENDPOINT_*` env vars (built in
//! `env.rs`). [`apply`] merges one layer into the effective
//! [`EndpointConfig`]; the merged result is validated in `mod.rs`
//! (`validate`: host:port shape, TLS cert/key pairing).
use bridge::config::EndpointConfig;
use serde::Deserialize;

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
}

impl EndpointOverride {
    /// True when the override sets any field. `env.rs` uses this to collapse
    /// an all-empty env override into `None`.
    pub(super) fn has_any(&self) -> bool {
        self.path.is_some()
            || self.tls_cert_path.is_some()
            || self.tls_key_path.is_some()
            || self.tls_ca_cert_path.is_some()
    }
}

/// Merge `o` into `config` field-wise: a `Some` field replaces the current
/// value, `None` keeps whatever the stock file or an earlier layer supplied.
/// `apply_overrides` in `mod.rs` calls this once per layer, user first and
/// env last, so env vars win. A TLS path can be set or replaced but never
/// cleared — the target field is only ever assigned `Some`.
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
}
