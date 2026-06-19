use crate::plugin_config::PluginConfig;
use anyhow::{Context, Result};
use journal_host::{LoadOptions, LocalJournalProvider};

pub(crate) fn load_local_journal_provider(cfg: &PluginConfig) -> Result<LocalJournalProvider> {
    let state_dir = cfg.journal_host_state_dir();
    LocalJournalProvider::load(LoadOptions::default().with_state_dir(&state_dir)).with_context(
        || {
            format!(
                "failed to load local journal host identity using state directory {}",
                state_dir.display()
            )
        },
    )
}
