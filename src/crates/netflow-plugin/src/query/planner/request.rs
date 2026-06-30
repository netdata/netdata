use super::*;

pub(crate) fn resolve_time_bounds(request: &FlowsRequest) -> (u32, u32) {
    let now = Utc::now().timestamp().max(1);
    let mut before = request.before.unwrap_or(now);
    if before == 0 {
        before = now;
    } else if before < 0 {
        before = now.saturating_add(before).max(1);
    }
    let after = match request.after {
        Some(after) if after < 0 => before.saturating_add(after),
        Some(after) => after,
        None => before.saturating_sub(i64::from(DEFAULT_QUERY_WINDOW_SECONDS)),
    }
    .max(0);
    let before = clamp_query_timestamp(before);
    let mut after = clamp_query_timestamp(after);
    if after >= before {
        after = before.saturating_sub(1);
    }
    (after, before)
}

fn clamp_query_timestamp(timestamp: i64) -> u32 {
    if timestamp <= 0 {
        0
    } else {
        timestamp.min(i64::from(u32::MAX)) as u32
    }
}

pub(crate) fn sanitize_limit(top_n: TopN) -> usize {
    top_n.as_usize().clamp(DEFAULT_QUERY_LIMIT, MAX_QUERY_LIMIT)
}

pub(crate) fn sanitize_explicit_limit(limit: usize) -> usize {
    if limit == 0 {
        DEFAULT_QUERY_LIMIT
    } else {
        limit.min(MAX_QUERY_LIMIT)
    }
}

pub(crate) fn resolve_effective_group_by(request: &FlowsRequest) -> Vec<String> {
    if request.is_country_map_view() {
        return COUNTRY_MAP_GROUP_BY_FIELDS
            .iter()
            .map(|field| (*field).to_string())
            .collect();
    }
    if request.is_state_map_view() {
        return STATE_MAP_GROUP_BY_FIELDS
            .iter()
            .map(|field| (*field).to_string())
            .collect();
    }
    if request.is_city_map_view() {
        return CITY_MAP_GROUP_BY_FIELDS
            .iter()
            .map(|field| (*field).to_string())
            .collect();
    }

    request.normalized_group_by()
}

pub(crate) fn grouped_query_can_use_projected_scan(request: &FlowsRequest) -> bool {
    request.query.is_empty()
        && resolve_effective_group_by(request)
            .iter()
            .all(|field| journal_projected_group_field_supported(field))
        && request
            .selections
            .keys()
            .all(|field| journal_projected_selection_field_supported(field))
        && request
            .selections
            .values()
            .all(|values| !values.is_empty() && values.iter().all(|value| !value.is_empty()))
}
