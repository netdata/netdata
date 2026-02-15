use arrow::array::*;
use arrow::datatypes::*;
use arrow::record_batch::RecordBatch;
use arrow::util::display::ArrayFormatter;
use opentelemetry_proto::tonic::{
    collector::logs::v1::ExportLogsServiceRequest,
    common::v1::{AnyValue, InstrumentationScope, KeyValue, any_value},
    logs::v1::{LogRecord, ResourceLogs, ScopeLogs},
    resource::v1::Resource,
};

const SERVICE_NAME: &str = "tlc-trip-data";
const SCOPE_NAME: &str = "tlc-otel-bridge";
const CRATE_VERSION: &str = env!("CARGO_PKG_VERSION");
const SEVERITY_INFO: i32 = 9;

/// Convert an Arrow RecordBatch of TLC trip data into OTLP LogRecords.
///
/// Each row becomes one LogRecord. Every column becomes an attribute
/// with the column name as key.
pub fn batch_to_log_records(batch: &RecordBatch) -> Vec<LogRecord> {
    let schema = batch.schema();
    let num_rows = batch.num_rows();
    let mut records = Vec::with_capacity(num_rows);

    let now_ns = std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .unwrap_or_default()
        .as_nanos() as u64;

    // Pre-compute column accessors.
    let columns: Vec<(&str, &dyn Array)> = schema
        .fields()
        .iter()
        .zip(batch.columns())
        .map(|(f, c)| (f.name().as_str(), c.as_ref()))
        .collect();

    // Find the pickup_datetime column for use as the event timestamp.
    let pickup_ts_idx = schema
        .fields()
        .iter()
        .position(|f| f.name() == "pickup_datetime");

    for row in 0..num_rows {
        let mut attributes = Vec::with_capacity(columns.len());

        for &(name, col) in &columns {
            if let Some(value) = column_value_to_any(col, row) {
                attributes.push(KeyValue {
                    key: name.to_string(),
                    value: Some(value),
                });
            }
        }

        // Use pickup_datetime as the event timestamp if available.
        let time_unix_nano = pickup_ts_idx
            .and_then(|idx| {
                let col = batch.column(idx);
                timestamp_to_nanos(col, row)
            })
            .unwrap_or(now_ns);

        records.push(LogRecord {
            time_unix_nano,
            observed_time_unix_nano: now_ns,
            severity_number: SEVERITY_INFO,
            severity_text: "INFO".to_string(),
            attributes,
            ..Default::default()
        });
    }

    records
}

/// Extract a nanosecond timestamp from a timestamp column at the given row.
fn timestamp_to_nanos(col: &dyn Array, row: usize) -> Option<u64> {
    if col.is_null(row) {
        return None;
    }

    // TLC uses timestamp[us] (microseconds).
    if let Some(ts) = col.as_any().downcast_ref::<TimestampMicrosecondArray>() {
        let us = ts.value(row);
        return Some((us as u64).wrapping_mul(1_000));
    }
    if let Some(ts) = col.as_any().downcast_ref::<TimestampNanosecondArray>() {
        return Some(ts.value(row) as u64);
    }
    if let Some(ts) = col.as_any().downcast_ref::<TimestampMillisecondArray>() {
        return Some((ts.value(row) as u64).wrapping_mul(1_000_000));
    }
    if let Some(ts) = col.as_any().downcast_ref::<TimestampSecondArray>() {
        return Some((ts.value(row) as u64).wrapping_mul(1_000_000_000));
    }
    None
}

/// Convert a single Arrow column value at `row` into an OTel AnyValue.
fn column_value_to_any(col: &dyn Array, row: usize) -> Option<AnyValue> {
    if col.is_null(row) {
        return None;
    }

    let dt = col.data_type();
    match dt {
        DataType::Utf8 => {
            let arr = col.as_any().downcast_ref::<StringArray>()?;
            Some(AnyValue {
                value: Some(any_value::Value::StringValue(arr.value(row).to_string())),
            })
        }
        DataType::LargeUtf8 => {
            let arr = col.as_any().downcast_ref::<LargeStringArray>()?;
            Some(AnyValue {
                value: Some(any_value::Value::StringValue(arr.value(row).to_string())),
            })
        }
        DataType::Int32 => {
            let arr = col.as_any().downcast_ref::<Int32Array>()?;
            Some(AnyValue {
                value: Some(any_value::Value::IntValue(arr.value(row) as i64)),
            })
        }
        DataType::Int64 => {
            let arr = col.as_any().downcast_ref::<Int64Array>()?;
            Some(AnyValue {
                value: Some(any_value::Value::IntValue(arr.value(row))),
            })
        }
        DataType::Float64 => {
            let arr = col.as_any().downcast_ref::<Float64Array>()?;
            Some(AnyValue {
                value: Some(any_value::Value::DoubleValue(arr.value(row))),
            })
        }
        DataType::Boolean => {
            let arr = col.as_any().downcast_ref::<BooleanArray>()?;
            Some(AnyValue {
                value: Some(any_value::Value::BoolValue(arr.value(row))),
            })
        }
        DataType::Timestamp(_, _) => {
            // Represent timestamps as their string form for attribute readability.
            let nanos = timestamp_to_nanos(col, row)?;
            let secs = (nanos / 1_000_000_000) as i64;
            let subsec_ns = (nanos % 1_000_000_000) as u32;
            let dt = chrono::DateTime::from_timestamp(secs, subsec_ns)?;
            Some(AnyValue {
                value: Some(any_value::Value::StringValue(
                    dt.format("%Y-%m-%dT%H:%M:%S").to_string(),
                )),
            })
        }
        _ => {
            // Fallback: use Arrow's Display formatting.
            let formatter = ArrayFormatter::try_new(col, &Default::default()).ok()?;
            Some(AnyValue {
                value: Some(any_value::Value::StringValue(
                    formatter.value(row).to_string(),
                )),
            })
        }
    }
}

/// Build an ExportLogsServiceRequest from a batch of LogRecords.
pub fn build_export_request(log_records: Vec<LogRecord>) -> ExportLogsServiceRequest {
    ExportLogsServiceRequest {
        resource_logs: vec![ResourceLogs {
            resource: Some(Resource {
                attributes: vec![KeyValue {
                    key: "service.name".to_string(),
                    value: Some(AnyValue {
                        value: Some(any_value::Value::StringValue(SERVICE_NAME.to_string())),
                    }),
                }],
                dropped_attributes_count: 0,
                entity_refs: vec![],
            }),
            scope_logs: vec![ScopeLogs {
                scope: Some(InstrumentationScope {
                    name: SCOPE_NAME.to_string(),
                    version: CRATE_VERSION.to_string(),
                    attributes: vec![],
                    dropped_attributes_count: 0,
                }),
                log_records,
                schema_url: String::new(),
            }],
            schema_url: String::new(),
        }],
    }
}
