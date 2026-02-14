use chrono::{DateTime, Local, TimeZone};
use std::collections::BTreeMap;
use std::io::{self, Write};

/// Collected fields from a single journal entry.
pub struct Entry {
    pub realtime_usec: u64,
    pub monotonic_usec: u64,
    pub boot_id: [u8; 16],
    pub fields: BTreeMap<String, Vec<u8>>,
}

impl Entry {
    fn field_str(&self, key: &str) -> &str {
        self.fields
            .get(key)
            .and_then(|v| std::str::from_utf8(v).ok())
            .unwrap_or("")
    }
}

fn format_timestamp_short(usec: u64, utc: bool) -> String {
    let secs = (usec / 1_000_000) as i64;
    let micros = (usec % 1_000_000) as u32;

    if utc {
        if let Some(dt) = DateTime::from_timestamp(secs, micros * 1000) {
            return dt.format("%a %Y-%m-%d %H:%M:%S UTC").to_string();
        }
    } else {
        let dt: DateTime<Local> = Local
            .timestamp_opt(secs, micros * 1000)
            .single()
            .unwrap_or_default();
        return dt.format("%a %Y-%m-%d %H:%M:%S").to_string();
    }

    format!("{usec}")
}

fn format_timestamp_verbose(usec: u64, utc: bool) -> String {
    let secs = (usec / 1_000_000) as i64;
    let micros = (usec % 1_000_000) as u32;

    if utc {
        if let Some(dt) = DateTime::from_timestamp(secs, micros * 1000) {
            return dt.format("%a %Y-%m-%d %H:%M:%S.%6f UTC").to_string();
        }
    } else {
        let dt: DateTime<Local> = Local
            .timestamp_opt(secs, micros * 1000)
            .single()
            .unwrap_or_default();
        return dt.format("%a %Y-%m-%d %H:%M:%S.%6f").to_string();
    }

    format!("{usec}")
}

/// short format: `Mon YYYY-MM-DD HH:MM:SS hostname process[pid]: message`
pub fn format_short(entry: &Entry, utc: bool, w: &mut impl Write) -> io::Result<()> {
    let ts = format_timestamp_short(entry.realtime_usec, utc);

    let hostname = entry.field_str("_HOSTNAME");

    let ident = {
        let s = entry.field_str("SYSLOG_IDENTIFIER");
        if s.is_empty() {
            entry.field_str("_COMM")
        } else {
            s
        }
    };

    let pid = entry.field_str("_PID");
    let message = entry.field_str("MESSAGE");

    if pid.is_empty() {
        writeln!(w, "{ts} {hostname} {ident}: {message}")
    } else {
        writeln!(w, "{ts} {hostname} {ident}[{pid}]: {message}")
    }
}

/// verbose format: all fields listed, one per line.
pub fn format_verbose(entry: &Entry, utc: bool, w: &mut impl Write) -> io::Result<()> {
    let ts = format_timestamp_verbose(entry.realtime_usec, utc);
    writeln!(
        w,
        "{ts} [s={:016x};i={:016x}]",
        entry.realtime_usec, entry.monotonic_usec
    )?;

    writeln!(
        w,
        "    _BOOT_ID={}",
        uuid::Uuid::from_bytes(entry.boot_id).as_hyphenated()
    )?;

    for (key, value) in &entry.fields {
        if let Ok(s) = std::str::from_utf8(value) {
            writeln!(w, "    {key}={s}")?;
        } else {
            // Show as hex for binary values
            write!(w, "    {key}=[")?;
            for (i, b) in value.iter().enumerate() {
                if i > 0 {
                    write!(w, ", ")?;
                }
                write!(w, "{b}")?;
            }
            writeln!(w, "]")?;
        }
    }

    Ok(())
}

/// json format: single-line JSON object per entry.
pub fn format_json(entry: &Entry, w: &mut impl Write) -> io::Result<()> {
    let mut map = serde_json::Map::new();

    map.insert(
        "__REALTIME_TIMESTAMP".to_string(),
        serde_json::Value::String(entry.realtime_usec.to_string()),
    );
    map.insert(
        "__MONOTONIC_TIMESTAMP".to_string(),
        serde_json::Value::String(entry.monotonic_usec.to_string()),
    );
    map.insert(
        "_BOOT_ID".to_string(),
        serde_json::Value::String(
            uuid::Uuid::from_bytes(entry.boot_id)
                .as_hyphenated()
                .to_string(),
        ),
    );

    for (key, value) in &entry.fields {
        let json_value = if let Ok(s) = std::str::from_utf8(value) {
            serde_json::Value::String(s.to_string())
        } else {
            // Non-UTF8: encode as integer array (matching journalctl behavior)
            serde_json::Value::Array(
                value
                    .iter()
                    .map(|&b| serde_json::Value::Number(serde_json::Number::from(b)))
                    .collect(),
            )
        };
        map.insert(key.clone(), json_value);
    }

    let obj = serde_json::Value::Object(map);
    serde_json::to_writer(&mut *w, &obj).map_err(|e| io::Error::new(io::ErrorKind::Other, e))?;
    writeln!(w)
}

/// cat format: just the MESSAGE value, one per line.
pub fn format_cat(entry: &Entry, w: &mut impl Write) -> io::Result<()> {
    if let Some(msg) = entry.fields.get("MESSAGE") {
        w.write_all(msg)?;
    }
    writeln!(w)
}
