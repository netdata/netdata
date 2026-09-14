//! Configuration types and management for metric processing.

use std::collections::HashMap;
use std::sync::Arc;
use std::time::Duration;

use anyhow::{Context, Result};
use opentelemetry_proto::tonic::common::v1::InstrumentationScope;
use regex::Regex;
use serde::{Deserialize, Serialize};

use crate::chart::ChartConfig;
use crate::iter::MetricRef;

/// Pattern matching for instrumentation scope fields
#[derive(Debug, Clone, Default, Serialize, Deserialize)]
pub struct InstrumentationScopePattern {
    #[serde(with = "serde_regex", skip_serializing_if = "Option::is_none", default)]
    pub name: Option<Regex>,

    #[serde(with = "serde_regex", skip_serializing_if = "Option::is_none", default)]
    pub version: Option<Regex>,
}

impl InstrumentationScopePattern {
    /// Check if this pattern matches the given instrumentation scope
    pub fn matches(&self, scope: Option<&InstrumentationScope>) -> bool {
        let scope = match scope {
            Some(s) => s,
            None => return self.name.is_none() && self.version.is_none(),
        };

        if let Some(r) = &self.name {
            if !r.is_match(&scope.name) {
                return false;
            }
        }

        if let Some(r) = &self.version {
            if !r.is_match(&scope.version) {
                return false;
            }
        }

        true
    }
}

/// Individual configuration for a metric under specific instrumentation scope
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct MetricConfig {
    #[serde(skip_serializing_if = "Option::is_none", default)]
    pub instrumentation_scope: Option<InstrumentationScopePattern>,

    /// The attribute key in DataPoint attributes whose value becomes the dimension name
    #[serde(skip_serializing_if = "Option::is_none", default)]
    pub dimension_attribute_key: Option<String>,

    /// Per-metric collection interval override (seconds).
    #[serde(skip_serializing_if = "Option::is_none", default)]
    pub interval_secs: Option<u64>,

    /// Per-metric grace period override (seconds).
    #[serde(skip_serializing_if = "Option::is_none", default)]
    pub grace_period_secs: Option<u64>,
}

impl MetricConfig {
    /// Check if this config matches the given instrumentation scope
    pub fn matches_scope(&self, scope: Option<&InstrumentationScope>) -> bool {
        match &self.instrumentation_scope {
            Some(pattern) => pattern.matches(scope),
            None => true, // No pattern means match any scope
        }
    }
}

/// Type alias for the config storage: metric name -> list of Arc-wrapped configs
pub type ConfigMap = HashMap<String, Vec<Arc<MetricConfig>>>;

/// Root configuration structure for YAML deserialization of per-metric mapping files.
#[derive(Debug, Clone, Default, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct MetricConfigs {
    /// Map from exact metric name to list of configurations
    #[serde(default)]
    pub metrics: ConfigMap,
}

/// Old-format configuration structure (for detection only).
#[derive(Deserialize)]
struct OldChartConfigs {
    #[allow(dead_code)]
    configs: Vec<serde_yaml::Value>,
}

#[derive(Debug, Default, Clone)]
pub struct ChartConfigManager {
    /// Stock configs wrapped in Arc for cheap cloning
    stock: Arc<ConfigMap>,
    /// User configs wrapped in Arc for cheap cloning
    user: Arc<ConfigMap>,
    /// Global timing defaults (from otel.yaml)
    defaults: ChartConfig,
    /// Whether expiry was explicitly set in the global defaults.
    expiry_explicit: bool,
}

impl ChartConfigManager {
    pub fn with_default_configs() -> Self {
        let mut manager = Self::default();
        manager.load_stock_config();
        manager
    }

    /// Set global timing defaults from plugin configuration (otel.yaml).
    ///
    /// When `interval_secs` is set but `grace_period_secs` is not, the grace
    /// period auto-derives as `5 * interval`. When the resulting grace exceeds
    /// expiry and expiry was not explicitly set, expiry is bumped to match.
    pub fn set_defaults(
        &mut self,
        interval_secs: Option<u64>,
        grace_period_secs: Option<u64>,
        expiry_duration_secs: Option<u64>,
    ) {
        if let Some(interval) = interval_secs {
            self.defaults.collection_interval = interval;
            self.defaults.grace_period = Duration::from_secs(5 * interval);
        }
        if let Some(grace) = grace_period_secs {
            self.defaults.grace_period = Duration::from_secs(grace);
        }
        if let Some(expiry) = expiry_duration_secs {
            self.defaults.expiry_duration = Duration::from_secs(expiry);
            self.expiry_explicit = true;
        }

        // Auto-bump expiry when grace exceeds it and expiry wasn't explicit.
        if !self.expiry_explicit && self.defaults.grace_period > self.defaults.expiry_duration {
            self.defaults.expiry_duration = self.defaults.grace_period;
        }
    }

