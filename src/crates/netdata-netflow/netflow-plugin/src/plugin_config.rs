use crate::tiering::TierKind;
use anyhow::{Context, Result};
use bytesize::ByteSize;
use clap::{Parser, ValueEnum};
use jaq_interpret::ParseCtx;
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

fn default_true() -> bool {
    true
}

fn default_dynamic_bmp_listen() -> String {
    "0.0.0.0:10179".to_string()
}

fn default_dynamic_bmp_keep() -> Duration {
    Duration::from_secs(5 * 60)
}

fn default_dynamic_bmp_max_consecutive_decode_errors() -> usize {
    8
}

fn default_dynamic_bioris_timeout() -> Duration {
    Duration::from_millis(200)
}

fn default_dynamic_bioris_refresh() -> Duration {
    Duration::from_secs(30 * 60)
}

fn default_dynamic_bioris_refresh_timeout() -> Duration {
    Duration::from_secs(10)
}

fn default_classifier_cache_duration() -> Duration {
    Duration::from_secs(5 * 60)
}

fn default_remote_network_source_method() -> String {
    "GET".to_string()
}

fn default_remote_network_source_timeout() -> Duration {
    Duration::from_secs(60)
}

fn default_remote_network_source_interval() -> Duration {
    Duration::from_secs(60)
}

fn default_remote_network_source_transform() -> String {
    ".".to_string()
}

fn default_network_source_tls_verify() -> bool {
    true
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

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(untagged)]
pub(crate) enum RouteDistinguisherConfig {
    Numeric(u64),
    Text(String),
}

#[derive(Debug, Clone, Copy, Serialize, Deserialize, PartialEq, Eq)]
#[serde(rename_all = "kebab-case")]
pub(crate) enum AsnProviderConfig {
    Flow,
    FlowExceptPrivate,
    FlowExceptDefaultRoute,
    Geoip,
    #[serde(alias = "bmp")]
    Routing,
    #[serde(alias = "bmp-except-private")]
    RoutingExceptPrivate,
}

#[derive(Debug, Clone, Copy, Serialize, Deserialize, PartialEq, Eq)]
#[serde(rename_all = "kebab-case")]
pub(crate) enum NetProviderConfig {
    Flow,
    #[serde(alias = "bmp")]
    Routing,
}

fn default_asn_providers() -> Vec<AsnProviderConfig> {
    vec![
        AsnProviderConfig::Flow,
        AsnProviderConfig::Routing,
        AsnProviderConfig::Geoip,
    ]
}

fn default_net_providers() -> Vec<NetProviderConfig> {
    vec![NetProviderConfig::Flow, NetProviderConfig::Routing]
}

#[derive(Debug, Clone, Default, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub(crate) struct StaticRoutingLargeCommunityConfig {
    #[serde(default)]
    pub(crate) asn: u32,
    #[serde(default, alias = "localdata1", alias = "local-data-1")]
    pub(crate) local_data1: u32,
    #[serde(default, alias = "localdata2", alias = "local-data-2")]
    pub(crate) local_data2: u32,
}

#[derive(Debug, Clone, Default, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub(crate) struct StaticRoutingEntryConfig {
    #[serde(default)]
    pub(crate) asn: u32,
    #[serde(default, alias = "aspath", alias = "as-path")]
    pub(crate) as_path: Vec<u32>,
    #[serde(default)]
    pub(crate) communities: Vec<u32>,
    #[serde(default, alias = "largecommunities", alias = "large-communities")]
    pub(crate) large_communities: Vec<StaticRoutingLargeCommunityConfig>,
    #[serde(default, alias = "nexthop", alias = "next-hop")]
    pub(crate) next_hop: String,
    #[serde(default, alias = "netmask", alias = "net-mask")]
    pub(crate) net_mask: Option<u8>,
}

#[derive(Debug, Clone, Default, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub(crate) struct StaticRoutingConfig {
    #[serde(default)]
    pub(crate) prefixes: BTreeMap<String, StaticRoutingEntryConfig>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub(crate) struct RoutingDynamicBmpConfig {
    #[serde(default)]
    pub(crate) enabled: bool,
    #[serde(default = "default_dynamic_bmp_listen")]
    pub(crate) listen: String,
    #[serde(default, alias = "RDs")]
    pub(crate) rds: Vec<RouteDistinguisherConfig>,
    #[serde(default = "default_true", alias = "collect-asns")]
    pub(crate) collect_asns: bool,
    #[serde(default = "default_true", alias = "collect-as-paths")]
    pub(crate) collect_as_paths: bool,
    #[serde(default = "default_true", alias = "collect-communities")]
    pub(crate) collect_communities: bool,
    #[serde(default, alias = "receivebuffer", alias = "receive-buffer")]
    pub(crate) receive_buffer: usize,
    #[serde(
        default = "default_dynamic_bmp_max_consecutive_decode_errors",
        alias = "maxconsecutivedecodeerrors",
        alias = "max-consecutive-decode-errors"
    )]
    pub(crate) max_consecutive_decode_errors: usize,
    #[serde(default = "default_dynamic_bmp_keep", with = "humantime_serde")]
    pub(crate) keep: Duration,
}

