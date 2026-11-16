use super::common::ServerTimings;
use super::dispatch::DispatchHandler;
use crate::protocol::{MAX_PAYLOAD_CAP, MAX_PAYLOAD_DEFAULT};

#[cfg(unix)]
pub(super) use crate::transport::posix::ServerConfig;

#[cfg(windows)]
pub(super) use crate::transport::windows::ServerConfig;

use std::sync::atomic::{AtomicBool, AtomicU32, Ordering};
use std::sync::{Arc, Condvar, Mutex};
use std::time::Duration;

/// Set once a managed server accepts connections. Clones share the state, so
/// a caller can keep one and wait while another thread runs the server.
#[derive(Clone, Default)]
pub struct ListeningSignal(Arc<(Mutex<bool>, Condvar)>);

impl ListeningSignal {
    /// Wait up to `timeout` for the server to listen. Returns whether it is
    /// listening; a server whose `run()` failed to bind never is.
    pub fn wait(&self, timeout: Duration) -> bool {
        let (lock, cond) = &*self.0;
        let guard = lock.lock().unwrap();
        let (guard, _) = cond
            .wait_timeout_while(guard, timeout, |listening| !*listening)
            .unwrap();
        *guard
    }

    /// Whether the server has started listening.
    pub fn is_set(&self) -> bool {
        *self.0.0.lock().unwrap()
    }

    pub(super) fn set(&self) {
        let (lock, cond) = &*self.0;
        *lock.lock().unwrap() = true;
        cond.notify_all();
    }
}

/// Test-only notifications from server internals, so tests wait on events
/// instead of sleeping.
#[cfg(test)]
#[derive(Clone, Default)]
pub(super) struct TestHooks {
    pub(super) session_exited: Option<std::sync::mpsc::Sender<()>>,
    pub(super) handshake_started: Option<std::sync::mpsc::Sender<()>>,
    /// A session's SHM region was created. SHM exists only on Linux and
    /// Windows.
    #[cfg_attr(
        not(any(target_os = "linux", target_os = "freebsd", windows)),
        allow(dead_code)
    )]
    pub(super) shm_created: Option<std::sync::mpsc::Sender<()>>,
    /// Makes SHM region creation fail, to exercise the baseline fallback.
    #[cfg_attr(
        not(any(target_os = "linux", target_os = "freebsd", windows)),
        allow(dead_code)
    )]
    pub(super) fail_shm_create: std::sync::Arc<AtomicBool>,
}

#[cfg(test)]
impl TestHooks {
    pub(super) fn notify(tx: &Option<std::sync::mpsc::Sender<()>>) {
        if let Some(tx) = tx {
            let _ = tx.send(());
        }
    }
}

/// L2 managed server. Typed request/response dispatcher.
///
/// Handles accept, spawns a thread per session (up to worker_count),
/// reads requests, dispatches to handler, sends responses.
pub struct ManagedServer {
    pub(super) run_dir: String,
    pub(super) service_name: String,
    pub(super) server_config: ServerConfig,
    pub(super) expected_method_code: u16,
    pub(super) handler: Option<DispatchHandler>,
    pub(super) running: Arc<AtomicBool>,
    /// Set by `stop()` and when `run()` exits, never cleared: `run()` after a
    /// stop returns at once, and pending handshakes abort on it.
    pub(super) stop_requested: Arc<AtomicBool>,
    pub(super) learned_request_payload_bytes: Arc<AtomicU32>,
    pub(super) learned_response_payload_bytes: Arc<AtomicU32>,
    pub(super) request_payload_growth_ceiling: u32,
    pub(super) response_payload_growth_ceiling: u32,
    pub(super) next_session_id: u64,
    pub(super) worker_count: usize,
    pub(super) timings: ServerTimings,
    pub(super) listening: ListeningSignal,
    #[cfg(test)]
    pub(super) test_hooks: TestHooks,
    #[cfg(windows)]
    pub(super) listener_waker:
        Arc<std::sync::Mutex<Option<crate::transport::windows::NpListenerWaker>>>,
}

