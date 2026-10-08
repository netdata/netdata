//! Extensions over the generated OTLP metric types that the chart pipeline
//! needs: a total order + normalization for OTLP values (canonical attribute
//! ordering), the [`MetricIdentityHash`] trait behind chart identity hashing,
//! and the per-metric [`DataPointRef`]/[`DataPointIter`] abstraction.
//!
//! Consumers: `metrics_service.rs::process_request` normalizes every export
//! with [`normalize_request`] before chart matching, so attribute order does
//! not affect the resulting hashes or chart names; `iter.rs` drives
//! [`MetricIdentityHash`] through its `MetricIdentityHasher` and iterates each
//! metric's points via [`DataPointIterExt`]. Note that `iter.rs` defines a
//! same-named request-level `DataPointIter`; this file's is the per-metric
//! iterator (`otel::DataPointIter` there).

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

/// Total order over OTLP value types — the sort key [`Normalize`] uses to
/// bring attribute lists into a canonical order. Values of different types
/// order by type tag; vecs compare by length before element-wise; doubles use
/// `total_cmp` so NaN does not break the order.
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
                Value::StringValueStrindex(_) => 8,
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
                (Value::StringValueStrindex(a), Value::StringValueStrindex(b)) => a.cmp(b),
                _ => unreachable!("tags were equal"),
            },
            ord => ord,
        }
    }
}

/// The value an `AnyValue` carries for ordering and identity. OTLP says
/// signals other than profiling process `string_value_strindex` (a reference
/// into the profiles string table) as an absent value, as flatten-otel does
/// for the chart labels.
fn present_value(any_value: &AnyValue) -> Option<&Value> {
    match &any_value.value {
        Some(Value::StringValueStrindex(_)) | None => None,
        Some(value) => Some(value),
    }
}

impl Compare for AnyValue {
    fn compare(&self, other: &Self) -> Ordering {
        match (present_value(self), present_value(other)) {
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

/// Bring OTLP structures into a canonical form: normalize nested values
/// recursively, then sort every attribute list with [`Compare`] — kvlists,
/// resource/scope attributes, metric metadata, and data-point attributes.
/// Array order is preserved. Paired with [`MetricIdentityHash`] (which hashes
/// vecs in slice order), this makes identity hashes independent of the order
/// an SDK emitted attributes in.
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

/// Normalize a whole export request: resource and scope attributes, metric
/// metadata, and every data point's attributes (see [`Normalize`]). Called by
/// `metrics_service.rs::process_request` before chart matching and hashing.
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

/// Hash the *identity* of an OTLP object — the fields that distinguish one
/// series from another (names, types, attribute keys and values, temporality)
/// — while skipping measurements and timestamps. `iter.rs` feeds this into
/// its `MetricIdentityHasher` to build chart identity hashes. Vecs hash in
/// slice order, so hash only normalized (sorted) structures for
/// order-independent results.
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
            Value::StringValueStrindex(_) => 8,
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
            Value::StringValueStrindex(v) => v.hash(state),
        }
    }
}

impl MetricIdentityHash for AnyValue {
    fn identity_hash<H: Hasher>(&self, state: &mut H) {
        present_value(self).identity_hash(state);
    }
}

impl<T: MetricIdentityHash> MetricIdentityHash for &T {
    fn identity_hash<H: Hasher>(&self, state: &mut H) {
        (**self).identity_hash(state);
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
        // entity_refs are excluded from the identity
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

        if let Some(data) = &self.data {
            std::mem::discriminant(data).hash(state);
            match data {
                metric::Data::Sum(s) => {
                    state.write_i32(s.aggregation_temporality);
                    state.write_u8(u8::from(s.is_monotonic));
                }
                metric::Data::Histogram(h) => {
                    state.write_i32(h.aggregation_temporality);
                }
                metric::Data::ExponentialHistogram(eh) => {
                    state.write_i32(eh.aggregation_temporality);
                }
                _ => {}
            }
        }
    }
}

/*
 * tag: datapoint
 */

