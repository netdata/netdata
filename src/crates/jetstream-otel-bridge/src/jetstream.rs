use futures::StreamExt;
use serde::Deserialize;
use tokio::sync::mpsc;
use tokio_tungstenite::connect_async;
use tracing::{error, info, warn};
use url::Url;

#[derive(Deserialize, Debug)]
pub struct Event {
    pub did: String,
    pub time_us: u64,
    pub kind: EventKind,
    pub commit: Option<Commit>,
    pub identity: Option<Identity>,
    pub account: Option<Account>,
}

#[derive(Deserialize, Debug)]
#[serde(rename_all = "snake_case")]
pub enum EventKind {
    Commit,
    Identity,
    Account,
}

impl std::fmt::Display for EventKind {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            EventKind::Commit => f.write_str("commit"),
            EventKind::Identity => f.write_str("identity"),
            EventKind::Account => f.write_str("account"),
        }
    }
}

#[allow(dead_code)]
#[derive(Deserialize, Debug)]
pub struct Commit {
    pub rev: String,
    pub operation: Operation,
    pub collection: String,
    pub rkey: String,
    pub record: Option<serde_json::Value>,
    pub cid: Option<String>,
}

#[derive(Deserialize, Debug)]
#[serde(rename_all = "snake_case")]
pub enum Operation {
    Create,
    Update,
    Delete,
}

impl std::fmt::Display for Operation {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            Operation::Create => f.write_str("create"),
            Operation::Update => f.write_str("update"),
            Operation::Delete => f.write_str("delete"),
        }
    }
}

#[allow(dead_code)]
#[derive(Deserialize, Debug)]
pub struct Identity {
    pub did: String,
    pub handle: Option<String>,
    pub seq: u64,
    pub time: String,
}

#[allow(dead_code)]
#[derive(Deserialize, Debug)]
pub struct Account {
    pub active: bool,
    pub did: String,
    pub seq: u64,
    pub time: String,
    pub status: Option<AccountStatus>,
}

#[derive(Deserialize, Debug)]
#[serde(rename_all = "snake_case")]
pub enum AccountStatus {
    Deactivated,
    Deleted,
    Suspended,
    TakenDown,
}

impl std::fmt::Display for AccountStatus {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            AccountStatus::Deactivated => f.write_str("deactivated"),
            AccountStatus::Deleted => f.write_str("deleted"),
            AccountStatus::Suspended => f.write_str("suspended"),
            AccountStatus::TakenDown => f.write_str("taken_down"),
        }
    }
}

pub fn build_jetstream_url(
    base_url: &str,
    collections: &Option<String>,
) -> Result<String, url::ParseError> {
    let mut url = Url::parse(base_url)?;

    if let Some(collections) = collections {
        for collection in collections.split(',') {
            let collection = collection.trim();
            if !collection.is_empty() {
                url.query_pairs_mut()
                    .append_pair("wantedCollections", collection);
            }
        }
    }

    Ok(url.to_string())
}

/// Connect to Jetstream and send parsed events through the channel.
/// Returns when the WebSocket connection closes or errors.
pub async fn connect(
    url: &str,
    tx: mpsc::Sender<(Event, serde_json::Value)>,
) -> anyhow::Result<()> {
    info!("Connecting to Jetstream: {}", url);

    let (ws_stream, _response) = connect_async(url).await?;
    info!("Connected to Jetstream");

    let (_write, mut read) = ws_stream.split();

    while let Some(msg) = read.next().await {
        match msg {
            Ok(tokio_tungstenite::tungstenite::Message::Text(text)) => {
                let raw_json: serde_json::Value = match serde_json::from_str(text.as_ref()) {
                    Ok(v) => v,
                    Err(e) => {
                        warn!("Failed to parse message as JSON: {}", e);
                        continue;
                    }
                };

                let event: Event = match serde_json::from_str(text.as_ref()) {
                    Ok(e) => e,
                    Err(e) => {
                        warn!("Failed to deserialize event: {}", e);
                        continue;
                    }
                };

                if tx.send((event, raw_json)).await.is_err() {
                    info!("Receiver dropped, stopping WebSocket reader");
                    break;
                }
            }
            Ok(tokio_tungstenite::tungstenite::Message::Close(_)) => {
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

    Ok(())
}
