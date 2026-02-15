use std::io::{BufRead, BufReader};
use std::time::{Duration, Instant};

use flate2::read::GzDecoder;
use serde::Deserialize;
use tokio::sync::mpsc;
use tracing::{error, info, warn};

use crate::mapping::{offset_within_hour_secs, parse_hour_boundary_secs};

#[derive(Debug, Deserialize)]
pub struct GitHubEvent {
    #[allow(dead_code)]
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

/// Cursor tracking which hour to download next.
struct HourCursor {
    year: i32,
    month: u32,
    day: u32,
    hour: u32,
}

impl HourCursor {
    /// Create a cursor from a "YYYY-MM-DD-H" string.
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

    /// Create a cursor for the previous UTC hour.
    fn previous_hour() -> Self {
        use std::time::{SystemTime, UNIX_EPOCH};
        let secs = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .unwrap_or_default()
            .as_secs();
        // Go back one hour
        let secs = secs.saturating_sub(3600);
        let days = secs / 86400;
        let day_secs = secs % 86400;
        let hour = (day_secs / 3600) as u32;

        // Convert days since epoch to civil date
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

    /// Advance to the next hour.
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

/// Convert days since Unix epoch to (year, month, day).
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

/// Download, decompress, parse, and replay events from GH Archive.
pub async fn replay_loop(
    start: Option<String>,
    no_pace: bool,
    tx: mpsc::Sender<(GitHubEvent, serde_json::Value)>,
) {
    let mut cursor = match start {
        Some(s) => match HourCursor::parse(&s) {
            Ok(c) => c,
            Err(e) => {
                error!("Failed to parse --start: {e}");
                return;
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

        let replay_start = Instant::now();

        // Find the hour boundary from the first event for pacing offsets.
        let hour_boundary_secs = events
            .first()
            .and_then(|(e, _)| parse_hour_boundary_secs(&e.created_at));

        for (event, raw_json) in events {
            if !no_pace {
                if let Some(_boundary) = hour_boundary_secs {
                    let offset = offset_within_hour_secs(&event.created_at);
                    let target = replay_start + Duration::from_secs_f64(offset);
                    let now = Instant::now();
                    if target > now {
                        tokio::time::sleep(target - now).await;
                    }
                }
            }

            if tx.send((event, raw_json)).await.is_err() {
                warn!("Mapper channel closed, stopping replay loop");
                return;
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

    // Decompress and parse in a blocking task to avoid starving the async runtime.
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

        // Sort by created_at (should already be roughly ordered).
        events.sort_by(|(a, _), (b, _)| a.created_at.cmp(&b.created_at));

        events
    })
    .await
    .map_err(|e| DownloadError::Other(e.into()))?;

    Ok(events)
}
