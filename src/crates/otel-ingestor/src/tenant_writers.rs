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

/// The answer to a request whose records could not be stored: the WAL
/// directory could not be made, or a write or an fsync failed. The storage
/// may work again (a full disk gets room), so the status is `UNAVAILABLE`
/// (HTTP 503), which the OTLP specification lists as retryable: the
/// OpenTelemetry Collector keeps the batch and sends it again. `INTERNAL`
/// (HTTP 500) is not retryable, and the sender would drop the batch.
///
/// A request that failed can have its records in the files, in part (the
/// frames of its other streams) or in full (frames that were whole but in
/// doubt when another write or fsync of the tenant's writer failed), and
/// the indexer indexes whole frames. So a resend can store records twice:
/// OTLP delivery is at-least-once.
pub(crate) fn storage_unavailable(message: &'static str) -> tonic::Status {
    tonic::Status::unavailable(message)
}

/// The answer of an export whose task (the one that syncs the request's
/// frames and counts its records) ended without a result. `what` names the
/// signal ("logs", "traces").
///
/// The task is cancelled when the runtime is dropped while the request
/// waits for its sync. At a stop that happens only to an export that is
/// still in progress when its part of the worker's stop budget ends: the
/// worker first lets the exports in progress finish (`run_ingestor`). It is
/// a normal end,
/// not an error. The frames are in their WAL file, so the next start indexes
/// them, and a sender that sends the records again stores them twice. The
/// sender's connection usually ends with no answer at that moment, so few
/// senders see this status. This is not a storage error, so the records are
/// not counted as `failed`. A task that panicked is a defect of this plugin.
pub(crate) fn export_task_ended(
    error: tokio::task::JoinError,
    what: &'static str,
) -> tonic::Status {
    if error.is_cancelled() {
        tracing::info!(
            "the plugin stopped before a {what} export was synced: it is not acknowledged"
        );
        return tonic::Status::unavailable("the plugin is stopping");
    }

    tracing::error!(%error, "the task that syncs and counts a {what} export failed");
    tonic::Status::internal("export task failed")
}

