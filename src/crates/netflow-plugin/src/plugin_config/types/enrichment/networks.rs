use super::*;

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
    pub(crate) latitude: Option<f64>,
    #[serde(default)]
    pub(crate) longitude: Option<f64>,
    #[serde(default)]
    pub(crate) tenant: String,
    #[serde(default)]
    pub(crate) asn: u32,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(untagged)]
pub(crate) enum NetworkAttributesValue {
    Name(String),
    Attributes(NetworkAttributesConfig),
}
