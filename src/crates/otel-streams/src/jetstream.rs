use std::ops::ControlFlow;

use opentelemetry_proto::tonic::logs::v1::LogRecord;
use serde::Deserialize;
use tokio::sync::mpsc;
use tracing::{info, warn};
use url::Url;

use crate::Source;
use crate::otel::{SEVERITY_INFO, bool_val, json_to_any_value, kv, now_unix_nanos, str_val};

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

#[derive(Deserialize, Debug)]
pub struct Identity {
    pub did: String,
    pub handle: Option<String>,
    pub seq: u64,
    pub time: String,
}

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
    collections: Option<&str>,
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
    crate::ws::run("Jetstream", url, None, None, move |raw_json| {
        let tx = tx.clone();
        async move {
            let event: Event = match serde_json::from_value(raw_json.clone()) {
                Ok(e) => e,
                Err(e) => {
                    warn!("Failed to deserialize event: {e}");
                    return ControlFlow::Continue(());
                }
            };

            if tx.send((event, raw_json)).await.is_err() {
                info!("Receiver dropped, stopping WebSocket reader");
                return ControlFlow::Break(());
            }

            ControlFlow::Continue(())
        }
    })
    .await
}

pub struct Jetstream;

const PROMOTED_ROOT_KEYS: &[&str] = &["did", "kind", "time_us"];
const PROMOTED_COMMIT_KEYS: &[&str] = &["operation", "collection", "rkey", "rev", "cid"];
const PROMOTED_IDENTITY_KEYS: &[&str] = &["handle"];
const PROMOTED_ACCOUNT_KEYS: &[&str] = &["active", "status"];

fn strip_promoted_keys(raw_json: &serde_json::Value) -> serde_json::Value {
    let mut body = raw_json.clone();
    if let Some(obj) = body.as_object_mut() {
        for key in PROMOTED_ROOT_KEYS {
            obj.remove(*key);
        }
        if let Some(commit) = obj.get_mut("commit").and_then(|c| c.as_object_mut()) {
            for key in PROMOTED_COMMIT_KEYS {
                commit.remove(*key);
            }
        }
        if let Some(identity) = obj.get_mut("identity").and_then(|i| i.as_object_mut()) {
            for key in PROMOTED_IDENTITY_KEYS {
                identity.remove(*key);
            }
        }
        if let Some(account) = obj.get_mut("account").and_then(|a| a.as_object_mut()) {
            for key in PROMOTED_ACCOUNT_KEYS {
                account.remove(*key);
            }
        }
    }
    body
}

impl Source for Jetstream {
    const SERVICE_NAME: &'static str = "bluesky-jetstream";
    const SCOPE_NAME: &'static str = "jetstream-otel-bridge";
    const SCOPE_VERSION: &'static str = env!("CARGO_PKG_VERSION");

    type Event = Event;

    fn event_to_log_record(event: &Event, raw_json: &serde_json::Value) -> LogRecord {
        let mut attributes = vec![
            kv("bluesky.did", str_val(&event.did)),
            kv("bluesky.event.kind", str_val(&event.kind.to_string())),
        ];

        if let Some(commit) = &event.commit {
            attributes.push(kv(
                "bluesky.commit.operation",
                str_val(&commit.operation.to_string()),
            ));
            attributes.push(kv("bluesky.commit.collection", str_val(&commit.collection)));
            attributes.push(kv("bluesky.commit.rkey", str_val(&commit.rkey)));
            attributes.push(kv("bluesky.commit.rev", str_val(&commit.rev)));
            if let Some(cid) = &commit.cid {
                attributes.push(kv("bluesky.commit.cid", str_val(cid)));
            }
        }

        if let Some(identity) = &event.identity {
            if let Some(handle) = &identity.handle {
                attributes.push(kv("bluesky.identity.handle", str_val(handle)));
            }
        }

        if let Some(account) = &event.account {
            attributes.push(kv("bluesky.account.active", bool_val(account.active)));
            if let Some(status) = &account.status {
                attributes.push(kv("bluesky.account.status", str_val(&status.to_string())));
            }
        }

        let body = strip_promoted_keys(raw_json);

        LogRecord {
            time_unix_nano: event.time_us * 1000,
            observed_time_unix_nano: now_unix_nanos(),
            severity_number: SEVERITY_INFO,
            severity_text: "INFO".to_string(),
            body: Some(json_to_any_value(&body)),
            attributes,
            event_name: event.kind.to_string(),
            ..Default::default()
        }
    }
}
