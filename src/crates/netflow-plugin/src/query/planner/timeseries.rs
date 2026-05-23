use super::*;

pub(crate) fn align_down(timestamp: u32, step: u32) -> u32 {
    if step == 0 {
        timestamp
    } else {
        (timestamp / step) * step
    }
}

pub(crate) fn align_up(timestamp: u32, step: u32) -> u32 {
    if step == 0 {
        timestamp
    } else {
        timestamp
            .saturating_add(step.saturating_sub(1))
            .saturating_div(step)
            .saturating_mul(step)
    }
}

pub(crate) fn aligned_bucket_count(after: u32, before: u32, step: u32) -> u32 {
    if before <= after || step == 0 {
        return 0;
    }

    let aligned_after = align_down(after, step);
    let aligned_before = align_up(before, step);
    aligned_before
        .saturating_sub(aligned_after)
        .saturating_div(step)
}

pub(crate) fn select_timeseries_source_tier(after: u32, before: u32, force_raw: bool) -> TierKind {
    if force_raw {
        return TierKind::Raw;
    }

    for tier in [TierKind::Hour1, TierKind::Minute5, TierKind::Minute1] {
        let step = tier.bucket_duration().unwrap().as_secs() as u32;
        if aligned_bucket_count(after, before, step) >= TIMESERIES_MIN_BUCKETS {
            return tier;
        }
    }

    TierKind::Minute1
}

pub(crate) fn init_timeseries_layout_for_tier(
    after: u32,
    before: u32,
    source_tier: TierKind,
) -> TimeseriesLayout {
    if before <= after {
        return TimeseriesLayout {
            after,
            before,
            bucket_seconds: 0,
            bucket_count: 0,
        };
    }

    let floor_seconds = source_tier
        .bucket_duration()
        .map(|duration| duration.as_secs() as u32)
        .unwrap_or(MIN_TIMESERIES_BUCKET_SECONDS)
        .max(MIN_TIMESERIES_BUCKET_SECONDS);
    let window = before - after;
    let raw_bucket_seconds = (window + TIMESERIES_MAX_BUCKETS - 1) / TIMESERIES_MAX_BUCKETS;
    let bucket_seconds = align_up(raw_bucket_seconds.max(floor_seconds), floor_seconds);
    let aligned_after = align_down(after, bucket_seconds);
    let aligned_before = align_up(before, bucket_seconds);
    let bucket_count = ((aligned_before.saturating_sub(aligned_after)) / bucket_seconds).max(1);

    TimeseriesLayout {
        after: aligned_after,
        before: aligned_before,
        bucket_seconds,
        bucket_count: bucket_count as usize,
    }
}

#[cfg(test)]
pub(crate) fn init_timeseries_layout(after: u32, before: u32) -> TimeseriesLayout {
    let source_tier = select_timeseries_source_tier(after, before, false);
    init_timeseries_layout_for_tier(after, before, source_tier)
}
