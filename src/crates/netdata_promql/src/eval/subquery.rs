// SPDX-License-Identifier: GPL-3.0-or-later
//
// Subquery evaluation. SOW-0026, rewritten for SOW-0031.
//
// A subquery `<expr>[range_ms:step_ms]` evaluates `expr` once over a
// nested grid spanning `[outer.start - range_ms, outer.end]` at
// `step_ms` stride. The resulting per-step values become the
// `RangeVector` samples that downstream rollups consume via two-pointer
// windowing.
//
// Pre-SOW-0031 this looped per outer grid point, calling `eval` for
// each inner step. The new shape does one inner evaluation over the
// union of all per-outer-point inner windows, which lets the inner
// selectors hit storage once and lets downstream rollups window the
// resulting samples without re-evaluating the inner expression.

use crate::plan::{AtMod, Plan, ValueType};

use super::context::EvalContext;
use super::dispatch::eval;
use super::types::{EvalError, EvalResult};

/// Resolve the subquery's own `@` modifier against the current context.
/// `AtMod::AtTs` pins to a fixed timestamp; `Start`/`End` pin to the
/// outer query's bounds. `None` follows the active grid.
fn resolve_pinned(ctx: &EvalContext, at: Option<&AtMod>) -> Option<i64> {
    at.map(|a| match a {
        AtMod::AtTs(ms) => *ms,
        AtMod::Start => ctx.outer_start_ms,
        AtMod::End => ctx.outer_end_ms,
    })
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

    // The subquery's output samples must cover every inner step that
    // any outer-grid-point's window will read. The union spans
    // `[outer.start - range_ms - offset, outer.end - offset]` (for the
    // grid-following case) or a single pinned window (for `@`).
    let (inner_start_ms, inner_end_ms) = match resolve_pinned(ctx, at) {
        Some(t) => {
            let effective = t - offset_ms;
            (effective - range_ms + step_ms, effective)
        }
        None => {
            let outer_start = ctx.grid.start_ms - offset_ms;
            let outer_end = ctx.grid.end_ms - offset_ms;
            (outer_start - range_ms + step_ms, outer_end)
        }
    };

    // If the requested window collapses, fall back to a single-point
    // grid at outer_end. The downstream rollup will reject empty
    // samples anyway.
    if inner_end_ms < inner_start_ms {
        return Ok(EvalResult::RangeVector(Vec::new()));
    }

    let inner_grid = super::grid::Grid::range(inner_start_ms, inner_end_ms, step_ms);
    let inner_ctx = EvalContext {
        grid: std::sync::Arc::new(inner_grid),
        lookback_ms: ctx.lookback_ms,
        host_machine_guid: ctx.host_machine_guid.clone(),
        max_series: ctx.max_series,
        // `@ start()` / `@ end()` inside the subquery resolve against
        // the *outer* range, not the subquery's window. SOW-0026.
        outer_start_ms: ctx.outer_start_ms,
        outer_end_ms: ctx.outer_end_ms,
        backend: std::sync::Arc::clone(&ctx.backend),
        // Inherit the outer tier hint; the inner step drives the
        // shim's points-wanted weighting separately at resolve time.
        tier_hint: ctx.tier_hint,
    };

    match eval(&inner_ctx, expr)? {
        EvalResult::InstantVector(series) => {
            // The inner evaluation produced grid-aligned samples (one
            // per inner step per series). Interpret them as the
            // subquery's range-vector samples.
            Ok(EvalResult::RangeVector(series))
        }
        other => Err(EvalError::Type {
            context: "subquery inner expression",
            expected: ValueType::InstantVector,
            got: other.value_type(),
        }),
    }
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
            grid: std::sync::Arc::new(crate::eval::Grid::instant(10_000)),
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
