use opentelemetry_proto::tonic::{
    common::v1::{any_value, AnyValue, ArrayValue, InstrumentationScope, KeyValue, KeyValueList},
    metrics::v1::{
        metric, number_data_point, Gauge, Metric, NumberDataPoint, ResourceMetrics, ScopeMetrics,
        Sum, Summary, SummaryDataPoint,
    },
    resource::v1::Resource,
};
use std::cmp::Ordering;

pub trait OtelCompare {
    fn otel_cmp(&self, other: &Self) -> Ordering;
}

impl<T: OtelCompare> OtelCompare for Option<T> {
    fn otel_cmp(&self, other: &Self) -> Ordering {
        match (self, other) {
            (None, None) => Ordering::Equal,
            (None, Some(_)) => Ordering::Less,
            (Some(_), None) => Ordering::Greater,
            (Some(a), Some(b)) => a.otel_cmp(b),
        }
    }
}

impl<T: OtelCompare> OtelCompare for Vec<T> {
    fn otel_cmp(&self, other: &Self) -> Ordering {
        match self.len().cmp(&other.len()) {
            Ordering::Equal => {}
            ordering => return ordering,
        }

        for (a, b) in self.iter().zip(other.iter()) {
            match a.otel_cmp(b) {
                Ordering::Equal => continue,
                ordering => return ordering,
            }
        }

        Ordering::Equal
    }
}

pub trait OtelSort {
    fn otel_sort(&mut self);
}

mod value {
    use super::*;

    fn value_order(v: &any_value::Value) -> u8 {
        match v {
            any_value::Value::StringValue(_) => 0,
            any_value::Value::BoolValue(_) => 1,
            any_value::Value::IntValue(_) => 2,
            any_value::Value::DoubleValue(_) => 3,
            any_value::Value::ArrayValue(_) => 4,
            any_value::Value::BytesValue(_) => 5,
            any_value::Value::KvlistValue(_) => 6,
        }
    }

    impl OtelCompare for any_value::Value {
        fn otel_cmp(&self, other: &Self) -> Ordering {
            match value_order(self).cmp(&value_order(other)) {
                Ordering::Equal => {}
                ordering => return ordering,
            }

            match (self, other) {
                (any_value::Value::StringValue(a), any_value::Value::StringValue(b)) => a.cmp(b),
                (any_value::Value::BoolValue(a), any_value::Value::BoolValue(b)) => a.cmp(b),
                (any_value::Value::IntValue(a), any_value::Value::IntValue(b)) => a.cmp(b),
                (any_value::Value::DoubleValue(a), any_value::Value::DoubleValue(b)) => {
                    a.partial_cmp(b).unwrap_or(Ordering::Equal)
                }
                (any_value::Value::BytesValue(a), any_value::Value::BytesValue(b)) => a.cmp(b),
                (any_value::Value::ArrayValue(a), any_value::Value::ArrayValue(b)) => a.otel_cmp(b),
                (any_value::Value::KvlistValue(a), any_value::Value::KvlistValue(b)) => {
                    a.otel_cmp(b)
                }
                _ => unreachable!("Types should be equal at this point"),
            }
        }
    }

    impl OtelSort for any_value::Value {
        fn otel_sort(&mut self) {
            match self {
                any_value::Value::ArrayValue(arr) => arr.otel_sort(),
                any_value::Value::KvlistValue(kvlist) => kvlist.otel_sort(),
                _ => {}
            }
        }
    }
}

mod common {
    use super::*;

    impl OtelCompare for AnyValue {
        fn otel_cmp(&self, other: &Self) -> Ordering {
            self.value.otel_cmp(&other.value)
        }
    }

    impl OtelSort for AnyValue {
        fn otel_sort(&mut self) {
            if let Some(ref mut value) = self.value {
                match value {
                    any_value::Value::ArrayValue(arr) => {
                        arr.otel_sort();
                    }
                    any_value::Value::KvlistValue(kvlist) => {
                        kvlist.otel_sort();
                    }
                    _ => {}
                }
            }
        }
    }

    impl OtelCompare for ArrayValue {
        fn otel_cmp(&self, other: &Self) -> Ordering {
            self.values.otel_cmp(&other.values)
        }
    }

    impl OtelSort for ArrayValue {
        fn otel_sort(&mut self) {
            for value in self.values.iter_mut() {
                value.otel_sort();
            }
            self.values.sort_by(|a, b| a.otel_cmp(b));
        }
    }

    impl OtelCompare for KeyValueList {
        fn otel_cmp(&self, other: &Self) -> Ordering {
            self.values.otel_cmp(&other.values)
        }
    }

