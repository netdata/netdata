//! Build a neutral [`sfsq::logs::LogsQuery`] from CLI inputs: filter grammar,
//! time window, direction, and limit.

use std::ops::Range;
use std::time::{SystemTime, UNIX_EPOCH};

use anyhow::{Result, anyhow, bail};
use sfsq::logs::{LogsQuery, LogsQueryBuilder};
use sfst::Filter;

const NS_PER_S: i64 = 1_000_000_000;

/// Parse a comma-separated filter expression into a [`Filter`].
///
/// Each term is `field=value` (exact) or `field~regex` (full-value-anchored
/// regex); the operator is whichever of `~` / `=` appears first, so a value may
/// contain the other character. Repeating a field ORs its terms; different
/// fields AND. A bare term (no `=`/`~`) is rejected — there is no field-less
/// search (use `--query` for that). Mirrors the `sfsq` query example's grammar.
pub fn parse_filter(expr: &str) -> Result<Filter> {
    let mut filter = Filter::new();
    for term in expr.split(',').map(str::trim).filter(|t| !t.is_empty()) {
        let (op_at, is_pattern) = match (term.find('~'), term.find('=')) {
            (Some(t), Some(e)) => (t.min(e), t < e),
            (Some(t), None) => (t, true),
            (None, Some(e)) => (e, false),
            (None, None) => {
                bail!(
                    "filter term '{term}' has no '=' or '~'; use field=value or field~regex \
                     (free-text search is --query)"
                );
            }
        };
        let field = term[..op_at].trim();
        let value = term[op_at + 1..].trim();
        if field.is_empty() {
            bail!("filter term '{term}' has an empty field");
        }
        filter = if is_pattern {
            filter.select_pattern(field, value)
        } else {
            filter.select(field, value)
        };
    }
    Ok(filter)
}

/// Parse the duration part of a relative time spec (the text after `-`/`+`) into
/// whole seconds. Errors on a duration exceeding `u32` rather than truncating —
/// matching the absolute-datetime overflow check, not silently wrapping.
fn relative_secs(rest: &str, sign: char) -> Result<u32> {
    let d = humantime::parse_duration(rest.trim())
        .map_err(|e| anyhow!("invalid relative time '{sign}{rest}': {e}"))?;
    u32::try_from(d.as_secs())
        .map_err(|_| anyhow!("relative time '{sign}{rest}' overflows u32 epoch seconds"))
}

/// Current wall-clock time in epoch seconds.
///
/// Saturates to `u32::MAX` past 2106 (rather than wrapping) and to `0` for a
/// clock before the epoch — both are degenerate clock states, not normal input.
pub fn now_secs() -> u32 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map(|d| u32::try_from(d.as_secs()).unwrap_or(u32::MAX))
        .unwrap_or(0)
}

/// Parse a time spec into epoch seconds, relative to `now_s`.
///
/// Accepts: `now`; a relative offset `-<dur>` / `+<dur>` (e.g. `-1h`, `+30m`)
/// using humantime duration syntax; a bare epoch-seconds integer; or an
/// absolute UTC datetime `YYYY-MM-DD HH:MM:SS` (a `T` separator, and a trailing
/// `Z` or `+00:00`, are accepted — all interpreted as UTC). A non-UTC timezone
/// offset (e.g. `+03:00`) is rejected; use epoch seconds for a non-UTC instant.
/// Sub-second precision is truncated to whole seconds (the window is
/// second-granular). Relative-duration units are lowercase (`-1h`, `+30m`).
pub fn parse_time(spec: &str, now_s: u32) -> Result<u32> {
    let spec = spec.trim();
    // Exact lowercase `now`, consistent with the lowercase-only duration units
    // (`-1h`/`+30m`) — no case-insensitive special case for one keyword.
    if spec == "now" {
        return Ok(now_s);
    }
    if let Some(rest) = spec.strip_prefix('-') {
        return Ok(now_s.saturating_sub(relative_secs(rest, '-')?));
    }
    if let Some(rest) = spec.strip_prefix('+') {
        return Ok(now_s.saturating_add(relative_secs(rest, '+')?));
    }
    if let Ok(epoch) = spec.parse::<u32>() {
        return Ok(epoch);
    }
    let t = humantime::parse_rfc3339_weak(spec).map_err(|e| {
        anyhow!(
            "invalid time '{spec}': {e} (use epoch seconds; -1h/+30m; now; or a \
             UTC datetime like 2026-06-16 10:00:00 — a trailing Z/+00:00 is fine, \
             a non-UTC offset like +03:00 is not)"
        )
    })?;
    let secs = t
        .duration_since(UNIX_EPOCH)
        .map_err(|e| anyhow!("time '{spec}' is before the epoch: {e}"))?
        .as_secs();
    // Window seconds are u32 (matching the engine's Query/Summary). Error on
    // overflow rather than silently truncating a post-2106 datetime.
    u32::try_from(secs)
        .map_err(|_| anyhow!("time '{spec}' overflows u32 epoch seconds (post-2106)"))
}

