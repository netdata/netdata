use crate::enrichment::{DynamicRoutingPeerKey, DynamicRoutingRuntime, DynamicRoutingUpdate};
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

#[derive(Debug, Clone)]
struct NlriRoute {
    prefix: IpNet,
    route_key: String,
    rd: u64,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
enum BmpSessionDecision {
    Process,
    CloseMissingInitiation,
    CloseTermination,
}

pub(crate) async fn run_bmp_listener(
    config: RoutingDynamicBmpConfig,
    runtime: DynamicRoutingRuntime,
    shutdown: CancellationToken,
) -> Result<()> {
    if !config.enabled {
        return Ok(());
    }

    let listen_addr = config
        .listen
        .parse::<SocketAddr>()
        .with_context(|| format!("invalid BMP listen address {}", config.listen))?;
    let listener = TcpListener::bind(listen_addr)
        .await
        .with_context(|| format!("failed to bind BMP listener on {}", listen_addr))?;
    let accepted_rds = Arc::new(
        parse_configured_rds(&config.rds)
            .with_context(|| "invalid enrichment.routing_dynamic.bmp.rds entries")?,
    );
    tracing::info!("dynamic BMP routing listener started on {}", listen_addr);

    let sessions = Arc::new(Mutex::new(HashMap::<SocketAddr, u64>::new()));
    let next_session = AtomicU64::new(1);

    loop {
        tokio::select! {
            _ = shutdown.cancelled() => {
                tracing::info!("dynamic BMP routing listener shutdown requested");
                return Ok(());
            }
            accepted = listener.accept() => {
                let (stream, remote_addr) = match accepted {
                    Ok(v) => v,
                    Err(err) => {
                        tracing::warn!("failed to accept BMP connection: {}", err);
                        continue;
                    }
                };
                configure_receive_buffer(&stream, remote_addr, config.receive_buffer);

                let session_id = next_session.fetch_add(1, Ordering::Relaxed);
                {
                    let mut guard = sessions.lock().await;
                    guard.insert(remote_addr, session_id);
                }

                let connection_runtime = runtime.clone();
                let connection_shutdown = shutdown.clone();
                let connection_config = config.clone();
                let connection_sessions = Arc::clone(&sessions);
                let connection_accepted_rds = Arc::clone(&accepted_rds);
                tokio::spawn(async move {
                    handle_bmp_connection(
                        stream,
                        remote_addr,
                        session_id,
                        connection_config,
                        connection_runtime,
                        connection_sessions,
                        connection_accepted_rds,
                        connection_shutdown,
                    )
                    .await;
                });
            }
        }
    }
}

async fn handle_bmp_connection(
    stream: tokio::net::TcpStream,
    remote_addr: SocketAddr,
    session_id: u64,
    config: RoutingDynamicBmpConfig,
    runtime: DynamicRoutingRuntime,
    sessions: Arc<Mutex<HashMap<SocketAddr, u64>>>,
    accepted_rds: Arc<HashSet<u64>>,
    shutdown: CancellationToken,
) {
    tracing::info!("BMP exporter connected: {}", remote_addr);
    let mut framed = Framed::new(stream, BmpCodec::default());
    let mut initialized = false;
    let mut consecutive_decode_errors = 0_usize;
    loop {
        tokio::select! {
            _ = shutdown.cancelled() => {
                break;
            }
            next = framed.next() => {
                match next {
                    Some(Ok(message)) => {
                        consecutive_decode_errors = 0;
                        let BmpMessage::V3(message) = message else {
                            continue;
                        };
                        match bmp_session_decision(&message, &mut initialized) {
                            BmpSessionDecision::Process => {}
                            BmpSessionDecision::CloseMissingInitiation => {
                                tracing::warn!(
                                    "closing BMP exporter {} session {}: first message was not initiation",
                                    remote_addr,
                                    session_id
                                );
                                break;
                            }
                            BmpSessionDecision::CloseTermination => {
                                tracing::info!(
                                    "closing BMP exporter {} session {}: termination received",
                                    remote_addr,
                                    session_id
                                );
                                break;
                            }
                        }
                        process_bmp_message(
                            remote_addr,
                            session_id,
                            message,
                            &config,
                            &accepted_rds,
                            &runtime,
                        );
                    }
                    Some(Err(err)) => {
                        consecutive_decode_errors += 1;
                        tracing::warn!(
                            "BMP decode error from {} ({} consecutive): {:?}",
                            remote_addr,
                            consecutive_decode_errors,
                            err
                        );
                        if consecutive_decode_errors >= config.max_consecutive_decode_errors {
                            tracing::warn!(
                                "closing BMP exporter {} session {} after {} consecutive decode errors",
                                remote_addr,
                                session_id,
                                consecutive_decode_errors
                            );
                            break;
                        }
                        continue;
                    }
                    None => {
                        break;
                    }
                }
            }
        }
    }

    let cleanup_runtime = runtime.clone();
    let cleanup_sessions = Arc::clone(&sessions);
    let cleanup_keep = config.keep;
    tokio::spawn(async move {
        sleep(cleanup_keep).await;
        let should_clear = {
            let mut guard = cleanup_sessions.lock().await;
            if guard.get(&remote_addr).copied() == Some(session_id) {
                guard.remove(&remote_addr);
                true
            } else {
                false
            }
        };
        if should_clear {
            cleanup_runtime.clear_session(remote_addr, session_id);
            tracing::info!(
                "cleared dynamic BMP routes for exporter {} session {} after keep interval",
                remote_addr,
                session_id
            );
        }
    });
}

fn configure_receive_buffer(
    stream: &tokio::net::TcpStream,
    remote_addr: SocketAddr,
    requested: usize,
) {
    if requested == 0 {
        return;
    }

    let socket = SockRef::from(stream);
    if let Err(err) = socket.set_recv_buffer_size(requested) {
        tracing::warn!(
            "failed to set BMP receive buffer for exporter {} (requested {} bytes): {}",
            remote_addr,
            requested,
            err
        );
        return;
    }

    match socket.recv_buffer_size() {
        Ok(actual) if actual < requested => {
            tracing::warn!(
                "BMP receive buffer for exporter {} is below requested size: requested={} actual={}",
                remote_addr,
                requested,
                actual
            );
        }
        Ok(actual) => {
            tracing::info!(
                "BMP receive buffer configured for exporter {}: requested={} actual={}",
                remote_addr,
                requested,
                actual
            );
        }
        Err(err) => {
            tracing::warn!(
                "failed to read BMP receive buffer size for exporter {}: {}",
                remote_addr,
                err
            );
        }
    }
}

fn process_bmp_message(
    exporter: SocketAddr,
    session_id: u64,
    message: BmpMessageValue,
    config: &RoutingDynamicBmpConfig,
    accepted_rds: &HashSet<u64>,
    runtime: &DynamicRoutingRuntime,
) {
    match message {
        BmpMessageValue::PeerDownNotification(msg) => {
            let peer = peer_key(exporter, session_id, msg.peer_header());
            runtime.clear_peer(&peer);
        }
        BmpMessageValue::RouteMonitoring(msg) => {
            if let BgpMessage::Update(update) = msg.update_message() {
                let header = msg.peer_header();
                let peer = peer_key(exporter, session_id, header);
                let peer_rd = route_distinguisher_to_u64(header.rd());
                let l3vpn_peer = is_l3vpn_peer_type(header.peer_type());
                if l3vpn_peer && !is_rd_accepted(accepted_rds, peer_rd) {
                    return;
                }
                apply_update(
                    &peer,
                    header.peer_as(),
                    header.peer_type(),
                    peer_rd,
                    update,
                    config,
                    accepted_rds,
                    runtime,
                );
            }
        }
        _ => {}
    }
}

fn bmp_session_decision(message: &BmpMessageValue, initialized: &mut bool) -> BmpSessionDecision {
    if !*initialized {
        if matches!(message, BmpMessageValue::Initiation(_)) {
            *initialized = true;
            return BmpSessionDecision::Process;
        }
        return BmpSessionDecision::CloseMissingInitiation;
    }

    if matches!(message, BmpMessageValue::Termination(_)) {
        return BmpSessionDecision::CloseTermination;
    }

    BmpSessionDecision::Process
}

fn peer_key(exporter: SocketAddr, session_id: u64, header: &PeerHeader) -> DynamicRoutingPeerKey {
    DynamicRoutingPeerKey {
        exporter,
        session_id,
        peer_id: format!(
            "{:?}|{:?}|{:?}|{}|{}",
            header.peer_type(),
            header.rd(),
            header.address(),
            header.peer_as(),
            header.bgp_id()
        ),
    }
}

fn apply_update(
    peer: &DynamicRoutingPeerKey,
    peer_as: u32,
    peer_type: BmpPeerType,
    peer_rd: u64,
    update: &BgpUpdateMessage,
    config: &RoutingDynamicBmpConfig,
    accepted_rds: &HashSet<u64>,
    runtime: &DynamicRoutingRuntime,
) {
    let mut flow_next_hop: Option<IpAddr> = None;
    let mut as_path: Vec<u32> = Vec::new();
    let mut communities: Vec<u32> = Vec::new();
    let mut large_communities: Vec<(u32, u32, u32)> = Vec::new();

    for attribute in update.path_attributes() {
        match attribute.value() {
            PathAttributeValue::NextHop(next_hop) => {
                flow_next_hop = Some(IpAddr::V4(next_hop.next_hop()));
            }
            PathAttributeValue::AsPath(path) => {
                as_path = flatten_as_path(path);
            }
            PathAttributeValue::As4Path(path) => {
                as_path = flatten_as4_path(path);
            }
            PathAttributeValue::Communities(values) => {
                communities = values
                    .communities()
                    .iter()
                    .map(|value| value.value())
                    .collect();
            }
            PathAttributeValue::LargeCommunities(values) => {
                large_communities = values
                    .communities()
                    .iter()
                    .map(|value| {
                        (
                            value.global_admin(),
                            value.local_data1(),
                            value.local_data2(),
                        )
                    })
                    .collect();
            }
            _ => {}
        }
    }

    let asn = if config.collect_asns {
        as_path.last().copied().unwrap_or(peer_as)
    } else {
        0
    };
    if !config.collect_as_paths {
        as_path.clear();
    }
    if !config.collect_communities {
        communities.clear();
        large_communities.clear();
    }

    if is_l3vpn_peer_type(peer_type) || is_rd_accepted(accepted_rds, 0) {
        for (prefix, path_id) in update.nlri().iter().map(ipv4_unicast_to_prefix) {
            let route_key = format!("ipv4-unicast|path_id={}", path_id_component(path_id));
            runtime.upsert(DynamicRoutingUpdate {
                peer: peer.clone(),
                prefix,
                route_key,
                next_hop: flow_next_hop,
                asn,
                as_path: as_path.clone(),
                communities: communities.clone(),
                large_communities: large_communities.clone(),
            });
        }

        for (prefix, path_id) in update.withdraw_routes().iter().map(ipv4_unicast_to_prefix) {
            runtime.withdraw(
                peer,
                prefix,
                &format!("ipv4-unicast|path_id={}", path_id_component(path_id)),
            );
        }
    }

    for attribute in update.path_attributes() {
        match attribute.value() {
            PathAttributeValue::MpReach(reach) => {
                let (next_hop, routes) = mp_reach_to_routes(reach, peer_rd);
                for route in routes {
                    if !is_l3vpn_peer_type(peer_type) && !is_rd_accepted(accepted_rds, route.rd) {
                        continue;
                    }
                    runtime.upsert(DynamicRoutingUpdate {
                        peer: peer.clone(),
                        prefix: route.prefix,
                        route_key: route.route_key,
                        next_hop,
                        asn,
                        as_path: as_path.clone(),
                        communities: communities.clone(),
                        large_communities: large_communities.clone(),
                    });
                }
            }
            PathAttributeValue::MpUnreach(unreach) => {
                for route in mp_unreach_to_routes(unreach, peer_rd) {
                    if !is_l3vpn_peer_type(peer_type) && !is_rd_accepted(accepted_rds, route.rd) {
                        continue;
                    }
                    runtime.withdraw(peer, route.prefix, &route.route_key);
                }
            }
            _ => {}
        }
    }
}

fn flatten_as_path(path: &AsPath) -> Vec<u32> {
    match path {
        AsPath::As2PathSegments(segments) => {
            let mut flattened = Vec::new();
            for segment in segments {
                match segment.segment_type() {
                    AsPathSegmentType::AsSet => {
                        if let Some(first) = segment.as_numbers().first() {
                            flattened.push(u32::from(*first));
                        }
                    }
                    AsPathSegmentType::AsSequence => {
                        flattened.extend(segment.as_numbers().iter().copied().map(u32::from))
                    }
                }
            }
            flattened
        }
        AsPath::As4PathSegments(segments) => {
            let mut flattened = Vec::new();
            for segment in segments {
                match segment.segment_type() {
                    AsPathSegmentType::AsSet => {
                        if let Some(first) = segment.as_numbers().first() {
                            flattened.push(*first);
                        }
                    }
                    AsPathSegmentType::AsSequence => {
                        flattened.extend(segment.as_numbers().iter().copied())
                    }
                }
            }
            flattened
        }
    }
}

fn flatten_as4_path(path: &As4Path) -> Vec<u32> {
    let mut flattened = Vec::new();
    for segment in path.segments() {
        match segment.segment_type() {
            AsPathSegmentType::AsSet => {
                if let Some(first) = segment.as_numbers().first() {
                    flattened.push(*first);
                }
            }
            AsPathSegmentType::AsSequence => flattened.extend(segment.as_numbers().iter().copied()),
        }
    }
    flattened
}

fn ipv4_unicast_to_prefix(
    value: &netgauze_bgp_pkt::nlri::Ipv4UnicastAddress,
) -> (IpNet, Option<u32>) {
    (IpNet::V4(value.network().address()), value.path_id())
}

fn mp_reach_to_routes(reach: &MpReach, peer_rd: u64) -> (Option<IpAddr>, Vec<NlriRoute>) {
    match reach {
        MpReach::Ipv4Unicast { next_hop, nlri, .. } => {
            (Some(*next_hop), ipv4_unicast_routes(nlri, peer_rd))
        }
        MpReach::Ipv6Unicast {
            next_hop_global,
            nlri,
            ..
        } => (
            Some(IpAddr::V6(*next_hop_global)),
            ipv6_unicast_routes(nlri, peer_rd),
        ),
        MpReach::Ipv4NlriMplsLabels { next_hop, nlri, .. } => {
            (Some(*next_hop), ipv4_mpls_label_routes(nlri, peer_rd))
        }
        MpReach::Ipv6NlriMplsLabels { next_hop, nlri, .. } => {
            (Some(*next_hop), ipv6_mpls_label_routes(nlri, peer_rd))
        }
        MpReach::Ipv4MplsVpnUnicast { next_hop, nlri } => {
            (Some(next_hop.next_hop()), ipv4_mpls_vpn_routes(nlri))
        }
        MpReach::Ipv6MplsVpnUnicast { next_hop, nlri } => {
            (Some(next_hop.next_hop()), ipv6_mpls_vpn_routes(nlri))
        }
        MpReach::L2Evpn { next_hop, nlri } => (Some(*next_hop), l2_evpn_routes(nlri)),
        _ => (None, Vec::new()),
    }
}

fn mp_unreach_to_routes(unreach: &MpUnreach, peer_rd: u64) -> Vec<NlriRoute> {
    match unreach {
        MpUnreach::Ipv4Unicast { nlri } => ipv4_unicast_routes(nlri, peer_rd),
        MpUnreach::Ipv6Unicast { nlri } => ipv6_unicast_routes(nlri, peer_rd),
        MpUnreach::Ipv4NlriMplsLabels { nlri } => ipv4_mpls_label_routes(nlri, peer_rd),
        MpUnreach::Ipv6NlriMplsLabels { nlri } => ipv6_mpls_label_routes(nlri, peer_rd),
        MpUnreach::Ipv4MplsVpnUnicast { nlri } => ipv4_mpls_vpn_routes(nlri),
        MpUnreach::Ipv6MplsVpnUnicast { nlri } => ipv6_mpls_vpn_routes(nlri),
        MpUnreach::L2Evpn { nlri } => l2_evpn_routes(nlri),
        _ => Vec::new(),
    }
}

fn ipv4_unicast_routes(
    nlri: &[netgauze_bgp_pkt::nlri::Ipv4UnicastAddress],
    rd: u64,
) -> Vec<NlriRoute> {
    nlri.iter()
        .map(|value| NlriRoute {
            prefix: IpNet::V4(value.network().address()),
            route_key: format!(
                "ipv4-unicast|path_id={}",
                path_id_component(value.path_id())
            ),
            rd,
        })
        .collect()
}

fn ipv6_unicast_routes(
    nlri: &[netgauze_bgp_pkt::nlri::Ipv6UnicastAddress],
    rd: u64,
) -> Vec<NlriRoute> {
    nlri.iter()
        .map(|value| NlriRoute {
            prefix: IpNet::V6(value.network().address()),
            route_key: format!(
                "ipv6-unicast|path_id={}",
                path_id_component(value.path_id())
            ),
            rd,
        })
        .collect()
}

fn ipv4_mpls_label_routes(
    nlri: &[netgauze_bgp_pkt::nlri::Ipv4NlriMplsLabelsAddress],
    rd: u64,
) -> Vec<NlriRoute> {
    nlri.iter()
        .map(|value| NlriRoute {
            prefix: IpNet::V4(value.prefix()),
            route_key: format!(
                "ipv4-nlri-mpls-labels|path_id={}",
                path_id_component(value.path_id())
            ),
            rd,
        })
        .collect()
}

fn ipv6_mpls_label_routes(
    nlri: &[netgauze_bgp_pkt::nlri::Ipv6NlriMplsLabelsAddress],
    rd: u64,
) -> Vec<NlriRoute> {
    nlri.iter()
        .map(|value| NlriRoute {
            prefix: IpNet::V6(value.prefix()),
            route_key: format!(
                "ipv6-nlri-mpls-labels|path_id={}",
                path_id_component(value.path_id())
            ),
            rd,
        })
        .collect()
}

fn ipv4_mpls_vpn_routes(
    nlri: &[netgauze_bgp_pkt::nlri::Ipv4MplsVpnUnicastAddress],
) -> Vec<NlriRoute> {
    nlri.iter()
        .map(|value| NlriRoute {
            prefix: IpNet::V4(value.network().address()),
            route_key: format!(
                "ipv4-mpls-vpn-unicast|path_id={}|rd={}",
                path_id_component(value.path_id()),
                value.rd()
            ),
            rd: route_distinguisher_to_u64(Some(value.rd())),
        })
        .collect()
}

fn ipv6_mpls_vpn_routes(
    nlri: &[netgauze_bgp_pkt::nlri::Ipv6MplsVpnUnicastAddress],
) -> Vec<NlriRoute> {
    nlri.iter()
        .map(|value| NlriRoute {
            prefix: IpNet::V6(value.network().address()),
            route_key: format!(
                "ipv6-mpls-vpn-unicast|path_id={}|rd={}",
                path_id_component(value.path_id()),
                value.rd()
            ),
            rd: route_distinguisher_to_u64(Some(value.rd())),
        })
        .collect()
}

fn l2_evpn_routes(nlri: &[L2EvpnAddress]) -> Vec<NlriRoute> {
    nlri.iter()
        .filter_map(|value| match value.route() {
            L2EvpnRoute::IpPrefixRoute(L2EvpnIpPrefixRoute::V4(route)) => Some(NlriRoute {
                prefix: IpNet::V4(route.prefix()),
                route_key: format!(
                    "l2-evpn-ip-prefix|path_id={}|rd={}|ip_version=4",
                    path_id_component(value.path_id()),
                    route.rd()
                ),
                rd: route_distinguisher_to_u64(Some(route.rd())),
            }),
            L2EvpnRoute::IpPrefixRoute(L2EvpnIpPrefixRoute::V6(route)) => Some(NlriRoute {
                prefix: IpNet::V6(route.prefix()),
                route_key: format!(
                    "l2-evpn-ip-prefix|path_id={}|rd={}|ip_version=6",
                    path_id_component(value.path_id()),
                    route.rd()
                ),
                rd: route_distinguisher_to_u64(Some(route.rd())),
            }),
            _ => None,
        })
        .collect()
}

fn path_id_component(path_id: Option<u32>) -> String {
    path_id
        .map(|value| value.to_string())
        .unwrap_or_else(|| "0".to_string())
}

fn is_l3vpn_peer_type(peer_type: BmpPeerType) -> bool {
    matches!(peer_type, BmpPeerType::RdInstancePeer { .. })
}

fn route_distinguisher_to_u64(rd: Option<RouteDistinguisher>) -> u64 {
    rd.map(u64::from).unwrap_or(0)
}

fn is_rd_accepted(accepted_rds: &HashSet<u64>, rd: u64) -> bool {
    accepted_rds.is_empty() || accepted_rds.contains(&rd)
}

fn parse_configured_rds(values: &[RouteDistinguisherConfig]) -> Result<HashSet<u64>> {
    let mut accepted_rds = HashSet::new();
    for value in values {
        let parsed = match value {
            RouteDistinguisherConfig::Numeric(raw) => *raw,
            RouteDistinguisherConfig::Text(raw) => parse_rd_text(raw)?,
        };
        accepted_rds.insert(parsed);
    }
    Ok(accepted_rds)
}

fn parse_rd_text(input: &str) -> Result<u64> {
    let parts: Vec<&str> = input.split(':').collect();
    match parts.len() {
        1 => Ok(parts[0]
            .parse::<u64>()
            .with_context(|| format!("cannot parse route distinguisher '{input}' as u64"))?),
        2 => parse_rd_parts(None, parts[0], parts[1], input),
        3 => {
            let rd_type = parts[0]
                .parse::<u8>()
                .with_context(|| format!("cannot parse route distinguisher type in '{input}'"))?;
            if rd_type > 2 {
                anyhow::bail!("route distinguisher type in '{input}' must be 0, 1, or 2");
            }
            parse_rd_parts(Some(rd_type), parts[1], parts[2], input)
        }
        _ => anyhow::bail!("cannot parse route distinguisher '{input}'"),
    }
}

fn parse_rd_parts(rd_type: Option<u8>, left: &str, right: &str, input: &str) -> Result<u64> {
    if rd_type == Some(1) || (rd_type.is_none() && left.contains('.')) {
        let ip = left
            .parse::<Ipv4Addr>()
            .with_context(|| format!("cannot parse route distinguisher '{input}' as IPv4:index"))?;
        let index = right
            .parse::<u16>()
            .with_context(|| format!("cannot parse route distinguisher '{input}' as IPv4:index"))?;
        let ip_u32 = u32::from(ip) as u64;
        return Ok(((1_u64) << 48) | (ip_u32 << 16) | u64::from(index));
    }

    let asn = left
        .parse::<u32>()
        .with_context(|| format!("cannot parse route distinguisher '{input}' as ASN:index"))?;
    let index_u32 = right
        .parse::<u32>()
        .with_context(|| format!("cannot parse route distinguisher '{input}' as ASN:index"))?;

    if rd_type == Some(0) && asn > u32::from(u16::MAX) {
        anyhow::bail!("cannot parse route distinguisher '{input}' as type-0 ASN2:index");
    }

    if asn <= u32::from(u16::MAX) && rd_type != Some(2) {
        return Ok(((0_u64) << 48) | ((asn as u64) << 32) | u64::from(index_u32));
    }

    let index_u16 = u16::try_from(index_u32).with_context(|| {
        format!("cannot parse route distinguisher '{input}' as type-2 ASN4:index")
    })?;
    Ok(((2_u64) << 48) | ((asn as u64) << 16) | u64::from(index_u16))
}

#[cfg(test)]
mod tests {
    use super::*;
    use ipnet::{Ipv4Net, Ipv6Net};
    use netgauze_bgp_pkt::nlri::{
        EthernetSegmentIdentifier, EthernetTag, Ipv4MplsVpnUnicastAddress,
        Ipv4NlriMplsLabelsAddress, Ipv4Unicast, L2EvpnIpv4PrefixRoute, L2EvpnIpv6PrefixRoute,
        MplsLabel,
    };
    use netgauze_bgp_pkt::path_attribute::{As2PathSegment, As4PathSegment};

