use super::*;

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(untagged)]
pub(crate) enum SamplingRateSetting {
    Single(u64),
    PerPrefix(BTreeMap<String, u64>),
}
