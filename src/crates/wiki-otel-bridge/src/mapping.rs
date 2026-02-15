use opentelemetry_proto::tonic::{
    collector::logs::v1::ExportLogsServiceRequest,
    common::v1::{AnyValue, ArrayValue, InstrumentationScope, KeyValue, KeyValueList, any_value},
    logs::v1::{LogRecord, ResourceLogs, ScopeLogs},
    resource::v1::Resource,
};

use crate::wiki::WikiEvent;

const SERVICE_NAME: &str = "wikimedia-eventstreams";
const SCOPE_NAME: &str = "wiki-otel-bridge";
const CRATE_VERSION: &str = env!("CARGO_PKG_VERSION");
const SEVERITY_INFO: i32 = 9;

fn str_val(s: &str) -> Option<AnyValue> {
    Some(AnyValue {
        value: Some(any_value::Value::StringValue(s.to_string())),
    })
}

fn int_val(i: i64) -> Option<AnyValue> {
    Some(AnyValue {
        value: Some(any_value::Value::IntValue(i)),
    })
}

fn bool_val(b: bool) -> Option<AnyValue> {
    Some(AnyValue {
        value: Some(any_value::Value::BoolValue(b)),
    })
}

fn kv(key: &str, value: Option<AnyValue>) -> KeyValue {
    KeyValue {
        key: key.to_string(),
        value,
    }
}

/// Convert a serde_json::Value into an OTel AnyValue.
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

/// Parse an ISO-8601 timestamp string to nanoseconds since Unix epoch.
/// Falls back to the current time if parsing fails.
fn parse_iso8601_to_nanos(dt: &str) -> u64 {
    // Try parsing with chrono-style manual approach: the timestamps look like
    // "2024-01-15T12:34:56Z" or "2024-01-15T12:34:56.123Z"
    // We parse by splitting on 'T', 'Z', '-', ':', '.'
    parse_iso8601_inner(dt).unwrap_or_else(|| {
        std::time::SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)
            .unwrap_or_default()
            .as_nanos() as u64
    })
}

fn parse_iso8601_inner(dt: &str) -> Option<u64> {
    // Expected format: "2024-01-15T12:34:56Z" or "2024-01-15T12:34:56.123Z"
    let dt = dt.trim();
    let (date_part, time_part) = dt.split_once('T')?;
    let time_part = time_part.strip_suffix('Z').unwrap_or(time_part);

    let mut date_iter = date_part.split('-');
    let year: i64 = date_iter.next()?.parse().ok()?;
    let month: u64 = date_iter.next()?.parse().ok()?;
    let day: u64 = date_iter.next()?.parse().ok()?;

    let (time_hms, frac_str) = match time_part.split_once('.') {
        Some((hms, frac)) => (hms, Some(frac)),
        None => (time_part, None),
    };

    let mut time_iter = time_hms.split(':');
    let hour: u64 = time_iter.next()?.parse().ok()?;
    let min: u64 = time_iter.next()?.parse().ok()?;
    let sec: u64 = time_iter.next()?.parse().ok()?;

    let frac_nanos: u64 = if let Some(frac) = frac_str {
        // Pad or truncate to 9 digits
        let mut s = frac.to_string();
        s.truncate(9);
        while s.len() < 9 {
            s.push('0');
        }
        s.parse().unwrap_or(0)
    } else {
        0
    };

    // Days from epoch (1970-01-01) — simplified calculation
    let days = days_from_civil(year, month as i64, day as i64)?;
    let total_secs = (days as u64) * 86400 + hour * 3600 + min * 60 + sec;
    Some(total_secs * 1_000_000_000 + frac_nanos)
}

/// Convert a civil date to days since Unix epoch.
/// Algorithm from Howard Hinnant's chrono-compatible date algorithms.
fn days_from_civil(year: i64, month: i64, day: i64) -> Option<u64> {
    let y = if month <= 2 { year - 1 } else { year };
    let era = y.div_euclid(400);
    let yoe = y.rem_euclid(400) as u64;
    let doy = {
        let m = if month > 2 { month - 3 } else { month + 9 };
        (153 * m as u64 + 2) / 5 + day as u64 - 1
    };
    let doe = yoe * 365 + yoe / 4 - yoe / 100 + doy;
    let days = era * 146097 + doe as i64 - 719468;
    if days < 0 { None } else { Some(days as u64) }
}

/// Keys promoted from the top-level JSON object to log record attributes or
/// timestamp fields. These are stripped from the body to avoid duplication.
const PROMOTED_ROOT_KEYS: &[&str] = &["type", "title", "user", "bot", "namespace"];

/// Keys promoted from the `meta` sub-object to log record attributes or
/// timestamp fields.
const PROMOTED_META_KEYS: &[&str] = &["dt", "stream", "domain"];

/// Strip keys that have been promoted to log record attributes / timestamp
/// from the raw JSON so the body carries only the remaining fields.
fn strip_promoted_keys(raw_json: &serde_json::Value) -> serde_json::Value {
    let mut body = raw_json.clone();
    if let Some(obj) = body.as_object_mut() {
        for key in PROMOTED_ROOT_KEYS {
            obj.remove(*key);
        }
        if let Some(meta) = obj.get_mut("meta").and_then(|m| m.as_object_mut()) {
            for key in PROMOTED_META_KEYS {
                meta.remove(*key);
            }
        }
    }
    body
}

/// Convert a Wikimedia EventStreams event into an OTel LogRecord.
pub fn event_to_log_record(event: &WikiEvent, raw_json: &serde_json::Value) -> LogRecord {
    let now_ns = std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .unwrap_or_default()
        .as_nanos() as u64;

    let time_unix_nano = parse_iso8601_to_nanos(&event.meta.dt);

    let mut attributes = Vec::new();

    if let Some(stream) = &event.meta.stream {
        attributes.push(kv("wiki.stream", str_val(stream)));
    }
    if let Some(domain) = &event.meta.domain {
        attributes.push(kv("wiki.domain", str_val(domain)));
    }
    if let Some(event_type) = &event.event_type {
        attributes.push(kv("wiki.type", str_val(event_type)));
    }
    if let Some(title) = &event.title {
        attributes.push(kv("wiki.title", str_val(title)));
    }
    if let Some(user) = &event.user {
        attributes.push(kv("wiki.user", str_val(user)));
    }
    if let Some(bot) = event.bot {
        attributes.push(kv("wiki.bot", bool_val(bot)));
    }
    if let Some(ns) = event.namespace {
        attributes.push(kv("wiki.namespace", int_val(ns)));
    }

    let event_name = event
        .meta
        .stream
        .as_deref()
        .or(event.event_type.as_deref())
        .unwrap_or("unknown")
        .to_string();

    let body = strip_promoted_keys(raw_json);

    LogRecord {
        time_unix_nano,
        observed_time_unix_nano: now_ns,
        severity_number: SEVERITY_INFO,
        severity_text: "INFO".to_string(),
        body: Some(json_to_any_value(&body)),
        attributes,
        event_name,
        ..Default::default()
    }
}

/// Build an ExportLogsServiceRequest from a batch of LogRecords.
pub fn build_export_request(log_records: Vec<LogRecord>) -> ExportLogsServiceRequest {
    ExportLogsServiceRequest {
        resource_logs: vec![ResourceLogs {
            resource: Some(Resource {
                attributes: vec![kv("service.name", str_val(SERVICE_NAME))],
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
