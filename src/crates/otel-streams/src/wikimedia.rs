use std::ops::ControlFlow;

use opentelemetry_proto::tonic::logs::v1::LogRecord;
use serde::Deserialize;
use tokio::sync::mpsc;
use tracing::{info, warn};

use crate::Source;
use crate::otel::{SEVERITY_INFO, bool_val, json_to_any_value, kv, now_unix_nanos, str_val};

/// A Wikimedia EventStreams `recentchange` event.
///
/// Only the fields promoted to OTLP attributes are typed; the rest (title,
/// comment, user, revision, length, log_params, ...) travel in the body via the
/// raw JSON. `log_type`/`log_action` appear only on `type: "log"` events.
#[derive(Deserialize, Debug)]
pub struct Event {
    #[serde(rename = "type", default)]
    pub change_type: String,
    #[serde(default)]
    pub wiki: String,
    #[serde(default)]
    pub server_name: String,
    #[serde(default)]
    pub namespace: Option<i64>,
    #[serde(default)]
    pub bot: Option<bool>,
    #[serde(default)]
    pub minor: Option<bool>,
    #[serde(default)]
    pub log_type: Option<String>,
    #[serde(default)]
    pub log_action: Option<String>,
    /// Epoch seconds of the change.
    #[serde(default)]
    pub timestamp: Option<i64>,
    #[serde(default)]
    pub meta: Option<Meta>,
}

#[derive(Deserialize, Debug)]
pub struct Meta {
    #[serde(default)]
    pub domain: String,
}

/// Connect to a Wikimedia EventStreams SSE endpoint and forward parsed events.
/// Returns when the stream closes or errors.
pub async fn connect(
    url: &str,
    tx: mpsc::Sender<(Event, serde_json::Value)>,
) -> anyhow::Result<()> {
    crate::sse::run("Wikimedia", url, move |raw_json| {
        let tx = tx.clone();
        async move {
            let event: Event = match serde_json::from_value(raw_json.clone()) {
                Ok(e) => e,
                Err(e) => {
                    warn!("Failed to deserialize event: {e}");
                    return ControlFlow::Continue(());
                }
            };

            // Skip Wikimedia's synthetic monitoring ("canary") events.
            if event.meta.as_ref().map(|m| m.domain.as_str()) == Some("canary") {
                return ControlFlow::Continue(());
            }

            if tx.send((event, raw_json)).await.is_err() {
                info!("Receiver dropped, stopping SSE reader");
                return ControlFlow::Break(());
            }

            ControlFlow::Continue(())
        }
    })
    .await
}

pub struct Wikimedia;

/// Top-level keys promoted to attributes; stripped from the body to avoid
/// duplicating them (jetstream-style mapping).
const PROMOTED_KEYS: &[&str] = &[
    "type",
    "wiki",
    "server_name",
    "namespace",
    "bot",
    "minor",
    "log_type",
    "log_action",
];

fn strip_promoted_keys(raw_json: &serde_json::Value) -> serde_json::Value {
    let mut body = raw_json.clone();
    if let Some(obj) = body.as_object_mut() {
        for key in PROMOTED_KEYS {
            obj.remove(*key);
        }
    }
    body
}

impl Source for Wikimedia {
    const SERVICE_NAME: &'static str = "wikimedia-recentchange";
    const SCOPE_NAME: &'static str = "wikimedia-otel-bridge";
    const SCOPE_VERSION: &'static str = env!("CARGO_PKG_VERSION");

    type Event = Event;