    #[test]
    fn parse_rd_text_accepts_akvorado_formats() {
        assert_eq!(parse_rd_text("0").expect("parse rd 0"), 0);
        assert_eq!(
            parse_rd_text("65000:100").expect("parse rd asn2"),
            ((0_u64) << 48) | ((65_000_u64) << 32) | 100
        );
        assert_eq!(
            parse_rd_text("192.0.2.1:100").expect("parse rd ipv4"),
            ((1_u64) << 48) | ((u32::from(Ipv4Addr::new(192, 0, 2, 1)) as u64) << 16) | 100
        );
        assert_eq!(
            parse_rd_text("2:650000:123").expect("parse rd asn4"),
            ((2_u64) << 48) | ((650_000_u64) << 16) | 123
        );
        assert_eq!(
            parse_rd_text("2:10:5").expect("parse typed rd"),
            ((2_u64) << 48) | ((10_u64) << 16) | 5
        );
    }

    #[test]
    fn parse_rd_text_rejects_invalid_inputs() {
        assert!(parse_rd_text("abc").is_err());
        assert!(parse_rd_text("3:1:2").is_err());
        assert!(parse_rd_text("0:70000:1").is_err());
        assert!(parse_rd_text("2:70000:99999").is_err());
        assert!(parse_rd_text("1:invalid:10").is_err());
    }

