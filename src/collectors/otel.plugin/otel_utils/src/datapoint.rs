#![allow(dead_code)]

use crate::config::{Config, MetricConfig, ScopeConfig};
use crate::format::OtelFormat;
use crate::hash_bytes::HashBuilder;

use opentelemetry_proto::tonic::{
    common::v1::{AnyValue, InstrumentationScope, KeyValue},
    metrics::v1::{
        metric, number_data_point, ExponentialHistogramDataPoint, HistogramDataPoint, Metric,
        NumberDataPoint, ResourceMetrics, ScopeMetrics, SummaryDataPoint,
    },
};

use std::fmt::Write;
use std::sync::{Arc, RwLock};

pub trait OtelDataPoint {
    fn otel_attributes(&self) -> &Vec<KeyValue>;

    fn otel_attribute(&self, key: &str) -> Option<&AnyValue> {
        self.otel_attributes()
            .iter()
            .find(|kv| kv.key == key)
            .and_then(|kv| kv.value.as_ref())
    }
}

impl OtelDataPoint for NumberDataPoint {
    fn otel_attributes(&self) -> &Vec<KeyValue> {
        &self.attributes
    }
}

impl OtelDataPoint for HistogramDataPoint {
    fn otel_attributes(&self) -> &Vec<KeyValue> {
        &self.attributes
    }
}

impl OtelDataPoint for ExponentialHistogramDataPoint {
    fn otel_attributes(&self) -> &Vec<KeyValue> {
        &self.attributes
    }
}

impl OtelDataPoint for SummaryDataPoint {
    fn otel_attributes(&self) -> &Vec<KeyValue> {
        &self.attributes
    }
}

pub struct DataPointInfo<'a, T: OtelDataPoint> {
    pub resource_metrics: &'a ResourceMetrics,
    pub scope_metrics: &'a ScopeMetrics,
    pub metric: &'a Metric,
    pub point: &'a T,
    pub metric_cfg: Option<Arc<MetricConfig>>,
    pub hash: u64,
}

impl<T: OtelDataPoint> DataPointInfo<'_, T> {
    pub fn name(&self, s: &mut String) -> std::fmt::Result {
        if let Some(cfg) = &self.metric_cfg {
            if let Some(dimension) = &cfg.dimension_attribute {
                if let Some(av) = self.point.otel_attribute(dimension) {
                    if let Some(v) = &av.value {
                        return v.otel_format(s);
                    }
                }
            }
        }

        s.push_str("value");
        Ok(())
    }

    pub fn instance(&self, s: &mut String) -> std::fmt::Result {
        s.push_str(&self.metric.name);

        if let Some(cfg) = &self.metric_cfg {
            for instance_attr in cfg.instance_attributes.iter() {
                if let Some(av) = self.point.otel_attribute(instance_attr) {
                    s.push('.');
                    av.otel_format(s)?
                } else {
                    todo!("Handle not-found attribute");
                }
            }
        }

        write!(s, ".{:016x}", self.hash)
    }
}

impl<T: std::fmt::Debug + OtelDataPoint> std::fmt::Debug for DataPointInfo<'_, T> {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        let mut debug_struct = f.debug_struct("DataPoints");

        let resource = self.resource_metrics.resource.as_ref();
        debug_struct.field("resource", &resource);
        let scope = self.scope_metrics.scope.as_ref();
        debug_struct.field("scope", &scope);

        debug_struct
            .field("metric_name", &self.metric.name)
            .field("metric_description", &self.metric.description)
            .field("metric_unit", &self.metric.unit)
            .field("point", &self.point)
            .field("hash", &format!("{:#016x}", self.hash))
            .field("metric_config", &self.metric_cfg);

        debug_struct.finish()
    }
}

pub struct DataPointCollector<'cfg> {
    cfg: &'cfg RwLock<Config>,
    hash_builder: HashBuilder,
}

