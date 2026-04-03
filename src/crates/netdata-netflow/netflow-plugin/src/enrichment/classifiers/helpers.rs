use super::runtime::ResolvedValue;
use super::*;
use regex::Regex;

pub(crate) fn normalize_classifier_value(input: &str) -> String {
    input
        .to_ascii_lowercase()
        .chars()
        .filter(|ch| ch.is_ascii_alphanumeric() || *ch == '.' || *ch == '+' || *ch == '-')
        .collect()
}

pub(crate) fn apply_regex_template(
    input: &str,
    pattern: &str,
    template: &str,
    compiled: Option<&Regex>,
) -> Result<Option<String>> {
    let owned_regex;
    let regex = if let Some(compiled) = compiled {
        compiled
    } else {
        owned_regex = Regex::new(pattern).with_context(|| format!("invalid regex '{pattern}'"))?;
        &owned_regex
    };

    if let Some(captures) = regex.captures(input) {
        let mut output = String::new();
        captures.expand(template, &mut output);
        return Ok(Some(output));
    }
    Ok(None)
}

pub(crate) fn format_with_percent_placeholders(
    pattern: &str,
    args: &[ResolvedValue],
) -> Result<String> {
    let mut out = String::new();
    let mut chars = pattern.chars().peekable();
    let mut arg_idx = 0_usize;

    while let Some(ch) = chars.next() {
        if ch == '%' {
            match chars.peek().copied() {
                Some(spec @ ('s' | 'v')) => {
                    chars.next();
                    let arg = args.get(arg_idx).with_context(|| {
                        format!("placeholder %{} is missing argument {}", spec, arg_idx)
                    })?;
                    out.push_str(&arg.to_string_value());
                    arg_idx += 1;
                    continue;
                }
                Some('d') => {
                    chars.next();
                    let arg = args.get(arg_idx).with_context(|| {
                        format!("placeholder %{} is missing argument {}", 'd', arg_idx)
                    })?;
                    let value = arg
                        .to_i64()
                        .with_context(|| format!("placeholder %{} expects a numeric value", 'd'))?;
                    out.push_str(&value.to_string());
                    arg_idx += 1;
                    continue;
                }
                Some('%') => {
                    chars.next();
                    out.push('%');
                    continue;
                }
                Some(_) => {
                    if arg_idx < args.len() {
                        arg_idx += 1;
                    }
                }
                None => {}
            }
        }
        out.push(ch);
    }

    if arg_idx < args.len() {
        anyhow::bail!(
            "format pattern has {} unused arguments",
            args.len() - arg_idx
        );
    }

    Ok(out)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn format_with_percent_placeholders_formats_known_specs() {
        let rendered = format_with_percent_placeholders(
            "π-%s-%d-%%",
            &[
                ResolvedValue::String("edge".to_string()),
                ResolvedValue::Number(42),
            ],
        )
        .unwrap();

        assert_eq!(rendered, "π-edge-42-%");
    }

    #[test]
    fn format_with_percent_placeholders_preserves_unknown_specs() {
        let rendered =
            format_with_percent_placeholders("keep-%x", &[ResolvedValue::String("unused".into())])
                .unwrap();

        assert_eq!(rendered, "keep-%x");
    }

    #[test]
    fn format_with_percent_placeholders_preserves_unknown_specs_without_arguments() {
        let rendered = format_with_percent_placeholders("keep-%x", &[]).unwrap();

        assert_eq!(rendered, "keep-%x");
    }

    #[test]
    fn format_with_percent_placeholders_rejects_non_numeric_percent_d() {
        let err =
            format_with_percent_placeholders("asn-%d", &[ResolvedValue::String("edge".into())])
                .unwrap_err();

        assert!(err.to_string().contains("expects a numeric value"));
    }

    #[test]
    fn format_with_percent_placeholders_rejects_missing_arguments() {
        let err =
            format_with_percent_placeholders("asn-%s-%d", &[ResolvedValue::String("edge".into())])
                .unwrap_err();

        assert!(err.to_string().contains("missing argument"));
    }

    #[test]
    fn format_with_percent_placeholders_rejects_unused_arguments() {
        let err = format_with_percent_placeholders(
            "asn-%s",
            &[
                ResolvedValue::String("edge".into()),
                ResolvedValue::String("extra".into()),
            ],
        )
        .unwrap_err();

        assert!(err.to_string().contains("unused arguments"));
    }
}
