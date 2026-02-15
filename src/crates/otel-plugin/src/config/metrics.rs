use bridge::config::MetricsConfig;
use serde::Deserialize;

#[derive(Debug, Default, Deserialize)]
pub(super) struct MetricsOverride {
    #[serde(default)]
    pub(super) chart_configs_dir: Option<String>,
    #[serde(default)]
    pub(super) interval_secs: Option<u64>,
    #[serde(default)]
    pub(super) grace_period_secs: Option<u64>,
    #[serde(default)]
    pub(super) expiry_duration_secs: Option<u64>,
    #[serde(default)]
    pub(super) max_new_charts_per_request: Option<usize>,
}

impl MetricsOverride {
    pub(super) fn has_any(&self) -> bool {
        self.chart_configs_dir.is_some()
            || self.interval_secs.is_some()
            || self.grace_period_secs.is_some()
            || self.expiry_duration_secs.is_some()
            || self.max_new_charts_per_request.is_some()
    }
}

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
