use super::*;

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
