use super::*;

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

pub(super) fn default_asn_providers() -> Vec<AsnProviderConfig> {
    vec![
        AsnProviderConfig::Flow,
        AsnProviderConfig::Routing,
        AsnProviderConfig::Geoip,
    ]
}

pub(super) fn default_net_providers() -> Vec<NetProviderConfig> {
    vec![NetProviderConfig::Flow, NetProviderConfig::Routing]
}