impl ManagedServer {
    /// Create a new managed server for a single service kind.
    pub fn new(
        run_dir: &str,
        service_name: &str,
        config: ServerConfig,
        expected_method_code: u16,
        handler: Option<DispatchHandler>,
    ) -> Self {
        Self::with_workers(
            run_dir,
            service_name,
            config,
            expected_method_code,
            handler,
            8,
        )
    }

    /// Create a managed server with an explicit worker count.
    pub fn with_workers(
        run_dir: &str,
        service_name: &str,
        config: ServerConfig,
        expected_method_code: u16,
        handler: Option<DispatchHandler>,
        worker_count: usize,
    ) -> Self {
        let learned_request = if config.max_request_payload_bytes != 0 {
            config.max_request_payload_bytes
        } else {
            MAX_PAYLOAD_DEFAULT
        };
        let learned_response = if config.max_response_payload_bytes != 0 {
            config.max_response_payload_bytes
        } else {
            MAX_PAYLOAD_DEFAULT
        };
        let request_payload_growth_ceiling = if config.max_request_payload_bytes != 0 {
            config.max_request_payload_bytes
        } else {
            MAX_PAYLOAD_CAP
        };
        let response_payload_growth_ceiling = if config.max_response_payload_bytes != 0 {
            config.max_response_payload_bytes
        } else {
            MAX_PAYLOAD_CAP
        };

        ManagedServer {
            run_dir: run_dir.to_string(),
            service_name: service_name.to_string(),
            server_config: config,
            expected_method_code,
            handler,
            running: Arc::new(AtomicBool::new(false)),
            stop_requested: Arc::new(AtomicBool::new(false)),
            learned_request_payload_bytes: Arc::new(AtomicU32::new(learned_request)),
            learned_response_payload_bytes: Arc::new(AtomicU32::new(learned_response)),
            request_payload_growth_ceiling,
            response_payload_growth_ceiling,
            next_session_id: 1,
            worker_count: if worker_count < 1 { 1 } else { worker_count },
            timings: ServerTimings::default(),
            listening: ListeningSignal::default(),
            #[cfg(test)]
            test_hooks: TestHooks::default(),
            #[cfg(windows)]
            listener_waker: Arc::new(std::sync::Mutex::new(None)),
        }
    }

    /// Signal shutdown. On Windows, also wakes the accept loop blocked in
    /// ConnectNamedPipe; it never touches a handle the loop owns.
    pub fn stop(&self) {
        self.stop_handle().stop();
    }

    fn stop_handle(&self) -> StopHandle {
        StopHandle {
            stop_requested: self.stop_requested.clone(),
            running: self.running.clone(),
            #[cfg(windows)]
            listener_waker: self.listener_waker.clone(),
        }
    }

    /// Returns a signal that is set once `run()` accepts connections.
    pub fn listening_signal(&self) -> ListeningSignal {
        self.listening.clone()
    }

    /// Returns the internal running flag for diagnostics and test helpers.
    ///
    /// For reliable shutdown, call `stop()`. On Windows, flipping this flag
    /// alone does not wake a blocking listener accept.
    pub fn running_flag(&self) -> Arc<AtomicBool> {
        self.running.clone()
    }
}

/// What `stop()` needs, detached from the server so another thread can stop
/// it while `run()` owns it.
struct StopHandle {
    stop_requested: Arc<AtomicBool>,
    running: Arc<AtomicBool>,
    #[cfg(windows)]
    listener_waker: Arc<std::sync::Mutex<Option<crate::transport::windows::NpListenerWaker>>>,
}

impl StopHandle {
    fn stop(&self) {
        self.stop_requested.store(true, Ordering::Release);
        self.running.store(false, Ordering::Release);

        #[cfg(windows)]
        if let Some(waker) = self.listener_waker.lock().unwrap().as_ref() {
            waker.wake();
        }
    }
}

/// Server waits for tests: timeout paths run in milliseconds while staying
/// well above scheduling noise.
#[cfg(test)]
pub(crate) const TEST_SERVER_TIMINGS: ServerTimings = ServerTimings {
    poll_ms: 10,
    stop_grace: Duration::from_millis(200),
    handshake_ms: 500,
};