impl Default for RoutingDynamicBmpConfig {
    fn default() -> Self {
        Self {
            enabled: false,
            listen: default_dynamic_bmp_listen(),
            rds: Vec::new(),
            collect_asns: true,
            collect_as_paths: true,
            collect_communities: true,
            receive_buffer: 0,
            max_consecutive_decode_errors: default_dynamic_bmp_max_consecutive_decode_errors(),
            keep: default_dynamic_bmp_keep(),
        }
    }
}

#[derive(Debug, Clone, Default, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub(crate) struct RoutingDynamicBiorisRisInstanceConfig {
    #[serde(
        default,
        alias = "grpcaddr",
        alias = "grpc-address",
        alias = "grpc_addr"
    )]
    pub(crate) grpc_addr: String,
    #[serde(
        default,
        alias = "grpcsecure",
        alias = "grpc-secure",
        alias = "grpc_secure"
    )]
    pub(crate) grpc_secure: bool,
    #[serde(default, alias = "vrfid", alias = "vrf-id", alias = "vrf_id")]
    pub(crate) vrf_id: u64,
    #[serde(default)]
    pub(crate) vrf: String,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub(crate) struct RoutingDynamicBiorisConfig {
    #[serde(default)]
    pub(crate) enabled: bool,
    #[serde(
        default,
        alias = "risinstances",
        alias = "ris-instances",
        alias = "ris_instances"
    )]
    pub(crate) ris_instances: Vec<RoutingDynamicBiorisRisInstanceConfig>,
    #[serde(default = "default_dynamic_bioris_timeout", with = "humantime_serde")]
    pub(crate) timeout: Duration,
    #[serde(default = "default_dynamic_bioris_refresh", with = "humantime_serde")]
    pub(crate) refresh: Duration,
    #[serde(
        default = "default_dynamic_bioris_refresh_timeout",
        alias = "refreshtimeout",
        alias = "refresh-timeout",
        alias = "refresh_timeout",
        with = "humantime_serde"
    )]
    pub(crate) refresh_timeout: Duration,
}

impl Default for RoutingDynamicBiorisConfig {
    fn default() -> Self {
        Self {
            enabled: false,
            ris_instances: Vec::new(),
            timeout: default_dynamic_bioris_timeout(),
            refresh: default_dynamic_bioris_refresh(),
            refresh_timeout: default_dynamic_bioris_refresh_timeout(),
        }
    }
}

#[derive(Debug, Clone, Default, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub(crate) struct RoutingDynamicConfig {
    #[serde(default)]
    pub(crate) bmp: RoutingDynamicBmpConfig,
    #[serde(default)]
    pub(crate) bioris: RoutingDynamicBiorisConfig,
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
pub(crate) struct GeoIpConfig {
    #[serde(default, alias = "asn-database")]
    pub(crate) asn_database: Vec<String>,
    #[serde(default, alias = "geo-database", alias = "country-database")]
    pub(crate) geo_database: Vec<String>,
    #[serde(default)]
    pub(crate) optional: bool,
}

#[derive(Debug, Clone, Default, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub(crate) struct NetworkAttributesConfig {
    #[serde(default)]
    pub(crate) name: String,
    #[serde(default)]
    pub(crate) role: String,
    #[serde(default)]
    pub(crate) site: String,
    #[serde(default)]
    pub(crate) region: String,
    #[serde(default)]
    pub(crate) country: String,
    #[serde(default)]
    pub(crate) state: String,
    #[serde(default)]
    pub(crate) city: String,
    #[serde(default)]
    pub(crate) tenant: String,
    #[serde(default)]
    pub(crate) asn: u32,
}

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

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(untagged)]
pub(crate) enum NetworkAttributesValue {
    Name(String),
    Attributes(NetworkAttributesConfig),
}

