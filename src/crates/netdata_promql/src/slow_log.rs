// SPDX-License-Identifier: GPL-3.0-or-later
//
// Slow-query log. SOW-0028.
//
// One JSONL line per PromQL query the FFI sees. Configured entirely
// through environment variables; unset `NETDATA_PROMQL_LOG` disables
// the feature with no per-call overhead beyond an atomic check.
//
// The log is observability, not auditing: failures (disk full, bad
// path, etc.) are swallowed silently so the query path remains
// unaffected. Log format is hand-rolled JSON to avoid pulling
// `serde` into the crate.

use std::fs::{File, OpenOptions};
use std::io::{BufWriter, Write};
use std::path::PathBuf;
use std::sync::{Mutex, OnceLock};
use std::time::{SystemTime, UNIX_EPOCH};

/// Configuration extracted once at first use. `path = None` means
/// the logger is disabled.
struct Config {
    path: Option<PathBuf>,
    threshold_ms: u64,
    max_query_len: usize,
}

impl Config {
    fn from_env() -> Self {
        let path = std::env::var("NETDATA_PROMQL_LOG")
            .ok()
            .filter(|s| !s.is_empty())
            .map(PathBuf::from);
        let threshold_ms = std::env::var("NETDATA_PROMQL_LOG_THRESHOLD_MS")
            .ok()
            .and_then(|s| s.parse().ok())
            .unwrap_or(0);
        let max_query_len = std::env::var("NETDATA_PROMQL_LOG_MAX_QUERY_LEN")
            .ok()
            .and_then(|s| s.parse().ok())
            .unwrap_or(4096);
        Self {
            path,
            threshold_ms,
            max_query_len,
        }
    }
}

/// Lazy-initialised global writer. `None` means the env didn't enable
/// the feature; further calls short-circuit.
struct Writer {
    cfg: Config,
    out: Mutex<BufWriter<File>>,
}

fn writer() -> Option<&'static Writer> {
    static WRITER: OnceLock<Option<Writer>> = OnceLock::new();
    WRITER
        .get_or_init(|| {
            let cfg = Config::from_env();
            let path = cfg.path.clone()?;
            let f = OpenOptions::new()
                .create(true)
                .append(true)
                .open(&path)
                .ok()?;
            Some(Writer {
                cfg,
                out: Mutex::new(BufWriter::new(f)),
            })
        })
        .as_ref()
}

/// One log record. Fields are written in the order declared.
pub(crate) struct Record<'a> {
    pub kind: Kind,
    pub query: &'a str,
    pub host: Option<&'a str>,
    /// For range queries; ignored for instant.
    pub start_ms: Option<i64>,
    pub end_ms: Option<i64>,
    pub step_ms: Option<i64>,
    pub elapsed_ms: u64,
    pub http_status: i32,
    /// Only populated when the call succeeded.
    pub series_count: Option<usize>,
    /// Populated on failure.
    pub error_type: Option<&'a str>,
    pub error_message: Option<&'a str>,
}

#[derive(Clone, Copy)]
pub(crate) enum Kind {
    Instant,
    Range,
}

impl Kind {
    fn as_str(self) -> &'static str {
        match self {
            Kind::Instant => "instant",
            Kind::Range => "range",
        }
    }
}

/// Emit one record. No-op when the logger is disabled, when the
/// elapsed time is below the threshold, or when the write itself
/// fails. Never returns an error to the caller.
pub(crate) fn record(rec: Record<'_>) {
    let Some(w) = writer() else {
        return;
    };
    if rec.elapsed_ms < w.cfg.threshold_ms {
        return;
    }
    let line = format_line(&rec, w.cfg.max_query_len);
    let Ok(mut out) = w.out.lock() else {
        return;
    };
    let _ = out.write_all(line.as_bytes());
    let _ = out.write_all(b"\n");
    // Flush so `tail -f` sees lines promptly.
    let _ = out.flush();
}

/// Write one record to a caller-supplied sink, applying the
/// threshold and truncation rules. Returns `Ok(true)` if a line
/// was written, `Ok(false)` if the threshold filtered it out.
/// Test-only entry point so unit tests can exercise the writer
/// path without depending on the lazy global writer's env state.
#[cfg(test)]
pub(crate) fn record_to<W: Write>(
    rec: &Record<'_>,
    sink: &mut W,
    threshold_ms: u64,
    max_query_len: usize,
) -> std::io::Result<bool> {
    if rec.elapsed_ms < threshold_ms {
        return Ok(false);
    }
    let line = format_line(rec, max_query_len);
    sink.write_all(line.as_bytes())?;
    sink.write_all(b"\n")?;
    Ok(true)
}

