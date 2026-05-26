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

    /// Per-tier retention. Each tier carries optional size_of_journal_files
    /// and duration_of_journal_files limits. Validation requires each
    /// resolved tier to have at least one positive size or duration limit.
    /// Tiers can be sized independently because raw and rollup tiers have
    /// very different storage costs and access patterns.
    #[arg(skip)]
    #[serde(default)]
    pub(crate) tiers: JournalTierRetentionOverrides,

    /// CLI-only compatibility alias for standalone runs. YAML config remains
    /// per-tier only; this legacy flag applies the same size limit to all tiers.
    #[arg(
        long = "netflow-retention-size-of-journal-files",
        value_parser = parse_bytesize
    )]
    #[serde(skip)]
    pub(crate) cli_retention_size_of_journal_files: Option<ByteSize>,

    /// CLI-only compatibility alias for standalone runs. YAML config remains
    /// per-tier only; this legacy flag applies the same time limit to all tiers.
    #[arg(
        long = "netflow-retention-duration-of-journal-files",
        value_parser = parse_duration
    )]
    #[serde(skip)]
    pub(crate) cli_retention_duration_of_journal_files: Option<Duration>,

    /// Caps the number of distinct group keys a single aggregation
    /// query may build before extra groups are folded into a
    /// synthetic `__overflow__` bucket. Protects the query worker
    /// from accidentally wide group-by combinations exhausting
    /// memory.
    #[arg(long = "netflow-query-max-groups", default_value_t = 50_000)]
    #[serde(default = "default_query_max_groups", alias = "query-max-groups")]
    pub(crate) query_max_groups: usize,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub(crate) struct JournalTierRetentionConfig {
    /// Hard size cap. Unset (`null`) disables the size limit; the tier
    /// is then bounded only by `duration_of_journal_files`. Defaults to
    /// the tier-default size if the field is omitted.
    #[serde(
        default = "default_retention_size_of_journal_files_opt",
        deserialize_with = "deserialize_opt_bytesize",
        serialize_with = "serialize_opt_bytesize"
    )]
    pub(crate) size_of_journal_files: Option<ByteSize>,

    /// Maximum age. Unset (`null`) disables the duration limit; the
    /// tier is then bounded only by `size_of_journal_files`. Defaults
    /// to the tier-default duration if the field is omitted.
    #[serde(
        default = "default_retention_duration_of_journal_files_opt",
        deserialize_with = "deserialize_opt_duration",
        serialize_with = "serialize_opt_duration"
    )]
    pub(crate) duration_of_journal_files: Option<Duration>,
}

impl JournalTierRetentionConfig {
    pub(crate) fn for_tier(_tier: TierKind) -> Self {
        // Tier-uniform defaults today (10GB, 7d). The shape is per-tier
        // so each tier can be tuned independently in user config; the
        // built-in defaults happen to be uniform across tiers but
        // nothing in the schema enforces that.
        Self {
            size_of_journal_files: default_retention_size_of_journal_files_opt(),
            duration_of_journal_files: default_retention_duration_of_journal_files_opt(),
        }
    }
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

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub(crate) struct JournalTierRetentionOverrides {
    #[serde(default = "default_raw_tier")]
    pub(crate) raw: JournalTierRetentionConfig,

    #[serde(
        default = "default_minute_1_tier",
        alias = "1m",
        alias = "minute-1",
        alias = "minute1"
    )]
    pub(crate) minute_1: JournalTierRetentionConfig,

    #[serde(
        default = "default_minute_5_tier",
        alias = "5m",
        alias = "minute-5",
        alias = "minute5"
    )]
    pub(crate) minute_5: JournalTierRetentionConfig,

    #[serde(
        default = "default_hour_1_tier",
        alias = "1h",
        alias = "hour-1",
        alias = "hour1"
    )]
    pub(crate) hour_1: JournalTierRetentionConfig,
}

impl Default for JournalTierRetentionOverrides {
    fn default() -> Self {
        Self {
            raw: JournalTierRetentionConfig::for_tier(TierKind::Raw),
            minute_1: JournalTierRetentionConfig::for_tier(TierKind::Minute1),
            minute_5: JournalTierRetentionConfig::for_tier(TierKind::Minute5),
            hour_1: JournalTierRetentionConfig::for_tier(TierKind::Hour1),
        }
    }
}

impl JournalTierRetentionOverrides {
    pub(crate) fn get(&self, tier: TierKind) -> &JournalTierRetentionConfig {
        match tier {
            TierKind::Raw => &self.raw,
            TierKind::Minute1 => &self.minute_1,
            TierKind::Minute5 => &self.minute_5,
            TierKind::Hour1 => &self.hour_1,
        }
    }
}

fn default_raw_tier() -> JournalTierRetentionConfig {
    JournalTierRetentionConfig::for_tier(TierKind::Raw)
}

fn default_minute_1_tier() -> JournalTierRetentionConfig {
    JournalTierRetentionConfig::for_tier(TierKind::Minute1)
}

fn default_minute_5_tier() -> JournalTierRetentionConfig {
    JournalTierRetentionConfig::for_tier(TierKind::Minute5)
}

fn default_hour_1_tier() -> JournalTierRetentionConfig {
    JournalTierRetentionConfig::for_tier(TierKind::Hour1)
}

impl Default for JournalConfig {
    fn default() -> Self {
        Self {
            journal_dir: "flows".to_string(),
            tiers: JournalTierRetentionOverrides::default(),
            cli_retention_size_of_journal_files: None,
            cli_retention_duration_of_journal_files: None,
            query_max_groups: default_query_max_groups(),
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

    pub(crate) fn retention_for_tier(&self, tier: TierKind) -> ResolvedJournalTierRetention {
        let cfg = self.tiers.get(tier);
        ResolvedJournalTierRetention {
            size_of_journal_files: self
                .cli_retention_size_of_journal_files
                .or(cfg.size_of_journal_files),
            duration_of_journal_files: self
                .cli_retention_duration_of_journal_files
                .or(cfg.duration_of_journal_files),
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
