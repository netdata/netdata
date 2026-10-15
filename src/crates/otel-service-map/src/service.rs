//! The running component: a bounded hand-off from the ingest path, one task
//! that owns pairing and expiry, a once-a-minute flush to disk, and window
//! queries.
//!
//! Ingestion never waits on the service map: `prepare` only reads the request
//! and `commit` drops (and counts) a batch when the queue is full.

use std::collections::HashMap;
use std::io;
use std::path::PathBuf;
use std::sync::{Arc, Mutex, RwLock};
use std::time::{Duration, SystemTime, UNIX_EPOCH};

use opentelemetry_proto::tonic::collector::trace::v1::ExportTraceServiceRequest;
use tokio::sync::{mpsc, oneshot};
use tokio::task::JoinHandle;

use crate::engine::{Engine, Limits};
use crate::facts::{Extracted, extract};
use crate::stats::NS_PER_MINUTE;
use crate::store::{Retention, Store};
use crate::view::{self, MapView};

#[derive(Clone, Debug)]
pub struct Config {
    /// Directory owned by the service map.
    pub root: PathBuf,
    pub limits: Limits,
    pub retention: Retention,
    /// Export requests waiting for the engine task.
    pub queue_capacity: usize,
}

impl Config {
    pub fn new(root: impl Into<PathBuf>) -> Self {
        Self {
            root: root.into(),
            limits: Limits::default(),
            retention: Retention::default(),
            queue_capacity: 1_024,
        }
    }
}

/// The facts of one export request, read on the request's task and handed
/// over once the request is stored.
pub struct Prepared {
    tenant: Arc<str>,
    extracted: Extracted,
}

struct Shared {
    engine: Mutex<Engine>,
    /// Held exclusively from handing out closed minutes until they are on
    /// disk, and shared by queries, so a query never sees a minute both open
    /// and stored, or in neither place.
    flush_gate: RwLock<()>,
    store: Store,
    handoff_drops: Mutex<HashMap<Arc<str>, u64>>,
}

pub struct ServiceMap {
    shared: Arc<Shared>,
    tx: mpsc::Sender<Prepared>,
    stop: Mutex<Option<oneshot::Sender<()>>>,
    task: Mutex<Option<JoinHandle<()>>>,
}

const TICK: Duration = Duration::from_secs(1);
const RETENTION_EVERY_MINUTES: u64 = 10;

impl ServiceMap {
    /// Opens the store and starts the engine task on the current runtime.
    pub fn start(config: Config) -> io::Result<Self> {
        let store = Store::open(&config.root, config.retention.clone())?;
        let shared = Arc::new(Shared {
            engine: Mutex::new(Engine::new(config.limits.clone())),
            flush_gate: RwLock::new(()),
            store,
            handoff_drops: Mutex::new(HashMap::new()),
        });
        let (tx, rx) = mpsc::channel(config.queue_capacity.max(1));
        let (stop_tx, stop_rx) = oneshot::channel();
        let task = tokio::spawn(run(shared.clone(), rx, stop_rx));
        Ok(Self {
            shared,
            tx,
            stop: Mutex::new(Some(stop_tx)),
            task: Mutex::new(Some(task)),
        })
    }

    pub fn prepare(&self, tenant: &str, request: &ExportTraceServiceRequest) -> Prepared {
        Prepared {
            tenant: Arc::from(tenant),
            extracted: extract(request),
        }
    }

    pub fn commit(&self, prepared: Prepared) {
        if prepared.extracted.spans.is_empty() && prepared.extracted.invalid == 0 {
            return;
        }
        if let Err(mpsc::error::TrySendError::Full(p) | mpsc::error::TrySendError::Closed(p)) =
            self.tx.try_send(prepared)
        {
            let mut drops = self.shared.handoff_drops.lock().expect("drops lock");
            *drops.entry(p.tenant).or_insert(0) += 1;
        }
    }

