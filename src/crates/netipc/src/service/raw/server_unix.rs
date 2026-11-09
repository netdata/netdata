use super::common::wait_for_worker_slot;
#[cfg(test)]
use super::server::TestHooks;
use super::server::{ManagedServer, ServerConfig};
use super::server_session_unix::{handle_session_threaded, poll_fd};
use crate::protocol::NipcError;
#[cfg(any(target_os = "linux", target_os = "freebsd"))]
use crate::protocol::{HEADER_SIZE, PROFILE_SHM_FUTEX, PROFILE_SHM_HYBRID};
use crate::transport::posix::{PendingConnection, UdsListener, UdsSession};
#[cfg(any(target_os = "linux", target_os = "freebsd"))]
use crate::transport::shm::ShmContext;
use std::os::unix::io::RawFd;
use std::sync::atomic::{AtomicBool, AtomicU32, Ordering};
use std::sync::{Arc, Mutex};
use std::time::{Duration, Instant};

/// Session socket of a running worker, published so a stopping server can
/// shut it down. The worker clears it before closing the socket, so a
/// shutdown never reaches a reused descriptor.
#[derive(Default)]
struct WorkerSocket(Mutex<Option<RawFd>>);

struct Worker {
    thread: std::thread::JoinHandle<()>,
    socket: Arc<WorkerSocket>,
}

impl ManagedServer {
    /// Run the acceptor loop. Blocking. Accepts clients and runs each
    /// connection, handshake included, in its own thread (up to
    /// worker_count); a connection over the limit is closed before
    /// HELLO_ACK.
    ///
    /// Returns when `stop()` is called or on fatal error. A stop before
    /// run makes run return at once; the server is single-use.
    #[cfg(unix)]
    pub fn run(&mut self) -> Result<(), NipcError> {
        // A stop before run must win; the server is single-use.
        if self.stop_requested.load(Ordering::Acquire) {
            return Ok(());
        }

        #[cfg(any(target_os = "linux", target_os = "freebsd"))]
        crate::transport::shm::cleanup_stale(&self.run_dir, &self.service_name);

        let mut listener = UdsListener::bind(
            &self.run_dir,
            &self.service_name,
            self.server_config.clone(),
        )
        .map_err(|_| NipcError::BadLayout)?;

        self.running.store(true, Ordering::Release);
        if self.stop_requested.load(Ordering::Acquire) {
            self.running.store(false, Ordering::Release);
        }
        self.listening.set();

        let mut workers: Vec<Worker> = Vec::new();

        while self.running.load(Ordering::Acquire) {
            let ready = poll_fd(listener.fd(), self.timings.poll_ms as i32);
            if ready < 0 {
                break;
            }
            if ready == 0 {
                workers.retain(|w| !w.thread.is_finished());
                continue;
            }

            // The handshake runs on the worker, so a client that never sends
            // HELLO cannot stall this loop.
            let pending = match listener.accept_pending() {
                Ok(p) => p,
                Err(_) => {
                    if !self.running.load(Ordering::Acquire) {
                        break;
                    }
                    std::thread::sleep(std::time::Duration::from_millis(10));
                    continue;
                }
            };

            // A connection holds its slot from accept, handshake included.
            // Refuse before HELLO_ACK so the client's connect fails instead of
            // a later call.
            let worker_count = self.worker_count;
            if !wait_for_worker_slot(&self.running, self.timings.slot_wait(), || {
                workers.retain(|w| !w.thread.is_finished());
                workers.len() < worker_count
            }) {
                drop(pending);
                continue;
            }

            let session_id = self.next_session_id;
            self.next_session_id += 1;
            let accept = AcceptContext {
                #[cfg(any(target_os = "linux", target_os = "freebsd"))]
                run_dir: self.run_dir.clone(),
                #[cfg(any(target_os = "linux", target_os = "freebsd"))]
                service_name: self.service_name.clone(),
                base_config: self.server_config.clone(),
                session_id,
                handshake_ms: self.timings.handshake_ms,
                #[cfg(all(test, any(target_os = "linux", target_os = "freebsd")))]
                hooks: self.test_hooks.clone(),
            };
            let socket = Arc::new(WorkerSocket::default());
            let worker_socket = socket.clone();
            let expected_method_code = self.expected_method_code;
            let handler = self.handler.clone();
            let running = self.running.clone();
            let poll_ms = self.timings.poll_ms;
            #[cfg(test)]
            let hooks = self.test_hooks.clone();
            let stop_requested = self.stop_requested.clone();
            let learned_request_payload_bytes = self.learned_request_payload_bytes.clone();
            let learned_response_payload_bytes = self.learned_response_payload_bytes.clone();
            let request_payload_growth_ceiling = self.request_payload_growth_ceiling;
            let response_payload_growth_ceiling = self.response_payload_growth_ceiling;

            let thread = std::thread::spawn(move || {
                #[cfg(test)]
                let _exit = SessionExitNotice(hooks.session_exited.clone());
                #[cfg(test)]
                TestHooks::notify(&hooks.handshake_started);
                let Some((mut session, shm)) = accept.handshake(
                    pending,
                    &stop_requested,
                    &learned_request_payload_bytes,
                    &learned_response_payload_bytes,
                ) else {
                    return;
                };
                *worker_socket.0.lock().unwrap() = Some(session.fd());

                handle_session_threaded(
                    &mut session,
                    shm,
                    expected_method_code,
                    handler,
                    running,
                    poll_ms,
                    learned_request_payload_bytes,
                    learned_response_payload_bytes,
                    request_payload_growth_ceiling,
                    response_payload_growth_ceiling,
                );

                let mut published = worker_socket.0.lock().unwrap();
                *published = None;
                drop(session);
            });
            workers.push(Worker { thread, socket });
        }

        self.stop_requested.store(true, Ordering::Release);
        self.running.store(false, Ordering::Release);
        // Close the socket now; the endpoint lock drops with the listener,
        // after the workers have exited.
        listener.stop_listening();
        stop_workers(workers, self.timings.stop_grace);
        drop(listener);

        Ok(())
    }
}