    #[test]
    fn parse_configured_rds_mixes_numeric_and_text() {
        let parsed = parse_configured_rds(&[
            RouteDistinguisherConfig::Numeric(0),
            RouteDistinguisherConfig::Text("65000:100".to_string()),
        ])
        .expect("parse configured rds");
        assert!(parsed.contains(&0));
        assert!(parsed.contains(&(((0_u64) << 48) | ((65_000_u64) << 32) | 100)));
    }

    #[test]
    fn l2_evpn_routes_extracts_only_ip_prefix_routes() {
        let rd_v4 = RouteDistinguisher::As2Administrator {
            asn2: 65_000,
            number: 100,
        };
        let rd_v6 = RouteDistinguisher::As4Administrator {
            asn4: 650_000,
            number: 12,
        };
        let segment_id = EthernetSegmentIdentifier([0; 10]);
        let tag = EthernetTag(7);
        let label = MplsLabel::new([0, 0, 0x01]);

        let v4_route = L2EvpnAddress::new(
            Some(42),
            L2EvpnRoute::IpPrefixRoute(L2EvpnIpPrefixRoute::V4(L2EvpnIpv4PrefixRoute::new(
                rd_v4,
                segment_id,
                tag,
                Ipv4Net::new(Ipv4Addr::new(198, 51, 100, 0), 24).expect("v4 prefix"),
                Ipv4Addr::new(198, 51, 100, 1),
                label,
            ))),
        );
        let v6_route = L2EvpnAddress::new(
            Some(7),
            L2EvpnRoute::IpPrefixRoute(L2EvpnIpPrefixRoute::V6(L2EvpnIpv6PrefixRoute::new(
                rd_v6,
                segment_id,
                tag,
                Ipv6Net::new("2001:db8::".parse().expect("v6 addr"), 64).expect("v6 prefix"),
                "2001:db8::1".parse().expect("v6 gateway"),
                label,
            ))),
        );
        let ignored_non_prefix = L2EvpnAddress::new(
            Some(100),
            L2EvpnRoute::Unknown {
                code: 1,
                value: vec![0],
            },
        );

        let routes = l2_evpn_routes(&[v4_route, v6_route, ignored_non_prefix]);
        assert_eq!(routes.len(), 2);
        assert_eq!(
            routes[0].prefix,
            IpNet::V4(Ipv4Net::new(Ipv4Addr::new(198, 51, 100, 0), 24).expect("v4 prefix"))
        );
        assert_eq!(routes[0].rd, route_distinguisher_to_u64(Some(rd_v4)));
        assert_eq!(
            routes[0].route_key,
            format!("l2-evpn-ip-prefix|path_id=42|rd={rd_v4}|ip_version=4")
        );
        assert_eq!(
            routes[1].prefix,
            IpNet::V6(Ipv6Net::new("2001:db8::".parse().expect("v6 addr"), 64).expect("v6 prefix"))
        );
        assert_eq!(routes[1].rd, route_distinguisher_to_u64(Some(rd_v6)));
        assert_eq!(
            routes[1].route_key,
            format!("l2-evpn-ip-prefix|path_id=7|rd={rd_v6}|ip_version=6")
        );
    }