/// Refuse the frame of one stream of an export request when it is over
/// [`wal::MAX_FRAME_DATA`]: the indexer could never read it, nor the rest of
/// its WAL file. `what` names the records ("records", "spans").
///
/// A frame keeps some bytes for each record, whatever the record holds, so
/// a request under [`MAX_REQUEST_BYTES`](crate::MAX_REQUEST_BYTES) of very
/// many records that are almost empty makes such a frame. The answer is
/// gRPC `INVALID_ARGUMENT` (HTTP 400), which is not retryable: the same
/// request would be refused again. The services check every frame of a
/// request before they write any, so the request is refused whole.
pub(crate) fn check_frame_size(
    records: usize,
    what: &'static str,
    frame_bytes: usize,
) -> Result<(), tonic::Status> {
    if frame_bytes <= wal::MAX_FRAME_DATA {
        return Ok(());
    }

    tracing::warn!(
        records,
        frame_bytes,
        limit = wal::MAX_FRAME_DATA,
        "refused an export request: the {what} of one stream are too many for one WAL frame",
    );
    Err(tonic::Status::invalid_argument(format!(
        "{records} {what} of one stream need a frame of {frame_bytes} bytes, and the limit is {} \
         bytes: send fewer {what} in one request",
        wal::MAX_FRAME_DATA,
    )))
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
    ///
    /// # Errors
    ///
    /// The round's first failure. Also `wal::Error::FramesLost`, when a
    /// write or an fsync of this writer failed while the frames were not
    /// durable yet (an earlier round's, a rotation's, the sweep's): no later
    /// sync can vouch for them. The request then fails with
    /// [`storage_unavailable`], and the sender sends the records again.
    pub(crate) async fn make_durable(&self, mark: wal::WriteMark) -> wal::Result<()> {
        let _turn = self.sync_turn.lock().await;

        let ticket = {
            let mut writer = self.lock();

            if writer.is_durable(mark)? {
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
    /// accepts exactly one connection, so every signal's events MUST funnel
    /// through one sender.
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

    #[tokio::test]
    async fn an_export_task_that_a_stop_cancelled_is_retryable_and_one_that_panicked_is_not() {
        let cancelled = tokio::spawn(std::future::pending::<()>());
        cancelled.abort();
        let status = export_task_ended(cancelled.await.unwrap_err(), "logs");
        assert_eq!(status.code(), tonic::Code::Unavailable);

        let panicked = tokio::spawn(async { panic!("a defect") });
        let status = export_task_ended(panicked.await.unwrap_err(), "logs");
        assert_eq!(status.code(), tonic::Code::Internal);
    }

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
            Arc::new(LedgerSender::new(
                socket,
                Arc::default(),
                tokio_util::sync::CancellationToken::new(),
            )),
        )
    }

    /// The queue to the ledger has no bound; its length is counted for the
    /// chart `netdata.otel.wal_events_queued`.
    #[tokio::test]
    async fn the_events_that_wait_for_the_ledger_are_counted() {
        use std::sync::atomic::{AtomicU64, Ordering};

        let socket = format!(
            "/tmp/netdata-tenant-writers-queue-test-{}.sock",
            std::process::id()
        );
        let _ = std::fs::remove_file(&socket);
        let queued = Arc::new(AtomicU64::new(0));
        let sender = LedgerSender::new(
            &socket,
            Arc::clone(&queued),
            tokio_util::sync::CancellationToken::new(),
        );
        let file_id = file_registry::FileId::new(
            Identity::new(
                MachineId::new(uuid::Uuid::from_u128(1)).unwrap(),
                InstanceId::new(uuid::Uuid::from_u128(2)).unwrap(),
            ),
            0,
            1,
            0,
        );
        let created = || wal::FileEvent::Created {
            file_id,
            created_at_ns: TimestampNs(1),
            content_meta: Vec::new(),
        };

        // Nobody listens yet: the events wait.
        sender.send_events(TenantId::from("a"), vec![created(), created(), created()]);
        assert_eq!(queued.load(Ordering::Relaxed), 3);

        // The ledger connects and takes them.
        let mut listener =
            ferryboat::Listener::<(), wal::Message>::bind(ferryboat::Endpoint::ipc(&socket))
                .open()
                .expect("bind ledger listener");
        let mut conn = listener.accept().await.expect("accept writer connection");
        for _ in 0..3 {
            tokio::time::timeout(Duration::from_secs(5), conn.recv())
                .await
                .expect("the event must reach the ledger")
                .expect("recv wal message");
        }
        assert_eq!(queued.load(Ordering::Relaxed), 0);
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
        assert!(writer.lock().is_durable(mark).unwrap());
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
        assert!(writer.lock().is_durable(marks[1]).unwrap());

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

    /// The link's contract: the ledger gets the events of one file in the
    /// file's order (`Created`, any number of `Synced`, `Closed`), whatever
    /// other tenants and the sweep do at the same time. Nothing orders the
    /// events of different files.
    #[tokio::test(flavor = "multi_thread", worker_threads = 4)]
    async fn the_ledger_gets_the_events_of_each_file_in_the_order_of_the_file() {
        use ferryboat::{Endpoint, Listener};
        use wal::FileEvent::{Closed, Created, Synced};

        const TENANTS: [&str; 4] = ["a", "b", "c", "d"];
        let tmp = tempfile::tempdir().unwrap();
        let socket = format!(
            "/tmp/netdata-tenant-writers-order-test-{}.sock",
            std::process::id()
        );
        let _ = std::fs::remove_file(&socket);
        // Ledger side: capture every event the writers send.
        let mut listener = Listener::<(), wal::Message>::bind(Endpoint::ipc(&socket))
            .open()
            .expect("bind ledger listener");
        let writers = Arc::new(tenant_writers(tmp.path(), &socket));
        let mut conn = listener.accept().await.expect("accept writer connection");
        // Past the one-hour rotation of every file written below.
        let two_hours_ns = 2 * 3600 * 1_000_000_000;
        let later = TimestampNs(MonotonicClock::new().now_ns().0 + two_hours_ns);

        // The tenants write and sync at the same time, while the sweep seals
        // their files under them.
        let mut tenants = Vec::new();
        for tenant in TENANTS {
            let writers = Arc::clone(&writers);
            tenants.push(tokio::spawn(async move {
                let mut clock = MonotonicClock::new();
                let writer = writers.writer_for(&TenantId::from(tenant)).unwrap();
                for _ in 0..200 {
                    let mark = {
                        let mut writer = writer.lock();
                        let meta = wal::FrameMeta {
                            entry_count: 1,
                            ingestion_ns: clock.now_ns(),
                            log_ts_range: None,
                        };
                        writer.write_frame(1, &[], b"frame", meta).unwrap();
                        writer.write_mark()
                    };
                    writer.make_durable(mark).await.unwrap();
                }
            }));
        }
        let sweeper = {
            let writers = Arc::clone(&writers);
            tokio::task::spawn_blocking(move || {
                for _ in 0..50 {
                    writers.rotate_expired(later);
                    std::thread::sleep(Duration::from_millis(1));
                }
            })
        };
        for tenant in tenants {
            tenant.await.unwrap();
        }
        sweeper.await.unwrap();
        // Seal what is still open, so that every file has all its events.
        writers.rotate_expired(later);

        let mut files_on_disk = 0;
        for tenant in TENANTS {
            for entry in std::fs::read_dir(tmp.path().join(tenant)).unwrap() {
                if entry.unwrap().path().extension() == Some("wal".as_ref()) {
                    files_on_disk += 1;
                }
            }
        }

        // With the writers gone the sender ends and closes the link: read
        // every event that was sent, to the end.
        drop(writers);
        let mut events_of_file: HashMap<u64, Vec<wal::FileEvent>> = HashMap::new();
        let mut closed_files = 0;
        loop {
            let received = tokio::time::timeout(Duration::from_secs(10), conn.recv())
                .await
                .expect("the link must close when the writers are gone");
            let Ok(message) = received else {
                break;
            };
            let seq = match &message.event {
                Created { file_id, .. } | Synced { file_id, .. } | Closed { file_id, .. } => {
                    file_id.seq
                }
            };
            if matches!(message.event, Closed { .. }) {
                closed_files += 1;
            }
            events_of_file.entry(seq).or_default().push(message.event);
        }

        assert_eq!(closed_files, files_on_disk);
        assert_eq!(events_of_file.len(), files_on_disk);
        assert!(files_on_disk > TENANTS.len(), "the sweep sealed no file");
        for (seq, events) in &events_of_file {
            assert!(
                matches!(events.first(), Some(Created { .. })),
                "file {seq} does not start with `Created`: {events:?}"
            );
            assert!(
                matches!(events.last(), Some(Closed { .. })),
                "file {seq} does not end with `Closed`: {events:?}"
            );
            // Between them only `Synced`, and the durable prefix never
            // shrinks.
            let mut durable = file_registry::ByteSize::ZERO;
            for (at, event) in events.iter().enumerate().skip(1) {
                let valid_up_to = match event {
                    Synced { valid_up_to, .. } => *valid_up_to,
                    Closed { valid_up_to, .. } if at == events.len() - 1 => *valid_up_to,
                    _ => panic!("file {seq} has {event:?} at {at}: {events:?}"),
                };
                assert!(valid_up_to >= durable, "file {seq}: {events:?}");
                durable = valid_up_to;
            }
        }
    }
}
