use crate::plugin_config::PluginConfig;
use anyhow::{Context, Result};
use std::collections::HashSet;
use std::net::SocketAddr;

pub(super) fn validate_listener_and_protocols(cfg: &PluginConfig) -> Result<()> {
    if cfg.listener.max_packet_size == 0 {
        anyhow::bail!("listener.max_packet_size must be greater than 0");
    }

    if cfg.listener.listen.is_empty() {
        anyhow::bail!("listener.listen must contain at least one address");
    }

    let mut seen = HashSet::new();
    for listen in &cfg.listener.listen {
        let addr = listen
            .parse::<SocketAddr>()
            .with_context(|| format!("invalid listener address: {}", listen))?;
        if !seen.insert(addr) {
            anyhow::bail!("listener.listen contains duplicate address: {}", addr);
        }
    }

    if !(cfg.protocols.v5
        || cfg.protocols.v7
        || cfg.protocols.v9
        || cfg.protocols.ipfix
        || cfg.protocols.sflow)
    {
        anyhow::bail!("at least one protocol must be enabled");
    }

    if cfg
        .protocols
        .v9_template_lifetime
        .get()
        .is_some_and(|lifetime| lifetime.is_zero())
    {
        anyhow::bail!("protocols.v9_template_lifetime must be greater than 0 or null");
    }
    if cfg.protocols.sampling_cache_max_entries == 0 {
        anyhow::bail!("protocols.sampling_cache_max_entries must be greater than 0");
    }
    if cfg.protocols.sampling_cache_max_entries_per_stream == 0 {
        anyhow::bail!("protocols.sampling_cache_max_entries_per_stream must be greater than 0");
    }

    Ok(())
}
