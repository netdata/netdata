//! Lazy traversal of an OTLP `ExportMetricsServiceRequest`: walks the
//! resource -> scope -> metric -> data point nesting and yields one
//! [`DataPointContext`] per data point, carrying the references, matched chart
//! config, and identity-hash state the chart pipeline needs.
//!
//! Chart identity is a chained XxHash64 (`MetricIdentityHasher`): each level
//! hashes its identity via `MetricIdentityHash` (otel.rs — skips volatile data
//! such as values and timestamps), and the resource and scope levels add their
//! schema_urls. Prefix states are pushed/popped so siblings share the parent
//! hashing work. The finished metric-level state rides along in
//! [`MetricRef::hasher_state`]; hashing data-point attributes on top yields
//! the chart hash used as the chart-name suffix.
//!
//! Consumers: `metrics_service.rs::process_request` drives chart
//! creation/ingest from this stream; `chart_config.rs` matches a
//! [`MetricConfig`] per [`MetricRef`]. Logs and traces do not pass through
//! here — they flatten via `ng_flatten::prepare_log_frame` /
//! `prepare_trace_frame`. Chart labels below are rendered with `flatten_otel`
//! (a separate crate, not ng-flatten), which produces the dotted
//! `resource.attributes.*` / `scope.*` key names.

use std::hash::{Hash, Hasher};
use std::sync::Arc;

use opentelemetry_proto::tonic::collector::metrics::v1::ExportMetricsServiceRequest;
use opentelemetry_proto::tonic::metrics::v1::{
    AggregationTemporality, Metric, ResourceMetrics, ScopeMetrics, metric,
};
use twox_hash::XxHash64;

use serde_json::{Map as JsonMap, Value as JsonValue};

use crate::chart_config::{ChartConfigManager, MetricConfig};
use crate::otel::{self, DataPointIterExt, DataPointRef, MetricIdentityHash};

/// Render a JSON value as a label value string. `None` for null, arrays, and
/// objects, which are dropped from [`DataPointContext::chart_labels`] output
/// rather than stringified.
fn json_value_to_string(v: JsonValue) -> Option<String> {
    match v {
        JsonValue::String(s) => Some(s),
        JsonValue::Number(n) => Some(n.to_string()),
        JsonValue::Bool(b) => Some(b.to_string()),
        JsonValue::Null | JsonValue::Array(_) | JsonValue::Object(_) => None,
    }
}

/// Hierarchical hasher for metric identity hashes (XxHash64).
///
/// Keeps one current hasher plus a stack of saved states: [`Self::push`] saves
/// the state before descending into a level and [`Self::pop`] restores it, so
/// the resource and scope prefixes are hashed once and shared by every metric
/// beneath them instead of being recomputed per metric.
#[derive(Clone)]
pub struct MetricIdentityHasher {
    current: XxHash64,
    stack: Vec<XxHash64>,
}

impl MetricIdentityHasher {
    pub fn new() -> Self {
        Self {
            current: XxHash64::default(),
            stack: Vec::new(),
        }
    }

    pub fn identity_hash<T: MetricIdentityHash>(&mut self, v: &T) {
        v.identity_hash(&mut self.current);
    }

    pub fn hash<T: Hash>(&mut self, v: &T) {
        v.hash(&mut self.current);
    }

    /// Save the current state onto the stack before hashing a child level.
    pub fn push(&mut self) {
        self.stack.push(self.current.clone());
    }

    /// Restore the most recently saved state, discarding everything hashed
    /// since; panics without a matching [`Self::push`].
    pub fn pop(&mut self) {
        self.current = self.stack.pop().expect("pop called without matching push");
    }

    /// Copy of the current state; hashing continues from it independently of
    /// the stack.
    pub fn snapshot(&self) -> XxHash64 {
        self.current.clone()
    }
}

impl Default for MetricIdentityHasher {
    fn default() -> Self {
        Self::new()
    }
}

impl Hasher for MetricIdentityHasher {
    fn finish(&self) -> u64 {
        self.current.finish()
    }

