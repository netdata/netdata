// SPDX-License-Identifier: GPL-3.0-or-later
//
// Evaluator result types.
//
// A `Series` is one labeled time series in column-oriented form:
// timestamps live in a shared `Arc<Vec<i64>>` so every grid-aligned
// series in a result reuses the same column, and values live in a
// tight `Vec<f64>` that auto-vectorises in inner loops. The
// `samples()` iterator adapter zips them back into `(ts, value)`
// pairs for read sites that prefer it.
//
// An `EvalResult` is what a single `eval()` call produces — a scalar,
// an instant vector (each series with one value per grid point), or a
// range vector (each series with raw storage samples at their own
// timestamps). Series are emitted in stable signature order so
// downstream consumers can compare deterministically.

use std::sync::Arc;

use crate::plan::ValueType;
use crate::storage::ResolveError;

/// `(ts, value)` pair. The crate's column-oriented [`Series`] materialises
/// these on demand for the small number of read sites that genuinely want
/// the pair shape (output serialization, test conversions).
#[derive(Debug, Clone, Copy, PartialEq)]
pub struct Sample {
    pub timestamp_ms: i64,
    pub value: f64,
}

/// A labeled time series in column-oriented form.
///
/// `labels` is sorted by name; the same labels at different times
/// produce the same `signature`.
///
/// For an InstantVector result, every series shares the same
/// `timestamps` Arc (the grid). For a RangeVector result, each series
/// has its own `timestamps` Arc holding the raw storage timestamps
/// (which differ per series). `values.len() == timestamps.len()` is
/// the invariant.
#[derive(Debug, Clone)]
pub struct Series {
    pub labels: Vec<(String, String)>,
    pub signature: u64,
    pub timestamps: Arc<Vec<i64>>,
    pub values: Vec<f64>,
}

impl Series {
    /// Construct a series from labels and a (shared) timestamps Arc
    /// plus a fresh values vector. Computes the signature from the
    /// labels. The two vectors must already have matching length;
    /// this is debug-asserted.
    pub fn new(labels: Vec<(String, String)>, timestamps: Arc<Vec<i64>>, values: Vec<f64>) -> Self {
        debug_assert_eq!(
            timestamps.len(),
            values.len(),
            "Series invariant: timestamps and values must have the same length"
        );
        let signature = labels_signature(&labels);
        Self {
            labels,
            signature,
            timestamps,
            values,
        }
    }

    /// Construct a single-sample series at one timestamp. Used by
    /// the legacy single-window rollup paths and by `vector(s)` and
    /// related single-point operators.
    pub fn scalar(labels: Vec<(String, String)>, timestamp_ms: i64, value: f64) -> Self {
        Self::new(labels, Arc::new(vec![timestamp_ms]), vec![value])
    }

    /// Construct a series from a pre-existing list of (ts, value)
    /// pairs. Used by RangeVector construction (matrix selector
    /// output) where each series owns private timestamps from
    /// storage.
    pub fn from_samples(labels: Vec<(String, String)>, samples: Vec<Sample>) -> Self {
        let mut timestamps = Vec::with_capacity(samples.len());
        let mut values = Vec::with_capacity(samples.len());
        for s in samples {
            timestamps.push(s.timestamp_ms);
            values.push(s.value);
        }
        Self::new(labels, Arc::new(timestamps), values)
    }

    /// Iterate as (timestamp, value) `Sample` pairs. Convenience for
    /// read sites that prefer the legacy shape (tests, the legacy
    /// rollup paths that don't see a grid). Hot loops should index
    /// `values` directly.
    pub fn samples(&self) -> impl Iterator<Item = Sample> + '_ {
        self.timestamps
            .iter()
            .copied()
            .zip(self.values.iter().copied())
            .map(|(timestamp_ms, value)| Sample {
                timestamp_ms,
                value,
            })
    }

    /// Materialise the per-sample shape as a `Vec<Sample>`. Used by
    /// boundary code (testing, JSON output prep) that genuinely
    /// needs the pair shape. Internal hot paths should not call
    /// this.
    pub fn collect_samples(&self) -> Vec<Sample> {
        self.samples().collect()
    }

    /// Slice access to a single-position view. Returns `None` if the
    /// series is empty.
    pub fn first(&self) -> Option<Sample> {
        let t = *self.timestamps.first()?;
        let v = *self.values.first()?;
        Some(Sample {
            timestamp_ms: t,
            value: v,
        })
    }

    pub fn last(&self) -> Option<Sample> {
        let t = *self.timestamps.last()?;
        let v = *self.values.last()?;
        Some(Sample {
            timestamp_ms: t,
            value: v,
        })
    }

    pub fn len(&self) -> usize {
        self.values.len()
    }

    pub fn is_empty(&self) -> bool {
        self.values.is_empty()
    }

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
