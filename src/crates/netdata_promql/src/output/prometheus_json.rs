// SPDX-License-Identifier: GPL-3.0-or-later
//
// Prometheus HTTP API response serializer.
//
// Output shape mirrors `https://prometheus.io/docs/prometheus/latest/querying/api/`:
//
//   {"status":"success","data":{"resultType":"<scalar|vector|matrix>","result":<...>}}
//   {"status":"error","errorType":"<bad_data|...>","error":"<msg>"}
//
// Timestamps render as Unix-seconds floats with 3 decimal places. Values
// render as JSON strings ("42" not 42) per Prometheus' element shape.

use std::fmt::Write;

use crate::eval::{EvalResult, Sample, Series};

/// Serialize a successful evaluation result.
pub fn serialize_success(r: &EvalResult) -> String {
    let mut out = String::with_capacity(256);
    out.push_str(r#"{"status":"success","data":"#);
    write_data(&mut out, r);
    out.push('}');
    out
}

/// Serialize an error envelope. `error_type` is a Prometheus error class
/// such as "bad_data", "execution", "internal".
pub fn serialize_error(error_type: &str, message: &str) -> String {
    let mut out = String::with_capacity(64 + message.len());
    out.push_str(r#"{"status":"error","errorType":""#);
    write_escaped(&mut out, error_type);
    out.push_str(r#"","error":""#);
    write_escaped(&mut out, message);
    out.push_str(r#""}"#);
    out
}

fn write_data(out: &mut String, r: &EvalResult) {
    match r {
        EvalResult::Scalar(v) => {
            // Prometheus' instant query may emit scalar; we tag at the
            // top-level data object. The timestamp for a scalar is the
            // query evaluation time; the caller stamps it (we don't have
            // access here), so we emit a placeholder 0 and Phase 1 callers
            // wrap this path explicitly.
            out.push_str(r#"{"resultType":"scalar","result":"#);
            write_pair(out, 0, *v);
            out.push('}');
        }
        EvalResult::InstantVector(series) => {
            out.push_str(r#"{"resultType":"vector","result":["#);
            write_instant_series(out, series);
            out.push_str("]}");
        }
        EvalResult::RangeVector(series) => {
            out.push_str(r#"{"resultType":"matrix","result":["#);
            write_range_series(out, series);
            out.push_str("]}");
        }
    }
}

/// Like `serialize_success`, but for scalar results stamps the timestamp
/// at the given Unix-ms. The Plan-level evaluator does not know its own
/// query time after the fact, so this wrapper exists for the FFI layer.
pub fn serialize_scalar_at(value: f64, at_ms: i64) -> String {
    let mut out = String::with_capacity(96);
    out.push_str(r#"{"status":"success","data":{"resultType":"scalar","result":"#);
    write_pair(&mut out, at_ms, value);
    out.push_str(r#"}}"#);
    out
}

fn write_instant_series(out: &mut String, series: &[Series]) {
    for (i, s) in series.iter().enumerate() {
        if i > 0 {
            out.push(',');
        }
        out.push_str(r#"{"metric":"#);
        write_metric_object(out, &s.labels);
        out.push_str(r#","value":"#);
        let sample = s.samples.first().copied().unwrap_or(Sample {
            timestamp_ms: 0,
            value: f64::NAN,
        });
        write_pair(out, sample.timestamp_ms, sample.value);
        out.push('}');
    }
}

fn write_range_series(out: &mut String, series: &[Series]) {
    for (i, s) in series.iter().enumerate() {
        if i > 0 {
            out.push(',');
        }
        out.push_str(r#"{"metric":"#);
        write_metric_object(out, &s.labels);
        out.push_str(r#","values":["#);
        // Filter NaN samples from matrix output to match Prometheus
        // ("no observation at this point" = absent from values array).
        // Grid-aligned series (SOW-0031) carry NaN at missing-data grid
        // positions; emitting them as numeric NaN would render incorrectly
        // in Grafana / break downstream consumers.
        let mut first = true;
        for sample in s.samples.iter() {
            if sample.value.is_nan() {
                continue;
            }
            if !first {
                out.push(',');
            }
            first = false;
            write_pair(out, sample.timestamp_ms, sample.value);
        }
        out.push_str("]}");
    }
}

fn write_metric_object(out: &mut String, labels: &[(String, String)]) {
    out.push('{');
    let mut first = true;
    for (n, v) in labels {
        if !first {
            out.push(',');
        }
        first = false;
        out.push('"');
        write_escaped(out, n);
        out.push_str(r#"":""#);
        write_escaped(out, v);
        out.push('"');
    }
    out.push('}');
}

fn write_pair(out: &mut String, ts_ms: i64, value: f64) {
    out.push('[');
    write_ts(out, ts_ms);
    out.push_str(r#","#);
    out.push('"');
    write_value(out, value);
    out.push('"');
    out.push(']');
}

fn write_ts(out: &mut String, ts_ms: i64) {
    // Float seconds with three-decimal precision.
    let secs = ts_ms / 1000;
    let frac = (ts_ms.rem_euclid(1000)) as u64;
    let _ = write!(out, "{}.{:03}", secs, frac);
}

fn write_value(out: &mut String, v: f64) {
    if v.is_nan() {
        out.push_str("NaN");
    } else if v.is_infinite() {
        out.push_str(if v > 0.0 { "+Inf" } else { "-Inf" });
    } else {
        // Use Rust's default float formatting; Prometheus accepts any IEEE
        // 754 finite repr.
        let _ = write!(out, "{}", v);
    }
}

pub(crate) fn write_escaped(out: &mut String, s: &str) {
    for c in s.chars() {
        match c {
            '"' => out.push_str("\\\""),
            '\\' => out.push_str("\\\\"),
            '\n' => out.push_str("\\n"),
            '\r' => out.push_str("\\r"),
            '\t' => out.push_str("\\t"),
            c if (c as u32) < 0x20 => {
                let _ = write!(out, "\\u{:04x}", c as u32);
            }
            c => out.push(c),
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::eval::{labels_signature, Sample, Series};

    fn series(labels: Vec<(&str, &str)>, samples: Vec<(i64, f64)>) -> Series {
        let owned: Vec<(String, String)> = labels
            .into_iter()
            .map(|(n, v)| (n.to_string(), v.to_string()))
            .collect();
        let signature = labels_signature(&owned);
        Series {
            labels: owned,
            signature,
            samples: samples
                .into_iter()
                .map(|(ts, v)| Sample {
                    timestamp_ms: ts,
                    value: v,
                })
                .collect(),
        }
    }

    #[test]
    fn scalar_at_renders_prometheus_shape() {
        let s = serialize_scalar_at(42.0, 1_715_594_400_000);
        assert!(s.contains(r#""resultType":"scalar""#));
        assert!(s.contains(r#"[1715594400.000,"42"]"#));
    }

    #[test]
    fn instant_vector_renders_metric_and_value() {
        let s = serialize_success(&EvalResult::InstantVector(vec![series(
            vec![("__name__", "foo"), ("instance", "host-1")],
            vec![(1_000_000, 3.14)],
        )]));
        assert!(s.contains(r#""resultType":"vector""#));
        assert!(s.contains(r#""metric":{"__name__":"foo","instance":"host-1"}"#));
        assert!(s.contains(r#""value":[1000.000,"3.14"]"#));
    }

    #[test]
    fn matrix_renders_values_array() {
        let s = serialize_success(&EvalResult::RangeVector(vec![series(
            vec![("__name__", "foo")],
            vec![(1_000_000, 1.0), (1_015_000, 2.0)],
        )]));
        assert!(s.contains(r#""resultType":"matrix""#));
        assert!(s.contains(r#""values":[[1000.000,"1"],[1015.000,"2"]]"#));
    }

    #[test]
    fn error_envelope_escapes_message() {
        let s = serialize_error("bad_data", r#"oops "x" failed"#);
        assert!(s.contains(r#""errorType":"bad_data""#));
        assert!(s.contains(r#"oops \"x\" failed"#));
    }

    #[test]
    fn nan_value_renders_as_nan() {
        let s = serialize_success(&EvalResult::InstantVector(vec![series(
            vec![("__name__", "foo")],
            vec![(0, f64::NAN)],
        )]));
        assert!(s.contains(r#""NaN""#));
    }
}
