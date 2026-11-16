use super::client::{ClientState, RawClient};
#[cfg(any(target_os = "linux", target_os = "freebsd"))]
use super::common::{CLIENT_SHM_ATTACH_RETRY_INTERVAL_MS, CLIENT_SHM_CALL_BOUND_FAILURE_LIMIT};
#[cfg(any(target_os = "linux", target_os = "freebsd"))]
use crate::protocol::{PROFILE_SHM_FUTEX, PROFILE_SHM_HYBRID};
use crate::transport::posix::UdsSession;
#[cfg(any(target_os = "linux", target_os = "freebsd"))]
use crate::transport::shm::{ShmContext, ShmError};

impl RawClient {
    /// Attempt a full connection: transport connect + handshake, then SHM
    /// upgrade if negotiated.
    #[cfg(unix)]
    pub(super) fn try_connect(&mut self) -> ClientState {
        let remaining = self.op_remaining_ms();
        if remaining == 0 || self.abort_requested() {
            return ClientState::Disconnected;
        }
        let abort = self.abort_requested.clone();
        match UdsSession::connect_with_timeout(
            &self.run_dir,
            &self.service_name,
            &self.transport_config,
            remaining,
            Some(&abort),
        ) {
            Ok(session) => {
                #[cfg(any(target_os = "linux", target_os = "freebsd"))]
                let selected_profile = session.selected_profile;
                #[cfg(any(target_os = "linux", target_os = "freebsd"))]
                let session_id = session.session_id;

                #[cfg(any(target_os = "linux", target_os = "freebsd"))]
                {
                    if selected_profile == PROFILE_SHM_HYBRID
                        || selected_profile == PROFILE_SHM_FUTEX
                    {
                        let mut shm_ok = false;
                        // When the call deadline ends the wait first, SHM is
                        // not at fault and stays enabled for the next connect,
                        // unless that keeps happening
                        // (CLIENT_SHM_CALL_BOUND_FAILURE_LIMIT).
                        let call_left =
                            std::time::Duration::from_millis(self.op_remaining_ms() as u64);
                        // The remaining time is rounded up, so a tie means the
                        // call deadline comes first.
                        let call_bound = call_left <= self.shm_attach_timeout;
                        let deadline =
                            std::time::Instant::now() + call_left.min(self.shm_attach_timeout);
                        loop {
                            match ShmContext::client_attach(
                                &self.run_dir,
                                &self.service_name,
                                session_id,
                            ) {
                                Ok(ctx) => {
                                    self.shm = Some(ctx);
                                    self.shm_call_bound_failures = 0;
                                    shm_ok = true;
                                    break;
                                }
                                // Only a region the server has not finished
                                // yet is worth waiting for.
                                Err(
                                    ShmError::NotReady | ShmError::Open(_) | ShmError::BadMagic,
                                ) => {
                                    if self.abort_requested() {
                                        return ClientState::Disconnected;
                                    }
                                    // A server that refused or died closes the
                                    // session socket; its SHM region will never
                                    // appear. Not an SHM problem: keep SHM enabled.
                                    // Checked before the deadline, so a peer gone
                                    // at either deadline is never an SHM failure.
                                    if super::server_session_unix::peer_disconnected(session.fd()) {
                                        return ClientState::Disconnected;
                                    }
                                    if std::time::Instant::now() >= deadline {
                                        if call_bound {
                                            self.shm_call_bound_failures += 1;
                                            if self.shm_call_bound_failures
                                                < CLIENT_SHM_CALL_BOUND_FAILURE_LIMIT
                                            {
                                                return ClientState::Disconnected;
                                            }
                                        }
                                        break;
                                    }
                                    std::thread::sleep(std::time::Duration::from_millis(
                                        CLIENT_SHM_ATTACH_RETRY_INTERVAL_MS,
                                    ));
                                }
                                Err(_) => break,
                            }
                        }
                        if !shm_ok {
                            drop(session);
                            self.transport_config.supported_profiles &=
                                !(PROFILE_SHM_HYBRID | PROFILE_SHM_FUTEX);
                            self.transport_config.preferred_profiles &=
                                !(PROFILE_SHM_HYBRID | PROFILE_SHM_FUTEX);
                            if self.transport_config.supported_profiles == 0 {
                                return ClientState::Disconnected;
                            }
                            return self.try_connect();
                        }
                    }
                }

                self.session = Some(session);
                ClientState::Ready
            }
            Err(e) => {
                use crate::transport::posix::UdsError;
                match e {
                    UdsError::Connect(_) => ClientState::NotFound,
                    UdsError::AuthFailed => ClientState::AuthFailed,
                    UdsError::NoProfile => ClientState::Incompatible,
                    UdsError::Incompatible(_) => ClientState::Incompatible,
                    _ => ClientState::Disconnected,
                }
            }
        }
    }
}
