use super::dispatch::DispatchHandler;
use crate::protocol::{MAX_PAYLOAD_CAP, MAX_PAYLOAD_DEFAULT};

#[cfg(unix)]
pub(super) use crate::transport::posix::ServerConfig;

#[cfg(windows)]
pub(super) use crate::transport::windows::ServerConfig;

use std::sync::atomic::{AtomicBool, AtomicU32, Ordering};
use std::sync::Arc;

/// L2 managed server. Typed request/response dispatcher.
///
/// Handles accept, spawns a thread per session (up to worker_count),
/// reads requests, dispatches to handler, sends responses.
pub struct ManagedServer {
    pub(super) run_dir: String,
    pub(super) service_name: String,
    pub(super) server_config: ServerConfig,
    pub(super) expected_method_code: u16,
    pub(super) handler: Option<DispatchHandler>,
    pub(super) running: Arc<AtomicBool>,
    pub(super) learned_request_payload_bytes: Arc<AtomicU32>,
    pub(super) learned_response_payload_bytes: Arc<AtomicU32>,
    pub(super) request_payload_growth_ceiling: u32,
    pub(super) response_payload_growth_ceiling: u32,
    pub(super) next_session_id: u64,
    pub(super) worker_count: usize,
    #[cfg(windows)]
    pub(super) listener_handle: Arc<std::sync::Mutex<Option<usize>>>,
}

impl ManagedServer {
    /// Create a new managed server for a single service kind.
    pub fn new(
        run_dir: &str,
        service_name: &str,
        config: ServerConfig,
        expected_method_code: u16,
        handler: Option<DispatchHandler>,
    ) -> Self {
        Self::with_workers(
            run_dir,
            service_name,
            config,
            expected_method_code,
            handler,
            8,
        )
    }

    /// Create a managed server with an explicit worker count.
    pub fn with_workers(
        run_dir: &str,
        service_name: &str,
        config: ServerConfig,
        expected_method_code: u16,
        handler: Option<DispatchHandler>,
        worker_count: usize,
    ) -> Self {
        let learned_request = if config.max_request_payload_bytes != 0 {
            config.max_request_payload_bytes
        } else {
            MAX_PAYLOAD_DEFAULT
        };
        let learned_response = if config.max_response_payload_bytes != 0 {
            config.max_response_payload_bytes
        } else {
            MAX_PAYLOAD_DEFAULT
        };
        let request_payload_growth_ceiling = if config.max_request_payload_bytes != 0 {
            config.max_request_payload_bytes
        } else {
            MAX_PAYLOAD_CAP
        };
        let response_payload_growth_ceiling = if config.max_response_payload_bytes != 0 {
            config.max_response_payload_bytes
        } else {
            MAX_PAYLOAD_CAP
        };

        ManagedServer {
            run_dir: run_dir.to_string(),
            service_name: service_name.to_string(),
            server_config: config,
            expected_method_code,
            handler,
            running: Arc::new(AtomicBool::new(false)),
            learned_request_payload_bytes: Arc::new(AtomicU32::new(learned_request)),
            learned_response_payload_bytes: Arc::new(AtomicU32::new(learned_response)),
            request_payload_growth_ceiling,
            response_payload_growth_ceiling,
            next_session_id: 1,
            worker_count: if worker_count < 1 { 1 } else { worker_count },
            #[cfg(windows)]
            listener_handle: Arc::new(std::sync::Mutex::new(None)),
        }
    }

    /// Signal shutdown. On Windows, also closes the listener pipe to
    /// unblock ConnectNamedPipe in the accept loop.
    pub fn stop(&self) {
        self.running.store(false, Ordering::Release);

        #[cfg(windows)]
        {
            let mut guard = self.listener_handle.lock().unwrap();
            if let Some(h) = guard.take() {
                extern "system" {
                    fn CloseHandle(h: isize) -> i32;
                }
                unsafe {
                    CloseHandle(h as isize);
                }
            }
        }
    }

    /// Returns the internal running flag for diagnostics and test helpers.
    ///
    /// For reliable shutdown, call `stop()`. On Windows, flipping this flag
    /// alone does not wake a blocking listener accept.
    pub fn running_flag(&self) -> Arc<AtomicBool> {
        self.running.clone()
    }
}
