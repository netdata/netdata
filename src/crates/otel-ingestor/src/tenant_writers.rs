//! The WAL writers of one signal, one per tenant. The logs and traces
//! services each own a [`TenantWriters`]: an export asks it for the tenant's
//! writer, writes its frames, and waits until they are durable; the periodic
//! idle-rotation sweep asks it to rotate what has expired.
//!
//! The writers' lifecycle events (a file was created, synced, closed) go to
//! the ledger from here and nowhere else, and always while the writer's lock
//! is still held: drained and sent under one lock hold, a file's
//! Created → Synced → Closed reach the single ledger channel in order.
//! Sending after releasing the lock would let another sync round or the
//! sweep on the same tenant slip its events in between. `send_events` only
//! pushes to an unbounded channel, so nothing waits under the lock.
use std::collections::HashMap;
use std::sync::{Arc, Mutex, MutexGuard};

use file_registry::{Identity, TenantId, TimestampNs};
use otel_bridge::signals::Signal;

use crate::ledger_sender::LedgerSender;

/// One tenant's WAL writer, for one signal.
///
/// An export writes under the writer's lock and syncs outside it
/// ([`Self::make_durable`]), so the tenant's other requests keep writing
/// while an fsync waits on the disk.
pub(crate) struct TenantWriter {
    tenant_id: TenantId,
    writer: Mutex<wal::Writer>,
    /// One sync round at a time. A round covers every frame written before
    /// it began, so the requests that queue up here during a round share the
    /// next one: the first of them runs it, the others find their frames
    /// durable already.
    sync_turn: tokio::sync::Mutex<()>,
    ledger_sender: Arc<LedgerSender>,
}

impl TenantWriter {
    /// Lock the writer, to write frames. Never held across an `.await`.
    pub(crate) fn lock(&self) -> MutexGuard<'_, wal::Writer> {
        self.writer.lock().unwrap()
    }

    /// Return once every frame of `mark` is durable and the ledger has been
    /// told. A request calls it after its writes, with the writer's lock
    /// released.
    ///
    /// Lock order: the sync turn, then the writer. The fsyncs run with the
    /// turn held and the writer free, all of the round's files at once.
    pub(crate) async fn make_durable(&self, mark: wal::WriteMark) -> wal::Result<()> {
        let _turn = self.sync_turn.lock().await;

        let ticket = {
            let mut writer = self.lock();

            if writer.is_durable(mark) {
                return Ok(());
            }

            writer.begin_sync()
        };

        let ticket = fsync_overlapped(ticket).await;

        let mut writer = self.lock();
        let result = writer.finish_sync(ticket);

        // Also after a failed round: the files that did sync are reported.
        self.send_events(&mut writer);

        result
    }

    /// Take the sync turn, so that no round runs until the guard is dropped.
    #[cfg(test)]
    pub(crate) async fn hold_sync_turn(&self) -> tokio::sync::MutexGuard<'_, ()> {
        self.sync_turn.lock().await
    }

    /// Drain the writer's lifecycle events and send them to the ledger,
    /// under the writer's lock (see the module docs). Returns how many.
    fn send_events(&self, writer: &mut wal::Writer) -> usize {
        let events = writer.take_all_events();
        let sent = events.len();

        if sent > 0 {
            self.ledger_sender
                .send_events(self.tenant_id.clone(), events);
        }

        sent
    }
}

/// fsync the ticket's files, each on the blocking pool (an fsync waits on the
/// disk), all at once. A tenant has one file per stream, and on a journaling
/// filesystem fsyncs that overlap share a journal commit, where fsyncs in
/// turn wait for one each.
async fn fsync_overlapped(mut ticket: wal::SyncTicket) -> wal::SyncTicket {
    let files = ticket.take_files();
    let mut fsyncs = Vec::with_capacity(files.len());

    for file in files {
        fsyncs.push(tokio::task::spawn_blocking(move || file.fsync()));
    }
    for fsync in fsyncs {
        match fsync.await {
            Ok(file) => ticket.give_back(file),
            // The file stays out of the ticket, which fails the round.
            Err(e) => tracing::error!(%e, "a WAL fsync task failed"),
        }
    }

    ticket
}

