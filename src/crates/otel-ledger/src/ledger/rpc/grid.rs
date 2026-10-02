//! The shared time-grid derivation for the otel ledger's Function
//! views. Wire requests carry only a loose second-granular
//! `[after, before)` window — no bucket geometry — so each signal's
//! adapter canonicalizes the window, then derives everything else
//! through here: a "nice" bucket width, the window snapped outward to
//! wall-clock multiples, and the exact [`sfst::Grid`] the engine
//! queries against.
//!
//! Consumers: the logs adapter's `into_query` (`rpc/logs/adapter.rs`),
//! the traces `overview` and the aggregate section of the traces
//! Functions view (`rpc/traces/handler.rs`).
//! A change here re-grids every otel Function view at once.

/// Aim for at least this many buckets across the window when picking
/// from [`VALID_BUCKET_WIDTHS_S`]; with these widths a 15-minute window
/// lands on 15s buckets (60 of them).
const TARGET_BUCKETS: u32 = 60;

/// "Nice" bucket widths in seconds, mirroring journal-engine's
/// `calculate_bucket_duration` (`journal-engine/src/histogram.rs`) so
/// otel charts land on the same wall-clock-friendly intervals
/// (1s, 2s, 5s, 10s, 15s, 30s,
/// 1m, 5m, …) as the journal histograms. Only the density target
/// differs: journal-engine settles for ≥ 50 buckets, this picks the
/// largest entry that still meets [`TARGET_BUCKETS`] (see
/// [`bucket_width_for_span_s`]), so chart density is stable as the
/// window scales.
const VALID_BUCKET_WIDTHS_S: &[u32] = &[
    1, 2, 5, 10, 15, 30, // seconds
    60, 120, 180, 300, 600, 900, 1800, // minutes
    3600, 7200, 21600, 28800, 43200, // hours
    86400, 172800, 259200, 432000, 604800, 1209600, 2592000, // days
];

/// Pick a "nice" bucket width (seconds) for a span: the largest entry
/// in [`VALID_BUCKET_WIDTHS_S`] yielding at least [`TARGET_BUCKETS`]
/// buckets; spans under [`TARGET_BUCKETS`] seconds fall back to `1`.
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
/// nice width, outward alignment, exact [`sfst::Grid`]. Also returns
/// the aligned `(after, before)` seconds — alignment moves both ends
/// outward, so capture and file-pruning windows come from the pair (or
/// the grid's own range), never the raw request.
///
/// `after < before` is the caller's job; the wire adapters' window
/// canonicalizers guarantee it (`effective_window` in
/// `rpc/logs/adapter.rs`, `validate_trace_bounds` in
/// `rpc/traces/adapter.rs`). Given that, the result always holds at
/// least one bucket, horizon saturation included.
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
