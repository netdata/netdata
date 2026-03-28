pub(in super::super) fn split_top_level_keyword(input: &str, keyword: &str) -> Vec<String> {
    let mut parts = Vec::new();
    let mut start = 0_usize;
    let mut paren_depth = 0_i32;
    let mut bracket_depth = 0_i32;
    let mut brace_depth = 0_i32;
    let mut in_string = false;
    let mut escaped = false;
    let bytes = input.as_bytes();
    let keyword_bytes = keyword.as_bytes();
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
            && bytes[i..].starts_with(keyword_bytes)
            && is_keyword_boundary(input, i, keyword_bytes.len())
        {
            parts.push(input[start..i].trim().to_string());
            i += keyword.len();
            start = i;
            continue;
        }

        i += 1;
    }

    parts.push(input[start..].trim().to_string());
    parts.into_iter().filter(|part| !part.is_empty()).collect()
}

pub(in super::super) fn split_once_top_level_keyword<'a>(
    input: &'a str,
    keyword: &str,
) -> Option<(&'a str, &'a str)> {
    let mut paren_depth = 0_i32;
    let mut bracket_depth = 0_i32;
    let mut brace_depth = 0_i32;
    let mut in_string = false;
    let mut escaped = false;
    let bytes = input.as_bytes();
    let keyword_bytes = keyword.as_bytes();
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
            && bytes[i..].starts_with(keyword_bytes)
            && is_keyword_boundary(input, i, keyword_bytes.len())
        {
            let left = &input[..i];
            let right = &input[i + keyword.len()..];
            return Some((left, right));
        }
        i += 1;
    }
    None
}

pub(in super::super) fn strip_keyword_prefix<'a>(input: &'a str, keyword: &str) -> Option<&'a str> {
    let input = input.trim_start();
    let keyword_bytes = keyword.as_bytes();
    if !input.as_bytes().starts_with(keyword_bytes) {
        return None;
    }
    if !is_keyword_boundary(input, 0, keyword.len()) {
        return None;
    }
    Some(input[keyword.len()..].trim_start())
}

pub(in super::super) fn is_keyword_boundary(input: &str, start: usize, len: usize) -> bool {
    let before_ok = if start == 0 {
        true
    } else {
        input[..start]
            .chars()
            .next_back()
            .map(|ch| !is_identifier_char(ch))
            .unwrap_or(true)
    };
    let after_index = start + len;
    let after_ok = if after_index >= input.len() {
        true
    } else {
        input[after_index..]
            .chars()
            .next()
            .map(|ch| !is_identifier_char(ch))
            .unwrap_or(true)
    };
    before_ok && after_ok
}

fn is_identifier_char(ch: char) -> bool {
    ch.is_ascii_alphanumeric() || ch == '_' || ch == '.'
}
