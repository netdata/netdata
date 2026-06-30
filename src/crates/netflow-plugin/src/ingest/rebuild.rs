use super::*;
use crate::memory_allocator::trim_allocator_if_worthwhile;

const REBUILD_SCAN_QUEUE_CAPACITY: usize = 1024;

impl IngestService {
    /// Find the most recent readable `_SOURCE_REALTIME_TIMESTAMP` in a tier's
    /// journal files. Returns `None` when the tier directory is empty or
    /// nothing is readable.
    ///
    /// This deliberately uses the tear-tolerant direct scanner instead of the
    /// index engine: after a power loss the newest tier file can have a torn
    /// tail, and an indexing failure here would collapse the cutoff to `None`,
    /// making the rebuild re-derive rows the tier journals still hold —
    /// duplicating them. The max readable source timestamp is exactly the
    /// cutoff contract. Files are visited newest-first and the first file
    /// that yields any readable row decides (tier rows are appended in
    /// monotonically increasing bucket order).
    pub(super) fn find_last_tier_timestamp(tier_dir: &std::path::Path) -> Option<u64> {
        let dir_str = tier_dir.to_str()?;
        let (monitor, _rx) = Monitor::new().ok()?;
        let registry = Registry::new(monitor);
        registry.watch_directory(dir_str).ok()?;

        let files = registry
            .find_files_in_range(Seconds(0), Seconds(u32::MAX))
            .ok()?;

        for file_info in files.iter().rev() {
            let path = vec![PathBuf::from(file_info.file.path())];
            let mut max_source_ts: Option<u64> = None;
            let scanned = crate::query::scan_journal_files_forward(
                &path,
                None,
                None,
                None,
                0,
                0,
                &[],
                "tier cutoff lookup",
                |file_path, journal, _timestamp_usec, data_offsets, decompress_buf| {
                    let mut source_ts: Option<u64> = None;
                    let visit_result = crate::query::visit_journal_payloads(
                        journal,
                        file_path,
                        data_offsets,
                        decompress_buf,
                        |payload| {
                            if let Some(value) =
                                payload.strip_prefix(b"_SOURCE_REALTIME_TIMESTAMP=")
                            {
                                source_ts = std::str::from_utf8(value)
                                    .ok()
                                    .and_then(|v| v.parse().ok());
                            }
                            Ok(())
                        },
                    );
                    if visit_result.is_err() {
                        // Torn payload: ignore this entry, keep what we have.
                        return Ok(false);
                    }
                    if let Some(ts) = source_ts {
                        max_source_ts = Some(max_source_ts.map_or(ts, |max| max.max(ts)));
                    }
                    Ok(true)
                },
            );
            if scanned.is_err() {
                continue;
            }
            if max_source_ts.is_some() {
                return max_source_ts;
            }
        }
        None
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
            let rebuild_result: Result<()> = async {
                let mut tier_cutoffs = HashMap::new();
                for tier in MATERIALIZED_TIERS {
                    let tier_dir = self.cfg.journal.tier_dir(tier);
                    if let Some(ts) = Self::find_last_tier_timestamp(&tier_dir) {
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
                            let visit_result = visit_journal_payloads(
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
                            );
                            if let Err(err) = visit_result {
                                // A torn payload (power loss mid-write) loses
                                // this entry only; the rebuild continues with
                                // everything that is still readable.
                                tracing::warn!(
                                    "skipping unreadable raw entry in {} during tier rebuild: {}",
                                    _file_path.display(),
                                    err
                                );
                                return Ok(false);
                            }

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

            rebuild_result?;
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
