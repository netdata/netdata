use super::common::wait_for_worker_slot;
#[cfg(test)]
use super::server::TestHooks;
use super::server::{ManagedServer, ServerConfig};
use super::server_session_windows::handle_session_win_threaded;
use crate::protocol::{HEADER_SIZE, NipcError};
use crate::transport::win_shm::{
    PROFILE_BUSYWAIT as WIN_SHM_PROFILE_BUSYWAIT, PROFILE_HYBRID as WIN_SHM_PROFILE_HYBRID,
    WinShmContext,
};
use crate::transport::windows::{NpListener, NpSession, PendingPipe};
use std::os::windows::io::AsRawHandle;
use std::sync::atomic::{AtomicBool, AtomicU32, Ordering};
use std::thread::JoinHandle;
use std::time::{Duration, Instant};

unsafe extern "system" {
    fn CancelSynchronousIo(thread: isize) -> i32;
}

impl ManagedServer {
    /// Windows: run the acceptor loop over Named Pipes.
    pub fn run(&mut self) -> Result<(), NipcError> {
        // A stop before run must win; the server is single-use.
        if self.stop_requested.load(Ordering::Acquire) {
            return Ok(());
        }

        let mut listener = NpListener::bind(
            &self.run_dir,
            &self.service_name,
            self.server_config.clone(),
        )
        .map_err(|_| NipcError::BadLayout)?;

        *self.listener_waker.lock().unwrap() = Some(listener.waker());

        self.running.store(true, Ordering::Release);
        if self.stop_requested.load(Ordering::Acquire) {
            self.running.store(false, Ordering::Release);
        }
        self.listening.set();

        let mut session_threads: Vec<JoinHandle<()>> = Vec::new();

        while self.running.load(Ordering::Acquire) {
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

            // Stop wakes ConnectNamedPipe with a throwaway connection.
            if !self.running.load(Ordering::Acquire) {
                break;
            }

            // A connection holds its slot from accept, handshake included.
            // Refuse before HELLO_ACK so the client's connect fails instead of
            // a later call.
            let worker_count = self.worker_count;
            if !wait_for_worker_slot(&self.running, self.timings.slot_wait(), || {
                session_threads.retain(|t| !t.is_finished());
                session_threads.len() < worker_count
            }) {
                drop(pending);
                continue;
            }

            let session_id = self.next_session_id;
            self.next_session_id += 1;
            let accept = AcceptContext {
                run_dir: self.run_dir.clone(),
                service_name: self.service_name.clone(),
                base_config: self.server_config.clone(),
                session_id,
                handshake_ms: self.timings.handshake_ms,
                #[cfg(test)]
                hooks: self.test_hooks.clone(),
            };
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
            let t = std::thread::spawn(move || {
                #[cfg(test)]
                let _exit = SessionExitNotice(hooks.session_exited.clone());
                #[cfg(test)]
                TestHooks::notify(&hooks.handshake_started);
                let Some((session, shm)) = accept.handshake(
                    pending,
                    &stop_requested,
                    &learned_request_payload_bytes,
                    &learned_response_payload_bytes,
                ) else {
                    return;
                };
                handle_session_win_threaded(
                    session,
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
            });
            session_threads.push(t);
        }

        *self.listener_waker.lock().unwrap() = None;
        self.stop_requested.store(true, Ordering::Release);
        self.running.store(false, Ordering::Release);
        drop(listener);
        stop_workers(session_threads, self.timings.stop_grace);

        Ok(())
    }
}

/// Give workers a grace to finish their current message, then cancel the
/// pipe I/O of the ones still blocked until they exit, and join them all. A
/// single cancel can land before a thread enters its next blocking call, so
/// it repeats.
fn stop_workers(workers: Vec<JoinHandle<()>>, grace: Duration) {
    let deadline = Instant::now() + grace;
    while Instant::now() < deadline && workers.iter().any(|t| !t.is_finished()) {
        std::thread::sleep(Duration::from_millis(5));
    }

    while workers.iter().any(|t| !t.is_finished()) {
        for t in workers.iter().filter(|t| !t.is_finished()) {
            // The JoinHandle keeps the thread handle valid until join.
            unsafe {
                CancelSynchronousIo(t.as_raw_handle() as isize);
            }
        }
        std::thread::sleep(Duration::from_millis(10));
    }

    for t in workers {
        let _ = t.join();
    }
}

/// What a worker needs to run the server side of one handshake.
struct AcceptContext {
    run_dir: String,
    service_name: String,
    base_config: ServerConfig,
    session_id: u64,
    handshake_ms: u32,
    #[cfg(test)]
    hooks: TestHooks,
}

/// Reports a session worker's exit, after its pipe closed, however the
/// worker returns.
#[cfg(test)]
struct SessionExitNotice(Option<std::sync::mpsc::Sender<()>>);

#[cfg(test)]
impl Drop for SessionExitNotice {
    fn drop(&mut self) {
        TestHooks::notify(&self.0);
    }
}

impl AcceptContext {
    /// Run the handshake with the per-session config; the session's SHM
    /// objects are created inside it, only for the selected SHM profile.
    /// Returns `None` if the session must not start.
    fn handshake(
        self,
        pending: PendingPipe,
        stop_requested: &AtomicBool,
        learned_request_payload_bytes: &AtomicU32,
        learned_response_payload_bytes: &AtomicU32,
    ) -> Option<(NpSession, Option<WinShmContext>)> {
        let mut cfg = self.base_config.clone();
        cfg.max_request_payload_bytes = learned_request_payload_bytes.load(Ordering::Acquire);
        cfg.max_response_payload_bytes = learned_response_payload_bytes.load(Ordering::Acquire);

        let mut shm: Option<WinShmContext> = None;
        let result = {
            let mut prepare = |profile: u32, request: u32, response: u32| -> bool {
                if profile != WIN_SHM_PROFILE_HYBRID && profile != WIN_SHM_PROFILE_BUSYWAIT {
                    return true;
                }
                shm = self.create_shm(profile, request, response);
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
        if profile != WIN_SHM_PROFILE_HYBRID && profile != WIN_SHM_PROFILE_BUSYWAIT {
            if let Some(mut shm) = shm {
                shm.destroy();
            }
            return Some((session, None));
        }
        // SHM profile guarantee: the prepare step created the selected
        // profile's objects before the ACK selected it.
        shm.map(|shm| (session, Some(shm)))
    }

    /// Create the session's SHM objects for one profile and the agreed
    /// payload limits.
    fn create_shm(
        &self,
        profile: u32,
        request_payload: u32,
        response_payload: u32,
    ) -> Option<WinShmContext> {
        #[cfg(test)]
        if self.hooks.fail_shm_create.load(Ordering::Acquire) {
            return None;
        }
        let ctx = WinShmContext::server_create(
            &self.run_dir,
            &self.service_name,
            self.base_config.auth_token,
            self.session_id,
            profile,
            request_payload.checked_add(HEADER_SIZE as u32)?,
            response_payload.checked_add(HEADER_SIZE as u32)?,
        )
        .ok()?;
        #[cfg(test)]
        TestHooks::notify(&self.hooks.shm_created);
        Some(ctx)
    }
}
