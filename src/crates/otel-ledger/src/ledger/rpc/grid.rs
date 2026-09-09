//! Shared time-grid derivation for the otel Functions' histogram/grid
//! views. The frontends send a loose second-granular window and no
//! bucket geometry; each wire adapter canonicalizes here — picking a
//! "nice" bucket width and snapping the window outward — before handing
//! the engine an exact [`sfst::Grid`]. Consumed by BOTH the logs
//! histogram and the traces overview; a change here changes every otel
//! Function's grid.

/// Aim for at least this many time buckets across the window when
/// picking from [`VALID_BUCKET_WIDTHS_S`]. With the curated widths and
/// a 15-minute window this yields 15-second buckets (60 of them).
const TARGET_BUCKETS: u32 = 60;

/// "Nice" bucket widths in seconds. Ported from the legacy
/// systemd-journal plugin's `calculate_bucket_duration` to keep
/// histograms anchored to wall-clock-friendly intervals (1s, 2s, 5s,
/// 10s, 15s, 30s, 1m, 5m, …). [`bucket_width_for_span_s`] picks the
/// largest entry that produces at least [`TARGET_BUCKETS`] buckets
/// across the span, so chart density is stable as the window scales.
const VALID_BUCKET_WIDTHS_S: &[u32] = &[
    1, 2, 5, 10, 15, 30, // seconds
    60, 120, 180, 300, 600, 900, 1800, // minutes
    3600, 7200, 21600, 28800, 43200, // hours
    86400, 172800, 259200, 432000, 604800, 1209600, 2592000, // days
];

/// Pick a "nice" bucket width (seconds) for a span: the largest entry in
/// [`VALID_BUCKET_WIDTHS_S`] producing at least [`TARGET_BUCKETS`]
/// buckets. Falls back to `1` for spans too short to satisfy it.
pub(crate) fn bucket_width_for_span_s(span_s: u32) -> u32 {
    VALID_BUCKET_WIDTHS_S
        .iter()
        .rev()
        .find(|&&w| span_s / w >= TARGET_BUCKETS)
        .copied()
        .unwrap_or(1)
}

/// Round `[after, before)` outward to multiples of `width_s` — `after`
/// floored, `before` ceiled — so the grid anchors to absolute wall-clock
/// boundaries (e.g. 15s buckets snap to `t % 15 == 0`). This keeps the
/// chart x-axis stable across the UI's per-second polling: requests
/// within the same bucket-width slot align to the same grid.
///
/// Ceiling near the u32 horizon saturates to the largest in-range
/// multiple of `width_s` instead of overflowing (an adversarial
/// `before` close to `u32::MAX` must not panic the request path).
pub(crate) fn align_window(after: u32, before: u32, width_s: u32) -> (u32, u32) {
    let aligned_after = (after / width_s) * width_s;
    let max_aligned = (u32::MAX / width_s) * width_s;
    let aligned_before = u32::try_from(u64::from(before).div_ceil(u64::from(width_s)) * u64::from(width_s))
        .unwrap_or(max_aligned)
        .min(max_aligned);
    (aligned_after, aligned_before)
}

/// Derive the whole grid for a canonicalized second-granular window:
/// nice width, outward alignment, exact [`sfst::Grid`]. Returns the
/// grid plus the aligned `(after, before)` seconds (the alignment can
/// widen the window the caller prunes files by). The caller guarantees
/// `after < before` (the canonicalizers do); the grid then always holds
/// at least one bucket, horizon saturation included.
pub(crate) fn grid_for_window_s(after: u32, before: u32) -> (sfst::Grid, u32, u32) {
    const NS_PER_S: i64 = 1_000_000_000;
    let width_s = bucket_width_for_span_s(before.saturating_sub(after));
    let (after, before) = align_window(after, before, width_s);
    // `width_s` divides `(before - after)` exactly after alignment.
    let grid = sfst::Grid::new(
        i64::from(after) * NS_PER_S,
        i64::from(width_s) * NS_PER_S,
        ((before - after) / width_s) as usize,
    );
    (grid, after, before)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn nice_widths_hit_the_target_density() {
        // 15-minute window → 15s (largest width with span/w >= 60).
        assert_eq!(bucket_width_for_span_s(900), 15);
        assert_eq!(bucket_width_for_span_s(60), 1);
        // Very small spans (< TARGET_BUCKETS seconds) → 1s fallback.
        assert_eq!(bucket_width_for_span_s(30), 1);
        assert_eq!(bucket_width_for_span_s(3600), 60);
        assert_eq!(bucket_width_for_span_s(86400), 900);
    }

    #[test]
    fn jittered_requests_in_the_same_slot_share_one_grid() {
        // The UI polls every second with a sliding window; requests
        // within the same bucket-width slot must produce the SAME grid
        // so the chart x-axis never jitters.
        let (a, ..) = grid_for_window_s(7, 907);
        let (b, ..) = grid_for_window_s(9, 909);
        assert_eq!(a.bucket_start_ns, b.bucket_start_ns);
        assert_eq!(a.bucket_width_ns, b.bucket_width_ns);
        assert_eq!(a.num_buckets, b.num_buckets);
    }

    #[test]
    fn adversarial_before_near_the_horizon_saturates_instead_of_overflowing() {
        // before near u32::MAX with a large nice width: the ceil-multiply
        // must saturate to the largest in-range multiple, never panic
        // (debug) or wrap (release).
        let (after, before) = align_window(1, u32::MAX, 2_592_000);
        assert_eq!(after, 0);
        assert_eq!(before, (u32::MAX / 2_592_000) * 2_592_000);
        // And the whole derivation stays sane end to end.
        let (grid, a, b) = grid_for_window_s(1, u32::MAX);
        assert!(a < b);
        assert!(grid.num_buckets > 0);
    }

    #[test]
    fn alignment_snaps_outward_and_the_grid_covers_it_exactly() {
        assert_eq!(align_window(0, 900, 15), (0, 900));
        assert_eq!(align_window(7, 893, 15), (0, 900));

        // 886s span: 15s gives only 59 buckets, so the next width down
        // (10s) wins; alignment then snaps to (0, 900) → 90 buckets.
        let (grid, after, before) = grid_for_window_s(7, 893);
        assert_eq!((after, before), (0, 900));
        assert_eq!(grid.bucket_start_ns, 0);
        assert_eq!(grid.bucket_width_ns, 10_000_000_000);
        assert_eq!(grid.num_buckets, 90);
    }
}
