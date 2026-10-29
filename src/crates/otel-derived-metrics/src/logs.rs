//! Per-service charts from logs: log records per second by severity.
//!
//! Services are identified as for spans (`services::service_labels`), so a
//! service's log and request charts carry the same labels and chart type.
//! The severity comes from the record's severity number alone (OTel ranges
//! 1-4 trace, 5-8 debug, 9-12 info, 13-16 warn, 17-20 error, 21-24 fatal);
//! anything else, including a record with only a severity text, is
//! `unspecified`. A record counts at its resolved time (its own time, else its
//! observed time).

use crate::services::service_labels;
use netdata_plugin_metrics::{
    Batch, ChartSpec, ChartType, Counter, Family, FamilyBuilder, InstanceRef, Labels,
};
use opentelemetry_proto::tonic::collector::logs::v1::ExportLogsServiceRequest;

/// As for spans: collectors and SDKs send logs in batches.
const LATENESS_SECONDS: u64 = 10;

/// Dimension order, most severe first; the index is the counter index.
const SEVERITIES: [&str; 7] = [
    "fatal",
    "error",
    "warn",
    "info",
    "debug",
    "trace",
    "unspecified",
];

fn severity_index(number: i32) -> usize {
    match number {
        21..=24 => 0,
        17..=20 => 1,
        13..=16 => 2,
        9..=12 => 3,
        5..=8 => 4,
        1..=4 => 5,
        _ => 6,
    }
}

pub(crate) struct ServiceLogs {
    pub(crate) family: Family,
    records: Counter,
}

impl ServiceLogs {
    pub(crate) fn new(max_services: usize) -> Self {
        let mut overflow = Labels::new();
        overflow.push("otel.metric.overflow", "true");
        let mut b = FamilyBuilder::new("otel_service")
            .lateness(LATENESS_SECONDS)
            .max_instances(max_services, overflow);
        let records = b.counter(SEVERITIES.len());
        b.chart(ChartSpec {
            id: "log_records",
            title: "Service log records by severity",
            units: "records/s",
            family: "logs",
            context: "otel_service.log_records",
            chart_type: ChartType::Stacked,
            priority: 60_010,
            dimensions: SEVERITIES
                .iter()
                .enumerate()
                .map(|(i, name)| records.rate(name, i))
                .collect(),
        });
        Self {
            family: b.build(),
            records,
        }
    }

    /// Record the log records of `request` into `batch`.
    pub(crate) fn record(
        &self,
        batch: &mut Batch,
        tenant: &str,
        request: &ExportLogsServiceRequest,
    ) {
        for resource_logs in &request.resource_logs {
            let mut instance: Option<InstanceRef> = None;
            for scope_logs in &resource_logs.scope_logs {
                for record in &scope_logs.log_records {
                    let instance = *instance.get_or_insert_with(|| {
                        batch.instance(service_labels(tenant, resource_logs.resource.as_ref()))
                    });
                    batch
                        .record(instance, record.time_unix_nano / 1_000_000_000)
                        .add(self.records, severity_index(record.severity_number), 1.0);
                }
            }
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use netdata_plugin_metrics::Store;
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
            assert_eq!(SEVERITIES[severity_index(number)], name, "{number}");
        }
    }

    #[test]
    fn counts_records_per_service_and_severity() {
        let logs = ServiceLogs::new(10);
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
        let mut store = Store::new(logs.family.clone());
        let mut batch = Batch::new(&logs.family);
        logs.record(&mut batch, "default", &request);
        store.merge(batch, 100);
        let out = String::from_utf8(store.emit(106, usize::MAX).concat()).unwrap();
        netdata_plugin_protocol::validate_chart_stream(out.as_bytes()).unwrap();
        assert!(out.contains("CLABEL 'resource.attributes.service.name' 'checkout' 1\n"));
        assert!(out.contains(
            "SET fatal = 1\nSET error = 2\nSET warn = 0\nSET info = 1\nSET debug = 0\n\
             SET trace = 0\nSET unspecified = 1\nEND 96\n"
        ));
    }
}
