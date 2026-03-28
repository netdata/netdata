use super::super::*;

pub(crate) const DEFAULT_QUERY_WINDOW_SECONDS: u32 = 15 * 60;
pub(crate) const DEFAULT_QUERY_LIMIT: usize = 25;
pub(crate) const MAX_QUERY_LIMIT: usize = 500;
pub(crate) const MAX_GROUP_BY_FIELDS: usize = 10;
#[cfg(test)]
pub(crate) const DEFAULT_GROUP_ACCUMULATOR_MAX_GROUPS: usize = 50_000;
pub(crate) const FACET_VALUE_LIMIT: usize = 100;
pub(crate) const FACET_CACHE_JOURNAL_WINDOW_SIZE: u64 = 8 * 1024 * 1024;
#[cfg(test)]
pub(crate) const DEFAULT_FACET_ACCUMULATOR_MAX_VALUES_PER_FIELD: usize = 5_000;
pub(crate) const TIMESERIES_MIN_BUCKETS: u32 = 100;
pub(crate) const TIMESERIES_MAX_BUCKETS: u32 = 500;
pub(crate) const MIN_TIMESERIES_BUCKET_SECONDS: u32 = 60;
pub(crate) const OTHER_BUCKET_LABEL: &str = "__other__";
pub(crate) const OVERFLOW_BUCKET_LABEL: &str = "__overflow__";
pub(crate) const VIRTUAL_FLOW_FIELDS: &[&str] = &["ICMPV4", "ICMPV6"];

pub(crate) const DEFAULT_GROUP_BY_FIELDS: &[&str] = &["SRC_AS_NAME", "PROTOCOL", "DST_AS_NAME"];
pub(crate) const COUNTRY_MAP_GROUP_BY_FIELDS: &[&str] = &["SRC_COUNTRY", "DST_COUNTRY"];

pub(crate) const RAW_ONLY_FIELDS: &[&str] = &["SRC_ADDR", "DST_ADDR", "SRC_PORT", "DST_PORT"];

pub(crate) const FACET_EXCLUDED_FIELDS: &[&str] = &[
    "_BOOT_ID",
    "_SOURCE_REALTIME_TIMESTAMP",
    "SRC_ADDR",
    "DST_ADDR",
    "SRC_PORT",
    "DST_PORT",
    "BYTES",
    "PACKETS",
    "FLOWS",
    "RAW_BYTES",
    "RAW_PACKETS",
];

pub(crate) fn default_group_by() -> Vec<String> {
    DEFAULT_GROUP_BY_FIELDS
        .iter()
        .map(|s| s.to_string())
        .collect()
}

pub(crate) fn supported_flow_field_names() -> impl Iterator<Item = &'static str> {
    canonical_flow_field_names()
        .filter(|field| !matches!(*field, "SAMPLING_RATE" | "RAW_BYTES" | "RAW_PACKETS"))
        .chain(VIRTUAL_FLOW_FIELDS.iter().copied())
}

pub(crate) static GROUP_BY_ALLOWED_FIELDS: LazyLock<HashSet<&'static str>> = LazyLock::new(|| {
    supported_flow_field_names()
        .filter(|field| field_is_groupable(field))
        .collect()
});

pub(crate) static SELECTION_ALLOWED_FIELDS: LazyLock<HashSet<&'static str>> =
    LazyLock::new(|| supported_flow_field_names().collect());

pub(crate) static GROUP_BY_ALLOWED_OPTIONS: LazyLock<Vec<String>> = LazyLock::new(|| {
    supported_flow_field_names()
        .filter(|field| field_is_groupable(field))
        .map(str::to_string)
        .collect()
});

pub(crate) static FACET_ALLOWED_OPTIONS: LazyLock<Vec<String>> = LazyLock::new(|| {
    supported_flow_field_names()
        .filter(|field| facet_field_requested(field))
        .map(str::to_string)
        .collect()
});

pub(crate) fn supported_group_by_fields() -> &'static [String] {
    GROUP_BY_ALLOWED_OPTIONS.as_slice()
}
