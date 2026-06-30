use super::*;

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
