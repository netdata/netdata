use super::client::{ClientConfig, RawCallKind, RawClient};
use super::dispatch::{DispatchError, DispatchHandler};
use crate::protocol::{
    self, string_reverse_decode, string_reverse_encode, NipcError, METHOD_STRING_REVERSE,
    STRING_REVERSE_HDR_SIZE,
};
use std::sync::Arc;

pub type StringReverseHandler = Arc<dyn Fn(&str) -> Option<String> + Send + Sync>;

impl RawClient {
    /// Create a new client context bound to the string-reverse service kind.
    /// Does NOT connect. Does NOT require the server to be running.
    pub fn new_string_reverse(run_dir: &str, service_name: &str, config: ClientConfig) -> Self {
        Self::new_bound(run_dir, service_name, METHOD_STRING_REVERSE, config)
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
        let req_size = STRING_REVERSE_HDR_SIZE
            .checked_add(s.len())
            .and_then(|size| size.checked_add(1))
            .ok_or(NipcError::Overflow)?;
        let req_len = {
            let req_buf = self.request_scratch(req_size);
            let req_len = string_reverse_encode(s.as_bytes(), req_buf);
            if req_len == 0 {
                return Err(NipcError::Truncated);
            }
            req_len
        };

        let response =
            self.raw_call_with_retry(METHOD_STRING_REVERSE, req_len, RawCallKind::single())?;
        string_reverse_decode(self.response_payload(response)?)
    }
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