    /// Resolve a `ChartConfig` by layering: global defaults -> per-metric overrides.
    ///
    /// When a per-metric config sets `interval_secs` but not `grace_period_secs`,
    /// the grace period derives from the per-metric interval (`5 * interval`),
    /// not from the global grace default.
    ///
    /// When the auto-derived grace exceeds expiry and expiry was not explicitly
    /// set, expiry is bumped to match grace so that a simple `interval_secs`
    /// override doesn't silently fall back to defaults.
    ///
    /// The resolved config must satisfy `0 < interval <= 3600` and
    /// `interval < grace <= expiry`.  If violated, the hardcoded defaults
    /// are used and a warning is logged.
    pub fn resolve_chart_config(&self, metric_config: Option<&MetricConfig>) -> ChartConfig {
        let mut cfg = self.defaults;
        let expiry_explicit = self.expiry_explicit;

        // Per-metric overrides
        if let Some(mc) = metric_config {
            if let Some(interval) = mc.interval_secs {
                cfg.collection_interval = interval;
                // Re-derive grace period from the per-metric interval,
                // unless the per-metric config also sets it explicitly.
                if mc.grace_period_secs.is_none() {
                    cfg.grace_period = Duration::from_secs(5 * interval);
                }
            }
            if let Some(grace) = mc.grace_period_secs {
                cfg.grace_period = Duration::from_secs(grace);
            }
        }

        // If grace was auto-derived and exceeds expiry, bump expiry to match
        // — but only if expiry was never explicitly configured.
        if !expiry_explicit && cfg.grace_period > cfg.expiry_duration {
            cfg.expiry_duration = cfg.grace_period;
        }

        // Validate: 0 < interval <= MAX_UPDATE_EVERY, interval < grace <= expiry
        const MAX_UPDATE_EVERY: u64 = 3600;
        let interval = Duration::from_secs(cfg.collection_interval);
        if cfg.collection_interval == 0
            || cfg.collection_interval > MAX_UPDATE_EVERY
            || interval >= cfg.grace_period
            || cfg.grace_period > cfg.expiry_duration
        {
            tracing::warn!(
                "invalid chart timing config: interval={}s, grace={}s, expiry={}s \
                 (must satisfy 0 < interval <= {}s and interval < grace <= expiry) - \
                 falling back to defaults",
                cfg.collection_interval,
                cfg.grace_period.as_secs(),
                cfg.expiry_duration.as_secs(),
                MAX_UPDATE_EVERY,
            );
            return ChartConfig::default();
        }

        cfg
    }

    /// Find matching config for a metric. Returns Arc<MetricConfig> for zero-copy access.
    pub fn find_matching_config(&self, m: &MetricRef<'_>) -> Option<Arc<MetricConfig>> {
        let scope = m.scope_metrics.scope.as_ref();

        // Check user configs first (priority)
        if let Some(configs) = self.user.get(&m.metric.name) {
            if let Some(cfg) = configs.iter().find(|c| c.matches_scope(scope)) {
                return Some(Arc::clone(cfg));
            }
        }

        // Fall back to stock configs
        if let Some(configs) = self.stock.get(&m.metric.name) {
            if let Some(cfg) = configs.iter().find(|c| c.matches_scope(scope)) {
                return Some(Arc::clone(cfg));
            }
        }

        None
    }

    fn load_stock_config(&mut self) {
        const DEFAULT_CONFIGS_YAML: &str =
            include_str!("../configs/otel.d/v1/metrics/hostmetrics-receiver.yaml");

        match serde_yaml::from_str::<MetricConfigs>(DEFAULT_CONFIGS_YAML) {
            Ok(configs) => {
                self.stock = Arc::new(configs.metrics);
            }
            Err(e) => {
                tracing::warn!("failed to parse default configs YAML: {}", e);
            }
        }
    }

