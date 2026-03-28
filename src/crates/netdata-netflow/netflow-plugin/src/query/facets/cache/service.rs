use super::*;

impl FlowQueryService {
    pub(crate) fn facet_vocabulary_payload(&self, request: &FlowsRequest) -> Result<Value> {
        let requested_fields = requested_facet_fields(request);
        let closed_values = self.closed_facet_vocabulary()?;
        let active_values = self.active_facet_vocabulary()?;

        Ok(build_facet_vocabulary_payload(
            &requested_fields,
            &request.selections,
            &closed_values.values,
            &active_values,
        ))
    }

    pub(crate) fn closed_facet_vocabulary(&self) -> Result<Arc<ClosedFacetVocabularyCache>> {
        let archived_files = self
            .registry
            .find_files_in_range(Seconds(0), Seconds(u32::MAX))
            .context("failed to enumerate retained netflow journal files for facet cache")?
            .into_iter()
            .filter(|file_info| file_info.file.is_archived())
            .collect::<Vec<_>>();
        let archived_paths = archived_file_paths(&archived_files);

        if let Ok(cache_guard) = self.closed_facet_cache.read()
            && let Some(existing_cache) = cache_guard.as_ref()
        {
            if existing_cache.archived_paths == archived_paths {
                return Ok(Arc::clone(existing_cache));
            }

            if existing_cache.archived_paths.is_subset(&archived_paths) {
                let added_files = archived_files
                    .iter()
                    .filter(|file_info| {
                        !existing_cache
                            .archived_paths
                            .contains(file_info.file.path())
                    })
                    .cloned()
                    .collect::<Vec<_>>();
                if !added_files.is_empty() {
                    let base_values = existing_cache.values.clone();
                    drop(cache_guard);
                    let added_values = self.build_closed_facet_vocabulary(&added_files)?;
                    let merged = Arc::new(ClosedFacetVocabularyCache {
                        archived_paths: archived_paths.clone(),
                        values: merge_facet_vocabulary_values(&base_values, &added_values),
                    });

                    let mut cache = self
                        .closed_facet_cache
                        .write()
                        .map_err(|_| anyhow::anyhow!("facet cache lock poisoned"))?;
                    if let Some(existing) = cache.as_ref()
                        && existing.archived_paths == archived_paths
                    {
                        return Ok(Arc::clone(existing));
                    }
                    *cache = Some(Arc::clone(&merged));
                    return Ok(merged);
                }
            }
        }

        let rebuilt = Arc::new(ClosedFacetVocabularyCache {
            archived_paths: archived_paths.clone(),
            values: self.build_closed_facet_vocabulary(&archived_files)?,
        });

        let mut cache = self
            .closed_facet_cache
            .write()
            .map_err(|_| anyhow::anyhow!("facet cache lock poisoned"))?;
        if let Some(existing) = cache.as_ref()
            && existing.archived_paths == archived_paths
        {
            return Ok(Arc::clone(existing));
        }
        *cache = Some(Arc::clone(&rebuilt));

        Ok(rebuilt)
    }

    pub(crate) fn build_closed_facet_vocabulary(
        &self,
        registry_files: &[FileInfo],
    ) -> Result<BTreeMap<String, Vec<String>>> {
        let requested_fields = FACET_ALLOWED_OPTIONS.clone();
        let requested_set = requested_fields.iter().cloned().collect::<HashSet<_>>();
        let simple_fields = requested_fields
            .iter()
            .filter(|field| !facet_field_requires_protocol_scan(field))
            .cloned()
            .collect::<Vec<_>>();

        let mut values = BTreeMap::new();
        let mut file_paths = Vec::with_capacity(registry_files.len());

        for file_info in registry_files {
            file_paths.push(PathBuf::from(file_info.file.path()));
            let journal = JournalFileMap::open(&file_info.file, FACET_CACHE_JOURNAL_WINDOW_SIZE)
                .with_context(|| {
                    format!(
                        "failed to open netflow journal {} for facet cache build",
                        file_info.file.path()
                    )
                })?;
            accumulate_simple_closed_file_facet_values(&journal, &simple_fields, &mut values)
                .with_context(|| {
                    format!(
                        "failed to enumerate facet values from {}",
                        file_info.file.path()
                    )
                })?;
        }

        if !file_paths.is_empty() {
            if requested_fields.iter().any(|field| field == "ICMPV4") {
                accumulate_targeted_facet_values(
                    &file_paths,
                    "ICMPV4",
                    &[("PROTOCOL".to_string(), "1".to_string())],
                    virtual_flow_field_dependencies("ICMPV4"),
                    &mut values,
                )
                .context("failed to scan ICMPv4 facet values for retained netflow journals")?;
            }
            if requested_fields.iter().any(|field| field == "ICMPV6") {
                accumulate_targeted_facet_values(
                    &file_paths,
                    "ICMPV6",
                    &[("PROTOCOL".to_string(), "58".to_string())],
                    virtual_flow_field_dependencies("ICMPV6"),
                    &mut values,
                )
                .context("failed to scan ICMPv6 facet values for retained netflow journals")?;
            }
        }

        Ok(finalize_facet_vocabulary(values, &requested_set))
    }

    pub(crate) fn active_facet_vocabulary(&self) -> Result<BTreeMap<String, Vec<String>>> {
        let active_files = self
            .registry
            .find_files_in_range(Seconds(0), Seconds(u32::MAX))
            .context("failed to enumerate active netflow journal files for facet vocabulary")?
            .into_iter()
            .filter(|file_info| file_info.file.is_active())
            .collect::<Vec<_>>();
        self.build_closed_facet_vocabulary(&active_files)
    }
}
