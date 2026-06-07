use super::client::{ClientConfig, RawCallKind, RawClient};
use super::dispatch::{DispatchError, DispatchHandler};
use crate::protocol::{
    self, CgroupsRequest, CgroupsResponseView, NipcError, METHOD_CGROUPS_SNAPSHOT,
};
use std::sync::Arc;

pub type SnapshotHandler =
    Arc<dyn for<'a> Fn(&CgroupsRequest, &mut protocol::CgroupsBuilder<'a>) -> bool + Send + Sync>;

impl RawClient {
    /// Create a new client context bound to the cgroups-snapshot service kind.
    /// Does NOT connect. Does NOT require the server to be running.
    pub fn new_snapshot(run_dir: &str, service_name: &str, config: ClientConfig) -> Self {
        Self::new_bound(run_dir, service_name, METHOD_CGROUPS_SNAPSHOT, config)
    }

    /// Blocking typed call: encode request, send, receive, check
    /// transport_status, decode response.
    ///
    /// The returned view is valid until the next typed call on this client.
    pub fn call_snapshot(&mut self) -> Result<CgroupsResponseView<'_>, NipcError> {
        self.validate_method(METHOD_CGROUPS_SNAPSHOT)?;
        let req = CgroupsRequest {
            layout_version: 1,
            flags: 0,
        };
        let req_len = {
            let req_buf = self.request_scratch(4);
            let req_len = req.encode(req_buf);
            if req_len == 0 {
                return Err(NipcError::Truncated);
            }
            req_len
        };

        let response =
            self.raw_call_with_retry(METHOD_CGROUPS_SNAPSHOT, req_len, RawCallKind::single())?;
        CgroupsResponseView::decode(self.response_payload(response)?)
    }
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
