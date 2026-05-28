use super::PluginConfig;
use anyhow::Result;

mod enrichment;
mod journal;
mod listener;

impl PluginConfig {
    pub(super) fn validate(&self) -> Result<()> {
        listener::validate_listener_and_protocols(self)?;
        journal::validate_journal(self)?;
        enrichment::validate_enrichment(self)?;
        Ok(())
    }
}