    impl OtelSort for KeyValueList {
        fn otel_sort(&mut self) {
            for kv in self.values.iter_mut() {
                if let Some(ref mut v) = kv.value {
                    v.otel_sort();
                }
            }

            self.values.sort_by(|a, b| a.otel_cmp(b));
        }
    }

    impl OtelCompare for KeyValue {
        fn otel_cmp(&self, other: &Self) -> Ordering {
            match self.key.cmp(&other.key) {
                Ordering::Equal => {}
                ordering => return ordering,
            }

            self.value.otel_cmp(&other.value)
        }
    }

    impl OtelSort for KeyValue {
        fn otel_sort(&mut self) {
            if let Some(ref mut value) = self.value {
                value.otel_sort()
            }
        }
    }

    impl OtelCompare for InstrumentationScope {
        fn otel_cmp(&self, other: &Self) -> Ordering {
            match self.name.cmp(&other.name) {
                Ordering::Equal => {}
                ordering => return ordering,
            }

            match self.version.cmp(&other.version) {
                Ordering::Equal => {}
                ordering => return ordering,
            }

            self.attributes.otel_cmp(&other.attributes)
        }
    }

    impl OtelSort for InstrumentationScope {
        fn otel_sort(&mut self) {
            for attr in self.attributes.iter_mut() {
                attr.otel_sort();
            }
            self.attributes.sort_by(|a, b| a.otel_cmp(b));
        }
    }
}

mod resource {
    use super::*;

    impl OtelCompare for Resource {
        fn otel_cmp(&self, other: &Self) -> Ordering {
            self.attributes.otel_cmp(&other.attributes)
        }
    }

    impl OtelSort for Resource {
        fn otel_sort(&mut self) {
            for attr in self.attributes.iter_mut() {
                attr.otel_sort();
            }

            self.attributes.sort_by(|a, b| a.otel_cmp(b));
        }
    }
}

mod metrics {
    use super::*;

    impl OtelCompare for number_data_point::Value {
        fn otel_cmp(&self, other: &Self) -> Ordering {
            match (self, other) {
                (number_data_point::Value::AsDouble(a), number_data_point::Value::AsDouble(b)) => {
                    a.partial_cmp(b).unwrap_or(Ordering::Equal)
                }
                (number_data_point::Value::AsInt(a), number_data_point::Value::AsInt(b)) => {
                    a.cmp(b)
                }
                (number_data_point::Value::AsDouble(_), _) => Ordering::Less,
                (_, number_data_point::Value::AsDouble(_)) => Ordering::Greater,
            }
        }
    }

    impl OtelCompare for metric::Data {
        fn otel_cmp(&self, other: &Self) -> Ordering {
            match (&self, &other) {
                (metric::Data::Gauge(a), metric::Data::Gauge(b)) => a.otel_cmp(b),
                (_, _) => {
                    todo!("at some point")
                }
            }
        }
    }

    impl OtelSort for metric::Data {
        fn otel_sort(&mut self) {
            match self {
                metric::Data::Gauge(a) => a.otel_sort(),
                _ => {
                    todo!("at some point")
                }
            }
        }
    }

    impl OtelCompare for NumberDataPoint {
        fn otel_cmp(&self, other: &Self) -> Ordering {
            match self.start_time_unix_nano.cmp(&other.start_time_unix_nano) {
                Ordering::Equal => {}
                ordering => return ordering,
            }

            match self.time_unix_nano.cmp(&other.time_unix_nano) {
                Ordering::Equal => {}
                ordering => return ordering,
            }

            match self.attributes.otel_cmp(&other.attributes) {
                Ordering::Equal => {}
                ordering => return ordering,
            }

            self.value.otel_cmp(&other.value)
        }
    }

    impl OtelCompare for SummaryDataPoint {
        fn otel_cmp(&self, other: &Self) -> Ordering {
            match self.start_time_unix_nano.cmp(&other.start_time_unix_nano) {
                Ordering::Equal => {}
                ordering => return ordering,
            }

            match self.time_unix_nano.cmp(&other.time_unix_nano) {
                Ordering::Equal => {}
                ordering => return ordering,
            }

            match self.attributes.otel_cmp(&other.attributes) {
                Ordering::Equal => {}
                ordering => return ordering,
            }

            match self.count.cmp(&other.count) {
                Ordering::Equal => {}
                ordering => return ordering,
            }

            match self.sum.partial_cmp(&other.sum) {
                Some(ordering) => ordering,
                None => Ordering::Equal,
            }
        }
    }

