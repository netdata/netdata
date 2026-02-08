use crate::tiering::TierKind;
use anyhow::{Context, Result};
use bytesize::ByteSize;
use clap::{Parser, ValueEnum};
use rt::NetdataEnv;
use serde::de;
use serde::{Deserialize, Serialize};
use std::collections::BTreeMap;
use std::fs;
use std::net::SocketAddr;
use std::path::{Path, PathBuf};
use std::time::Duration;

fn parse_duration(value: &str) -> Result<Duration, String> {
    humantime::parse_duration(value).map_err(|e| {
        format!(
            "invalid duration '{}' (examples: '1s', '5m', '1h'): {}",
            value, e
        )
    })
}

fn parse_bytesize(value: &str) -> Result<ByteSize, String> {
    value.parse().map_err(|e| {
        format!(
            "invalid size '{}' (examples: '256MB', '10GB'): {}",
            value, e
        )
    })
}

#[derive(Debug, Parser, Clone, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub(crate) struct ListenerConfig {
    #[arg(long = "netflow-listen", default_value = "0.0.0.0:2055")]
    pub(crate) listen: String,

    #[arg(long = "netflow-max-packet-size", default_value_t = 9216)]
    pub(crate) max_packet_size: usize,

    #[arg(long = "netflow-sync-every-entries", default_value_t = 1024)]
    pub(crate) sync_every_entries: usize,

    #[arg(
        long = "netflow-sync-interval",
        default_value = "1s",
        value_parser = parse_duration
    )]
    #[serde(with = "humantime_serde")]
    pub(crate) sync_interval: Duration,
}

impl Default for ListenerConfig {
    fn default() -> Self {
        Self {
            listen: "0.0.0.0:2055".to_string(),
            max_packet_size: 9216,
            sync_every_entries: 1024,
            sync_interval: Duration::from_secs(1),
        }
    }
}

#[derive(Debug, Parser, Clone, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub(crate) struct ProtocolConfig {
    #[arg(long = "netflow-enable-v5", default_value_t = true)]
    pub(crate) v5: bool,

    #[arg(long = "netflow-enable-v7", default_value_t = true)]
    pub(crate) v7: bool,

    #[arg(long = "netflow-enable-v9", default_value_t = true)]
    pub(crate) v9: bool,

    #[arg(long = "netflow-enable-ipfix", default_value_t = true)]
    pub(crate) ipfix: bool,

    #[arg(long = "netflow-enable-sflow", default_value_t = true)]
    pub(crate) sflow: bool,

    #[arg(
        long = "netflow-decapsulation-mode",
        value_enum,
        default_value_t = DecapsulationMode::None
    )]
    pub(crate) decapsulation_mode: DecapsulationMode,

    #[arg(
        long = "netflow-timestamp-source",
        value_enum,
        default_value_t = TimestampSource::Input
    )]
    pub(crate) timestamp_source: TimestampSource,
}

impl Default for ProtocolConfig {
    fn default() -> Self {
        Self {
            v5: true,
            v7: true,
            v9: true,
            ipfix: true,
            sflow: true,
            decapsulation_mode: DecapsulationMode::None,
            timestamp_source: TimestampSource::Input,
        }
    }
}

#[derive(Debug, Clone, Copy, Default, Serialize, Deserialize, ValueEnum, PartialEq, Eq)]
#[serde(rename_all = "lowercase")]
pub(crate) enum DecapsulationMode {
    #[default]
    None,
    Srv6,
    Vxlan,
}

#[derive(Debug, Clone, Copy, Default, Serialize, Deserialize, ValueEnum, PartialEq, Eq)]
#[serde(rename_all = "snake_case")]
pub(crate) enum TimestampSource {
    #[default]
    Input,
    NetflowPacket,
    NetflowFirstSwitched,
}

#[derive(Debug, Parser, Clone, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub(crate) struct JournalConfig {
    #[arg(long = "netflow-journal-dir", default_value = "flows")]
    pub(crate) journal_dir: String,

    #[arg(
        long = "netflow-rotation-size-of-journal-file",
        default_value = "256MB",
        value_parser = parse_bytesize
    )]
    #[serde(with = "bytesize_serde")]
    pub(crate) size_of_journal_file: ByteSize,

    #[arg(
        long = "netflow-rotation-duration-of-journal-file",
        default_value = "1h",
        value_parser = parse_duration
    )]
    #[serde(with = "humantime_serde")]
    pub(crate) duration_of_journal_file: Duration,

    #[arg(
        long = "netflow-retention-number-of-journal-files",
        default_value_t = 64
    )]
    pub(crate) number_of_journal_files: usize,

    #[arg(
        long = "netflow-retention-size-of-journal-files",
        default_value = "10GB",
        value_parser = parse_bytesize
    )]
    #[serde(with = "bytesize_serde")]
    pub(crate) size_of_journal_files: ByteSize,

    #[arg(
        long = "netflow-retention-duration-of-journal-files",
        default_value = "7d",
        value_parser = parse_duration
    )]
    #[serde(with = "humantime_serde")]
    pub(crate) duration_of_journal_files: Duration,

    #[arg(
        long = "netflow-query-1m-max-window",
        default_value = "6h",
        value_parser = parse_duration
    )]
    #[serde(with = "humantime_serde")]
    pub(crate) query_1m_max_window: Duration,

    #[arg(
        long = "netflow-query-5m-max-window",
        default_value = "24h",
        value_parser = parse_duration
    )]
    #[serde(with = "humantime_serde")]
    pub(crate) query_5m_max_window: Duration,
}

