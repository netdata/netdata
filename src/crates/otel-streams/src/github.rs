use std::io::{BufRead, BufReader};
use std::time::{Duration, Instant};

use flate2::read::GzDecoder;
use opentelemetry_proto::tonic::logs::v1::LogRecord;
use serde::Deserialize;
use tokio::sync::mpsc;
use tracing::{error, info, warn};

use crate::Source;
use crate::otel::{SEVERITY_INFO, bool_val, json_to_any_value, kv, now_unix_nanos, str_val};

pub struct GitHub;

#[derive(Debug, Deserialize)]
pub struct GitHubEvent {
    pub id: String,
    #[serde(rename = "type")]
    pub event_type: String,
    pub actor: Actor,
    pub repo: Repo,
    pub public: bool,
    pub created_at: String,
    pub org: Option<Org>,
}

#[derive(Debug, Deserialize)]
pub struct Actor {
    pub login: String,
}

#[derive(Debug, Deserialize)]
pub struct Repo {
    pub name: String,
}

#[derive(Debug, Deserialize)]
pub struct Org {
    pub login: String,
}

struct HourCursor {
    year: i32,
    month: u32,
    day: u32,
    hour: u32,
}

impl HourCursor {
    fn parse(s: &str) -> anyhow::Result<Self> {
        let parts: Vec<&str> = s.split('-').collect();
        if parts.len() != 4 {
            anyhow::bail!("Expected YYYY-MM-DD-H format, got: {s}");
        }
        Ok(Self {
            year: parts[0].parse()?,
            month: parts[1].parse()?,
            day: parts[2].parse()?,
            hour: parts[3].parse()?,
        })
    }

    fn previous_hour() -> Self {
        use std::time::{SystemTime, UNIX_EPOCH};
        let secs = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .unwrap_or_default()
            .as_secs()
            .saturating_sub(3600);
        let days = secs / 86400;
        let day_secs = secs % 86400;
        let hour = (day_secs / 3600) as u32;
        let (y, m, d) = days_to_civil(days as i64);
        Self {
            year: y as i32,
            month: m as u32,
            day: d as u32,
            hour,
        }
    }

    fn url(&self) -> String {
        format!(
            "https://data.gharchive.org/{:04}-{:02}-{:02}-{}.json.gz",
            self.year, self.month, self.day, self.hour
        )
    }

    fn label(&self) -> String {
        format!(
            "{:04}-{:02}-{:02}-{}",
            self.year, self.month, self.day, self.hour
        )
    }

    fn advance(&mut self) {
        self.hour += 1;
        if self.hour >= 24 {
            self.hour = 0;
            self.day += 1;
            let days_in_month = days_in_month(self.year, self.month);
            if self.day > days_in_month {
                self.day = 1;
                self.month += 1;
                if self.month > 12 {
                    self.month = 1;
                    self.year += 1;
                }
            }
        }
    }
}

fn days_in_month(year: i32, month: u32) -> u32 {
    match month {
        1 | 3 | 5 | 7 | 8 | 10 | 12 => 31,
        4 | 6 | 9 | 11 => 30,
        2 => {
            if (year % 4 == 0 && year % 100 != 0) || year % 400 == 0 {
                29
            } else {
                28
            }
        }
        _ => 30,
    }
}

fn days_to_civil(days: i64) -> (i64, u64, u64) {
    let z = days + 719468;
    let era = z.div_euclid(146097);
    let doe = z.rem_euclid(146097) as u64;
    let yoe = (doe - doe / 1460 + doe / 36524 - doe / 146096) / 365;
    let y = yoe as i64 + era * 400;
    let doy = doe - (365 * yoe + yoe / 4 - yoe / 100);
    let mp = (5 * doy + 2) / 153;
    let d = doy - (153 * mp + 2) / 5 + 1;
    let m = if mp < 10 { mp + 3 } else { mp - 9 };
    let y = if m <= 2 { y + 1 } else { y };
    (y, m, d)
}

const PROMOTED_ROOT_KEYS: &[&str] = &["id", "type", "public", "created_at"];

