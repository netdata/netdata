use clap::Parser;
use serde::{Deserialize, Serialize};

#[derive(Parser, Debug, Clone, Serialize, Deserialize)]
pub struct EndpointConfig {
    /// gRPC endpoint to listen on
    #[arg(long = "otel-endpoint", default_value = "127.0.0.1:4317")]
    pub path: String,

    /// Path to TLS certificate file (enables TLS when provided)
    #[arg(long = "otel-tls-cert-path")]
    pub tls_cert_path: Option<String>,

    /// Path to TLS private key file (required when TLS certificate is provided)
    #[arg(long = "otel-tls-key-path")]
    pub tls_key_path: Option<String>,

    /// Path to TLS CA certificate file for client authentication (optional)
    #[arg(long = "otel-tls-ca-cert-path")]
    pub tls_ca_cert_path: Option<String>,
}

impl Default for EndpointConfig {
    fn default() -> Self {
        Self {
            path: String::from("127.0.0.1:4317"),
            tls_cert_path: None,
            tls_key_path: None,
            tls_ca_cert_path: None,
        }
    }
}

#[derive(Debug, Default, Deserialize)]
pub(super) struct EndpointConfigOverride {
    #[serde(default)]
    pub(super) path: Option<String>,
    #[serde(default)]
    pub(super) tls_cert_path: Option<String>,
    #[serde(default)]
    pub(super) tls_key_path: Option<String>,
    #[serde(default)]
    pub(super) tls_ca_cert_path: Option<String>,
}

impl EndpointConfig {
    pub(super) fn apply_overrides(&mut self, o: &EndpointConfigOverride) {
        if let Some(v) = &o.path {
            self.path = v.clone();
        }
        if let Some(v) = &o.tls_cert_path {
            self.tls_cert_path = Some(v.clone());
        }
        if let Some(v) = &o.tls_key_path {
            self.tls_key_path = Some(v.clone());
        }
        if let Some(v) = &o.tls_ca_cert_path {
            self.tls_ca_cert_path = Some(v.clone());
        }
    }
}
