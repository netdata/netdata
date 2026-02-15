use opentelemetry_proto::tonic::{
    collector::logs::v1::ExportLogsServiceRequest,
    common::v1::{AnyValue, ArrayValue, InstrumentationScope, KeyValue, KeyValueList, any_value},
    logs::v1::{LogRecord, ResourceLogs, ScopeLogs},
    resource::v1::Resource,
};

use crate::github::GitHubEvent;

const SERVICE_NAME: &str = "github-gharchive";
const SCOPE_NAME: &str = "github-otel-bridge";
const CRATE_VERSION: &str = env!("CARGO_PKG_VERSION");
const SEVERITY_INFO: i32 = 9;

fn str_val(s: &str) -> Option<AnyValue> {
    Some(AnyValue {
        value: Some(any_value::Value::StringValue(s.to_string())),
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
        let mut s = frac.to_string();
        s.truncate(9);
        while s.len() < 9 {
            s.push('0');
        }
        s.parse().unwrap_or(0)
    } else {
        0
    };

    let days = days_from_civil(year, month as i64, day as i64)?;
    let total_secs = (days as u64) * 86400 + hour * 3600 + min * 60 + sec;
    Some(total_secs * 1_000_000_000 + frac_nanos)
}

/// Convert a civil date to days since Unix epoch.
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

/// Extract the hour-boundary timestamp (in seconds) from an ISO-8601 string.
/// Returns seconds since epoch, floored to the hour.
pub fn parse_hour_boundary_secs(dt: &str) -> Option<u64> {
    let nanos = parse_iso8601_inner(dt)?;
    let secs = nanos / 1_000_000_000;
    Some((secs / 3600) * 3600)
}

/// Compute the offset in seconds of this timestamp within its hour.
pub fn offset_within_hour_secs(dt: &str) -> f64 {
    let nanos = parse_iso8601_inner(dt).unwrap_or(0);
    let secs = nanos / 1_000_000_000;
    let hour_boundary = (secs / 3600) * 3600;
    let remainder_secs = secs - hour_boundary;
    let remainder_nanos = nanos - secs * 1_000_000_000;
    remainder_secs as f64 + remainder_nanos as f64 / 1_000_000_000.0
}

/// Keys promoted from the raw JSON to log record attributes.
const PROMOTED_ROOT_KEYS: &[&str] = &["id", "type", "public", "created_at"];

/// Strip keys that have been promoted to log record attributes from the raw JSON.
fn strip_promoted_keys(raw_json: &serde_json::Value) -> serde_json::Value {
    let mut body = raw_json.clone();
    if let Some(obj) = body.as_object_mut() {
        for key in PROMOTED_ROOT_KEYS {
            obj.remove(*key);
        }
        // Strip promoted nested keys: actor.login, repo.name, org.login
        if let Some(actor) = obj.get_mut("actor").and_then(|a| a.as_object_mut()) {
            actor.remove("login");
        }
        if let Some(repo) = obj.get_mut("repo").and_then(|r| r.as_object_mut()) {
            repo.remove("name");
        }
        if let Some(org) = obj.get_mut("org").and_then(|o| o.as_object_mut()) {
            org.remove("login");
        }
    }
    body
}

/// Convert a GitHub event into an OTel LogRecord.
pub fn event_to_log_record(event: &GitHubEvent, raw_json: &serde_json::Value) -> LogRecord {
    let now_ns = std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .unwrap_or_default()
        .as_nanos() as u64;

    // time_unix_nano = wall-clock "now" (so the UI histogram shows current traffic)
    // observed_time_unix_nano = original event timestamp from created_at
    let original_ns = parse_iso8601_to_nanos(&event.created_at);

    let mut attributes = vec![
        kv("github.event.type", str_val(&event.event_type)),
        kv("github.actor", str_val(&event.actor.login)),
        kv("github.repo", str_val(&event.repo.name)),
        kv("github.public", bool_val(event.public)),
    ];

    if let Some(org) = &event.org {
        attributes.push(kv("github.org", str_val(&org.login)));
    }

    let body = strip_promoted_keys(raw_json);

    LogRecord {
        time_unix_nano: now_ns,
        observed_time_unix_nano: original_ns,
        severity_number: SEVERITY_INFO,
        severity_text: "INFO".to_string(),
        body: Some(json_to_any_value(&body)),
        attributes,
        event_name: event.event_type.clone(),
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