/// Build the JSON line. Public for test access.
pub(crate) fn format_line(rec: &Record<'_>, max_query_len: usize) -> String {
    let mut s = String::with_capacity(256 + rec.query.len().min(max_query_len));
    s.push('{');
    write_kv_i64(&mut s, "ts_ms", now_unix_ms(), false);
    write_kv_str(&mut s, "kind", rec.kind.as_str(), true);
    write_kv_str_truncated(&mut s, "query", rec.query, max_query_len, true);
    match rec.host {
        Some(h) => write_kv_str(&mut s, "host", h, true),
        None => write_kv_null(&mut s, "host", true),
    }
    if let Some(v) = rec.start_ms {
        write_kv_i64(&mut s, "start_ms", v, true);
    }
    if let Some(v) = rec.end_ms {
        write_kv_i64(&mut s, "end_ms", v, true);
    }
    if let Some(v) = rec.step_ms {
        write_kv_i64(&mut s, "step_ms", v, true);
    }
    write_kv_u64(&mut s, "elapsed_ms", rec.elapsed_ms, true);
    write_kv_i64(&mut s, "http_status", rec.http_status as i64, true);
    if let Some(n) = rec.series_count {
        write_kv_u64(&mut s, "series_count", n as u64, true);
    }
    if let Some(t) = rec.error_type {
        write_kv_str(&mut s, "error_type", t, true);
    }
    if let Some(m) = rec.error_message {
        write_kv_str(&mut s, "error_message", m, true);
    }
    s.push('}');
    s
}

fn now_unix_ms() -> i64 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map(|d| d.as_millis() as i64)
        .unwrap_or(0)
}

fn write_kv_str(s: &mut String, key: &str, value: &str, leading_comma: bool) {
    if leading_comma {
        s.push(',');
    }
    s.push('"');
    s.push_str(key);
    s.push_str("\":");
    json_string(s, value);
}

fn write_kv_str_truncated(s: &mut String, key: &str, value: &str, cap: usize, leading_comma: bool) {
    if leading_comma {
        s.push(',');
    }
    s.push('"');
    s.push_str(key);
    s.push_str("\":");
    // Truncate at a char boundary; avoid splitting multibyte.
    let truncated = if value.len() <= cap {
        value
    } else {
        let mut end = cap;
        while end > 0 && !value.is_char_boundary(end) {
            end -= 1;
        }
        &value[..end]
    };
    json_string(s, truncated);
}

fn write_kv_i64(s: &mut String, key: &str, value: i64, leading_comma: bool) {
    if leading_comma {
        s.push(',');
    }
    s.push('"');
    s.push_str(key);
    s.push_str("\":");
    s.push_str(&value.to_string());
}

fn write_kv_u64(s: &mut String, key: &str, value: u64, leading_comma: bool) {
    if leading_comma {
        s.push(',');
    }
    s.push('"');
    s.push_str(key);
    s.push_str("\":");
    s.push_str(&value.to_string());
}

fn write_kv_null(s: &mut String, key: &str, leading_comma: bool) {
    if leading_comma {
        s.push(',');
    }
    s.push('"');
    s.push_str(key);
    s.push_str("\":null");
}

/// Emit a JSON string with the minimum required escapes. Most query
/// strings are ASCII-printable, so the fast path is common.
fn json_string(out: &mut String, value: &str) {
    out.push('"');
    for c in value.chars() {
        match c {
            '"' => out.push_str("\\\""),
            '\\' => out.push_str("\\\\"),
            '\n' => out.push_str("\\n"),
            '\r' => out.push_str("\\r"),
            '\t' => out.push_str("\\t"),
            '\x08' => out.push_str("\\b"),
            '\x0c' => out.push_str("\\f"),
            c if (c as u32) < 0x20 => {
                out.push_str(&format!("\\u{:04x}", c as u32));
            }
            c => out.push(c),
        }
    }
    out.push('"');
}

#[cfg(test)]
mod tests {
    use super::*;

