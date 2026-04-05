use super::*;

#[derive(Debug, Parser, Clone, Serialize, Deserialize)]
#[command(name = "netflow-plugin")]
#[command(about = "NetFlow/IPFIX/sFlow journal ingestion plugin")]
#[command(version = "0.1")]
#[serde(deny_unknown_fields)]
pub(crate) struct PluginConfig {
    #[arg(long = "netflow-enabled", default_value_t = true)]
    #[serde(default = "default_plugin_enabled")]
    pub(crate) enabled: bool,

    #[command(flatten)]
    #[serde(rename = "listener")]
    pub(crate) listener: ListenerConfig,

    #[command(flatten)]
    #[serde(rename = "protocols")]
    pub(crate) protocols: ProtocolConfig,

    #[command(flatten)]
    #[serde(rename = "journal")]
    pub(crate) journal: JournalConfig,

    #[arg(skip)]
    #[serde(default, rename = "enrichment")]
    pub(crate) enrichment: EnrichmentConfig,

    #[arg(hide = true, help = "Collection interval in seconds (ignored)")]
    #[serde(skip)]
    pub(crate) _update_frequency: Option<u32>,

    #[arg(skip)]
    #[serde(skip)]
    pub(crate) _netdata_env: NetdataEnv,
}

impl Default for PluginConfig {
    fn default() -> Self {
        Self {
            enabled: default_plugin_enabled(),
            listener: ListenerConfig::default(),
            protocols: ProtocolConfig::default(),
            journal: JournalConfig::default(),
            enrichment: EnrichmentConfig::default(),
            _update_frequency: None,
            _netdata_env: NetdataEnv::default(),
        }
    }
}