/// Assemble the engine query. The grid is a single bucket spanning the window
/// (we emit rows, not the histogram).
///
/// Direction is always the engine default (`Backward`): the returned page is
/// the newest `limit` rows, presented newest-first. The engine's `Forward` is a
/// pagination concept (which rows relative to an anchor), not an output-order
/// flip — `finalize_page` always presents newest-first — so oldest-first
/// display is handled by the caller reversing the page, not here.
pub fn build_query(
    window: Range<u32>,
    filter: Filter,
    query: Option<String>,
    limit: usize,
) -> LogsQuery {
    let after_ns = i64::from(window.start) * NS_PER_S;
    let span_ns = (i64::from(window.end) - i64::from(window.start)) * NS_PER_S;
    // The CLI guarantees end > start (≥ 1s), so span_ns is always ≥ NS_PER_S;
    // .max(1) is defensive for any direct caller of this pub fn.
    let grid = sfst::Grid::new(after_ns, span_ns.max(1), 1);

    let mut builder = LogsQueryBuilder::new(grid).filter(filter).limit(limit);
    if let Some(q) = query {
        builder = builder.query(q);
    }
    builder.build()
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn relative_and_now() {
        let now = 1_000_000u32;
        assert_eq!(parse_time("now", now).unwrap(), now);
        assert_eq!(parse_time("-1h", now).unwrap(), now - 3600);
        assert_eq!(parse_time("+30m", now).unwrap(), now + 1800);
        assert_eq!(parse_time("12345", now).unwrap(), 12345);
    }

    #[test]
    fn accepts_utc_datetime_forms() {
        // Bare, T-separated, trailing Z, and +00:00 all parse to the same UTC
        // instant (they are all UTC); the value is the headline absolute form.
        let bare = parse_time("2026-06-16 10:00:00", 0).unwrap();
        assert!(bare > 0);
        assert_eq!(parse_time("2026-06-16T10:00:00", 0).unwrap(), bare);
        assert_eq!(parse_time("2026-06-16T10:00:00Z", 0).unwrap(), bare);
        assert_eq!(parse_time("2026-06-16T10:00:00+00:00", 0).unwrap(), bare);
    }

    #[test]
    fn rejects_post_2106_overflow() {
        // u32 epoch seconds overflow after 2106-02-07; an absolute datetime past
        // that errors rather than silently truncating.
        let err = parse_time("2200-01-01 00:00:00", 1_000_000).unwrap_err();
        assert!(err.to_string().contains("overflows u32"), "got: {err}");
    }

    #[test]
    fn rejects_relative_overflow() {
        // A relative offset beyond u32 seconds errors, not truncates (mirroring
        // the absolute-datetime overflow check).
        let err = parse_time("+200years", 1_000_000).unwrap_err();
        assert!(err.to_string().contains("overflows u32"), "got: {err}");
    }

    #[test]
    fn timezone_offset_is_rejected() {
        // A non-UTC numeric offset is not accepted (UTC `Z`/`+00:00` are — see
        // accepts_utc_datetime_forms).
        assert!(parse_time("2026-06-16T10:00:00+03:00", 1_000_000).is_err());
    }

    #[test]
    fn filter_operator_precedence_and_empty_field() {
        // Empty field with either operator is rejected.
        assert!(parse_filter("~oops").is_err());
        // Whichever of `=`/`~` comes first is the operator; the other may appear
        // in the value.
        assert!(parse_filter("f=v~w").is_ok()); // exact, value "v~w"
        assert!(parse_filter("f~v=w").is_ok()); // pattern, value "v=w"
        // A malformed regex parses into the Filter but fails validation.
        assert!(parse_filter("f~(").unwrap().validate().is_err());
    }

    #[test]
    fn bare_filter_term_rejected() {
        assert!(parse_filter("severity_text").is_err());
        assert!(parse_filter("=oops").is_err());
        assert!(parse_filter("severity_text=ERROR, host~web.*").is_ok());
    }
}
