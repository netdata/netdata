// SPDX-License-Identifier: GPL-3.0-or-later
//
// Public testing surface. Re-exports the in-memory backend and a
// minimal `eval_instant_against` entry point so integration tests
// (and future external consumers) can drive the evaluator without
// the C shim. SOW-0030.

pub use crate::storage::{Backend, MemBackend, MemSeries};

use std::sync::Arc;

use crate::eval::{eval, EvalContext, EvalError, EvalResult, Grid};
use crate::plan::lower_query;

/// One series in an instant-vector result. Owned strings for ease of
/// consumption from integration tests.
#[derive(Debug)]
pub struct TestSeries {
    pub labels: Vec<(String, String)>,
    pub value: f64,
}

/// What an instant evaluation can produce. The test runner only ever
/// needs scalars or instant vectors; range vectors at the top level
/// are an error in instant queries.
#[derive(Debug)]
pub enum TestResult {
    Scalar(f64),
    InstantVector(Vec<TestSeries>),
}

/// Lower + evaluate a PromQL string at one timestamp against a
/// custom backend. Used by the compliance corpus runner.
pub fn eval_instant_against(
    backend: Arc<dyn Backend>,
    query: &str,
    at_ms: i64,
) -> Result<TestResult, String> {
    let plan = lower_query(query).map_err(|e| format!("lower: {e}"))?;
    let ctx = EvalContext {
        grid: Arc::new(Grid::instant(at_ms)),
        lookback_ms: 5 * 60 * 1000,
        host_machine_guid: None,
        max_series: 10_000,
        outer_start_ms: at_ms,
        outer_end_ms: at_ms,
        backend,
        // Compliance harness uses MemBackend which ignores tier hints.
        tier_hint: -1,
    };
    match eval(&ctx, &plan).map_err(|e: EvalError| format!("eval: {e}"))? {
        EvalResult::Scalar(v) => Ok(TestResult::Scalar(v)),
        EvalResult::InstantVector(series) => {
            let out = series
                .into_iter()
                .map(|s| TestSeries {
                    labels: s.labels,
                    value: s.values.first().copied().unwrap_or(f64::NAN),
                })
                .collect();
            Ok(TestResult::InstantVector(out))
        }
        EvalResult::RangeVector(_) => Err("instant query produced a range vector".into()),
    }
}
