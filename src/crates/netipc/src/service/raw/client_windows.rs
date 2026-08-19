use super::client::{ClientState, RawClient};
use super::common::{CLIENT_SHM_ATTACH_RETRY_INTERVAL_MS, CLIENT_SHM_ATTACH_RETRY_TIMEOUT_MS};
use crate::transport::win_shm::{
    WinShmContext, PROFILE_BUSYWAIT as WIN_SHM_PROFILE_BUSYWAIT,
    PROFILE_HYBRID as WIN_SHM_PROFILE_HYBRID,
};
use crate::transport::windows::{NpError, NpSession};

impl RawClient {
    /// Windows: attempt a full Named Pipe connection + Win SHM upgrade.
    pub(super) fn try_connect(&mut self) -> ClientState {
        match NpSession::connect(&self.run_dir, &self.service_name, &self.transport_config) {
            Ok(session) => {
                let selected_profile = session.selected_profile;

                if selected_profile == WIN_SHM_PROFILE_HYBRID
                    || selected_profile == WIN_SHM_PROFILE_BUSYWAIT
                {
                    let mut shm_ok = false;
                    let deadline = std::time::Instant::now()
                        + std::time::Duration::from_millis(CLIENT_SHM_ATTACH_RETRY_TIMEOUT_MS);
                    loop {
                        match WinShmContext::client_attach(
                            &self.run_dir,
                            &self.service_name,
                            self.transport_config.auth_token,
                            session.session_id,
                            selected_profile,
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
                            !(WIN_SHM_PROFILE_HYBRID | WIN_SHM_PROFILE_BUSYWAIT);
                        self.transport_config.preferred_profiles &=
                            !(WIN_SHM_PROFILE_HYBRID | WIN_SHM_PROFILE_BUSYWAIT);
                        if self.transport_config.supported_profiles == 0 {
                            return ClientState::Disconnected;
                        }
                        return self.try_connect();
                    }
                }

                self.session = Some(session);
                ClientState::Ready
            }
            Err(e) => match e {
                NpError::Connect(_) => ClientState::NotFound,
                NpError::AuthFailed => ClientState::AuthFailed,
                NpError::NoProfile => ClientState::Incompatible,
                NpError::Incompatible(_) => ClientState::Incompatible,
                _ => ClientState::Disconnected,
            },
        }
    }
}
