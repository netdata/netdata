use super::*;

impl FlowQueryService {
    pub(crate) fn files_for_query_span(&self, span: QueryTierSpan) -> Result<Vec<PathBuf>> {
        let tier_dir = self
            .tier_dirs
            .get(&span.tier)
            .context("missing selected tier directory")?;
        Ok(self
            .registry
            .find_files_in_range(Seconds(span.after), Seconds(span.before))
            .context("failed to locate netflow journal files in time range")?
            .into_iter()
            .filter(|file_info| Path::new(file_info.file.path()).starts_with(tier_dir.as_path()))
            .map(|file_info| PathBuf::from(file_info.file.path()))
            .collect())
    }

    pub(crate) fn prepare_query_span_with_fallback(
        &self,
        span: QueryTierSpan,
        prepared: &mut Vec<PreparedQuerySpan>,
    ) -> Result<()> {
        let files = self.files_for_query_span(span)?;
        if span.tier == TierKind::Raw || !files.is_empty() {
            prepared.push(PreparedQuerySpan { span, files });
            return Ok(());
        }

        for fallback_span in plan_query_tier_spans(
            span.after,
            span.before,
            lower_fallback_candidate_tiers(span.tier),
            false,
        ) {
            self.prepare_query_span_with_fallback(fallback_span, prepared)?;
        }

        Ok(())
    }

    pub(crate) fn prepare_query(&self, request: &FlowsRequest) -> Result<QuerySetup> {
        let sort_by = request.normalized_sort_by();
        let (requested_after, requested_before) = resolve_time_bounds(request);
        let effective_group_by = resolve_effective_group_by(request);
        let force_raw_tier =
            requires_raw_tier_for_fields(&effective_group_by, &request.selections, &request.query);
        let (timeseries_layout, after, before, planned_spans) = if request.is_timeseries_view() {
            let source_tier =
                select_timeseries_source_tier(requested_after, requested_before, force_raw_tier);
            let layout =
                init_timeseries_layout_for_tier(requested_after, requested_before, source_tier);
            let planned_spans = plan_query_tier_spans(
                layout.after,
                layout.before,
                timeseries_candidate_tiers(source_tier),
                force_raw_tier,
            );
            (Some(layout), layout.after, layout.before, planned_spans)
        } else {
            let planned_spans = plan_query_tier_spans(
                requested_after,
                requested_before,
                &[TierKind::Hour1, TierKind::Minute5, TierKind::Minute1],
                force_raw_tier,
            );
            (None, requested_after, requested_before, planned_spans)
        };

        let mut spans = Vec::with_capacity(planned_spans.len());
        for span in planned_spans {
            self.prepare_query_span_with_fallback(span, &mut spans)?;
        }
        let selected_tier = summary_query_tier(
            &spans
                .iter()
                .map(|prepared| prepared.span)
                .collect::<Vec<_>>(),
        );

        let limit = sanitize_limit(request.top_n);
        let mut stats = HashMap::new();
        stats.insert(
            "query_tier".to_string(),
            match selected_tier {
                TierKind::Raw => 0,
                TierKind::Minute1 => 1,
                TierKind::Minute5 => 5,
                TierKind::Hour1 => 60,
            },
        );
        stats.insert("query_after".to_string(), after as u64);
        stats.insert("query_before".to_string(), before as u64);
        stats.insert("query_requested_after".to_string(), requested_after as u64);
        stats.insert(
            "query_requested_before".to_string(),
            requested_before as u64,
        );
        stats.insert("query_limit".to_string(), limit as u64);
        stats.insert(
            "query_files".to_string(),
            spans.iter().map(|span| span.files.len() as u64).sum(),
        );
        stats.insert("query_span_count".to_string(), spans.len() as u64);
        stats.insert(
            "query_raw_spans".to_string(),
            spans
                .iter()
                .filter(|span| span.span.tier == TierKind::Raw)
                .count() as u64,
        );
        stats.insert(
            "query_1m_spans".to_string(),
            spans
                .iter()
                .filter(|span| span.span.tier == TierKind::Minute1)
                .count() as u64,
        );
        stats.insert(
            "query_5m_spans".to_string(),
            spans
                .iter()
                .filter(|span| span.span.tier == TierKind::Minute5)
                .count() as u64,
        );
        stats.insert(
            "query_1h_spans".to_string(),
            spans
                .iter()
                .filter(|span| span.span.tier == TierKind::Hour1)
                .count() as u64,
        );
        stats.insert(
            "query_forced_raw_tier".to_string(),
            u64::from(force_raw_tier),
        );
        stats.insert(
            "query_group_by_fields".to_string(),
            effective_group_by.len() as u64,
        );
        stats.insert(
            "query_sort_metric".to_string(),
            match sort_by {
                SortBy::Bytes => 1,
                SortBy::Packets => 2,
            },
        );
        stats.insert(
            "query_group_accumulator_limit".to_string(),
            self.max_groups as u64,
        );
        if let Some(layout) = timeseries_layout {
            stats.insert(
                "query_bucket_seconds".to_string(),
                layout.bucket_seconds as u64,
            );
            stats.insert("query_bucket_count".to_string(), layout.bucket_count as u64);
        }

        Ok(QuerySetup {
            sort_by,
            timeseries_layout,
            effective_group_by,
            limit,
            spans,
            stats,
        })
    }
}
