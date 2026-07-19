use super::*;

#[derive(Debug, Default)]
pub(crate) struct SamplingState {
    pub(crate) by_exporter: HashMap<IpAddr, HashMap<SamplingKey, u64>>,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash)]
pub(crate) struct SamplingKey {
    pub(crate) version: u16,
    pub(crate) observation_domain_id: u32,
    pub(crate) sampler_id: u64,
}
