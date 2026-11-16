//! The profiling-only fields that opentelemetry-proto 0.33 (proto v1.10.0)
//! added to the common messages, `KeyValue.key_strindex` and the
//! `AnyValue` `string_value_strindex` kind, must not change what is written
//! to the WAL.
//!
//! opentelemetry-proto 0.31 skipped both as unknown protobuf fields: the
//! same bytes decoded to the plain key and an `AnyValue` with no value.
//! Each test builds a request in both forms and requires byte-identical
//! frame payloads, so files written before and after the upgrade agree.

use ng_flatten::{prepare_log_frame, prepare_trace_frame};
use opentelemetry_proto::tonic::collector::logs::v1::ExportLogsServiceRequest;
use opentelemetry_proto::tonic::collector::trace::v1::ExportTraceServiceRequest;
use opentelemetry_proto::tonic::common::v1::{
    AnyValue, ArrayValue, InstrumentationScope, KeyValue, KeyValueList, any_value::Value as Av,
};
use opentelemetry_proto::tonic::logs::v1::{LogRecord, ResourceLogs, ScopeLogs};
use opentelemetry_proto::tonic::resource::v1::Resource;
use opentelemetry_proto::tonic::trace::v1::{ResourceSpans, ScopeSpans, Span, span};

const TIME_NS: u64 = 1_700_000_000_000_000_000;

/// How a request carries the profiling-only fields.
#[derive(Clone, Copy)]
enum Form {
    /// As 0.33 decodes them.
    WithFields,
    /// As 0.31 decoded the same bytes.
    Skipped,
}

impl Form {
    /// The value of an attribute whose `AnyValue` holds only a string-table
    /// reference.
    fn strindex_value(self) -> AnyValue {
        match self {
            Form::WithFields => AnyValue {
                value: Some(Av::StringValueStrindex(7)),
            },
            Form::Skipped => AnyValue { value: None },
        }
    }

    fn key_strindex(self) -> i32 {
        match self {
            Form::WithFields => 9,
            Form::Skipped => 0,
        }
    }

    /// Attributes covering every place the two fields can appear: a
    /// top-level value, a key index next to a real value, and both inside a
    /// kvlist and an array.
    fn attributes(self) -> Vec<KeyValue> {
        let nested = KeyValue {
            key: "inner".into(),
            value: Some(self.strindex_value()),
            key_strindex: self.key_strindex(),
        };
        vec![
            KeyValue {
                key: "ref".into(),
                value: Some(self.strindex_value()),
                key_strindex: 0,
            },
            KeyValue {
                key: "indexed.key".into(),
                value: Some(AnyValue {
                    value: Some(Av::StringValue("v".into())),
                }),
                key_strindex: self.key_strindex(),
            },
            KeyValue {
                key: "map".into(),
                value: Some(AnyValue {
                    value: Some(Av::KvlistValue(KeyValueList {
                        values: vec![nested],
                    })),
                }),
                key_strindex: 0,
            },
            KeyValue {
                key: "list".into(),
                value: Some(AnyValue {
                    value: Some(Av::ArrayValue(ArrayValue {
                        values: vec![
                            self.strindex_value(),
                            AnyValue {
                                value: Some(Av::IntValue(1)),
                            },
                        ],
                    })),
                }),
                key_strindex: 0,
            },
        ]
    }

    fn resource(self) -> Resource {
        Resource {
            attributes: self.attributes(),
            ..Default::default()
        }
    }

    fn scope(self) -> InstrumentationScope {
        InstrumentationScope {
            name: "scope".into(),
            attributes: self.attributes(),
            ..Default::default()
        }
    }

    fn log_request(self) -> ExportLogsServiceRequest {
        ExportLogsServiceRequest {
            resource_logs: vec![ResourceLogs {
                resource: Some(self.resource()),
                scope_logs: vec![ScopeLogs {
                    scope: Some(self.scope()),
                    log_records: vec![LogRecord {
                        time_unix_nano: TIME_NS,
                        severity_number: 9,
                        body: Some(self.strindex_value()),
                        attributes: self.attributes(),
                        ..Default::default()
                    }],
                    ..Default::default()
                }],
                ..Default::default()
            }],
        }
    }

    fn trace_request(self) -> ExportTraceServiceRequest {
        ExportTraceServiceRequest {
            resource_spans: vec![ResourceSpans {
                resource: Some(self.resource()),
                scope_spans: vec![ScopeSpans {
                    scope: Some(self.scope()),
                    spans: vec![Span {
                        trace_id: vec![1; 16],
                        span_id: vec![2; 8],
                        name: "span".into(),
                        start_time_unix_nano: TIME_NS,
                        end_time_unix_nano: TIME_NS + 1_000,
                        attributes: self.attributes(),
                        events: vec![span::Event {
                            time_unix_nano: TIME_NS + 10,
                            name: "event".into(),
                            attributes: self.attributes(),
                            ..Default::default()
                        }],
                        links: vec![span::Link {
                            trace_id: vec![3; 16],
                            span_id: vec![4; 8],
                            attributes: self.attributes(),
                            ..Default::default()
                        }],
                        ..Default::default()
                    }],
                    ..Default::default()
                }],
                ..Default::default()
            }],
        }
    }
}

#[test]
fn log_frame_bytes_ignore_the_profiling_fields() {
    let with = prepare_log_frame(Form::WithFields.log_request(), TIME_NS, None).unwrap();
    let skipped = prepare_log_frame(Form::Skipped.log_request(), TIME_NS, None).unwrap();
    assert_eq!(with.records, 1);
    assert!(!with.data.is_empty());
    assert_eq!(with.data, skipped.data);
}

#[test]
fn trace_frame_bytes_ignore_the_profiling_fields() {
    let with = prepare_trace_frame(Form::WithFields.trace_request(), TIME_NS, None).unwrap();
    let skipped = prepare_trace_frame(Form::Skipped.trace_request(), TIME_NS, None).unwrap();
    assert_eq!(with.records, 1);
    assert!(!with.data.is_empty());
    assert_eq!(with.data, skipped.data);
}