impl Default for JournalConfig {
    fn default() -> Self {
        Self {
            journal_dir: "flows".to_string(),
            size_of_journal_file: ByteSize::mb(256),
            duration_of_journal_file: Duration::from_secs(60 * 60),
            number_of_journal_files: 64,
            size_of_journal_files: ByteSize::gb(10),
            duration_of_journal_files: Duration::from_secs(7 * 24 * 60 * 60),
            query_1m_max_window: Duration::from_secs(6 * 60 * 60),
            query_5m_max_window: Duration::from_secs(24 * 60 * 60),
        }
    }
}

impl JournalConfig {
    pub(crate) fn base_dir(&self) -> PathBuf {
        PathBuf::from(&self.journal_dir)
    }

    pub(crate) fn tier_dir(&self, tier: TierKind) -> PathBuf {
        self.base_dir().join(tier.dir_name())
    }

    pub(crate) fn raw_tier_dir(&self) -> PathBuf {
        self.tier_dir(TierKind::Raw)
    }

    pub(crate) fn minute_1_tier_dir(&self) -> PathBuf {
        self.tier_dir(TierKind::Minute1)
    }

    pub(crate) fn minute_5_tier_dir(&self) -> PathBuf {
        self.tier_dir(TierKind::Minute5)
    }

    pub(crate) fn hour_1_tier_dir(&self) -> PathBuf {
        self.tier_dir(TierKind::Hour1)
    }

    pub(crate) fn all_tier_dirs(&self) -> [PathBuf; 4] {
        [
            self.raw_tier_dir(),
            self.minute_1_tier_dir(),
            self.minute_5_tier_dir(),
            self.hour_1_tier_dir(),
        ]
    }

    pub(crate) fn decoder_state_path(&self) -> PathBuf {
        self.base_dir().join("decoder-state.json")
    }
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(untagged)]
pub(crate) enum SamplingRateSetting {
    Single(u64),
    PerPrefix(BTreeMap<String, u64>),
}

#[derive(Debug, Clone, Default, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub(crate) struct StaticInterfaceConfig {
    #[serde(default)]
    pub(crate) name: String,
    #[serde(default)]
    pub(crate) description: String,
    #[serde(default)]
    pub(crate) speed: u64,
    #[serde(default)]
    pub(crate) provider: String,
    #[serde(default)]
    pub(crate) connectivity: String,
    #[serde(default, deserialize_with = "deserialize_interface_boundary")]
    pub(crate) boundary: u8,
}

#[derive(Debug, Clone, Default, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub(crate) struct StaticExporterConfig {
    #[serde(default)]
    pub(crate) name: String,
    #[serde(default)]
    pub(crate) region: String,
    #[serde(default)]
    pub(crate) role: String,
    #[serde(default)]
    pub(crate) tenant: String,
    #[serde(default)]
    pub(crate) site: String,
    #[serde(default)]
    pub(crate) group: String,
    #[serde(default)]
    pub(crate) default: StaticInterfaceConfig,
    #[serde(default, alias = "ifindexes", alias = "if-indexes")]
    pub(crate) if_indexes: BTreeMap<u32, StaticInterfaceConfig>,
    #[serde(
        default,
        alias = "skipmissinginterfaces",
        alias = "skip-missing-interfaces"
    )]
    pub(crate) skip_missing_interfaces: bool,
}

#[derive(Debug, Clone, Default, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub(crate) struct StaticMetadataConfig {
    #[serde(default)]
    pub(crate) exporters: BTreeMap<String, StaticExporterConfig>,
}

#[derive(Debug, Clone, Default, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub(crate) struct EnrichmentConfig {
    #[serde(
        default,
        alias = "defaultsamplingrate",
        alias = "default-sampling-rate"
    )]
    pub(crate) default_sampling_rate: Option<SamplingRateSetting>,
    #[serde(
        default,
        alias = "overridesamplingrate",
        alias = "override-sampling-rate"
    )]
    pub(crate) override_sampling_rate: Option<SamplingRateSetting>,
    #[serde(default, alias = "metadata-static", alias = "static-metadata")]
    pub(crate) metadata_static: StaticMetadataConfig,
    #[serde(default, alias = "exporterclassifiers", alias = "exporter-classifiers")]
    pub(crate) exporter_classifiers: Vec<String>,
    #[serde(
        default,
        alias = "interfaceclassifiers",
        alias = "interface-classifiers"
    )]
    pub(crate) interface_classifiers: Vec<String>,
}