    fn write(&mut self, bytes: &[u8]) {
        self.current.write(bytes);
    }
}

/// A metric with references to its containing scope and resource, the identity
/// hash state through the metric level, and the chart config matched for it
/// (None when no user or stock config matched — see
/// `ChartConfigManager::find_matching_config`).
pub struct MetricRef<'a> {
    pub resource_metrics: &'a ResourceMetrics,
    pub scope_metrics: &'a ScopeMetrics,
    pub metric: &'a Metric,
    /// Hasher state covering resource + scope + metric identity, including
    /// the resource's and scope's schema_urls. Clone it and hash data-point
    /// attributes on top for chart identity.
    pub hasher_state: XxHash64,
    pub config: Option<Arc<MetricConfig>>,
}

/// Copyable tag for which of the five OTLP metric data types a metric
/// carries — a payload-free mirror of `metric::Data`, so chart type selection
/// (chart.rs) can pattern-match without holding the prost variant.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum MetricDataKind {
    Gauge,
    Sum,
    Histogram,
    ExponentialHistogram,
    Summary,
}

impl From<&metric::Data> for MetricDataKind {
    fn from(data: &metric::Data) -> Self {
        match data {
            metric::Data::Gauge(_) => MetricDataKind::Gauge,
            metric::Data::Sum(_) => MetricDataKind::Sum,
            metric::Data::Histogram(_) => MetricDataKind::Histogram,
            metric::Data::ExponentialHistogram(_) => MetricDataKind::ExponentialHistogram,
            metric::Data::Summary(_) => MetricDataKind::Summary,
        }
    }
}

impl MetricRef<'_> {
    /// Aggregation temporality for Sum, Histogram, and ExponentialHistogram;
    /// None for Gauge and Summary, or when the prost discriminant is not a
    /// valid temporality value.
    pub fn aggregation_temporality(&self) -> Option<AggregationTemporality> {
        match &self.metric.data {
            Some(metric::Data::Sum(s)) => {
                AggregationTemporality::try_from(s.aggregation_temporality).ok()
            }
            Some(metric::Data::Histogram(h)) => {
                AggregationTemporality::try_from(h.aggregation_temporality).ok()
            }
            Some(metric::Data::ExponentialHistogram(eh)) => {
                AggregationTemporality::try_from(eh.aggregation_temporality).ok()
            }
            _ => None,
        }
    }

    /// Which data type the metric carries, or None when the metric has no
    /// data set.
    pub fn data_kind(&self) -> Option<MetricDataKind> {
        self.metric.data.as_ref().map(MetricDataKind::from)
    }

    /// Returns `is_monotonic` for Sum metrics. Returns `None` for non-Sum types.
    pub fn is_monotonic(&self) -> Option<bool> {
        match &self.metric.data {
            Some(metric::Data::Sum(s)) => Some(s.is_monotonic),
            _ => None,
        }
    }
}

/// One data point plus everything needed to chart it: the metric's
/// [`MetricRef`] (references, identity hash state, matched config) and the
/// point itself as a `DataPointRef`. This is the item [`DataPointIter`]
/// yields.
pub struct DataPointContext<'a> {
    pub metric_ref: MetricRef<'a>,
    pub datapoint_ref: DataPointRef<'a>,
}

