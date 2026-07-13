//! TraceQL duration literals → integer nanoseconds.
//!
//! The form's duration input admits decimals and the full unit set —
//! validation regex `\d+(?:\.\d)?\d*(?:us|µs|ns|ms|s|m|h)`
//! (`SearchTraceQLEditor/DurationInput.tsx:16`) — so the parser accepts
//! `300µs`, `1.2ms`, `2m`, `1h`, not just integral `100ms`. The engine
//! takes durations as [`sfsq::traces::PredicateValue::Integer`]
//! nanoseconds.

/// Parse a duration literal to nanoseconds. Errors carry the human
/// reason (the caller wraps position/literal context).
pub fn parse_duration_ns(lit: &str) -> Result<i64, String> {
    let unit_at = lit
        .char_indices()
        .find(|(_, c)| !c.is_ascii_digit() && *c != '.')
        .map(|(i, _)| i)
        .ok_or_else(|| "missing unit (ns, us, µs, ms, s, m, h)".to_string())?;
    let (num, unit) = lit.split_at(unit_at);
    let unit_ns: f64 = match unit {
        "ns" => 1.0,
        "us" | "µs" => 1e3,
        "ms" => 1e6,
        "s" => 1e9,
        "m" => 60e9,
        "h" => 3_600e9,
        _ => return Err(format!("unknown unit {unit:?} (expected ns, us, µs, ms, s, m, h)")),
    };
    if num.is_empty() {
        return Err("missing numeric part".to_string());
    }
    let num: f64 = num
        .parse()
        .map_err(|_| format!("invalid numeric part {num:?}"))?;
    let ns = num * unit_ns;
    // i64::MAX is not exactly representable in f64; the strict < bound
    // against 2^63 keeps the cast in range.
    if !ns.is_finite() || ns < 0.0 || ns >= i64::MAX as f64 {
        return Err("duration out of range".to_string());
    }
    Ok(ns.round() as i64)
}

#[cfg(test)]
mod tests {
    use super::parse_duration_ns;

    #[test]
    fn units_and_decimals() {
        assert_eq!(parse_duration_ns("100ms"), Ok(100_000_000));
        assert_eq!(parse_duration_ns("1.2ms"), Ok(1_200_000));
        assert_eq!(parse_duration_ns("300µs"), Ok(300_000));
        assert_eq!(parse_duration_ns("300us"), Ok(300_000));
        assert_eq!(parse_duration_ns("7ns"), Ok(7));
        assert_eq!(parse_duration_ns("1.5s"), Ok(1_500_000_000));
        assert_eq!(parse_duration_ns("2m"), Ok(120_000_000_000));
        assert_eq!(parse_duration_ns("1h"), Ok(3_600_000_000_000));
    }

    #[test]
    fn rejects() {
        assert!(parse_duration_ns("100").is_err()); // no unit
        assert!(parse_duration_ns("ms").is_err()); // no number
        assert!(parse_duration_ns("5d").is_err()); // unknown unit
        assert!(parse_duration_ns("1.2.3ms").is_err());
        assert!(parse_duration_ns("999999999999h").is_err()); // overflow
    }
}
