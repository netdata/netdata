use crate::datapoint::OtelDataPoint;

use opentelemetry_proto::tonic::{
    common::v1::{any_value, AnyValue, ArrayValue, InstrumentationScope, KeyValue, KeyValueList},
    metrics::v1::{metric, Metric, ResourceMetrics, ScopeMetrics},
    resource::v1::Resource,
};

use std::io::Write;

trait OtelHashBytes {
    fn otel_hash_bytes<W: Write>(&self, writer: &mut W) -> std::io::Result<()>;
}

mod value {
    use super::*;

    impl OtelHashBytes for any_value::Value {
        fn otel_hash_bytes<W: Write>(&self, writer: &mut W) -> std::io::Result<()> {
            match self {
                any_value::Value::StringValue(s) => {
                    writer.write_all(&[0])?;
                    writer.write_all(s.as_bytes())
                }
                any_value::Value::BoolValue(b) => {
                    writer.write_all(&[1])?;
                    writer.write_all(&[*b as u8])
                }
                any_value::Value::IntValue(i) => {
                    writer.write_all(&[2])?;
                    writer.write_all(&i.to_ne_bytes())
                }
                any_value::Value::DoubleValue(d) => {
                    writer.write_all(&[3])?;
                    writer.write_all(&d.to_ne_bytes())
                }
                any_value::Value::BytesValue(bytes) => {
                    writer.write_all(&[4])?;
                    writer.write_all(bytes)
                }
                any_value::Value::ArrayValue(arr) => {
                    writer.write_all(&[5])?;
                    arr.otel_hash_bytes(writer)
                }
                any_value::Value::KvlistValue(kvlist) => {
                    writer.write_all(&[6])?;
                    kvlist.otel_hash_bytes(writer)
                }
            }
        }
    }
}

mod common {
    use super::*;

    impl OtelHashBytes for AnyValue {
        fn otel_hash_bytes<W: Write>(&self, writer: &mut W) -> std::io::Result<()> {
            if let Some(value) = &self.value {
                value.otel_hash_bytes(writer)?;
            }

            Ok(())
        }
    }

    impl OtelHashBytes for ArrayValue {
        fn otel_hash_bytes<W: Write>(&self, writer: &mut W) -> std::io::Result<()> {
            for value in &self.values {
                value.otel_hash_bytes(writer)?;
            }

            Ok(())
        }
    }

    impl OtelHashBytes for KeyValueList {
        fn otel_hash_bytes<W: Write>(&self, writer: &mut W) -> std::io::Result<()> {
            for kv in &self.values {
                kv.otel_hash_bytes(writer)?;
            }

            Ok(())
        }
    }

    impl OtelHashBytes for KeyValue {
        fn otel_hash_bytes<W: Write>(&self, writer: &mut W) -> std::io::Result<()> {
            writer.write_all(self.key.as_bytes())?;

            if let Some(value) = &self.value {
                value.otel_hash_bytes(writer)?;
            }

            Ok(())
        }
    }

    impl OtelHashBytes for InstrumentationScope {
        fn otel_hash_bytes<W: Write>(&self, writer: &mut W) -> std::io::Result<()> {
            writer.write_all(self.name.as_bytes())?;
            writer.write_all(self.version.as_bytes())?;

            for attr in &self.attributes {
                attr.otel_hash_bytes(writer)?;
            }

            Ok(())
        }
    }
}

mod resource {
    use super::*;

    impl OtelHashBytes for Resource {
        fn otel_hash_bytes<W: Write>(&self, writer: &mut W) -> std::io::Result<()> {
            for attr in &self.attributes {
                attr.otel_hash_bytes(writer)?;
            }

            Ok(())
        }
    }
}

mod metrics {
    use super::*;

    impl OtelHashBytes for ResourceMetrics {
        fn otel_hash_bytes<W: Write>(&self, writer: &mut W) -> std::io::Result<()> {
            if let Some(resource) = &self.resource {
                resource.otel_hash_bytes(writer)?;
            }

            writer.write_all(self.schema_url.as_bytes())
        }
    }

    impl OtelHashBytes for ScopeMetrics {
        fn otel_hash_bytes<W: Write>(&self, writer: &mut W) -> std::io::Result<()> {
            if let Some(scope) = &self.scope {
                scope.otel_hash_bytes(writer)?;
            }

            writer.write_all(self.schema_url.as_bytes())
        }
    }

    impl OtelHashBytes for Metric {
        fn otel_hash_bytes<W: Write>(&self, writer: &mut W) -> std::io::Result<()> {
            writer.write_all(self.name.as_bytes())?;
            writer.write_all(self.description.as_bytes())?;
            writer.write_all(self.unit.as_bytes())
        }
    }