    /// Build a minimal record for a successful range query.
    fn sample_record() -> Record<'static> {
        Record {
            kind: Kind::Range,
            query: "rate(metric[1m])",
            host: None,
            start_ms: Some(1_000_000),
            end_ms: Some(2_000_000),
            step_ms: Some(15_000),
            elapsed_ms: 7,
            http_status: 200,
            series_count: Some(3),
            error_type: None,
            error_message: None,
        }
    }

    #[test]
    fn format_emits_all_required_keys_on_success() {
        let line = format_line(&sample_record(), 4096);
        // Headers we always want present.
        for needle in [
            r#""kind":"range""#,
            r#""query":"rate(metric[1m])""#,
            r#""host":null"#,
            r#""start_ms":1000000"#,
            r#""end_ms":2000000"#,
            r#""step_ms":15000"#,
            r#""elapsed_ms":7"#,
            r#""http_status":200"#,
            r#""series_count":3"#,
        ] {
            assert!(line.contains(needle), "missing {needle} in {line}");
        }
        // Optional error fields absent on success.
        assert!(!line.contains("error_type"));
        assert!(!line.contains("error_message"));
        // Well-formed JSON object boundaries.
        assert!(line.starts_with('{') && line.ends_with('}'));
    }

    #[test]
    fn format_emits_error_fields_on_failure() {
        let line = format_line(
            &Record {
                kind: Kind::Instant,
                query: "bad{",
                host: None,
                start_ms: None,
                end_ms: None,
                step_ms: None,
                elapsed_ms: 1,
                http_status: 400,
                series_count: None,
                error_type: Some("bad_data"),
                error_message: Some("parse error: unmatched brace"),
            },
            4096,
        );
        assert!(line.contains(r#""kind":"instant""#));
        assert!(line.contains(r#""http_status":400"#));
        assert!(line.contains(r#""error_type":"bad_data""#));
        assert!(line.contains(r#""error_message":"parse error: unmatched brace""#));
        // No start/end/step on an instant.
        assert!(!line.contains("start_ms"));
        assert!(!line.contains("end_ms"));
        assert!(!line.contains("step_ms"));
        // No series_count on an error.
        assert!(!line.contains("series_count"));
    }

    #[test]
    fn format_truncates_long_queries_at_char_boundary() {
        let long = "a".repeat(10_000);
        let mut rec = sample_record();
        rec.query = &long;
        let line = format_line(&rec, 100);
        // The 100-char run, plus opening/closing quote, plus the
        // key. Verify by counting the run between the first quote
        // after `"query":` and the closing quote.
        let key = r#""query":""#;
        let i = line.find(key).expect("query key present");
        let after = &line[i + key.len()..];
        let end = after.find('"').expect("query value closed");
        assert_eq!(end, 100, "expected exactly 100 chars of query value, got {end}");
    }

    #[test]
    fn json_string_escapes_specials() {
        let mut out = String::new();
        json_string(&mut out, "a\"b\\c\nd\te\r");
        assert_eq!(out, r#""a\"b\\c\nd\te\r""#);
        let mut out2 = String::new();
        json_string(&mut out2, "x\x01y");
        assert_eq!(out2, "\"x\\u0001y\"");
    }

    #[test]
    fn newlines_in_query_are_escaped_not_embedded() {
        // The multi-line Grafana-style query case.
        let multiline = "rate(foo[1m])\n  / ignoring(x)\n  rate(bar[1m])";
        let mut rec = sample_record();
        rec.query = multiline;
        let line = format_line(&rec, 4096);
        // No literal newline in output.
        assert!(!line.contains('\n'), "raw \\n in {line:?}");
        // The escaped form is present.
        assert!(line.contains(r#"rate(foo[1m])\n  / ignoring(x)\n  rate(bar[1m])"#));
    }

    #[test]
    fn record_to_writes_one_line_with_trailing_newline() {
        let mut buf: Vec<u8> = Vec::new();
        let written = record_to(&sample_record(), &mut buf, 0, 4096).unwrap();
        assert!(written);
        // Exactly one '\n' at the end of the buffer.
        assert_eq!(buf.iter().filter(|&&b| b == b'\n').count(), 1);
        assert_eq!(buf.last(), Some(&b'\n'));
        // The line before the newline starts with `{` and ends with `}`.
        let s = std::str::from_utf8(&buf[..buf.len() - 1]).unwrap();
        assert!(s.starts_with('{') && s.ends_with('}'));
    }

    #[test]
    fn record_to_respects_threshold_filter() {
        let mut rec = sample_record();
        rec.elapsed_ms = 5;
        let mut buf: Vec<u8> = Vec::new();
        // threshold=100ms: 5ms record skipped
        let written = record_to(&rec, &mut buf, 100, 4096).unwrap();
        assert!(!written);
        assert!(buf.is_empty());
        // threshold=5ms: 5ms record passes (>=)
        rec.elapsed_ms = 5;
        let written2 = record_to(&rec, &mut buf, 5, 4096).unwrap();
        assert!(written2);
        assert!(!buf.is_empty());
    }

    #[test]
    fn record_to_appends_multiple_lines_in_order() {
        let mut buf: Vec<u8> = Vec::new();
        let mut r1 = sample_record();
        r1.elapsed_ms = 1;
        let mut r2 = sample_record();
        r2.elapsed_ms = 2;
        let mut r3 = sample_record();
        r3.elapsed_ms = 3;
        record_to(&r1, &mut buf, 0, 4096).unwrap();
        record_to(&r2, &mut buf, 0, 4096).unwrap();
        record_to(&r3, &mut buf, 0, 4096).unwrap();
        let s = std::str::from_utf8(&buf).unwrap();
        let lines: Vec<&str> = s.lines().collect();
        assert_eq!(lines.len(), 3);
        assert!(lines[0].contains(r#""elapsed_ms":1"#));
        assert!(lines[1].contains(r#""elapsed_ms":2"#));
        assert!(lines[2].contains(r#""elapsed_ms":3"#));
    }

    #[test]
    fn config_defaults_when_env_unset() {
        // Unset all three env vars and verify the defaults via Config.
        // SAFETY: these env vars are namespaced; nothing in the test
        // process consumes them concurrently.
        unsafe {
            std::env::remove_var("NETDATA_PROMQL_LOG");
            std::env::remove_var("NETDATA_PROMQL_LOG_THRESHOLD_MS");
            std::env::remove_var("NETDATA_PROMQL_LOG_MAX_QUERY_LEN");
        }
        let cfg = Config::from_env();
        assert!(cfg.path.is_none());
        assert_eq!(cfg.threshold_ms, 0);
        assert_eq!(cfg.max_query_len, 4096);
    }
}