/// One WAL writer per tenant, for one signal.
///
/// Two locks, never held together by this type: the map's, held only to look
/// a writer up, create it, or snapshot the handles; and each writer's own,
/// which an export holds for its writes. So tenants ingest in parallel, and
/// same-tenant requests overlap everything but their writes.
pub(crate) struct TenantWriters {
    writers: Mutex<HashMap<TenantId, Arc<TenantWriter>>>,
    signal: Signal,
    /// Stamped into every WAL file: the signal's pipeline id and the format
    /// of its frame payloads.
    file_stamp: wal::FileStamp,
    /// Each tenant's WAL files live in `config.dir/<tenant>`.
    config: otel_bridge::config::WalConfig,
    /// The global seq allocator (lib.rs hands the same one to the logs and
    /// traces services), so file `seq` is unique across signals.
    seq_allocator: Arc<wal::SeqAllocator>,
    /// Identity stamped into every WAL FileId (the machine GUID resolved by the
    /// supervisor plus its self-generated per-process instance id; see
    /// `otel_bridge::config::PluginConfig::identity`).
    instance_identity: Identity,
    /// Shared with the other signal's writers: the writer → ledger IPC
    /// accepts exactly one connection (the ledger gap-checks frame sequences
    /// per signal), so every signal's events MUST funnel through one sender.
    ledger_sender: Arc<LedgerSender>,
}

impl TenantWriters {
    pub(crate) fn new(
        signal: Signal,
        payload_format: u16,
        config: otel_bridge::config::WalConfig,
        seq_allocator: Arc<wal::SeqAllocator>,
        instance_identity: Identity,
        ledger_sender: Arc<LedgerSender>,
    ) -> Self {
        Self {
            writers: Mutex::new(HashMap::new()),
            signal,
            file_stamp: wal::FileStamp {
                pipeline_id: signal.pipeline_id(),
                payload_format,
            },
            config,
            seq_allocator,
            instance_identity,
            ledger_sender,
        }
    }

    /// The tenant's writer; its first request creates it, under the map's
    /// lock.
    pub(crate) fn writer_for(&self, tenant_id: &TenantId) -> wal::Result<Arc<TenantWriter>> {
        let mut writers = self.writers.lock().unwrap();

        if let Some(writer) = writers.get(tenant_id) {
            return Ok(Arc::clone(writer));
        }

        let path = self.config.dir.join(tenant_id.as_str());
        let writer = wal::Writer::new(
            &path,
            self.resolve_config(tenant_id.as_str()),
            Arc::clone(&self.seq_allocator),
            self.file_stamp,
            self.instance_identity,
        )?;
        let writer = Arc::new(TenantWriter {
            tenant_id: tenant_id.clone(),
            writer: Mutex::new(writer),
            sync_turn: tokio::sync::Mutex::new(()),
            ledger_sender: Arc::clone(&self.ledger_sender),
        });
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
    /// - snapshot the writer handles under the map lock (held only for the
    ///   clone), so exports and new-tenant creation are not blocked by the sweep;
    /// - per writer: lock it, rotate, then drain AND send UNDER that lock (see
    ///   the module docs). The sweep does not take the sync turn: a round's
    ///   `finish_sync` skips a file the sweep sealed meanwhile.
    pub(crate) fn rotate_expired(&self, now_ns: TimestampNs) {
        // The traces messages name their signal; the logs ones never did.
        let wal_label = match self.signal {
            Signal::Logs => "WAL",
            Signal::Traces => "traces WAL",
        };

        let writers: Vec<Arc<TenantWriter>> = {
            let guard = self.writers.lock().unwrap();
            guard.values().map(Arc::clone).collect()
        };

        for tenant_writer in writers {
            let (result, forwarded) = {
                let mut writer = tenant_writer.lock();
                let result = writer.rotate_expired(now_ns);
                // Drain UNCONDITIONALLY, even when the rotation errored: it is
                // best-effort per stream, so it may have already sealed earlier
                // streams and queued their `Closed` events — those must still
                // reach the ledger.
                let forwarded = tenant_writer.send_events(&mut writer);

                (result, forwarded)
            };
            match result {
                Ok(n) if n > 0 => tracing::debug!(
                    tenant = %tenant_writer.tenant_id,
                    rotated = n,
                    "idle {wal_label} rotation: sealed expired stream(s)"
                ),
                Ok(_) => {}
                Err(e) => tracing::error!(
                    %e,
                    tenant = %tenant_writer.tenant_id,
                    forwarded_events = forwarded,
                    "idle {wal_label} rotation failed; forwarded already-sealed streams' events"
                ),
            }
        }
    }

    /// The tenant's writer, if a request has created it.
    #[cfg(test)]
    pub(crate) fn existing(&self, tenant_id: &TenantId) -> Option<Arc<TenantWriter>> {
        self.writers.lock().unwrap().get(tenant_id).map(Arc::clone)
    }

    /// Whether no tenant has a writer yet.
    #[cfg(test)]
    pub(crate) fn is_empty(&self) -> bool {
        self.writers.lock().unwrap().is_empty()
    }
}

#[cfg(test)]
mod tests {
    use std::time::Duration;

