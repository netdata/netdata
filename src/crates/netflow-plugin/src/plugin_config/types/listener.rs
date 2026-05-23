use super::*;

#[derive(Debug, Parser, Clone, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub(crate) struct ListenerConfig {
    #[arg(long = "netflow-listen", default_value = "0.0.0.0:2055")]
    pub(crate) listen: String,

    #[arg(long = "netflow-max-packet-size", default_value_t = 9216)]
    pub(crate) max_packet_size: usize,

    #[arg(long = "netflow-sync-every-entries", default_value_t = 1024)]
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
            listen: "0.0.0.0:2055".to_string(),
            max_packet_size: 9216,
            sync_every_entries: 1024,
            sync_interval: Duration::from_secs(1),
        }
    }
}