    #[test]
    fn mp_reach_and_unreach_include_l2_evpn_ip_prefix_routes() {
        let rd = RouteDistinguisher::As2Administrator {
            asn2: 65_000,
            number: 55,
        };
        let route = L2EvpnAddress::new(
            Some(9),
            L2EvpnRoute::IpPrefixRoute(L2EvpnIpPrefixRoute::V4(L2EvpnIpv4PrefixRoute::new(
                rd,
                EthernetSegmentIdentifier([0; 10]),
                EthernetTag(0),
                Ipv4Net::new(Ipv4Addr::new(203, 0, 113, 0), 24).expect("v4 prefix"),
                Ipv4Addr::new(203, 0, 113, 1),
                MplsLabel::new([0, 0, 0x01]),
            ))),
        );
        let reach = MpReach::L2Evpn {
            next_hop: IpAddr::V4(Ipv4Addr::new(192, 0, 2, 1)),
            nlri: vec![route.clone()],
        };
        let unreach = MpUnreach::L2Evpn { nlri: vec![route] };

        let (next_hop, reached) = mp_reach_to_routes(&reach, 0);
        let withdrawn = mp_unreach_to_routes(&unreach, 0);

        assert_eq!(next_hop, Some(IpAddr::V4(Ipv4Addr::new(192, 0, 2, 1))));
        assert_eq!(reached.len(), 1);
        assert_eq!(withdrawn.len(), 1);
        assert_eq!(reached[0].prefix, withdrawn[0].prefix);
        assert_eq!(reached[0].route_key, withdrawn[0].route_key);
        assert_eq!(reached[0].rd, withdrawn[0].rd);
    }