#[derive(Debug, Clone, Serialize, Deserialize)]
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
    #[serde(default)]
    pub(crate) geoip: GeoIpConfig,
    #[serde(default)]
    pub(crate) networks: BTreeMap<String, NetworkAttributesValue>,
    #[serde(default, alias = "network-sources")]
    pub(crate) network_sources: BTreeMap<String, RemoteNetworkSourceConfig>,
    #[serde(default, alias = "exporterclassifiers", alias = "exporter-classifiers")]
    pub(crate) exporter_classifiers: Vec<String>,
    #[serde(
        default,
        alias = "interfaceclassifiers",
        alias = "interface-classifiers"
    )]
    pub(crate) interface_classifiers: Vec<String>,
    #[serde(
        default = "default_classifier_cache_duration",
        with = "humantime_serde",
        alias = "classifiercacheduration",
        alias = "classifier-cache-duration"
    )]
    pub(crate) classifier_cache_duration: Duration,
    #[serde(
        default = "default_asn_providers",
        alias = "asnproviders",
        alias = "asn-providers"
    )]
    pub(crate) asn_providers: Vec<AsnProviderConfig>,
    #[serde(
        default = "default_net_providers",
        alias = "netproviders",
        alias = "net-providers"
    )]
    pub(crate) net_providers: Vec<NetProviderConfig>,
    #[serde(default, alias = "routing-static", alias = "static-routing")]
    pub(crate) routing_static: StaticRoutingConfig,
    #[serde(default, alias = "routing-dynamic", alias = "dynamic-routing")]
    pub(crate) routing_dynamic: RoutingDynamicConfig,
}

