use super::*;
use crate::memory_allocator::trim_allocator_if_worthwhile;

impl IngestService {
    /// Query the most recent `_SOURCE_REALTIME_TIMESTAMP` from a tier's journal
    /// files. Returns `None` when the tier directory is empty or unreadable.
    pub(super) async fn find_last_tier_timestamp(
        tier_dir: &std::path::Path,
        cache: &journal_engine::FileIndexCache,
    ) -> Option<u64> {
        let dir_str = tier_dir.to_str()?;
        let (monitor, _rx) = Monitor::new().ok()?;
        let registry = Registry::new(monitor);
        registry.watch_directory(dir_str).ok()?;

        let files = registry
            .find_files_in_range(Seconds(0), Seconds(u32::MAX))
            .ok()?;
        if files.is_empty() {
            return None;
        }

        let source_ts = FieldName::new_unchecked("_SOURCE_REALTIME_TIMESTAMP");
        let facets = Facets::new(&["_SOURCE_REALTIME_TIMESTAMP".to_string()]);
        let keys: Vec<FileIndexKey> = files
            .iter()
            .map(|fi| FileIndexKey::new(&fi.file, &facets, Some(source_ts.clone())))
            .collect();

        // QueryTimeRange aligns the end boundary up to a histogram bucket.
        // Using u32::MAX here overflows that alignment math in debug builds.
        let query_end = super::tier_timestamp_lookup_query_end(super::now_usec());
        let time_range = QueryTimeRange::new(0, query_end).ok()?;
        let cancel = CancellationToken::new();
        let indexed = batch_compute_file_indexes(
            cache,
            &registry,
            keys,
            &time_range,
            cancel,
            IndexingLimits::default(),
            None,
        )
        .await
        .ok()?;

        let indexes: Vec<_> = indexed.into_iter().map(|(_, idx)| idx).collect();
        if indexes.is_empty() {
            return None;
        }

        let entries = LogQuery::new(&indexes, Anchor::Tail, Direction::Backward)
            .with_limit(1)
            .execute()
            .ok()?;

        let entry = entries.into_iter().next()?;
        for pair in &entry.fields {
            if pair.field() == "_SOURCE_REALTIME_TIMESTAMP" {
                return pair.value().parse::<u64>().ok();
            }
        }
        Some(entry.timestamp)
    }

    pub(super) async fn rebuild_materialized_from_raw(&mut self) -> Result<()> {
        let now = now_usec();
        let before = (now / 1_000_000).max(1) as u32;
        let after = before.saturating_sub(REBUILD_WINDOW_SECONDS);

        let raw_dir = self.cfg.journal.raw_tier_dir();
        let raw_dir_str = raw_dir
            .to_str()
            .context("raw tier directory contains invalid UTF-8")?;

        let (monitor, _notify_rx) = Monitor::new().context("failed to initialize raw monitor")?;
        let registry = Registry::new(monitor);
        registry
            .watch_directory(raw_dir_str)
            .with_context(|| format!("failed to watch raw tier directory {}", raw_dir.display()))?;

        let files = registry
            .find_files_in_range(Seconds(after), Seconds(before))
            .context("failed to find raw files for tier rebuild")?;
        if files.is_empty() {
            self.refresh_open_tier_state(now);
            return Ok(());
        }

        {
            let cache = FileIndexCacheBuilder::new()
                .with_memory_capacity(REBUILD_CACHE_MEMORY_CAPACITY)
                .without_disk_cache()
                .build()
                .await
                .context("failed to initialize rebuild index cache")?;

            let rebuild_result: Result<()> = async {
                let mut tier_cutoffs = HashMap::new();
                for tier in MATERIALIZED_TIERS {
                    let tier_dir = self.cfg.journal.tier_dir(tier);
                    if let Some(ts) = Self::find_last_tier_timestamp(&tier_dir, &cache).await {
                        tracing::info!(
                            "tier {:?}: last flushed timestamp {} — skipping rebuild before it",
                            tier,
                            ts
                        );
                        tier_cutoffs.insert(tier, ts);
                    }
                }

                let source_timestamp_field = FieldName::new_unchecked("_SOURCE_REALTIME_TIMESTAMP");
                let facets = Facets::new(&["_SOURCE_REALTIME_TIMESTAMP".to_string()]);
                let keys: Vec<FileIndexKey> = files
                    .iter()
                    .map(|file_info| {
                        FileIndexKey::new(
                            &file_info.file,
                            &facets,
                            Some(source_timestamp_field.clone()),
                        )
                    })
                    .collect();

                let time_range =
                    QueryTimeRange::new(after, before).context("invalid rebuild raw time range")?;

                let indexing_cancellation = CancellationToken::new();
                let indexed_files = tokio::select! {
                    result = batch_compute_file_indexes(
                        &cache,
                        &registry,
                        keys,
                        &time_range,
                        indexing_cancellation.clone(),
                        IndexingLimits::default(),
                        None,
                    ) => result.context("failed to build raw indexes for tier rebuild"),
                    _ = tokio::time::sleep(Duration::from_secs(REBUILD_TIMEOUT_SECONDS)) => {
                        indexing_cancellation.cancel();
                        Err(anyhow!(
                            "timed out building raw indexes for tier rebuild after {}s",
                            REBUILD_TIMEOUT_SECONDS
                        ))
                    }
                }?;
                let file_indexes: Vec<_> = indexed_files.into_iter().map(|(_, idx)| idx).collect();

                if file_indexes.is_empty() {
                    self.refresh_open_tier_state(now);
                    return Ok(());
                }

                let after_usec = (after as u64).saturating_mul(1_000_000);
                let before_usec = (before as u64).saturating_mul(1_000_000);
                let anchor_usec = before_usec.saturating_sub(1);

                let entries = LogQuery::new(
                    &file_indexes,
                    Anchor::Timestamp(Microseconds(anchor_usec)),
                    Direction::Backward,
                )
                .with_after_usec(after_usec)
                .with_before_usec(before_usec)
                .execute()
                .context("failed to query raw flows for tier rebuild")?;

                for entry in entries {
                    let mut fields = crate::flow::FlowFields::new();
                    for pair in entry.fields {
                        let name = pair.field();
                        if let Some(interned) = crate::decoder::intern_field_name(name) {
                            fields.insert(interned, pair.value().to_string());
                        }
                    }

                    self.observe_tiers_with_cutoffs(entry.timestamp, &fields, &tier_cutoffs);
                }

                Ok(())
            }
            .await;

            let close_result = cache
                .close()
                .await
                .context("failed to close rebuild index cache");
            rebuild_result?;
            close_result?;
        }

        self.flush_closed_tiers(now)?;
        self.prune_unused_tier_flow_indexes();
        self.refresh_open_tier_state(now);

        if let Some(trimmed) = trim_allocator_if_worthwhile() {
            tracing::info!(
                before_heap_free = trimmed.before.heap_free_bytes,
                after_heap_free = trimmed.after.heap_free_bytes,
                before_heap_arena = trimmed.before.heap_arena_bytes,
                after_heap_arena = trimmed.after.heap_arena_bytes,
                "trimmed glibc heap after raw rebuild"
            );
        }

        Ok(())
    }

    #[cfg(test)]
    pub(crate) async fn rebuild_materialized_from_raw_for_test(&mut self) -> Result<()> {
        self.rebuild_materialized_from_raw().await
    }
}
