//! Partial overrides for the OTLP listeners (`receivers.otlp.protocols.grpc`
//! and `.http`): enable flag, bind address and server TLS material for each
//! (`otel-ingestor` serves them; not the ferryboat IPC sockets between
//! supervisor and workers).
//!
//! Two of the layers `ConfigResolver` merges with stock < user < env
//! precedence surface these types: the user YAML's `receivers:` section
//! (parsed here) and the `NETDATA_OTEL_CFG_RECEIVERS_*` env vars (built in
//! `env.rs`). [`apply`] merges one layer into the effective
//! [`ReceiversConfig`]; the merged result is validated in `mod.rs`
//! (`validate`: at least one listener enabled, host:port shape, TLS
//! cert/key pairing, distinct listener addresses).
//!
//! The user file may still carry the gRPC-only `endpoint:` section of the
//! first release ([`DeprecatedEndpointOverride`]); its keys fold onto the
//! gRPC listener with a deprecation warning.
use bridge::config::{ProtocolConfig, ReceiversConfig};
use serde::Deserialize;

/// The first release's `endpoint.*` keys, each paired with the
/// `receivers.*` key that replaced it. An old key still works with a
/// deprecation warning; when a layer sets both names, the new one wins, also
/// with a warning. `env.rs` derives the old and new env var names from this
/// table.
pub(super) const DEPRECATED_ENDPOINT_KEYS: [(&str, &str); 4] = [
    ("endpoint.path", "receivers.otlp.protocols.grpc.endpoint"),
    (
        "endpoint.tls_cert_path",
        "receivers.otlp.protocols.grpc.tls.cert_file",
    ),
    (
        "endpoint.tls_key_path",
        "receivers.otlp.protocols.grpc.tls.key_file",
    ),
    (
        "endpoint.tls_ca_cert_path",
        "receivers.otlp.protocols.grpc.tls.client_ca_file",
    ),
];

/// Resolve one deprecated key against its replacement within a single layer.
/// `source` prefixes the warnings (the user file path, or "environment").
/// The new name wins when both are set; either way the operator is told
/// which name to keep.
pub(super) fn resolve_deprecated<T>(
    source: &str,
    old_name: &str,
    old: Option<T>,
    new_name: &str,
    new: Option<T>,
) -> Option<T> {
    match (old, new) {
        (Some(_), Some(new)) => {
            tracing::warn!(
                "{source}: {old_name} and {new_name} are both set; using {new_name} \
                 ({old_name} is its deprecated name, remove it)"
            );
            Some(new)
        }
        (Some(old), None) => {
            tracing::warn!("{source}: {old_name} is deprecated, use {new_name}");
            Some(old)
        }
        (None, new) => new,
    }
}

/// The user file's deprecated `endpoint:` section: exactly the four keys of
/// the first release. Unknown keys are a parse error, so names that were never
/// released (`grpc_path`, `http_path`, ...) are rejected too. A key set to
/// `null` carries no value and is ignored, so a copy of the former stock file
/// needs no edit.
#[derive(Debug, Default, Deserialize)]
#[serde(deny_unknown_fields)]
pub(super) struct DeprecatedEndpointOverride {
    #[serde(default)]
    path: Option<String>,
    #[serde(default)]
    tls_cert_path: Option<String>,
    #[serde(default)]
    tls_key_path: Option<String>,
    #[serde(default)]
    tls_ca_cert_path: Option<String>,
}

/// Partial form of [`ReceiversConfig`]: every level optional, so a config
/// layer overrides only the fields it names. `null` at any level (and so a
/// bare `http:` key, which YAML reads as `null`) means "no override", like an
/// absent key. Unknown keys are a parse error (`deny_unknown_fields`).
#[derive(Debug, Default, Deserialize)]
#[serde(deny_unknown_fields)]
pub(super) struct ReceiversOverride {
    #[serde(default)]
    otlp: Option<OtlpOverride>,
}

#[derive(Debug, Default, Deserialize)]
#[serde(deny_unknown_fields)]
struct OtlpOverride {
    #[serde(default)]
    protocols: Option<ProtocolsOverride>,
}

#[derive(Debug, Default, Deserialize)]
#[serde(deny_unknown_fields)]
struct ProtocolsOverride {
    #[serde(default)]
    grpc: Option<ProtocolOverride>,
    #[serde(default)]
    http: Option<ProtocolOverride>,
}

/// Partial form of one listener's [`ProtocolConfig`].
#[derive(Debug, Default, Deserialize)]
#[serde(deny_unknown_fields)]
pub(super) struct ProtocolOverride {
    #[serde(default)]
    pub(super) enabled: Option<bool>,
    #[serde(default)]
    pub(super) endpoint: Option<String>,
    #[serde(default)]
    pub(super) tls: Option<TlsOverride>,
}

