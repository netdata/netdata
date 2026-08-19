use super::client::{ClientConfig, RawCallKind, RawClient};
use super::common::lookup_raw_response_size;
use super::dispatch::{dispatch_error_from_protocol, DispatchHandler};
use crate::protocol::{
    self, AppsLookupBuilder, AppsLookupRequestView, AppsLookupResponseView, NipcError,
    APPS_LOOKUP_KEY_SIZE, APPS_LOOKUP_REQ_HDR_SIZE, APPS_LOOKUP_RESP_HDR_SIZE,
    LOOKUP_DIR_ENTRY_SIZE, METHOD_APPS_LOOKUP, PID_LOOKUP_PAYLOAD_EXCEEDED,
};
use std::sync::Arc;

fn apps_lookup_request_size(count: usize) -> Result<usize, NipcError> {
    let dir_size = count
        .checked_mul(LOOKUP_DIR_ENTRY_SIZE)
        .ok_or(NipcError::Overflow)?;
    let key_size = count
        .checked_mul(APPS_LOOKUP_KEY_SIZE)
        .ok_or(NipcError::Overflow)?;
    APPS_LOOKUP_REQ_HDR_SIZE
        .checked_add(dir_size)
        .and_then(|v| v.checked_add(key_size))
        .ok_or(NipcError::Overflow)
}

fn apps_lookup_next_request_count(
    remaining: usize,
    max_payload: u32,
) -> Result<(usize, usize), NipcError> {
    if remaining == 0 {
        return Ok((0, apps_lookup_request_size(0)?));
    }

    let cap = if max_payload == 0 {
        protocol::MAX_PAYLOAD_DEFAULT
    } else {
        max_payload
    } as usize;
    let per_item = LOOKUP_DIR_ENTRY_SIZE
        .checked_add(APPS_LOOKUP_KEY_SIZE)
        .ok_or(NipcError::Overflow)?;
    if cap < APPS_LOOKUP_REQ_HDR_SIZE || cap - APPS_LOOKUP_REQ_HDR_SIZE < per_item {
        return Err(NipcError::Overflow);
    }
    let max_count = (cap - APPS_LOOKUP_REQ_HDR_SIZE) / per_item;
    if max_count == 0 {
        return Err(NipcError::Overflow);
    }
    let count = remaining.min(max_count);
    Ok((count, apps_lookup_request_size(count)?))
}

pub type AppsLookupHandler =
    Arc<dyn for<'a> Fn(&AppsLookupRequestView, &mut AppsLookupBuilder<'a>) -> bool + Send + Sync>;

impl RawClient {
    /// Create a new client context bound to the apps-lookup service kind.
    /// Does NOT connect. Does NOT require the server to be running.
    pub fn new_apps_lookup(run_dir: &str, service_name: &str, config: ClientConfig) -> Self {
        Self::new_bound(run_dir, service_name, METHOD_APPS_LOOKUP, config)
    }

