use super::client::{
    ClientResponseRef, ClientResponseSource, ClientState, RawCallKind, RawClient,
    CLIENT_ABORT_POLL_MS,
};
use super::common::{ensure_client_scratch, CACHE_RESPONSE_BUF_SIZE};
use crate::protocol::{
    self, Header, NipcError, HEADER_SIZE, KIND_REQUEST, KIND_RESPONSE, MAGIC_MSG,
    STATUS_LIMIT_EXCEEDED, STATUS_OK, VERSION,
};
use std::sync::atomic::{AtomicBool, Ordering};
use std::time::{Duration, Instant};

impl RawClient {
    pub(super) fn raw_call_with_retry_timeout(
        &mut self,
        method_code: u16,
        req_len: usize,
        call: RawCallKind,
        timeout_ms: u32,
    ) -> Result<ClientResponseRef, NipcError> {
        if self.state != ClientState::Ready {
            self.error_count += 1;
            return Err(NipcError::BadLayout);
        }
        if self.abort_requested() {
            self.disconnect();
            self.state = ClientState::Broken;
            self.error_count += 1;
            return Err(NipcError::Aborted);
        }

        let timeout_ms = self.resolved_call_timeout(timeout_ms);

        let mut overflow_retries = 0u32;
        loop {
            let prev_req = self.session_max_request_payload_bytes();
            let prev_resp = self.session_max_response_payload_bytes();
            let prev_cfg_req = self.transport_config.max_request_payload_bytes;
            let prev_cfg_resp = self.transport_config.max_response_payload_bytes;

            match self.do_raw_call(method_code, req_len, call, timeout_ms) {
                Ok(payload) => {
                    self.call_count += 1;
                    return Ok(payload);
                }
                Err(first_err) => {
                    if first_err == NipcError::Timeout || first_err == NipcError::Aborted {
                        self.disconnect();
                        self.state = ClientState::Broken;
                        self.error_count += 1;
                        return Err(first_err);
                    }

                    if first_err != NipcError::Overflow {
                        self.disconnect();
                        self.state = ClientState::Broken;
                        self.state = self.try_connect();
                        if self.state != ClientState::Ready {
                            self.error_count += 1;
                            return Err(first_err);
                        }
                        self.reconnect_count += 1;

                        match self.do_raw_call(method_code, req_len, call, timeout_ms) {
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

    /// Single attempt at a raw call.
    fn do_raw_call(
        &mut self,
        method_code: u16,
        req_len: usize,
        call: RawCallKind,
        timeout_ms: u32,
    ) -> Result<ClientResponseRef, NipcError> {
        if self.abort_requested() {
            return Err(NipcError::Aborted);
        }

        let mut hdr = Header {
            kind: KIND_REQUEST,
            code: method_code,
            flags: call.flags,
            item_count: call.item_count,
            message_id: (self.call_count as u64) + 1,
            transport_status: STATUS_OK,
            ..Header::default()
        };

        self.transport_send_request_buf(&mut hdr, req_len)?;
        let (resp_hdr, response) = self.transport_receive(timeout_ms)?;

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

        if call.check_item_count && resp_hdr.item_count != call.item_count {
            return Err(NipcError::BadItemCount);
        }

        Ok(response)
    }

    /// Compatibility test seam for sending a borrowed payload through the
    /// single shared request-buffer transport path.
    #[cfg(test)]
    #[allow(dead_code)]
    pub(super) fn transport_send(
        &mut self,
        hdr: &mut Header,
        payload: &[u8],
    ) -> Result<(), NipcError> {
        let req = self.request_scratch(payload.len());
        req.copy_from_slice(payload);
        self.transport_send_request_buf(hdr, payload.len())
    }

    /// Send via the active transport (SHM if available, baseline otherwise).
    pub(super) fn transport_send_request_buf(
        &mut self,
        hdr: &mut Header,
        req_len: usize,
    ) -> Result<(), NipcError> {
        let max_request_payload_bytes = self.session_max_request_payload_bytes();

        #[cfg(target_os = "linux")]
        {
            if self.shm.is_some() {
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

                let send_result = self.shm.as_mut().unwrap().send(&msg[..msg_len]);
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
            if self.shm.is_some() {
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

                let send_result = self.shm.as_mut().unwrap().send(&msg[..msg_len]);
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

    /// Receive via the active transport. Returns (header, payload view).
    pub(super) fn transport_receive(
        &mut self,
        timeout_ms: u32,
    ) -> Result<(Header, ClientResponseRef), NipcError> {
        let needed = self.max_receive_message_bytes();
        let abort = self.abort_requested.clone();
        let scratch = ensure_client_scratch(&mut self.transport_buf, needed);

        #[cfg(target_os = "linux")]
        {
            if let Some(ref mut shm) = self.shm {
                let mlen = receive_posix_shm_with_control(shm, scratch, timeout_ms, &abort)?;

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
                let mlen = receive_win_shm_with_control(shm, scratch, timeout_ms, &abort)?;

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

        let session = self.session.as_mut().ok_or(NipcError::Truncated)?;

        #[cfg(unix)]
        {
            let scratch_payload_ptr = unsafe { scratch.as_ptr().add(HEADER_SIZE) };
            let (hdr, payload) = session
                .receive_with_control(scratch, timeout_ms, Some(&abort))
                .map_err(map_uds_receive_error)?;
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
            let (hdr, payload) = session
                .receive_with_control(scratch, timeout_ms, Some(&abort))
                .map_err(map_np_receive_error)?;
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

    pub(super) fn response_payload(&self, response: ClientResponseRef) -> Result<&[u8], NipcError> {
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

    pub(super) fn max_receive_message_bytes(&self) -> usize {
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

fn receive_deadline(timeout_ms: u32) -> Option<Instant> {
    (timeout_ms != 0).then(|| Instant::now() + Duration::from_millis(timeout_ms as u64))
}

fn receive_wait_slice_ms(deadline: Option<Instant>, abort: &AtomicBool) -> Result<u32, NipcError> {
    if abort.load(Ordering::Acquire) {
        return Err(NipcError::Aborted);
    }

    let mut wait_ms: Option<u128> = if let Some(deadline) = deadline {
        let now = Instant::now();
        if now >= deadline {
            return Err(NipcError::Timeout);
        }
        Some(deadline.duration_since(now).as_millis().max(1))
    } else {
        None
    };

    wait_ms = Some(wait_ms.map_or(CLIENT_ABORT_POLL_MS as u128, |ms| {
        ms.min(CLIENT_ABORT_POLL_MS as u128)
    }));

    Ok(wait_ms.unwrap().min(u32::MAX as u128) as u32)
}

#[cfg(target_os = "linux")]
fn receive_posix_shm_with_control(
    shm: &mut crate::transport::shm::ShmContext,
    scratch: &mut [u8],
    timeout_ms: u32,
    abort: &AtomicBool,
) -> Result<usize, NipcError> {
    use crate::transport::shm::ShmError;

    let deadline = receive_deadline(timeout_ms);
    loop {
        let wait_ms = receive_wait_slice_ms(deadline, abort)?;
        match shm.receive(scratch, wait_ms) {
            Ok(mlen) => return Ok(mlen),
            Err(ShmError::Timeout) => continue,
            Err(ShmError::MsgTooLarge) => return Err(NipcError::Overflow),
            Err(_) => return Err(NipcError::Truncated),
        }
    }
}

#[cfg(windows)]
fn receive_win_shm_with_control(
    shm: &mut crate::transport::win_shm::WinShmContext,
    scratch: &mut [u8],
    timeout_ms: u32,
    abort: &AtomicBool,
) -> Result<usize, NipcError> {
    use crate::transport::win_shm::WinShmError;

    if abort.load(Ordering::Acquire) {
        return Err(NipcError::Aborted);
    }
    match shm.receive_ready(scratch) {
        Ok(Some(mlen)) => return Ok(mlen),
        Ok(None) => {}
        Err(WinShmError::MsgTooLarge) => return Err(NipcError::Overflow),
        Err(_) => return Err(NipcError::Truncated),
    }

    let deadline = receive_deadline(timeout_ms);
    loop {
        let wait_ms = receive_wait_slice_ms(deadline, abort)?;
        match shm.receive(scratch, wait_ms) {
            Ok(mlen) => return Ok(mlen),
            Err(WinShmError::Timeout) => continue,
            Err(WinShmError::MsgTooLarge) => return Err(NipcError::Overflow),
            Err(_) => return Err(NipcError::Truncated),
        }
    }
}

#[cfg(unix)]
fn map_uds_receive_error(err: crate::transport::posix::UdsError) -> NipcError {
    match err {
        crate::transport::posix::UdsError::Timeout => NipcError::Timeout,
        crate::transport::posix::UdsError::Aborted => NipcError::Aborted,
        crate::transport::posix::UdsError::LimitExceeded => NipcError::Overflow,
        _ => NipcError::Truncated,
    }
}

#[cfg(windows)]
fn map_np_receive_error(err: crate::transport::windows::NpError) -> NipcError {
    match err {
        crate::transport::windows::NpError::Timeout => NipcError::Timeout,
        crate::transport::windows::NpError::Aborted => NipcError::Aborted,
        crate::transport::windows::NpError::LimitExceeded => NipcError::Overflow,
        _ => NipcError::Truncated,
    }
}

#[cfg(all(test, windows))]
mod tests {
    use super::*;
    use crate::transport::win_shm::{WinShmContext, PROFILE_HYBRID};
    use std::sync::atomic::{AtomicBool, AtomicU64, Ordering};

    static RAW_WIN_SHM_TEST_COUNTER: AtomicU64 = AtomicU64::new(0);

    fn win_shm_pair(prefix: &str) -> (WinShmContext, WinShmContext) {
        let id = RAW_WIN_SHM_TEST_COUNTER.fetch_add(1, Ordering::Relaxed) + 1;
        let run_dir = std::env::temp_dir().join("nipc_raw_win_shm_rust_test");
        let _ = std::fs::create_dir_all(&run_dir);
        let service = format!("{}_{}_{}", prefix, std::process::id(), id);
        let auth_token = 0xfeed_0000 + id;
        let session_id = 0x1000 + id;

        let server = WinShmContext::server_create(
            &run_dir.display().to_string(),
            &service,
            auth_token,
            session_id,
            PROFILE_HYBRID,
            4096,
            4096,
        )
        .expect("server_create");
        let client = WinShmContext::client_attach(
            &run_dir.display().to_string(),
            &service,
            auth_token,
            session_id,
            PROFILE_HYBRID,
        )
        .expect("client_attach");

        (server, client)
    }

    #[test]
    fn test_receive_win_shm_with_control_fast_path_and_fallbacks_windows() {
        let (mut server, mut client) = win_shm_pair("rs_raw_win_shm_ready");
        let abort = AtomicBool::new(false);
        let mut scratch = [0u8; 128];

        server.send(b"response").expect("server send");
        let len = receive_win_shm_with_control(&mut client, &mut scratch, 100, &abort)
            .expect("receive ready response");
        assert_eq!(len, 8);
        assert_eq!(&scratch[..len], b"response");

        client.close();
        server.destroy();

        let (mut server, mut client) = win_shm_pair("rs_raw_win_shm_abort");
        let abort = AtomicBool::new(true);
        assert_eq!(
            receive_win_shm_with_control(&mut client, &mut scratch, 100, &abort),
            Err(NipcError::Aborted)
        );
        client.close();
        server.destroy();

        let (mut server, mut client) = win_shm_pair("rs_raw_win_shm_timeout");
        let abort = AtomicBool::new(false);
        assert_eq!(
            receive_win_shm_with_control(&mut client, &mut scratch, 1, &abort),
            Err(NipcError::Timeout)
        );
        client.close();
        server.destroy();

        let (mut server, mut client) = win_shm_pair("rs_raw_win_shm_close");
        let abort = AtomicBool::new(false);
        server.destroy();
        assert_eq!(
            receive_win_shm_with_control(&mut client, &mut scratch, 100, &abort),
            Err(NipcError::Truncated)
        );
        client.close();
    }
}