    use file_registry::{InstanceId, MachineId, MonotonicClock};

    use super::*;

    /// A socket nobody listens on: events sent to it only queue.
    fn dead_socket() -> String {
        format!(
            "/tmp/netdata-tenant-writers-test-{}.sock",
            std::process::id()
        )
    }

    /// Logs writers under `base_dir`, sending their events to `socket`: a
    /// one-hour default rotation, and a zero-duration override for the
    /// tenant `short`.
    fn tenant_writers(base_dir: &std::path::Path, socket: &str) -> TenantWriters {
        let mut rotation = HashMap::new();
        rotation.insert(
            "default".to_string(),
            otel_bridge::config::RotationEntry {
                max_file_size: Some(bytesize::ByteSize::mb(64)),
                max_entries: Some(100_000),
                max_file_duration: Some(Duration::from_secs(3600)),
            },
        );
        rotation.insert(
            "short".to_string(),
            otel_bridge::config::RotationEntry {
                max_file_size: None,
                max_entries: None,
                max_file_duration: Some(Duration::ZERO),
            },
        );
        let config = otel_bridge::config::WalConfig {
            dir: base_dir.to_path_buf(),
            crc_enabled: true,
            compression_enabled: true,
            rotation: otel_bridge::config::RotationPolicy::try_from(rotation).unwrap(),
        };

        TenantWriters::new(
            Signal::Logs,
            ng_flatten::LOG_FRAME_PAYLOAD_FORMAT,
            config,
            Arc::new(wal::SeqAllocator::ephemeral(0)),
            Identity::new(
                MachineId::new(uuid::Uuid::from_u128(1)).unwrap(),
                InstanceId::new(uuid::Uuid::from_u128(2)).unwrap(),
            ),
            Arc::new(LedgerSender::new(socket)),
        )
    }

    #[tokio::test]
    async fn writer_for_creates_one_writer_per_tenant() {
        let tmp = tempfile::tempdir().unwrap();
        let writers = tenant_writers(tmp.path(), &dead_socket());

        let first = writers.writer_for(&TenantId::from("a")).unwrap();
        let again = writers.writer_for(&TenantId::from("a")).unwrap();
        let other = writers.writer_for(&TenantId::from("b")).unwrap();

        assert!(Arc::ptr_eq(&first, &again));
        assert!(!Arc::ptr_eq(&first, &other));
        let mut dirs: Vec<String> = std::fs::read_dir(tmp.path())
            .unwrap()
            .map(|entry| entry.unwrap().file_name().into_string().unwrap())
            .collect();
        dirs.sort();
        assert_eq!(dirs, ["a", "b"]);
    }

