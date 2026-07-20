use super::*;

impl IngestService {
    pub(super) fn persist_decoder_state_if_due(&mut self, now_usec: u64) {
        if now_usec.saturating_sub(self.last_decoder_state_persist_usec)
            < DECODER_STATE_PERSIST_INTERVAL_USEC
        {
            return;
        }
        self.persist_decoder_state();
        self.last_decoder_state_persist_usec = now_usec;
    }

    pub(crate) fn persist_decoder_state(&mut self) {
        for key in self.decoders.dirty_decoder_state_namespaces() {
            let path = self.decoder_state_namespace_path(&key);
            if self.protected_decoder_state_namespaces.contains(&key) {
                tracing::warn!(
                    "preserving unreadable netflow decoder state {}; updates for this stream will remain memory-only until restart",
                    path.display()
                );
                self.decoders.mark_decoder_state_namespace_persisted(&key);
                continue;
            }
            let data = match self.decoders.export_decoder_state_namespace(&key) {
                Ok(Some(data)) => data,
                Ok(None) => {
                    if path.is_file()
                        && let Err(err) = fs::remove_file(&path)
                    {
                        self.metrics
                            .decoder_state_write_errors
                            .fetch_add(1, Ordering::Relaxed);
                        tracing::warn!(
                            "failed to remove stale netflow decoder state {}: {}",
                            path.display(),
                            err
                        );
                        continue;
                    }
                    self.decoders.mark_decoder_state_namespace_persisted(&key);
                    continue;
                }
                Err(err) => {
                    tracing::warn!(
                        "failed to serialize netflow decoder state namespace {}: {}",
                        path.display(),
                        err
                    );
                    continue;
                }
            };

            self.metrics
                .decoder_state_persist_calls
                .fetch_add(1, Ordering::Relaxed);
            self.metrics
                .decoder_state_persist_bytes
                .fetch_add(data.len() as u64, Ordering::Relaxed);

            let tmp_path = path.with_extension("bin.tmp");
            if let Err(err) = fs::write(&tmp_path, &data) {
                self.metrics
                    .decoder_state_write_errors
                    .fetch_add(1, Ordering::Relaxed);
                tracing::warn!(
                    "failed to write temporary netflow decoder state {}: {}",
                    tmp_path.display(),
                    err
                );
                continue;
            }

            if let Err(err) = fs::rename(&tmp_path, &path) {
                self.metrics
                    .decoder_state_move_errors
                    .fetch_add(1, Ordering::Relaxed);
                tracing::warn!(
                    "failed to move netflow decoder state {} to {}: {}",
                    tmp_path.display(),
                    path.display(),
                    err
                );
                let _ = fs::remove_file(&tmp_path);
                continue;
            }

            self.decoders.mark_decoder_state_namespace_persisted(&key);
        }
    }

    fn decoder_state_namespace_path(&self, key: &DecoderStateNamespaceKey) -> PathBuf {
        self.decoder_state_dir
            .join(FlowDecoders::decoder_state_namespace_filename(key))
    }

    pub(super) fn prepare_decoder_state_namespace(
        &mut self,
        source: std::net::SocketAddr,
        payload: &[u8],
    ) -> Option<DecoderPacketContext> {
        let Some(context) = FlowDecoders::decoder_packet_context(source, payload) else {
            return None;
        };

        if !self
            .decoders
            .is_decoder_state_namespace_loaded(&context.key)
        {
            let path = self.decoder_state_namespace_path(&context.key);
            match fs::metadata(&path) {
                Ok(metadata)
                    if metadata.len() > crate::decoder::MAX_DECODER_STATE_FILE_LEN as u64 =>
                {
                    tracing::warn!(
                        "preserving oversized netflow decoder state namespace {} (max {} bytes, got {})",
                        path.display(),
                        crate::decoder::MAX_DECODER_STATE_FILE_LEN,
                        metadata.len()
                    );
                    self.protected_decoder_state_namespaces
                        .insert(context.key.clone());
                    self.decoders
                        .mark_decoder_state_namespace_absent_for_normalized_source(
                            context.key.clone(),
                            context.parser_source,
                        );
                }
                Ok(_) => match fs::read(&path) {
                    Ok(data) => {
                        if let Err(err) = self.decoders.import_decoder_state_namespace(
                            context.key.clone(),
                            context.parser_source,
                            &data,
                        ) {
                            tracing::warn!(
                                "preserving unreadable netflow decoder state namespace {}: {}",
                                path.display(),
                                err
                            );
                            self.protected_decoder_state_namespaces
                                .insert(context.key.clone());
                            self.decoders
                                .mark_decoder_state_namespace_absent_for_normalized_source(
                                    context.key.clone(),
                                    context.parser_source,
                                );
                        }
                    }
                    Err(err) => {
                        tracing::warn!(
                            "failed to read netflow decoder state namespace {}: {}",
                            path.display(),
                            err
                        );
                        self.protected_decoder_state_namespaces
                            .insert(context.key.clone());
                        self.decoders
                            .mark_decoder_state_namespace_absent_for_normalized_source(
                                context.key.clone(),
                                context.parser_source,
                            );
                    }
                },
                Err(err) if err.kind() == std::io::ErrorKind::NotFound => self
                    .decoders
                    .mark_decoder_state_namespace_absent_for_normalized_source(
                        context.key.clone(),
                        context.parser_source,
                    ),
                Err(err) => {
                    tracing::warn!(
                        "failed to inspect netflow decoder state namespace {}: {}",
                        path.display(),
                        err
                    );
                    self.protected_decoder_state_namespaces
                        .insert(context.key.clone());
                    self.decoders
                        .mark_decoder_state_namespace_absent_for_normalized_source(
                            context.key.clone(),
                            context.parser_source,
                        );
                }
            }
            return Some(context);
        }

        if self
            .decoders
            .decoder_state_normalized_source_needs_hydration(&context.key, context.parser_source)
            && let Err(err) = self
                .decoders
                .hydrate_loaded_decoder_state_namespace_for_normalized_source(
                    &context.key,
                    context.parser_source,
                )
        {
            tracing::warn!(
                "failed to hydrate netflow decoder state namespace {} for {}: {}",
                self.decoder_state_namespace_path(&context.key).display(),
                source,
                err
            );
        }

        Some(context)
    }

    pub(super) fn cleanup_obsolete_decoder_state_namespaces(decoder_state_dir: &Path) {
        let read_dir = match fs::read_dir(decoder_state_dir) {
            Ok(entries) => entries,
            Err(err) => {
                tracing::warn!(
                    "failed to read netflow decoder state directory {}: {}",
                    decoder_state_dir.display(),
                    err
                );
                return;
            }
        };

        for entry in read_dir {
            let Ok(entry) = entry else {
                continue;
            };
            let path = entry.path();
            if !path.is_file() {
                continue;
            }
            let mut header = [0_u8; 8];
            let Ok(mut file) = fs::File::open(&path) else {
                continue;
            };
            if file.read_exact(&mut header).is_err() {
                continue;
            }
            if matches!(
                crate::decoder::decoder_state_schema_version(&header),
                Some(2 | 3)
            ) && let Err(err) = fs::remove_file(&path)
            {
                tracing::warn!(
                    "failed to remove obsolete netflow decoder state namespace {}: {}",
                    path.display(),
                    err
                );
            }
        }
    }
}
