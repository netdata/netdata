//! Internal single-kind L2 service helpers for tests and benchmarks.
//!
//! This module is not the production service contract. It exists to preserve
//! internal benchmark/stress coverage while the public service modules expose
//! one service kind per endpoint.

use crate::protocol::{
    self, batch_item_get, increment_decode, increment_encode, string_reverse_decode,
    string_reverse_encode, BatchBuilder, CgroupsRequest, CgroupsResponseView, Header, NipcError,
    FLAG_BATCH, HEADER_SIZE, INCREMENT_PAYLOAD_SIZE, KIND_REQUEST, KIND_RESPONSE, MAGIC_MSG,
    MAX_PAYLOAD_CAP, MAX_PAYLOAD_DEFAULT, METHOD_CGROUPS_SNAPSHOT, METHOD_INCREMENT,
    METHOD_STRING_REVERSE, STATUS_BAD_ENVELOPE, STATUS_INTERNAL_ERROR, STATUS_LIMIT_EXCEEDED,
    STATUS_OK, STRING_REVERSE_HDR_SIZE, VERSION,
};

#[cfg(unix)]
use crate::protocol::{PROFILE_SHM_FUTEX, PROFILE_SHM_HYBRID};

#[cfg(unix)]
use crate::transport::posix::{ClientConfig, ServerConfig, UdsListener, UdsSession};

#[cfg(target_os = "linux")]
use crate::transport::shm::ShmContext;

#[cfg(windows)]
use crate::transport::windows::{ClientConfig, NpError, NpListener, NpSession, ServerConfig};

#[cfg(windows)]
use crate::transport::win_shm::{
    WinShmContext, PROFILE_BUSYWAIT as WIN_SHM_PROFILE_BUSYWAIT,
    PROFILE_HYBRID as WIN_SHM_PROFILE_HYBRID,
};

use std::sync::atomic::{AtomicBool, AtomicU32, Ordering};
use std::sync::Arc;

/// Poll/receive timeout for server loops (ms). Controls shutdown detection latency.
const SERVER_POLL_TIMEOUT_MS: u32 = 100;
const CLIENT_SHM_ATTACH_RETRY_INTERVAL_MS: u64 = 5;
const CLIENT_SHM_ATTACH_RETRY_TIMEOUT_MS: u64 = 5_000;

fn next_power_of_2_u32(n: u32) -> u32 {
    if n < 16 {
        return 16;
    }

    // Cap at 2^31 — the largest power of 2 that fits in u32
    if n > (1u32 << 31) {
        return 1u32 << 31;
    }

    let mut value = n - 1;
    value |= value >> 1;
    value |= value >> 2;
    value |= value >> 4;
    value |= value >> 8;
    value |= value >> 16;
    value + 1
}

// ---------------------------------------------------------------------------
//  Client state
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
//  Client context
// ---------------------------------------------------------------------------

/// L2 client context bound to one service kind.
///
/// Manages connection lifecycle and provides typed blocking calls with
/// at-least-once retry semantics. The outer request code remains only for
/// validation; each client instance is bound to one expected request kind.
pub struct RawClient {
    state: ClientState,
    run_dir: String,
    service_name: String,
    expected_method_code: u16,
    transport_config: ClientConfig,

    // Connection (managed internally)
    #[cfg(unix)]
    session: Option<UdsSession>,
    #[cfg(target_os = "linux")]
    shm: Option<ShmContext>,

    #[cfg(windows)]
    session: Option<NpSession>,
    #[cfg(windows)]
    shm: Option<WinShmContext>,

    // Reusable scratch buffers owned by the client for hot request paths.
    request_buf: Vec<u8>,
    send_buf: Vec<u8>,
    transport_buf: Vec<u8>,

    // Stats
    connect_count: u32,
    reconnect_count: u32,
    call_count: u32,
    error_count: u32,
}

