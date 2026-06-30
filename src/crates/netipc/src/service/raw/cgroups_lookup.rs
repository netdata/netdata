use super::client::{ClientConfig, RawCallKind, RawClient};
use super::common::lookup_raw_response_size;
use super::dispatch::{dispatch_error_from_protocol, DispatchHandler};
use crate::protocol::{
    self, CgroupsLookupBuilder, CgroupsLookupRequestView, CgroupsLookupResponseView, NipcError,
    CGROUPS_LOOKUP_ITEM_HDR_SIZE, CGROUPS_LOOKUP_REQ_HDR_SIZE, CGROUPS_LOOKUP_RESP_HDR_SIZE,
    CGROUP_LOOKUP_OVERSIZED_ITEM, CGROUP_LOOKUP_PAYLOAD_EXCEEDED, LOOKUP_DIR_ENTRY_SIZE,
    METHOD_CGROUPS_LOOKUP,
};
use std::sync::Arc;

fn cgroups_lookup_request_size(paths: &[&[u8]]) -> Result<usize, NipcError> {
    let dir_size = paths
        .len()
        .checked_mul(LOOKUP_DIR_ENTRY_SIZE)
        .ok_or(NipcError::Overflow)?;
    let mut data = CGROUPS_LOOKUP_REQ_HDR_SIZE
        .checked_add(dir_size)
        .ok_or(NipcError::Overflow)?;
    for path in paths {
        data = data
            .checked_add(7)
            .map(|v| v & !7)
            .ok_or(NipcError::Overflow)?;
        data = data
            .checked_add(path.len())
            .and_then(|v| v.checked_add(1))
            .ok_or(NipcError::Overflow)?;
    }
    Ok(data)
}

fn cgroups_lookup_next_request_count(
    paths: &[&[u8]],
    max_payload: u32,
) -> Result<(usize, usize), NipcError> {
    if paths.is_empty() {
        return Ok((0, cgroups_lookup_request_size(&[])?));
    }
    let cap = if max_payload == 0 {
        protocol::MAX_PAYLOAD_DEFAULT
    } else {
        max_payload
    } as usize;
    let mut lo = 1usize;
    let mut hi = paths.len();
    let mut best_count = 0usize;
    let mut best_size = 0usize;
    while lo <= hi {
        let mid = lo + (hi - lo) / 2;
        let size = cgroups_lookup_request_size(&paths[..mid])?;
        if size <= cap {
            best_count = mid;
            best_size = size;
            lo = mid + 1;
        } else {
            hi = mid - 1;
        }
    }
    if best_count == 0 {
        return Err(NipcError::Overflow);
    }
    Ok((best_count, best_size))
}

fn cgroups_lookup_oversized_request_item(path: &[u8]) -> Result<Vec<u8>, NipcError> {
    let mut size = CGROUPS_LOOKUP_RESP_HDR_SIZE
        .checked_add(LOOKUP_DIR_ENTRY_SIZE)
        .ok_or(NipcError::Overflow)?;
    size = size
        .checked_add(7)
        .map(|v| v & !7)
        .ok_or(NipcError::Overflow)?;
    let item_size = CGROUPS_LOOKUP_ITEM_HDR_SIZE
        .checked_add(path.len())
        .and_then(|v| v.checked_add(2))
        .ok_or(NipcError::Overflow)?;
    size = size.checked_add(item_size).ok_or(NipcError::Overflow)?;

    let mut buf = vec![0u8; size];
    let mut builder = CgroupsLookupBuilder::new(&mut buf, 1, 0);
    builder.add(CGROUP_LOOKUP_OVERSIZED_ITEM, 0, path, b"", &[])?;
    let n = builder.finish()?;
    let view = CgroupsLookupResponseView::decode(&buf[..n])?;
    Ok(view.raw_item(0)?.to_vec())
}

pub type CgroupsLookupHandler = Arc<
    dyn for<'a> Fn(&CgroupsLookupRequestView, &mut CgroupsLookupBuilder<'a>) -> bool + Send + Sync,
>;

impl RawClient {
    /// Create a new client context bound to the cgroups-lookup service kind.
    /// Does NOT connect. Does NOT require the server to be running.
    pub fn new_cgroups_lookup(run_dir: &str, service_name: &str, config: ClientConfig) -> Self {
        Self::new_bound(run_dir, service_name, METHOD_CGROUPS_LOOKUP, config)
    }

