use super::common::next_power_of_2_u32;
use std::sync::atomic::{AtomicU32, Ordering};
use std::sync::Arc;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum DispatchError {
    BadEnvelope,
    Overflow,
    HandlerFailed,
}

pub type DispatchHandler =
    Arc<dyn Fn(&[u8], &mut [u8]) -> Result<usize, DispatchError> + Send + Sync>;

pub(super) fn dispatch_single_internal(
    expected_method_code: u16,
    handler: Option<&DispatchHandler>,
    method_code: u16,
    request: &[u8],
    response_buf: &mut [u8],
) -> Result<usize, DispatchError> {
    if method_code != expected_method_code {
        return Err(DispatchError::HandlerFailed);
    }

    match handler {
        Some(dispatch) => match dispatch(request, response_buf) {
            Ok(n) if n <= response_buf.len() => Ok(n),
            Ok(_) => Err(DispatchError::Overflow),
            Err(err) => Err(err),
        },
        None => Err(DispatchError::HandlerFailed),
    }
}

#[cfg(test)]
#[allow(dead_code)]
pub(super) fn dispatch_single(
    expected_method_code: u16,
    handler: Option<&DispatchHandler>,
    method_code: u16,
    request: &[u8],
    response_buf: &mut [u8],
) -> Result<usize, DispatchError> {
    dispatch_single_internal(
        expected_method_code,
        handler,
        method_code,
        request,
        response_buf,
    )
}

pub(super) fn method_supported_internal(
    expected_method_code: u16,
    handler: Option<&DispatchHandler>,
    method_code: u16,
) -> bool {
    handler.is_some() && method_code == expected_method_code
}

pub(super) fn server_note_payload_capacity(target: &AtomicU32, payload_len: u32) {
    let grown = next_power_of_2_u32(payload_len);
    let mut current = target.load(Ordering::Relaxed);
    while grown > current {
        match target.compare_exchange_weak(current, grown, Ordering::Release, Ordering::Relaxed) {
            Ok(_) => break,
            Err(observed) => current = observed,
        }
    }
}
