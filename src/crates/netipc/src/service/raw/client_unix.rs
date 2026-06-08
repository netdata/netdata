#![cfg(unix)]

use super::client::{ClientState, RawClient};
use super::common::{CLIENT_SHM_ATTACH_RETRY_INTERVAL_MS, CLIENT_SHM_ATTACH_RETRY_TIMEOUT_MS};
#[cfg(target_os = "linux")]
use crate::protocol::{PROFILE_SHM_FUTEX, PROFILE_SHM_HYBRID};
use crate::transport::posix::UdsSession;
#[cfg(target_os = "linux")]
use crate::transport::shm::ShmContext;

impl RawClient {
    /// Attempt a full connection: transport connect + handshake, then SHM
    /// upgrade if negotiated.
    #[cfg(unix)]
    pub(super) fn try_connect(&mut self) -> ClientState {
        match UdsSession::connect(&self.run_dir, &self.service_name, &self.transport_config) {
            Ok(session) => {
                #[cfg(target_os = "linux")]
                let selected_profile = session.selected_profile;
                #[cfg(target_os = "linux")]
                let session_id = session.session_id;

                #[cfg(target_os = "linux")]
                {
                    if selected_profile == PROFILE_SHM_HYBRID
                        || selected_profile == PROFILE_SHM_FUTEX
                    {
                        let mut shm_ok = false;
                        let deadline = std::time::Instant::now()
                            + std::time::Duration::from_millis(CLIENT_SHM_ATTACH_RETRY_TIMEOUT_MS);
                        loop {
                            match ShmContext::client_attach(
                                &self.run_dir,
                                &self.service_name,
                                session_id,
                            ) {
                                Ok(ctx) => {
                                    self.shm = Some(ctx);
                                    shm_ok = true;
                                    break;
                                }
                                Err(_) => {
                                    if std::time::Instant::now() >= deadline {
                                        break;
                                    }
                                    std::thread::sleep(std::time::Duration::from_millis(
                                        CLIENT_SHM_ATTACH_RETRY_INTERVAL_MS,
                                    ));
                                }
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
