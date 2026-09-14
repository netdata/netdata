use super::super::super::*;

#[derive(Debug, Deserialize)]
#[serde(untagged)]
pub(crate) enum GroupBySelection {
    One(String),
    Many(Vec<String>),
}

#[derive(Debug, Deserialize)]
#[serde(untagged)]
pub(crate) enum FacetSelection {
    One(String),
    Many(Vec<String>),
}
