//! OpenTelemetry protocol extensions for normalization, comparison, hashing, and data point iteration.

use opentelemetry_proto::tonic::{
    collector::metrics::v1::ExportMetricsServiceRequest,
    common::v1::{
        AnyValue, ArrayValue, InstrumentationScope, KeyValue, KeyValueList, any_value::Value,
    },
    metrics::v1::{
        ExponentialHistogram, ExponentialHistogramDataPoint, Gauge, Histogram, HistogramDataPoint,
        Metric, NumberDataPoint, Sum, Summary, SummaryDataPoint, metric, number_data_point,
    },
    resource::v1::Resource,
};
use std::cmp::Ordering;
use std::hash::{Hash, Hasher};

/*
 * tag: compare
 */

trait Compare {
    fn compare(&self, other: &Self) -> Ordering;
}

impl Compare for Value {
    fn compare(&self, other: &Self) -> Ordering {
        fn tag(value: &Value) -> u8 {
            match value {
                Value::StringValue(_) => 1,
                Value::BoolValue(_) => 2,
                Value::IntValue(_) => 3,
                Value::DoubleValue(_) => 4,
                Value::ArrayValue(_) => 5,
                Value::KvlistValue(_) => 6,
                Value::BytesValue(_) => 7,
            }
        }

        match tag(self).cmp(&tag(other)) {
            Ordering::Equal => match (self, other) {
                (Value::StringValue(a), Value::StringValue(b)) => a.cmp(b),
                (Value::BoolValue(a), Value::BoolValue(b)) => a.cmp(b),
                (Value::IntValue(a), Value::IntValue(b)) => a.cmp(b),
                (Value::DoubleValue(a), Value::DoubleValue(b)) => a.total_cmp(b),
                (Value::ArrayValue(a), Value::ArrayValue(b)) => a.compare(b),
                (Value::KvlistValue(a), Value::KvlistValue(b)) => a.compare(b),
                (Value::BytesValue(a), Value::BytesValue(b)) => a.cmp(b),
                _ => unreachable!("tags were equal"),
            },
            ord => ord,
        }
    }
}

impl Compare for AnyValue {
    fn compare(&self, other: &Self) -> Ordering {
        match (&self.value, &other.value) {
            (None, None) => Ordering::Equal,
            (None, Some(_)) => Ordering::Less,
            (Some(_), None) => Ordering::Greater,
            (Some(a), Some(b)) => a.compare(b),
        }
    }
}

impl<T: Compare> Compare for Vec<T> {
    fn compare(&self, other: &Self) -> Ordering {
        match self.len().cmp(&other.len()) {
            Ordering::Equal => {
                for (a, b) in self.iter().zip(other.iter()) {
                    match a.compare(b) {
                        Ordering::Equal => continue,
                        ord => return ord,
                    }
                }
                Ordering::Equal
            }
            ord => ord,
        }
    }
}

impl Compare for ArrayValue {
    fn compare(&self, other: &Self) -> Ordering {
        self.values.compare(&other.values)
    }
}

impl Compare for KeyValue {
    fn compare(&self, other: &Self) -> Ordering {
        match self.key.cmp(&other.key) {
            Ordering::Equal => self.value.compare(&other.value),
            ord => ord,
        }
    }
}

impl Compare for KeyValueList {
    fn compare(&self, other: &Self) -> Ordering {
        self.values.compare(&other.values)
    }
}

impl Compare for Option<AnyValue> {
    fn compare(&self, other: &Self) -> Ordering {
        match (self, other) {
            (None, None) => Ordering::Equal,
            (None, Some(_)) => Ordering::Less,
            (Some(_), None) => Ordering::Greater,
            (Some(a), Some(b)) => a.compare(b),
        }
    }
}

/*
 * tag: normalize
 */

trait Normalize {
    fn normalize(&mut self);
}

impl Normalize for Value {
    fn normalize(&mut self) {
        match self {
            Value::KvlistValue(kv) => kv.normalize(),
            Value::ArrayValue(arr) => arr.normalize(),
            _ => {} // Primitive types don't need normalization
        }
    }
}

impl Normalize for AnyValue {
    fn normalize(&mut self) {
        if let Some(v) = &mut self.value {
            v.normalize();
        }
    }
}

