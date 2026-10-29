//! The `derived_metrics:` override section of the plugin's layered
//! configuration: the partial (all-`Option`) mirror of
//! `otel_bridge::config::DerivedMetricsConfig`, patched from the user
//! `otel.yaml` and the `NETDATA_OTEL_CFG_DERIVED_METRICS_*` environment
//! variables. Layering lives in `mod.rs`; defaults live on the bridge type
//! (the section is optional in the stock file too).
use otel_bridge::config::DerivedMetricsConfig;
use serde::Deserialize;

/// The user-layer `derived_metrics:` section; `None` keeps the previous
/// layer's value.
#[derive(Debug, Default, Deserialize)]
#[serde(deny_unknown_fields)]
pub(super) struct DerivedMetricsOverride {
    #[serde(default)]
    pub(super) enabled: Option<bool>,
    #[serde(default)]
    pub(super) max_services: Option<usize>,
}

impl DerivedMetricsOverride {
    pub(super) fn has_any(&self) -> bool {
        self.enabled.is_some() || self.max_services.is_some()
    }
}

/// Copy every `Some` field of `o` onto `config`.
pub(super) fn apply(config: &mut DerivedMetricsConfig, o: &DerivedMetricsOverride) {
    if let Some(v) = o.enabled {
        config.enabled = v;
    }
    if let Some(v) = o.max_services {
        config.max_services = v;
    }
}
