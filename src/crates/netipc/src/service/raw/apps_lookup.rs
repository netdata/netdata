use super::client::{ClientConfig, RawCallKind, RawClient};
use super::dispatch::{DispatchError, DispatchHandler};
use crate::protocol::{
    self, AppsLookupBuilder, AppsLookupRequestView, AppsLookupResponseView, NipcError,
    APPS_LOOKUP_KEY_SIZE, APPS_LOOKUP_REQ_HDR_SIZE, LOOKUP_DIR_ENTRY_SIZE, METHOD_APPS_LOOKUP,
};
use std::sync::Arc;

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
        self.validate_method(METHOD_APPS_LOOKUP)?;

        let dir_size = pids
            .len()
            .checked_mul(LOOKUP_DIR_ENTRY_SIZE)
            .ok_or(NipcError::Overflow)?;
        let key_size = pids
            .len()
            .checked_mul(APPS_LOOKUP_KEY_SIZE)
            .ok_or(NipcError::Overflow)?;
        let req_size = APPS_LOOKUP_REQ_HDR_SIZE
            .checked_add(dir_size)
            .and_then(|v| v.checked_add(key_size))
            .ok_or(NipcError::Overflow)?;
        let req_len = {
            let req_buf = self.request_scratch(req_size);
            protocol::encode_apps_lookup_request(pids, req_buf)?
        };
        let response =
            self.raw_call_with_retry(METHOD_APPS_LOOKUP, req_len, RawCallKind::single())?;
        let view = AppsLookupResponseView::decode(self.response_payload(response)?)?;
        if view.item_count != pids.len() as u32 {
            return Err(NipcError::BadItemCount);
        }
        for (i, expected) in pids.iter().enumerate() {
            let item = view.item(i as u32)?;
            if item.pid != *expected {
                return Err(NipcError::BadLayout);
            }
        }
        Ok(view)
    }
}

pub fn apps_lookup_dispatch(handler: AppsLookupHandler) -> DispatchHandler {
    Arc::new(move |request, response_buf| {
        let request =
            AppsLookupRequestView::decode(request).map_err(|_| DispatchError::BadEnvelope)?;
        let dir_size = (request.item_count as usize)
            .checked_mul(LOOKUP_DIR_ENTRY_SIZE)
            .ok_or(DispatchError::Overflow)?;
        let min_required = protocol::APPS_LOOKUP_RESP_HDR_SIZE
            .checked_add(dir_size)
            .ok_or(DispatchError::Overflow)?;
        if response_buf.len() < min_required {
            return Err(DispatchError::Overflow);
        }
        let mut builder = AppsLookupBuilder::new(response_buf, request.item_count, 0);
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
