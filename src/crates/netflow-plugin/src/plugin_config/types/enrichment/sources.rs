use super::*;

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub(crate) struct RemoteNetworkSourceTlsConfig {
    #[serde(default)]
    pub(crate) enable: bool,
    #[serde(default = "default_network_source_tls_verify")]
    pub(crate) verify: bool,
    #[serde(default, alias = "skip-verify")]
    pub(crate) skip_verify: bool,
    #[serde(default, alias = "ca-file")]
    pub(crate) ca_file: String,
    #[serde(default, alias = "cert-file")]
    pub(crate) cert_file: String,
    #[serde(default, alias = "key-file")]
    pub(crate) key_file: String,
}

impl Default for RemoteNetworkSourceTlsConfig {
    fn default() -> Self {
        Self {
            enable: false,
            verify: true,
            skip_verify: false,
            ca_file: String::new(),
            cert_file: String::new(),
            key_file: String::new(),
        }
    }
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub(crate) struct RemoteNetworkSourceConfig {
    #[serde(default)]
    pub(crate) url: String,
    #[serde(default = "default_remote_network_source_method")]
    pub(crate) method: String,
    #[serde(default)]
    pub(crate) headers: BTreeMap<String, String>,
    #[serde(default = "default_true")]
    pub(crate) proxy: bool,
    #[serde(default)]
    pub(crate) tls: RemoteNetworkSourceTlsConfig,
    #[serde(
        default = "default_remote_network_source_timeout",
        with = "humantime_serde"
    )]
    pub(crate) timeout: Duration,
    #[serde(
        default = "default_remote_network_source_interval",
        with = "humantime_serde"
    )]
    pub(crate) interval: Duration,
    #[serde(default = "default_remote_network_source_transform")]
    pub(crate) transform: String,
}

impl Default for RemoteNetworkSourceConfig {
    fn default() -> Self {
        Self {
            url: String::new(),
            method: default_remote_network_source_method(),
            headers: BTreeMap::new(),
            proxy: true,
            tls: RemoteNetworkSourceTlsConfig::default(),
            timeout: default_remote_network_source_timeout(),
            interval: default_remote_network_source_interval(),
            transform: default_remote_network_source_transform(),
        }
    }
}
