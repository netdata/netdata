// SPDX-License-Identifier: GPL-3.0-or-later
//
// Label manipulation functions.
//
// `label_replace(v, dst, replacement, src, regex)` matches the regex
// against the value of `src` on each input series; on match, the
// expansion of `replacement` (with `$N` capture-group expansion)
// becomes the new value of `dst`.
//
// `label_join(v, dst, sep, src1, src2, ...)` concatenates the listed
// source label values into `dst`, separated by `sep`.
//
// Both functions preserve `__name__` (they're label munging, not
// numeric transforms).

use crate::plan::LabelOpKind;
use crate::storage::compile_regex;

use super::types::{EvalError, EvalResult, Series, labels_signature};

pub fn apply_label_op(op: &LabelOpKind, inner: EvalResult) -> Result<EvalResult, EvalError> {
    let series = match inner {
        EvalResult::InstantVector(s) => s,
        other => {
            return Err(EvalError::Type {
                context: "label op",
                expected: crate::plan::ValueType::InstantVector,
                got: other.value_type(),
            });
        }
    };
    let out = match op {
        LabelOpKind::Replace {
            dst,
            replacement,
            src,
            regex,
        } => apply_replace(series, dst, replacement, src, regex)?,
        LabelOpKind::Join { dst, sep, srcs } => apply_join(series, dst, sep, srcs),
    };
    Ok(EvalResult::InstantVector(out))
}

fn apply_replace(
    series: Vec<Series>,
    dst: &str,
    replacement: &str,
    src: &str,
    regex: &str,
) -> Result<Vec<Series>, EvalError> {
    let re = compile_regex(regex)
        .map_err(|e| EvalError::Other(format!("label_replace regex {regex}: {e}")))?;

    let mut out = Vec::with_capacity(series.len());
    for s in series.into_iter() {
        let src_value = s
            .labels
            .iter()
            .find(|(n, _)| n == src)
            .map(|(_, v)| v.as_str())
            .unwrap_or("");
        // Prometheus anchors the regex to the full label value. We
        // check the leftmost match spans the entire string; if not,
        // the series is emitted unchanged.
        let new_value = if let Some(m) = re.find(src_value) {
            if m.start() == 0 && m.end() == src_value.len() {
                // Expand `$N` capture references in `replacement` against
                // the captures from this match.
                Some(expand_replacement(&re, src_value, replacement))
            } else {
                None
            }
        } else {
            None
        };

        let new_labels = match new_value {
            None => s.labels,
            Some(v) if v.is_empty() => {
                // Empty replacement removes the destination label.
                s.labels.into_iter().filter(|(n, _)| n != dst).collect()
            }
            Some(v) => set_label(s.labels, dst, &v),
        };
        out.push(Series::new(new_labels, s.timestamps, s.values));
    }
    out.sort_by_key(|s| s.signature);
    Ok(out)
}

fn apply_join(series: Vec<Series>, dst: &str, sep: &str, srcs: &[String]) -> Vec<Series> {
    let mut out = Vec::with_capacity(series.len());
    for s in series.into_iter() {
        let mut parts: Vec<&str> = Vec::with_capacity(srcs.len());
        for src in srcs {
            let v = s
                .labels
                .iter()
                .find(|(n, _)| n == src)
                .map(|(_, v)| v.as_str())
                .unwrap_or("");
            parts.push(v);
        }
        let joined = parts.join(sep);
        let new_labels = if joined.is_empty() {
            s.labels.into_iter().filter(|(n, _)| n != dst).collect()
        } else {
            set_label(s.labels, dst, &joined)
        };
        out.push(Series::new(new_labels, s.timestamps, s.values));
    }
    out.sort_by_key(|s| s.signature);
    out
}

/// Insert or replace a label, keeping the sorted-by-name invariant.
fn set_label(labels: Vec<(String, String)>, name: &str, value: &str) -> Vec<(String, String)> {
    let mut out: Vec<(String, String)> = labels.into_iter().filter(|(n, _)| n != name).collect();
    let pos = out
        .binary_search_by(|(n, _)| n.as_str().cmp(name))
        .unwrap_or_else(|p| p);
    out.insert(pos, (name.to_string(), value.to_string()));
    out
}

