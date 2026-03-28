use crate::plugin_config::{JournalTierRetentionConfig, PluginConfig};
use anyhow::Result;

pub(super) fn validate_journal(cfg: &PluginConfig) -> Result<()> {
    if cfg.journal.query_1m_max_window.is_zero() {
        anyhow::bail!("journal.query_1m_max_window must be greater than 0");
    }
    if cfg.journal.query_5m_max_window.is_zero() {
        anyhow::bail!("journal.query_5m_max_window must be greater than 0");
    }
    if cfg.journal.query_5m_max_window < cfg.journal.query_1m_max_window {
        anyhow::bail!("journal.query_5m_max_window must be >= journal.query_1m_max_window");
    }
    if cfg.journal.query_max_groups == 0 {
        anyhow::bail!("journal.query_max_groups must be greater than 0");
    }
    if cfg.journal.query_facet_max_values_per_field == 0 {
        anyhow::bail!("journal.query_facet_max_values_per_field must be greater than 0");
    }

    let default_retention = cfg.journal.default_retention();
    validate_retention("journal", &default_retention)?;
    if let Some(raw) = &cfg.journal.tiers.raw {
        validate_retention("journal.tiers.raw", raw)?;
    }
    if let Some(minute_1) = &cfg.journal.tiers.minute_1 {
        validate_retention("journal.tiers.minute_1", minute_1)?;
    }
    if let Some(minute_5) = &cfg.journal.tiers.minute_5 {
        validate_retention("journal.tiers.minute_5", minute_5)?;
    }
    if let Some(hour_1) = &cfg.journal.tiers.hour_1 {
        validate_retention("journal.tiers.hour_1", hour_1)?;
    }

    Ok(())
}

fn validate_retention(scope: &str, retention: &JournalTierRetentionConfig) -> Result<()> {
    if retention.number_of_journal_files == 0 {
        anyhow::bail!("{scope}.number_of_journal_files must be greater than 0");
    }
    if retention.size_of_journal_files.as_u64() == 0 {
        anyhow::bail!("{scope}.size_of_journal_files must be greater than 0");
    }
    if retention.duration_of_journal_files.is_zero() {
        anyhow::bail!("{scope}.duration_of_journal_files must be greater than 0");
    }
    Ok(())
}
