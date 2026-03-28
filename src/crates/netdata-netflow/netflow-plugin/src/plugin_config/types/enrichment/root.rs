use super::*;

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
