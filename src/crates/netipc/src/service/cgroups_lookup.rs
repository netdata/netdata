//! L2 cgroups-lookup service facade.

use super::raw;
use crate::protocol::{
    CgroupsLookupResponseView, NipcError, METHOD_CGROUPS_LOOKUP, PROFILE_BASELINE,
};

#[cfg(unix)]
use crate::transport::posix::{
    ClientConfig as TransportClientConfig, ServerConfig as TransportServerConfig,
};

#[cfg(windows)]
use crate::transport::windows::{
    ClientConfig as TransportClientConfig, ServerConfig as TransportServerConfig,
};

use std::sync::atomic::AtomicBool;
use std::sync::Arc;

pub use raw::{CgroupsLookupHandler, ClientState, ClientStatus};

#[derive(Debug, Clone)]
pub struct ClientConfig {
    pub supported_profiles: u32,
    pub preferred_profiles: u32,
    pub max_request_batch_items: u32,
    pub max_response_payload_bytes: u32,
    pub auth_token: u64,
}

impl Default for ClientConfig {
    fn default() -> Self {
        Self {
            supported_profiles: PROFILE_BASELINE,
            preferred_profiles: 0,
            max_request_batch_items: 0,
            max_response_payload_bytes: 0,
            auth_token: 0,
        }
    }
}

impl ClientConfig {
    fn into_transport(self) -> TransportClientConfig {
        let mut transport = TransportClientConfig::default();
        transport.supported_profiles = self.supported_profiles;
        transport.preferred_profiles = self.preferred_profiles;
        transport.max_request_batch_items = self.max_request_batch_items;
        transport.max_response_payload_bytes = self.max_response_payload_bytes;
        transport.max_response_batch_items = self.max_request_batch_items;
        transport.auth_token = self.auth_token;
        transport
    }
}

#[derive(Debug, Clone)]
pub struct ServerConfig {
    pub supported_profiles: u32,
    pub preferred_profiles: u32,
    pub max_request_batch_items: u32,
    pub max_response_payload_bytes: u32,
    pub auth_token: u64,
}

impl Default for ServerConfig {
    fn default() -> Self {
        Self {
            supported_profiles: PROFILE_BASELINE,
            preferred_profiles: 0,
            max_request_batch_items: 0,
            max_response_payload_bytes: 0,
            auth_token: 0,
        }
    }
}

impl ServerConfig {
    fn into_transport(self) -> TransportServerConfig {
        let mut transport = TransportServerConfig::default();
        transport.supported_profiles = self.supported_profiles;
        transport.preferred_profiles = self.preferred_profiles;
        transport.max_request_batch_items = self.max_request_batch_items;
        transport.max_response_payload_bytes = self.max_response_payload_bytes;
        transport.max_response_batch_items = self.max_request_batch_items;
        transport.auth_token = self.auth_token;
        transport
    }
}

pub struct CgroupsLookupClient {
    inner: raw::RawClient,
}

impl CgroupsLookupClient {
    pub fn new(run_dir: &str, service_name: &str, config: ClientConfig) -> Self {
        Self {
            inner: raw::RawClient::new_cgroups_lookup(
                run_dir,
                service_name,
                config.into_transport(),
            ),
        }
    }

    pub fn refresh(&mut self) -> bool {
        self.inner.refresh()
    }

    pub fn ready(&self) -> bool {
        self.inner.ready()
    }

    pub fn status(&self) -> ClientStatus {
        self.inner.status()
    }

    pub fn call(&mut self, paths: &[&[u8]]) -> Result<CgroupsLookupResponseView<'_>, NipcError> {
        self.inner.call_cgroups_lookup(paths)
    }

    pub fn close(&mut self) {
        self.inner.close();
    }
}

impl Drop for CgroupsLookupClient {
    fn drop(&mut self) {
        self.close();
    }
}

#[derive(Clone, Default)]
pub struct Handler {
    pub handle: Option<CgroupsLookupHandler>,
}

pub struct ManagedServer {
    inner: raw::ManagedServer,
}

impl ManagedServer {
    pub fn new(run_dir: &str, service_name: &str, config: ServerConfig, handler: Handler) -> Self {
        Self::with_workers(run_dir, service_name, config, handler, 8)
    }

