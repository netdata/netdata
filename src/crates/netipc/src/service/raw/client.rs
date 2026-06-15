use super::common::{ensure_client_scratch, next_power_of_2_u32};
use crate::protocol::{self, NipcError, MAX_PAYLOAD_CAP};
use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::Arc;

#[cfg(unix)]
pub(super) use crate::transport::posix::ClientConfig;

#[cfg(unix)]
use crate::transport::posix::UdsSession;

#[cfg(target_os = "linux")]
use crate::transport::shm::ShmContext;

#[cfg(windows)]
pub(super) use crate::transport::windows::ClientConfig;

#[cfg(windows)]
use crate::transport::windows::NpSession;

#[cfg(windows)]
use crate::transport::win_shm::WinShmContext;

/// Default timeout for synchronous client calls.
pub const CLIENT_CALL_TIMEOUT_DEFAULT_MS: u32 = 30_000;

pub(super) const CLIENT_ABORT_POLL_MS: u32 = 100;

/// Client connection state machine.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum ClientState {
    Disconnected,
    Connecting,
    Ready,
    NotFound,
    AuthFailed,
    Incompatible,
    Broken,
}

/// Diagnostic counters snapshot.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct ClientStatus {
    pub state: ClientState,
    pub connect_count: u32,
    pub reconnect_count: u32,
    pub call_count: u32,
    pub error_count: u32,
}

/// L2 client context bound to one service kind.
///
/// Manages connection lifecycle and provides typed blocking calls with
/// at-least-once retry semantics. The outer request code remains only for
/// validation; each client instance is bound to one expected request kind.
pub struct RawClient {
    pub(super) state: ClientState,
    pub(super) run_dir: String,
    pub(super) service_name: String,
    pub(super) expected_method_code: u16,
    pub(super) transport_config: ClientConfig,

    #[cfg(unix)]
    pub(super) session: Option<UdsSession>,
    #[cfg(target_os = "linux")]
    pub(super) shm: Option<ShmContext>,

    #[cfg(windows)]
    pub(super) session: Option<NpSession>,
    #[cfg(windows)]
    pub(super) shm: Option<WinShmContext>,

    pub(super) request_buf: Vec<u8>,
    pub(super) send_buf: Vec<u8>,
    pub(super) transport_buf: Vec<u8>,

    pub(super) connect_count: u32,
    pub(super) reconnect_count: u32,
    pub(super) call_count: u32,
    pub(super) error_count: u32,

    pub(super) call_timeout_ms: u32,
    pub(super) abort_requested: Arc<AtomicBool>,
}

/// Cloneable handle that can abort a blocking client call from another thread.
///
/// Abort is sticky until `clear()` is called on either the handle or the
/// owning client.
#[derive(Clone, Debug)]
pub struct ClientAbortHandle {
    requested: Arc<AtomicBool>,
}

impl ClientAbortHandle {
    /// Request abort of an in-flight or future synchronous call.
    pub fn abort(&self) {
        self.requested.store(true, Ordering::Release);
    }

    /// Clear a previous abort request.
    pub fn clear(&self) {
        self.requested.store(false, Ordering::Release);
    }
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(super) struct RawCallKind {
    pub(super) flags: u16,
    pub(super) item_count: u32,
    pub(super) check_item_count: bool,
}

impl RawCallKind {
    pub(super) fn single() -> Self {
        Self {
            flags: 0,
            item_count: 1,
            check_item_count: false,
        }
    }

    pub(super) fn batch(item_count: u32) -> Self {
        Self {
            flags: protocol::FLAG_BATCH,
            item_count,
            check_item_count: true,
        }
    }
}

impl RawClient {
    pub(super) fn new_bound(
        run_dir: &str,
        service_name: &str,
        expected_method_code: u16,
        config: ClientConfig,
    ) -> Self {
        RawClient {
            state: ClientState::Disconnected,
            run_dir: run_dir.to_string(),
            service_name: service_name.to_string(),
            expected_method_code,
            transport_config: config,
            session: None,
            #[cfg(target_os = "linux")]
            shm: None,
            #[cfg(windows)]
            shm: None,
            request_buf: Vec::new(),
            send_buf: Vec::new(),
            transport_buf: Vec::new(),
            connect_count: 0,
            reconnect_count: 0,
            call_count: 0,
            error_count: 0,
            call_timeout_ms: CLIENT_CALL_TIMEOUT_DEFAULT_MS,
            abort_requested: Arc::new(AtomicBool::new(false)),
        }
    }

    pub(super) fn resolved_call_timeout(&self, timeout_ms: u32) -> u32 {
        if timeout_ms != 0 {
            return timeout_ms;
        }
        if self.call_timeout_ms != 0 {
            return self.call_timeout_ms;
        }
        CLIENT_CALL_TIMEOUT_DEFAULT_MS
    }

