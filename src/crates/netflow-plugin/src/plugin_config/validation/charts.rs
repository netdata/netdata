use super::super::PluginConfig;
use anyhow::{Result, bail};
use std::time::Duration;

pub(super) fn validate_charts(cfg: &PluginConfig) -> Result<()> {
    let diagnostics = &cfg.charts.memory_diagnostics;
    if diagnostics.enabled && diagnostics.interval < Duration::from_secs(1) {
        bail!("charts.memory_diagnostics.interval must be at least 1s when enabled");
    }

    Ok(())
}
