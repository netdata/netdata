//! The seam for features that only need to see accepted trace exports, such
//! as the service map, without being part of how spans are stored.
//!
//! An observer reads the request on the request's task before ingestion
//! consumes it, and its observation is committed only after the spans are
//! durable. A failed export drops the observation, so a client's retry is
//! not seen twice.

use std::sync::Arc;

use opentelemetry_proto::tonic::collector::trace::v1::ExportTraceServiceRequest;
use otel_service_map::service::{Prepared, ServiceMap};

pub trait TraceObserver: Send + Sync {
    fn prepare(
        &self,
        tenant: &str,
        request: &ExportTraceServiceRequest,
    ) -> Box<dyn PreparedObservation>;
}

pub trait PreparedObservation: Send {
    fn commit(self: Box<Self>);
}

pub(crate) struct ServiceMapObserver(pub(crate) Arc<ServiceMap>);

struct ServiceMapObservation {
    map: Arc<ServiceMap>,
    prepared: Prepared,
}

impl TraceObserver for ServiceMapObserver {
    fn prepare(
        &self,
        tenant: &str,
        request: &ExportTraceServiceRequest,
    ) -> Box<dyn PreparedObservation> {
        Box::new(ServiceMapObservation {
            map: self.0.clone(),
            prepared: self.0.prepare(tenant, request),
        })
    }
}

impl PreparedObservation for ServiceMapObservation {
    fn commit(self: Box<Self>) {
        self.map.commit(self.prepared);
    }
}
