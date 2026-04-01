pub(in super::super) fn split_top_level(input: &str, sep: &str) -> Vec<String> {
    let mut parts = Vec::new();
    let mut start = 0_usize;
    let mut paren_depth = 0_i32;
    let mut bracket_depth = 0_i32;
    let mut brace_depth = 0_i32;
    let mut in_string = false;
    let mut escaped = false;
    let bytes = input.as_bytes();
    let sep_bytes = sep.as_bytes();
    let mut i = 0_usize;

    while i < bytes.len() {
        let ch = bytes[i] as char;
        if in_string {
            if escaped {
                escaped = false;
            } else if ch == '\\' {
                escaped = true;
            } else if ch == '"' {
                in_string = false;
            }
            i += 1;
            continue;
        }

        match ch {
            '"' => in_string = true,
            '(' => paren_depth += 1,
            ')' => paren_depth -= 1,
            '[' => bracket_depth += 1,
            ']' => bracket_depth -= 1,
            '{' => brace_depth += 1,
            '}' => brace_depth -= 1,
            _ => {}
        }

        if paren_depth == 0
            && bracket_depth == 0
            && brace_depth == 0
            && bytes[i..].starts_with(sep_bytes)
        {
            parts.push(input[start..i].trim().to_string());
            i += sep.len();
            start = i;
            continue;
        }
        i += 1;
    }

    parts.push(input[start..].trim().to_string());
    parts.into_iter().filter(|part| !part.is_empty()).collect()
}

pub(in super::super) fn split_once_top_level<'a>(
    input: &'a str,
    sep: &str,
) -> Option<(&'a str, &'a str)> {
    let mut paren_depth = 0_i32;
    let mut bracket_depth = 0_i32;
    let mut brace_depth = 0_i32;
    let mut in_string = false;
    let mut escaped = false;
    let bytes = input.as_bytes();
    let sep_bytes = sep.as_bytes();
    let mut i = 0_usize;

    while i < bytes.len() {
        let ch = bytes[i] as char;
        if in_string {
            if escaped {
                escaped = false;
            } else if ch == '\\' {
                escaped = true;
            } else if ch == '"' {
                in_string = false;
            }
            i += 1;
            continue;
        }

        match ch {
            '"' => in_string = true,
            '(' => paren_depth += 1,
            ')' => paren_depth -= 1,
            '[' => bracket_depth += 1,
            ']' => bracket_depth -= 1,
            '{' => brace_depth += 1,
            '}' => brace_depth -= 1,
            _ => {}
        }

        if paren_depth == 0
            && bracket_depth == 0
            && brace_depth == 0
            && bytes[i..].starts_with(sep_bytes)
        {
            let left = &input[..i];
            let right = &input[i + sep.len()..];
            return Some((left, right));
        }
        i += 1;
    }

    None
}