impl Default for EnrichmentConfig {
    fn default() -> Self {
        Self {
            default_sampling_rate: None,
            override_sampling_rate: None,
            metadata_static: StaticMetadataConfig::default(),
            geoip: GeoIpConfig::default(),
            networks: BTreeMap::new(),
            network_sources: BTreeMap::new(),
            exporter_classifiers: Vec::new(),
            interface_classifiers: Vec::new(),
            classifier_cache_duration: default_classifier_cache_duration(),
            asn_providers: default_asn_providers(),
            net_providers: default_net_providers(),
            routing_static: StaticRoutingConfig::default(),
            routing_dynamic: RoutingDynamicConfig::default(),
        }
    }
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
        if self.enrichment.routing_dynamic.bmp.enabled {
            self.enrichment
                .routing_dynamic
                .bmp
                .listen
                .parse::<SocketAddr>()
                .with_context(|| {
                    format!(
                        "invalid enrichment.routing_dynamic.bmp.listen address: {}",
                        self.enrichment.routing_dynamic.bmp.listen
                    )
                })?;
            if self.enrichment.routing_dynamic.bmp.keep.is_zero() {
                anyhow::bail!("enrichment.routing_dynamic.bmp.keep must be greater than 0");
            }
            if self
                .enrichment
                .routing_dynamic
                .bmp
                .max_consecutive_decode_errors
                == 0
            {
                anyhow::bail!(
                    "enrichment.routing_dynamic.bmp.max_consecutive_decode_errors must be greater than 0"
                );
            }
        }
        if self.enrichment.routing_dynamic.bioris.enabled {
            if self
                .enrichment
                .routing_dynamic
                .bioris
                .ris_instances
                .is_empty()
            {
                anyhow::bail!(
                    "enrichment.routing_dynamic.bioris.ris_instances must contain at least one instance when bioris is enabled"
                );
            }
            if self.enrichment.routing_dynamic.bioris.timeout.is_zero() {
                anyhow::bail!("enrichment.routing_dynamic.bioris.timeout must be greater than 0");
            }
            if self.enrichment.routing_dynamic.bioris.refresh.is_zero() {
                anyhow::bail!("enrichment.routing_dynamic.bioris.refresh must be greater than 0");
            }
            if self
                .enrichment
                .routing_dynamic
                .bioris
                .refresh_timeout
                .is_zero()
            {
                anyhow::bail!(
                    "enrichment.routing_dynamic.bioris.refresh_timeout must be greater than 0"
                );
            }
            for (idx, instance) in self
                .enrichment
                .routing_dynamic
                .bioris
                .ris_instances
                .iter()
                .enumerate()
            {
                let addr = instance.grpc_addr.trim();
                if addr.is_empty() {
                    anyhow::bail!(
                        "enrichment.routing_dynamic.bioris.ris_instances[{idx}].grpc_addr must be non-empty"
                    );
                }
                if !addr.contains("://") && !addr.contains(':') {
                    anyhow::bail!(
                        "enrichment.routing_dynamic.bioris.ris_instances[{idx}].grpc_addr must include host:port or an explicit URI scheme"
                    );
                }
            }
        }
        if self.enrichment.classifier_cache_duration < Duration::from_secs(1) {
            anyhow::bail!("enrichment.classifier_cache_duration must be >= 1s");
        }
        for (source_name, source_cfg) in &self.enrichment.network_sources {
            if source_cfg.url.trim().is_empty() {
                anyhow::bail!("enrichment.network_sources.{source_name}.url must be non-empty");
            }
            let method = source_cfg.method.trim().to_ascii_uppercase();
            if method != "GET" && method != "POST" {
                anyhow::bail!(
                    "enrichment.network_sources.{source_name}.method must be GET or POST"
                );
            }
            if source_cfg.timeout.is_zero() {
                anyhow::bail!("enrichment.network_sources.{source_name}.timeout must be > 0");
            }
            if source_cfg.interval.is_zero() {
                anyhow::bail!("enrichment.network_sources.{source_name}.interval must be > 0");
            }
            validate_network_source_tls(source_name, &source_cfg.tls)?;

            validate_network_source_transform(source_name, &source_cfg.transform)?;
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

fn validate_network_source_transform(source_name: &str, transform: &str) -> Result<()> {
    let normalized = if transform.trim().is_empty() {
        "."
    } else {
        transform.trim()
    };
    let tokens = jaq_syn::Lexer::new(normalized).lex().map_err(|errs| {
        anyhow::anyhow!(
            "enrichment.network_sources.{source_name}.transform lex error: {:?}",
            errs
        )
    })?;
    let main = jaq_syn::Parser::new(&tokens)
        .parse(|parser| parser.module(|module| module.term()))
        .map_err(|errs| {
            anyhow::anyhow!(
                "enrichment.network_sources.{source_name}.transform parse error: {:?}",
                errs
            )
        })?
        .conv(normalized);
    let mut ctx = ParseCtx::new(Vec::new());
    ctx.insert_natives(jaq_core::core());
    ctx.insert_defs(jaq_std::std());
    let _compiled = ctx.compile(main);
    if !ctx.errs.is_empty() {
        let errors = ctx
            .errs
            .into_iter()
            .map(|err| err.0.to_string())
            .collect::<Vec<_>>()
            .join("; ");
        anyhow::bail!(
            "enrichment.network_sources.{source_name}.transform compile error: {}",
            errors
        );
    }
    Ok(())
}

fn validate_network_source_tls(
    source_name: &str,
    tls: &RemoteNetworkSourceTlsConfig,
) -> Result<()> {
    let has_tls_fields = !tls.ca_file.trim().is_empty()
        || !tls.cert_file.trim().is_empty()
        || !tls.key_file.trim().is_empty();
    if has_tls_fields && !tls.enable {
        anyhow::bail!(
            "enrichment.network_sources.{source_name}.tls.enable must be true when tls.ca_file/tls.cert_file/tls.key_file are set"
        );
    }
    if !tls.key_file.trim().is_empty() && tls.cert_file.trim().is_empty() {
        anyhow::bail!(
            "enrichment.network_sources.{source_name}.tls.cert_file must be set when tls.key_file is set"
        );
    }
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn dynamic_bmp_decode_error_threshold_defaults_to_8() {
        let cfg = PluginConfig::default();
        assert_eq!(
            cfg.enrichment
                .routing_dynamic
                .bmp
                .max_consecutive_decode_errors,
            8
        );
    }

    #[test]
    fn validate_rejects_zero_bmp_decode_error_threshold_when_enabled() {
        let mut cfg = PluginConfig::default();
        cfg.enrichment.routing_dynamic.bmp.enabled = true;
        cfg.enrichment
            .routing_dynamic
            .bmp
            .max_consecutive_decode_errors = 0;

        let err = cfg.validate().expect_err("expected validation error");
        assert!(
            err.to_string()
                .contains("max_consecutive_decode_errors must be greater than 0")
        );
    }

    #[test]
    fn validate_rejects_enabled_bioris_without_instances() {
        let mut cfg = PluginConfig::default();
        cfg.enrichment.routing_dynamic.bioris.enabled = true;

        let err = cfg.validate().expect_err("expected validation error");
        assert!(
            err.to_string()
                .contains("ris_instances must contain at least one")
        );
    }

    #[test]
    fn validate_accepts_enabled_bioris_with_instance() {
        let mut cfg = PluginConfig::default();
        cfg.enrichment.routing_dynamic.bioris.enabled = true;
        cfg.enrichment.routing_dynamic.bioris.ris_instances.push(
            RoutingDynamicBiorisRisInstanceConfig {
                grpc_addr: "127.0.0.1:50051".to_string(),
                grpc_secure: false,
                vrf_id: 0,
                vrf: String::new(),
            },
        );

        cfg.validate().expect("configuration should be valid");
    }
}