/// Give workers a grace to finish their current message, then shut down the
/// sockets of the ones still blocked in send/recv, and join them all.
fn stop_workers(workers: Vec<Worker>, grace: Duration) {
    let deadline = Instant::now() + grace;
    while Instant::now() < deadline && workers.iter().any(|w| !w.thread.is_finished()) {
        std::thread::sleep(Duration::from_millis(5));
    }

    for w in &workers {
        if w.thread.is_finished() {
            continue;
        }
        if let Some(fd) = *w.socket.0.lock().unwrap() {
            unsafe {
                libc::shutdown(fd, libc::SHUT_RDWR);
            }
        }
    }

    for w in workers {
        let _ = w.thread.join();
    }
}

/// What a worker needs to run the server side of one handshake.
struct AcceptContext {
    #[cfg(any(target_os = "linux", target_os = "freebsd"))]
    run_dir: String,
    #[cfg(any(target_os = "linux", target_os = "freebsd"))]
    service_name: String,
    base_config: ServerConfig,
    session_id: u64,
    handshake_ms: u32,
    #[cfg(all(test, any(target_os = "linux", target_os = "freebsd")))]
    hooks: TestHooks,
}

/// Reports a session worker's exit, after its socket closed, however the
/// worker returns.
#[cfg(test)]
struct SessionExitNotice(Option<std::sync::mpsc::Sender<()>>);

#[cfg(test)]
impl Drop for SessionExitNotice {
    fn drop(&mut self) {
        TestHooks::notify(&self.0);
    }
}

#[cfg(any(target_os = "linux", target_os = "freebsd"))]
type WorkerShm = Option<ShmContext>;
#[cfg(not(any(target_os = "linux", target_os = "freebsd")))]
type WorkerShm = Option<()>;

impl AcceptContext {
    /// Run the handshake with the per-session config; the session's SHM
    /// region is created inside it, only once SHM is selected. Returns
    /// `None` if the session must not start.
    fn handshake(
        self,
        pending: PendingConnection,
        stop_requested: &AtomicBool,
        learned_request_payload_bytes: &AtomicU32,
        learned_response_payload_bytes: &AtomicU32,
    ) -> Option<(UdsSession, WorkerShm)> {
        let mut cfg = self.base_config.clone();
        cfg.max_request_payload_bytes = learned_request_payload_bytes.load(Ordering::Acquire);
        cfg.max_response_payload_bytes = learned_response_payload_bytes.load(Ordering::Acquire);

        #[cfg(any(target_os = "linux", target_os = "freebsd"))]
        {
            let mut shm: WorkerShm = None;
            let result = {
                let mut prepare = |profile: u32, request: u32, response: u32| -> bool {
                    if profile != PROFILE_SHM_HYBRID && profile != PROFILE_SHM_FUTEX {
                        return true;
                    }
                    shm = self.create_shm(request, response);
                    shm.is_some()
                };
                pending.handshake_prepared(
                    self.session_id,
                    &cfg,
                    self.handshake_ms,
                    Some(stop_requested),
                    &mut prepare,
                )
            };
            let session = match result {
                Ok(session) => session,
                Err(_) => {
                    if let Some(mut shm) = shm {
                        shm.destroy();
                    }
                    return None;
                }
            };

            let profile = session.selected_profile;
            let wants_shm = profile == PROFILE_SHM_HYBRID || profile == PROFILE_SHM_FUTEX;
            match (wants_shm, shm) {
                (true, Some(shm)) => Some((session, Some(shm))),
                // SHM profile guarantee: the prepare step created the region
                // before the ACK selected SHM.
                (true, None) => None,
                (false, shm) => {
                    if let Some(mut shm) = shm {
                        shm.destroy();
                    }
                    Some((session, None))
                }
            }
        }
        #[cfg(not(any(target_os = "linux", target_os = "freebsd")))]
        {
            let session = pending
                .handshake(
                    self.session_id,
                    &cfg,
                    self.handshake_ms,
                    Some(stop_requested),
                )
                .ok()?;
            Some((session, None))
        }
    }

    /// Create the session's SHM region for the agreed payload limits.
    #[cfg(any(target_os = "linux", target_os = "freebsd"))]
    fn create_shm(&self, request_payload: u32, response_payload: u32) -> Option<ShmContext> {
        #[cfg(test)]
        if self.hooks.fail_shm_create.load(Ordering::Acquire) {
            return None;
        }
        let ctx = ShmContext::server_create(
            &self.run_dir,
            &self.service_name,
            self.session_id,
            request_payload.checked_add(HEADER_SIZE as u32)?,
            response_payload.checked_add(HEADER_SIZE as u32)?,
        )
        .ok()?;
        #[cfg(test)]
        TestHooks::notify(&self.hooks.shm_created);
        Some(ctx)
    }
}
