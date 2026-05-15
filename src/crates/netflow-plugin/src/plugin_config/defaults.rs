use super::*;

pub(super) const DEFAULT_NETDATA_CACHE_DIR: &str = "/var/cache/netdata";
pub(super) const DEFAULT_NETDATA_STOCK_DATA_DIR: &str = "/usr/share/netdata";
pub(super) const TOPOLOGY_IP_INTEL_DIR: &str = "topology-ip-intel";
pub(super) const TOPOLOGY_IP_ASN_MMDB: &str = "topology-ip-asn.mmdb";
pub(super) const TOPOLOGY_IP_GEO_MMDB: &str = "topology-ip-geo.mmdb";

pub(super) fn parse_duration(value: &str) -> Result<Duration, String> {
    humantime::parse_duration(value).map_err(|e| {
        format!(
            "invalid duration '{}' (examples: '1s', '5m', '1h'): {}",
            value, e
        )
    })
}

pub(super) fn parse_bytesize(value: &str) -> Result<ByteSize, String> {
    value.parse().map_err(|e| {
        format!(
            "invalid byte size '{}' (examples: '100MB', '10GB'): {}",
            value, e
        )
    })
}

pub(super) fn default_true() -> bool {
    true
}

pub(super) fn default_dynamic_bmp_listen() -> String {
    "0.0.0.0:10179".to_string()
}

pub(super) fn default_dynamic_bmp_keep() -> Duration {
    Duration::from_secs(5 * 60)
}

pub(super) fn default_dynamic_bmp_max_consecutive_decode_errors() -> usize {
    8
}

pub(super) fn default_dynamic_bioris_timeout() -> Duration {
    Duration::from_millis(200)
}

pub(super) fn default_dynamic_bioris_refresh() -> Duration {
    Duration::from_secs(30 * 60)
}

pub(super) fn default_dynamic_bioris_refresh_timeout() -> Duration {
    Duration::from_secs(10)
}

pub(super) fn default_classifier_cache_duration() -> Duration {
    Duration::from_secs(5 * 60)
}

pub(super) fn default_remote_network_source_method() -> String {
    "GET".to_string()
}

pub(super) fn default_remote_network_source_timeout() -> Duration {
    Duration::from_secs(60)
}

pub(super) fn default_remote_network_source_interval() -> Duration {
    Duration::from_secs(60)
}

pub(super) fn default_remote_network_source_transform() -> String {
    ".".to_string()
}

pub(super) fn default_query_max_groups() -> usize {
    50_000
}

pub(super) fn default_network_source_tls_verify() -> bool {
    true
}

pub(super) fn default_plugin_enabled() -> bool {
    true
}

pub(super) fn default_retention_size_of_journal_files() -> ByteSize {
    ByteSize::gb(10)
}

pub(super) fn default_retention_duration_of_journal_files() -> Duration {
    Duration::from_secs(7 * 24 * 60 * 60)
}

pub(super) fn default_retention_size_of_journal_files_opt() -> Option<ByteSize> {
    Some(default_retention_size_of_journal_files())
}

pub(super) fn default_retention_duration_of_journal_files_opt() -> Option<Duration> {
    Some(default_retention_duration_of_journal_files())
}

pub(super) fn default_rotation_duration_of_journal_file() -> Duration {
    Duration::from_secs(60 * 60)
}

pub(super) fn deserialize_opt_bytesize<'de, D>(
    deserializer: D,
) -> Result<Option<ByteSize>, D::Error>
where
    D: serde::Deserializer<'de>,
{
    let value = Option::<String>::deserialize(deserializer)?;
    match value {
        Some(value) => value.parse().map(Some).map_err(de::Error::custom),
        None => Ok(None),
    }
}

pub(super) fn deserialize_opt_duration<'de, D>(
    deserializer: D,
) -> Result<Option<Duration>, D::Error>
where
    D: serde::Deserializer<'de>,
{
    let value = Option::<String>::deserialize(deserializer)?;
    match value {
        Some(value) => humantime::parse_duration(&value)
            .map(Some)
            .map_err(de::Error::custom),
        None => Ok(None),
    }
}

pub(super) fn serialize_opt_bytesize<S>(
    value: &Option<ByteSize>,
    serializer: S,
) -> Result<S::Ok, S::Error>
where
    S: serde::Serializer,
{
    match value {
        Some(value) => serializer.serialize_some(&value.to_string()),
        None => serializer.serialize_none(),
    }
}

pub(super) fn serialize_opt_duration<S>(
    value: &Option<Duration>,
    serializer: S,
) -> Result<S::Ok, S::Error>
where
    S: serde::Serializer,
{
    match value {
        Some(value) => serializer.serialize_some(&humantime::format_duration(*value).to_string()),
        None => serializer.serialize_none(),
    }
}

pub(super) fn deserialize_interface_boundary<'de, D>(deserializer: D) -> Result<u8, D::Error>
where
    D: serde::Deserializer<'de>,
{
    #[derive(Deserialize)]
    #[serde(untagged)]
    enum BoundaryValue {
        Numeric(u8),
        Text(String),
    }

    let Some(value) = Option::<BoundaryValue>::deserialize(deserializer)? else {
        return Ok(0);
    };

    match value {
        BoundaryValue::Numeric(value) => Ok(value),
        BoundaryValue::Text(value) => match value.to_ascii_lowercase().as_str() {
            "undefined" => Ok(0),
            "external" => Ok(1),
            "internal" => Ok(2),
            _ => Err(de::Error::custom(format!(
                "invalid interface boundary '{value}', expected one of: undefined, external, internal"
            ))),
        },
    }
}