    #[test]
    fn path_id_component_none_uses_zero_for_parity() {
        assert_eq!(path_id_component(None), "0");
        assert_eq!(path_id_component(Some(0)), "0");
        assert_eq!(path_id_component(Some(42)), "42");
    }

    #[test]
    fn mpls_route_keys_ignore_label_stack_for_parity() {
        let prefix = Ipv4Net::new(Ipv4Addr::new(198, 51, 100, 0), 24).expect("v4 prefix");
        let rd = RouteDistinguisher::As2Administrator {
            asn2: 65_000,
            number: 200,
        };

        let labeled_a =
            Ipv4NlriMplsLabelsAddress::from(Some(7), vec![MplsLabel::new([0, 0, 0x01])], prefix)
                .expect("nlri a");
        let labeled_b =
            Ipv4NlriMplsLabelsAddress::from(Some(7), vec![MplsLabel::new([0, 0, 0x21])], prefix)
                .expect("nlri b");
        let key_labeled_a = ipv4_mpls_label_routes(&[labeled_a], 0)
            .into_iter()
            .next()
            .expect("route a")
            .route_key;
        let key_labeled_b = ipv4_mpls_label_routes(&[labeled_b], 0)
            .into_iter()
            .next()
            .expect("route b")
            .route_key;
        assert_eq!(key_labeled_a, key_labeled_b);

        let network = Ipv4Unicast::from_net(prefix).expect("unicast network");
        let vpn_a = Ipv4MplsVpnUnicastAddress::new(
            Some(9),
            rd,
            vec![MplsLabel::new([0, 0, 0x01])],
            network,
        );
        let vpn_b = Ipv4MplsVpnUnicastAddress::new(
            Some(9),
            rd,
            vec![MplsLabel::new([0, 0, 0x45])],
            network,
        );
        let key_vpn_a = ipv4_mpls_vpn_routes(&[vpn_a])
            .into_iter()
            .next()
            .expect("vpn route a")
            .route_key;
        let key_vpn_b = ipv4_mpls_vpn_routes(&[vpn_b])
            .into_iter()
            .next()
            .expect("vpn route b")
            .route_key;
        assert_eq!(key_vpn_a, key_vpn_b);
    }

