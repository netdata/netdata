//! Output formatting for query results.
//!
//! Format selection goes through [`OutputFormat`] so additional formats
//! (e.g. `key=value` lines, bare message) are a new enum variant plus a match
//! arm — no change to callers. v1 ships NDJSON only.

use std::io::{self, Write};

use clap::ValueEnum;
use sfsq::logs::Cursor;
use sfst::MaterializedRow;

#[derive(Debug, Clone, Copy, PartialEq, Eq, ValueEnum, Default)]
#[value(rename_all = "lowercase")]
pub enum OutputFormat {
    /// One JSON object per record, newline-separated (jq-friendly).
    #[default]
    Ndjson,
}

/// Write each row in `rows` to `out` in the selected format. `fields`, when
/// `Some`, restricts emitted fields to that set (journalctl `--output-fields`).
pub fn write_rows(
    out: &mut impl Write,
    rows: &[(Cursor, MaterializedRow)],
    fields: Option<&[String]>,
    format: OutputFormat,
) -> io::Result<()> {
    for (_, row) in rows {
        match format {
            OutputFormat::Ndjson => {
                writeln!(out, "{}", ndjson_record(row, fields))?;
            }
        }
    }
    Ok(())
}

/// Serialize one record as a compact JSON object:
/// `{"timestamp_ns": <i64>, "fields": [[key, value], ...]}`.
///
/// `fields` is always an array of `[key, value]` pairs. Records permit
/// non-unique field keys (a multi-valued attribute flattens to repeated keys),
/// which a JSON object cannot represent; an array keeps the shape stable and
/// lossless across every record so `jq` consumers never see it shift mid stream
/// (`jq '.fields[] | select(.[0]=="severity_text") | .[1]'`).
fn ndjson_record(row: &MaterializedRow, fields: Option<&[String]>) -> String {
    let pairs = row
        .fields
        .iter()
        .filter(|(k, _)| fields.is_none_or(|keep| keep.iter().any(|f| f == k)))
        .map(|(k, v)| serde_json::json!([k, v]))
        .collect::<Vec<_>>();

    serde_json::json!({
        "timestamp_ns": row.timestamp_ns,
        "fields": serde_json::Value::Array(pairs),
    })
    .to_string()
}

#[cfg(test)]
mod tests {
    use super::*;

    fn row(ts: i64, fields: &[(&str, &str)]) -> MaterializedRow {
        MaterializedRow {
            timestamp_ns: ts,
            fields: fields
                .iter()
                .map(|(k, v)| (k.to_string(), v.to_string()))
                .collect(),
        }
    }

    #[test]
    fn ndjson_fields_is_array_of_pairs() {
        let r = row(42, &[("severity_text", "ERROR"), ("host", "web1")]);
        let v: serde_json::Value = serde_json::from_str(&ndjson_record(&r, None)).unwrap();
        assert_eq!(v["timestamp_ns"], 42);
        assert!(v["fields"].is_array());
        assert_eq!(v["fields"][0][0], "severity_text");
        assert_eq!(v["fields"][0][1], "ERROR");
        assert_eq!(v["fields"][1][0], "host");
        assert_eq!(v["fields"][1][1], "web1");
    }

    #[test]
    fn ndjson_keeps_duplicate_keys_losslessly() {
        let r = row(7, &[("k", "a"), ("k", "b")]);
        let v: serde_json::Value = serde_json::from_str(&ndjson_record(&r, None)).unwrap();
        assert_eq!(v["fields"].as_array().unwrap().len(), 2);
        assert_eq!(v["fields"][0], serde_json::json!(["k", "a"]));
        assert_eq!(v["fields"][1], serde_json::json!(["k", "b"]));
    }

    #[test]
    fn projection_filters_fields() {
        let r = row(1, &[("a", "1"), ("b", "2"), ("c", "3")]);
        let keep = vec!["a".to_string(), "c".to_string()];
        let v: serde_json::Value = serde_json::from_str(&ndjson_record(&r, Some(&keep))).unwrap();
        let keys: Vec<&str> = v["fields"]
            .as_array()
            .unwrap()
            .iter()
            .map(|p| p[0].as_str().unwrap())
            .collect();
        assert_eq!(keys, vec!["a", "c"]);
    }
}
