fn skip_to_byte_boundary(
    iter: &mut std::iter::Peekable<std::str::CharIndices<'_>>,
    next_index: usize,
) {
    while let Some(&(index, _)) = iter.peek() {
        if index < next_index {
            iter.next();
        } else {
            break;
        }
    }
}

fn has_negative_depth(paren_depth: i32, bracket_depth: i32, brace_depth: i32) -> bool {
    paren_depth < 0 || bracket_depth < 0 || brace_depth < 0
}

pub(in super::super) fn split_top_level(input: &str, sep: &str) -> Vec<String> {
    let mut parts = Vec::new();
    let mut start = 0_usize;
    let mut paren_depth = 0_i32;
    let mut bracket_depth = 0_i32;
    let mut brace_depth = 0_i32;
    let mut in_string = false;
    let mut escaped = false;
    let mut chars = input.char_indices().peekable();

    while let Some((i, ch)) = chars.next() {
        if in_string {
            if escaped {
                escaped = false;
            } else if ch == '\\' {
                escaped = true;
            } else if ch == '"' {
                in_string = false;
            }
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

        if has_negative_depth(paren_depth, bracket_depth, brace_depth) {
            let trimmed = input.trim();
            return if trimmed.is_empty() {
                Vec::new()
            } else {
                vec![trimmed.to_string()]
            };
        }

        if paren_depth == 0 && bracket_depth == 0 && brace_depth == 0 && input[i..].starts_with(sep)
        {
            parts.push(input[start..i].trim().to_string());
            let next_index = i + sep.len();
            skip_to_byte_boundary(&mut chars, next_index);
            start = next_index;
            continue;
        }
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
    let mut chars = input.char_indices().peekable();

    while let Some((i, ch)) = chars.next() {
        if in_string {
            if escaped {
                escaped = false;
            } else if ch == '\\' {
                escaped = true;
            } else if ch == '"' {
                in_string = false;
            }
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

        if has_negative_depth(paren_depth, bracket_depth, brace_depth) {
            return None;
        }

        if paren_depth == 0 && bracket_depth == 0 && brace_depth == 0 && input[i..].starts_with(sep)
        {
            let left = &input[..i];
            let right = &input[i + sep.len()..];
            return Some((left, right));
        }
    }

    None
}

#[cfg(test)]
mod tests {
    use super::{split_once_top_level, split_top_level};

    #[test]
    fn split_top_level_handles_utf8_input() {
        assert_eq!(
            split_top_level("α,β,\"γ,δ\"", ","),
            vec!["α".to_string(), "β".to_string(), "\"γ,δ\"".to_string()]
        );
    }

    #[test]
    fn split_once_top_level_handles_utf8_input() {
        assert_eq!(split_once_top_level("α == β", " == "), Some(("α", "β")));
    }

    #[test]
    fn split_top_level_does_not_split_after_negative_depth() {
        assert_eq!(
            split_top_level("foo), bar", ","),
            vec!["foo), bar".to_string()]
        );
    }

    #[test]
    fn split_once_top_level_returns_none_after_negative_depth() {
        assert_eq!(split_once_top_level("foo) == bar", " == "), None);
    }
}