    pub fn with_workers(
        run_dir: &str,
        service_name: &str,
        config: ServerConfig,
        handler: Handler,
        worker_count: usize,
    ) -> Self {
        let raw_handler = handler.handle.map(raw::cgroups_lookup_dispatch);
        Self {
            inner: raw::ManagedServer::with_workers(
                run_dir,
                service_name,
                config.into_transport(),
                METHOD_CGROUPS_LOOKUP,
                raw_handler,
                worker_count,
            ),
        }
    }

    pub fn run(&mut self) -> Result<(), NipcError> {
        self.inner.run()
    }

    pub fn stop(&self) {
        self.inner.stop();
    }

    pub fn running_flag(&self) -> Arc<AtomicBool> {
        self.inner.running_flag()
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::protocol::PROFILE_SHM_FUTEX;
    use std::sync::atomic::Ordering;
    use std::sync::Arc;

    #[test]
    fn client_config_maps_to_transport() {
        let cfg = ClientConfig {
            supported_profiles: PROFILE_BASELINE | PROFILE_SHM_FUTEX,
            preferred_profiles: PROFILE_SHM_FUTEX,
            max_request_batch_items: 17,
            max_response_payload_bytes: 8192,
            auth_token: 99,
        };

        let transport = cfg.into_transport();
        assert_eq!(
            transport.supported_profiles,
            PROFILE_BASELINE | PROFILE_SHM_FUTEX
        );
        assert_eq!(transport.preferred_profiles, PROFILE_SHM_FUTEX);
        assert_eq!(transport.max_request_batch_items, 17);
        assert_eq!(transport.max_response_batch_items, 17);
        assert_eq!(transport.max_response_payload_bytes, 8192);
        assert_eq!(transport.auth_token, 99);
    }

    #[test]
    fn server_config_maps_to_transport() {
        let cfg = ServerConfig {
            supported_profiles: PROFILE_BASELINE | PROFILE_SHM_FUTEX,
            preferred_profiles: PROFILE_SHM_FUTEX,
            max_request_batch_items: 23,
            max_response_payload_bytes: 16384,
            auth_token: 123,
        };

        let transport = cfg.into_transport();
        assert_eq!(
            transport.supported_profiles,
            PROFILE_BASELINE | PROFILE_SHM_FUTEX
        );
        assert_eq!(transport.preferred_profiles, PROFILE_SHM_FUTEX);
        assert_eq!(transport.max_request_batch_items, 23);
        assert_eq!(transport.max_response_batch_items, 23);
        assert_eq!(transport.max_response_payload_bytes, 16384);
        assert_eq!(transport.auth_token, 123);
    }

    #[test]
    fn managed_server_initializes_stopped_with_handler() {
        let handler = Handler {
            handle: Some(Arc::new(|_, _| true)),
        };
        let server = ManagedServer::with_workers(
            "/tmp/netipc-cgroups-lookup-test",
            "cgroups-lookup-facade-test",
            ServerConfig::default(),
            handler,
            0,
        );
        let running = server.running_flag();

        assert!(!running.load(Ordering::SeqCst));
        server.stop();
        assert!(!running.load(Ordering::SeqCst));
    }

    #[cfg(windows)]
    #[test]
    fn client_lifecycle_without_server_windows() {
        let mut client = CgroupsLookupClient::new(
            r"C:\Temp\nipc-cgroups-lookup-facade",
            "cgroups-lookup-facade-no-server",
            ClientConfig::default(),
        );
        assert!(!client.ready());
        assert!(client.refresh());
        assert_eq!(client.status().state, ClientState::NotFound);

        let paths: [&[u8]; 1] = [b"/x".as_slice()];
        assert_eq!(client.call(&paths).unwrap_err(), NipcError::BadLayout);
        assert_eq!(client.status().error_count, 1);

        client.close();
        assert_eq!(client.status().state, ClientState::Disconnected);
    }

    #[cfg(windows)]
    #[test]
    fn managed_server_new_initializes_stopped_windows() {
        let server = ManagedServer::new(
            r"C:\Temp\nipc-cgroups-lookup-facade",
            "cgroups-lookup-facade-new",
            ServerConfig::default(),
            Handler::default(),
        );
        let running = server.running_flag();
        assert!(!running.load(Ordering::SeqCst));
        server.stop();
        assert!(!running.load(Ordering::SeqCst));
    }
}
