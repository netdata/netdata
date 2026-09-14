pub(in super::super) fn is_wrapped_by_top_level_delimiters(
    input: &str,
    open: char,
    close: char,
) -> bool {
    let input = input.trim();
    if !input.starts_with(open) || !input.ends_with(close) {
        return false;
    }

    let mut paren_depth = 0_i32;
    let mut bracket_depth = 0_i32;
    let mut brace_depth = 0_i32;
    let mut in_string = false;
    let mut escaped = false;

    for (idx, ch) in input.char_indices() {
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

        let current_depth = match open {
            '(' => paren_depth,
            '[' => bracket_depth,
            '{' => brace_depth,
            _ => return false,
        };
        if current_depth == 0 && idx < input.len() - ch.len_utf8() {
            return false;
        }
        if paren_depth < 0 || bracket_depth < 0 || brace_depth < 0 {
            return false;
        }
    }

    paren_depth == 0 && bracket_depth == 0 && brace_depth == 0 && !in_string && !escaped
}

#[cfg(test)]
mod tests {
    use super::is_wrapped_by_top_level_delimiters;

    #[test]
    fn wrapped_by_top_level_delimiters_handles_utf8_without_allocation_changes() {
        assert!(is_wrapped_by_top_level_delimiters("(α, [β, γ])", '(', ')'));
    }

    #[test]
    fn wrapped_by_top_level_delimiters_rejects_negative_depth() {
        assert!(!is_wrapped_by_top_level_delimiters("(α))", '(', ')'));
    }
}