    /// Blocking typed call: CGROUPS_LOOKUP method.
    ///
    /// The returned view is valid until the next typed call on this client.
    pub fn call_cgroups_lookup(
        &mut self,
        paths: &[&[u8]],
    ) -> Result<CgroupsLookupResponseView<'_>, NipcError> {
        self.call_cgroups_lookup_with_timeout(paths, 0)
    }

    /// Blocking typed call with an explicit timeout in milliseconds.
    ///
    /// A zero timeout uses the client's context-level default.
    pub fn call_cgroups_lookup_with_timeout(
        &mut self,
        paths: &[&[u8]],
        timeout_ms: u32,
    ) -> Result<CgroupsLookupResponseView<'_>, NipcError> {
        self.validate_method(METHOD_CGROUPS_LOOKUP)?;
        if paths.len() > self.max_logical_lookup_items as usize {
            return Err(NipcError::Overflow);
        }
        self.ensure_ready_for_logical_lookup()?;

        let mut raw_items: Vec<Vec<u8>> = Vec::with_capacity(paths.len());
        let mut start = 0usize;
        let mut generation: Option<u64> = None;
        let mut subcalls = 0u32;
        let deadline = self.lookup_deadline(timeout_ms)?;
        loop {
            let (req_count, req_size) = match cgroups_lookup_next_request_count(
                &paths[start..],
                self.session_max_request_payload_bytes(),
            ) {
                Ok(next) => next,
                Err(err) if err == NipcError::Overflow && start < paths.len() => {
                    let one_item_size = cgroups_lookup_request_size(&paths[start..start + 1])?;
                    if self.ensure_lookup_request_capacity(one_item_size)? {
                        continue;
                    }
                    raw_items.push(cgroups_lookup_oversized_request_item(paths[start])?);
                    start += 1;
                    if start < paths.len() {
                        continue;
                    }
                    cgroups_lookup_next_request_count(
                        &paths[start..],
                        self.session_max_request_payload_bytes(),
                    )?
                }
                Err(err) => return Err(err),
            };
            let req_paths = &paths[start..start + req_count];

            let req_len = {
                let req_buf = self.request_scratch(req_size);
                protocol::encode_cgroups_lookup_request(req_paths, req_buf)?
            };
            let remaining_timeout = self.lookup_remaining_timeout(deadline)?;
            subcalls += 1;
            if subcalls > self.max_logical_lookup_subcalls {
                return Err(NipcError::Overflow);
            }
            let response = self.raw_call_with_retry_timeout(
                METHOD_CGROUPS_LOOKUP,
                req_len,
                RawCallKind::single(),
                remaining_timeout,
            )?;
            let final_size = {
                let view = CgroupsLookupResponseView::decode(self.response_payload(response)?)?;
                if let Some(expected_generation) = generation {
                    if view.generation != expected_generation {
                        return Err(NipcError::BadLayout);
                    }
                } else {
                    generation = Some(view.generation);
                }
                if view.item_count != req_paths.len() as u32 {
                    return Err(NipcError::BadItemCount);
                }
                let mut payload_exceeded_at: Option<usize> = None;
                for (i, expected) in req_paths.iter().enumerate() {
                    let item = view.item(i as u32)?;
                    if item.path.as_bytes() != *expected {
                        return Err(NipcError::BadLayout);
                    }
                    if item.status == CGROUP_LOOKUP_PAYLOAD_EXCEEDED {
                        payload_exceeded_at = Some(i);
                        break;
                    }
                    raw_items.push(view.raw_item(i as u32)?.to_vec());
                }
                if let Some(first) = payload_exceeded_at {
                    for (i, expected) in req_paths.iter().enumerate().skip(first) {
                        let item = view.item(i as u32)?;
                        if item.path.as_bytes() != *expected
                            || item.status != CGROUP_LOOKUP_PAYLOAD_EXCEEDED
                        {
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
                if start < paths.len() {
                    continue;
                }
                if raw_items.len() != paths.len() {
                    return Err(NipcError::BadItemCount);
                }
                let size = lookup_raw_response_size(
                    CGROUPS_LOOKUP_RESP_HDR_SIZE,
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
            let n = protocol::encode_cgroups_lookup_raw_response(
                &refs,
                generation.unwrap_or(0),
                &mut self.transport_buf[..final_size],
            )?;
            return CgroupsLookupResponseView::decode(&self.transport_buf[..n]);
        }
    }
}

pub fn cgroups_lookup_dispatch(handler: CgroupsLookupHandler) -> DispatchHandler {
    Arc::new(move |request, response_buf| {
        protocol::dispatch_cgroups_lookup(request, response_buf, |request, builder| {
            handler(request, builder)
        })
        .map_err(dispatch_error_from_protocol)
    })
}
