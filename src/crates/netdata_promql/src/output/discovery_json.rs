// SPDX-License-Identifier: GPL-3.0-or-later
//
// JSON serializer for the Phase 2 discovery endpoints.
//
// `/api/v1/labels` and `/api/v1/label/<name>/values` return a flat string
// array under `data`. `/api/v1/series` returns an array of label maps under
// `data`. `/api/v1/metadata` returns a map keyed by metric name; values are
// arrays of {type, help, unit} entries. All four can optionally carry a
// `warnings` field per Prometheus convention -- emitted only when the
// caller hit `max_series` during resolution.

use super::prometheus_json::write_escaped;

/// `{"status":"success","data":[...]}` with optional `warnings`.
pub fn serialize_string_list(items: &[String], warnings: &[String]) -> String {
    let mut out = String::with_capacity(64 + items.iter().map(|s| s.len() + 4).sum::<usize>());
    out.push_str(r#"{"status":"success","data":["#);
    for (i, s) in items.iter().enumerate() {
        if i > 0 {
            out.push(',');
        }
        out.push('"');
        write_escaped(&mut out, s);
        out.push('"');
    }
    out.push(']');
    append_warnings(&mut out, warnings);
    out.push('}');
    out
}

/// `{"status":"success","data":[{"k":"v",...},...]}` for `/api/v1/series`.
pub fn serialize_series_list(items: &[Vec<(String, String)>], warnings: &[String]) -> String {
    let mut out = String::with_capacity(256);
    out.push_str(r#"{"status":"success","data":["#);
    for (i, labels) in items.iter().enumerate() {
        if i > 0 {
            out.push(',');
        }
        out.push('{');
        for (j, (k, v)) in labels.iter().enumerate() {
            if j > 0 {
                out.push(',');
            }
            out.push('"');
            write_escaped(&mut out, k);
            out.push_str(r#"":""#);
            write_escaped(&mut out, v);
            out.push('"');
        }
        out.push('}');
    }
    out.push(']');
    append_warnings(&mut out, warnings);
    out.push('}');
    out
}

/// One TYPE/HELP entry for `/api/v1/metadata`.
pub struct MetadataEntry<'a> {
    pub metric_name: &'a str,
    pub type_: &'a str,
    pub help: &'a str,
    pub unit: &'a str,
}

/// `{"status":"success","data":{"name":[{"type":"...","help":"...","unit":"..."}]}}`.
///
/// Phase 2 emits at most one entry per metric (matching the
/// many-to-one-sanitization collapse done by the shim). Prometheus permits
/// multiple entries per metric (one per scraping target), but Grafana
/// reads the first.
pub fn serialize_metadata_map(entries: &[MetadataEntry<'_>], warnings: &[String]) -> String {
    let mut out = String::with_capacity(256);
    out.push_str(r#"{"status":"success","data":{"#);
    for (i, e) in entries.iter().enumerate() {
        if i > 0 {
            out.push(',');
        }
        out.push('"');
        write_escaped(&mut out, e.metric_name);
        out.push_str(r#"":[{"type":""#);
        write_escaped(&mut out, e.type_);
        out.push_str(r#"","help":""#);
        write_escaped(&mut out, e.help);
        out.push_str(r#"","unit":""#);
        write_escaped(&mut out, e.unit);
        out.push_str(r#""}]"#);
    }
    out.push('}');
    append_warnings(&mut out, warnings);
    out.push('}');
    out
}

fn append_warnings(out: &mut String, warnings: &[String]) {
    if warnings.is_empty() {
        return;
    }
    out.push_str(r#","warnings":["#);
    for (i, w) in warnings.iter().enumerate() {
        if i > 0 {
            out.push(',');
        }
        out.push('"');
        write_escaped(out, w);
        out.push('"');
    }
    out.push(']');
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn empty_label_list_renders() {
        let s = serialize_string_list(&[], &[]);
        assert_eq!(s, r#"{"status":"success","data":[]}"#);
    }

    #[test]
    fn label_list_renders_sorted_items() {
        let items = vec!["__name__".to_string(), "instance".to_string()];
        let s = serialize_string_list(&items, &[]);
        assert_eq!(s, r#"{"status":"success","data":["__name__","instance"]}"#);
    }

    #[test]
    fn series_list_renders_label_maps() {
        let items = vec![vec![
            ("__name__".to_string(), "foo".to_string()),
            ("instance".to_string(), "host-1".to_string()),
        ]];
        let s = serialize_series_list(&items, &[]);
        assert!(s.contains(r#"{"__name__":"foo","instance":"host-1"}"#));
    }

    #[test]
    fn metadata_map_renders_type_help_unit() {
        let entries = vec![MetadataEntry {
            metric_name: "system_cpu",
            type_: "gauge",
            help: "Total CPU utilization",
            unit: "",
        }];
        let s = serialize_metadata_map(&entries, &[]);
        assert!(s.contains(
            r#""system_cpu":[{"type":"gauge","help":"Total CPU utilization","unit":""}]"#
        ));
    }

    #[test]
    fn warnings_field_appears_when_provided() {
        let items = vec!["foo".to_string()];
        let warns = vec!["truncated".to_string()];
        let s = serialize_string_list(&items, &warns);
        assert!(s.contains(r#""warnings":["truncated"]"#));
    }

    #[test]
    fn warnings_field_omitted_when_empty() {
        let items = vec!["foo".to_string()];
        let s = serialize_string_list(&items, &[]);
        assert!(!s.contains("warnings"));
    }

    #[test]
    fn quotes_in_label_values_are_escaped() {
        let items = vec![vec![("k".to_string(), r#"v"x"#.to_string())]];
        let s = serialize_series_list(&items, &[]);
        assert!(s.contains(r#""k":"v\"x""#));
    }
}