/// Borrowed view over one of the four OTLP data-point shapes. `Number` covers
/// both Gauge and Sum, which share `NumberDataPoint`; `metrics_service.rs`
/// pattern-matches the Histogram and Summary variants for decomposition.
pub enum DataPointRef<'a> {
    Number(&'a NumberDataPoint),
    Histogram(&'a HistogramDataPoint),
    ExponentialHistogram(&'a ExponentialHistogramDataPoint),
    Summary(&'a SummaryDataPoint),
}

impl<'a> DataPointRef<'a> {
    /// The wrapped data point's attributes.
    pub fn attributes(&self) -> &[KeyValue] {
        match self {
            DataPointRef::Number(dp) => &dp.attributes,
            DataPointRef::Histogram(dp) => &dp.attributes,
            DataPointRef::ExponentialHistogram(dp) => &dp.attributes,
            DataPointRef::Summary(dp) => &dp.attributes,
        }
    }

    /// First attribute with this key, if any.
    pub fn get_attribute(&self, key: &str) -> Option<&KeyValue> {
        self.attributes().iter().find(|kv| kv.key == key)
    }

    /// Dimension name for this data point: the string value of the attribute
    /// named `dimension_attr_key`, or the literal "value" when no key is
    /// given, the attribute is missing, or its value is not a string.
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

    /// Hash the attributes in slice order via [`MetricIdentityHash`], skipping
    /// every attribute whose key is `exclude_key` — the dimension attribute,
    /// which varies per dimension within one chart.
    pub fn hash_attributes<H: Hasher>(&self, state: &mut H, exclude_key: Option<&str>) {
        for kv in self.attributes() {
            if exclude_key.is_some_and(|k| k == kv.key) {
                continue;
            }
            kv.identity_hash(state);
        }
    }

    /// The wrapped `NumberDataPoint` for Gauge/Sum points, `None` for the
    /// other shapes.
    pub fn as_number(&self) -> Option<&NumberDataPoint> {
        match self {
            DataPointRef::Number(dp) => Some(dp),
            _ => None,
        }
    }

    /// Numeric value of a number data point, widening ints to f64. `None` for
    /// histogram, exponential histogram, and summary points, or when the
    /// value field is unset.
    pub fn value_as_f64(&self) -> Option<f64> {
        self.as_number().and_then(|dp| {
            dp.value.as_ref().map(|v| match v {
                number_data_point::Value::AsDouble(d) => *d,
                number_data_point::Value::AsInt(i) => *i as f64,
            })
        })
    }

    /// Collection timestamp (`time_unix_nano`) of the wrapped data point,
    /// whatever its shape.
    pub fn time_unix_nano(&self) -> u64 {
        match self {
            DataPointRef::Number(dp) => dp.time_unix_nano,
            DataPointRef::Histogram(dp) => dp.time_unix_nano,
            DataPointRef::ExponentialHistogram(dp) => dp.time_unix_nano,
            DataPointRef::Summary(dp) => dp.time_unix_nano,
        }
    }

    /// Start timestamp (`start_time_unix_nano`) of the wrapped data point,
    /// whatever its shape.
    pub fn start_time_unix_nano(&self) -> u64 {
        match self {
            DataPointRef::Number(dp) => dp.start_time_unix_nano,
            DataPointRef::Histogram(dp) => dp.start_time_unix_nano,
            DataPointRef::ExponentialHistogram(dp) => dp.start_time_unix_nano,
            DataPointRef::Summary(dp) => dp.start_time_unix_nano,
        }
    }
}

/// Lazy iterator over a single metric's data points, yielding [`DataPointRef`]s
/// in stored order; yields nothing for a metric with no data set. (Not to be
/// confused with `iter.rs`'s request-level iterator of the same name.)
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

/// Extension trait: iterate a [`Metric`]'s data points as [`DataPointRef`]s.
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

#[cfg(test)]
mod tests {
    use super::*;
    use std::collections::hash_map::DefaultHasher;

    fn hash_of(value: &AnyValue) -> u64 {
        let mut hasher = DefaultHasher::new();
        value.identity_hash(&mut hasher);
        hasher.finish()
    }

    #[test]
    fn profiling_string_table_reference_orders_and_hashes_as_absent() {
        let unset = AnyValue { value: None };
        let strindex = AnyValue {
            value: Some(Value::StringValueStrindex(3)),
        };
        let other_strindex = AnyValue {
            value: Some(Value::StringValueStrindex(4)),
        };
        assert_eq!(strindex.compare(&unset), Ordering::Equal);
        assert_eq!(strindex.compare(&other_strindex), Ordering::Equal);
        assert_eq!(hash_of(&strindex), hash_of(&unset));
        assert_eq!(hash_of(&strindex), hash_of(&other_strindex));
    }
}
