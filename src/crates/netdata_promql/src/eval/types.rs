// SPDX-License-Identifier: GPL-3.0-or-later
//
// Evaluator result types.
//
// A `Series` is one labeled time series. An `EvalResult` is what a single
// `eval()` call produces against the Plan IR -- a scalar, an instant
// vector (each series with one sample), or a range vector (each series
// with many samples). Series are emitted in stable signature order so
// downstream consumers can compare results deterministically.

use crate::plan::ValueType;
use crate::storage::ResolveError;

#[derive(Debug, Clone, Copy, PartialEq)]
pub struct Sample {
    pub timestamp_ms: i64,
    pub value: f64,
}

/// A labeled time series.
///
/// `labels` is sorted by name; the same labels at different times produce
/// the same `signature`. For instant vectors, `samples` has length 1
/// (a single timestamp+value at the query time). For range vectors,
/// `samples` carries one entry per stored point in the time window.
#[derive(Debug, Clone)]
pub struct Series {
    pub labels: Vec<(String, String)>,
    pub signature: u64,
    pub samples: Vec<Sample>,
}

impl Series {
    pub fn get(&self, name: &str) -> Option<&str> {
        self.labels
            .binary_search_by(|(n, _)| n.as_str().cmp(name))
            .ok()
            .map(|i| self.labels[i].1.as_str())
    }

    /// Drop the named labels (or, with `keep=true`, drop everything *except*
    /// the named labels). Recomputes the signature from the resulting set.
    pub fn project(mut self, names: &[String], keep: bool) -> Self {
        if keep {
            self.labels.retain(|(n, _)| names.iter().any(|k| k == n));
        } else {
            self.labels.retain(|(n, _)| !names.iter().any(|k| k == n));
        }
        self.signature = labels_signature(&self.labels);
        self
    }
}

/// FNV-1a over the sorted label set. Stable across processes; matches the
/// shim's hash of the same input.
pub fn labels_signature(labels: &[(String, String)]) -> u64 {
    let mut h = 0xcbf29ce484222325u64;
    for (n, v) in labels {
        for &b in n.as_bytes() {
            h ^= b as u64;
            h = h.wrapping_mul(0x100000001b3);
        }
        h ^= 0x5c;
        for &b in v.as_bytes() {
            h ^= b as u64;
            h = h.wrapping_mul(0x100000001b3);
        }
        h ^= 0x1e;
    }
    h
}

#[derive(Debug, Clone)]
pub enum EvalResult {
    Scalar(f64),
    InstantVector(Vec<Series>),
    RangeVector(Vec<Series>),
}

impl EvalResult {
    pub fn value_type(&self) -> ValueType {
        match self {
            EvalResult::Scalar(_) => ValueType::Scalar,
            EvalResult::InstantVector(_) => ValueType::InstantVector,
            EvalResult::RangeVector(_) => ValueType::RangeVector,
        }
    }
}

#[derive(Debug, thiserror::Error)]
pub enum EvalError {
    #[error("storage: {0}")]
    Storage(#[from] ResolveError),

    #[error("type error in {context}: expected {expected:?}, got {got:?}")]
    Type {
        context: &'static str,
        expected: ValueType,
        got: ValueType,
    },

    #[error("evaluation error: {0}")]
    Other(String),

    #[error("not implemented yet: {0}")]
    NotYetImplemented(String),
}
