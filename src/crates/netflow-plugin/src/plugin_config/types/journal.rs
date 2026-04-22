use super::*;

const MIN_TIER_RETENTION_SIZE_BYTES: u64 = 100_000_000;
const MIN_ROTATION_SIZE_BYTES: u64 = 5_000_000;
const DEFAULT_TIME_ONLY_ROTATION_SIZE_BYTES: u64 = 100_000_000;
const MAX_ROTATION_SIZE_BYTES: u64 = 200_000_000;
const ROTATION_SIZE_DIVISOR: u64 = 20;

#[derive(Debug, Parser, Clone, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub(crate) struct JournalConfig {
    #[arg(long = "netflow-journal-dir", default_value = "flows")]
    pub(crate) journal_dir: String,

    #[arg(
        long = "netflow-retention-size-of-journal-files",
        default_value = "10GB",
        value_parser = parse_bytesize
    )]
    #[serde(
        default = "default_retention_size_of_journal_files_opt",
        deserialize_with = "deserialize_opt_bytesize",
        serialize_with = "serialize_opt_bytesize"
    )]
    pub(crate) size_of_journal_files: Option<ByteSize>,

    #[arg(
        long = "netflow-retention-duration-of-journal-files",
        default_value = "7d",
        value_parser = parse_duration
    )]
    #[serde(
        default = "default_retention_duration_of_journal_files_opt",
        deserialize_with = "deserialize_opt_duration",
        serialize_with = "serialize_opt_duration"
    )]
    pub(crate) duration_of_journal_files: Option<Duration>,

    #[arg(skip)]
    #[serde(default)]
    pub(crate) tiers: JournalTierRetentionOverrides,

    #[arg(
        long = "netflow-query-1m-max-window",
        default_value = "6h",
        value_parser = parse_duration
    )]
    #[serde(with = "humantime_serde")]
    pub(crate) query_1m_max_window: Duration,

    #[arg(
        long = "netflow-query-5m-max-window",
        default_value = "24h",
        value_parser = parse_duration
    )]
    #[serde(with = "humantime_serde")]
    pub(crate) query_5m_max_window: Duration,

    #[arg(long = "netflow-query-max-groups", default_value_t = 50_000)]
    #[serde(default = "default_query_max_groups", alias = "query-max-groups")]
    pub(crate) query_max_groups: usize,

    #[arg(
        long = "netflow-query-facet-max-values-per-field",
        default_value_t = 5_000
    )]
    #[serde(
        default = "default_query_facet_max_values_per_field",
        alias = "query-facet-max-values-per-field"
    )]
    pub(crate) query_facet_max_values_per_field: usize,
}

#[derive(Debug, Clone, Default, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub(crate) struct JournalTierRetentionConfig {
    #[serde(
        default,
        deserialize_with = "deserialize_retention_override_bytesize",
        serialize_with = "serialize_retention_override_bytesize",
        skip_serializing_if = "RetentionLimitOverride::is_inherit"
    )]
    pub(crate) size_of_journal_files: RetentionLimitOverride<ByteSize>,

    #[serde(
        default,
        deserialize_with = "deserialize_retention_override_duration",
        serialize_with = "serialize_retention_override_duration",
        skip_serializing_if = "RetentionLimitOverride::is_inherit"
    )]
    pub(crate) duration_of_journal_files: RetentionLimitOverride<Duration>,
}

#[derive(Debug, Clone, Default, PartialEq, Eq)]
pub(crate) struct ResolvedJournalTierRetention {
    pub(crate) size_of_journal_files: Option<ByteSize>,
    pub(crate) duration_of_journal_files: Option<Duration>,
}

impl ResolvedJournalTierRetention {
    pub(crate) fn has_limits(&self) -> bool {
        self.size_of_journal_files.is_some() || self.duration_of_journal_files.is_some()
    }
}

#[derive(Debug, Clone, Default, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub(crate) struct JournalTierRetentionOverrides {
    #[serde(default)]
    pub(crate) raw: Option<JournalTierRetentionConfig>,

    #[serde(default, alias = "1m", alias = "minute-1", alias = "minute1")]
    pub(crate) minute_1: Option<JournalTierRetentionConfig>,

    #[serde(default, alias = "5m", alias = "minute-5", alias = "minute5")]
    pub(crate) minute_5: Option<JournalTierRetentionConfig>,

    #[serde(default, alias = "1h", alias = "hour-1", alias = "hour1")]
    pub(crate) hour_1: Option<JournalTierRetentionConfig>,
}

