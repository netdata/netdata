// SPDX-License-Identifier: GPL-3.0-or-later
//
// Subquery evaluation. SOW-0026.
//
// A subquery `<expr>[range_ms:step_ms]` re-evaluates `expr` at every grid
// point in `[t - range_ms, t]` at `step_ms` stride and assembles the
// per-step instant-vector results into a range vector keyed by series
// signature. The inner expression must lower to `InstantVector`; the
// lowering layer enforces this.
//
// Shape mirrors `lib.rs::run_range` but at one nesting level deeper:
// the outer query's `at_ms` is unchanged across the inner steps, and
// the inner evaluation receives a derived `EvalContext` whose `at_ms`
// is the per-step time. `outer_start_ms`/`outer_end_ms` stay with the
// outer query's bounds so that `@ start()` / `@ end()` inside the
// subquery resolve against the outer range, matching Prometheus.

use std::collections::BTreeMap;

use crate::plan::{AtMod, Plan, ValueType};

use super::context::EvalContext;
use super::dispatch::eval;
use super::types::{EvalError, EvalResult, Sample, Series};

/// Resolve the subquery's own `@` modifier against the current context.
/// Same semantics as on a vector/matrix selector.
fn resolve_at(ctx: &EvalContext, at: Option<&AtMod>) -> i64 {
    match at {
        None => ctx.at_ms,
        Some(AtMod::AtTs(ms)) => *ms,
        Some(AtMod::Start) => ctx.outer_start_ms,
        Some(AtMod::End) => ctx.outer_end_ms,
    }
}

/// Evaluate a `Plan::Subquery` and return its range vector.
pub fn eval_subquery(
    ctx: &EvalContext,
    expr: &Plan,
    range_ms: i64,
    step_ms: i64,
    offset_ms: i64,
    at: Option<&AtMod>,
) -> Result<EvalResult, EvalError> {
    debug_assert!(range_ms > 0, "lowering guarantees positive range");
    debug_assert!(step_ms > 0, "lowering guarantees positive step");

    let base_t_ms = resolve_at(ctx, at);
    let effective_t_ms = base_t_ms - offset_ms;
    let window_start_ms = effective_t_ms - range_ms;

    // Accumulate per-signature label set + sample sequence across steps,
    // exactly like `run_range` does at the outer level. Same BTreeMap
    // discipline keeps the output ordering deterministic.
    let mut accum: BTreeMap<u64, (Vec<(String, String)>, Vec<Sample>)> = BTreeMap::new();

    let mut t = window_start_ms + step_ms;
    while t <= effective_t_ms {
        let inner_ctx = EvalContext {
            at_ms: t,
            lookback_ms: ctx.lookback_ms,
            host_machine_guid: ctx.host_machine_guid.clone(),
            max_series: ctx.max_series,
            outer_start_ms: ctx.outer_start_ms,
            outer_end_ms: ctx.outer_end_ms,
        };
        match eval(&inner_ctx, expr)? {
            EvalResult::InstantVector(series) => {
                for s in series {
                    let value = s.samples.first().map(|s| s.value).unwrap_or(f64::NAN);
                    accum
                        .entry(s.signature)
                        .or_insert_with(|| (s.labels, Vec::new()))
                        .1
                        .push(Sample {
                            timestamp_ms: t,
                            value,
                        });
                }
            }
            other => {
                // The lowering layer rejects non-instant inner
                // expressions; this is defense in depth.
                return Err(EvalError::Type {
                    context: "subquery inner expression",
                    expected: ValueType::InstantVector,
                    got: other.value_type(),
                });
            }
        }
        let next = match t.checked_add(step_ms) {
            Some(n) => n,
            None => break,
        };
        t = next;
    }

    let mut out: Vec<Series> = accum
        .into_iter()
        .map(|(sig, (labels, samples))| Series {
            labels,
            signature: sig,
            samples,
        })
        .collect();
    out.sort_by_key(|s| s.signature);
    Ok(EvalResult::RangeVector(out))
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn inner_scalar_is_rejected_at_eval() {
        // The lowering layer already rejects non-instant-vector inner
        // expressions, but the evaluator carries a defense-in-depth
        // type check. Drive it directly: a `Number` plan evaluates
        // to a scalar, which is not allowed.
        let ctx = EvalContext {
            at_ms: 10_000,
            outer_start_ms: 10_000,
            outer_end_ms: 10_000,
            ..EvalContext::default()
        };
        let inner = Plan::Number(1.0);
        let err = eval_subquery(&ctx, &inner, 1000, 500, 0, None).err().unwrap();
        match err {
            EvalError::Type { context, expected, got } => {
                assert_eq!(context, "subquery inner expression");
                assert_eq!(expected, ValueType::InstantVector);
                assert_eq!(got, ValueType::Scalar);
            }
            other => panic!("unexpected: {other:?}"),
        }
    }
}
