use crate::plugin_config::{PluginConfig, ResolvedJournalTierRetention};
use crate::tiering::TierKind;
use anyhow::Result;

pub(super) fn validate_journal(cfg: &PluginConfig) -> Result<()> {
    if cfg.journal.query_max_groups == 0 {
        anyhow::bail!("journal.query_max_groups must be greater than 0");
    }

    for (scope, tier) in [
        ("journal.tiers.raw", TierKind::Raw),
        ("journal.tiers.minute_1", TierKind::Minute1),
        ("journal.tiers.minute_5", TierKind::Minute5),
        ("journal.tiers.hour_1", TierKind::Hour1),
    ] {
        validate_retention(
            scope,
            &cfg.journal.retention_for_tier(tier),
            cfg.journal.minimum_retention_size_of_journal_files(),
        )?;
    }

    Ok(())
}

fn validate_retention(
    scope: &str,
    retention: &ResolvedJournalTierRetention,
    minimum_size_bytes: u64,
) -> Result<()> {
    if !retention.has_limits() {
        anyhow::bail!(
            "{scope} must define at least one of size_of_journal_files or duration_of_journal_files"
        );
    }

    if let Some(size_of_journal_files) = retention.size_of_journal_files {
        if size_of_journal_files.as_u64() == 0 {
            anyhow::bail!("{scope}.size_of_journal_files must be greater than 0");
        }
        if size_of_journal_files.as_u64() < minimum_size_bytes {
            anyhow::bail!(
                "{scope}.size_of_journal_files must be at least {}MB",
                minimum_size_bytes / 1_000_000
            );
        }
    }

    if let Some(duration_of_journal_files) = retention.duration_of_journal_files
        && duration_of_journal_files.is_zero()
    {
        anyhow::bail!("{scope}.duration_of_journal_files must be greater than 0");
    }
    Ok(())
}
