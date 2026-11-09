//! Per-service charts from logs: log records per second by severity.
//!
//! Services are identified as in [`crate::service`], so a service's log and
//! request charts carry the same labels and the same chart id suffix.
//! The severity comes from the record's severity number alone (OTel ranges
//! 1-4 trace, 5-8 debug, 9-12 info, 13-16 warn, 17-20 error, 21-24 fatal);
//! anything else, including a record with only a severity text, is
//! `unspecified`. A record counts at its resolved time (its own time, else its
//! observed time).

use crate::service::{service_labels, service_settings};
use crate::shared_aggregator::SharedAggregator;
use netdata_plugin_metrics::{
    Batch, BatchEntity, ChartDescriptor, ChartType, Counter, MetricSet, MetricSetBuilder, columns,
};
use opentelemetry_proto::tonic::collector::logs::v1::ExportLogsServiceRequest;

columns! {
    /// Severity ranges, most severe first.
    enum Severity {
        Fatal = "fatal",
        Error = "error",
        Warn = "warn",
        Info = "info",
        Debug = "debug",
        Trace = "trace",
        Unspecified = "unspecified",
    }
}

impl Severity {
    fn from_number(number: i32) -> Self {
        match number {
            21..=24 => Severity::Fatal,
            17..=20 => Severity::Error,
            13..=16 => Severity::Warn,
            9..=12 => Severity::Info,
            5..=8 => Severity::Debug,
            1..=4 => Severity::Trace,
            _ => Severity::Unspecified,
        }
    }
}

pub(crate) struct ServiceLogRecords {
    metric_set: MetricSet,
    pub(crate) aggregator: SharedAggregator,
    records: Counter<Severity>,
}

impl ServiceLogRecords {
    pub(crate) fn new(max_services: usize) -> Result<Self, netdata_plugin_metrics::Error> {
        let mut builder = MetricSetBuilder::new(service_settings(max_services))?;
        let records = builder.counter::<Severity>(ChartDescriptor {
            title: "Service log records by severity".into(),
            units: "records/s".into(),
            family: "logs".into(),
            context: "otel_service.log_records".into(),
            chart_type: ChartType::Stacked,
            priority: 60_000,
        })?;
        let metric_set = builder.build()?;
        let aggregator = SharedAggregator::new(&metric_set);
        Ok(Self {
            metric_set,
            aggregator,
            records,
        })
    }

    /// An empty batch for the log records of one export request.
    pub(crate) fn batch(&self) -> Batch {
        Batch::new(&self.metric_set)
    }

    /// Record the log records of `request` into `batch`.
    pub(crate) fn record(
        &self,
        batch: &mut Batch,
        tenant: &str,
        request: &ExportLogsServiceRequest,
    ) {
        for resource_logs in &request.resource_logs {
            let mut entity: Option<BatchEntity<'_>> = None;
            for scope_logs in &resource_logs.scope_logs {
                for record in &scope_logs.log_records {
                    if entity.is_none() {
                        entity = Some(
                            batch.entity(service_labels(tenant, resource_logs.resource.as_ref())),
                        );
                    }
                    let entity = entity.as_mut().expect("set above");
                    entity.record(record.time_unix_nano / 1_000_000_000).add(
                        self.records,
                        Severity::from_number(record.severity_number),
                        1.0,
                    );
                }
            }
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use netdata_plugin_metrics::Column;
    use opentelemetry_proto::tonic::common::v1::any_value::Value;
    use opentelemetry_proto::tonic::common::v1::{AnyValue, KeyValue};
    use opentelemetry_proto::tonic::logs::v1::{LogRecord, ResourceLogs, ScopeLogs};
    use opentelemetry_proto::tonic::resource::v1::Resource;

    #[test]
    fn severity_ranges() {
        let expected = [
            (0, "unspecified"),
            (1, "trace"),
            (4, "trace"),
            (5, "debug"),
            (9, "info"),
            (13, "warn"),
            (17, "error"),
            (20, "error"),
            (21, "fatal"),
            (24, "fatal"),
            (25, "unspecified"),
            (-1, "unspecified"),
        ];
        for (number, name) in expected {
            let severity = Severity::from_number(number);
            assert_eq!(Severity::NAMES[severity.index()], name, "{number}");
        }
    }

    #[test]
    fn counts_records_per_service_and_severity() {
        let log_records = ServiceLogRecords::new(10).unwrap();
        let record = |severity: i32| LogRecord {
            time_unix_nano: 95_000_000_000,
            severity_number: severity,
            severity_text: "ignored".to_string(),
            ..Default::default()
        };
        let request = ExportLogsServiceRequest {
            resource_logs: vec![ResourceLogs {
                resource: Some(Resource {
                    attributes: vec![KeyValue {
                        key: "service.name".to_string(),
                        value: Some(AnyValue {
                            value: Some(Value::StringValue("checkout".to_string())),
                        }),
                        ..Default::default()
                    }],
                    ..Default::default()
                }),
                scope_logs: vec![ScopeLogs {
                    log_records: vec![record(17), record(17), record(9), record(0), record(22)],
                    ..Default::default()
                }],
                ..Default::default()
            }],
        };
        let mut batch = log_records.batch();
        log_records.record(&mut batch, "default", &request);
        log_records.aggregator.merge(batch, 100);
        let mut chunks = Vec::new();
        log_records.aggregator.emit(106, &mut chunks);
        let out = netdata_plugin_protocol::readable_chart_stream(&chunks.concat());
        netdata_plugin_protocol::validate_chart_stream(out.as_bytes()).unwrap();
        assert!(out.contains("CLABEL 'resource.attributes.service.name' 'checkout' 1\n"));
        assert!(out.contains(
            "SET fatal = 1\nSET error = 2\nSET warn = 0\nSET info = 1\nSET debug = 0\n\
             SET trace = 0\nSET unspecified = 1\nEND 96 0 1\n"
        ));
    }
}