impl RawClient {
    fn new_bound(
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
        }
    }

    /// Create a new client context bound to the cgroups-snapshot service kind.
    /// Does NOT connect. Does NOT require the server to be running.
    pub fn new_snapshot(run_dir: &str, service_name: &str, config: ClientConfig) -> Self {
        Self::new_bound(run_dir, service_name, METHOD_CGROUPS_SNAPSHOT, config)
    }

    /// Create a new client context bound to the increment service kind.
    /// Does NOT connect. Does NOT require the server to be running.
    pub fn new_increment(run_dir: &str, service_name: &str, config: ClientConfig) -> Self {
        Self::new_bound(run_dir, service_name, METHOD_INCREMENT, config)
    }

    /// Create a new client context bound to the string-reverse service kind.
    /// Does NOT connect. Does NOT require the server to be running.
    pub fn new_string_reverse(run_dir: &str, service_name: &str, config: ClientConfig) -> Self {
        Self::new_bound(run_dir, service_name, METHOD_STRING_REVERSE, config)
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

    fn session_max_request_payload_bytes(&self) -> u32 {
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

    fn session_max_response_payload_bytes(&self) -> u32 {
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

    fn client_note_request_capacity(&mut self, payload_len: u32) {
        let grown = next_power_of_2_u32(payload_len).min(MAX_PAYLOAD_CAP);
        if grown > self.transport_config.max_request_payload_bytes {
            self.transport_config.max_request_payload_bytes = grown;
        }
    }

    fn client_note_response_capacity(&mut self, payload_len: u32) {
        let grown = next_power_of_2_u32(payload_len).min(MAX_PAYLOAD_CAP);
        if grown > self.transport_config.max_response_payload_bytes {
            self.transport_config.max_response_payload_bytes = grown;
        }
    }

    fn validate_method(&self, method_code: u16) -> Result<(), NipcError> {
        if self.expected_method_code == method_code {
            Ok(())
        } else {
            Err(NipcError::BadLayout)
        }
    }

    /// Blocking typed call: encode request, send, receive, check
    /// transport_status, decode response.
    ///
    /// The returned view is valid until the next typed call on this client.
    ///
    /// Retry policy (per spec): if the call fails and the context was
    /// previously READY, disconnect, reconnect (full handshake), and retry.
    /// Ordinary failures retry once. Overflow-driven resize recovery may
    /// reconnect more than once while negotiated capacities grow.
    pub fn call_snapshot(&mut self) -> Result<CgroupsResponseView<'_>, NipcError> {
        self.validate_method(METHOD_CGROUPS_SNAPSHOT)?;
        let req = CgroupsRequest {
            layout_version: 1,
            flags: 0,
        };
        let mut req_buf = [0u8; 4];
        let req_len = req.encode(&mut req_buf);
        if req_len == 0 {
            return Err(NipcError::Truncated);
        }

        let response = self.raw_call_with_retry(METHOD_CGROUPS_SNAPSHOT, &req_buf[..req_len])?;
        CgroupsResponseView::decode(self.response_payload(response)?)
    }

    /// Blocking typed call: INCREMENT method.
    /// Sends a u64 value, receives the incremented u64 back.
    pub fn call_increment(&mut self, value: u64) -> Result<u64, NipcError> {
        self.validate_method(METHOD_INCREMENT)?;
        let mut req_buf = [0u8; INCREMENT_PAYLOAD_SIZE];
        let req_len = increment_encode(value, &mut req_buf);
        if req_len == 0 {
            return Err(NipcError::Truncated);
        }

        let response = self.raw_call_with_retry(METHOD_INCREMENT, &req_buf[..req_len])?;
        increment_decode(self.response_payload(response)?)
    }

    /// Blocking typed call: STRING_REVERSE method.
    /// Sends a string, receives the reversed string back.
    ///
    /// The returned view is valid until the next typed call on this client.
    pub fn call_string_reverse(
        &mut self,
        s: &str,
    ) -> Result<protocol::StringReverseView<'_>, NipcError> {
        self.validate_method(METHOD_STRING_REVERSE)?;
        let req_size = STRING_REVERSE_HDR_SIZE + s.len() + 1;
        let req_buf = ensure_client_scratch(&mut self.request_buf, req_size);
        let req_len = string_reverse_encode(s.as_bytes(), req_buf);
        if req_len == 0 {
            return Err(NipcError::Truncated);
        }

        let response = self.raw_call_with_retry_request_buf(METHOD_STRING_REVERSE, req_len)?;
        string_reverse_decode(self.response_payload(response)?)
    }

    /// Blocking typed batch call: INCREMENT method.
    /// Sends multiple u64 values, receives the incremented u64s back.
    pub fn call_increment_batch(&mut self, values: &[u64]) -> Result<Vec<u64>, NipcError> {
        self.validate_method(METHOD_INCREMENT)?;
        if values.is_empty() {
            return Ok(Vec::new());
        }

        // Single value: use the non-batch path
        if values.len() == 1 {
            let r = self.call_increment(values[0])?;
            return Ok(vec![r]);
        }

        let count = values.len() as u32;

        let req_buf_size = protocol::align8(count as usize * 8)
            + count as usize * protocol::align8(INCREMENT_PAYLOAD_SIZE)
            + 64;
        let req_buf = ensure_client_scratch(&mut self.request_buf, req_buf_size);
        let req_len = {
            let mut bb = BatchBuilder::new(req_buf, count);
            for &v in values {
                let mut item_buf = [0u8; INCREMENT_PAYLOAD_SIZE];
                if increment_encode(v, &mut item_buf) == 0 {
                    return Err(NipcError::Truncated);
                }
                bb.add(&item_buf).map_err(|_| NipcError::Overflow)?;
            }
            let (req_len, _out_count) = bb.finish();
            req_len
        };

        let response =
            self.raw_batch_call_with_retry_request_buf(METHOD_INCREMENT, req_len, count)?;
        let resp_payload = self.response_payload(response)?;
        let mut results = Vec::with_capacity(values.len());
        for i in 0..count {
            let (item_data, _item_len) = batch_item_get(resp_payload, count, i)?;
            let val = increment_decode(item_data)?;
            results.push(val);
        }

        Ok(results)
    }

    /// Tear down connection and release resources.
    pub fn close(&mut self) {
        self.disconnect();
        self.state = ClientState::Disconnected;
    }

    // ------------------------------------------------------------------
    //  Internal helpers
    // ------------------------------------------------------------------

    /// Tear down the current connection.
    fn disconnect(&mut self) {
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

        // Drop the session (closes handle/fd via Drop impl)
        self.session.take();
    }

    /// Attempt a full connection: transport connect + handshake, then SHM
    /// upgrade if negotiated.
    #[cfg(unix)]
    fn try_connect(&mut self) -> ClientState {
        match UdsSession::connect(&self.run_dir, &self.service_name, &self.transport_config) {
            Ok(session) => {
                #[cfg(target_os = "linux")]
                let selected_profile = session.selected_profile;
                #[cfg(target_os = "linux")]
                let session_id = session.session_id;

                // SHM upgrade if negotiated
                #[cfg(target_os = "linux")]
                {
                    if selected_profile == PROFILE_SHM_HYBRID
                        || selected_profile == PROFILE_SHM_FUTEX
                    {
                        // Retry attach: server creates the SHM region after
                        // the UDS handshake, so it may not exist yet.
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
                            // SHM attach failed after negotiation. Close that session,
                            // blacklist SHM for this client context, and retry baseline.
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

    /// Windows: attempt a full Named Pipe connection + Win SHM upgrade.
    #[cfg(windows)]
    fn try_connect(&mut self) -> ClientState {
        match NpSession::connect(&self.run_dir, &self.service_name, &self.transport_config) {
            Ok(session) => {
                let selected_profile = session.selected_profile;

                // Win SHM upgrade if negotiated
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
                        // WinSHM attach failed after negotiation. Close that
                        // session, blacklist WinSHM for this client context,
                        // and retry baseline.
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

    /// Reconnect-driven recovery for a single-item raw call.
    /// Ordinary failures retry once. Overflow-driven resize recovery may
    /// reconnect more than once while negotiated capacities grow.
    fn raw_call_with_retry<'a>(
        &mut self,
        method_code: u16,
        request_payload: &[u8],
    ) -> Result<ClientResponseRef, NipcError> {
        if self.state != ClientState::Ready {
            self.error_count += 1;
            return Err(NipcError::BadLayout);
        }

        // Cap overflow-driven retries: payloads grow by powers of 2, so 8
        // retries allows ~256x growth from the initial negotiated size.
        let mut overflow_retries = 0u32;
        loop {
            let prev_req = self.session_max_request_payload_bytes();
            let prev_resp = self.session_max_response_payload_bytes();
            let prev_cfg_req = self.transport_config.max_request_payload_bytes;
            let prev_cfg_resp = self.transport_config.max_response_payload_bytes;

            match self.do_raw_call(method_code, request_payload) {
                Ok(payload) => {
                    self.call_count += 1;
                    return Ok(payload);
                }
                Err(first_err) => {
                    if first_err != NipcError::Overflow {
                        self.disconnect();
                        self.state = ClientState::Broken;
                        self.state = self.try_connect();
                        if self.state != ClientState::Ready {
                            self.error_count += 1;
                            return Err(first_err);
                        }
                        self.reconnect_count += 1;

                        match self.do_raw_call(method_code, request_payload) {
                            Ok(payload) => {
                                self.call_count += 1;
                                return Ok(payload);
                            }
                            Err(retry_err) => {
                                self.disconnect();
                                self.state = ClientState::Broken;
                                self.error_count += 1;
                                return Err(retry_err);
                            }
                        }
                    }

                    self.disconnect();
                    self.state = ClientState::Broken;
                    self.state = self.try_connect();
                    if self.state != ClientState::Ready {
                        self.error_count += 1;
                        return Err(first_err);
                    }
                    self.reconnect_count += 1;

                    if self.session_max_request_payload_bytes() <= prev_req
                        && self.session_max_response_payload_bytes() <= prev_resp
                        && self.transport_config.max_request_payload_bytes <= prev_cfg_req
                        && self.transport_config.max_response_payload_bytes <= prev_cfg_resp
                    {
                        self.disconnect();
                        self.state = ClientState::Broken;
                        self.error_count += 1;
                        return Err(first_err);
                    }

                    overflow_retries += 1;
                    if overflow_retries >= 8 {
                        self.disconnect();
                        self.state = ClientState::Broken;
                        self.error_count += 1;
                        return Err(first_err);
                    }
                }
            }
        }
    }

    fn raw_call_with_retry_request_buf<'a>(
        &mut self,
        method_code: u16,
        req_len: usize,
    ) -> Result<ClientResponseRef, NipcError> {
        if self.state != ClientState::Ready {
            self.error_count += 1;
            return Err(NipcError::BadLayout);
        }

        let mut overflow_retries = 0u32;
        loop {
            let prev_req = self.session_max_request_payload_bytes();
            let prev_resp = self.session_max_response_payload_bytes();
            let prev_cfg_req = self.transport_config.max_request_payload_bytes;
            let prev_cfg_resp = self.transport_config.max_response_payload_bytes;

            match self.do_raw_call_from_request_buf(method_code, req_len) {
                Ok(payload) => {
                    self.call_count += 1;
                    return Ok(payload);
                }
                Err(first_err) => {
                    if first_err != NipcError::Overflow {
                        self.disconnect();
                        self.state = ClientState::Broken;
                        self.state = self.try_connect();
                        if self.state != ClientState::Ready {
                            self.error_count += 1;
                            return Err(first_err);
                        }
                        self.reconnect_count += 1;

                        match self.do_raw_call_from_request_buf(method_code, req_len) {
                            Ok(payload) => {
                                self.call_count += 1;
                                return Ok(payload);
                            }
                            Err(retry_err) => {
                                self.disconnect();
                                self.state = ClientState::Broken;
                                self.error_count += 1;
                                return Err(retry_err);
                            }
                        }
                    }

                    self.disconnect();
                    self.state = ClientState::Broken;
                    self.state = self.try_connect();
                    if self.state != ClientState::Ready {
                        self.error_count += 1;
                        return Err(first_err);
                    }
                    self.reconnect_count += 1;

                    if self.session_max_request_payload_bytes() <= prev_req
                        && self.session_max_response_payload_bytes() <= prev_resp
                        && self.transport_config.max_request_payload_bytes <= prev_cfg_req
                        && self.transport_config.max_response_payload_bytes <= prev_cfg_resp
                    {
                        self.disconnect();
                        self.state = ClientState::Broken;
                        self.error_count += 1;
                        return Err(first_err);
                    }

                    overflow_retries += 1;
                    if overflow_retries >= 8 {
                        self.disconnect();
                        self.state = ClientState::Broken;
                        self.error_count += 1;
                        return Err(first_err);
                    }
                }
            }
        }
    }

    fn raw_batch_call_with_retry_request_buf<'a>(
        &mut self,
        method_code: u16,
        req_len: usize,
        item_count: u32,
    ) -> Result<ClientResponseRef, NipcError> {
        if self.state != ClientState::Ready {
            self.error_count += 1;
            return Err(NipcError::BadLayout);
        }

        let mut overflow_retries = 0u32;
        loop {
            let prev_req = self.session_max_request_payload_bytes();
            let prev_resp = self.session_max_response_payload_bytes();
            let prev_cfg_req = self.transport_config.max_request_payload_bytes;
            let prev_cfg_resp = self.transport_config.max_response_payload_bytes;

            match self.do_raw_batch_call_from_request_buf(method_code, req_len, item_count) {
                Ok(payload) => {
                    self.call_count += 1;
                    return Ok(payload);
                }
                Err(first_err) => {
                    if first_err != NipcError::Overflow {
                        self.disconnect();
                        self.state = ClientState::Broken;
                        self.state = self.try_connect();
                        if self.state != ClientState::Ready {
                            self.error_count += 1;
                            return Err(first_err);
                        }
                        self.reconnect_count += 1;

                        match self.do_raw_batch_call_from_request_buf(
                            method_code,
                            req_len,
                            item_count,
                        ) {
                            Ok(payload) => {
                                self.call_count += 1;
                                return Ok(payload);
                            }
                            Err(retry_err) => {
                                self.disconnect();
                                self.state = ClientState::Broken;
                                self.error_count += 1;
                                return Err(retry_err);
                            }
                        }
                    }

                    self.disconnect();
                    self.state = ClientState::Broken;
                    self.state = self.try_connect();
                    if self.state != ClientState::Ready {
                        self.error_count += 1;
                        return Err(first_err);
                    }
                    self.reconnect_count += 1;

                    if self.session_max_request_payload_bytes() <= prev_req
                        && self.session_max_response_payload_bytes() <= prev_resp
                        && self.transport_config.max_request_payload_bytes <= prev_cfg_req
                        && self.transport_config.max_response_payload_bytes <= prev_cfg_resp
                    {
                        self.disconnect();
                        self.state = ClientState::Broken;
                        self.error_count += 1;
                        return Err(first_err);
                    }

                    overflow_retries += 1;
                    if overflow_retries >= 8 {
                        self.disconnect();
                        self.state = ClientState::Broken;
                        self.error_count += 1;
                        return Err(first_err);
                    }
                }
            }
        }
    }

    /// Single attempt at a raw call for any method.
    fn do_raw_call(
        &mut self,
        method_code: u16,
        request_payload: &[u8],
    ) -> Result<ClientResponseRef, NipcError> {
        // 1. Build outer header
        let mut hdr = Header {
            kind: KIND_REQUEST,
            code: method_code,
            flags: 0,
            item_count: 1,
            message_id: (self.call_count as u64) + 1,
            transport_status: STATUS_OK,
            ..Header::default()
        };

        // 2. Send via L1 (SHM or UDS)
        self.transport_send(&mut hdr, request_payload)?;

        // 3. Receive via L1
        let (resp_hdr, response) = self.transport_receive()?;

        // 4. Verify response envelope fields before decode
        if resp_hdr.kind != KIND_RESPONSE {
            return Err(NipcError::BadKind);
        }
        if resp_hdr.code != method_code {
            return Err(NipcError::BadLayout);
        }
        if resp_hdr.message_id != hdr.message_id {
            return Err(NipcError::BadLayout);
        }

        // 5. Check transport_status BEFORE decode (spec requirement)
        match resp_hdr.transport_status {
            STATUS_OK => {}
            STATUS_LIMIT_EXCEEDED => {
                let current = self.session_max_response_payload_bytes();
                if current > 0 {
                    self.client_note_response_capacity(current.saturating_mul(2));
                }
                return Err(NipcError::Overflow);
            }
            _ => return Err(NipcError::BadLayout),
        }
        Ok(response)
    }

    fn do_raw_call_from_request_buf(
        &mut self,
        method_code: u16,
        req_len: usize,
    ) -> Result<ClientResponseRef, NipcError> {
        let mut hdr = Header {
            kind: KIND_REQUEST,
            code: method_code,
            flags: 0,
            item_count: 1,
            message_id: (self.call_count as u64) + 1,
            transport_status: STATUS_OK,
            ..Header::default()
        };

        self.transport_send_request_buf(&mut hdr, req_len)?;
        let (resp_hdr, response) = self.transport_receive()?;

        if resp_hdr.kind != KIND_RESPONSE {
            return Err(NipcError::BadKind);
        }
        if resp_hdr.code != method_code {
            return Err(NipcError::BadLayout);
        }
        if resp_hdr.message_id != hdr.message_id {
            return Err(NipcError::BadLayout);
        }
        match resp_hdr.transport_status {
            STATUS_OK => {}
            STATUS_LIMIT_EXCEEDED => {
                let current = self.session_max_response_payload_bytes();
                if current > 0 {
                    self.client_note_response_capacity(current.saturating_mul(2));
                }
                return Err(NipcError::Overflow);
            }
            _ => return Err(NipcError::BadLayout),
        }
        Ok(response)
    }

    /// Single attempt at a raw batch call. Like `do_raw_call` but sets
    /// FLAG_BATCH and item_count, and validates the response matches.
    fn do_raw_batch_call_from_request_buf(
        &mut self,
        method_code: u16,
        req_len: usize,
        item_count: u32,
    ) -> Result<ClientResponseRef, NipcError> {
        let mut hdr = Header {
            kind: KIND_REQUEST,
            code: method_code,
            flags: FLAG_BATCH,
            item_count,
            message_id: (self.call_count as u64) + 1,
            transport_status: STATUS_OK,
            ..Header::default()
        };

        self.transport_send_request_buf(&mut hdr, req_len)?;

        let (resp_hdr, response) = self.transport_receive()?;

        if resp_hdr.kind != KIND_RESPONSE {
            return Err(NipcError::BadKind);
        }
        if resp_hdr.code != method_code {
            return Err(NipcError::BadLayout);
        }
        if resp_hdr.message_id != hdr.message_id {
            return Err(NipcError::BadLayout);
        }
        match resp_hdr.transport_status {
            STATUS_OK => {}
            STATUS_LIMIT_EXCEEDED => {
                let current = self.session_max_response_payload_bytes();
                if current > 0 {
                    self.client_note_response_capacity(current.saturating_mul(2));
                }
                return Err(NipcError::Overflow);
            }
            _ => return Err(NipcError::BadLayout),
        }
        if resp_hdr.item_count != item_count {
            return Err(NipcError::BadItemCount);
        }
        Ok(response)
    }

    /// Send via the active transport (SHM if available, baseline otherwise).
    fn transport_send(&mut self, hdr: &mut Header, payload: &[u8]) -> Result<(), NipcError> {
        let max_request_payload_bytes = self.session_max_request_payload_bytes();

        // SHM path (POSIX or Windows)
        #[cfg(target_os = "linux")]
        {
            if let Some(ref mut shm) = self.shm {
                if payload.len() > max_request_payload_bytes as usize {
                    self.client_note_request_capacity(payload.len() as u32);
                    return Err(NipcError::Overflow);
                }

                let msg_len = HEADER_SIZE + payload.len();
                let msg = ensure_client_scratch(&mut self.send_buf, msg_len);

                hdr.magic = MAGIC_MSG;
                hdr.version = VERSION;
                hdr.header_len = protocol::HEADER_LEN;
                hdr.payload_len = payload.len() as u32;

                hdr.encode(&mut msg[..HEADER_SIZE]);
                if !payload.is_empty() {
                    msg[HEADER_SIZE..].copy_from_slice(payload);
                }

                let send_result = shm.send(&msg);
                return match send_result {
                    Ok(()) => Ok(()),
                    Err(crate::transport::shm::ShmError::MsgTooLarge) => {
                        self.client_note_request_capacity(payload.len() as u32);
                        Err(NipcError::Overflow)
                    }
                    Err(_) => Err(NipcError::Truncated),
                };
            }
        }

        #[cfg(windows)]
        {
            if let Some(ref mut shm) = self.shm {
                if payload.len() > max_request_payload_bytes as usize {
                    self.client_note_request_capacity(payload.len() as u32);
                    return Err(NipcError::Overflow);
                }

                let msg_len = HEADER_SIZE + payload.len();
                let msg = ensure_client_scratch(&mut self.send_buf, msg_len);

                hdr.magic = MAGIC_MSG;
                hdr.version = VERSION;
                hdr.header_len = protocol::HEADER_LEN;
                hdr.payload_len = payload.len() as u32;

                hdr.encode(&mut msg[..HEADER_SIZE]);
                if !payload.is_empty() {
                    msg[HEADER_SIZE..].copy_from_slice(payload);
                }

                let send_result = shm.send(&msg);
                return match send_result {
                    Ok(()) => Ok(()),
                    Err(crate::transport::win_shm::WinShmError::MsgTooLarge) => {
                        self.client_note_request_capacity(payload.len() as u32);
                        Err(NipcError::Overflow)
                    }
                    Err(_) => Err(NipcError::Truncated),
                };
            }
        }

        // Baseline transport path
        let send_result = {
            let session = self.session.as_mut().ok_or(NipcError::Truncated)?;
            session.send(hdr, payload)
        };
        match send_result {
            Ok(()) => Ok(()),
            #[cfg(unix)]
            Err(crate::transport::posix::UdsError::LimitExceeded) => {
                self.client_note_request_capacity(payload.len() as u32);
                Err(NipcError::Overflow)
            }
            #[cfg(windows)]
            Err(crate::transport::windows::NpError::LimitExceeded) => {
                self.client_note_request_capacity(payload.len() as u32);
                Err(NipcError::Overflow)
            }
            Err(_) => Err(NipcError::Truncated),
        }
    }

    fn transport_send_request_buf(
        &mut self,
        hdr: &mut Header,
        req_len: usize,
    ) -> Result<(), NipcError> {
        let max_request_payload_bytes = self.session_max_request_payload_bytes();

        #[cfg(target_os = "linux")]
        {
            if let Some(ref mut shm) = self.shm {
                if req_len > max_request_payload_bytes as usize {
                    self.client_note_request_capacity(req_len as u32);
                    return Err(NipcError::Overflow);
                }

                let msg_len = HEADER_SIZE + req_len;
                let msg = ensure_client_scratch(&mut self.send_buf, msg_len);

                hdr.magic = MAGIC_MSG;
                hdr.version = VERSION;
                hdr.header_len = protocol::HEADER_LEN;
                hdr.payload_len = req_len as u32;

                hdr.encode(&mut msg[..HEADER_SIZE]);
                if req_len > 0 {
                    msg[HEADER_SIZE..HEADER_SIZE + req_len]
                        .copy_from_slice(&self.request_buf[..req_len]);
                }

                let send_result = shm.send(&msg[..msg_len]);
                return match send_result {
                    Ok(()) => Ok(()),
                    Err(crate::transport::shm::ShmError::MsgTooLarge) => {
                        self.client_note_request_capacity(req_len as u32);
                        Err(NipcError::Overflow)
                    }
                    Err(_) => Err(NipcError::Truncated),
                };
            }
        }

        #[cfg(windows)]
        {
            if let Some(ref mut shm) = self.shm {
                if req_len > max_request_payload_bytes as usize {
                    self.client_note_request_capacity(req_len as u32);
                    return Err(NipcError::Overflow);
                }

                let msg_len = HEADER_SIZE + req_len;
                let msg = ensure_client_scratch(&mut self.send_buf, msg_len);

                hdr.magic = MAGIC_MSG;
                hdr.version = VERSION;
                hdr.header_len = protocol::HEADER_LEN;
                hdr.payload_len = req_len as u32;

                hdr.encode(&mut msg[..HEADER_SIZE]);
                if req_len > 0 {
                    msg[HEADER_SIZE..HEADER_SIZE + req_len]
                        .copy_from_slice(&self.request_buf[..req_len]);
                }

                let send_result = shm.send(&msg[..msg_len]);
                return match send_result {
                    Ok(()) => Ok(()),
                    Err(crate::transport::win_shm::WinShmError::MsgTooLarge) => {
                        self.client_note_request_capacity(req_len as u32);
                        Err(NipcError::Overflow)
                    }
                    Err(_) => Err(NipcError::Truncated),
                };
            }
        }

        let send_result = {
            let session = self.session.as_mut().ok_or(NipcError::Truncated)?;
            session.send(hdr, &self.request_buf[..req_len])
        };
        match send_result {
            Ok(()) => Ok(()),
            #[cfg(unix)]
            Err(crate::transport::posix::UdsError::LimitExceeded) => {
                self.client_note_request_capacity(req_len as u32);
                Err(NipcError::Overflow)
            }
            #[cfg(windows)]
            Err(crate::transport::windows::NpError::LimitExceeded) => {
                self.client_note_request_capacity(req_len as u32);
                Err(NipcError::Overflow)
            }
            Err(_) => Err(NipcError::Truncated),
        }
    }

    /// Receive via the active transport. Returns (header, payload_view).
    fn transport_receive(&mut self) -> Result<(Header, ClientResponseRef), NipcError> {
        let needed = self.max_receive_message_bytes();
        let scratch = ensure_client_scratch(&mut self.transport_buf, needed);

        // SHM path (POSIX or Windows)
        #[cfg(target_os = "linux")]
        {
            if let Some(ref mut shm) = self.shm {
                let mlen = shm
                    .receive(scratch, 30000)
                    .map_err(|_| NipcError::Truncated)?;

                if mlen < HEADER_SIZE {
                    return Err(NipcError::Truncated);
                }

                let hdr = Header::decode(&scratch[..mlen])?;
                return Ok((
                    hdr,
                    ClientResponseRef {
                        source: ClientResponseSource::TransportBuf,
                        len: mlen - HEADER_SIZE,
                    },
                ));
            }
        }

        #[cfg(windows)]
        {
            if let Some(ref mut shm) = self.shm {
                let mlen = shm
                    .receive(scratch, 30000)
                    .map_err(|_| NipcError::Truncated)?;

                if mlen < HEADER_SIZE {
                    return Err(NipcError::Truncated);
                }

                let hdr = Header::decode(&scratch[..mlen])?;
                return Ok((
                    hdr,
                    ClientResponseRef {
                        source: ClientResponseSource::TransportBuf,
                        len: mlen - HEADER_SIZE,
                    },
                ));
            }
        }

        // Baseline transport: UDS on POSIX, Named Pipe on Windows
        let session = self.session.as_mut().ok_or(NipcError::Truncated)?;

        #[cfg(unix)]
        {
            let scratch_payload_ptr = unsafe { scratch.as_ptr().add(HEADER_SIZE) };
            let (hdr, payload) = session.receive(scratch).map_err(|_| NipcError::Truncated)?;
            let source = if payload.as_ptr() == scratch_payload_ptr {
                ClientResponseSource::TransportBuf
            } else {
                ClientResponseSource::SessionBuf
            };
            Ok((
                hdr,
                ClientResponseRef {
                    source,
                    len: payload.len(),
                },
            ))
        }

        #[cfg(windows)]
        {
            let scratch_payload_ptr = unsafe { scratch.as_ptr().add(HEADER_SIZE) };
            let (hdr, payload) = session.receive(scratch).map_err(|_| NipcError::Truncated)?;
            let source = if payload.as_ptr() == scratch_payload_ptr {
                ClientResponseSource::TransportBuf
            } else {
                ClientResponseSource::SessionBuf
            };
            Ok((
                hdr,
                ClientResponseRef {
                    source,
                    len: payload.len(),
                },
            ))
        }
    }

    fn response_payload(&self, response: ClientResponseRef) -> Result<&[u8], NipcError> {
        match response.source {
            ClientResponseSource::TransportBuf => {
                let start = HEADER_SIZE;
                let end = HEADER_SIZE + response.len;
                if end > self.transport_buf.len() {
                    return Err(NipcError::Truncated);
                }
                Ok(&self.transport_buf[start..end])
            }
            ClientResponseSource::SessionBuf => {
                #[cfg(unix)]
                {
                    let session = self.session.as_ref().ok_or(NipcError::Truncated)?;
                    return Ok(session.received_payload(response.len));
                }
                #[cfg(windows)]
                {
                    let session = self.session.as_ref().ok_or(NipcError::Truncated)?;
                    return Ok(session.received_payload(response.len));
                }
                #[allow(unreachable_code)]
                Err(NipcError::Truncated)
            }
        }
    }

    fn max_receive_message_bytes(&self) -> usize {
        let mut max_payload = self.transport_config.max_response_payload_bytes as usize;
        #[cfg(unix)]
        if let Some(ref session) = self.session {
            if session.max_response_payload_bytes > 0 {
                max_payload = session.max_response_payload_bytes as usize;
            }
        }
        #[cfg(windows)]
        if let Some(ref session) = self.session {
            if session.max_response_payload_bytes > 0 {
                max_payload = session.max_response_payload_bytes as usize;
            }
        }
        if max_payload == 0 {
            max_payload = CACHE_RESPONSE_BUF_SIZE;
        }
        HEADER_SIZE + max_payload
    }
}

impl Drop for RawClient {
    fn drop(&mut self) {
        self.close();
    }
}

fn ensure_client_scratch(buf: &mut Vec<u8>, needed: usize) -> &mut [u8] {
    if buf.len() < needed {
        buf.resize(needed, 0);
    }
    &mut buf[..needed]
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
enum ClientResponseSource {
    TransportBuf,
    SessionBuf,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
struct ClientResponseRef {
    source: ClientResponseSource,
    len: usize,
}

fn dispatch_single_internal(
    expected_method_code: u16,
    handler: Option<&DispatchHandler>,
    method_code: u16,
    request: &[u8],
    response_buf: &mut [u8],
) -> Result<usize, DispatchError> {
    if method_code != expected_method_code {
        return Err(DispatchError::HandlerFailed);
    }

    match handler {
        Some(dispatch) => match dispatch(request, response_buf) {
            Ok(n) if n <= response_buf.len() => Ok(n),
            Ok(_) => Err(DispatchError::Overflow),
            Err(err) => Err(err),
        },
        None => Err(DispatchError::HandlerFailed),
    }
}

#[cfg(test)]
#[allow(dead_code)]
fn dispatch_single(
    expected_method_code: u16,
    handler: Option<&DispatchHandler>,
    method_code: u16,
    request: &[u8],
    response_buf: &mut [u8],
) -> Result<usize, DispatchError> {
    dispatch_single_internal(
        expected_method_code,
        handler,
        method_code,
        request,
        response_buf,
    )
}

fn method_supported_internal(
    expected_method_code: u16,
    handler: Option<&DispatchHandler>,
    method_code: u16,
) -> bool {
    handler.is_some() && method_code == expected_method_code
}

fn server_note_payload_capacity(target: &AtomicU32, payload_len: u32) {
    let grown = next_power_of_2_u32(payload_len);
    let mut current = target.load(Ordering::Relaxed);
    while grown > current {
        match target.compare_exchange_weak(current, grown, Ordering::Release, Ordering::Relaxed) {
            Ok(_) => break,
            Err(observed) => current = observed,
        }
    }
}

// ---------------------------------------------------------------------------
//  Managed server
// ---------------------------------------------------------------------------

pub type IncrementHandler = Arc<dyn Fn(u64) -> Option<u64> + Send + Sync>;
pub type StringReverseHandler = Arc<dyn Fn(&str) -> Option<String> + Send + Sync>;
pub type SnapshotHandler =
    Arc<dyn for<'a> Fn(&CgroupsRequest, &mut protocol::CgroupsBuilder<'a>) -> bool + Send + Sync>;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum DispatchError {
    BadEnvelope,
    Overflow,
    HandlerFailed,
}

pub type DispatchHandler =
    Arc<dyn Fn(&[u8], &mut [u8]) -> Result<usize, DispatchError> + Send + Sync>;

pub fn increment_dispatch(handler: IncrementHandler) -> DispatchHandler {
    Arc::new(move |request, response_buf| {
        let value = increment_decode(request).map_err(|_| DispatchError::BadEnvelope)?;
        let result = handler(value).ok_or(DispatchError::HandlerFailed)?;
        let n = increment_encode(result, response_buf);
        if n == 0 {
            return Err(DispatchError::Overflow);
        }
        Ok(n)
    })
}

pub fn string_reverse_dispatch(handler: StringReverseHandler) -> DispatchHandler {
    Arc::new(move |request, response_buf| {
        let view = string_reverse_decode(request).map_err(|_| DispatchError::BadEnvelope)?;
        let result = handler(view.as_str()).ok_or(DispatchError::HandlerFailed)?;
        let n = string_reverse_encode(result.as_bytes(), response_buf);
        if n == 0 {
            return Err(DispatchError::Overflow);
        }
        Ok(n)
    })
}

pub fn snapshot_max_items(response_buf_size: usize, override_max_items: u32) -> u32 {
    if override_max_items != 0 {
        return override_max_items;
    }
    protocol::estimate_cgroups_max_items(response_buf_size)
}

pub fn snapshot_dispatch(handler: SnapshotHandler, max_items: u32) -> DispatchHandler {
    Arc::new(move |request, response_buf| {
        let request = CgroupsRequest::decode(request).map_err(|_| DispatchError::BadEnvelope)?;
        let item_budget = snapshot_max_items(response_buf.len(), max_items);
        if item_budget == 0 {
            return Err(DispatchError::Overflow);
        }
        let mut builder = protocol::CgroupsBuilder::new(response_buf, item_budget, 0, 0);
        if !handler(&request, &mut builder) {
            return Err(DispatchError::HandlerFailed);
        }
        let n = builder.finish();
        if n == 0 {
            return Err(DispatchError::Overflow);
        }
        Ok(n)
    })
}

/// L2 managed server. Typed request/response dispatcher.
///
/// Handles accept, spawns a thread per session (up to worker_count),
/// reads requests, dispatches to handler, sends responses.
pub struct ManagedServer {
    run_dir: String,
    service_name: String,
    server_config: ServerConfig,
    expected_method_code: u16,
    handler: Option<DispatchHandler>,
    running: Arc<AtomicBool>,
    learned_request_payload_bytes: Arc<AtomicU32>,
    learned_response_payload_bytes: Arc<AtomicU32>,
    next_session_id: u64,
    worker_count: usize,
    /// Windows: stored listener handle so stop() can close it to unblock Accept.
    #[cfg(windows)]
    listener_handle: Arc<std::sync::Mutex<Option<usize>>>,
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

        ManagedServer {
            run_dir: run_dir.to_string(),
            service_name: service_name.to_string(),
            server_config: config,
            expected_method_code,
            handler,
            running: Arc::new(AtomicBool::new(false)),
            learned_request_payload_bytes: Arc::new(AtomicU32::new(learned_request)),
            learned_response_payload_bytes: Arc::new(AtomicU32::new(learned_response)),
            next_session_id: 1,
            worker_count: if worker_count < 1 { 1 } else { worker_count },
            #[cfg(windows)]
            listener_handle: Arc::new(std::sync::Mutex::new(None)),
        }
    }

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
                // Reap finished threads periodically
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

            // Check worker count limit (non-blocking)
            // Reap finished threads first
            session_threads.retain(|t| !t.is_finished());
            if session_threads.len() >= self.worker_count {
                // At capacity: reject client
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

            // Spawn a handler thread for this session
            let expected_method_code = self.expected_method_code;
            let handler = self.handler.clone();
            let running = self.running.clone();
            let learned_request_payload_bytes = self.learned_request_payload_bytes.clone();
            let learned_response_payload_bytes = self.learned_response_payload_bytes.clone();

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
                );
            });
            session_threads.push(t);
        }

        // Wait for all active session threads
        for t in session_threads {
            let _ = t.join();
        }

        Ok(())
    }

    /// Windows: run the acceptor loop over Named Pipes.
    #[cfg(windows)]
    pub fn run(&mut self) -> Result<(), NipcError> {
        // Win SHM cleanup is a no-op: kernel objects auto-clean on handle close.

        let mut listener = NpListener::bind(
            &self.run_dir,
            &self.service_name,
            self.server_config.clone(),
        )
        .map_err(|_| NipcError::BadLayout)?;

        // Store listener handle so stop() can close it to unblock Accept
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

            // Reap finished threads
            session_threads.retain(|t| !t.is_finished());
            if session_threads.len() >= self.worker_count {
                if let Some(mut prepared) = prepared_shm {
                    prepared.destroy_all();
                }
                drop(session);
                continue;
            }

            let shm = match self.finalize_windows_shm(&session, prepared_shm) {
                Some(shm) => Some(shm),
                None if session.selected_profile == WIN_SHM_PROFILE_HYBRID
                    || session.selected_profile == WIN_SHM_PROFILE_BUSYWAIT =>
                {
                    drop(session);
                    continue;
                }
                None => None,
            };

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

    /// Signal shutdown. On Windows, also closes the listener pipe to
    /// unblock ConnectNamedPipe in the accept loop.
    pub fn stop(&self) {
        self.running.store(false, Ordering::Release);

        #[cfg(windows)]
        {
            let mut guard = self.listener_handle.lock().unwrap();
            if let Some(h) = guard.take() {
                // Close the listener pipe to unblock ConnectNamedPipe
                extern "system" {
                    fn CloseHandle(h: isize) -> i32;
                }
                unsafe {
                    CloseHandle(h as isize);
                }
            }
        }
    }

    /// Returns the internal running flag for diagnostics and test helpers.
    ///
    /// For reliable shutdown, call `stop()`. On Windows, flipping this flag
    /// alone does not wake a blocking listener accept.
    pub fn running_flag(&self) -> Arc<AtomicBool> {
        self.running.clone()
    }

    // ------------------------------------------------------------------
    //  Internal helpers
    // ------------------------------------------------------------------

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
        session: &UdsSession,
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

    #[cfg(windows)]
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

    #[cfg(windows)]
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
        let selected = prepared.take(profile);
        prepared.destroy_all();
        selected
    }
}

#[cfg(windows)]
#[derive(Default)]
struct PreparedWinShm {
    hybrid: Option<WinShmContext>,
    busywait: Option<WinShmContext>,
}

#[cfg(windows)]
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

/// Windows: handle one client session over Named Pipe + optional Win SHM.
/// Standalone function for use in per-session threads.
#[cfg(windows)]
fn handle_session_win_threaded(
    mut session: NpSession,
    mut shm: Option<WinShmContext>,
    expected_method_code: u16,
    handler: Option<DispatchHandler>,
    running: Arc<AtomicBool>,
    learned_request_payload_bytes: Arc<AtomicU32>,
    learned_response_payload_bytes: Arc<AtomicU32>,
) {
    let mut recv_buf = vec![0u8; HEADER_SIZE + session.max_request_payload_bytes as usize];
    let mut resp_buf = vec![0u8; session.max_response_payload_bytes as usize];
    let mut item_resp_buf = vec![0u8; session.max_response_payload_bytes as usize];
    let mut msg_buf = vec![0u8; HEADER_SIZE + session.max_response_payload_bytes as usize];

    while running.load(Ordering::Acquire) {
        let (hdr, payload) = {
            if let Some(ref mut shm_ctx) = shm {
                match shm_ctx.receive(&mut recv_buf, SERVER_POLL_TIMEOUT_MS) {
                    Ok(mlen) => {
                        if mlen < HEADER_SIZE {
                            break;
                        }
                        let hdr = match Header::decode(&recv_buf[..mlen]) {
                            Ok(h) => h,
                            Err(_) => break,
                        };
                        let payload = &recv_buf[HEADER_SIZE..mlen];
                        (hdr, payload)
                    }
                    Err(crate::transport::win_shm::WinShmError::Timeout) => continue,
                    Err(_) => break,
                }
            } else {
                // Named Pipe path
                match session.wait_readable(SERVER_POLL_TIMEOUT_MS) {
                    Ok(true) => {}
                    Ok(false) => continue,
                    Err(_) => break,
                }
                match session.receive(&mut recv_buf) {
                    Ok((hdr, payload)) => (hdr, payload),
                    Err(_) => break,
                }
            }
        };

        // Protocol violation: unexpected message kind terminates session
        if hdr.kind != KIND_REQUEST {
            break;
        }

        if payload.len() <= u32::MAX as usize {
            server_note_payload_capacity(&learned_request_payload_bytes, payload.len() as u32);
        }

        if !method_supported_internal(expected_method_code, handler.as_ref(), hdr.code) {
            let mut resp_hdr = Header {
                kind: KIND_RESPONSE,
                code: hdr.code,
                message_id: hdr.message_id,
                transport_status: protocol::STATUS_UNSUPPORTED,
                item_count: 1,
                ..Header::default()
            };

            if let Some(ref mut shm_ctx) = shm {
                let msg = ensure_client_scratch(&mut msg_buf, HEADER_SIZE);
                resp_hdr.magic = MAGIC_MSG;
                resp_hdr.version = VERSION;
                resp_hdr.header_len = protocol::HEADER_LEN;
                resp_hdr.payload_len = 0;
                resp_hdr.encode(&mut msg[..HEADER_SIZE]);
                if shm_ctx.send(&msg[..HEADER_SIZE]).is_err() {
                    break;
                }
            } else if session.send(&mut resp_hdr, &[]).is_err() {
                break;
            }
            continue;
        }

        // Dispatch: single-item or batch
        let is_batch = (hdr.flags & FLAG_BATCH) != 0 && hdr.item_count >= 1;
        let response_len;
        let dispatch_result = if !is_batch {
            dispatch_single_internal(
                expected_method_code,
                handler.as_ref(),
                hdr.code,
                payload,
                &mut resp_buf,
            )
        } else {
            let mut bb = BatchBuilder::new(&mut resp_buf, hdr.item_count);
            let mut batch_result = Ok(0usize);

            for i in 0..hdr.item_count {
                let (item_data, _item_len) = match batch_item_get(payload, hdr.item_count, i) {
                    Ok(v) => v,
                    Err(_) => {
                        batch_result = Err(DispatchError::BadEnvelope);
                        break;
                    }
                };
                let item_len = match dispatch_single_internal(
                    expected_method_code,
                    handler.as_ref(),
                    hdr.code,
                    item_data,
                    &mut item_resp_buf,
                ) {
                    Ok(n) => n,
                    Err(err) => {
                        batch_result = Err(err);
                        break;
                    }
                };
                if bb.add(&item_resp_buf[..item_len]).is_err() {
                    batch_result = Err(DispatchError::Overflow);
                    break;
                }
            }
            if batch_result.is_ok() {
                let (n, _) = bb.finish();
                batch_result = Ok(n);
            }
            batch_result
        };

        let mut resp_hdr = Header {
            kind: KIND_RESPONSE,
            code: hdr.code,
            message_id: hdr.message_id,
            ..Header::default()
        };

        match dispatch_result {
            Ok(n) => {
                response_len = n;
                if response_len <= u32::MAX as usize {
                    server_note_payload_capacity(
                        &learned_response_payload_bytes,
                        response_len as u32,
                    );
                }
                resp_hdr.transport_status = STATUS_OK;
                if is_batch {
                    resp_hdr.flags = FLAG_BATCH;
                    resp_hdr.item_count = hdr.item_count;
                } else {
                    resp_hdr.flags = 0;
                    resp_hdr.item_count = 1;
                }
            }
            Err(DispatchError::Overflow) => {
                let current = session.max_response_payload_bytes;
                if current >= u32::MAX / 2 {
                    server_note_payload_capacity(&learned_response_payload_bytes, u32::MAX);
                } else {
                    server_note_payload_capacity(&learned_response_payload_bytes, current * 2);
                }
                resp_hdr.transport_status = STATUS_LIMIT_EXCEEDED;
                resp_hdr.item_count = 1;
                resp_hdr.flags = 0;
                response_len = 0;
            }
            Err(DispatchError::BadEnvelope) => {
                resp_hdr.transport_status = STATUS_BAD_ENVELOPE;
                resp_hdr.item_count = 1;
                resp_hdr.flags = 0;
                response_len = 0;
            }
            Err(DispatchError::HandlerFailed) => {
                resp_hdr.transport_status = STATUS_INTERNAL_ERROR;
                resp_hdr.item_count = 1;
                resp_hdr.flags = 0;
                response_len = 0;
            }
        }

        if let Some(ref mut shm_ctx) = shm {
            let msg_len = HEADER_SIZE + response_len;
            let msg = ensure_client_scratch(&mut msg_buf, msg_len);

            resp_hdr.magic = MAGIC_MSG;
            resp_hdr.version = VERSION;
            resp_hdr.header_len = protocol::HEADER_LEN;
            resp_hdr.payload_len = response_len as u32;

            resp_hdr.encode(&mut msg[..HEADER_SIZE]);
            if response_len > 0 {
                msg[HEADER_SIZE..].copy_from_slice(&resp_buf[..response_len]);
            }

            if shm_ctx.send(msg).is_err() {
                break;
            }
            if resp_hdr.transport_status == STATUS_LIMIT_EXCEEDED {
                break;
            }
            continue;
        }

        if session
            .send(&mut resp_hdr, &resp_buf[..response_len])
            .is_err()
        {
            break;
        }
        if resp_hdr.transport_status == STATUS_LIMIT_EXCEEDED {
            break;
        }
    }

    if let Some(mut shm_ctx) = shm {
        shm_ctx.destroy();
    }
    session.close();
}

/// POSIX: Handle one client session in its own thread.
#[cfg(unix)]
fn handle_session_threaded(
    mut session: UdsSession,
    #[cfg(target_os = "linux")] mut shm: Option<ShmContext>,
    #[cfg(not(target_os = "linux"))] _shm: Option<()>,
    expected_method_code: u16,
    handler: Option<DispatchHandler>,
    running: Arc<AtomicBool>,
    learned_request_payload_bytes: Arc<AtomicU32>,
    learned_response_payload_bytes: Arc<AtomicU32>,
) {
    let mut recv_buf = vec![0u8; HEADER_SIZE + session.max_request_payload_bytes as usize];
    let mut resp_buf = vec![0u8; session.max_response_payload_bytes as usize];
    let mut item_resp_buf = vec![0u8; session.max_response_payload_bytes as usize];
    let mut msg_buf = vec![0u8; HEADER_SIZE + session.max_response_payload_bytes as usize];

    while running.load(Ordering::Acquire) {
        // Receive request via the active transport
        let (hdr, payload) = {
            #[cfg(target_os = "linux")]
            {
                if let Some(ref mut shm_ctx) = shm {
                    match shm_ctx.receive(&mut recv_buf, SERVER_POLL_TIMEOUT_MS) {
                        Ok(mlen) => {
                            if mlen < HEADER_SIZE {
                                break;
                            }
                            let hdr = match Header::decode(&recv_buf[..mlen]) {
                                Ok(h) => h,
                                Err(_) => break,
                            };
                            let payload = &recv_buf[HEADER_SIZE..mlen];
                            (hdr, payload)
                        }
                        Err(crate::transport::shm::ShmError::Timeout) => continue,
                        Err(_) => break,
                    }
                } else {
                    // UDS path with poll
                    let ready = poll_fd(session.fd(), SERVER_POLL_TIMEOUT_MS as i32);
                    if ready < 0 {
                        break;
                    }
                    if ready == 0 {
                        continue;
                    }

                    match session.receive(&mut recv_buf) {
                        Ok((hdr, payload)) => (hdr, payload),
                        Err(_) => break,
                    }
                }
            }

            #[cfg(not(target_os = "linux"))]
            {
                let ready = poll_fd(session.fd(), SERVER_POLL_TIMEOUT_MS as i32);
                if ready < 0 {
                    break;
                }
                if ready == 0 {
                    continue;
                }

                match session.receive(&mut recv_buf) {
                    Ok((hdr, payload)) => (hdr, payload),
                    Err(_) => break,
                }
            }
        };

        // Protocol violation: unexpected message kind terminates session
        if hdr.kind != KIND_REQUEST {
            break;
        }

        if payload.len() <= u32::MAX as usize {
            server_note_payload_capacity(&learned_request_payload_bytes, payload.len() as u32);
        }

        if !method_supported_internal(expected_method_code, handler.as_ref(), hdr.code) {
            let mut resp_hdr = Header {
                kind: KIND_RESPONSE,
                code: hdr.code,
                message_id: hdr.message_id,
                transport_status: protocol::STATUS_UNSUPPORTED,
                item_count: 1,
                ..Header::default()
            };

            #[cfg(target_os = "linux")]
            {
                if let Some(ref mut shm_ctx) = shm {
                    let msg = ensure_client_scratch(&mut msg_buf, HEADER_SIZE);
                    resp_hdr.magic = MAGIC_MSG;
                    resp_hdr.version = VERSION;
                    resp_hdr.header_len = protocol::HEADER_LEN;
                    resp_hdr.payload_len = 0;
                    resp_hdr.encode(&mut msg[..HEADER_SIZE]);
                    if shm_ctx.send(&msg[..HEADER_SIZE]).is_err() {
                        break;
                    }
                    continue;
                }
            }

            if session.send(&mut resp_hdr, &[]).is_err() {
                break;
            }
            continue;
        }

        // Dispatch: single-item or batch
        let is_batch = (hdr.flags & FLAG_BATCH) != 0 && hdr.item_count >= 1;
        let response_len;
        let dispatch_result = if !is_batch {
            dispatch_single_internal(
                expected_method_code,
                handler.as_ref(),
                hdr.code,
                payload,
                &mut resp_buf,
            )
        } else {
            let mut bb = BatchBuilder::new(&mut resp_buf, hdr.item_count);
            let mut batch_result = Ok(0usize);

            for i in 0..hdr.item_count {
                let (item_data, _item_len) = match batch_item_get(payload, hdr.item_count, i) {
                    Ok(v) => v,
                    Err(_) => {
                        batch_result = Err(DispatchError::BadEnvelope);
                        break;
                    }
                };
                let item_len = match dispatch_single_internal(
                    expected_method_code,
                    handler.as_ref(),
                    hdr.code,
                    item_data,
                    &mut item_resp_buf,
                ) {
                    Ok(n) => n,
                    Err(err) => {
                        batch_result = Err(err);
                        break;
                    }
                };
                if bb.add(&item_resp_buf[..item_len]).is_err() {
                    batch_result = Err(DispatchError::Overflow);
                    break;
                }
            }
            if batch_result.is_ok() {
                let (n, _) = bb.finish();
                batch_result = Ok(n);
            }
            batch_result
        };

        // Build response header
        let mut resp_hdr = Header {
            kind: KIND_RESPONSE,
            code: hdr.code,
            message_id: hdr.message_id,
            ..Header::default()
        };

        match dispatch_result {
            Ok(n) => {
                response_len = n;
                if response_len <= u32::MAX as usize {
                    server_note_payload_capacity(
                        &learned_response_payload_bytes,
                        response_len as u32,
                    );
                }
                resp_hdr.transport_status = STATUS_OK;
                if is_batch {
                    resp_hdr.flags = FLAG_BATCH;
                    resp_hdr.item_count = hdr.item_count;
                } else {
                    resp_hdr.flags = 0;
                    resp_hdr.item_count = 1;
                }
            }
            Err(DispatchError::Overflow) => {
                let current = session.max_response_payload_bytes;
                if current >= u32::MAX / 2 {
                    server_note_payload_capacity(&learned_response_payload_bytes, u32::MAX);
                } else {
                    server_note_payload_capacity(&learned_response_payload_bytes, current * 2);
                }
                resp_hdr.transport_status = STATUS_LIMIT_EXCEEDED;
                resp_hdr.item_count = 1;
                resp_hdr.flags = 0;
                response_len = 0;
            }
            Err(DispatchError::BadEnvelope) => {
                resp_hdr.transport_status = STATUS_BAD_ENVELOPE;
                resp_hdr.item_count = 1;
                resp_hdr.flags = 0;
                response_len = 0;
            }
            Err(DispatchError::HandlerFailed) => {
                resp_hdr.transport_status = STATUS_INTERNAL_ERROR;
                resp_hdr.item_count = 1;
                resp_hdr.flags = 0;
                response_len = 0;
            }
        }

        // Send response via the active transport
        #[cfg(target_os = "linux")]
        {
            if let Some(ref mut shm_ctx) = shm {
                let msg_len = HEADER_SIZE + response_len;
                let msg = ensure_client_scratch(&mut msg_buf, msg_len);

                resp_hdr.magic = MAGIC_MSG;
                resp_hdr.version = VERSION;
                resp_hdr.header_len = protocol::HEADER_LEN;
                resp_hdr.payload_len = response_len as u32;

                resp_hdr.encode(&mut msg[..HEADER_SIZE]);
                if response_len > 0 {
                    msg[HEADER_SIZE..].copy_from_slice(&resp_buf[..response_len]);
                }

                if shm_ctx.send(msg).is_err() {
                    break;
                }
                if resp_hdr.transport_status == STATUS_LIMIT_EXCEEDED {
                    break;
                }
                continue;
            }
        }

        // UDS path
        if session
            .send(&mut resp_hdr, &resp_buf[..response_len])
            .is_err()
        {
            break;
        }
        if resp_hdr.transport_status == STATUS_LIMIT_EXCEEDED {
            break;
        }
    }

    // Cleanup
    #[cfg(target_os = "linux")]
    {
        if let Some(mut shm_ctx) = shm {
            shm_ctx.destroy();
        }
    }
    drop(session);
}

// ---------------------------------------------------------------------------
//  Internal: poll helper
// ---------------------------------------------------------------------------

/// Poll a file descriptor for readability with a timeout in milliseconds.
/// Returns: 1 = data ready, 0 = timeout, -1 = error/hangup.
#[cfg(unix)]
fn poll_fd(fd: i32, timeout_ms: i32) -> i32 {
    let mut pfd = libc::pollfd {
        fd,
        events: libc::POLLIN,
        revents: 0,
    };

    let ret = unsafe { libc::poll(&mut pfd, 1, timeout_ms) };

    if ret < 0 {
        let errno = unsafe { *libc::__errno_location() };
        if errno == libc::EINTR {
            return 0;
        }
        return -1;
    }

    if ret == 0 {
        return 0;
    }

    if pfd.revents & (libc::POLLERR | libc::POLLHUP | libc::POLLNVAL) != 0 {
        return -1;
    }

    if pfd.revents & libc::POLLIN != 0 {
        return 1;
    }

    0
}

// ---------------------------------------------------------------------------
//  L3: Client-side cgroups snapshot cache
// ---------------------------------------------------------------------------

/// Cached copy of a single cgroup item. Owns its strings.
/// Built from ephemeral L2 views during cache construction.
#[derive(Debug, Clone)]
pub struct CgroupsCacheItem {
    pub hash: u32,
    pub options: u32,
    pub enabled: u32,
    pub name: String,
    pub path: String,
}

/// L3 cache status snapshot (for diagnostics, not hot path).
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct CgroupsCacheStatus {
    pub populated: bool,
    pub item_count: u32,
    pub systemd_enabled: u32,
    pub generation: u64,
    pub refresh_success_count: u32,
    pub refresh_failure_count: u32,
    pub connection_state: ClientState,
    /// Monotonic milliseconds of last successful refresh (0 if never).
    pub last_refresh_ts: u64,
}

/// Default response buffer size for L3 cache refresh.
const CACHE_RESPONSE_BUF_SIZE: usize = 65536;

#[derive(Debug, Clone, Copy, Default)]
struct CgroupsHashBucket {
    index: u32,
    used: bool,
}

fn cache_hash_name(name: &str) -> u32 {
    let mut h: u32 = 5381;
    for b in name.as_bytes() {
        h = ((h << 5).wrapping_add(h)).wrapping_add(*b as u32);
    }
    h
}

/// L3 client-side cgroups snapshot cache.
///
/// Wraps an L2 client and maintains a local owned copy of the most
/// recent successful snapshot. Lookup by hash+name is O(1) via HashMap.
///
/// On refresh failure, the previous cache is preserved. The cache
/// is empty only if no successful refresh has ever occurred.
pub struct CgroupsCache {
    client: RawClient,
    items: Vec<CgroupsCacheItem>,
    /// Open-addressing hash table: (hash ^ djb2(name)) -> index into items vec
    buckets: Vec<CgroupsHashBucket>,
    systemd_enabled: u32,
    generation: u64,
    populated: bool,
    refresh_success_count: u32,
    refresh_failure_count: u32,
    /// Monotonic reference point for timestamp calculation
    epoch: std::time::Instant,
    /// Monotonic ms of last successful refresh (0 if never)
    pub last_refresh_ts: u64,
}

impl CgroupsCache {
    /// Create a new L3 cache. Creates the underlying L2 client context.
    /// Does NOT connect. Does NOT require the server to be running.
    /// Cache starts empty (populated == false).
    pub fn new(run_dir: &str, service_name: &str, config: ClientConfig) -> Self {
        CgroupsCache {
            client: RawClient::new_snapshot(run_dir, service_name, config),
            items: Vec::new(),
            buckets: Vec::new(),
            systemd_enabled: 0,
            generation: 0,
            populated: false,
            refresh_success_count: 0,
            refresh_failure_count: 0,
            epoch: std::time::Instant::now(),
            last_refresh_ts: 0,
        }
    }

    /// Refresh the cache. Drives the L2 client (connect/reconnect as
    /// needed) and requests a fresh snapshot. On success, rebuilds the
    /// local cache. On failure, preserves the previous cache.
    ///
    /// Returns true if the cache was updated.
    pub fn refresh(&mut self) -> bool {
        // Drive L2 connection lifecycle
        self.client.refresh();

        // Attempt snapshot call
        match self.client.call_snapshot() {
            Ok(view) => {
                // Build new cache from snapshot view
                let mut new_items = Vec::with_capacity(view.item_count as usize);
                for i in 0..view.item_count {
                    match view.item(i) {
                        Ok(iv) => {
                            let name = match iv.name.as_str() {
                                Ok(s) => s.to_string(),
                                Err(_) => {
                                    // Non-UTF8 name: use lossy conversion
                                    String::from_utf8_lossy(iv.name.as_bytes()).into_owned()
                                }
                            };
                            let path = match iv.path.as_str() {
                                Ok(s) => s.to_string(),
                                Err(_) => String::from_utf8_lossy(iv.path.as_bytes()).into_owned(),
                            };
                            new_items.push(CgroupsCacheItem {
                                hash: iv.hash,
                                options: iv.options,
                                enabled: iv.enabled,
                                name,
                                path,
                            });
                        }
                        Err(_) => {
                            // Malformed item: abort, preserve old cache
                            self.refresh_failure_count += 1;
                            return false;
                        }
                    }
                }

                // Rebuild open-addressing lookup table.
                let mut buckets = Vec::new();
                if !new_items.is_empty() {
                    let bcount = next_power_of_2_u32((new_items.len() as u32) * 2) as usize;
                    buckets.resize(bcount, CgroupsHashBucket::default());
                    let mask = (bcount - 1) as u32;
                    for (i, item) in new_items.iter().enumerate() {
                        let mut slot = (item.hash ^ cache_hash_name(&item.name)) & mask;
                        while buckets[slot as usize].used {
                            slot = (slot + 1) & mask;
                        }
                        buckets[slot as usize] = CgroupsHashBucket {
                            index: i as u32,
                            used: true,
                        };
                    }
                }

                // Replace old cache
                self.items = new_items;
                self.buckets = buckets;
                self.systemd_enabled = view.systemd_enabled;
                self.generation = view.generation;
                self.populated = true;
                self.refresh_success_count += 1;
                self.last_refresh_ts = self.epoch.elapsed().as_millis() as u64;
                true
            }
            Err(_) => {
                // Refresh failed: preserve previous cache
                self.refresh_failure_count += 1;
                false
            }
        }
    }

    /// Returns true if at least one successful refresh has occurred.
    /// Cheap cached boolean. No I/O, no syscalls.
    ///
    /// Note: ready means "has cached data", not "is connected."
    #[inline]
    pub fn ready(&self) -> bool {
        self.populated
    }

    /// Look up a cached item by hash + name. O(1) via open-addressing hash
    /// table. No I/O.
    pub fn lookup(&self, hash: u32, name: &str) -> Option<&CgroupsCacheItem> {
        if !self.populated {
            return None;
        }
        if !self.buckets.is_empty() {
            let mask = (self.buckets.len() - 1) as u32;
            let mut slot = (hash ^ cache_hash_name(name)) & mask;
            while self.buckets[slot as usize].used {
                let item = &self.items[self.buckets[slot as usize].index as usize];
                if item.hash == hash && item.name == name {
                    return Some(item);
                }
                slot = (slot + 1) & mask;
            }
            return None;
        }

        self.items
            .iter()
            .find(|item| item.hash == hash && item.name == name)
    }

    /// Fill a status snapshot for diagnostics.
    pub fn status(&self) -> CgroupsCacheStatus {
        CgroupsCacheStatus {
            populated: self.populated,
            item_count: self.items.len() as u32,
            systemd_enabled: self.systemd_enabled,
            generation: self.generation,
            refresh_success_count: self.refresh_success_count,
            refresh_failure_count: self.refresh_failure_count,
            connection_state: self.client.state,
            last_refresh_ts: self.last_refresh_ts,
        }
    }

    /// Close the cache: free all cached items, close the L2 client.
    pub fn close(&mut self) {
        self.items.clear();
        self.buckets.clear();
        self.populated = false;
        self.client.close();
    }
}

impl Drop for CgroupsCache {
    fn drop(&mut self) {
        self.close();
    }
}

// ---------------------------------------------------------------------------
//  Tests
// ---------------------------------------------------------------------------

#[cfg(all(test, unix))]
#[path = "raw_unix_tests.rs"]
mod tests;

#[cfg(all(test, windows))]
#[path = "raw_windows_tests.rs"]
mod windows_tests;
