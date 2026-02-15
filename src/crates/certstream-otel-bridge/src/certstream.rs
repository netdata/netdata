use std::time::Duration;

use futures::{SinkExt, StreamExt};
use serde::Deserialize;
use tokio::sync::mpsc;
use tokio_tungstenite::connect_async;
use tokio_tungstenite::tungstenite::Message as WsMessage;
use tracing::{error, info, warn};

/// Top-level CertStream message envelope.
#[derive(Deserialize, Debug)]
pub struct Message {
    #[allow(dead_code)]
    pub message_type: String,
    pub data: Option<CertData>,
}

#[allow(dead_code)]
#[derive(Deserialize, Debug)]
pub struct CertData {
    pub update_type: String,
    pub cert_index: u64,
    pub cert_link: String,
    pub seen: f64,
    pub source: Source,
    pub leaf_cert: LeafCert,
}

#[allow(dead_code)]
#[derive(Deserialize, Debug)]
pub struct Source {
    pub name: String,
    pub url: String,
}

#[allow(dead_code)]
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

#[allow(dead_code)]
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
/// Heartbeat messages (where `data` is `None`) are silently skipped.
/// Sends WebSocket pings every 30 s to satisfy server keepalive requirements.
/// Returns when the WebSocket connection closes or errors.
pub async fn connect(
    url: &str,
    tx: mpsc::Sender<(CertData, serde_json::Value)>,
) -> anyhow::Result<()> {
    info!("Connecting to CertStream: {}", url);

    let (ws_stream, _response) = connect_async(url).await?;
    info!("Connected to CertStream");

    let (mut write, mut read) = ws_stream.split();

    let mut ping_timer = tokio::time::interval(PING_INTERVAL);
    // Consume the immediate first tick.
    ping_timer.tick().await;

    loop {
        tokio::select! {
            msg = read.next() => {
                let Some(msg) = msg else {
                    info!("WebSocket stream ended");
                    break;
                };

                match msg {
                    Ok(WsMessage::Text(text)) => {
                        let raw_json: serde_json::Value = match serde_json::from_str(text.as_ref()) {
                            Ok(v) => v,
                            Err(e) => {
                                warn!("Failed to parse message as JSON: {}", e);
                                continue;
                            }
                        };

                        let message: Message = match serde_json::from_str(text.as_ref()) {
                            Ok(m) => m,
                            Err(e) => {
                                warn!("Failed to deserialize CertStream message: {}", e);
                                continue;
                            }
                        };

                        // Skip heartbeat messages (no data payload).
                        let data = match message.data {
                            Some(d) => d,
                            None => continue,
                        };

                        if tx.send((data, raw_json)).await.is_err() {
                            info!("Receiver dropped, stopping WebSocket reader");
                            break;
                        }
                    }
                    Ok(WsMessage::Close(_)) => {
                        info!("WebSocket closed by server");
                        break;
                    }
                    Err(e) => {
                        error!("WebSocket error: {}", e);
                        break;
                    }
                    _ => {}
                }
            }
            _ = ping_timer.tick() => {
                if let Err(e) = write.send(WsMessage::Ping(vec![].into())).await {
                    error!("Failed to send WebSocket ping: {}", e);
                    break;
                }
            }
        }
    }

    Ok(())
}
