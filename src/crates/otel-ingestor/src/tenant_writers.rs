//! The WAL writers of one signal, one per tenant. The logs and traces
//! services each own a [`TenantWriters`]: an export asks it for the tenant's
//! writer, and the periodic idle-rotation sweep asks it to rotate what has
//! expired.
use std::collections::HashMap;
use std::path::PathBuf;
use std::sync::{Arc, Mutex};

use file_registry::{Identity, TenantId, TimestampNs};
use otel_bridge::signals::Signal;

use crate::ledger_sender::LedgerSender;

/// One WAL writer per tenant, for one signal.
///
/// Two locks, never held together by this type: the map's, held only to look
/// a writer up, create it, or snapshot the handles; and each writer's own,
/// which an export holds for its serialized write+sync region. So tenants
/// ingest in parallel, and same-tenant requests overlap everything before
/// that region.
pub(crate) struct TenantWriters {
    writers: Mutex<HashMap<TenantId, Arc<Mutex<wal::Writer>>>>,
    signal: Signal,
    /// Stamped into every WAL file: the signal's pipeline id and the format
    /// of its frame payloads.
    file_stamp: wal::FileStamp,
    /// Each tenant's WAL files live in `base_dir/<tenant>`.
    base_dir: PathBuf,
    config: otel_bridge::config::WalConfig,
    /// The global seq allocator (lib.rs hands the same one to the logs and
    /// traces services), so file `seq` is unique across signals.
    seq_allocator: Arc<wal::SeqAllocator>,
    /// Identity stamped into every WAL FileId (the machine GUID resolved by the
    /// supervisor plus its self-generated per-process instance id; see
    /// `otel_bridge::config::PluginConfig::identity`).
    instance_identity: Identity,
}

impl TenantWriters {
    pub(crate) fn new(
        signal: Signal,
        payload_format: u16,
        base_dir: PathBuf,
        config: otel_bridge::config::WalConfig,
        seq_allocator: Arc<wal::SeqAllocator>,
        instance_identity: Identity,
    ) -> Self {
        Self {
            writers: Mutex::new(HashMap::new()),
            signal,
            file_stamp: wal::FileStamp {
                pipeline_id: signal.pipeline_id(),
                payload_format,
            },
            base_dir,
            config,
            seq_allocator,
            instance_identity,
        }
    }

    /// The tenant's writer; its first request creates it, under the map's
    /// lock.
    pub(crate) fn writer_for(&self, tenant_id: &TenantId) -> wal::Result<Arc<Mutex<wal::Writer>>> {
        let mut writers = self.writers.lock().unwrap();
        if let Some(writer) = writers.get(tenant_id) {
            return Ok(Arc::clone(writer));
        }

        let path = self.base_dir.join(tenant_id.as_str());
        let writer = wal::Writer::new(
            &path,
            self.resolve_config(tenant_id.as_str()),
            Arc::clone(&self.seq_allocator),
            self.file_stamp,
            self.instance_identity,
        )?;
        let writer = Arc::new(Mutex::new(writer));
        writers.insert(tenant_id.clone(), Arc::clone(&writer));
        Ok(writer)
    }

    /// The tenant's effective WAL rotation config: its per-tenant override,
    /// or the global default when it has none (see `RotationPolicy::resolve`).
    fn resolve_config(&self, tenant_id: &str) -> wal::Config {
        let rotation = self.config.rotation.resolve(tenant_id);
        wal::Config {
            rotation: wal::RotationConfig {
                max_entries: rotation.max_entries,
                max_file_size: file_registry::ByteSize(rotation.max_file_size.as_u64()),
                max_duration: Some(rotation.max_file_duration),
            },
            crc_enabled: self.config.crc_enabled,
            compression_enabled: self.config.compression_enabled,
        }
    }

    /// The default (non-overridden) WAL rotation `max_duration` — the value the
    /// idle-rotation sweep enforces for most streams.
    pub(crate) fn default_max_file_duration(&self) -> std::time::Duration {
        self.config.rotation.resolve("default").max_file_duration
    }

    /// Rotate every tenant's WAL streams whose active file has passed a
    /// rotation threshold as of `now_ns`, without a new frame, and forward
    /// the resulting lifecycle events to the ledger.
    ///
    /// Lock discipline:
    /// - the caller reads the clock for `now_ns` and drops its guard before
    ///   calling: the sweep never holds the clock while acquiring a writer
    ///   lock, so the export path's one lock nesting (writer → clock, for the
    ///   per-frame `ingestion_ns` tick) cannot form an AB-BA cycle with it;
    /// - snapshot the tenant→writer handles under the map lock (held only for the
    ///   clone), so exports and new-tenant creation are not blocked by the sweep;
    /// - per writer: lock it, rotate, then drain AND send UNDER that lock so the
    ///   sealed file's events reach the ledger in order (see the export path).
    pub(crate) fn rotate_expired(&self, now_ns: TimestampNs, ledger_sender: &LedgerSender) {
        // The traces messages name their signal; the logs ones never did.
        let wal = match self.signal {
            Signal::Logs => "WAL",
            Signal::Traces => "traces WAL",
        };

        let writers: Vec<(TenantId, Arc<Mutex<wal::Writer>>)> = {
            let guard = self.writers.lock().unwrap();
            guard
                .iter()
                .map(|(t, w)| (t.clone(), Arc::clone(w)))
                .collect()
        };

        for (tenant_id, writer) in writers {
            let (result, forwarded) = {
                let mut w = writer.lock().unwrap();
                let result = w.rotate_expired(now_ns);
                // Drain UNCONDITIONALLY, even when the rotation errored: it is
                // best-effort per stream, so it may have already sealed earlier
                // streams and queued their `Closed` events — those must still
                // reach the ledger. Send under the lock (ordering, as in export).
                let events = w.take_all_events();
                let forwarded = events.len();
                if forwarded > 0 {
                    ledger_sender.send_events(tenant_id.clone(), events);
                }
                (result, forwarded)
            };
            match result {
                Ok(n) if n > 0 => tracing::debug!(
                    tenant = %tenant_id,
                    rotated = n,
                    "idle {wal} rotation: sealed expired stream(s)"
                ),
                Ok(_) => {}
                Err(e) => tracing::error!(
                    %e,
                    tenant = %tenant_id,
                    forwarded_events = forwarded,
                    "idle {wal} rotation failed; forwarded already-sealed streams' events"
                ),
            }
        }
    }

    /// The tenant's writer, if a request has created it.
    #[cfg(test)]
    pub(crate) fn existing(&self, tenant_id: &TenantId) -> Option<Arc<Mutex<wal::Writer>>> {
        self.writers.lock().unwrap().get(tenant_id).map(Arc::clone)
    }

    /// Whether no tenant has a writer yet.
    #[cfg(test)]
    pub(crate) fn is_empty(&self) -> bool {
        self.writers.lock().unwrap().is_empty()
    }
}
