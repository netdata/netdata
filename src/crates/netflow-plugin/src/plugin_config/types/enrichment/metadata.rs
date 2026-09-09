use super::*;

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
