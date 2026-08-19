use crate::plugin_config::PluginConfig;
use anyhow::{Context, Result};
use journal_sdk_host::{LoadOptions, LocalJournalProvider};

pub(crate) fn load_local_journal_provider(cfg: &PluginConfig) -> Result<LocalJournalProvider> {
    let state_dir = cfg.journal_host_state_dir();
    LocalJournalProvider::load(local_journal_load_options(cfg, &state_dir)).with_context(|| {
        format!(
            "failed to load local journal host identity using state directory {}",
            state_dir.display()
        )
    })
}

fn local_journal_load_options(cfg: &PluginConfig, state_dir: &std::path::Path) -> LoadOptions {
    let mut options = LoadOptions::default().with_state_dir(state_dir);
    if let Some(prefix) = cfg.journal_host_filesystem_prefix() {
        options = options.with_host_filesystem_prefix(prefix);
    }
    options
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::path::PathBuf;

    #[test]
    fn local_journal_load_options_forwards_host_filesystem_prefix() {
        let state_dir = PathBuf::from("/tmp/netdata-state");
        let mut cfg = PluginConfig::default();
        cfg._netdata_env.host_prefix = Some("/host".to_string());

        let options = local_journal_load_options(&cfg, &state_dir);

        assert_eq!(options.state_dir, Some(state_dir));
        assert_eq!(options.host_filesystem_prefix, Some(PathBuf::from("/host")));
    }

    #[test]
    fn local_journal_load_options_ignores_empty_host_filesystem_prefix() {
        let state_dir = PathBuf::from("/tmp/netdata-state");
        let mut cfg = PluginConfig::default();
        cfg._netdata_env.host_prefix = Some("  ".to_string());

        let options = local_journal_load_options(&cfg, &state_dir);

        assert_eq!(options.state_dir, Some(state_dir));
        assert_eq!(options.host_filesystem_prefix, None);
    }
}