/// Expand `$1`, `$2`, ..., `${name}` capture references in `template`
/// against the regex's match on `input`. Falls back to leaving the
/// reference verbatim when the group doesn't exist.
fn expand_replacement(re: &regex::Regex, input: &str, template: &str) -> String {
    let caps = match re.captures(input) {
        Some(c) => c,
        None => return template.to_string(),
    };
    let mut out = String::with_capacity(template.len());
    re_replace_walk(
        template,
        |key, sink| match key {
            CapRef::Index(n) => {
                if let Some(m) = caps.get(n) {
                    sink.push_str(m.as_str());
                }
            }
            CapRef::Name(n) => {
                if let Some(m) = caps.name(n) {
                    sink.push_str(m.as_str());
                }
            }
        },
        &mut out,
    );
    out
}

#[derive(Debug)]
enum CapRef<'a> {
    Index(usize),
    Name(&'a str),
}

/// Walk `template`, copying plain chars verbatim and invoking `on_ref`
/// for each `$N` / `${name}` reference. `$$` is an escaped dollar.
fn re_replace_walk(
    template: &str,
    mut on_ref: impl FnMut(CapRef<'_>, &mut String),
    out: &mut String,
) {
    let bytes = template.as_bytes();
    let mut i = 0;
    while i < bytes.len() {
        if bytes[i] == b'$' && i + 1 < bytes.len() {
            let next = bytes[i + 1];
            if next == b'$' {
                out.push('$');
                i += 2;
                continue;
            }
            if next == b'{' {
                // ${name} form.
                if let Some(end) = bytes[i + 2..].iter().position(|&b| b == b'}') {
                    let name = &template[i + 2..i + 2 + end];
                    if let Ok(n) = name.parse::<usize>() {
                        on_ref(CapRef::Index(n), out);
                    } else {
                        on_ref(CapRef::Name(name), out);
                    }
                    i += 2 + end + 1;
                    continue;
                }
            }
            if next.is_ascii_digit() {
                // $N or $NN form: take maximal digit run.
                let mut j = i + 1;
                while j < bytes.len() && bytes[j].is_ascii_digit() {
                    j += 1;
                }
                let n: usize = template[i + 1..j].parse().unwrap_or(0);
                on_ref(CapRef::Index(n), out);
                i = j;
                continue;
            }
        }
        out.push(template[i..].chars().next().unwrap());
        i += template[i..].chars().next().unwrap().len_utf8();
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::eval::types::{Sample, Series};

    fn ser(labels: &[(&str, &str)]) -> Series {
        let labels: Vec<(String, String)> = labels
            .iter()
            .map(|(n, v)| (n.to_string(), v.to_string()))
            .collect();
        Series::scalar(labels, 0, 1.0)
    }

    #[test]
    fn replace_writes_capture_into_dst() {
        let s = ser(&[("__name__", "x"), ("src", "abc123")]);
        let out = apply_replace(vec![s], "dst", "$1", "src", "abc(\\d+)").unwrap();
        assert_eq!(out.len(), 1);
        let v = out[0]
            .labels
            .iter()
            .find(|(n, _)| n == "dst")
            .map(|(_, v)| v.as_str())
            .unwrap();
        assert_eq!(v, "123");
    }

    #[test]
    fn replace_leaves_series_when_no_match() {
        let s = ser(&[("__name__", "x"), ("src", "no-digits")]);
        let out = apply_replace(vec![s.clone()], "dst", "$1", "src", "abc(\\d+)").unwrap();
        assert_eq!(out[0].labels.len(), s.labels.len());
        assert!(out[0].labels.iter().all(|(n, _)| n != "dst"));
    }

    #[test]
    fn join_concatenates_with_separator() {
        let s = ser(&[("a", "x"), ("b", "y"), ("c", "z")]);
        let out = apply_join(
            vec![s],
            "joined",
            "-",
            &["a".to_string(), "b".to_string(), "c".to_string()],
        );
        let v = out[0]
            .labels
            .iter()
            .find(|(n, _)| n == "joined")
            .map(|(_, v)| v.as_str())
            .unwrap();
        assert_eq!(v, "x-y-z");
    }

    #[test]
    fn join_missing_label_becomes_empty_part() {
        let s = ser(&[("a", "x"), ("c", "z")]);
        let out = apply_join(
            vec![s],
            "joined",
            "-",
            &["a".to_string(), "b".to_string(), "c".to_string()],
        );
        let v = out[0]
            .labels
            .iter()
            .find(|(n, _)| n == "joined")
            .map(|(_, v)| v.as_str())
            .unwrap();
        assert_eq!(v, "x--z");
    }
}
