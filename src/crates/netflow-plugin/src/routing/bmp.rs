use super::{DynamicRoutingPeerKey, DynamicRoutingRuntime, DynamicRoutingUpdate};
use crate::plugin_config::{RouteDistinguisherConfig, RoutingDynamicBmpConfig};
use anyhow::{Context, Result};
use ipnet::IpNet;
use netgauze_bgp_pkt::BgpMessage;
use netgauze_bgp_pkt::nlri::{L2EvpnAddress, L2EvpnIpPrefixRoute, L2EvpnRoute, RouteDistinguisher};
use netgauze_bgp_pkt::path_attribute::{
    As4Path, AsPath, AsPathSegmentType, MpReach, MpUnreach, PathAttributeValue,
};
use netgauze_bgp_pkt::update::BgpUpdateMessage;
use netgauze_bmp_pkt::codec::BmpCodec;
use netgauze_bmp_pkt::v3::BmpMessageValue;
use netgauze_bmp_pkt::{BmpMessage, BmpPeerType, PeerHeader};
use socket2::SockRef;
use std::collections::{HashMap, HashSet};
use std::net::{IpAddr, Ipv4Addr, SocketAddr};
use std::sync::Arc;
use std::sync::atomic::{AtomicU64, Ordering};
use tokio::net::TcpListener;
use tokio::sync::Mutex;
use tokio::time::sleep;
use tokio_stream::StreamExt;
use tokio_util::codec::Framed;
use tokio_util::sync::CancellationToken;

mod listener;
mod rd;
mod routes;
mod session;

#[cfg(test)]
mod tests;

pub(crate) use listener::run_bmp_listener;
