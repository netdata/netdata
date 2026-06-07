use super::client::{ClientConfig, RawCallKind, RawClient};
use super::dispatch::{DispatchError, DispatchHandler};
use crate::protocol::{
    self, batch_item_get, increment_decode, increment_encode, BatchBuilder, NipcError,
    INCREMENT_PAYLOAD_SIZE, METHOD_INCREMENT,
};
use std::sync::Arc;

pub type IncrementHandler = Arc<dyn Fn(u64) -> Option<u64> + Send + Sync>;

impl RawClient {
    /// Create a new client context bound to the increment service kind.
    /// Does NOT connect. Does NOT require the server to be running.
    pub fn new_increment(run_dir: &str, service_name: &str, config: ClientConfig) -> Self {
        Self::new_bound(run_dir, service_name, METHOD_INCREMENT, config)
    }

    /// Blocking typed call: INCREMENT method.
    /// Sends a u64 value, receives the incremented u64 back.
    pub fn call_increment(&mut self, value: u64) -> Result<u64, NipcError> {
        self.validate_method(METHOD_INCREMENT)?;
        let req_len = {
            let req_buf = self.request_scratch(INCREMENT_PAYLOAD_SIZE);
            let req_len = increment_encode(value, req_buf);
            if req_len == 0 {
                return Err(NipcError::Truncated);
            }
            req_len
        };

        let response =
            self.raw_call_with_retry(METHOD_INCREMENT, req_len, RawCallKind::single())?;
        increment_decode(self.response_payload(response)?)
    }

    /// Blocking typed batch call: INCREMENT method.
    /// Sends multiple u64 values, receives the incremented u64s back.
    pub fn call_increment_batch(&mut self, values: &[u64]) -> Result<Vec<u64>, NipcError> {
        self.validate_method(METHOD_INCREMENT)?;
        if values.is_empty() {
            return Ok(Vec::new());
        }

        if values.len() == 1 {
            let r = self.call_increment(values[0])?;
            return Ok(vec![r]);
        }

        let count = values.len() as u32;
        let req_buf_size = protocol::align8(count as usize * 8)
            + count as usize * protocol::align8(INCREMENT_PAYLOAD_SIZE)
            + 64;
        let req_len = {
            let req_buf = self.request_scratch(req_buf_size);
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
            self.raw_call_with_retry(METHOD_INCREMENT, req_len, RawCallKind::batch(count))?;
        let resp_payload = self.response_payload(response)?;
        let mut results = Vec::with_capacity(values.len());
        for i in 0..count {
            let (item_data, _item_len) = batch_item_get(resp_payload, count, i)?;
            let val = increment_decode(item_data)?;
            results.push(val);
        }

        Ok(results)
    }
}

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