    pub(super) fn abort_requested(&self) -> bool {
        self.abort_requested.load(Ordering::Acquire)
    }

    /// Set the context-level default timeout for blocking calls.
    pub fn set_call_timeout(&mut self, timeout_ms: u32) {
        self.call_timeout_ms = if timeout_ms == 0 {
            CLIENT_CALL_TIMEOUT_DEFAULT_MS
        } else {
            timeout_ms
        };
    }

    /// Return a cloneable handle that can abort calls from another thread.
    pub fn abort_handle(&self) -> ClientAbortHandle {
        ClientAbortHandle {
            requested: self.abort_requested.clone(),
        }
    }

    /// Request abort of an in-flight or future synchronous call.
    pub fn abort(&self) {
        self.abort_requested.store(true, Ordering::Release);
    }

    /// Clear a previous abort request so the client can be refreshed/reused.
    pub fn clear_abort(&self) {
        self.abort_requested.store(false, Ordering::Release);
    }

    /// Attempt connect if DISCONNECTED/NOT_FOUND, reconnect if BROKEN.
    /// Returns true if the state changed.
    pub fn refresh(&mut self) -> bool {
        let old_state = self.state;

        match self.state {
            ClientState::Disconnected | ClientState::NotFound => {
                self.state = ClientState::Connecting;
                self.state = self.try_connect();
                if self.state == ClientState::Ready {
                    self.connect_count += 1;
                }
            }
            ClientState::Broken => {
                self.disconnect();
                self.state = ClientState::Connecting;
                self.state = self.try_connect();
                if self.state == ClientState::Ready {
                    self.reconnect_count += 1;
                }
            }
            ClientState::Ready
            | ClientState::Connecting
            | ClientState::AuthFailed
            | ClientState::Incompatible => {}
        }

        self.state != old_state
    }

    /// Cheap cached boolean. No I/O, no syscalls.
    #[inline]
    pub fn ready(&self) -> bool {
        self.state == ClientState::Ready
    }

    /// Detailed status snapshot for diagnostics.
    pub fn status(&self) -> ClientStatus {
        ClientStatus {
            state: self.state,
            connect_count: self.connect_count,
            reconnect_count: self.reconnect_count,
            call_count: self.call_count,
            error_count: self.error_count,
        }
    }

    pub(super) fn request_scratch(&mut self, needed: usize) -> &mut [u8] {
        ensure_client_scratch(&mut self.request_buf, needed)
    }

    pub(super) fn validate_method(&self, method_code: u16) -> Result<(), NipcError> {
        if self.expected_method_code == method_code {
            Ok(())
        } else {
            Err(NipcError::BadLayout)
        }
    }

    pub(super) fn session_max_request_payload_bytes(&self) -> u32 {
        #[cfg(unix)]
        if let Some(ref session) = self.session {
            return session.max_request_payload_bytes;
        }

        #[cfg(windows)]
        if let Some(ref session) = self.session {
            return session.max_request_payload_bytes;
        }

        self.transport_config.max_request_payload_bytes
    }

    pub(super) fn session_max_response_payload_bytes(&self) -> u32 {
        #[cfg(unix)]
        if let Some(ref session) = self.session {
            return session.max_response_payload_bytes;
        }

        #[cfg(windows)]
        if let Some(ref session) = self.session {
            return session.max_response_payload_bytes;
        }

        self.transport_config.max_response_payload_bytes
    }

    pub(super) fn client_note_request_capacity(&mut self, payload_len: u32) {
        let grown = next_power_of_2_u32(payload_len).min(MAX_PAYLOAD_CAP);
        if grown > self.transport_config.max_request_payload_bytes {
            self.transport_config.max_request_payload_bytes = grown;
        }
    }

    pub(super) fn client_note_response_capacity(&mut self, payload_len: u32) {
        let grown = next_power_of_2_u32(payload_len).min(MAX_PAYLOAD_CAP);
        if grown > self.transport_config.max_response_payload_bytes {
            self.transport_config.max_response_payload_bytes = grown;
        }
    }

    /// Tear down connection and release resources.
    pub fn close(&mut self) {
        self.disconnect();
        self.state = ClientState::Disconnected;
        self.clear_abort();
    }

    /// Tear down the current connection.
    pub(super) fn disconnect(&mut self) {
        #[cfg(target_os = "linux")]
        {
            if let Some(mut shm) = self.shm.take() {
                shm.close();
            }
        }

        #[cfg(windows)]
        {
            if let Some(mut shm) = self.shm.take() {
                shm.close();
            }
        }

        self.session.take();
    }
}

impl Drop for RawClient {
    fn drop(&mut self) {
        self.close();
    }
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(super) enum ClientResponseSource {
    TransportBuf,
    SessionBuf,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(super) struct ClientResponseRef {
    pub(super) source: ClientResponseSource,
    pub(super) len: usize,
}