impl Normalize for ArrayValue {
    fn normalize(&mut self) {
        // Normalize elements but don't sort - array order is meaningful
        for v in &mut self.values {
            v.normalize();
        }
    }
}

impl Normalize for KeyValue {
    fn normalize(&mut self) {
        if let Some(v) = &mut self.value {
            v.normalize();
        }
    }
}

impl Normalize for KeyValueList {
    fn normalize(&mut self) {
        for kv in &mut self.values {
            kv.normalize();
        }
        self.values.sort_by(|a, b| a.compare(b));
    }
}

impl Normalize for Resource {
    fn normalize(&mut self) {
        for kv in &mut self.attributes {
            kv.normalize();
        }
        self.attributes.sort_by(|a, b| a.compare(b));
    }
}

impl Normalize for InstrumentationScope {
    fn normalize(&mut self) {
        for kv in &mut self.attributes {
            kv.normalize();
        }
        self.attributes.sort_by(|a, b| a.compare(b));
    }
}

impl<T: Normalize> Normalize for Option<T> {
    fn normalize(&mut self) {
        if let Some(v) = self {
            v.normalize();
        }
    }
}

impl Normalize for NumberDataPoint {
    fn normalize(&mut self) {
        for kv in &mut self.attributes {
            kv.normalize();
        }
        self.attributes.sort_by(|a, b| a.compare(b));
    }
}

impl Normalize for HistogramDataPoint {
    fn normalize(&mut self) {
        for kv in &mut self.attributes {
            kv.normalize();
        }
        self.attributes.sort_by(|a, b| a.compare(b));
    }
}

impl Normalize for ExponentialHistogramDataPoint {
    fn normalize(&mut self) {
        for kv in &mut self.attributes {
            kv.normalize();
        }
        self.attributes.sort_by(|a, b| a.compare(b));
    }
}

impl Normalize for SummaryDataPoint {
    fn normalize(&mut self) {
        for kv in &mut self.attributes {
            kv.normalize();
        }
        self.attributes.sort_by(|a, b| a.compare(b));
    }
}

impl Normalize for Gauge {
    fn normalize(&mut self) {
        for dp in &mut self.data_points {
            dp.normalize();
        }
    }
}

impl Normalize for Sum {
    fn normalize(&mut self) {
        for dp in &mut self.data_points {
            dp.normalize();
        }
    }
}

impl Normalize for Histogram {
    fn normalize(&mut self) {
        for dp in &mut self.data_points {
            dp.normalize();
        }
    }
}

impl Normalize for ExponentialHistogram {
    fn normalize(&mut self) {
        for dp in &mut self.data_points {
            dp.normalize();
        }
    }
}

impl Normalize for Summary {
    fn normalize(&mut self) {
        for dp in &mut self.data_points {
            dp.normalize();
        }
    }
}

impl Normalize for metric::Data {
    fn normalize(&mut self) {
        match self {
            metric::Data::Gauge(g) => g.normalize(),
            metric::Data::Sum(s) => s.normalize(),
            metric::Data::Histogram(h) => h.normalize(),
            metric::Data::ExponentialHistogram(eh) => eh.normalize(),
            metric::Data::Summary(s) => s.normalize(),
        }
    }
}

impl Normalize for Metric {
    fn normalize(&mut self) {
        for kv in &mut self.metadata {
            kv.normalize();
        }
        self.metadata.sort_by(|a, b| a.compare(b));
        if let Some(data) = &mut self.data {
            data.normalize();
        }
    }
}

/// Normalize an ExportMetricsServiceRequest by recursively sorting all attributes
pub fn normalize_request(request: &mut ExportMetricsServiceRequest) {
    for rm in &mut request.resource_metrics {
        rm.resource.normalize();
        for sm in &mut rm.scope_metrics {
            sm.scope.normalize();
            for m in &mut sm.metrics {
                m.normalize();
            }
        }
    }
}

/*
 * tag: hash
 */

pub trait MetricIdentityHash {
    fn identity_hash<H: Hasher>(&self, state: &mut H);
}

