use super::split::*;
use super::*;

pub(super) fn one_arg(name: &str, args: &[String]) -> Result<ValueExpr> {
    if args.len() != 1 {
        anyhow::bail!("{name}() expects exactly 1 argument");
    }
    parse_value_expr(args[0].trim())
}

fn three_args(name: &str, args: &[String]) -> Result<(ValueExpr, ValueExpr, ValueExpr)> {
    if args.len() != 3 {
        anyhow::bail!("{name}() expects exactly 3 arguments");
    }
    Ok((
        parse_value_expr(args[0].trim())?,
        parse_value_expr(args[1].trim())?,
        parse_value_expr(args[2].trim())?,
    ))
}

pub(super) fn one_string_arg(name: &str, args: &[String]) -> Result<ValueExpr> {
    let value = one_arg(name, args)?;
    if !value.is_string_expression() {
        anyhow::bail!("{name}() expects a string argument");
    }
    Ok(value)
}

pub(super) fn three_string_args(
    name: &str,
    args: &[String],
) -> Result<(ValueExpr, ValueExpr, ValueExpr)> {
    let (arg1, arg2, arg3) = three_args(name, args)?;
    if !arg1.is_string_expression() || !arg2.is_string_expression() || !arg3.is_string_expression()
    {
        anyhow::bail!("{name}() expects string arguments");
    }
    validate_literal_regex_value(&arg2, name)?;
    Ok((arg1, arg2, arg3))
}

pub(super) fn validate_literal_regex_value(value: &ValueExpr, context: &str) -> Result<()> {
    if let ValueExpr::StringLiteral(pattern) = value {
        Regex::new(pattern)
            .with_context(|| format!("invalid regex '{pattern}' in {context} expression"))?;
    }
    Ok(())
}

pub(super) fn parse_value_expr(input: &str) -> Result<ValueExpr> {
    let input = input.trim();
    if input.is_empty() {
        anyhow::bail!("empty expression");
    }

    let plus_parts = split_top_level(input, "+");
    if plus_parts.len() > 1 {
        let mut parts = Vec::with_capacity(plus_parts.len());
        for part in plus_parts {
            parts.push(parse_value_expr(part.trim())?);
        }
        return Ok(ValueExpr::Concat(parts));
    }

    if let Some(string) = parse_quoted_string(input) {
        return Ok(ValueExpr::StringLiteral(string));
    }
    if let Ok(number) = input.parse::<i64>() {
        return Ok(ValueExpr::NumberLiteral(number));
    }
    if let Some(items) = parse_array_literal(input)? {
        return Ok(ValueExpr::List(items));
    }
    if let Some(field) = FieldExpr::parse(input) {
        return Ok(ValueExpr::Field(field));
    }
    if let Some((name, args)) = parse_function_call(input)
        && name == "Format"
    {
        if args.is_empty() {
            anyhow::bail!("Format() expects at least one argument");
        }
        let pattern = parse_value_expr(args[0].trim())?;
        let mut fmt_args = Vec::new();
        for arg in args.iter().skip(1) {
            fmt_args.push(parse_value_expr(arg.trim())?);
        }
        return Ok(ValueExpr::Format {
            pattern: Box::new(pattern),
            args: fmt_args,
        });
    }

    anyhow::bail!("unsupported value expression: {input}")
}

fn parse_array_literal(input: &str) -> Result<Option<Vec<ValueExpr>>> {
    let input = input.trim();
    if !input.starts_with('[') || !input.ends_with(']') {
        return Ok(None);
    }

    if !is_wrapped_by_top_level_delimiters(input, '[', ']') {
        return Ok(None);
    }

    let inner = input[1..input.len() - 1].trim();
    if inner.is_empty() {
        return Ok(Some(Vec::new()));
    }

    let mut values = Vec::new();
    for item in split_top_level(inner, ",") {
        values.push(parse_value_expr(item.trim())?);
    }
    Ok(Some(values))
}

pub(super) fn parse_function_call(input: &str) -> Option<(String, Vec<String>)> {
    let input = input.trim();
    if !input.ends_with(')') {
        return None;
    }
    let open = input.find('(')?;
    let name = input[..open].trim();
    if name.is_empty() {
        return None;
    }
    let args_raw = &input[open + 1..input.len() - 1];
    let args = if args_raw.trim().is_empty() {
        Vec::new()
    } else {
        split_top_level(args_raw, ",")
    };
    Some((name.to_string(), args))
}

fn parse_quoted_string(input: &str) -> Option<String> {
    if input.len() < 2 || !input.starts_with('"') || !input.ends_with('"') {
        return None;
    }
    serde_json::from_str::<String>(input).ok()
}

pub(super) fn strip_outer_parentheses(input: &str) -> Option<&str> {
    let input = input.trim();
    if !is_wrapped_by_top_level_delimiters(input, '(', ')') {
        return None;
    }
    Some(input[1..input.len() - 1].trim())
}