impl DataPointContext<'_> {
    /// Get the dimension attribute key from the config, if configured.
    fn dimension_attr_key(&self) -> Option<&str> {
        self.metric_ref
            .config
            .as_ref()
            .and_then(|c| c.dimension_attribute_key.as_deref())
    }

    /// Dimension name for this data point: the configured attribute's string
    /// value, or the literal "value" when no key is configured, the attribute
    /// is missing or has no value, or its value is not a string (see
    /// `DataPointRef::dimension_name`).
    pub fn dimension_name(&self) -> &str {
        self.datapoint_ref.dimension_name(self.dimension_attr_key())
    }

    /// Hash identifying the chart this data point belongs to: the metric's
    /// identity hash extended with the data point's attributes, excluding the
    /// dimension attribute (it varies per dimension within one chart).
    /// metrics_service.rs composes it into the chart name, so points sharing
    /// it land on the same chart as dimensions.
    pub fn chart_hash(&self) -> u64 {
        let mut hasher = self.metric_ref.hasher_state.clone();
        self.datapoint_ref
            .hash_attributes(&mut hasher, self.dimension_attr_key());
        hasher.finish()
    }

    /// Returns the aggregation temporality for this data point's metric.
    /// Returns None for Gauge and Summary metrics.
    pub fn aggregation_temporality(&self) -> Option<AggregationTemporality> {
        self.metric_ref.aggregation_temporality()
    }

    /// Returns the metric data kind.
    pub fn data_kind(&self) -> Option<MetricDataKind> {
        self.metric_ref.data_kind()
    }

    /// Returns `is_monotonic` for Sum metrics. Returns `None` for non-Sum types.
    pub fn is_monotonic(&self) -> Option<bool> {
        self.metric_ref.is_monotonic()
    }

    /// Chart labels for this data point, keyed with `flatten_otel`'s naming:
    /// - `resource.attributes.{key}` for resource attributes
    /// - `scope.name`, `scope.version`, `scope.attributes.{key}` for scope info
    /// - unprefixed data point attributes, excluding the dimension attribute
    ///
    /// Values that are not strings, numbers, or bools after flattening are
    /// dropped ([`json_value_to_string`]).
    pub fn chart_labels(&self) -> Vec<(String, String)> {
        let mut jm = JsonMap::new();

        // Resource attributes
        if let Some(resource) = &self.metric_ref.resource_metrics.resource {
            flatten_otel::json_from_resource(&mut jm, resource);
        }

        // Instrumentation scope name, version, and attributes
        if let Some(scope) = &self.metric_ref.scope_metrics.scope {
            flatten_otel::json_from_instrumentation_scope(&mut jm, scope);
        }

        // Data point attributes
        let exclude = self.dimension_attr_key();
        let dp_attrs: Vec<_> = self
            .datapoint_ref
            .attributes()
            .iter()
            .filter(|kv| exclude.is_none_or(|k| k != kv.key))
            .cloned()
            .collect();
        for (key, value) in flatten_otel::json_from_key_value_list(&dp_attrs) {
            jm.insert(key, value);
        }

        jm.into_iter()
            .filter_map(|(k, v)| Some((k, json_value_to_string(v)?)))
            .collect()
    }
}

/// Lazy iterator over every data point of an `ExportMetricsServiceRequest`,
/// flattening the resource -> scope -> metric -> data point nesting into
/// [`DataPointContext`] items. Borrowing only: nothing in the request is
/// cloned or copied. Each level's identity is hashed once, per metric the
/// matched config and hash state are cached in [`CurrentMetricContext`], and
/// per data point the work is a clone-and-extend of that state. Created via
/// [`DataPointContextIterExt::datapoint_iter`].
pub struct DataPointIter<'a> {
    request: &'a ExportMetricsServiceRequest,
    ccm: &'a ChartConfigManager,
    hasher: MetricIdentityHasher,
    rm_idx: usize,
    sm_idx: usize,
    m_idx: usize,
    // Metric-level context, refreshed only when the iterator crosses to a new
    // metric (hashing and config lookup run once per metric, not per point).
    current_metric: Option<CurrentMetricContext<'a>>,
    // Which hierarchy level the state machine is descending: 0 = resource,
    // 1 = scope, 2 = metric.
    depth: u8,
    finished: bool,
}

/// Cached context for the metric currently being drained: its [`MetricRef`]
/// (with the hashed resource+scope+metric prefix and matched config) and its
/// data-point iterator.
struct CurrentMetricContext<'a> {
    metric_ref: MetricRef<'a>,
    dp_iter: otel::DataPointIter<'a>,
}

impl<'a> DataPointIter<'a> {
    pub fn new(request: &'a ExportMetricsServiceRequest, ccm: &'a ChartConfigManager) -> Self {
        Self {
            request,
            ccm,
            hasher: MetricIdentityHasher::new(),
            rm_idx: 0,
            sm_idx: 0,
            m_idx: 0,
            current_metric: None,
            depth: 0,
            finished: false,
        }
    }
}