    /// The map of `tenant` for `[after_s, before_s)` (unix seconds). An error
    /// means the engine is broken (a panic poisoned its state), not that the
    /// window is empty.
    pub async fn view(&self, tenant: &str, after_s: u64, before_s: u64) -> Result<MapView, String> {
        let Some((from, to)) = view::window_minutes(after_s, before_s) else {
            return Ok(MapView::default());
        };
        let shared = self.shared.clone();
        let tenant = tenant.to_string();
        tokio::task::spawn_blocking(move || {
            let _gate = shared
                .flush_gate
                .read()
                .map_err(|_| "the service map's flush failed".to_string())?;
            let open = shared
                .engine
                .lock()
                .map_err(|_| "the service map's engine failed".to_string())?
                .open_minutes(&tenant, from, to);
            let stored = shared.store.load(&tenant, from, to);
            Ok(view::build(stored, open))
        })
        .await
        .map_err(|e| format!("the service map query failed: {e}"))?
    }

    /// Stops the engine task after it wrote every open minute.
    pub async fn shutdown(&self) {
        if let Some(stop) = self.stop.lock().expect("stop lock").take() {
            let _ = stop.send(());
        }
        let task = self.task.lock().expect("task lock").take();
        if let Some(task) = task {
            let _ = task.await;
        }
    }
}

fn now_ns() -> u64 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map_or(0, |d| d.as_nanos() as u64)
}

async fn run(
    shared: Arc<Shared>,
    mut rx: mpsc::Receiver<Prepared>,
    mut stop: oneshot::Receiver<()>,
) {
    let mut tick = tokio::time::interval(TICK);
    tick.set_missed_tick_behavior(tokio::time::MissedTickBehavior::Delay);
    let mut last_flush_minute = now_ns() / NS_PER_MINUTE;
    let mut last_retention_minute = None;
    let mut flushing: Option<JoinHandle<()>> = None;
    loop {
        tokio::select! {
            prepared = rx.recv() => {
                let Some(p) = prepared else { break };
                let now = now_ns();
                let mut engine = shared.engine.lock().expect("engine lock");
                engine.ingest(&p.tenant, p.extracted.spans, p.extracted.invalid, now);
            }
            _ = tick.tick() => {
                let now = now_ns();
                let drops = std::mem::take(&mut *shared.handoff_drops.lock().expect("drops lock"));
                {
                    let mut engine = shared.engine.lock().expect("engine lock");
                    for (tenant, n) in drops {
                        engine.note_handoff_drops(&tenant, n, now);
                    }
                    engine.expire(now);
                }
                // The flush runs beside the loop, one at a time, so a slow
                // write or a long query holding the flush gate does not stop
                // pairing and expiry; a minute whose flush is still busy
                // waits for the next tick.
                let minute = now / NS_PER_MINUTE;
                let idle = flushing.as_ref().is_none_or(|h: &JoinHandle<()>| h.is_finished());
                if minute != last_flush_minute && idle {
                    last_flush_minute = minute;
                    let retention_due = last_retention_minute
                        .is_none_or(|last| minute >= last + RETENTION_EVERY_MINUTES);
                    if retention_due {
                        last_retention_minute = Some(minute);
                    }
                    let shared = shared.clone();
                    flushing = Some(tokio::task::spawn_blocking(move || flush(&shared, now, retention_due, false)));
                }
            }
            _ = &mut stop => break,
        }
    }
    // Spans already handed over are counted before the final write.
    while let Ok(p) = rx.try_recv() {
        let mut engine = shared.engine.lock().expect("engine lock");
        engine.ingest(&p.tenant, p.extracted.spans, p.extracted.invalid, now_ns());
    }
    if let Some(running) = flushing {
        let _ = running.await;
    }
    let shared = shared.clone();
    let _ = tokio::task::spawn_blocking(move || flush(&shared, now_ns(), false, true)).await;
}