    fn event_to_log_record(event: &Event, raw_json: &serde_json::Value) -> LogRecord {
        let mut attributes = vec![
            kv("wiki.change_type", str_val(&event.change_type)),
            kv("wiki.name", str_val(&event.wiki)),
            kv("wiki.server_name", str_val(&event.server_name)),
        ];
        // namespace is a small enum-like integer; promote as a string so it
        // faces the same way as the other categorical dimensions.
        if let Some(ns) = event.namespace {
            attributes.push(kv("wiki.namespace", str_val(&ns.to_string())));
        }
        if let Some(bot) = event.bot {
            attributes.push(kv("wiki.bot", bool_val(bot)));
        }
        if let Some(minor) = event.minor {
            attributes.push(kv("wiki.minor", bool_val(minor)));
        }
        if let Some(log_type) = &event.log_type {
            attributes.push(kv("wiki.log_type", str_val(log_type)));
        }
        if let Some(log_action) = &event.log_action {
            attributes.push(kv("wiki.log_action", str_val(log_action)));
        }

        // Change time (epoch seconds) as the record time; now for observed —
        // matching the other live sources. Fall back to now if absent.
        let time_unix_nano = match event.timestamp {
            Some(ts) if ts > 0 => ts as u64 * 1_000_000_000,
            _ => now_unix_nanos(),
        };

        LogRecord {
            time_unix_nano,
            observed_time_unix_nano: now_unix_nanos(),
            severity_number: SEVERITY_INFO,
            severity_text: "INFO".to_string(),
            body: Some(json_to_any_value(&strip_promoted_keys(raw_json))),
            attributes,
            event_name: event.change_type.clone(),
            ..Default::default()
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use opentelemetry_proto::tonic::common::v1::any_value;

    fn map(raw: &str) -> LogRecord {
        let value: serde_json::Value = serde_json::from_str(raw).unwrap();
        let event: Event = serde_json::from_value(value.clone()).unwrap();
        Wikimedia::event_to_log_record(&event, &value)
    }

    fn attr_str(rec: &LogRecord, key: &str) -> Option<String> {
        rec.attributes
            .iter()
            .find(|kv| kv.key == key)
            .and_then(|kv| match kv.value.as_ref()?.value.as_ref()? {
                any_value::Value::StringValue(s) => Some(s.clone()),
                _ => None,
            })
    }

    fn attr_bool(rec: &LogRecord, key: &str) -> Option<bool> {
        rec.attributes
            .iter()
            .find(|kv| kv.key == key)
            .and_then(|kv| match kv.value.as_ref()?.value.as_ref()? {
                any_value::Value::BoolValue(b) => Some(*b),
                _ => None,
            })
    }

    const EDIT: &str = r#"{"type":"edit","namespace":0,"bot":false,"minor":true,
        "server_name":"en.wikipedia.org","wiki":"enwiki","user":"Ivtue",
        "title":"Gary Spivey","comment":"short desc",
        "length":{"old":4929,"new":4930},"revision":{"old":1256451708,"new":1363071360},
        "timestamp":1783459585,"meta":{"domain":"en.wikipedia.org","stream":"mediawiki.recentchange"}}"#;

    #[test]
    fn edit_promotes_categorical_fields() {
        let rec = map(EDIT);
        assert_eq!(attr_str(&rec, "wiki.change_type").as_deref(), Some("edit"));
        assert_eq!(attr_str(&rec, "wiki.name").as_deref(), Some("enwiki"));
        assert_eq!(
            attr_str(&rec, "wiki.server_name").as_deref(),
            Some("en.wikipedia.org")
        );
        assert_eq!(attr_str(&rec, "wiki.namespace").as_deref(), Some("0"));
        assert_eq!(attr_bool(&rec, "wiki.bot"), Some(false));
        assert_eq!(attr_bool(&rec, "wiki.minor"), Some(true));
        assert_eq!(rec.event_name, "edit");
        assert_eq!(rec.time_unix_nano, 1_783_459_585_000_000_000);
    }

    #[test]
    fn edit_has_no_log_attributes() {
        let rec = map(EDIT);
        assert!(attr_str(&rec, "wiki.log_type").is_none());
        assert!(attr_str(&rec, "wiki.log_action").is_none());
    }

    #[test]
    fn log_event_promotes_log_fields_and_negative_namespace() {
        // Log events carry log_type/log_action and can sit in negative
        // namespaces (e.g. Special = -1); free-form log_params stays in body.
        let raw = r#"{"type":"log","namespace":-1,"bot":true,
            "server_name":"commons.wikimedia.org","wiki":"commonswiki",
            "log_type":"upload","log_action":"upload",
            "log_params":{"img_sha1":"abc"},"timestamp":1783459600,
            "meta":{"domain":"commons.wikimedia.org"}}"#;
        let rec = map(raw);
        assert_eq!(attr_str(&rec, "wiki.log_type").as_deref(), Some("upload"));
        assert_eq!(attr_str(&rec, "wiki.log_action").as_deref(), Some("upload"));
        assert_eq!(attr_str(&rec, "wiki.namespace").as_deref(), Some("-1"));
        assert_eq!(attr_bool(&rec, "wiki.bot"), Some(true));
        assert_eq!(rec.event_name, "log");
    }

    #[test]
    fn promoted_keys_stripped_from_body_non_promoted_kept() {
        let value: serde_json::Value = serde_json::from_str(EDIT).unwrap();
        let body = strip_promoted_keys(&value);
        for key in PROMOTED_KEYS {
            assert!(body.get(*key).is_none(), "{key} should be stripped");
        }
        // Non-promoted fields remain queryable in the body.
        assert_eq!(body["user"], "Ivtue");
        assert_eq!(body["revision"]["new"], 1363071360_i64);
    }

    #[test]
    fn missing_timestamp_falls_back_to_now() {
        let raw = r#"{"type":"new","wiki":"enwiki","server_name":"en.wikipedia.org",
            "meta":{"domain":"en.wikipedia.org"}}"#;
        let rec = map(raw);
        assert!(rec.time_unix_nano > 0);
    }

    #[test]
    fn canary_domain_detectable() {
        let value: serde_json::Value =
            serde_json::from_str(r#"{"type":"edit","meta":{"domain":"canary"}}"#).unwrap();
        let event: Event = serde_json::from_value(value).unwrap();
        assert_eq!(event.meta.as_ref().map(|m| m.domain.as_str()), Some("canary"));
    }
}
