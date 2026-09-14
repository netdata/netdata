use super::*;

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(untagged)]
pub(crate) enum RouteDistinguisherConfig {
    Numeric(u64),
    Text(String),
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
