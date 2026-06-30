use super::*;

#[derive(Debug, Parser, Clone, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub(crate) struct ListenerConfig {
    #[arg(
        long = "netflow-listen",
        value_name = "ADDR",
        default_values_t = default_netflow_listen()
    )]
    #[serde(default = "default_netflow_listen")]
    #[serde(deserialize_with = "deserialize_listen")]
    pub(crate) listen: Vec<String>,

    #[arg(long = "netflow-max-packet-size", default_value_t = 9216)]
    pub(crate) max_packet_size: usize,

    /// Fsync the active raw journal after this many entries. 0 (default)
    /// disables periodic fsync: entries reach disk via kernel writeback, and
    /// every journal file is still fully synced when it is rotated/archived
    /// and once at shutdown. Values > 0 also fsync at least once per
    /// `sync_interval`; at high flow rates this stalls the receive path and
    /// can cause UDP drops.
    #[arg(long = "netflow-sync-every-entries", default_value_t = 0)]
    pub(crate) sync_every_entries: usize,

    #[arg(
        long = "netflow-sync-interval",
        default_value = "1s",
        value_parser = parse_duration
    )]
    #[serde(with = "humantime_serde")]
    pub(crate) sync_interval: Duration,
}

impl Default for ListenerConfig {
    fn default() -> Self {
        Self {
            listen: default_netflow_listen(),
            max_packet_size: 9216,
            sync_every_entries: 0,
            sync_interval: Duration::from_secs(1),
        }
    }
}

fn deserialize_listen<'de, D>(deserializer: D) -> std::result::Result<Vec<String>, D::Error>
where
    D: serde::Deserializer<'de>,
{
    #[derive(Deserialize)]
    #[serde(untagged)]
    enum ListenValue {
        Scalar(String),
        List(Vec<String>),
    }

    match ListenValue::deserialize(deserializer)? {
        ListenValue::Scalar(value) => Ok(vec![value]),
        ListenValue::List(values) => Ok(values),
    }
}
