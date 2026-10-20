use super::common::{ensure_client_scratch, SERVER_POLL_TIMEOUT_MS};
use super::dispatch::{
    dispatch_single_internal, method_supported_internal, server_note_payload_capacity,
    DispatchError, DispatchHandler,
};
use crate::protocol::{
    self, batch_item_get, BatchBuilder, Header, FLAG_BATCH, HEADER_SIZE, KIND_REQUEST,
    KIND_RESPONSE, MAGIC_MSG, STATUS_BAD_ENVELOPE, STATUS_INTERNAL_ERROR, STATUS_LIMIT_EXCEEDED,
    STATUS_OK, VERSION,
};
use crate::transport::win_shm::WinShmContext;
use crate::transport::windows::NpSession;
use std::sync::atomic::{AtomicBool, AtomicU32, Ordering};
use std::sync::Arc;

/// Windows: handle one client session over Named Pipe + optional Win SHM.
/// Standalone function for use in per-session threads.
pub(super) fn handle_session_win_threaded(
    mut session: NpSession,
    mut shm: Option<WinShmContext>,
    expected_method_code: u16,
    handler: Option<DispatchHandler>,
    running: Arc<AtomicBool>,
    learned_request_payload_bytes: Arc<AtomicU32>,
    learned_response_payload_bytes: Arc<AtomicU32>,
    request_payload_growth_ceiling: u32,
    response_payload_growth_ceiling: u32,
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

        if hdr.kind != KIND_REQUEST {
            break;
        }

        if payload.len() <= u32::MAX as usize {
            server_note_payload_capacity(
                &learned_request_payload_bytes,
                payload.len() as u32,
                request_payload_growth_ceiling,
            );
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
                        response_payload_growth_ceiling,
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
                    server_note_payload_capacity(
                        &learned_response_payload_bytes,
                        u32::MAX,
                        response_payload_growth_ceiling,
                    );
                } else {
                    server_note_payload_capacity(
                        &learned_response_payload_bytes,
                        current * 2,
                        response_payload_growth_ceiling,
                    );
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
