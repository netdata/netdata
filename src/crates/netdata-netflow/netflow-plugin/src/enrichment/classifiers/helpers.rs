use super::runtime::ResolvedValue;
use super::*;

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
) -> Result<Option<String>> {
    let regex = Regex::new(pattern).with_context(|| format!("invalid regex '{pattern}'"))?;
    if let Some(captures) = regex.captures(input) {
        let mut output = String::new();
        captures.expand(template, &mut output);
        return Ok(Some(output));
    }
    Ok(None)
}

pub(crate) fn format_with_percent_placeholders(pattern: &str, args: &[ResolvedValue]) -> String {
    let mut out = String::new();
    let chars: Vec<char> = pattern.chars().collect();
    let mut idx = 0_usize;
    let mut arg_idx = 0_usize;

    while idx < chars.len() {
        let ch = chars[idx];
        if ch == '%' && idx + 1 < chars.len() {
            let spec = chars[idx + 1];
            match spec {
                's' | 'v' => {
                    if let Some(arg) = args.get(arg_idx) {
                        out.push_str(&arg.to_string_value());
                        arg_idx += 1;
                    }
                    idx += 2;
                    continue;
                }
                'd' => {
                    if let Some(arg) = args.get(arg_idx) {
                        out.push_str(&arg.to_i64().unwrap_or(0).to_string());
                        arg_idx += 1;
                    }
                    idx += 2;
                    continue;
                }
                '%' => {
                    out.push('%');
                    idx += 2;
                    continue;
                }
                _ => {}
            }
        }
        out.push(ch);
        idx += 1;
    }

    out
}
