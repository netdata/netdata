use super::*;

#[derive(Debug, Default, Deserialize)]
pub(super) struct RemoteRecord {
    pub(super) prefix: String,
    #[serde(default)]
    pub(super) name: String,
    #[serde(default)]
    pub(super) role: String,
    #[serde(default)]
    pub(super) site: String,
    #[serde(default)]
    pub(super) region: String,
    #[serde(default)]
    pub(super) country: String,
    #[serde(default)]
    pub(super) state: String,
    #[serde(default)]
    pub(super) city: String,
    #[serde(default)]
    pub(super) tenant: String,
    #[serde(default)]
    pub(super) asn: AsnValue,
    #[serde(default)]
    pub(super) asn_name: String,
}

#[derive(Debug, Default, Deserialize)]
#[serde(untagged)]
pub(super) enum AsnValue {
    #[default]
    Empty,
    Number(u32),
    Text(String),
}

#[derive(Debug, Clone)]
pub(super) struct SourceRecordState {
    pub(super) by_source: Arc<RwLock<BTreeMap<String, Vec<NetworkSourceRecord>>>>,
}

#[derive(Debug, Clone)]
pub(super) struct CompiledTransform {
    pub(super) expression: String,
    pub(super) filter: Filter,
}
