use futures::StreamExt;
use reqwest_eventsource::{Event as SseEvent, EventSource};
use serde::Deserialize;
use tokio::sync::mpsc;
use tracing::{error, info, warn};

const USER_AGENT: &str = concat!("wiki-otel-bridge/", env!("CARGO_PKG_VERSION"));

/// Known Wikipedia/Wikimedia EventStreams stream names.
/// Some short aliases (e.g. revision-tags-change) are broken on the server,
/// so we use the `mediawiki.` prefixed forms where necessary.
#[allow(dead_code)]
pub const ALL_STREAMS: &[&str] = &[
    "recentchange",
    "revision-create",
    "mediawiki.revision-tags-change",
    "mediawiki.revision-visibility-change",
    "page-create",
    "page-delete",
    "page-move",
    "page-undelete",
    "page-links-change",
    "page-properties-change",
];

/// Subset of fields from a Wikimedia EventStreams event.
/// Only the fields we need for OTel attributes are deserialized;
/// the full JSON payload is forwarded as the log body.
#[allow(dead_code)]
#[derive(Deserialize, Debug)]
pub struct WikiEvent {
    pub meta: Meta,
    #[serde(rename = "type")]
    pub event_type: Option<String>,
    pub title: Option<String>,
    pub user: Option<String>,
    pub bot: Option<bool>,
    pub namespace: Option<i64>,
    pub comment: Option<String>,
}

#[allow(dead_code)]
#[derive(Deserialize, Debug)]
pub struct Meta {
    pub dt: String,
    pub domain: Option<String>,
    pub stream: Option<String>,
    pub uri: Option<String>,
    pub id: Option<String>,
}

/// Build the SSE endpoint URL from a base URL and comma-separated stream names.
pub fn build_stream_url(base_url: &str, streams: &str) -> String {
    let base = base_url.trim_end_matches('/');
    format!("{}/{}", base, streams)
}

/// Connect to Wikimedia EventStreams via SSE and send parsed events through the channel.
/// Returns when the SSE connection closes or encounters a fatal error.
pub async fn connect(
    url: &str,
    tx: mpsc::Sender<(WikiEvent, serde_json::Value)>,
) -> anyhow::Result<()> {
    info!("Connecting to Wikimedia EventStreams: {}", url);

    let client = reqwest::Client::builder().user_agent(USER_AGENT).build()?;
    let mut es = EventSource::new(client.get(url))?;

    while let Some(event) = es.next().await {
        match event {
            Ok(SseEvent::Open) => {
                info!("Connected to Wikimedia EventStreams");
            }
            Ok(SseEvent::Message(msg)) => {
                let raw_json: serde_json::Value = match serde_json::from_str(&msg.data) {
                    Ok(v) => v,
                    Err(e) => {
                        warn!("Failed to parse SSE data as JSON: {}", e);
                        continue;
                    }
                };

                let wiki_event: WikiEvent = match serde_json::from_str(&msg.data) {
                    Ok(e) => e,
                    Err(e) => {
                        warn!("Failed to deserialize WikiEvent: {}", e);
                        continue;
                    }
                };

                if tx.send((wiki_event, raw_json)).await.is_err() {
                    info!("Receiver dropped, stopping SSE reader");
                    break;
                }
            }
            Err(reqwest_eventsource::Error::StreamEnded) => {
                info!("SSE stream ended");
                break;
            }
            Err(e) => {
                error!("SSE error: {}", e);
                es.close();
                return Err(anyhow::anyhow!("SSE connection error: {}", e));
            }
        }
    }

    Ok(())
}