    impl OtelHashBytes for metric::Data {
        fn otel_hash_bytes<W: Write>(&self, writer: &mut W) -> std::io::Result<()> {
            match self {
                metric::Data::Gauge(g) => {
                    for dp in g.data_points.iter() {
                        dp.otel_hash_bytes(writer)?;
                    }

                    Ok(())
                }
                metric::Data::Sum(s) => {
                    for dp in s.data_points.iter() {
                        dp.otel_hash_bytes(writer)?;
                    }

                    writer.write_all(&s.aggregation_temporality.to_ne_bytes())?;
                    writer.write_all(&[s.is_monotonic as u8])
                }
                metric::Data::Histogram(_h) => {
                    eprintln!("hash bytes: found histogram");
                    Ok(())
                }
                metric::Data::ExponentialHistogram(_eh) => {
                    eprintln!("hash bytes: found exponential histogram");
                    Ok(())
                }
                metric::Data::Summary(s) => {
                    for dp in s.data_points.iter() {
                        dp.otel_hash_bytes(writer)?;
                    }
                    Ok(())
                }
            }
        }
    }

    // impl OtelHashBytes for NumberDataPoint {
    //     fn otel_hash_bytes<W: Write>(&self, writer: &mut W) -> std::io::Result<()> {
    //         for attr in self.attributes.iter() {
    //             attr.otel_hash_bytes(writer)?;
    //         }

    //         Ok(())
    //     }
    // }
}

impl<T: OtelDataPoint> OtelHashBytes for T {
    fn otel_hash_bytes<W: Write>(&self, writer: &mut W) -> std::io::Result<()> {
        for attr in self.otel_attributes() {
            attr.otel_hash_bytes(writer)?
        }

        Ok(())
    }
}

#[derive(Default)]
pub struct HashBuilder {
    buffer: Vec<u8>,

    resource_builder: xxhash_rust::xxh3::Xxh3,
    scope_builder: xxhash_rust::xxh3::Xxh3,
    metric_builder: xxhash_rust::xxh3::Xxh3,
    metric_data_builder: xxhash_rust::xxh3::Xxh3,
    point_builder: xxhash_rust::xxh3::Xxh3,
}

impl HashBuilder {
    pub fn new() -> Self {
        Self::default()
    }

    pub fn update_resource_metrics(
        &mut self,
        resource_metrics: &ResourceMetrics,
    ) -> Result<(), tonic::Status> {
        self.buffer.clear();
        resource_metrics
            .otel_hash_bytes(&mut self.buffer)
            .map_err(|_| {
                let msg = "Failed to serialize hash bytes for resource metrics";
                tonic::Status::internal(msg)
            })?;

        self.resource_builder.reset();
        self.resource_builder.update(&self.buffer);

        Ok(())
    }

    pub fn update_scope_metrics(
        &mut self,
        scope_metrics: &ScopeMetrics,
    ) -> Result<(), tonic::Status> {
        self.buffer.clear();
        scope_metrics
            .otel_hash_bytes(&mut self.buffer)
            .map_err(|_| {
                let msg = "Failed to serialize hash bytes for scope metrics";
                tonic::Status::internal(msg)
            })?;

        self.scope_builder = self.resource_builder.clone();
        self.scope_builder.update(&self.buffer);

        Ok(())
    }

    pub fn update_metric(&mut self, metric: &Metric) -> Result<(), tonic::Status> {
        self.buffer.clear();
        metric.otel_hash_bytes(&mut self.buffer).map_err(|_| {
            let msg = "Failed to serialize hash bytes for metric";
            tonic::Status::internal(msg)
        })?;

        self.metric_builder = self.scope_builder.clone();
        self.metric_builder.update(&self.buffer);

        Ok(())
    }

    pub fn update_metric_data(&mut self, data: &metric::Data) -> Result<(), tonic::Status> {
        self.buffer.clear();
        data.otel_hash_bytes(&mut self.buffer).map_err(|_| {
            let msg = "Failed to serialize hash bytes for metric data";
            tonic::Status::internal(msg)
        })?;

        self.metric_data_builder = self.metric_builder.clone();
        self.metric_data_builder.update(&self.buffer);

        Ok(())
    }

    pub fn update_data_point<T: OtelDataPoint>(
        &mut self,
        point: &T,
        dimension_attribute: Option<&String>,
    ) -> Result<(), tonic::Status> {
        self.buffer.clear();

        if let Some(key) = dimension_attribute {
            for kv in point.otel_attributes() {
                if &kv.key == key {
                    continue;
                }

                kv.otel_hash_bytes(&mut self.buffer).map_err(|_| {
                    let msg = "Failed to serialize hash bytes for number data point";
                    tonic::Status::internal(msg)
                })?;
            }
        } else {
            point.otel_hash_bytes(&mut self.buffer).map_err(|_| {
                let msg = "Failed to serialize hash bytes for number data point";
                tonic::Status::internal(msg)
            })?;
        }

        self.point_builder = self.metric_data_builder.clone();
        self.point_builder.update(&self.buffer);

        Ok(())
    }

    pub fn digest(&self) -> u64 {
        self.point_builder.digest()
    }
}
