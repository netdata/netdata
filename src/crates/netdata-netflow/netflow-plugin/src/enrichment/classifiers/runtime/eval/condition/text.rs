use super::context::EvalContext;
use crate::enrichment::ValueExpr;
use anyhow::{Context, Result};
use regex::Regex;

pub(super) fn eval_contains(
    context: &EvalContext<'_>,
    left: &ValueExpr,
    right: &ValueExpr,
) -> Result<bool> {
    let (left, right) = context.resolve_binary(left, right)?;
    Ok(left.to_string_value().contains(&right.to_string_value()))
}

pub(super) fn eval_starts_with(
    context: &EvalContext<'_>,
    left: &ValueExpr,
    right: &ValueExpr,
) -> Result<bool> {
    let (left, right) = context.resolve_binary(left, right)?;
    Ok(left.to_string_value().starts_with(&right.to_string_value()))
}

pub(super) fn eval_ends_with(
    context: &EvalContext<'_>,
    left: &ValueExpr,
    right: &ValueExpr,
) -> Result<bool> {
    let (left, right) = context.resolve_binary(left, right)?;
    Ok(left.to_string_value().ends_with(&right.to_string_value()))
}

pub(super) fn eval_matches(
    context: &EvalContext<'_>,
    left: &ValueExpr,
    right: &ValueExpr,
    compiled: Option<&Regex>,
) -> Result<bool> {
    let (left, right) = context.resolve_binary(left, right)?;
    let pattern = right.to_string_value();
    let regex = match compiled {
        Some(regex) => regex.clone(),
        None => {
            Regex::new(&pattern)
                .with_context(|| format!("invalid regex '{pattern}' for 'matches'"))?
        }
    };
    Ok(regex.is_match(&left.to_string_value()))
}
