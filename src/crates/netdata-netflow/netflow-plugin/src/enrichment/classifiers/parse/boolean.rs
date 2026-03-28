use super::action::parse_action;
use super::split::*;
use super::value::{
    parse_function_call, parse_value_expr, strip_outer_parentheses, validate_literal_regex_value,
};
use super::*;

pub(crate) fn parse_boolean_expr(input: &str) -> Result<BoolExpr> {
    let input = input.trim();
    if input.is_empty() {
        anyhow::bail!("empty classifier rule");
    }
    parse_or_expr(input)
}

fn parse_or_expr(input: &str) -> Result<BoolExpr> {
    let parts = split_top_level(input, "||");
    let parts = if parts.len() > 1 {
        parts
    } else {
        split_top_level_keyword(input, "or")
    };
    if parts.len() > 1 {
        let mut iter = parts.into_iter();
        let first = parse_and_expr(
            iter.next()
                .expect("split_top_level for '||' must return non-empty parts")
                .trim(),
        )?;
        return iter.try_fold(first, |left, part| {
            Ok(BoolExpr::Or(
                Box::new(left),
                Box::new(parse_and_expr(part.trim())?),
            ))
        });
    }
    parse_and_expr(input)
}

fn parse_and_expr(input: &str) -> Result<BoolExpr> {
    let parts = split_top_level(input, "&&");
    let parts = if parts.len() > 1 {
        parts
    } else {
        split_top_level_keyword(input, "and")
    };
    if parts.len() > 1 {
        let mut iter = parts.into_iter();
        let first = parse_unary_expr(
            iter.next()
                .expect("split_top_level for '&&' must return non-empty parts")
                .trim(),
        )?;
        return iter.try_fold(first, |left, part| {
            Ok(BoolExpr::And(
                Box::new(left),
                Box::new(parse_unary_expr(part.trim())?),
            ))
        });
    }
    parse_unary_expr(input)
}

fn parse_unary_expr(input: &str) -> Result<BoolExpr> {
    let input = input.trim();
    if input.is_empty() {
        anyhow::bail!("empty classifier expression");
    }

    if let Some(rest) = input.strip_prefix('!') {
        return Ok(BoolExpr::Not(Box::new(parse_unary_expr(rest)?)));
    }
    if let Some(rest) = strip_keyword_prefix(input, "not") {
        return Ok(BoolExpr::Not(Box::new(parse_unary_expr(rest)?)));
    }

    if let Some(inner) = strip_outer_parentheses(input) {
        return parse_boolean_expr(inner);
    }

    Ok(BoolExpr::Term(parse_rule_term(input)?))
}

fn parse_rule_term(term: &str) -> Result<RuleTerm> {
    let term = term.trim();
    if term.is_empty() {
        anyhow::bail!("empty rule term");
    }

    if term == "true" {
        return Ok(RuleTerm::Condition(ConditionExpr::Literal(true)));
    }
    if term == "false" {
        return Ok(RuleTerm::Condition(ConditionExpr::Literal(false)));
    }

    if let Some((name, args)) = parse_function_call(term) {
        let action = parse_action(&name, &args)?;
        return Ok(RuleTerm::Action(action));
    }

    if let Some((left, right)) = split_once_top_level(term, " startsWith ") {
        return Ok(RuleTerm::Condition(ConditionExpr::StartsWith(
            parse_value_expr(left.trim())?,
            parse_value_expr(right.trim())?,
        )));
    }
    if let Some((left, right)) = split_once_top_level(term, " matches ") {
        let left = parse_value_expr(left.trim())?;
        let right = parse_value_expr(right.trim())?;
        validate_literal_regex_value(&right, "matches")?;
        return Ok(RuleTerm::Condition(ConditionExpr::Matches(left, right)));
    }
    if let Some((left, right)) = split_once_top_level(term, " contains ") {
        return Ok(RuleTerm::Condition(ConditionExpr::Contains(
            parse_value_expr(left.trim())?,
            parse_value_expr(right.trim())?,
        )));
    }
    if let Some((left, right)) = split_once_top_level(term, " endsWith ") {
        return Ok(RuleTerm::Condition(ConditionExpr::EndsWith(
            parse_value_expr(left.trim())?,
            parse_value_expr(right.trim())?,
        )));
    }
    if let Some((left, right)) = split_once_top_level(term, " != ") {
        return Ok(RuleTerm::Condition(ConditionExpr::NotEquals(
            parse_value_expr(left.trim())?,
            parse_value_expr(right.trim())?,
        )));
    }
    if let Some((left, right)) = split_once_top_level(term, " >= ") {
        return Ok(RuleTerm::Condition(ConditionExpr::GreaterOrEqual(
            parse_value_expr(left.trim())?,
            parse_value_expr(right.trim())?,
        )));
    }
    if let Some((left, right)) = split_once_top_level(term, " <= ") {
        return Ok(RuleTerm::Condition(ConditionExpr::LessOrEqual(
            parse_value_expr(left.trim())?,
            parse_value_expr(right.trim())?,
        )));
    }
    if let Some((left, right)) = split_once_top_level_keyword(term, "in") {
        return Ok(RuleTerm::Condition(ConditionExpr::In(
            parse_value_expr(left.trim())?,
            parse_value_expr(right.trim())?,
        )));
    }
    if let Some((left, right)) = split_once_top_level(term, " == ") {
        return Ok(RuleTerm::Condition(ConditionExpr::Equals(
            parse_value_expr(left.trim())?,
            parse_value_expr(right.trim())?,
        )));
    }
    if let Some((left, right)) = split_once_top_level(term, " > ") {
        return Ok(RuleTerm::Condition(ConditionExpr::Greater(
            parse_value_expr(left.trim())?,
            parse_value_expr(right.trim())?,
        )));
    }
    if let Some((left, right)) = split_once_top_level(term, " < ") {
        return Ok(RuleTerm::Condition(ConditionExpr::Less(
            parse_value_expr(left.trim())?,
            parse_value_expr(right.trim())?,
        )));
    }

    anyhow::bail!("unsupported rule term: {term}")
}