impl MetricIdentityHash for Value {
    fn identity_hash<H: Hasher>(&self, state: &mut H) {
        // Hash the discriminant tag first (same tags as Compare)
        let tag: u8 = match self {
            Value::StringValue(_) => 1,
            Value::BoolValue(_) => 2,
            Value::IntValue(_) => 3,
            Value::DoubleValue(_) => 4,
            Value::ArrayValue(_) => 5,
            Value::KvlistValue(_) => 6,
            Value::BytesValue(_) => 7,
        };
        tag.hash(state);

        match self {
            Value::StringValue(v) => v.hash(state),
            Value::BoolValue(v) => v.hash(state),
            Value::IntValue(v) => v.hash(state),
            Value::DoubleValue(v) => v.to_bits().hash(state),
            Value::ArrayValue(v) => v.identity_hash(state),
            Value::KvlistValue(v) => v.identity_hash(state),
            Value::BytesValue(v) => v.hash(state),
        }
    }
}

impl MetricIdentityHash for AnyValue {
    fn identity_hash<H: Hasher>(&self, state: &mut H) {
        self.value.identity_hash(state);
    }
}

impl<T: MetricIdentityHash> MetricIdentityHash for Option<T> {
    fn identity_hash<H: Hasher>(&self, state: &mut H) {
        match self {
            None => 0u8.hash(state),
            Some(v) => {
                1u8.hash(state);
                v.identity_hash(state);
            }
        }
    }
}

impl<T: MetricIdentityHash> MetricIdentityHash for Vec<T> {
    fn identity_hash<H: Hasher>(&self, state: &mut H) {
        self.len().hash(state);
        for item in self {
            item.identity_hash(state);
        }
    }
}

impl MetricIdentityHash for ArrayValue {
    fn identity_hash<H: Hasher>(&self, state: &mut H) {
        self.values.identity_hash(state);
    }
}

impl MetricIdentityHash for KeyValue {
    fn identity_hash<H: Hasher>(&self, state: &mut H) {
        self.key.hash(state);
        self.value.identity_hash(state);
    }
}

impl MetricIdentityHash for KeyValueList {
    fn identity_hash<H: Hasher>(&self, state: &mut H) {
        self.values.identity_hash(state);
    }
}

impl MetricIdentityHash for Resource {
    fn identity_hash<H: Hasher>(&self, state: &mut H) {
        self.attributes.identity_hash(state);
        state.write_u32(self.dropped_attributes_count);
        // ignore entity refs
    }
}

impl MetricIdentityHash for InstrumentationScope {
    fn identity_hash<H: Hasher>(&self, state: &mut H) {
        self.name.hash(state);
        self.version.hash(state);
        self.attributes.identity_hash(state);
        state.write_u32(self.dropped_attributes_count);
    }
}

impl MetricIdentityHash for Metric {
    fn identity_hash<H: Hasher>(&self, state: &mut H) {
        self.name.hash(state);
        self.description.hash(state);
        self.unit.hash(state);
    }
}

/*
 * tag: datapoint
 */