/// Bounds test waits that only expire when the code under test is broken;
/// reaching it fails the test.
#[cfg(test)]
pub(crate) const TEST_WAIT_LIMIT: Duration = Duration::from_secs(10);

/// Receivers for a test server's session events.
#[cfg(test)]
pub(crate) struct TestEvents {
    session_exited: std::sync::mpsc::Receiver<()>,
    handshake_started: std::sync::mpsc::Receiver<()>,
    shm_created: std::sync::mpsc::Receiver<()>,
    fail_shm_create: std::sync::Arc<AtomicBool>,
}

#[cfg(test)]
impl TestEvents {
    /// Wait until `n` session workers have exited and freed their slots.
    pub(crate) fn wait_session_exits(&self, n: usize) {
        for _ in 0..n {
            self.session_exited
                .recv_timeout(TEST_WAIT_LIMIT)
                .expect("session worker did not exit");
        }
    }

    /// Number of SHM regions created since the last call. A region exists
    /// before HELLO_ACK, so a READY client's region is already counted.
    #[cfg_attr(
        not(any(target_os = "linux", target_os = "freebsd", windows)),
        allow(dead_code)
    )]
    pub(crate) fn take_shm_created(&self) -> usize {
        self.shm_created.try_iter().count()
    }

    /// Make the server's SHM region creation fail (or succeed again).
    #[cfg_attr(
        not(any(target_os = "linux", target_os = "freebsd", windows)),
        allow(dead_code)
    )]
    pub(crate) fn set_fail_shm_create(&self, fail: bool) {
        self.fail_shm_create.store(fail, Ordering::Release);
    }

    /// Wait until `n` workers are waiting for HELLO.
    pub(crate) fn wait_handshakes_started(&self, n: usize) {
        for _ in 0..n {
            self.handshake_started
                .recv_timeout(TEST_WAIT_LIMIT)
                .expect("no handshake started");
        }
    }
}

#[cfg(test)]
impl ManagedServer {
    /// Switch to test timings and report session events.
    pub(crate) fn use_test_timings(&mut self) -> TestEvents {
        let (exit_tx, session_exited) = std::sync::mpsc::channel();
        let (hello_tx, handshake_started) = std::sync::mpsc::channel();
        let (shm_tx, shm_created) = std::sync::mpsc::channel();
        let fail_shm_create = std::sync::Arc::new(AtomicBool::new(false));
        self.timings = TEST_SERVER_TIMINGS;
        self.test_hooks = TestHooks {
            session_exited: Some(exit_tx),
            handshake_started: Some(hello_tx),
            shm_created: Some(shm_tx),
            fail_shm_create: fail_shm_create.clone(),
        };
        TestEvents {
            session_exited,
            handshake_started,
            shm_created,
            fail_shm_create,
        }
    }

    /// Run on a new thread with test timings; returns once the server
    /// accepts connections.
    pub(crate) fn spawn_for_test(mut self) -> RunningTestServer {
        let events = self.use_test_timings();
        let listening = self.listening_signal();
        let stopper = self.stop_handle();
        let thread = std::thread::spawn(move || self.run());
        if !listening.wait(TEST_WAIT_LIMIT) {
            panic!("test server is not listening");
        }
        RunningTestServer {
            stopper,
            thread: Some(thread),
            events,
        }
    }
}

/// A managed server running on its own thread; dropping it stops and joins
/// the server.
#[cfg(test)]
pub(crate) struct RunningTestServer {
    stopper: StopHandle,
    thread: Option<std::thread::JoinHandle<Result<(), crate::protocol::NipcError>>>,
    pub(crate) events: TestEvents,
}

#[cfg(test)]
impl RunningTestServer {
    /// Stop the server and wait for `run()` to return.
    pub(crate) fn stop(&mut self) {
        self.stopper.stop();
        if let Some(t) = self.thread.take() {
            let _ = t.join();
        }
    }
}

#[cfg(test)]
impl Drop for RunningTestServer {
    fn drop(&mut self) {
        self.stop();
    }
}
