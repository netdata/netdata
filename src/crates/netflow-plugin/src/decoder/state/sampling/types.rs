use super::*;

#[derive(Debug, Default)]
pub(crate) struct SamplingState {
    pub(crate) by_exporter: HashMap<IpAddr, HashMap<SamplingKey, u64>>,
    pub(crate) v9_sampling_templates:
        HashMap<IpAddr, HashMap<u32, HashMap<u16, V9SamplingTemplate>>>,
    pub(crate) v9_datalink_templates:
        HashMap<IpAddr, HashMap<u32, HashMap<u16, V9DataLinkTemplate>>>,
    pub(crate) ipfix_datalink_templates:
        HashMap<IpAddr, HashMap<u32, HashMap<u16, IPFixDataLinkTemplate>>>,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash)]
pub(crate) struct SamplingKey {
    pub(crate) version: u16,
    pub(crate) observation_domain_id: u32,
    pub(crate) sampler_id: u64,
}

#[derive(Debug, Clone, Copy)]
pub(crate) struct V9TemplateField {
    pub(crate) field_type: u16,
    pub(crate) field_length: usize,
}

#[derive(Debug, Clone)]
pub(crate) struct V9SamplingTemplate {
    pub(crate) scope_fields: Vec<V9TemplateField>,
    pub(crate) option_fields: Vec<V9TemplateField>,
    pub(crate) record_length: usize,
}

#[derive(Debug, Clone)]
pub(crate) struct V9DataLinkTemplate {
    pub(crate) fields: Vec<V9TemplateField>,
}

#[derive(Debug, Clone, Copy)]
pub(crate) struct IPFixTemplateField {
    pub(crate) field_type: u16,
    pub(crate) field_length: u16,
    pub(crate) enterprise_number: Option<u32>,
}

#[derive(Debug, Clone)]
pub(crate) struct IPFixDataLinkTemplate {
    pub(crate) fields: Vec<IPFixTemplateField>,
}