    pub fn load_user_configs<P: AsRef<std::path::Path>>(&mut self, config_dir: P) -> Result<()> {
        let config_path = config_dir.as_ref();
        if !config_path.exists() {
            return Err(anyhow::anyhow!(
                "Configuration directory does not exist: {}",
                config_path.display()
            ));
        }
        if !config_path.is_dir() {
            return Err(anyhow::anyhow!(
                "Configuration path is not a directory: {}",
                config_path.display()
            ));
        }

        // Collect YAML files sorted alphabetically
        let mut config_files: Vec<_> = std::fs::read_dir(config_path)
            .with_context(|| {
                format!(
                    "failed to read chart config directory: {}",
                    config_path.display()
                )
            })?
            .filter_map(|entry| {
                let entry = entry.ok()?;
                let path = entry.path();
                if path.is_file()
                    && matches!(
                        path.extension().and_then(|s| s.to_str()),
                        Some("yml" | "yaml")
                    )
                {
                    Some(path)
                } else {
                    None
                }
            })
            .collect();
        config_files.sort();

        let mut accumulated = ConfigMap::new();

        for path in config_files {
            let contents = match std::fs::read_to_string(&path) {
                Ok(c) => c,
                Err(e) => {
                    tracing::error!("failed to read file {}: {}", path.display(), e);
                    continue;
                }
            };

            // Try new format first
            match serde_yaml::from_str::<MetricConfigs>(&contents) {
                Ok(metric_configs) => {
                    for (metric_name, configs) in metric_configs.metrics {
                        accumulated.entry(metric_name).or_default().extend(configs);
                    }
                }
                Err(new_format_err) => {
                    // Try to detect old format
                    if serde_yaml::from_str::<OldChartConfigs>(&contents).is_ok() {
                        tracing::error!(
                            "Old metrics configuration format detected in {}. \
                             Ignoring file; falling back to stock configuration. \
                             Please migrate to the new format.",
                            path.display()
                        );
                    } else {
                        tracing::error!(
                            "failed to parse config file {}: {}",
                            path.display(),
                            new_format_err
                        );
                    }
                }
            }
        }

        self.user = Arc::new(accumulated);
        Ok(())
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::chart::ChartConfig;

    fn defaults() -> ChartConfig {
        ChartConfig::default()
    }

    /// Helper: create a manager with specific global defaults.
    fn manager_with_defaults(
        interval: Option<u64>,
        grace: Option<u64>,
        expiry: Option<u64>,
    ) -> ChartConfigManager {
        let mut m = ChartConfigManager::default();
        m.set_defaults(interval, grace, expiry);
        m
    }

    /// Helper: create a MetricConfig with only timing overrides.
    fn metric_config(interval: Option<u64>, grace: Option<u64>) -> MetricConfig {
        MetricConfig {
            instrumentation_scope: None,
            dimension_attribute_key: None,
            interval_secs: interval,
            grace_period_secs: grace,
        }
    }

    mod timing_validation {
        use super::*;

        #[test]
        fn valid_defaults_are_accepted() {
            let m = ChartConfigManager::default();
            let cfg = m.resolve_chart_config(None);
            // Default: interval=10, grace=50, expiry=900
            assert_eq!(cfg.collection_interval, defaults().collection_interval);
            assert_eq!(cfg.grace_period, defaults().grace_period);
            assert_eq!(cfg.expiry_duration, defaults().expiry_duration);
        }

        #[test]
        fn grace_equal_to_expiry_is_valid() {
            let m = manager_with_defaults(Some(10), Some(100), Some(100));
            let cfg = m.resolve_chart_config(None);
            assert_eq!(cfg.collection_interval, 10);
            assert_eq!(cfg.grace_period, Duration::from_secs(100));
            assert_eq!(cfg.expiry_duration, Duration::from_secs(100));
        }

        #[test]
        fn grace_greater_than_expiry_falls_back_to_defaults() {
            let m = manager_with_defaults(Some(10), Some(200), Some(100));
            let cfg = m.resolve_chart_config(None);
            assert_eq!(cfg.collection_interval, defaults().collection_interval);
            assert_eq!(cfg.grace_period, defaults().grace_period);
            assert_eq!(cfg.expiry_duration, defaults().expiry_duration);
        }

        #[test]
        fn interval_equal_to_grace_falls_back_to_defaults() {
            let m = manager_with_defaults(Some(50), Some(50), Some(900));
            let cfg = m.resolve_chart_config(None);
            assert_eq!(cfg.collection_interval, defaults().collection_interval);
        }

        #[test]
        fn interval_greater_than_grace_falls_back_to_defaults() {
            let m = manager_with_defaults(Some(100), Some(50), Some(900));
            let cfg = m.resolve_chart_config(None);
            assert_eq!(cfg.collection_interval, defaults().collection_interval);
        }

        #[test]
        fn per_metric_interval_exceeding_global_grace() {
            // Global defaults set grace=20, expiry=900.
            // Per-metric sets interval=25, which auto-derives grace=125.
            // That's valid (25 < 125 <= 900), so it should be accepted.
            let m = manager_with_defaults(None, Some(20), Some(900));
            let mc = metric_config(Some(25), None);
            let cfg = m.resolve_chart_config(Some(&mc));
            // grace is re-derived as 5*25=125
            assert_eq!(cfg.collection_interval, 25);
            assert_eq!(cfg.grace_period, Duration::from_secs(125));
        }

        #[test]
        fn per_metric_grace_less_than_interval_falls_back_to_defaults() {
            let m = ChartConfigManager::default();
            let mc = metric_config(Some(100), Some(50));
            let cfg = m.resolve_chart_config(Some(&mc));
            assert_eq!(cfg.collection_interval, defaults().collection_interval);
        }

        #[test]
        fn per_metric_grace_exceeding_expiry_falls_back_to_defaults() {
            let m = manager_with_defaults(None, None, Some(100));
            let mc = metric_config(Some(10), Some(200));
            let cfg = m.resolve_chart_config(Some(&mc));
            assert_eq!(cfg.collection_interval, defaults().collection_interval);
        }

        #[test]
        fn zero_interval_falls_back_to_defaults() {
            let m = manager_with_defaults(Some(0), Some(50), Some(900));
            let cfg = m.resolve_chart_config(None);
            assert_eq!(cfg.collection_interval, defaults().collection_interval);
        }

        #[test]
        fn zero_expiry_falls_back_to_defaults() {
            let m = manager_with_defaults(Some(10), Some(0), Some(0));
            let cfg = m.resolve_chart_config(None);
            assert_eq!(cfg.collection_interval, defaults().collection_interval);
        }

        #[test]
        fn large_interval_auto_derives_grace_and_bumps_expiry() {
            // Global sets only interval=200. Grace auto-derives to 1000,
            // which exceeds the default expiry of 900. Since expiry was
            // never explicitly set, it should be bumped to 1000.
            let m = manager_with_defaults(Some(200), None, None);
            let cfg = m.resolve_chart_config(None);
            assert_eq!(cfg.collection_interval, 200);
            assert_eq!(cfg.grace_period, Duration::from_secs(1000));
            assert_eq!(cfg.expiry_duration, Duration::from_secs(1000));
        }

        #[test]
        fn per_metric_large_interval_bumps_expiry() {
            // No global overrides. Per-metric sets interval=200.
            // Grace auto-derives to 1000, default expiry is 900.
            // Expiry not explicit → bumped to 1000.
            let m = ChartConfigManager::default();
            let mc = metric_config(Some(200), None);
            let cfg = m.resolve_chart_config(Some(&mc));
            assert_eq!(cfg.collection_interval, 200);
            assert_eq!(cfg.grace_period, Duration::from_secs(1000));
            assert_eq!(cfg.expiry_duration, Duration::from_secs(1000));
        }

        #[test]
        fn explicit_expiry_not_bumped_when_grace_exceeds_it() {
            // Global explicitly sets expiry=100. Per-metric sets
            // grace=200 which exceeds it. Since expiry is explicit,
            // it should NOT be bumped — validation fails → fallback.
            let m = manager_with_defaults(None, None, Some(100));
            let mc = metric_config(Some(10), Some(200));
            let cfg = m.resolve_chart_config(Some(&mc));
            assert_eq!(cfg.collection_interval, defaults().collection_interval);
        }

        #[test]
        fn interval_at_max_update_every_is_accepted() {
            let m = manager_with_defaults(Some(3600), None, None);
            let cfg = m.resolve_chart_config(None);
            assert_eq!(cfg.collection_interval, 3600);
            assert_eq!(cfg.grace_period, Duration::from_secs(18000));
            assert_eq!(cfg.expiry_duration, Duration::from_secs(18000));
        }

        #[test]
        fn interval_exceeding_max_update_every_falls_back_to_defaults() {
            let m = manager_with_defaults(Some(3601), None, None);
            let cfg = m.resolve_chart_config(None);
            assert_eq!(cfg.collection_interval, defaults().collection_interval);
        }

        #[test]
        fn interval_30_auto_derives_correctly() {
            // Interval=30 → grace=150, default expiry=900.
            // All valid: 30 < 150 <= 900.
            let m = manager_with_defaults(Some(30), None, None);
            let cfg = m.resolve_chart_config(None);
            assert_eq!(cfg.collection_interval, 30);
            assert_eq!(cfg.grace_period, Duration::from_secs(150));
            assert_eq!(cfg.expiry_duration, Duration::from_secs(900));
        }
    }
}