fn deserialize_interface_boundary<'de, D>(deserializer: D) -> Result<u8, D::Error>
where
    D: serde::Deserializer<'de>,
{
    #[derive(Deserialize)]
    #[serde(untagged)]
    enum BoundaryValue {
        Numeric(u8),
        Text(String),
    }

    let Some(value) = Option::<BoundaryValue>::deserialize(deserializer)? else {
        return Ok(0);
    };

    match value {
        BoundaryValue::Numeric(value) => Ok(value),
        BoundaryValue::Text(value) => match value.to_ascii_lowercase().as_str() {
            "undefined" => Ok(0),
            "external" => Ok(1),
            "internal" => Ok(2),
            _ => Err(de::Error::custom(format!(
                "invalid interface boundary '{value}', expected one of: undefined, external, internal"
            ))),
        },
    }
}

#[derive(Debug, Parser, Clone, Serialize, Deserialize)]
#[command(name = "netflow-plugin")]
#[command(about = "NetFlow/IPFIX/sFlow journal ingestion plugin")]
#[command(version = "0.1")]
#[serde(deny_unknown_fields)]
pub(crate) struct PluginConfig {
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
            listener: ListenerConfig::default(),
            protocols: ProtocolConfig::default(),
            journal: JournalConfig::default(),
            enrichment: EnrichmentConfig::default(),
            _update_frequency: None,
            _netdata_env: NetdataEnv::default(),
        }
    }
}

impl PluginConfig {
    pub(crate) fn new() -> Result<Self> {
        let netdata_env = NetdataEnv::from_environment();

        let mut cfg = if netdata_env.running_under_netdata() {
            Self::load_from_netdata_config(&netdata_env)?
        } else {
            Self::parse()
        };

        cfg._netdata_env = netdata_env.clone();
        cfg.journal.journal_dir =
            resolve_relative_path(&cfg.journal.journal_dir, netdata_env.cache_dir.as_deref());
        cfg.ensure_storage_layout()?;

        cfg.validate()?;
        Ok(cfg)
    }

    fn load_from_netdata_config(netdata_env: &NetdataEnv) -> Result<Self> {
        let candidates = [
            netdata_env
                .user_config_dir
                .as_ref()
                .map(|p| p.join("netflow.yaml")),
            netdata_env
                .stock_config_dir
                .as_ref()
                .map(|p| p.join("netflow.yaml")),
        ];

        for path in candidates.into_iter().flatten() {
            if path.is_file() {
                return Self::from_yaml_file(&path).with_context(|| {
                    format!("failed to load netflow config from {}", path.display())
                });
            }
        }

        Ok(Self::default())
    }

    fn from_yaml_file(path: &Path) -> Result<Self> {
        let content = fs::read_to_string(path)
            .with_context(|| format!("failed to read {}", path.display()))?;
        let cfg = serde_yaml::from_str::<Self>(&content)
            .with_context(|| format!("failed to parse {}", path.display()))?;
        Ok(cfg)
    }

    fn validate(&self) -> Result<()> {
        if self.listener.max_packet_size == 0 {
            anyhow::bail!("listener.max_packet_size must be greater than 0");
        }
        if self.listener.sync_every_entries == 0 {
            anyhow::bail!("listener.sync_every_entries must be greater than 0");
        }

        self.listener
            .listen
            .parse::<SocketAddr>()
            .with_context(|| format!("invalid listener address: {}", self.listener.listen))?;

        if !(self.protocols.v5
            || self.protocols.v7
            || self.protocols.v9
            || self.protocols.ipfix
            || self.protocols.sflow)
        {
            anyhow::bail!("at least one protocol must be enabled");
        }
        if self.journal.query_1m_max_window.is_zero() {
            anyhow::bail!("journal.query_1m_max_window must be greater than 0");
        }
        if self.journal.query_5m_max_window.is_zero() {
            anyhow::bail!("journal.query_5m_max_window must be greater than 0");
        }
        if self.journal.query_5m_max_window < self.journal.query_1m_max_window {
            anyhow::bail!("journal.query_5m_max_window must be >= journal.query_1m_max_window");
        }

        Ok(())
    }

    fn ensure_storage_layout(&self) -> Result<()> {
        for dir in self.journal.all_tier_dirs() {
            fs::create_dir_all(&dir)
                .with_context(|| format!("failed to create tier directory {}", dir.display()))?;
        }
        Ok(())
    }
}

fn resolve_relative_path(path: &str, base_dir: Option<&Path>) -> String {
    let p = Path::new(path);
    if p.is_absolute() {
        return p.to_string_lossy().to_string();
    }

    if let Some(base) = base_dir {
        return PathBuf::from(base).join(p).to_string_lossy().to_string();
    }

    p.to_string_lossy().to_string()
}