    #[test]
    fn flatten_as_path_keeps_first_from_set_segments() {
        let as2_path = AsPath::As2PathSegments(vec![
            As2PathSegment::new(AsPathSegmentType::AsSequence, vec![65000, 65001]),
            As2PathSegment::new(AsPathSegmentType::AsSet, vec![65100, 65101, 65102]),
            As2PathSegment::new(AsPathSegmentType::AsSequence, vec![65200]),
        ]);
        assert_eq!(flatten_as_path(&as2_path), vec![65000, 65001, 65100, 65200]);

        let as4_path = AsPath::As4PathSegments(vec![
            As4PathSegment::new(AsPathSegmentType::AsSequence, vec![66000, 66001]),
            As4PathSegment::new(AsPathSegmentType::AsSet, vec![66100, 66101]),
        ]);
        assert_eq!(flatten_as_path(&as4_path), vec![66000, 66001, 66100]);
    }

    #[test]
    fn flatten_as4_path_keeps_first_from_set_segments() {
        let path = As4Path::new(vec![
            As4PathSegment::new(AsPathSegmentType::AsSequence, vec![100, 200]),
            As4PathSegment::new(AsPathSegmentType::AsSet, vec![300, 400]),
        ]);
        assert_eq!(flatten_as4_path(&path), vec![100, 200, 300]);
    }

    #[test]
    fn bmp_session_requires_initiation_first() {
        let mut initialized = false;
        let decision = bmp_session_decision(
            &BmpMessageValue::Experimental251(vec![1, 2, 3]),
            &mut initialized,
        );
        assert_eq!(decision, BmpSessionDecision::CloseMissingInitiation);
        assert!(!initialized);
    }

    #[test]
    fn bmp_session_accepts_initiation_and_then_processes_messages() {
        let mut initialized = false;
        let initiation =
            BmpMessageValue::Initiation(netgauze_bmp_pkt::v3::InitiationMessage::new(Vec::new()));
        let termination =
            BmpMessageValue::Termination(netgauze_bmp_pkt::v3::TerminationMessage::new(Vec::new()));

        assert_eq!(
            bmp_session_decision(&initiation, &mut initialized),
            BmpSessionDecision::Process
        );
        assert!(initialized);
        assert_eq!(
            bmp_session_decision(&BmpMessageValue::Experimental252(vec![]), &mut initialized),
            BmpSessionDecision::Process
        );
        assert_eq!(
            bmp_session_decision(&termination, &mut initialized),
            BmpSessionDecision::CloseTermination
        );
    }
}
