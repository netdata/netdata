use opentelemetry_proto::tonic::{
    collector::logs::v1::ExportLogsServiceRequest,
    common::v1::{AnyValue, ArrayValue, InstrumentationScope, KeyValue, KeyValueList, any_value},
    logs::v1::{LogRecord, ResourceLogs, ScopeLogs},
    resource::v1::Resource,
};

pub fn str_val(s: &str) -> Option<AnyValue> {
    Some(AnyValue {
        value: Some(any_value::Value::StringValue(s.to_string())),
    })
}

pub fn bool_val(b: bool) -> Option<AnyValue> {
    Some(AnyValue {
        value: Some(any_value::Value::BoolValue(b)),
    })
}

pub fn kv(key: &str, value: Option<AnyValue>) -> KeyValue {
    KeyValue {
        key: key.to_string(),
        value,
    }
}

pub fn json_to_any_value(value: &serde_json::Value) -> AnyValue {
    match value {
        serde_json::Value::Null => AnyValue { value: None },
        serde_json::Value::Bool(b) => AnyValue {
            value: Some(any_value::Value::BoolValue(*b)),
        },
        serde_json::Value::Number(n) => {
            if let Some(i) = n.as_i64() {
                AnyValue {
                    value: Some(any_value::Value::IntValue(i)),
                }
            } else if let Some(f) = n.as_f64() {
                AnyValue {
                    value: Some(any_value::Value::DoubleValue(f)),
                }
            } else {
                AnyValue {
                    value: Some(any_value::Value::StringValue(n.to_string())),
                }
            }
        }
        serde_json::Value::String(s) => AnyValue {
            value: Some(any_value::Value::StringValue(s.clone())),
        },
        serde_json::Value::Array(arr) => AnyValue {
            value: Some(any_value::Value::ArrayValue(ArrayValue {
                values: arr.iter().map(json_to_any_value).collect(),
            })),
        },
        serde_json::Value::Object(obj) => AnyValue {
            value: Some(any_value::Value::KvlistValue(KeyValueList {
                values: obj
                    .iter()
                    .map(|(k, v)| KeyValue {
                        key: k.clone(),
                        value: Some(json_to_any_value(v)),
                    })
                    .collect(),
            })),
        },
    }
}

pub fn build_export_request(
    log_records: Vec<LogRecord>,
    service_name: &str,
    service_namespace: Option<&str>,
    scope_name: &str,
    scope_version: &str,
) -> ExportLogsServiceRequest {
    // service.name always; service.namespace only when set. The otel-ledger
    // indexer derives a file's single (namespace, name) stream from these two
    // resource attributes, so a batch must carry one identity (callers vary it
    // per invocation to create distinct streams).
    let mut attributes = vec![kv("service.name", str_val(service_name))];
    if let Some(namespace) = service_namespace {
        attributes.push(kv("service.namespace", str_val(namespace)));
    }
    ExportLogsServiceRequest {
        resource_logs: vec![ResourceLogs {
            resource: Some(Resource {
                attributes,
                dropped_attributes_count: 0,
                entity_refs: vec![],
            }),
            scope_logs: vec![ScopeLogs {
                scope: Some(InstrumentationScope {
                    name: scope_name.to_string(),
                    version: scope_version.to_string(),
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

/// OTel severity number for INFO (9 = INFO per OpenTelemetry logs severity matrix).
pub const SEVERITY_INFO: i32 = 9;

pub fn now_unix_nanos() -> u64 {
    std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .unwrap_or_default()
        .as_nanos() as u64
}

#[cfg(test)]
mod tests {
    use super::*;

    fn resource_attrs(req: &ExportLogsServiceRequest) -> Vec<(String, String)> {
        req.resource_logs[0]
            .resource
            .as_ref()
            .unwrap()
            .attributes
            .iter()
            .map(|kv| {
                let v = match kv.value.as_ref().and_then(|a| a.value.as_ref()) {
                    Some(any_value::Value::StringValue(s)) => s.clone(),
                    other => format!("{other:?}"),
                };
                (kv.key.clone(), v)
            })
            .collect()
    }

    #[test]
    fn omits_namespace_when_unset() {
        let req = build_export_request(vec![], "svc", None, "scope", "1.0");
        let attrs = resource_attrs(&req);
        assert_eq!(attrs, vec![("service.name".into(), "svc".into())]);
    }

    #[test]
    fn emits_namespace_when_set() {
        let req = build_export_request(vec![], "api", Some("prod"), "scope", "1.0");
        let attrs = resource_attrs(&req);
        assert_eq!(
            attrs,
            vec![
                ("service.name".into(), "api".into()),
                ("service.namespace".into(), "prod".into()),
            ]
        );
    }
}