fn strip_promoted_keys(raw_json: &serde_json::Value) -> serde_json::Value {
    let mut body = raw_json.clone();
    if let Some(obj) = body.as_object_mut() {
        for key in PROMOTED_ROOT_KEYS {
            obj.remove(*key);
        }
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

fn parse_iso8601_to_nanos(dt: &str) -> u64 {
    parse_iso8601_inner(dt).unwrap_or_else(now_unix_nanos)
}

fn parse_iso8601_inner(dt: &str) -> Option<u64> {
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
    let total_secs = days * 86400 + hour * 3600 + min * 60 + sec;
    Some(total_secs * 1_000_000_000 + frac_nanos)
}

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

/// Download, decompress, parse, and replay events from GH Archive.
/// Loops forever advancing hour-by-hour. Handles download failures with
/// per-error-type retries (404 → 60s, other → 30s).
pub async fn replay_loop(
    start: Option<String>,
    rate: u64,
    tx: mpsc::Sender<(GitHubEvent, serde_json::Value)>,
) -> anyhow::Result<()> {
    let mut cursor = match start {
        Some(s) => match HourCursor::parse(&s) {
            Ok(c) => c,
            Err(e) => {
                return Err(e);
            }
        },
        None => HourCursor::previous_hour(),
    };

    let client = reqwest::Client::new();

    loop {
        let url = cursor.url();
        let label = cursor.label();
        info!(hour = %label, "Downloading archive");

        let events = match download_and_parse(&client, &url).await {
            Ok(events) => events,
            Err(DownloadError::NotReady) => {
                info!(hour = %label, "Archive not ready (404), retrying in 60s");
                tokio::time::sleep(Duration::from_secs(60)).await;
                continue;
            }
            Err(DownloadError::Other(e)) => {
                error!(hour = %label, error = %e, "Download failed, retrying in 30s");
                tokio::time::sleep(Duration::from_secs(30)).await;
                continue;
            }
        };

        let count = events.len();
        info!(hour = %label, count, "Parsed events, starting replay");

        let interval = if rate > 0 {
            Some(Duration::from_secs_f64(1.0 / rate as f64))
        } else {
            None
        };

        let replay_start = Instant::now();

        for (i, (event, raw_json)) in events.into_iter().enumerate() {
            if let Some(interval) = interval {
                let target = replay_start + interval * i as u32;
                let now = Instant::now();
                if target > now {
                    tokio::time::sleep(target - now).await;
                }
            }

            if tx.send((event, raw_json)).await.is_err() {
                return Ok(());
            }
        }

        info!(hour = %label, count, "Replay complete");
        cursor.advance();
    }
}

enum DownloadError {
    NotReady,
    Other(anyhow::Error),
}

async fn download_and_parse(
    client: &reqwest::Client,
    url: &str,
) -> Result<Vec<(GitHubEvent, serde_json::Value)>, DownloadError> {
    let response = client
        .get(url)
        .send()
        .await
        .map_err(|e| DownloadError::Other(e.into()))?;

    if response.status() == reqwest::StatusCode::NOT_FOUND {
        return Err(DownloadError::NotReady);
    }

    if !response.status().is_success() {
        return Err(DownloadError::Other(anyhow::anyhow!(
            "HTTP {}",
            response.status()
        )));
    }

    let compressed = response
        .bytes()
        .await
        .map_err(|e| DownloadError::Other(e.into()))?;

    info!(compressed_bytes = compressed.len(), "Downloaded archive");

    let events = tokio::task::spawn_blocking(move || {
        let decoder = GzDecoder::new(&compressed[..]);
        let reader = BufReader::new(decoder);
        let mut events = Vec::new();

        for line in reader.lines() {
            let line = match line {
                Ok(l) => l,
                Err(e) => {
                    warn!(error = %e, "Skipping malformed line");
                    continue;
                }
            };

            if line.is_empty() {
                continue;
            }

            let raw_json: serde_json::Value = match serde_json::from_str(&line) {
                Ok(v) => v,
                Err(e) => {
                    warn!(error = %e, "Skipping unparseable JSON line");
                    continue;
                }
            };

            let event: GitHubEvent = match serde_json::from_value(raw_json.clone()) {
                Ok(e) => e,
                Err(e) => {
                    warn!(error = %e, "Skipping event with missing fields");
                    continue;
                }
            };

            events.push((event, raw_json));
        }

        events.sort_by(|(a, _), (b, _)| a.created_at.cmp(&b.created_at));

        events
    })
    .await
    .map_err(|e| DownloadError::Other(e.into()))?;

    Ok(events)
}

impl Source for GitHub {
    const SERVICE_NAME: &'static str = "github-gharchive";
    const SCOPE_NAME: &'static str = "github-otel-bridge";
    const SCOPE_VERSION: &'static str = env!("CARGO_PKG_VERSION");

    type Event = GitHubEvent;

    fn event_to_log_record(event: &GitHubEvent, raw_json: &serde_json::Value) -> LogRecord {
        let now_ns = now_unix_nanos();
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
}