impl<'a> Iterator for DataPointIter<'a> {
    type Item = DataPointContext<'a>;

    fn next(&mut self) -> Option<Self::Item> {
        if self.finished {
            return None;
        }

        loop {
            // If we have a current metric, try to yield its next data point
            if let Some(ref mut ctx) = self.current_metric {
                if let Some(dp) = ctx.dp_iter.next() {
                    return Some(DataPointContext {
                        metric_ref: MetricRef {
                            resource_metrics: ctx.metric_ref.resource_metrics,
                            scope_metrics: ctx.metric_ref.scope_metrics,
                            metric: ctx.metric_ref.metric,
                            hasher_state: ctx.metric_ref.hasher_state.clone(),
                            config: ctx.metric_ref.config.clone(),
                        },
                        datapoint_ref: dp,
                    });
                } else {
                    // No more data points in this metric
                    self.current_metric = None;
                    // Continue to find next metric
                }
            }

            // Find the next metric
            match self.depth {
                0 => {
                    // Enter a resource: push the baseline state so the matching
                    // pop below returns to it
                    if let Some(rm) = self.request.resource_metrics.get(self.rm_idx) {
                        self.hasher.push();
                        self.hasher.identity_hash(&rm.resource);
                        self.hasher.hash(&rm.schema_url);
                        self.depth = 1;
                        self.sm_idx = 0;
                    } else {
                        self.finished = true;
                        return None;
                    }
                }
                1 => {
                    // Enter a scope (push first to save resource state)
                    let rm = &self.request.resource_metrics[self.rm_idx];
                    if let Some(sm) = rm.scope_metrics.get(self.sm_idx) {
                        self.hasher.push();
                        self.hasher.identity_hash(&sm.scope);
                        self.hasher.hash(&sm.schema_url);
                        self.depth = 2;
                        self.m_idx = 0;
                    } else {
                        // No more scopes in this resource
                        self.hasher.pop();
                        self.depth = 0;
                        self.rm_idx += 1;
                    }
                }
                2 => {
                    // Enter a metric (push first to save scope state)
                    let rm = &self.request.resource_metrics[self.rm_idx];
                    let sm = &rm.scope_metrics[self.sm_idx];
                    if let Some(m) = sm.metrics.get(self.m_idx) {
                        self.hasher.push();
                        self.hasher.identity_hash(m);
                        let hasher_state = self.hasher.snapshot();
                        self.hasher.pop();

                        self.m_idx += 1;

                        // Build a temporary MetricRef to find config
                        let metric_ref = MetricRef {
                            resource_metrics: rm,
                            scope_metrics: sm,
                            metric: m,
                            hasher_state,
                            config: None,
                        };
                        let config = self.ccm.find_matching_config(&metric_ref);

                        // Cache the metric context for data point iteration
                        let dp_iter = m.data_points();
                        self.current_metric = Some(CurrentMetricContext {
                            metric_ref: MetricRef {
                                config,
                                ..metric_ref
                            },
                            dp_iter,
                        });
                        // Loop back to yield data points
                    } else {
                        // No more metrics in this scope
                        self.hasher.pop();
                        self.depth = 1;
                        self.sm_idx += 1;
                    }
                }
                _ => unreachable!(),
            }
        }
    }
}

/// Turns an `ExportMetricsServiceRequest` into a [`DataPointIter`] over all
/// of its data points.
pub trait DataPointContextIterExt {
    /// Iterate every data point in this request, attaching each metric's
    /// matched config from `ccm`.
    fn datapoint_iter<'a>(&'a self, ccm: &'a ChartConfigManager) -> DataPointIter<'a>;
}

impl DataPointContextIterExt for ExportMetricsServiceRequest {
    fn datapoint_iter<'a>(&'a self, ccm: &'a ChartConfigManager) -> DataPointIter<'a> {
        DataPointIter::new(self, ccm)
    }
}
