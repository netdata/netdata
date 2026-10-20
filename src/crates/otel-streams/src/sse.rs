use std::future::Future;
use std::ops::ControlFlow;

use futures::StreamExt;
use tracing::{error, info, warn};

/// Client identifier for SSE requests. Wikimedia asks clients to identify
/// themselves with a descriptive User-Agent.
const USER_AGENT: &str = concat!(
    "otel-streams/",
    env!("CARGO_PKG_VERSION"),
    " (Netdata otel-streams dev tool)"
);

/// Incremental Server-Sent Events frame decoder.
///
/// Feed it raw response chunks; it buffers partial lines across chunk
/// boundaries and returns each completed event's accumulated `data` payload.
/// Only the `data` field is collected — `id`/`event`/`retry` are ignored (no
/// Last-Event-ID resumption), and `:` comment lines are skipped. Multiple
/// `data:` lines in one event are joined with `\n` per the SSE spec.
#[derive(Default)]
struct SseDecoder {
    buf: Vec<u8>,
    data: String,
}

impl SseDecoder {
    /// Consume a chunk, returning the `data` payloads of any events it completed.
    fn push(&mut self, chunk: &[u8]) -> Vec<String> {
        let mut out = Vec::new();
        self.buf.extend_from_slice(chunk);
        while let Some(nl) = self.buf.iter().position(|&b| b == b'\n') {
            let mut line: Vec<u8> = self.buf.drain(..=nl).collect();
            line.pop(); // drop '\n'
            if line.last() == Some(&b'\r') {
                line.pop(); // drop '\r' (CRLF)
            }
            if let Some(done) = self.handle_line(&line) {
                out.push(done);
            }
        }
        out
    }

    /// Process one complete line; returns a payload when the line terminates
    /// an event (a blank line) and data was accumulated.
    fn handle_line(&mut self, line: &[u8]) -> Option<String> {
        if line.is_empty() {
            if self.data.is_empty() {
                return None;
            }
            return Some(std::mem::take(&mut self.data));
        }
        if line.first() == Some(&b':') {
            return None; // comment
        }

        let line = String::from_utf8_lossy(line);
        // Per SSE: split on the first colon; if the value has a leading space,
        // strip exactly one.
        let (field, value) = match line.split_once(':') {
            Some((f, v)) => (f, v.strip_prefix(' ').unwrap_or(v)),
            None => (line.as_ref(), ""),
        };
        if field == "data" {
            if !self.data.is_empty() {
                self.data.push('\n');
            }
            self.data.push_str(value);
        }
        None
    }
}

/// Run a Server-Sent Events read loop.
///
/// GETs `url` as `text/event-stream`, parses each event's `data` payload as
/// JSON, and passes it to `handler`. Mirrors [`crate::ws::run`]: the handler
/// returns `ControlFlow::Continue(())` to keep going or `ControlFlow::Break(())`
/// to stop (typically because the downstream channel closed). Returns `Ok` on
/// stream end or a read error so the caller's reconnect loop takes over.
pub async fn run<H, F>(name: &str, url: &str, mut handler: H) -> anyhow::Result<()>
where
    H: FnMut(serde_json::Value) -> F + Send + 'static,
    F: Future<Output = ControlFlow<(), ()>> + Send,
{
    info!("Connecting to {name}: {url}");

    let client = reqwest::Client::new();
    let response = client
        .get(url)
        .header(reqwest::header::USER_AGENT, USER_AGENT)
        .header(reqwest::header::ACCEPT, "text/event-stream")
        .send()
        .await?;

    if !response.status().is_success() {
        anyhow::bail!("{name} HTTP {}", response.status());
    }
    info!("Connected to {name}");

    let mut stream = response.bytes_stream();
    let mut decoder = SseDecoder::default();

    while let Some(chunk) = stream.next().await {
        let chunk = match chunk {
            Ok(c) => c,
            Err(e) => {
                error!("{name} SSE stream error: {e}");
                break;
            }
        };

        for raw in decoder.push(&chunk) {
            let value: serde_json::Value = match serde_json::from_str(&raw) {
                Ok(v) => v,
                Err(e) => {
                    warn!("Failed to parse SSE data as JSON: {e}");
                    continue;
                }
            };
            match handler(value).await {
                ControlFlow::Continue(()) => {}
                ControlFlow::Break(()) => return Ok(()),
            }
        }
    }

    info!("{name} SSE stream ended");
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;

    fn feed(chunks: &[&str]) -> Vec<String> {
        let mut d = SseDecoder::default();
        let mut out = Vec::new();
        for c in chunks {
            out.extend(d.push(c.as_bytes()));
        }
        out
    }

    #[test]
    fn single_event_one_chunk() {
        assert_eq!(feed(&["data: {\"a\":1}\n\n"]), vec!["{\"a\":1}"]);
    }

    #[test]
    fn event_split_across_chunks() {
        // A line and its terminator arrive in separate chunks.
        assert_eq!(feed(&["data: {\"a", "\":1}\n\n"]), vec!["{\"a\":1}"]);
    }

    #[test]
    fn multiple_data_lines_joined_with_newline() {
        assert_eq!(feed(&["data: a\ndata: b\n\n"]), vec!["a\nb"]);
    }

    #[test]
    fn comment_lines_ignored() {
        assert_eq!(feed(&[": keep-alive\ndata: x\n\n"]), vec!["x"]);
    }

    #[test]
    fn crlf_terminators() {
        assert_eq!(feed(&["data: x\r\n\r\n"]), vec!["x"]);
    }

    #[test]
    fn value_with_colons_preserved() {
        // Only the first colon separates field from value; JSON colons survive.
        assert_eq!(feed(&["data: {\"t\":\"12:30\"}\n\n"]), vec!["{\"t\":\"12:30\"}"]);
    }

    #[test]
    fn blank_line_without_data_emits_nothing() {
        assert!(feed(&["\n", ": c\n\n"]).is_empty());
    }

    #[test]
    fn back_to_back_events() {
        assert_eq!(feed(&["data: a\n\ndata: b\n\n"]), vec!["a", "b"]);
    }
}
