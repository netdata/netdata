use super::*;

#[derive(Debug, Parser, Clone, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub(crate) struct JournalConfig {
    #[arg(long = "netflow-journal-dir", default_value = "flows")]
    pub(crate) journal_dir: String,

    #[arg(
        long = "netflow-rotation-size-of-journal-file",
        default_value = "256MB",
        value_parser = parse_bytesize
    )]
    #[serde(with = "bytesize_serde")]
    pub(crate) size_of_journal_file: ByteSize,

    #[arg(
        long = "netflow-rotation-duration-of-journal-file",
        default_value = "1h",
        value_parser = parse_duration
    )]
    #[serde(with = "humantime_serde")]
    pub(crate) duration_of_journal_file: Duration,

    #[arg(
        long = "netflow-retention-number-of-journal-files",
        default_value_t = 64
    )]
    pub(crate) number_of_journal_files: usize,

    #[arg(
        long = "netflow-retention-size-of-journal-files",
        default_value = "10GB",
        value_parser = parse_bytesize
    )]
    #[serde(with = "bytesize_serde")]
    pub(crate) size_of_journal_files: ByteSize,

    #[arg(
        long = "netflow-retention-duration-of-journal-files",
        default_value = "7d",
        value_parser = parse_duration
    )]
    #[serde(with = "humantime_serde")]
    pub(crate) duration_of_journal_files: Duration,

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

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub(crate) struct JournalTierRetentionConfig {
    #[serde(default = "default_retention_number_of_journal_files")]
    pub(crate) number_of_journal_files: usize,

    #[serde(
        default = "default_retention_size_of_journal_files",
        with = "bytesize_serde"
    )]
    pub(crate) size_of_journal_files: ByteSize,

    #[serde(
        default = "default_retention_duration_of_journal_files",
        with = "humantime_serde"
    )]
    pub(crate) duration_of_journal_files: Duration,
}

impl Default for JournalTierRetentionConfig {
    fn default() -> Self {
        Self {
            number_of_journal_files: default_retention_number_of_journal_files(),
            size_of_journal_files: default_retention_size_of_journal_files(),
            duration_of_journal_files: default_retention_duration_of_journal_files(),
        }
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
            size_of_journal_file: ByteSize::mb(256),
            duration_of_journal_file: Duration::from_secs(60 * 60),
            number_of_journal_files: default_retention_number_of_journal_files(),
            size_of_journal_files: default_retention_size_of_journal_files(),
            duration_of_journal_files: default_retention_duration_of_journal_files(),
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

    pub(crate) fn default_retention(&self) -> JournalTierRetentionConfig {
        JournalTierRetentionConfig {
            number_of_journal_files: self.number_of_journal_files,
            size_of_journal_files: self.size_of_journal_files,
            duration_of_journal_files: self.duration_of_journal_files,
        }
    }

    pub(crate) fn retention_for_tier(&self, tier: TierKind) -> JournalTierRetentionConfig {
        self.tiers
            .get(tier)
            .cloned()
            .unwrap_or_else(|| self.default_retention())
    }

    pub(crate) fn decoder_state_dir(&self) -> PathBuf {
        self.base_dir().join("decoder-state.d")
    }
}
