use std::ops::ControlFlow;

use opentelemetry_proto::tonic::logs::v1::LogRecord;
use serde::Deserialize;
use serde_json::json;
use tokio::sync::mpsc;
use tracing::{info, warn};

use crate::Source;
use crate::otel::{SEVERITY_INFO, json_to_any_value, kv, now_unix_nanos, str_val};

/// A single BGP message from RIS Live (the `data` object of a `ris_message`
/// envelope).
///
/// Only the fields promoted to OTLP attributes are typed. The rest (`peer`,
/// `id`, `path`, `community`, `announcements`, `withdrawals`, `med`, ...) stay
/// untyped and travel in the log body via the raw JSON — deliberately, so a
/// string `peer_asn` or a nested AS-SET in `path` cannot break deserialization.
#[derive(Deserialize, Debug)]
pub struct RisData {
    /// Epoch seconds (float, ms precision) when the collector saw the message.
    #[serde(default)]
    pub timestamp: f64,
    /// RIS route collector, e.g. `rrc03.ripe.net`.
    #[serde(default)]
    pub host: String,
    /// Peer ASN — a string on the wire (may exceed u32 or be absent).
    #[serde(default)]
    pub peer_asn: String,
    /// BGP message type, e.g. `UPDATE`, `KEEPALIVE`, `STATE`, `OPEN`.
    #[serde(rename = "type", default)]
    pub bgp_type: String,
    /// Path origin (`IGP`/`EGP`/`INCOMPLETE`); absent on non-UPDATE messages.
    #[serde(default)]
    pub origin: Option<String>,
}

/// Build the `ris_subscribe` frame. An empty `data` object subscribes to the
/// full firehose; `host`/`msg_type` narrow it (a volume throttle).
pub fn build_subscribe(host: Option<&str>, msg_type: Option<&str>) -> String {
    let mut data = serde_json::Map::new();
    if let Some(host) = host {
        data.insert("host".to_string(), json!(host));
    }
    if let Some(msg_type) = msg_type {
        data.insert("type".to_string(), json!(msg_type));
    }
    json!({ "type": "ris_subscribe", "data": data }).to_string()
}

/// Connect to RIS Live, subscribe, and forward parsed BGP messages.
/// Returns when the WebSocket connection closes or errors.
pub async fn connect(
    url: &str,
    subscribe: String,
    tx: mpsc::Sender<(RisData, serde_json::Value)>,
) -> anyhow::Result<()> {
    crate::ws::run("RIS Live", url, Some(subscribe), None, move |raw_json| {
        let tx = tx.clone();
        async move {
            match raw_json.get("type").and_then(|t| t.as_str()) {
                Some("ris_message") => {
                    let Some(data_json) = raw_json.get("data") else {
                        warn!("ris_message without data field, skipping");
                        return ControlFlow::Continue(());
                    };
                    let data: RisData = match serde_json::from_value(data_json.clone()) {
                        Ok(d) => d,
                        Err(e) => {
                            warn!("Failed to deserialize ris_message data: {e}");
                            return ControlFlow::Continue(());
                        }
                    };
                    if tx.send((data, raw_json)).await.is_err() {
                        info!("Receiver dropped, stopping WebSocket reader");
                        return ControlFlow::Break(());
                    }
                }
                // Surface server-side errors (RIS Live sends one before dropping
                // a slow consumer) so backpressure is visible during load tests.
                Some("ris_error") => {
                    warn!(
                        "RIS Live error frame: {}",
                        raw_json.get("data").unwrap_or(&raw_json)
                    );
                }
                // ris_subscribe_ok, pong, and unknown control frames: ignore.
                _ => {}
            }
            ControlFlow::Continue(())
        }
    })
    .await
}

pub struct RisLive;

/// Data-object keys promoted to attributes; stripped from the body to avoid
/// duplicating them (jetstream-style mapping).
const PROMOTED_DATA_KEYS: &[&str] = &["host", "peer_asn", "type", "origin"];

fn strip_promoted_keys(raw_json: &serde_json::Value) -> serde_json::Value {
    let mut body = raw_json.clone();
    if let Some(data) = body.get_mut("data").and_then(|d| d.as_object_mut()) {
        for key in PROMOTED_DATA_KEYS {
            data.remove(*key);
        }
    }
    body
}

impl Source for RisLive {
    const SERVICE_NAME: &'static str = "ripe-ris-live";
    const SCOPE_NAME: &'static str = "ris-live-otel-bridge";
    const SCOPE_VERSION: &'static str = env!("CARGO_PKG_VERSION");

    type Event = RisData;