/// A reference to a data point, abstracting over the different data point types.
pub enum DataPointRef<'a> {
    Number(&'a NumberDataPoint),
    Histogram(&'a HistogramDataPoint),
    ExponentialHistogram(&'a ExponentialHistogramDataPoint),
    Summary(&'a SummaryDataPoint),
}

impl<'a> DataPointRef<'a> {
    /// Get the attributes of this data point.
    pub fn attributes(&self) -> &[KeyValue] {
        match self {
            DataPointRef::Number(dp) => &dp.attributes,
            DataPointRef::Histogram(dp) => &dp.attributes,
            DataPointRef::ExponentialHistogram(dp) => &dp.attributes,
            DataPointRef::Summary(dp) => &dp.attributes,
        }
    }

    /// Get the value of a specific attribute by key.
    pub fn get_attribute(&self, key: &str) -> Option<&KeyValue> {
        self.attributes().iter().find(|kv| kv.key == key)
    }

    /// Get the dimension name based on the dimension attribute key.
    /// If the key is provided and found, returns the string value of that attribute.
    /// Otherwise returns "value".
    pub fn dimension_name(&self, dimension_attr_key: Option<&str>) -> &str {
        let Some(key) = dimension_attr_key else {
            return "value";
        };

        self.get_attribute(key)
            .and_then(|kv| kv.value.as_ref())
            .and_then(|v| v.value.as_ref())
            .and_then(|v| match v {
                Value::StringValue(s) => Some(s.as_str()),
                _ => None,
            })
            .unwrap_or("value")
    }

    /// Hash the attributes, excluding the dimension attribute key if provided.
    pub fn hash_attributes<H: Hasher>(&self, state: &mut H, exclude_key: Option<&str>) {
        for kv in self.attributes() {
            if exclude_key.is_some_and(|k| k == kv.key) {
                continue;
            }
            kv.identity_hash(state);
        }
    }

    /// Get the underlying NumberDataPoint if this is a number type (Gauge or Sum).
    pub fn as_number(&self) -> Option<&NumberDataPoint> {
        match self {
            DataPointRef::Number(dp) => Some(dp),
            _ => None,
        }
    }

    /// Get the numeric value as f64.
    /// Returns None if this is not a number data point or has no value.
    pub fn value_as_f64(&self) -> Option<f64> {
        self.as_number().and_then(|dp| {
            dp.value.as_ref().map(|v| match v {
                number_data_point::Value::AsDouble(d) => *d,
                number_data_point::Value::AsInt(i) => *i as f64,
            })
        })
    }

    /// Get the time_unix_nano field.
    /// Returns 0 if not a number data point.
    pub fn time_unix_nano(&self) -> u64 {
        match self {
            DataPointRef::Number(dp) => dp.time_unix_nano,
            DataPointRef::Histogram(dp) => dp.time_unix_nano,
            DataPointRef::ExponentialHistogram(dp) => dp.time_unix_nano,
            DataPointRef::Summary(dp) => dp.time_unix_nano,
        }
    }

    /// Get the start_time_unix_nano field.
    /// Returns 0 if not a number data point.
    pub fn start_time_unix_nano(&self) -> u64 {
        match self {
            DataPointRef::Number(dp) => dp.start_time_unix_nano,
            DataPointRef::Histogram(dp) => dp.start_time_unix_nano,
            DataPointRef::ExponentialHistogram(dp) => dp.start_time_unix_nano,
            DataPointRef::Summary(dp) => dp.start_time_unix_nano,
        }
    }
}

/// Iterator over data points in a metric.
pub struct DataPointIter<'a> {
    inner: DataPointIterInner<'a>,
}

enum DataPointIterInner<'a> {
    Number(std::slice::Iter<'a, NumberDataPoint>),
    Histogram(std::slice::Iter<'a, HistogramDataPoint>),
    ExponentialHistogram(std::slice::Iter<'a, ExponentialHistogramDataPoint>),
    Summary(std::slice::Iter<'a, SummaryDataPoint>),
    Empty,
}

impl<'a> Iterator for DataPointIter<'a> {
    type Item = DataPointRef<'a>;

    fn next(&mut self) -> Option<Self::Item> {
        match &mut self.inner {
            DataPointIterInner::Number(iter) => iter.next().map(DataPointRef::Number),
            DataPointIterInner::Histogram(iter) => iter.next().map(DataPointRef::Histogram),
            DataPointIterInner::ExponentialHistogram(iter) => {
                iter.next().map(DataPointRef::ExponentialHistogram)
            }
            DataPointIterInner::Summary(iter) => iter.next().map(DataPointRef::Summary),
            DataPointIterInner::Empty => None,
        }
    }
}

/// Extension trait to iterate over data points in a metric.
pub trait DataPointIterExt {
    fn data_points(&self) -> DataPointIter<'_>;
}

impl DataPointIterExt for Metric {
    fn data_points(&self) -> DataPointIter<'_> {
        let inner = match &self.data {
            Some(metric::Data::Gauge(g)) => DataPointIterInner::Number(g.data_points.iter()),
            Some(metric::Data::Sum(s)) => DataPointIterInner::Number(s.data_points.iter()),
            Some(metric::Data::Histogram(h)) => DataPointIterInner::Histogram(h.data_points.iter()),
            Some(metric::Data::ExponentialHistogram(eh)) => {
                DataPointIterInner::ExponentialHistogram(eh.data_points.iter())
            }
            Some(metric::Data::Summary(s)) => DataPointIterInner::Summary(s.data_points.iter()),
            None => DataPointIterInner::Empty,
        };
        DataPointIter { inner }
    }
}