    #[tokio::test]
    async fn the_sweep_seals_only_expired_streams() {
        let tmp = tempfile::tempdir().unwrap();
        let writers = tenant_writers(tmp.path(), &dead_socket());
        let mut clock = MonotonicClock::new();
        let short = TenantId::from("short");
        let hourly = TenantId::from("hourly");

        for tenant in [&short, &hourly] {
            let writer = writers.writer_for(tenant).unwrap();
            let mut writer = writer.lock();
            let meta = wal::FrameMeta {
                entry_count: 1,
                ingestion_ns: clock.now_ns(),
                log_ts_range: None,
            };
            writer.write_frame(1, &[], b"frame", meta).unwrap();
            writer.sync_all().unwrap();
            writer.take_all_events();
        }

        writers.rotate_expired(clock.now_ns());

        // A stream the sweep sealed has nothing left for shutdown to close.
        let closed_at_shutdown = |tenant: &TenantId| {
            let writer = writers.existing(tenant).unwrap();
            let events = writer.lock().shutdown_all().unwrap();
            events
                .iter()
                .filter(|e| matches!(e, wal::FileEvent::Closed { .. }))
                .count()
        };
        assert_eq!(closed_at_shutdown(&short), 0);
        assert_eq!(closed_at_shutdown(&hourly), 1);
    }

    #[tokio::test]
    async fn a_round_syncs_every_file_of_the_tenant() {
        let tmp = tempfile::tempdir().unwrap();
        let writers = tenant_writers(tmp.path(), &dead_socket());
        let mut clock = MonotonicClock::new();
        let writer = writers.writer_for(&TenantId::from("a")).unwrap();

        // Three streams, so three files.
        let mark = {
            let mut writer = writer.lock();

            for partition_key in 1..=3 {
                let meta = wal::FrameMeta {
                    entry_count: 1,
                    ingestion_ns: clock.now_ns(),
                    log_ts_range: None,
                };
                writer
                    .write_frame(partition_key, &[], b"frame", meta)
                    .unwrap();
            }
            writer.take_all_events();

            writer.write_mark()
        };

        // Had the round lost one of the files, it would have failed.
        writer.make_durable(mark).await.unwrap();
        assert!(writer.lock().is_durable(mark));
    }

    #[tokio::test]
    async fn one_round_covers_every_request_that_wrote_before_it() {
        use ferryboat::{Endpoint, Listener};

        let tmp = tempfile::tempdir().unwrap();
        let socket = format!(
            "/tmp/netdata-tenant-writers-round-test-{}.sock",
            std::process::id()
        );
        let _ = std::fs::remove_file(&socket);
        // Ledger side: capture every event the writers send.
        let mut listener = Listener::<(), wal::Message>::bind(Endpoint::ipc(&socket))
            .open()
            .expect("bind ledger listener");
        let writers = tenant_writers(tmp.path(), &socket);
        let mut conn = listener.accept().await.expect("accept writer connection");
        let mut clock = MonotonicClock::new();
        let writer = writers.writer_for(&TenantId::from("a")).unwrap();

        // Two requests write their frame before either one syncs.
        let mut marks = Vec::new();
        for _ in 0..2 {
            let mut writer = writer.lock();
            let meta = wal::FrameMeta {
                entry_count: 1,
                ingestion_ns: clock.now_ns(),
                log_ts_range: None,
            };
            writer.write_frame(1, &[], b"frame", meta).unwrap();
            marks.push(writer.write_mark());
        }

        let (first, second) =
            tokio::join!(writer.make_durable(marks[0]), writer.make_durable(marks[1]));
        first.unwrap();
        second.unwrap();
        assert!(writer.lock().is_durable(marks[1]));

        // The one round that ran told the ledger about the file. The sweep,
        // an hour later, seals it: nothing but its `Closed` was still to send.
        let mut next_kind = async || {
            let message = tokio::time::timeout(Duration::from_secs(5), conn.recv())
                .await
                .expect("the event must reach the ledger")
                .expect("recv wal message");
            match message.event {
                wal::FileEvent::Created { .. } => "created",
                wal::FileEvent::Synced { .. } => "synced",
                wal::FileEvent::Closed { .. } => "closed",
            }
        };
        assert_eq!(
            [next_kind().await, next_kind().await],
            ["created", "synced"]
        );

        let an_hour_ns = 3600 * 1_000_000_000;
        writers.rotate_expired(TimestampNs(clock.now_ns().0 + an_hour_ns));
        assert_eq!(next_kind().await, "closed");
    }
}