    fn event_to_log_record(data: &RisData, raw_json: &serde_json::Value) -> LogRecord {
        let mut attributes = vec![
            kv("bgp.host", str_val(&data.host)),
            kv("bgp.peer_asn", str_val(&data.peer_asn)),
            kv("bgp.type", str_val(&data.bgp_type)),
        ];
        if let Some(origin) = &data.origin {
            attributes.push(kv("bgp.origin", str_val(origin)));
        }

        // The stream carries the collector-observed time; fall back to now for
        // any frame missing it (e.g. malformed), like the other live sources.
        let time_unix_nano = if data.timestamp > 0.0 {
            (data.timestamp * 1e9) as u64
        } else {
            now_unix_nanos()
        };

        LogRecord {
            time_unix_nano,
            observed_time_unix_nano: now_unix_nanos(),
            severity_number: SEVERITY_INFO,
            severity_text: "INFO".to_string(),
            body: Some(json_to_any_value(&strip_promoted_keys(raw_json))),
            attributes,
            event_name: data.bgp_type.clone(),
            ..Default::default()
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use opentelemetry_proto::tonic::common::v1::any_value;

    fn map(raw: &str) -> LogRecord {
        let env: serde_json::Value = serde_json::from_str(raw).unwrap();
        let data: RisData = serde_json::from_value(env.get("data").unwrap().clone()).unwrap();
        RisLive::event_to_log_record(&data, &env)
    }

    fn attr(rec: &LogRecord, key: &str) -> Option<String> {
        rec.attributes
            .iter()
            .find(|kv| kv.key == key)
            .and_then(|kv| match kv.value.as_ref()?.value.as_ref()? {
                any_value::Value::StringValue(s) => Some(s.clone()),
                _ => None,
            })
    }

    #[test]
    fn build_subscribe_full_firehose() {
        assert_eq!(build_subscribe(None, None), r#"{"data":{},"type":"ris_subscribe"}"#);
    }

    #[test]
    fn build_subscribe_filtered() {
        let s = build_subscribe(Some("rrc00"), Some("UPDATE"));
        let v: serde_json::Value = serde_json::from_str(&s).unwrap();
        assert_eq!(v["data"]["host"], "rrc00");
        assert_eq!(v["data"]["type"], "UPDATE");
    }

    #[test]
    fn update_promotes_attributes_and_survives_as_set() {
        // Nested array in `path` (an AS-SET) and string peer_asn must not break
        // deserialization or body conversion.
        let raw = r#"{"type":"ris_message","data":{"timestamp":1783452279.28,"host":"rrc03.ripe.net","peer_asn":"1140","type":"UPDATE","origin":"IGP","path":[1140,[64512,64513]],"announcements":[{"prefixes":["44.32.91.0/24"]}]}}"#;
        let rec = map(raw);
        assert_eq!(attr(&rec, "bgp.host").as_deref(), Some("rrc03.ripe.net"));
        assert_eq!(attr(&rec, "bgp.peer_asn").as_deref(), Some("1140"));
        assert_eq!(attr(&rec, "bgp.type").as_deref(), Some("UPDATE"));
        assert_eq!(attr(&rec, "bgp.origin").as_deref(), Some("IGP"));
        assert_eq!(rec.event_name, "UPDATE");
        // Second-granularity check (f64 epoch-nanos loses sub-second precision).
        assert_eq!(rec.time_unix_nano / 1_000_000_000, 1783452279);
    }

    #[test]
    fn keepalive_without_origin_ok() {
        let raw = r#"{"type":"ris_message","data":{"timestamp":1783452280.0,"host":"rrc21.ripe.net","peer_asn":"3333","type":"KEEPALIVE"}}"#;
        let rec = map(raw);
        assert_eq!(attr(&rec, "bgp.type").as_deref(), Some("KEEPALIVE"));
        assert!(attr(&rec, "bgp.origin").is_none());
        assert_eq!(rec.event_name, "KEEPALIVE");
    }

    #[test]
    fn missing_timestamp_falls_back_to_now() {
        let raw = r#"{"type":"ris_message","data":{"host":"rrc00.ripe.net","peer_asn":"1","type":"UPDATE"}}"#;
        let rec = map(raw);
        assert!(rec.time_unix_nano > 0);
    }

    #[test]
    fn promoted_keys_stripped_from_body() {
        let raw = r#"{"type":"ris_message","data":{"timestamp":1.0,"host":"rrc00.ripe.net","peer_asn":"1","type":"UPDATE","origin":"IGP","med":100}}"#;
        let env: serde_json::Value = serde_json::from_str(raw).unwrap();
        let stripped = strip_promoted_keys(&env);
        let data = &stripped["data"];
        assert!(data.get("host").is_none());
        assert!(data.get("peer_asn").is_none());
        assert!(data.get("type").is_none());
        assert!(data.get("origin").is_none());
        // Non-promoted fields remain in the body.
        assert_eq!(data["med"], 100);
    }
}