impl JournalTierRetentionOverrides {
    pub(crate) fn get(&self, tier: TierKind) -> Option<&JournalTierRetentionConfig> {
        match tier {
            TierKind::Raw => self.raw.as_ref(),
            TierKind::Minute1 => self.minute_1.as_ref(),
            TierKind::Minute5 => self.minute_5.as_ref(),
            TierKind::Hour1 => self.hour_1.as_ref(),
        }
    }
}

impl Default for JournalConfig {
    fn default() -> Self {
        Self {
            journal_dir: "flows".to_string(),
            size_of_journal_files: default_retention_size_of_journal_files_opt(),
            duration_of_journal_files: default_retention_duration_of_journal_files_opt(),
            tiers: JournalTierRetentionOverrides::default(),
            query_1m_max_window: Duration::from_secs(6 * 60 * 60),
            query_5m_max_window: Duration::from_secs(24 * 60 * 60),
            query_max_groups: default_query_max_groups(),
            query_facet_max_values_per_field: default_query_facet_max_values_per_field(),
        }
    }
}

impl JournalConfig {
    pub(crate) fn base_dir(&self) -> PathBuf {
        PathBuf::from(&self.journal_dir)
    }

    pub(crate) fn tier_dir(&self, tier: TierKind) -> PathBuf {
        self.base_dir().join(tier.dir_name())
    }

    pub(crate) fn raw_tier_dir(&self) -> PathBuf {
        self.tier_dir(TierKind::Raw)
    }

    pub(crate) fn minute_1_tier_dir(&self) -> PathBuf {
        self.tier_dir(TierKind::Minute1)
    }

    pub(crate) fn minute_5_tier_dir(&self) -> PathBuf {
        self.tier_dir(TierKind::Minute5)
    }

    pub(crate) fn hour_1_tier_dir(&self) -> PathBuf {
        self.tier_dir(TierKind::Hour1)
    }

    pub(crate) fn all_tier_dirs(&self) -> [PathBuf; 4] {
        [
            self.raw_tier_dir(),
            self.minute_1_tier_dir(),
            self.minute_5_tier_dir(),
            self.hour_1_tier_dir(),
        ]
    }

    pub(crate) fn default_retention(&self) -> ResolvedJournalTierRetention {
        ResolvedJournalTierRetention {
            size_of_journal_files: self.size_of_journal_files,
            duration_of_journal_files: self.duration_of_journal_files,
        }
    }

    pub(crate) fn retention_for_tier(&self, tier: TierKind) -> ResolvedJournalTierRetention {
        let defaults = self.default_retention();
        let Some(overrides) = self.tiers.get(tier) else {
            return defaults;
        };

        ResolvedJournalTierRetention {
            size_of_journal_files: overrides
                .size_of_journal_files
                .resolve(defaults.size_of_journal_files),
            duration_of_journal_files: overrides
                .duration_of_journal_files
                .resolve(defaults.duration_of_journal_files),
        }
    }

    pub(crate) fn rotation_size_for_tier(&self, tier: TierKind) -> u64 {
        match self.retention_for_tier(tier).size_of_journal_files {
            Some(size_of_journal_files) => size_of_journal_files
                .as_u64()
                .saturating_div(ROTATION_SIZE_DIVISOR)
                .clamp(MIN_ROTATION_SIZE_BYTES, MAX_ROTATION_SIZE_BYTES),
            None => DEFAULT_TIME_ONLY_ROTATION_SIZE_BYTES,
        }
    }

    pub(crate) fn rotation_duration_of_journal_file(&self) -> Duration {
        default_rotation_duration_of_journal_file()
    }

    pub(crate) fn minimum_retention_size_of_journal_files(&self) -> u64 {
        MIN_TIER_RETENTION_SIZE_BYTES
    }

    pub(crate) fn decoder_state_dir(&self) -> PathBuf {
        self.base_dir().join("decoder-state.d")
    }
}
