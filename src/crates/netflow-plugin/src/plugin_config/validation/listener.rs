use crate::plugin_config::PluginConfig;
use anyhow::{Context, Result};
use std::net::SocketAddr;

pub(super) fn validate_listener_and_protocols(cfg: &PluginConfig) -> Result<()> {
    if cfg.listener.max_packet_size == 0 {
        anyhow::bail!("listener.max_packet_size must be greater than 0");
    }
    if cfg.listener.sync_every_entries == 0 {
        anyhow::bail!("listener.sync_every_entries must be greater than 0");
    }

    cfg.listener
        .listen
        .parse::<SocketAddr>()
        .with_context(|| format!("invalid listener address: {}", cfg.listener.listen))?;

    if !(cfg.protocols.v5
        || cfg.protocols.v7
        || cfg.protocols.v9
        || cfg.protocols.ipfix
        || cfg.protocols.sflow)
    {
        anyhow::bail!("at least one protocol must be enabled");
    }

    Ok(())
}
