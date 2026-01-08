//! Iteration types for traversing OTLP metrics and data points.

use std::hash::{Hash, Hasher};
use std::sync::Arc;

use opentelemetry_proto::tonic::collector::metrics::v1::ExportMetricsServiceRequest;
use opentelemetry_proto::tonic::metrics::v1::{
    AggregationTemporality, Metric, ResourceMetrics, ScopeMetrics, metric,
};
use twox_hash::XxHash64;

use serde_json::{Map as JsonMap, Value as JsonValue};

use crate::chart_config::{ChartConfigManager, MetricConfig};
use crate::otel::{DataPointIterExt, DataPointRef, MetricIdentityHash};

/// Convert a JSON value to a string suitable for a Netdata label value.
/// Returns `None` for null, arrays, and objects (which are not meaningful as labels).
fn json_value_to_string(v: JsonValue) -> Option<String> {
    match v {
        JsonValue::String(s) => Some(s),
        JsonValue::Number(n) => Some(n.to_string()),
        JsonValue::Bool(b) => Some(b.to_string()),
        JsonValue::Null | JsonValue::Array(_) | JsonValue::Object(_) => None,
    }
}

/// Hierarchical hasher for computing metric identity hashes.
///
/// Maintains a stack of hasher states to efficiently compute hashes
/// at different levels of the OTLP hierarchy (resource -> scope -> metric).
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

    /// Save the current hasher state onto the stack
    pub fn push(&mut self) {
        self.stack.push(self.current.clone());
    }

    /// Restore the most recently saved state, discarding current progress
    pub fn pop(&mut self) {
        self.current = self.stack.pop().expect("pop called without matching push");
    }

    /// Return a clone of the current hasher state.
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

/// A reference to a metric along with its containing scope and resource,
/// plus a captured hasher state and optional matched configuration.
pub struct MetricRef<'a> {
    pub resource_metrics: &'a ResourceMetrics,
    pub scope_metrics: &'a ScopeMetrics,
    pub metric: &'a Metric,
    /// Hasher state with resource + scope + metric identity already hashed in.
    /// Clone this and continue hashing to incorporate data-point-level fields.
    pub hasher_state: XxHash64,
    pub config: Option<Arc<MetricConfig>>,
}

/// Simplified metric data kind for quick pattern matching.
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
    /// Returns the aggregation temporality for Sum, Histogram, and ExponentialHistogram.
    /// Returns None for Gauge and Summary.
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

    /// Returns the metric data kind (for pattern matching on the metric type).
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

/// A data point along with its full context: metric, scope, resource, config, and pre-computed values.
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

    /// Get the dimension name from the data point's attributes.
    /// Uses the configured dimension_attr_key to look up the attribute value,
    /// or returns "value" if not configured or not found.
    pub fn dimension_name(&self) -> &str {
        self.datapoint_ref.dimension_name(self.dimension_attr_key())
    }

    /// Compute a single hash that uniquely identifies which chart this data point
    /// belongs to: resource + scope + metric identity + data point attributes
    /// (excluding the dimension attribute).
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

    /// Collect all chart labels from resource attributes, instrumentation scope
    /// name/version/attributes, and data point attributes (excluding the dimension
    /// attribute).
    ///
    /// Uses `flatten_otel` for consistent key naming:
    /// - `resource.attributes.{key}` for resource attributes
    /// - `scope.name`, `scope.version`, `scope.attributes.{key}` for scope info
    /// - Data point attributes are unprefixed (excluding the dimension attribute)
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

        // Data point attributes (excluding the dimension attribute)
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

/// Iterator over all data points in an `ExportMetricsServiceRequest`.
///
/// Yields `DataPointContext` items containing references to the data point and its
/// full context (metric, scope, resource), along with pre-computed values like
/// the metric identity hash and dimension attribute key.
pub struct DataPointIter<'a> {
    request: &'a ExportMetricsServiceRequest,
    ccm: &'a ChartConfigManager,
    hasher: MetricIdentityHasher,
    rm_idx: usize,
    sm_idx: usize,
    m_idx: usize,
    dp_idx: usize,
    // Cached metric-level data (to avoid re-computing for each data point)
    current_metric: Option<CurrentMetricContext<'a>>,
    depth: u8,
    finished: bool,
}

/// Cached context for the current metric being iterated.
struct CurrentMetricContext<'a> {
    metric_ref: MetricRef<'a>,
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
            dp_idx: 0,
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
            if let Some(ref ctx) = self.current_metric {
                if let Some(dp) = ctx.metric_ref.metric.data_points().nth(self.dp_idx) {
                    self.dp_idx += 1;
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
                    self.dp_idx = 0;
                    // Continue to find next metric
                }
            }

            // Find the next metric
            match self.depth {
                0 => {
                    // Enter a resource (push first to save parent state)
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
                        self.current_metric = Some(CurrentMetricContext {
                            metric_ref: MetricRef {
                                config,
                                ..metric_ref
                            },
                        });
                        self.dp_idx = 0;
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

pub trait DataPointContextIterExt {
    fn datapoint_iter<'a>(&'a self, ccm: &'a ChartConfigManager) -> DataPointIter<'a>;
}

impl DataPointContextIterExt for ExportMetricsServiceRequest {
    fn datapoint_iter<'a>(&'a self, ccm: &'a ChartConfigManager) -> DataPointIter<'a> {
        DataPointIter::new(self, ccm)
    }
}
