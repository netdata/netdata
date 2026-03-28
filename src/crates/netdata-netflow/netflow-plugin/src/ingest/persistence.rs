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
    ) {
        let Some(key) = FlowDecoders::decoder_state_namespace_key(source, payload) else {
            return;
        };

        if !self.decoders.is_decoder_state_namespace_loaded(&key) {
            self.decoders
                .mark_decoder_state_namespace_absent(key, source);
            return;
        }

        if self
            .decoders
            .decoder_state_source_needs_hydration(&key, source)
            && let Err(err) = self
                .decoders
                .hydrate_loaded_decoder_state_namespace(&key, source)
        {
            tracing::warn!(
                "failed to hydrate netflow decoder state namespace {} for {}: {}",
                self.decoder_state_namespace_path(&key).display(),
                source,
                err
            );
        }
    }

    pub(super) fn preload_decoder_state_namespaces(
        decoders: &mut FlowDecoders,
        decoder_state_dir: &Path,
    ) {
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

        let mut paths = read_dir
            .filter_map(|entry| entry.ok().map(|entry| entry.path()))
            .filter(|path| path.is_file())
            .collect::<Vec<_>>();
        paths.sort();

        for path in paths {
            match fs::read(&path) {
                Ok(data) => {
                    if let Err(err) = decoders.preload_decoder_state_namespace(&data) {
                        tracing::warn!(
                            "failed to preload netflow decoder state namespace {}: {}",
                            path.display(),
                            err
                        );
                    }
                }
                Err(err) => {
                    tracing::warn!(
                        "failed to read netflow decoder state namespace {}: {}",
                        path.display(),
                        err
                    );
                }
            }
        }
    }
}
