use super::*;
use crate::memory_allocator::trim_allocator_if_worthwhile;

const REBUILD_SCAN_QUEUE_CAPACITY: usize = 1024;

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
        let before = now
            .saturating_add(999_999)
            .saturating_div(1_000_000)
            .min(u64::from(u32::MAX))
            .max(1) as u32;
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

                let after_usec = (after as u64).saturating_mul(1_000_000);
                let before_usec = (before as u64).saturating_mul(1_000_000);
                let file_paths = files
                    .iter()
                    .map(|file_info| PathBuf::from(file_info.file.path()))
                    .collect::<Vec<_>>();
                let rebuild_started = Instant::now();
                let rebuild_timeout = Duration::from_secs(REBUILD_TIMEOUT_SECONDS);
                let (rebuild_tx, mut rebuild_rx) =
                    tokio::sync::mpsc::channel::<(u64, crate::flow::FlowFields)>(
                        REBUILD_SCAN_QUEUE_CAPACITY,
                    );
                let scan_handle = tokio::task::spawn_blocking(move || -> Result<()> {
                    let mut scanned_entries = 0_u64;

                    scan_journal_files_forward(
                        &file_paths,
                        Some(after_usec),
                        Some(before_usec),
                        None,
                        0,
                        0,
                        &[],
                        "raw tier rebuild",
                        |_file_path, journal, entry_timestamp_usec, data_offsets, decompress_buf| {
                            scanned_entries = scanned_entries.saturating_add(1);
                            if rebuild_started.elapsed() >= rebuild_timeout {
                                return Err(anyhow!(
                                    "timed out scanning raw flows for tier rebuild after {}s (scanned {} entries)",
                                    REBUILD_TIMEOUT_SECONDS,
                                    scanned_entries
                                ));
                            }

                            let mut fields = crate::flow::FlowFields::new();
                            visit_journal_payloads(
                                journal,
                                _file_path,
                                data_offsets,
                                decompress_buf,
                                |payload| {
                                    let Some(eq_pos) = payload.iter().position(|&b| b == b'=') else {
                                        return Ok(());
                                    };
                                    let Ok(name) = std::str::from_utf8(&payload[..eq_pos]) else {
                                        return Ok(());
                                    };
                                    let Some(interned) = crate::decoder::intern_field_name(name) else {
                                        return Ok(());
                                    };
                                    let value =
                                        String::from_utf8_lossy(&payload[eq_pos + 1..]).into_owned();
                                    fields.insert(interned, value);
                                    Ok(())
                                },
                            )?;

                            if rebuild_started.elapsed() >= rebuild_timeout {
                                return Err(anyhow!(
                                    "timed out scanning raw flows for tier rebuild after {}s (scanned {} entries)",
                                    REBUILD_TIMEOUT_SECONDS,
                                    scanned_entries
                                ));
                            }

                            rebuild_tx
                                .blocking_send((entry_timestamp_usec, fields))
                                .map_err(|_| anyhow!("raw tier rebuild receiver dropped"))?;
                            Ok(true)
                        },
                    )
                    .context("failed to scan raw flows for tier rebuild")?;

                    Ok(())
                });

                while let Some((entry_timestamp_usec, fields)) = rebuild_rx.recv().await {
                    self.observe_tiers_with_cutoffs(entry_timestamp_usec, &fields, &tier_cutoffs);
                }

                scan_handle
                    .await
                    .context("raw tier rebuild scan task join failed")??;

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
