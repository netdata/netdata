use std::ops::ControlFlow;
use std::time::Duration;

use opentelemetry_proto::tonic::logs::v1::LogRecord;
use serde::Deserialize;
use tokio::sync::mpsc;
use tracing::{info, warn};

use crate::Source;
use crate::otel::{SEVERITY_INFO, json_to_any_value, now_unix_nanos};

/// Top-level CertStream message envelope.
///
/// Wire format: `{"message_type": "certificate_update" | ..., "data": {...}}`.
/// Heartbeats and unknown `message_type` values are silently skipped via the
/// `Other` variant.
#[derive(Deserialize, Debug)]
#[serde(tag = "message_type", content = "data", rename_all = "snake_case")]
pub enum Message {
    CertificateUpdate(Box<CertData>),
    #[serde(other)]
    Other,
}

#[derive(Deserialize, Debug)]
pub struct CertData {
    pub update_type: String,
    pub cert_index: u64,
    pub cert_link: String,
    pub seen: f64,
    pub source: CtSource,
    pub leaf_cert: LeafCert,
}

#[derive(Deserialize, Debug)]
pub struct CtSource {
    pub name: String,
    pub url: String,
}

#[derive(Deserialize, Debug)]
pub struct LeafCert {
    pub subject: Subject,
    pub issuer: Subject,
    #[serde(default)]
    pub all_domains: Vec<String>,
    #[serde(default)]
    pub fingerprint: String,
    #[serde(default)]
    pub serial_number: String,
    #[serde(default)]
    pub not_before: f64,
    #[serde(default)]
    pub not_after: f64,
    #[serde(default)]
    pub signature_algorithm: String,
    #[serde(default)]
    pub extensions: serde_json::Value,
}

#[derive(Deserialize, Debug)]
pub struct Subject {
    pub aggregated: Option<String>,
    #[serde(rename = "CN")]
    pub cn: Option<String>,
    #[serde(rename = "O")]
    pub o: Option<String>,
    #[serde(rename = "C")]
    pub c: Option<String>,
}

const PING_INTERVAL: Duration = Duration::from_secs(30);

/// Connect to CertStream and send parsed certificate data through the channel.
/// Heartbeats and unknown messages are silently skipped.
/// Returns when the WebSocket connection closes or errors.
pub async fn connect(
    url: &str,
    tx: mpsc::Sender<(CertData, serde_json::Value)>,
) -> anyhow::Result<()> {
    crate::ws::run("CertStream", url, Some(PING_INTERVAL), move |raw_json| {
        let tx = tx.clone();
        async move {
            let message: Message = match serde_json::from_value(raw_json.clone()) {
                Ok(m) => m,
                Err(e) => {
                    warn!("Failed to deserialize CertStream message: {e}");
                    return ControlFlow::Continue(());
                }
            };

            let data = match message {
                Message::CertificateUpdate(data) => *data,
                Message::Other => return ControlFlow::Continue(()),
            };

            if tx.send((data, raw_json)).await.is_err() {
                info!("Receiver dropped, stopping WebSocket reader");
                return ControlFlow::Break(());
            }

            ControlFlow::Continue(())
        }
    })
    .await
}

pub struct Certstream;

impl Source for Certstream {
    const SERVICE_NAME: &'static str = "certstream";
    const SCOPE_NAME: &'static str = "certstream-otel-bridge";
    const SCOPE_VERSION: &'static str = env!("CARGO_PKG_VERSION");

    type Event = CertData;

    fn event_to_log_record(data: &CertData, raw_json: &serde_json::Value) -> LogRecord {
        let time_unix_nano = (data.seen * 1e9) as u64;

        LogRecord {
            time_unix_nano,
            observed_time_unix_nano: now_unix_nanos(),
            severity_number: SEVERITY_INFO,
            severity_text: "INFO".to_string(),
            body: Some(json_to_any_value(raw_json)),
            attributes: vec![],
            event_name: "certificate_update".to_string(),
            ..Default::default()
        }
    }
}
