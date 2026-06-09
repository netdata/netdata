use super::client::{ClientConfig, RawCallKind, RawClient};
use super::dispatch::{DispatchError, DispatchHandler};
use crate::protocol::{
    self, CgroupsLookupBuilder, CgroupsLookupRequestView, CgroupsLookupResponseView, NipcError,
    CGROUPS_LOOKUP_REQ_HDR_SIZE, LOOKUP_DIR_ENTRY_SIZE, METHOD_CGROUPS_LOOKUP,
};
use std::sync::Arc;

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
        self.validate_method(METHOD_CGROUPS_LOOKUP)?;

        let dir_size = paths
            .len()
            .checked_mul(LOOKUP_DIR_ENTRY_SIZE)
            .ok_or(NipcError::Overflow)?;
        let mut req_size = CGROUPS_LOOKUP_REQ_HDR_SIZE
            .checked_add(dir_size)
            .ok_or(NipcError::Overflow)?;
        let mut data = req_size;
        for path in paths {
            let aligned = data
                .checked_add(7)
                .map(|v| v & !7)
                .ok_or(NipcError::Overflow)?;
            data = aligned
                .checked_add(path.len())
                .and_then(|v| v.checked_add(1))
                .ok_or(NipcError::Overflow)?;
        }
        req_size = data;

        let req_len = {
            let req_buf = self.request_scratch(req_size);
            protocol::encode_cgroups_lookup_request(paths, req_buf)?
        };
        let response =
            self.raw_call_with_retry(METHOD_CGROUPS_LOOKUP, req_len, RawCallKind::single())?;
        let view = CgroupsLookupResponseView::decode(self.response_payload(response)?)?;
        if view.item_count != paths.len() as u32 {
            return Err(NipcError::BadItemCount);
        }
        for (i, expected) in paths.iter().enumerate() {
            let item = view.item(i as u32)?;
            if item.path.as_bytes() != *expected {
                return Err(NipcError::BadLayout);
            }
        }
        Ok(view)
    }
}

pub fn cgroups_lookup_dispatch(handler: CgroupsLookupHandler) -> DispatchHandler {
    Arc::new(move |request, response_buf| {
        let request =
            CgroupsLookupRequestView::decode(request).map_err(|_| DispatchError::BadEnvelope)?;
        let dir_size = (request.item_count as usize)
            .checked_mul(LOOKUP_DIR_ENTRY_SIZE)
            .ok_or(DispatchError::Overflow)?;
        let min_required = protocol::CGROUPS_LOOKUP_RESP_HDR_SIZE
            .checked_add(dir_size)
            .ok_or(DispatchError::Overflow)?;
        if response_buf.len() < min_required {
            return Err(DispatchError::Overflow);
        }
        let mut builder = CgroupsLookupBuilder::new(response_buf, request.item_count, 0);
        if !handler(&request, &mut builder) {
            return Err(DispatchError::HandlerFailed);
        }
        if let Some(err) = builder.error() {
            return match err {
                NipcError::Overflow => Err(DispatchError::Overflow),
                _ => Err(DispatchError::BadEnvelope),
            };
        }
        if builder.item_count() != request.item_count {
            return Err(DispatchError::BadEnvelope);
        }
        builder.finish().map_err(|err| match err {
            NipcError::Overflow => DispatchError::Overflow,
            _ => DispatchError::BadEnvelope,
        })
    })
}
