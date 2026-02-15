use bridge::config::EndpointConfig;
use serde::Deserialize;

#[derive(Debug, Default, Deserialize)]
pub(super) struct EndpointOverride {
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
    pub(super) fn has_any(&self) -> bool {
        self.path.is_some()
            || self.tls_cert_path.is_some()
            || self.tls_key_path.is_some()
            || self.tls_ca_cert_path.is_some()
    }
}

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
