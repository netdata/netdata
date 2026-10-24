use super::*;

fn push_query_tier_span(spans: &mut Vec<QueryTierSpan>, tier: TierKind, after: u32, before: u32) {
    if before <= after {
        return;
    }

    if let Some(last) = spans.last_mut() {
        if last.tier == tier && last.before == after {
            last.before = before;
            return;
        }
    }

    spans.push(QueryTierSpan {
        tier,
        after,
        before,
    });
}

fn plan_query_tier_spans_recursive(
    spans: &mut Vec<QueryTierSpan>,
    after: u32,
    before: u32,
    candidate_tiers: &[TierKind],
) {
    if before <= after {
        return;
    }

    let Some((&tier, remaining_tiers)) = candidate_tiers.split_first() else {
        push_query_tier_span(spans, TierKind::Raw, after, before);
        return;
    };

    let Some(step) = tier
        .bucket_duration()
        .map(|duration| duration.as_secs() as u32)
    else {
        push_query_tier_span(spans, TierKind::Raw, after, before);
        return;
    };

    let aligned_after = align_up(after, step);
    let aligned_before = align_down(before, step);

    if aligned_after < aligned_before {
        plan_query_tier_spans_recursive(spans, after, aligned_after, remaining_tiers);
        push_query_tier_span(spans, tier, aligned_after, aligned_before);
        plan_query_tier_spans_recursive(spans, aligned_before, before, remaining_tiers);
    } else {
        plan_query_tier_spans_recursive(spans, after, before, remaining_tiers);
    }
}

pub(crate) fn plan_query_tier_spans(
    after: u32,
    before: u32,
    candidate_tiers: &[TierKind],
    force_raw: bool,
) -> Vec<QueryTierSpan> {
    if before <= after {
        return Vec::new();
    }
    if force_raw {
        return vec![QueryTierSpan {
            tier: TierKind::Raw,
            after,
            before,
        }];
    }

    let mut spans = Vec::new();
    plan_query_tier_spans_recursive(&mut spans, after, before, candidate_tiers);
    spans
}

pub(crate) fn summary_query_tier(spans: &[QueryTierSpan]) -> TierKind {
    if spans.iter().any(|span| span.tier == TierKind::Hour1) {
        TierKind::Hour1
    } else if spans.iter().any(|span| span.tier == TierKind::Minute5) {
        TierKind::Minute5
    } else if spans.iter().any(|span| span.tier == TierKind::Minute1) {
        TierKind::Minute1
    } else {
        TierKind::Raw
    }
}

pub(crate) fn timeseries_candidate_tiers(source_tier: TierKind) -> &'static [TierKind] {
    match source_tier {
        TierKind::Hour1 => &[TierKind::Hour1, TierKind::Minute5, TierKind::Minute1],
        TierKind::Minute5 => &[TierKind::Minute5, TierKind::Minute1],
        TierKind::Minute1 | TierKind::Raw => &[TierKind::Minute1],
    }
}

pub(crate) fn lower_fallback_candidate_tiers(tier: TierKind) -> &'static [TierKind] {
    match tier {
        TierKind::Hour1 => &[TierKind::Minute5, TierKind::Minute1],
        TierKind::Minute5 => &[TierKind::Minute1],
        TierKind::Minute1 | TierKind::Raw => &[],
    }
}
