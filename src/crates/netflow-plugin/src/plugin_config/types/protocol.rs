use super::*;
use std::str::FromStr;

const DEFAULT_V9_TEMPLATE_LIFETIME: Duration = Duration::from_secs(90 * 60);
const DEFAULT_SAMPLING_CACHE_MAX_ENTRIES: usize = 100_000;
const DEFAULT_SAMPLING_CACHE_MAX_ENTRIES_PER_STREAM: usize = 65_536;

fn default_v9_template_lifetime() -> NullableDuration {
    NullableDuration(Some(DEFAULT_V9_TEMPLATE_LIFETIME))
}

fn default_sampling_cache_max_entries() -> usize {
    DEFAULT_SAMPLING_CACHE_MAX_ENTRIES
}

fn default_sampling_cache_max_entries_per_stream() -> usize {
    DEFAULT_SAMPLING_CACHE_MAX_ENTRIES_PER_STREAM
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) struct NullableDuration(Option<Duration>);

impl NullableDuration {
    pub(crate) fn get(self) -> Option<Duration> {
        self.0
    }
}

impl FromStr for NullableDuration {
    type Err = String;

    fn from_str(value: &str) -> Result<Self, Self::Err> {
        if value == "null" {
            return Ok(Self(None));
        }
        humantime::parse_duration(value)
            .map(|duration| Self(Some(duration)))
            .map_err(|err| err.to_string())
    }
}

impl Serialize for NullableDuration {
    fn serialize<S>(&self, serializer: S) -> Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        match self.0 {
            Some(duration) => {
                serializer.serialize_some(&humantime::format_duration(duration).to_string())
            }
            None => serializer.serialize_none(),
        }
    }
}

impl<'de> Deserialize<'de> for NullableDuration {
    fn deserialize<D>(deserializer: D) -> Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        let value = Option::<String>::deserialize(deserializer)?;
        match value {
            Some(value) => Self::from_str(&value).map_err(de::Error::custom),
            None => Ok(Self(None)),
        }
    }
}

#[derive(Debug, Parser, Clone, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub(crate) struct ProtocolConfig {
    #[arg(long = "netflow-enable-v5", default_value_t = true)]
    pub(crate) v5: bool,

    #[arg(long = "netflow-enable-v7", default_value_t = true)]
    pub(crate) v7: bool,

    #[arg(long = "netflow-enable-v9", default_value_t = true)]
    pub(crate) v9: bool,

    #[arg(long = "netflow-enable-ipfix", default_value_t = true)]
    pub(crate) ipfix: bool,

    #[arg(long = "netflow-enable-sflow", default_value_t = true)]
    pub(crate) sflow: bool,

    #[arg(
        long = "netflow-decapsulation-mode",
        value_enum,
        default_value_t = DecapsulationMode::None
    )]
    pub(crate) decapsulation_mode: DecapsulationMode,

    #[arg(
        long = "netflow-timestamp-source",
        value_enum,
        default_value_t = TimestampSource::Input
    )]
    pub(crate) timestamp_source: TimestampSource,

    #[arg(long = "netflow-v9-template-lifetime", default_value = "90m")]
    #[serde(default = "default_v9_template_lifetime")]
    pub(crate) v9_template_lifetime: NullableDuration,

    #[arg(
        long = "netflow-sampling-cache-max-entries",
        default_value_t = DEFAULT_SAMPLING_CACHE_MAX_ENTRIES
    )]
    #[serde(default = "default_sampling_cache_max_entries")]
    pub(crate) sampling_cache_max_entries: usize,

    #[arg(
        long = "netflow-sampling-cache-max-entries-per-stream",
        default_value_t = DEFAULT_SAMPLING_CACHE_MAX_ENTRIES_PER_STREAM
    )]
    #[serde(default = "default_sampling_cache_max_entries_per_stream")]
    pub(crate) sampling_cache_max_entries_per_stream: usize,
}

impl ProtocolConfig {
    pub(crate) fn effective_sampling_cache_limits(&self) -> (usize, usize) {
        (
            self.sampling_cache_max_entries,
            self.sampling_cache_max_entries_per_stream
                .min(self.sampling_cache_max_entries),
        )
    }
}

impl Default for ProtocolConfig {
    fn default() -> Self {
        Self {
            v5: true,
            v7: true,
            v9: true,
            ipfix: true,
            sflow: true,
            decapsulation_mode: DecapsulationMode::None,
            timestamp_source: TimestampSource::Input,
            v9_template_lifetime: default_v9_template_lifetime(),
            sampling_cache_max_entries: default_sampling_cache_max_entries(),
            sampling_cache_max_entries_per_stream: default_sampling_cache_max_entries_per_stream(),
        }
    }
}

#[derive(Debug, Clone, Copy, Default, Serialize, Deserialize, ValueEnum, PartialEq, Eq)]
#[serde(rename_all = "lowercase")]
pub(crate) enum DecapsulationMode {
    #[default]
    None,
    Srv6,
    Vxlan,
}

#[derive(Debug, Clone, Copy, Default, Serialize, Deserialize, ValueEnum, PartialEq, Eq)]
#[serde(rename_all = "snake_case")]
pub(crate) enum TimestampSource {
    #[default]
    Input,
    NetflowPacket,
    NetflowFirstSwitched,
}
