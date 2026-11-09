use super::client::{ClientState, RawClient};
use super::common::CLIENT_SHM_ATTACH_RETRY_INTERVAL_MS;
use crate::transport::win_shm::{
    PROFILE_BUSYWAIT as WIN_SHM_PROFILE_BUSYWAIT, PROFILE_HYBRID as WIN_SHM_PROFILE_HYBRID,
    WinShmContext,
};
use crate::transport::windows::{NpError, NpSession};

impl RawClient {
    /// Windows: attempt a full Named Pipe connection + Win SHM upgrade.
    pub(super) fn try_connect(&mut self) -> ClientState {
        let remaining = self.op_remaining_ms();
        if remaining == 0 || self.abort_requested() {
            return ClientState::Disconnected;
        }
        let abort = self.abort_requested.clone();
        match NpSession::connect_with_timeout(
            &self.run_dir,
            &self.service_name,
            &self.transport_config,
            remaining,
            Some(&abort),
        ) {
            Ok(session) => {
                let selected_profile = session.selected_profile;

                if selected_profile == WIN_SHM_PROFILE_HYBRID
                    || selected_profile == WIN_SHM_PROFILE_BUSYWAIT
                {
                    let mut shm_ok = false;
                    // When the call deadline ends the wait first, WinSHM is
                    // not at fault and stays enabled for the next connect.
                    let call_left = std::time::Duration::from_millis(self.op_remaining_ms() as u64);
                    // The remaining time is rounded up, so a tie means the
                    // call deadline comes first.
                    let call_bound = call_left <= self.shm_attach_timeout;
                    let deadline =
                        std::time::Instant::now() + call_left.min(self.shm_attach_timeout);
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
                                    if call_bound {
                                        return ClientState::Disconnected;
                                    }
                                    break;
                                }
                                if self.abort_requested() {
                                    return ClientState::Disconnected;
                                }
                                // A server that refused or died breaks the
                                // session pipe; its SHM objects will never
                                // appear. Not a WinSHM problem: keep it enabled.
                                if super::server_session_windows::peer_disconnected(&session) {
                                    return ClientState::Disconnected;
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
                NpError::AuthFailed | NpError::UntrustedServer => ClientState::AuthFailed,
                NpError::NoProfile => ClientState::Incompatible,
                NpError::Incompatible(_) => ClientState::Incompatible,
                _ => ClientState::Disconnected,
            },
        }
    }
}
