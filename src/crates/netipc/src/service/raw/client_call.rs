use super::client::{ClientResponseRef, ClientResponseSource, ClientState, RawCallKind, RawClient};
use super::common::{ensure_client_scratch, CACHE_RESPONSE_BUF_SIZE};
use crate::protocol::{
    self, Header, NipcError, HEADER_SIZE, KIND_REQUEST, KIND_RESPONSE, MAGIC_MSG,
    STATUS_LIMIT_EXCEEDED, STATUS_OK, VERSION,
};

impl RawClient {
    /// Reconnect-driven recovery for raw calls.
    ///
    /// Ordinary failures retry once. Overflow-driven resize recovery may
    /// reconnect more than once while negotiated capacities grow.
    pub(super) fn raw_call_with_retry(
        &mut self,
        method_code: u16,
        req_len: usize,
        call: RawCallKind,
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

            match self.do_raw_call(method_code, req_len, call) {
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

                        match self.do_raw_call(method_code, req_len, call) {
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
    ) -> Result<ClientResponseRef, NipcError> {
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
    pub(super) fn transport_receive(&mut self) -> Result<(Header, ClientResponseRef), NipcError> {
        let needed = self.max_receive_message_bytes();
        let scratch = ensure_client_scratch(&mut self.transport_buf, needed);

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
