use super::{DynamicRoutingPeerKey, DynamicRoutingRuntime, DynamicRoutingUpdate};
use crate::plugin_config::RoutingDynamicBiorisRisInstanceConfig;
use anyhow::{Context, Result};
use ipnet::{IpNet, Ipv4Net, Ipv6Net};
use std::collections::HashSet;
use std::net::{IpAddr, Ipv4Addr, Ipv6Addr, SocketAddr};
use std::time::Duration;
use tonic::transport::{Channel, ClientTlsConfig, Endpoint};

pub(crate) mod proto {
    pub(crate) mod bio {
        pub(crate) mod net {
            include!("proto/bio.net.rs");
        }
        pub(crate) mod route {
            include!("proto/bio.route.rs");
        }
        pub(crate) mod ris {
            include!("proto/bio.ris.rs");
        }
    }
}

use proto::bio::net::ip::Version as ProtoIpVersion;
use proto::bio::net::{Ip as ProtoIp, Prefix as ProtoPrefix};
use proto::bio::ris::dump_rib_request::Afisafi as ProtoAfiSafi;
use proto::bio::ris::observe_rib_request::Afisafi as ProtoObserveAfiSafi;
use proto::bio::ris::routing_information_service_client::RoutingInformationServiceClient;
use proto::bio::ris::{DumpRibRequest, GetRoutersRequest, ObserveRibRequest, RibUpdate};
use proto::bio::route::{AsPathSegment as ProtoAsPathSegment, BgpPath, Route};

mod client;
mod route;
mod runtime;

#[cfg(test)]
mod tests;

pub(crate) use runtime::run_bioris_listener;