    impl OtelSort for NumberDataPoint {
        fn otel_sort(&mut self) {
            for attr in self.attributes.iter_mut() {
                attr.otel_sort();
            }
            self.attributes.sort_by(|a, b| a.otel_cmp(b));
        }
    }

    impl OtelSort for SummaryDataPoint {
        fn otel_sort(&mut self) {
            for attr in self.attributes.iter_mut() {
                attr.otel_sort();
            }
            self.attributes.sort_by(|a, b| a.otel_cmp(b));
        }
    }

    impl OtelCompare for Gauge {
        fn otel_cmp(&self, other: &Self) -> Ordering {
            self.data_points.otel_cmp(&other.data_points)
        }
    }

    impl OtelSort for Gauge {
        fn otel_sort(&mut self) {
            for dp in self.data_points.iter_mut() {
                dp.otel_sort();
            }
            self.data_points.sort_by(|a, b| a.otel_cmp(b));
        }
    }

    impl OtelCompare for Sum {
        fn otel_cmp(&self, other: &Self) -> Ordering {
            match self.is_monotonic.cmp(&other.is_monotonic) {
                Ordering::Equal => {}
                ordering => return ordering,
            }

            match self
                .aggregation_temporality
                .cmp(&other.aggregation_temporality)
            {
                Ordering::Equal => {}
                ordering => return ordering,
            }

            self.data_points.otel_cmp(&other.data_points)
        }
    }
    impl OtelSort for Sum {
        fn otel_sort(&mut self) {
            for dp in self.data_points.iter_mut() {
                dp.otel_sort();
            }
            self.data_points.sort_by(|a, b| a.otel_cmp(b));
        }
    }

    impl OtelSort for Summary {
        fn otel_sort(&mut self) {
            for dp in self.data_points.iter_mut() {
                dp.otel_sort();
            }
            self.data_points.sort_by(|a, b| a.otel_cmp(b));
        }
    }

    impl OtelCompare for Metric {
        fn otel_cmp(&self, other: &Self) -> Ordering {
            match self.name.cmp(&other.name) {
                Ordering::Equal => {}
                ordering => return ordering,
            }

            match self.description.cmp(&other.description) {
                Ordering::Equal => {}
                ordering => return ordering,
            }

            match self.unit.cmp(&other.unit) {
                Ordering::Equal => {}
                ordering => return ordering,
            }

            self.data.otel_cmp(&other.data)
        }
    }

    impl OtelSort for Metric {
        fn otel_sort(&mut self) {
            for md in self.metadata.iter_mut() {
                md.otel_sort();
            }

            match &mut self.data {
                None => {}
                Some(metric::Data::Gauge(g)) => g.otel_sort(),
                Some(metric::Data::Sum(s)) => s.otel_sort(),
                Some(metric::Data::Histogram(_h)) => {
                    eprintln!("sort: found histogram")
                }
                Some(metric::Data::ExponentialHistogram(_eh)) => {
                    eprintln!("sourt: found exponential histogram")
                }
                Some(metric::Data::Summary(s)) => s.otel_sort(),
            }
        }
    }

    impl OtelCompare for ScopeMetrics {
        fn otel_cmp(&self, other: &Self) -> Ordering {
            match self.scope.otel_cmp(&other.scope) {
                Ordering::Equal => {}
                ordering => return ordering,
            }

            match self.schema_url.cmp(&other.schema_url) {
                Ordering::Equal => {}
                ordering => return ordering,
            }

            self.metrics.otel_cmp(&other.metrics)
        }
    }

    impl OtelSort for ScopeMetrics {
        fn otel_sort(&mut self) {
            if let Some(scope) = &mut self.scope {
                scope.otel_sort();
            }

            for m in self.metrics.iter_mut() {
                m.otel_sort();
            }
            self.metrics.sort_by(|a, b| a.otel_cmp(b));
        }
    }

    impl OtelCompare for ResourceMetrics {
        fn otel_cmp(&self, other: &Self) -> Ordering {
            match self.resource.otel_cmp(&other.resource) {
                Ordering::Equal => {}
                ordering => return ordering,
            }

            match self.schema_url.cmp(&other.schema_url) {
                Ordering::Equal => {}
                ordering => return ordering,
            }

            self.scope_metrics.otel_cmp(&other.scope_metrics)
        }
    }

    impl OtelSort for ResourceMetrics {
        fn otel_sort(&mut self) {
            if let Some(resource) = &mut self.resource {
                resource.otel_sort();
            }

            for sm in self.scope_metrics.iter_mut() {
                sm.otel_sort();
            }
            self.scope_metrics.sort_by(|a, b| a.otel_cmp(b));
        }
    }
}