fn flush(shared: &Shared, now: u64, retention: bool, shutdown: bool) {
    let _gate = shared.flush_gate.write().expect("flush gate");
    let closed = {
        let mut engine = shared.engine.lock().expect("engine lock");
        if shutdown {
            engine.shutdown(now)
        } else {
            engine.take_closed(now)
        }
    };
    if !closed.is_empty() {
        let failed = shared.store.write(closed);
        if !failed.is_empty() {
            let mut engine = shared.engine.lock().expect("engine lock");
            for (tenant, minutes) in failed {
                engine.note_unwritten_minutes(&tenant, minutes, now);
            }
        }
    }
    if retention {
        if let Err(e) = shared.store.enforce_retention(now / NS_PER_MINUTE) {
            tracing::warn!(%e, "service map: retention failed");
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::facts::tests::request;
    use crate::stats::Node;
    use opentelemetry_proto::tonic::trace::v1::Span;
    use opentelemetry_proto::tonic::trace::v1::span::SpanKind;

    fn root_server(id: u8, end_ns: u64) -> Span {
        Span {
            trace_id: vec![id; 16],
            span_id: vec![id; 8],
            kind: SpanKind::Server as i32,
            start_time_unix_nano: end_ns - 1_000_000,
            end_time_unix_nano: end_ns,
            ..Default::default()
        }
    }

    #[tokio::test]
    async fn committed_spans_are_visible_and_survive_a_restart() {
        let dir = tempfile::tempdir().unwrap();
        let now = now_ns();
        let now_s = now / 1_000_000_000;

        let map = ServiceMap::start(Config::new(dir.path())).unwrap();
        let req = request("frontend", vec![root_server(1, now - 1_000_000_000)]);
        let prepared = map.prepare("default", &req);
        map.commit(prepared);
        // Let the engine task take the batch.
        let mut seen = 0;
        for _ in 0..100 {
            let view = map.view("default", now_s - 600, now_s + 60).await.unwrap();
            seen = view.edges.values().map(|e| e.server.calls).sum::<u64>();
            if seen > 0 {
                break;
            }
            tokio::time::sleep(Duration::from_millis(10)).await;
        }
        assert_eq!(seen, 1);
        map.shutdown().await;

        let map = ServiceMap::start(Config::new(dir.path())).unwrap();
        let view = map.view("default", now_s - 600, now_s + 60).await.unwrap();
        let edge = view.edges.keys().next().unwrap();
        assert_eq!(edge.caller, Node::UnknownCaller);
        assert_eq!(view.edges.values().map(|e| e.server.calls).sum::<u64>(), 1);
        assert!(
            map.view("other", now_s - 600, now_s + 60)
                .await
                .unwrap()
                .edges
                .is_empty()
        );
        map.shutdown().await;
    }

    #[tokio::test]
    async fn a_broken_engine_is_an_error_not_an_empty_map() {
        let dir = tempfile::tempdir().unwrap();
        let map = ServiceMap::start(Config::new(dir.path())).unwrap();
        let shared = map.shared.clone();
        let _ = std::thread::spawn(move || {
            let _engine = shared.engine.lock().unwrap();
            panic!("poison the engine lock");
        })
        .join();
        assert!(map.view("default", 0, 600).await.is_err());
    }

    #[tokio::test]
    async fn a_full_queue_drops_and_counts() {
        let dir = tempfile::tempdir().unwrap();
        let mut config = Config::new(dir.path());
        config.queue_capacity = 1;
        let map = ServiceMap::start(config).unwrap();
        let now = now_ns();
        let req = request("frontend", vec![root_server(1, now)]);
        // The engine task cannot run between these: a current-thread runtime
        // only switches tasks at an await.
        for _ in 0..5 {
            let prepared = map.prepare("default", &req);
            map.commit(prepared);
        }
        let dropped = *map
            .shared
            .handoff_drops
            .lock()
            .unwrap()
            .get("default")
            .unwrap();
        assert_eq!(dropped, 4);
        map.shutdown().await;
    }
}
