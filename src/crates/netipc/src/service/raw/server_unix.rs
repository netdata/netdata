use super::common::SERVER_POLL_TIMEOUT_MS;
use super::server::{ManagedServer, ServerConfig};
use super::server_session_unix::{handle_session_threaded, poll_fd};
use crate::protocol::NipcError;
#[cfg(target_os = "linux")]
use crate::protocol::{HEADER_SIZE, PROFILE_SHM_FUTEX, PROFILE_SHM_HYBRID};
use crate::transport::posix::UdsListener;
#[cfg(target_os = "linux")]
use crate::transport::shm::ShmContext;
use std::sync::atomic::Ordering;

impl ManagedServer {
    /// Run the acceptor loop. Blocking. Accepts clients, spawns a
    /// thread per session (up to worker_count concurrent sessions).
    ///
    /// Returns when `stop()` is called or on fatal error.
    #[cfg(unix)]
    pub fn run(&mut self) -> Result<(), NipcError> {
        #[cfg(target_os = "linux")]
        crate::transport::shm::cleanup_stale(&self.run_dir, &self.service_name);

        let listener = UdsListener::bind(
            &self.run_dir,
            &self.service_name,
            self.server_config.clone(),
        )
        .map_err(|_| NipcError::BadLayout)?;

        self.running.store(true, Ordering::Release);

        let mut session_threads: Vec<std::thread::JoinHandle<()>> = Vec::new();

        while self.running.load(Ordering::Acquire) {
            let ready = poll_fd(listener.fd(), SERVER_POLL_TIMEOUT_MS as i32);
            if ready < 0 {
                break;
            }
            if ready == 0 {
                session_threads.retain(|t| !t.is_finished());
                continue;
            }

            let (session_id, accept_cfg, precreated_shm, ready) = self.prepare_unix_accept();
            if !ready {
                std::thread::sleep(std::time::Duration::from_millis(10));
                continue;
            }

            let session = match listener.accept_with_config(session_id, accept_cfg) {
                Ok(s) => s,
                Err(_) => {
                    #[cfg(target_os = "linux")]
                    if let Some(mut shm) = precreated_shm {
                        shm.destroy();
                    }
                    if !self.running.load(Ordering::Acquire) {
                        break;
                    }
                    std::thread::sleep(std::time::Duration::from_millis(10));
                    continue;
                }
            };

            session_threads.retain(|t| !t.is_finished());
            if session_threads.len() >= self.worker_count {
                #[cfg(target_os = "linux")]
                if let Some(mut shm) = precreated_shm {
                    shm.destroy();
                }
                drop(session);
                continue;
            }

            #[cfg(target_os = "linux")]
            let shm = match self.finalize_unix_shm(&session, precreated_shm) {
                Some(shm) => Some(shm),
                None if session.selected_profile == PROFILE_SHM_HYBRID
                    || session.selected_profile == PROFILE_SHM_FUTEX =>
                {
                    drop(session);
                    continue;
                }
                None => None,
            };
            #[cfg(not(target_os = "linux"))]
            let shm: Option<()> = None;

            let expected_method_code = self.expected_method_code;
            let handler = self.handler.clone();
            let running = self.running.clone();
            let learned_request_payload_bytes = self.learned_request_payload_bytes.clone();
            let learned_response_payload_bytes = self.learned_response_payload_bytes.clone();
            let request_payload_growth_ceiling = self.request_payload_growth_ceiling;
            let response_payload_growth_ceiling = self.response_payload_growth_ceiling;

            let t = std::thread::spawn(move || {
                handle_session_threaded(
                    session,
                    #[cfg(target_os = "linux")]
                    shm,
                    #[cfg(not(target_os = "linux"))]
                    shm,
                    expected_method_code,
                    handler,
                    running,
                    learned_request_payload_bytes,
                    learned_response_payload_bytes,
                    request_payload_growth_ceiling,
                    response_payload_growth_ceiling,
                );
            });
            session_threads.push(t);
        }

        for t in session_threads {
            let _ = t.join();
        }

        Ok(())
    }

    #[cfg(target_os = "linux")]
    fn prepare_unix_accept(&mut self) -> (u64, ServerConfig, Option<ShmContext>, bool) {
        let session_id = self.next_session_id;
        self.next_session_id += 1;

        let mut cfg = self.server_config.clone();
        cfg.max_request_payload_bytes = self.learned_request_payload_bytes.load(Ordering::Acquire);
        cfg.max_response_payload_bytes =
            self.learned_response_payload_bytes.load(Ordering::Acquire);

        let shm_profiles = cfg.supported_profiles & (PROFILE_SHM_HYBRID | PROFILE_SHM_FUTEX);
        if shm_profiles == 0 {
            return (session_id, cfg, None, true);
        }

        match ShmContext::server_create(
            &self.run_dir,
            &self.service_name,
            session_id,
            cfg.max_request_payload_bytes + HEADER_SIZE as u32,
            cfg.max_response_payload_bytes + HEADER_SIZE as u32,
        ) {
            Ok(ctx) => (session_id, cfg, Some(ctx), true),
            Err(_) => {
                cfg.supported_profiles &= !(PROFILE_SHM_HYBRID | PROFILE_SHM_FUTEX);
                cfg.preferred_profiles &= !(PROFILE_SHM_HYBRID | PROFILE_SHM_FUTEX);
                (session_id, cfg.clone(), None, cfg.supported_profiles != 0)
            }
        }
    }

    #[cfg(target_os = "linux")]
    fn finalize_unix_shm(
        &self,
        session: &crate::transport::posix::UdsSession,
        mut shm: Option<ShmContext>,
    ) -> Option<ShmContext> {
        let profile = session.selected_profile;
        if profile != PROFILE_SHM_HYBRID && profile != PROFILE_SHM_FUTEX {
            if let Some(ref mut ctx) = shm {
                ctx.destroy();
            }
            return None;
        }
        shm
    }
}