#[derive(Debug, Default, Deserialize)]
#[serde(deny_unknown_fields)]
pub(super) struct TlsOverride {
    #[serde(default)]
    pub(super) cert_file: Option<String>,
    #[serde(default)]
    pub(super) key_file: Option<String>,
    #[serde(default)]
    pub(super) client_ca_file: Option<String>,
}

impl TlsOverride {
    fn is_empty(&self) -> bool {
        self.cert_file.is_none() && self.key_file.is_none() && self.client_ca_file.is_none()
    }
}

impl ProtocolOverride {
    fn is_empty(&self) -> bool {
        self.enabled.is_none()
            && self.endpoint.is_none()
            && self.tls.as_ref().is_none_or(TlsOverride::is_empty)
    }
}

impl ReceiversOverride {
    /// Build an override from per-listener parts, or `None` when neither
    /// sets anything. `env.rs` uses this so an env snapshot without receiver
    /// variables leaves the section untouched.
    pub(super) fn from_protocols(grpc: ProtocolOverride, http: ProtocolOverride) -> Option<Self> {
        if grpc.is_empty() && http.is_empty() {
            return None;
        }
        Some(Self {
            otlp: Some(OtlpOverride {
                protocols: Some(ProtocolsOverride {
                    grpc: Some(grpc),
                    http: Some(http),
                }),
            }),
        })
    }

    fn grpc_mut(&mut self) -> &mut ProtocolOverride {
        self.otlp
            .get_or_insert_with(OtlpOverride::default)
            .protocols
            .get_or_insert_with(ProtocolsOverride::default)
            .grpc
            .get_or_insert_with(ProtocolOverride::default)
    }
}

/// Fold the user file's deprecated `endpoint:` section onto the gRPC
/// listener of `receivers` (created when absent), warning per key with
/// `source` (the user file path). See [`resolve_deprecated`] for the
/// both-names rule.
pub(super) fn fold_deprecated_endpoint(
    receivers: &mut Option<ReceiversOverride>,
    endpoint: DeprecatedEndpointOverride,
    source: &str,
) {
    let DeprecatedEndpointOverride {
        path,
        tls_cert_path,
        tls_key_path,
        tls_ca_cert_path,
    } = endpoint;
    if path.is_none()
        && tls_cert_path.is_none()
        && tls_key_path.is_none()
        && tls_ca_cert_path.is_none()
    {
        return;
    }

    let grpc = receivers
        .get_or_insert_with(ReceiversOverride::default)
        .grpc_mut();
    let [path_key, cert_key, key_key, ca_key] = DEPRECATED_ENDPOINT_KEYS;
    grpc.endpoint = resolve_deprecated(source, path_key.0, path, path_key.1, grpc.endpoint.take());
    let tls = grpc.tls.get_or_insert_with(TlsOverride::default);
    tls.cert_file = resolve_deprecated(
        source,
        cert_key.0,
        tls_cert_path,
        cert_key.1,
        tls.cert_file.take(),
    );
    tls.key_file = resolve_deprecated(
        source,
        key_key.0,
        tls_key_path,
        key_key.1,
        tls.key_file.take(),
    );
    tls.client_ca_file = resolve_deprecated(
        source,
        ca_key.0,
        tls_ca_cert_path,
        ca_key.1,
        tls.client_ca_file.take(),
    );
}

/// Merge `o` into `config` field-wise: a `Some` field replaces the current
/// value, `None` keeps whatever the stock file or an earlier layer supplied.
/// `apply_overrides` in `mod.rs` calls this once per layer, user first and
/// env last, so env vars win. A TLS path can be set or replaced but never
/// cleared (`null` means "no override").
pub(super) fn apply(config: &mut ReceiversConfig, o: &ReceiversOverride) {
    let Some(protocols) = o.otlp.as_ref().and_then(|otlp| otlp.protocols.as_ref()) else {
        return;
    };
    if let Some(grpc) = &protocols.grpc {
        apply_protocol(&mut config.otlp.protocols.grpc, grpc);
    }
    if let Some(http) = &protocols.http {
        apply_protocol(&mut config.otlp.protocols.http, http);
    }
}

fn apply_protocol(config: &mut ProtocolConfig, o: &ProtocolOverride) {
    if let Some(v) = o.enabled {
        config.enabled = v;
    }
    if let Some(v) = &o.endpoint {
        config.endpoint = v.clone();
    }
    if let Some(tls) = &o.tls {
        if let Some(v) = &tls.cert_file {
            config.tls.cert_file = Some(v.clone());
        }
        if let Some(v) = &tls.key_file {
            config.tls.key_file = Some(v.clone());
        }
        if let Some(v) = &tls.client_ca_file {
            config.tls.client_ca_file = Some(v.clone());
        }
    }
}
