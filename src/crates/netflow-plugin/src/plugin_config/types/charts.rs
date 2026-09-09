use super::*;

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub(crate) struct ChartsConfig {
    #[serde(default)]
    pub(crate) memory_diagnostics: MemoryDiagnosticsConfig,
}

impl Default for ChartsConfig {
    fn default() -> Self {
        Self {
            memory_diagnostics: MemoryDiagnosticsConfig::default(),
        }
    }
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub(crate) struct MemoryDiagnosticsConfig {
    #[serde(default)]
    pub(crate) enabled: bool,

    #[serde(
        default = "default_memory_diagnostics_interval",
        with = "humantime_serde"
    )]
    pub(crate) interval: Duration,
}

impl Default for MemoryDiagnosticsConfig {
    fn default() -> Self {
        Self {
            enabled: false,
            interval: default_memory_diagnostics_interval(),
        }
    }
}
