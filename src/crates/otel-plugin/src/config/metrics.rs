//! The `metrics:` override section of the plugin's layered configuration:
//! the partial (all-`Option`) mirror of [`bridge::config::MetricsConfig`]
//! used to patch the effective metrics config from the user `otel.yaml` and
//! the `NETDATA_OTEL_CFG_METRICS_*` environment variables. Layering lives in
//! `mod.rs` (stock file → user file → env; later layers win per field); this
//! module serves it (`mod.rs`) and the env parser (`env.rs`) — hence
//! `pub(super)` throughout.
//!
//! Nothing defaults or validates here. Defaults come from the shipped stock
//! `otel.yaml` (`configs/otel.yaml.in`), which must carry the whole section
//! because `PluginConfig.metrics` has no serde default and only the stock
//! file is parsed directly into [`bridge::config::MetricsConfig`]. Semantic
//! validation of the resulting timing happens downstream in the ingestor's
//! `ChartConfigManager` (`otel-ingestor/src/chart_config.rs`), the consumer
//! of the resolved config sent to workers over IPC.
use bridge::config::MetricsConfig;
use serde::Deserialize;

/// The user-layer `metrics:` section: every field optional, with `None`
/// meaning "keep the value from the previous layer". Field names and units
/// mirror [`bridge::config::MetricsConfig`]; unknown keys are rejected, not
/// ignored.
#[derive(Debug, Default, Deserialize)]
#[serde(deny_unknown_fields)]
pub(super) struct MetricsOverride {
    /// Directory of per-metric chart config files the ingestor loads; a load
    /// failure is logged and the stock configs are kept (non-fatal).
    #[serde(default)]
    pub(super) chart_configs_dir: Option<String>,
    /// Chart update interval in seconds. Also derives the grace period
    /// (`5 * interval`) whenever no grace period is set.
    #[serde(default)]
    pub(super) interval_secs: Option<u64>,
    /// Seconds to wait after a metric's last data point before gap-filling.
    #[serde(default)]
    pub(super) grace_period_secs: Option<u64>,
    /// Seconds without data before an inactive chart is removed.
    #[serde(default)]
    pub(super) expiry_duration_secs: Option<u64>,
    /// New charts allowed per export request — the cardinality guard. The
    /// only non-`Option` field in [`MetricsConfig`], so the stock file must
    /// always provide it; override layers can only replace the value.
    #[serde(default)]
    pub(super) max_new_charts_per_request: Option<usize>,
}

impl MetricsOverride {
    /// Whether any field is set. The env layer (`env.rs`) collapses an
    /// all-empty override to `None` instead of storing it; on the YAML path
    /// an empty `metrics:` section stays `Some` and is a harmless no-op.
    pub(super) fn has_any(&self) -> bool {
        self.chart_configs_dir.is_some()
            || self.interval_secs.is_some()
            || self.grace_period_secs.is_some()
            || self.expiry_duration_secs.is_some()
            || self.max_new_charts_per_request.is_some()
    }
}

/// Copy every `Some` field of `o` onto `config`, leaving `None` fields (and
/// therefore the previous layer's values) untouched. `apply_overrides` in
/// `mod.rs` calls this once per layer, so env vars override the user file.
pub(super) fn apply(config: &mut MetricsConfig, o: &MetricsOverride) {
    if let Some(v) = &o.chart_configs_dir {
        config.chart_configs_dir = Some(v.clone());
    }
    if let Some(v) = o.interval_secs {
        config.interval_secs = Some(v);
    }
    if let Some(v) = o.grace_period_secs {
        config.grace_period_secs = Some(v);
    }
    if let Some(v) = o.expiry_duration_secs {
        config.expiry_duration_secs = Some(v);
    }
    if let Some(v) = o.max_new_charts_per_request {
        config.max_new_charts_per_request = v;
    }
}
