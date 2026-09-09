use super::*;

static DECODER_STATE_QUARANTINE_SEQUENCE: AtomicU64 = AtomicU64::new(0);

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
            match read_decoder_state_namespace(&path) {
                Ok(None) => self.mark_decoder_state_namespace_absent(&context),
                Ok(Some(data)) if data.len() > crate::decoder::MAX_DECODER_STATE_FILE_LEN => {
                    let reason = format!(
                        "decoder state exceeds the {} byte limit",
                        crate::decoder::MAX_DECODER_STATE_FILE_LEN
                    );
                    self.handle_invalid_decoder_state(&context, &path, &data, &reason);
                }
                Ok(Some(data)) => {
                    if let Err(err) = self.decoders.import_decoder_state_namespace(
                        context.key.clone(),
                        context.parser_source,
                        &data,
                    ) {
                        self.handle_invalid_decoder_state(&context, &path, &data, &err);
                    }
                }
                Err(err) => {
                    tracing::warn!(
                        "preserving unreadable netflow decoder state namespace {}: {}",
                        path.display(),
                        err
                    );
                    self.protect_decoder_state_namespace(&context);
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

    fn handle_invalid_decoder_state(
        &mut self,
        context: &DecoderPacketContext,
        path: &Path,
        data: &[u8],
        reason: &str,
    ) {
        if crate::decoder::decoder_state_schema_version(data)
            != Some(crate::decoder::DECODER_STATE_SCHEMA_VERSION)
        {
            tracing::warn!(
                "preserving unsupported netflow decoder state namespace {}: {}",
                path.display(),
                reason
            );
            self.protect_decoder_state_namespace(context);
            return;
        }

        match quarantine_decoder_state_namespace(path) {
            Ok(quarantine_path) => {
                tracing::warn!(
                    "moved invalid netflow decoder state namespace {} to {}: {}",
                    path.display(),
                    quarantine_path.display(),
                    reason
                );
                self.mark_decoder_state_namespace_absent(context);
            }
            Err(err) => {
                self.metrics
                    .decoder_state_move_errors
                    .fetch_add(1, Ordering::Relaxed);
                tracing::warn!(
                    "failed to quarantine invalid netflow decoder state namespace {}: {}; original error: {}",
                    path.display(),
                    err,
                    reason
                );
                self.protect_decoder_state_namespace(context);
            }
        }
    }

    fn mark_decoder_state_namespace_absent(&mut self, context: &DecoderPacketContext) {
        self.decoders
            .mark_decoder_state_namespace_absent_for_normalized_source(
                context.key.clone(),
                context.parser_source,
            );
    }

    fn protect_decoder_state_namespace(&mut self, context: &DecoderPacketContext) {
        self.protected_decoder_state_namespaces
            .insert(context.key.clone());
        self.mark_decoder_state_namespace_absent(context);
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
            if path.extension().and_then(|extension| extension.to_str()) != Some("bin") {
                continue;
            }
            let Ok(metadata) = fs::symlink_metadata(&path) else {
                continue;
            };
            if !metadata.file_type().is_file() {
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
                Some(2..=4)
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

fn read_decoder_state_namespace(path: &Path) -> std::io::Result<Option<Vec<u8>>> {
    let path_metadata = match fs::symlink_metadata(path) {
        Ok(metadata) => metadata,
        Err(err) if err.kind() == std::io::ErrorKind::NotFound => return Ok(None),
        Err(err) => return Err(err),
    };
    if !path_metadata.file_type().is_file() {
        return Err(std::io::Error::new(
            std::io::ErrorKind::InvalidData,
            "decoder state path is not a regular file",
        ));
    }

    let mut file = fs::File::open(path)?;
    let metadata = file.metadata()?;
    if !metadata.file_type().is_file() {
        return Err(std::io::Error::new(
            std::io::ErrorKind::InvalidData,
            "opened decoder state is not a regular file",
        ));
    }

    let read_limit = crate::decoder::MAX_DECODER_STATE_FILE_LEN + 1;
    let initial_capacity = usize::try_from(metadata.len())
        .unwrap_or(read_limit)
        .min(read_limit);
    let mut data = Vec::with_capacity(initial_capacity);
    file.by_ref()
        .take(read_limit as u64)
        .read_to_end(&mut data)?;
    Ok(Some(data))
}

fn quarantine_decoder_state_namespace(path: &Path) -> std::io::Result<PathBuf> {
    let file_name = path.file_name().ok_or_else(|| {
        std::io::Error::new(
            std::io::ErrorKind::InvalidInput,
            "decoder state path has no file name",
        )
    })?;

    for _ in 0..16 {
        let sequence = DECODER_STATE_QUARANTINE_SEQUENCE.fetch_add(1, Ordering::Relaxed);
        let mut quarantine_name = file_name.to_os_string();
        quarantine_name.push(format!(
            ".quarantine-{}-{}-{sequence}",
            now_usec(),
            std::process::id()
        ));
        let quarantine_path = path.with_file_name(quarantine_name);
        match fs::symlink_metadata(&quarantine_path) {
            Err(err) if err.kind() == std::io::ErrorKind::NotFound => {
                fs::rename(path, &quarantine_path)?;
                return Ok(quarantine_path);
            }
            Ok(_) => continue,
            Err(err) => return Err(err),
        }
    }

    Err(std::io::Error::new(
        std::io::ErrorKind::AlreadyExists,
        "could not allocate a unique decoder state quarantine path",
    ))
}
