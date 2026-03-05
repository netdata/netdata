use clap::Parser;
use serde::{Deserialize, Serialize};

fn default_max_new_charts_per_request() -> usize {
    100
}

#[derive(Parser, Debug, Clone, Serialize, Deserialize)]
pub struct MetricsConfig {
    /// Directory with configuration files for mapping OTEL metrics to Netdata charts
    #[arg(long = "otel-metrics-charts-configs-dir")]
    pub chart_configs_dir: Option<String>,

    /// Collection interval in seconds (1–3600). Default: 10.
    #[arg(long = "otel-metrics-interval")]
    pub interval_secs: Option<u64>,

    /// Grace period in seconds before gap-filling begins. Default: 5 * interval.
    #[arg(long = "otel-metrics-grace-period")]
    pub grace_period_secs: Option<u64>,

    /// Expiry duration in seconds after which charts with no data are removed. Default: 900.
    #[arg(long = "otel-metrics-expiry")]
    pub expiry_duration_secs: Option<u64>,

    /// Maximum number of new charts that can be created per gRPC request. Default: 100.
    #[arg(
        long = "otel-metrics-max-new-charts-per-request",
        default_value = "100"
    )]
    #[serde(default = "default_max_new_charts_per_request")]
    pub max_new_charts_per_request: usize,
}

impl Default for MetricsConfig {
    fn default() -> Self {
        Self {
            chart_configs_dir: None,
            interval_secs: None,
            grace_period_secs: None,
            expiry_duration_secs: None,
            max_new_charts_per_request: 100,
        }
    }
}

#[derive(Debug, Default, Deserialize)]
pub(super) struct MetricsConfigOverride {
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

impl MetricsConfig {
    pub(super) fn apply_overrides(&mut self, o: &MetricsConfigOverride) {
        if let Some(v) = &o.chart_configs_dir {
            self.chart_configs_dir = Some(v.clone());
        }
        if let Some(v) = o.interval_secs {
            self.interval_secs = Some(v);
        }
        if let Some(v) = o.grace_period_secs {
            self.grace_period_secs = Some(v);
        }
        if let Some(v) = o.expiry_duration_secs {
            self.expiry_duration_secs = Some(v);
        }
        if let Some(v) = o.max_new_charts_per_request {
            self.max_new_charts_per_request = v;
        }
    }
}