    /// Blocking typed call: APPS_LOOKUP method.
    ///
    /// The returned view is valid until the next typed call on this client.
    pub fn call_apps_lookup(
        &mut self,
        pids: &[u32],
    ) -> Result<AppsLookupResponseView<'_>, NipcError> {
        self.call_apps_lookup_with_timeout(pids, 0)
    }

    /// Blocking typed call with an explicit timeout in milliseconds.
    ///
    /// A zero timeout uses the client's context-level default.
    pub fn call_apps_lookup_with_timeout(
        &mut self,
        pids: &[u32],
        timeout_ms: u32,
    ) -> Result<AppsLookupResponseView<'_>, NipcError> {
        self.validate_method(METHOD_APPS_LOOKUP)?;
        if pids.len() > self.max_logical_lookup_items as usize {
            return Err(NipcError::Overflow);
        }
        self.ensure_ready_for_logical_lookup()?;

        let mut raw_items: Vec<Vec<u8>> = Vec::with_capacity(pids.len());
        let mut start = 0usize;
        let mut generation: Option<u64> = None;
        let mut subcalls = 0u32;
        let deadline = self.lookup_deadline(timeout_ms)?;
        loop {
            let (req_count, req_size) = match apps_lookup_next_request_count(
                pids.len() - start,
                self.session_max_request_payload_bytes(),
            ) {
                Ok(next) => next,
                Err(NipcError::Overflow) if start < pids.len() => {
                    let one_item_size = apps_lookup_request_size(1)?;
                    if self.ensure_lookup_request_capacity(one_item_size)? {
                        continue;
                    }
                    return Err(NipcError::Overflow);
                }
                Err(err) => return Err(err),
            };
            let req_pids = &pids[start..start + req_count];
            let req_len = {
                let req_buf = self.request_scratch(req_size);
                protocol::encode_apps_lookup_request(req_pids, req_buf)?
            };
            let remaining_timeout = self.lookup_remaining_timeout(deadline)?;
            subcalls += 1;
            if subcalls > self.max_logical_lookup_subcalls {
                return Err(NipcError::Overflow);
            }
            let response = self.raw_call_with_retry_timeout(
                METHOD_APPS_LOOKUP,
                req_len,
                RawCallKind::single(),
                remaining_timeout,
            )?;
            let final_size = {
                let view = AppsLookupResponseView::decode(self.response_payload(response)?)?;
                if let Some(expected_generation) = generation {
                    if view.generation != expected_generation {
                        return Err(NipcError::BadLayout);
                    }
                } else {
                    generation = Some(view.generation);
                }
                if view.item_count != req_pids.len() as u32 {
                    return Err(NipcError::BadItemCount);
                }
                let mut payload_exceeded_at: Option<usize> = None;
                for (i, expected) in req_pids.iter().enumerate() {
                    let item = view.item(i as u32)?;
                    if item.pid != *expected {
                        return Err(NipcError::BadLayout);
                    }
                    if item.status == PID_LOOKUP_PAYLOAD_EXCEEDED {
                        payload_exceeded_at = Some(i);
                        break;
                    }
                    raw_items.push(view.raw_item(i as u32)?.to_vec());
                }
                if let Some(first) = payload_exceeded_at {
                    for (i, expected) in req_pids.iter().enumerate().skip(first) {
                        let item = view.item(i as u32)?;
                        if item.pid != *expected || item.status != PID_LOOKUP_PAYLOAD_EXCEEDED {
                            return Err(NipcError::BadLayout);
                        }
                    }
                    if first == 0 {
                        return Err(NipcError::Overflow);
                    }
                    start += first;
                    continue;
                }
                start += req_count;
                if start < pids.len() {
                    continue;
                }
                if raw_items.len() != pids.len() {
                    return Err(NipcError::BadItemCount);
                }
                let size = lookup_raw_response_size(
                    APPS_LOOKUP_RESP_HDR_SIZE,
                    raw_items.len(),
                    &raw_items,
                )?;
                if size > self.max_logical_lookup_response_bytes as usize {
                    return Err(NipcError::Overflow);
                }
                size
            };
            self.transport_buf.resize(final_size, 0);
            let refs: Vec<&[u8]> = raw_items.iter().map(Vec::as_slice).collect();
            let n = protocol::encode_apps_lookup_raw_response(
                &refs,
                generation.unwrap_or(0),
                &mut self.transport_buf[..final_size],
            )?;
            return AppsLookupResponseView::decode(&self.transport_buf[..n]);
        }
    }
}

pub fn apps_lookup_dispatch(handler: AppsLookupHandler) -> DispatchHandler {
    Arc::new(move |request, response_buf| {
        protocol::dispatch_apps_lookup(request, response_buf, |request, builder| {
            handler(request, builder)
        })
        .map_err(dispatch_error_from_protocol)
    })
}
