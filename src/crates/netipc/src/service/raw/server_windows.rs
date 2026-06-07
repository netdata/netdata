use super::server::{ManagedServer, ServerConfig};
use super::server_session_windows::handle_session_win_threaded;
use crate::protocol::{NipcError, HEADER_SIZE};
use crate::transport::win_shm::{
    WinShmContext, PROFILE_BUSYWAIT as WIN_SHM_PROFILE_BUSYWAIT,
    PROFILE_HYBRID as WIN_SHM_PROFILE_HYBRID,
};
use crate::transport::windows::{NpListener, NpSession};
use std::sync::atomic::Ordering;

impl ManagedServer {
    /// Windows: run the acceptor loop over Named Pipes.
    pub fn run(&mut self) -> Result<(), NipcError> {
        let mut listener = NpListener::bind(
            &self.run_dir,
            &self.service_name,
            self.server_config.clone(),
        )
        .map_err(|_| NipcError::BadLayout)?;

        *self.listener_handle.lock().unwrap() = Some(listener.handle() as usize);

        self.running.store(true, Ordering::Release);

        let mut session_threads: Vec<std::thread::JoinHandle<()>> = Vec::new();

        while self.running.load(Ordering::Acquire) {
            let (session_id, accept_cfg, prepared_shm, ready) = self.prepare_windows_accept();
            if !ready {
                std::thread::sleep(std::time::Duration::from_millis(10));
                continue;
            }

            let session = match listener.accept_with_config(session_id, accept_cfg) {
                Ok(s) => s,
                Err(_) => {
                    if let Some(mut prepared) = prepared_shm {
                        prepared.destroy_all();
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
                if let Some(mut prepared) = prepared_shm {
                    prepared.destroy_all();
                }
                drop(session);
                continue;
            }

            let shm = self.finalize_windows_shm(&session, prepared_shm);
            if shm.is_none()
                && (session.selected_profile == WIN_SHM_PROFILE_HYBRID
                    || session.selected_profile == WIN_SHM_PROFILE_BUSYWAIT)
            {
                drop(session);
                continue;
            }

            let expected_method_code = self.expected_method_code;
            let handler = self.handler.clone();
            let running = self.running.clone();
            let learned_request_payload_bytes = self.learned_request_payload_bytes.clone();
            let learned_response_payload_bytes = self.learned_response_payload_bytes.clone();
            let t = std::thread::spawn(move || {
                handle_session_win_threaded(
                    session,
                    shm,
                    expected_method_code,
                    handler,
                    running,
                    learned_request_payload_bytes,
                    learned_response_payload_bytes,
                );
            });
            session_threads.push(t);
        }

        for t in session_threads {
            let _ = t.join();
        }

        Ok(())
    }

    fn prepare_windows_accept(&mut self) -> (u64, ServerConfig, Option<PreparedWinShm>, bool) {
        let session_id = self.next_session_id;
        self.next_session_id += 1;

        let mut cfg = self.server_config.clone();
        cfg.max_request_payload_bytes = self.learned_request_payload_bytes.load(Ordering::Acquire);
        cfg.max_response_payload_bytes =
            self.learned_response_payload_bytes.load(Ordering::Acquire);

        let shm_profiles =
            cfg.supported_profiles & (WIN_SHM_PROFILE_HYBRID | WIN_SHM_PROFILE_BUSYWAIT);
        if shm_profiles == 0 {
            return (session_id, cfg, None, true);
        }

        let mut prepared = PreparedWinShm::default();
        for profile in [WIN_SHM_PROFILE_HYBRID, WIN_SHM_PROFILE_BUSYWAIT] {
            if cfg.supported_profiles & profile == 0 {
                continue;
            }

            match WinShmContext::server_create(
                &self.run_dir,
                &self.service_name,
                self.server_config.auth_token,
                session_id,
                profile,
                cfg.max_request_payload_bytes + HEADER_SIZE as u32,
                cfg.max_response_payload_bytes + HEADER_SIZE as u32,
            ) {
                Ok(ctx) => prepared.insert(profile, ctx),
                Err(_) => {
                    cfg.supported_profiles &= !profile;
                    cfg.preferred_profiles &= !profile;
                }
            }
        }

        if cfg.supported_profiles == 0 {
            prepared.destroy_all();
            return (session_id, cfg, None, false);
        }

        if prepared.is_empty() {
            return (session_id, cfg, None, true);
        }

        (session_id, cfg, Some(prepared), true)
    }

    fn finalize_windows_shm(
        &self,
        session: &NpSession,
        mut prepared: Option<PreparedWinShm>,
    ) -> Option<WinShmContext> {
        let profile = session.selected_profile;
        if profile != WIN_SHM_PROFILE_HYBRID && profile != WIN_SHM_PROFILE_BUSYWAIT {
            if let Some(ref mut prepared) = prepared {
                prepared.destroy_all();
            }
            return None;
        }
        let mut prepared = prepared?;
        // Keep the negotiated context and destroy every unused prepared context.
        let selected = prepared.take(profile);
        prepared.destroy_all();
        selected
    }
}

#[derive(Default)]
struct PreparedWinShm {
    hybrid: Option<WinShmContext>,
    busywait: Option<WinShmContext>,
}

impl PreparedWinShm {
    fn insert(&mut self, profile: u32, ctx: WinShmContext) {
        if profile == WIN_SHM_PROFILE_HYBRID {
            self.hybrid = Some(ctx);
        } else if profile == WIN_SHM_PROFILE_BUSYWAIT {
            self.busywait = Some(ctx);
        }
    }

    fn take(&mut self, profile: u32) -> Option<WinShmContext> {
        if profile == WIN_SHM_PROFILE_HYBRID {
            self.hybrid.take()
        } else if profile == WIN_SHM_PROFILE_BUSYWAIT {
            self.busywait.take()
        } else {
            None
        }
    }

    fn destroy_all(&mut self) {
        if let Some(mut ctx) = self.hybrid.take() {
            ctx.destroy();
        }
        if let Some(mut ctx) = self.busywait.take() {
            ctx.destroy();
        }
    }

    fn is_empty(&self) -> bool {
        self.hybrid.is_none() && self.busywait.is_none()
    }
}