impl<'cfg, 'data> DataPointCollector<'cfg> {
    pub fn new(cfg: &'cfg RwLock<Config>) -> Self {
        Self {
            cfg,
            hash_builder: HashBuilder::new(),
        }
    }

    fn lookup_scope_config(
        &self,
        scope: &Option<InstrumentationScope>,
    ) -> Result<Option<Arc<ScopeConfig>>, tonic::Status> {
        if let Some(scope) = scope {
            {
                let cfg = self
                    .cfg
                    .read()
                    .map_err(|_| tonic::Status::internal("Failed to acquire config lock"))?;

                if let Some(scope_cfg) = cfg.scope_config(&scope.name) {
                    return Ok(Some(scope_cfg));
                }
            }

            let mut cfg = self
                .cfg
                .write()
                .map_err(|_| tonic::Status::internal("Failed to acquire config lock"))?;

            if let Some(scope_cfg) = cfg.match_scope(&scope.name) {
                cfg.insert_scope(&scope.name, scope_cfg.clone());
                Ok(Some(scope_cfg))
            } else {
                Ok(None)
            }
        } else {
            Ok(None)
        }
    }

    #[allow(clippy::ptr_arg)]
    pub fn collect(
        &mut self,
        service_metrics: &'data Vec<ResourceMetrics>,
        gauge_points: &mut Vec<ScalarPointInfo<'data>>,
        sum_points: &mut Vec<ScalarPointInfo<'data>>,
        histogram_points: &mut Vec<DataPointInfo<'data, HistogramDataPoint>>,
        exponential_histogram_points: &mut Vec<DataPointInfo<'data, ExponentialHistogramDataPoint>>,
        summary_points: &mut Vec<DataPointInfo<'data, SummaryDataPoint>>,
    ) -> Result<(), tonic::Status> {
        for resource_metrics in service_metrics.iter() {
            self.hash_builder
                .update_resource_metrics(resource_metrics)?;

            for scope_metrics in resource_metrics.scope_metrics.iter() {
                self.hash_builder.update_scope_metrics(scope_metrics)?;

                let scope_cfg = self.lookup_scope_config(&scope_metrics.scope)?;

                for metric in scope_metrics.metrics.iter() {
                    self.hash_builder.update_metric(metric)?;

                    let metric_cfg = match &scope_cfg {
                        Some(scope_cfg) => scope_cfg.metrics.get(&metric.name).cloned(),
                        None => None,
                    };

                    if let Some(data) = &metric.data {
                        self.hash_builder.update_metric_data(data)?;

                        match data {
                            metric::Data::Gauge(gauge) => {
                                for point in &gauge.data_points {
                                    let dim_attr = metric_cfg
                                        .as_ref()
                                        .and_then(|cfg| cfg.dimension_attribute.as_ref());
                                    self.hash_builder.update_data_point(point, dim_attr)?;

                                    gauge_points.push(DataPointInfo {
                                        resource_metrics,
                                        scope_metrics,
                                        metric,
                                        point,
                                        hash: self.hash_builder.digest(),
                                        metric_cfg: metric_cfg.clone(),
                                    });
                                }
                            }
                            metric::Data::Sum(sum) => {
                                for point in &sum.data_points {
                                    let dim_attr = metric_cfg
                                        .as_ref()
                                        .and_then(|cfg| cfg.dimension_attribute.as_ref());
                                    self.hash_builder.update_data_point(point, dim_attr)?;

                                    sum_points.push(DataPointInfo {
                                        resource_metrics,
                                        scope_metrics,
                                        metric,
                                        point,
                                        hash: self.hash_builder.digest(),
                                        metric_cfg: metric_cfg.clone(),
                                    });
                                }
                            }
                            metric::Data::Histogram(_) => {
                                eprintln!("datapoint collect: found histogram");
                            }
                            metric::Data::ExponentialHistogram(_) => {
                                eprintln!("datapoint collect: found exponential histogram");
                            }
                            metric::Data::Summary(summary) => {
                                for point in &summary.data_points {
                                    let dim_attr = metric_cfg
                                        .as_ref()
                                        .and_then(|cfg| cfg.dimension_attribute.as_ref());
                                    self.hash_builder.update_data_point(point, dim_attr)?;

                                    summary_points.push(DataPointInfo {
                                        resource_metrics,
                                        scope_metrics,
                                        metric,
                                        point,
                                        hash: self.hash_builder.digest(),
                                        metric_cfg: metric_cfg.clone(),
                                    });
                                }
                            }
                        }
                    }
                }
            }
        }

        gauge_points.sort_by_key(|dpi| dpi.hash);
        sum_points.sort_by_key(|dpi| dpi.hash);
        histogram_points.sort_by_key(|dpi| dpi.hash);
        exponential_histogram_points.sort_by_key(|dpi| dpi.hash);
        summary_points.sort_by_key(|dpi| dpi.hash);

        Ok(())
    }
}

pub type ScalarPointInfo<'a> = DataPointInfo<'a, NumberDataPoint>;

impl ScalarPointInfo<'_> {
    pub fn value(&self) -> f64 {
        if let Some(v) = self.point.value {
            match v {
                number_data_point::Value::AsInt(i) => i as f64,
                number_data_point::Value::AsDouble(d) => d,
            }
        } else {
            0.0
        }
    }

    pub fn unix_time(&self) -> u64 {
        self.point.time_unix_nano
    }

    pub fn is_monotonic(&self) -> bool {
        if let Some(metric::Data::Sum(s)) = self.metric.data.as_ref() {
            s.is_monotonic
        } else {
            false
        }
    }
}

pub type SummaryPointInfo<'a> = DataPointInfo<'a, SummaryDataPoint>;
