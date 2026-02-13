use crate::enrichment::{DynamicRoutingPeerKey, DynamicRoutingRuntime, DynamicRoutingUpdate};
use crate::ingest::IngestMetrics;
use crate::plugin_config::{RoutingDynamicBiorisConfig, RoutingDynamicBiorisRisInstanceConfig};
use anyhow::{Context, Result};
use ipnet::{IpNet, Ipv4Net, Ipv6Net};
use std::collections::{HashMap, HashSet};
use std::net::{IpAddr, Ipv4Addr, Ipv6Addr, SocketAddr};
use std::sync::Arc;
use std::sync::atomic::Ordering;
use std::time::Duration;
use tokio::task::JoinSet;
use tokio::time::MissedTickBehavior;
use tokio_stream::StreamExt;
use tokio_util::sync::CancellationToken;
use tonic::transport::{Channel, ClientTlsConfig, Endpoint};

pub(crate) mod proto {
    pub(crate) mod bio {
        pub(crate) mod net {
            tonic::include_proto!("bio.net");
        }
        pub(crate) mod route {
            tonic::include_proto!("bio.route");
        }
        pub(crate) mod ris {
            tonic::include_proto!("bio.ris");
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

const OBSERVE_RETRY_MIN_INTERVAL: Duration = Duration::from_secs(1);
const OBSERVE_RETRY_MAX_INTERVAL: Duration = Duration::from_secs(30);
const OBSERVE_TASK_STOP_TIMEOUT: Duration = Duration::from_secs(2);

#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash)]
enum AfiSafi {
    Ipv4Unicast,
    Ipv6Unicast,
}

impl AfiSafi {
    fn as_proto(self) -> i32 {
        match self {
            Self::Ipv4Unicast => ProtoAfiSafi::IPv4Unicast as i32,
            Self::Ipv6Unicast => ProtoAfiSafi::IPv6Unicast as i32,
        }
    }

    fn as_str(self) -> &'static str {
        match self {
            Self::Ipv4Unicast => "ipv4-unicast",
            Self::Ipv6Unicast => "ipv6-unicast",
        }
    }

    fn as_observe_proto(self) -> i32 {
        match self {
            Self::Ipv4Unicast => ProtoObserveAfiSafi::IPv4Unicast as i32,
            Self::Ipv6Unicast => ProtoObserveAfiSafi::IPv6Unicast as i32,
        }
    }
}

#[derive(Debug, Clone)]
struct ObserveTarget {
    router: String,
    afisafi: AfiSafi,
}

struct ObserveTaskHandle {
    cancel: CancellationToken,
    handle: tokio::task::JoinHandle<()>,
}

pub(crate) async fn run_bioris_listener(
    config: RoutingDynamicBiorisConfig,
    runtime: DynamicRoutingRuntime,
    metrics: Arc<IngestMetrics>,
    shutdown: CancellationToken,
) -> Result<()> {
    if !config.enabled {
        return Ok(());
    }
    if config.ris_instances.is_empty() {
        tracing::warn!("BioRIS routing is enabled but no RIS instances are configured");
        return Ok(());
    }

    let mut tasks = JoinSet::new();
    for instance in config.ris_instances {
        let instance_runtime = runtime.clone();
        let instance_metrics = Arc::clone(&metrics);
        let instance_shutdown = shutdown.clone();
        let timeout = config.timeout;
        let refresh = config.refresh;
        let refresh_timeout = config.refresh_timeout;
        tasks.spawn(async move {
            run_instance_loop(
                instance,
                timeout,
                refresh,
                refresh_timeout,
                instance_runtime,
                instance_metrics,
                instance_shutdown,
            )
            .await;
        });
    }

    shutdown.cancelled().await;
    tasks.abort_all();
    while let Some(result) = tasks.join_next().await {
        if let Err(err) = result
            && !err.is_cancelled()
        {
            tracing::warn!("BioRIS routing task join error: {}", err);
        }
    }

    Ok(())
}

async fn run_instance_loop(
    instance: RoutingDynamicBiorisRisInstanceConfig,
    timeout: Duration,
    refresh: Duration,
    refresh_timeout: Duration,
    runtime: DynamicRoutingRuntime,
    metrics: Arc<IngestMetrics>,
    shutdown: CancellationToken,
) {
    let regular_interval = refresh.max(Duration::from_secs(10));
    let mut retry_interval = (regular_interval / 10).max(Duration::from_secs(1));
    let mut next_wait = Duration::ZERO;
    let mut ticker = tokio::time::interval(regular_interval);
    ticker.set_missed_tick_behavior(MissedTickBehavior::Skip);
    let mut known_peers: HashSet<DynamicRoutingPeerKey> = HashSet::new();
    let mut observe_tasks: HashMap<DynamicRoutingPeerKey, ObserveTaskHandle> = HashMap::new();

    loop {
        if next_wait.is_zero() {
            match refresh_instance_once(
                &instance,
                timeout,
                refresh_timeout,
                &runtime,
                &metrics,
                &shutdown,
            )
            .await
            {
                Ok(expected_targets) => {
                    metrics
                        .bioris_refresh_success
                        .fetch_add(1, Ordering::Relaxed);
                    for (peer, target) in &expected_targets {
                        if observe_tasks.contains_key(peer) {
                            continue;
                        }
                        let task = start_observe_task(
                            instance.clone(),
                            target.clone(),
                            peer.clone(),
                            timeout,
                            runtime.clone(),
                            Arc::clone(&metrics),
                            shutdown.clone(),
                        );
                        observe_tasks.insert(peer.clone(), task);
                    }

                    let expected_peers: HashSet<DynamicRoutingPeerKey> =
                        expected_targets.keys().cloned().collect();
                    for stale_peer in known_peers.difference(&expected_peers) {
                        if let Some(task) = observe_tasks.remove(stale_peer) {
                            stop_observe_task(stale_peer, task, &metrics).await;
                        }
                        runtime.clear_peer(stale_peer);
                    }
                    known_peers = expected_peers;
                    next_wait = regular_interval;
                    retry_interval = (regular_interval / 10).max(Duration::from_secs(1));
                }
                Err(err) => {
                    metrics
                        .bioris_refresh_errors
                        .fetch_add(1, Ordering::Relaxed);
                    tracing::warn!(
                        "BioRIS refresh failed for {}: {:#}",
                        instance.grpc_addr,
                        err
                    );
                    next_wait = retry_interval;
                    retry_interval = std::cmp::min(retry_interval * 2, regular_interval);
                }
            }
        }

        tokio::select! {
            _ = shutdown.cancelled() => break,
            _ = tokio::time::sleep(next_wait) => {
                next_wait = Duration::ZERO;
            }
            _ = ticker.tick() => {
                if next_wait > regular_interval {
                    next_wait = regular_interval;
                }
            }
        }
    }

    for (peer, task) in observe_tasks {
        stop_observe_task(&peer, task, &metrics).await;
    }
}

async fn refresh_instance_once(
    instance: &RoutingDynamicBiorisRisInstanceConfig,
    timeout: Duration,
    refresh_timeout: Duration,
    runtime: &DynamicRoutingRuntime,
    metrics: &IngestMetrics,
    shutdown: &CancellationToken,
) -> Result<HashMap<DynamicRoutingPeerKey, ObserveTarget>> {
    let mut client = connect_bioris_client(instance, timeout).await?;
    let routers_response = tokio::time::timeout(timeout, client.get_routers(GetRoutersRequest {}))
        .await
        .context("BioRIS GetRouters request timed out")?
        .context("BioRIS GetRouters request failed")?;

    let mut expected_targets: HashMap<DynamicRoutingPeerKey, ObserveTarget> = HashMap::new();
    for router in routers_response.into_inner().routers {
        if shutdown.is_cancelled() {
            return Ok(expected_targets);
        }

        let Some(router_ip) = parse_router_ip(&router.address) else {
            tracing::warn!(
                "skipping BioRIS router '{}' from {}: invalid address '{}'",
                router.sys_name,
                instance.grpc_addr,
                router.address
            );
            continue;
        };
        for afisafi in [AfiSafi::Ipv4Unicast, AfiSafi::Ipv6Unicast] {
            let routing_key = bioris_peer_key(instance, router_ip, afisafi);
            expected_targets.insert(
                routing_key.clone(),
                ObserveTarget {
                    router: router.address.clone(),
                    afisafi,
                },
            );
            if let Err(err) = dump_peer_routes(
                &mut client,
                instance,
                &router.address,
                afisafi,
                refresh_timeout,
                runtime,
                routing_key,
            )
            .await
            {
                metrics.bioris_dump_errors.fetch_add(1, Ordering::Relaxed);
                tracing::warn!(
                    "BioRIS dump failed for {} router {} afi {}: {:#}",
                    instance.grpc_addr,
                    router.address,
                    afisafi.as_str(),
                    err
                );
            } else {
                metrics.bioris_dump_success.fetch_add(1, Ordering::Relaxed);
            }
        }
    }

    Ok(expected_targets)
}

fn start_observe_task(
    instance: RoutingDynamicBiorisRisInstanceConfig,
    target: ObserveTarget,
    peer: DynamicRoutingPeerKey,
    timeout: Duration,
    runtime: DynamicRoutingRuntime,
    metrics: Arc<IngestMetrics>,
    shutdown: CancellationToken,
) -> ObserveTaskHandle {
    metrics
        .bioris_observe_stream_starts
        .fetch_add(1, Ordering::Relaxed);
    metrics
        .bioris_observe_streams_active
        .fetch_add(1, Ordering::Relaxed);
    let cancel = CancellationToken::new();
    let task_cancel = cancel.clone();
    let handle = tokio::spawn(async move {
        run_observe_loop(
            instance,
            target,
            peer,
            timeout,
            runtime,
            metrics,
            shutdown,
            task_cancel,
        )
        .await;
    });
    ObserveTaskHandle { cancel, handle }
}

async fn stop_observe_task(
    peer: &DynamicRoutingPeerKey,
    task: ObserveTaskHandle,
    metrics: &IngestMetrics,
) {
    task.cancel.cancel();
    match tokio::time::timeout(OBSERVE_TASK_STOP_TIMEOUT, task.handle).await {
        Ok(Ok(())) => {}
        Ok(Err(err)) if !err.is_cancelled() => {
            tracing::warn!(
                "BioRIS observe task join error for {}: {}",
                peer.peer_id,
                err
            );
        }
        Ok(Err(_)) => {}
        Err(_) => {
            tracing::warn!(
                "BioRIS observe task stop timed out for {}, aborting task",
                peer.peer_id
            );
        }
    }
    let _ = metrics.bioris_observe_streams_active.fetch_update(
        Ordering::Relaxed,
        Ordering::Relaxed,
        |current| Some(current.saturating_sub(1)),
    );
}

async fn run_observe_loop(
    instance: RoutingDynamicBiorisRisInstanceConfig,
    target: ObserveTarget,
    peer: DynamicRoutingPeerKey,
    timeout: Duration,
    runtime: DynamicRoutingRuntime,
    metrics: Arc<IngestMetrics>,
    shutdown: CancellationToken,
    task_cancel: CancellationToken,
) {
    let mut retry_interval = OBSERVE_RETRY_MIN_INTERVAL;
    loop {
        if shutdown.is_cancelled() || task_cancel.is_cancelled() {
            return;
        }

        match observe_peer_stream(
            &instance,
            &target,
            &peer,
            timeout,
            &runtime,
            &shutdown,
            &task_cancel,
        )
        .await
        {
            Ok(()) => {
                retry_interval = OBSERVE_RETRY_MIN_INTERVAL;
            }
            Err(err) => {
                metrics
                    .bioris_observe_stream_errors
                    .fetch_add(1, Ordering::Relaxed);
                tracing::warn!(
                    "BioRIS observe stream failed for {} router {} afi {}: {:#}",
                    instance.grpc_addr,
                    target.router,
                    target.afisafi.as_str(),
                    err
                );
            }
        }

        tokio::select! {
            _ = shutdown.cancelled() => return,
            _ = task_cancel.cancelled() => return,
            _ = tokio::time::sleep(retry_interval) => {}
        }
        metrics
            .bioris_observe_stream_reconnects
            .fetch_add(1, Ordering::Relaxed);
        retry_interval = std::cmp::min(retry_interval * 2, OBSERVE_RETRY_MAX_INTERVAL);
    }
}

async fn observe_peer_stream(
    instance: &RoutingDynamicBiorisRisInstanceConfig,
    target: &ObserveTarget,
    peer: &DynamicRoutingPeerKey,
    timeout: Duration,
    runtime: &DynamicRoutingRuntime,
    shutdown: &CancellationToken,
    task_cancel: &CancellationToken,
) -> Result<()> {
    let mut client = connect_bioris_client(instance, timeout).await?;
    let request = ObserveRibRequest {
        router: target.router.clone(),
        vrf_id: instance.vrf_id,
        vrf: instance.vrf.clone(),
        afisafi: target.afisafi.as_observe_proto(),
        allow_unready_rib: true,
    };
    let response = tokio::time::timeout(timeout, client.observe_rib(request))
        .await
        .with_context(|| {
            format!(
                "BioRIS ObserveRIB request timed out for router {} afi {}",
                target.router,
                target.afisafi.as_str()
            )
        })?
        .with_context(|| {
            format!(
                "BioRIS ObserveRIB request failed for router {} afi {}",
                target.router,
                target.afisafi.as_str()
            )
        })?;
    let mut stream = response.into_inner();

    loop {
        tokio::select! {
            _ = shutdown.cancelled() => return Ok(()),
            _ = task_cancel.cancelled() => return Ok(()),
            next = stream.next() => {
                match next {
                    Some(Ok(update)) => {
                        apply_observe_update(update, peer, target.afisafi, runtime);
                    }
                    Some(Err(err)) => {
                        return Err(anyhow::anyhow!(
                            "BioRIS ObserveRIB stream read failed for router {} afi {}: {}",
                            target.router,
                            target.afisafi.as_str(),
                            err
                        ));
                    }
                    None => return Ok(()),
                }
            }
        }
    }
}

fn apply_observe_update(
    update: RibUpdate,
    peer: &DynamicRoutingPeerKey,
    afisafi: AfiSafi,
    runtime: &DynamicRoutingRuntime,
) {
    if update.end_of_rib {
        return;
    }
    let Some(route) = update.route else {
        return;
    };

    if update.advertisement {
        if let Some(update) = route_to_update(route, peer, afisafi) {
            runtime.upsert(update);
        }
        return;
    }

    if let Some((prefix, route_keys)) = route_withdraw_keys(&route, afisafi) {
        for route_key in route_keys {
            runtime.withdraw(peer, prefix, &route_key);
        }
    }
}

async fn dump_peer_routes(
    client: &mut RoutingInformationServiceClient<Channel>,
    instance: &RoutingDynamicBiorisRisInstanceConfig,
    router: &str,
    afisafi: AfiSafi,
    refresh_timeout: Duration,
    runtime: &DynamicRoutingRuntime,
    peer: DynamicRoutingPeerKey,
) -> Result<()> {
    let request = DumpRibRequest {
        router: router.to_string(),
        vrf_id: instance.vrf_id,
        vrf: instance.vrf.clone(),
        afisafi: afisafi.as_proto(),
        filter: None,
    };

    let mut updates = Vec::new();
    let response = tokio::time::timeout(refresh_timeout, client.dump_rib(request))
        .await
        .with_context(|| {
            format!(
                "BioRIS DumpRIB request timed out for router {} afi {}",
                router,
                afisafi.as_str()
            )
        })?
        .with_context(|| {
            format!(
                "BioRIS DumpRIB request failed for router {} afi {}",
                router,
                afisafi.as_str()
            )
        })?;
    let mut stream = response.into_inner();

    loop {
        let next = tokio::time::timeout(refresh_timeout, stream.next())
            .await
            .with_context(|| {
                format!(
                    "BioRIS DumpRIB stream timed out for router {} afi {}",
                    router,
                    afisafi.as_str()
                )
            })?;
        let Some(next) = next else {
            break;
        };
        let reply = next.with_context(|| {
            format!(
                "BioRIS DumpRIB stream read failed for router {} afi {}",
                router,
                afisafi.as_str()
            )
        })?;
        if let Some(route) = reply.route
            && let Some(update) = route_to_update(route, &peer, afisafi)
        {
            updates.push(update);
        }
    }

    runtime.clear_peer(&peer);
    for update in updates {
        runtime.upsert(update);
    }

    Ok(())
}

async fn connect_bioris_client(
    instance: &RoutingDynamicBiorisRisInstanceConfig,
    timeout: Duration,
) -> Result<RoutingInformationServiceClient<Channel>> {
    let endpoint_uri = build_endpoint_uri(instance);
    let mut endpoint = Endpoint::from_shared(endpoint_uri.clone())
        .with_context(|| format!("invalid BioRIS endpoint URI: {}", endpoint_uri))?;
    endpoint = endpoint.connect_timeout(timeout);
    if instance.grpc_secure {
        endpoint = endpoint
            .tls_config(ClientTlsConfig::new().with_native_roots())
            .context("failed to configure BioRIS TLS settings")?;
    }
    let channel = endpoint
        .connect()
        .await
        .with_context(|| format!("failed to connect to BioRIS endpoint {}", endpoint_uri))?;
    Ok(RoutingInformationServiceClient::new(channel))
}

fn build_endpoint_uri(instance: &RoutingDynamicBiorisRisInstanceConfig) -> String {
    if instance.grpc_addr.contains("://") {
        instance.grpc_addr.clone()
    } else if instance.grpc_secure {
        format!("https://{}", instance.grpc_addr)
    } else {
        format!("http://{}", instance.grpc_addr)
    }
}

fn parse_router_ip(value: &str) -> Option<IpAddr> {
    if let Ok(ip) = value.trim().parse::<IpAddr>() {
        return Some(ip);
    }
    value
        .trim()
        .parse::<SocketAddr>()
        .ok()
        .map(|addr| addr.ip())
}

fn bioris_peer_key(
    instance: &RoutingDynamicBiorisRisInstanceConfig,
    router_ip: IpAddr,
    afisafi: AfiSafi,
) -> DynamicRoutingPeerKey {
    DynamicRoutingPeerKey {
        exporter: SocketAddr::new(router_ip, 0),
        session_id: 0,
        peer_id: format!(
            "bioris|grpc={}|router={}|vrf_id={}|vrf={}|afi={}",
            instance.grpc_addr,
            router_ip,
            instance.vrf_id,
            instance.vrf,
            afisafi.as_str()
        ),
    }
}

fn route_to_update(
    route: Route,
    peer: &DynamicRoutingPeerKey,
    afisafi: AfiSafi,
) -> Option<DynamicRoutingUpdate> {
    let prefix = proto_prefix_to_ipnet(route.pfx?)?;
    let best_path = route.paths.into_iter().next()?;
    let bgp = best_path.bgp_path?;

    let as_path = flatten_as_path(&bgp);
    let asn = as_path.last().copied().unwrap_or(0);
    let communities = bgp.communities;
    let large_communities = bgp
        .large_communities
        .into_iter()
        .map(|community| {
            (
                community.global_administrator,
                community.data_part1,
                community.data_part2,
            )
        })
        .collect::<Vec<_>>();
    let next_hop = bgp.next_hop.and_then(proto_ip_to_ip_addr);
    let route_key = route_key_for_path_id(afisafi, bgp.path_identifier);

    Some(DynamicRoutingUpdate {
        peer: peer.clone(),
        prefix,
        route_key,
        next_hop,
        asn,
        as_path,
        communities,
        large_communities,
    })
}

fn route_withdraw_keys(route: &Route, afisafi: AfiSafi) -> Option<(IpNet, Vec<String>)> {
    let prefix = proto_prefix_to_ipnet(route.pfx.clone()?)?;
    let mut path_ids: HashSet<u32> = HashSet::new();
    for path in &route.paths {
        if let Some(bgp_path) = &path.bgp_path {
            path_ids.insert(bgp_path.path_identifier);
        }
    }
    if path_ids.is_empty() {
        path_ids.insert(0);
    }

    let mut route_keys = path_ids
        .into_iter()
        .map(|path_id| route_key_for_path_id(afisafi, path_id))
        .collect::<Vec<_>>();
    route_keys.sort_unstable();
    Some((prefix, route_keys))
}

fn route_key_for_path_id(afisafi: AfiSafi, path_id: u32) -> String {
    format!("bioris|afi={}|path_id={path_id}", afisafi.as_str())
}

fn flatten_as_path(path: &BgpPath) -> Vec<u32> {
    let mut flattened = Vec::new();
    for segment in &path.as_path {
        flatten_as_segment(segment, &mut flattened);
    }
    flattened
}

fn flatten_as_segment(segment: &ProtoAsPathSegment, out: &mut Vec<u32>) {
    if segment.as_sequence {
        out.extend(segment.asns.iter().copied());
        return;
    }
    if let Some(first) = segment.asns.first() {
        out.push(*first);
    }
}

fn proto_prefix_to_ipnet(prefix: ProtoPrefix) -> Option<IpNet> {
    let address = proto_ip_to_ip_addr(prefix.address?)?;
    let length = u8::try_from(prefix.length).ok()?;
    match address {
        IpAddr::V4(addr) => Ipv4Net::new(addr, length).ok().map(IpNet::V4),
        IpAddr::V6(addr) => Ipv6Net::new(addr, length).ok().map(IpNet::V6),
    }
}

fn proto_ip_to_ip_addr(ip: ProtoIp) -> Option<IpAddr> {
    match ProtoIpVersion::try_from(ip.version).ok()? {
        ProtoIpVersion::IPv4 => {
            let raw = u32::try_from(ip.lower).ok()?;
            Some(IpAddr::V4(Ipv4Addr::from(raw)))
        }
        ProtoIpVersion::IPv6 => {
            let mut bytes = [0_u8; 16];
            bytes[..8].copy_from_slice(&ip.higher.to_be_bytes());
            bytes[8..].copy_from_slice(&ip.lower.to_be_bytes());
            Some(IpAddr::V6(Ipv6Addr::from(bytes)))
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn proto_ip_conversion_handles_ipv4_and_ipv6() {
        let v4 = ProtoIp {
            higher: 0,
            lower: u64::from(u32::from(Ipv4Addr::new(198, 51, 100, 10))),
            version: ProtoIpVersion::IPv4 as i32,
        };
        assert_eq!(
            proto_ip_to_ip_addr(v4),
            Some(IpAddr::V4(Ipv4Addr::new(198, 51, 100, 10)))
        );

        let v6_addr = Ipv6Addr::new(0x2001, 0xdb8, 0, 1, 0, 0, 0, 0x1234);
        let octets = v6_addr.octets();
        let v6 = ProtoIp {
            higher: u64::from_be_bytes(octets[..8].try_into().expect("first half")),
            lower: u64::from_be_bytes(octets[8..].try_into().expect("second half")),
            version: ProtoIpVersion::IPv6 as i32,
        };
        assert_eq!(proto_ip_to_ip_addr(v6), Some(IpAddr::V6(v6_addr)));
    }

    #[test]
    fn route_to_update_flattens_set_path_segments_like_akvorado() {
        let route = Route {
            pfx: Some(ProtoPrefix {
                address: Some(ProtoIp {
                    higher: 0,
                    lower: u64::from(u32::from(Ipv4Addr::new(203, 0, 113, 0))),
                    version: ProtoIpVersion::IPv4 as i32,
                }),
                length: 24,
            }),
            paths: vec![proto::bio::route::Path {
                r#type: 0,
                static_path: None,
                bgp_path: Some(BgpPath {
                    path_identifier: 77,
                    next_hop: Some(ProtoIp {
                        higher: 0,
                        lower: u64::from(u32::from(Ipv4Addr::new(198, 51, 100, 1))),
                        version: ProtoIpVersion::IPv4 as i32,
                    }),
                    local_pref: 0,
                    as_path: vec![
                        ProtoAsPathSegment {
                            as_sequence: true,
                            asns: vec![64500, 64501],
                        },
                        ProtoAsPathSegment {
                            as_sequence: false,
                            asns: vec![64600, 64601],
                        },
                    ],
                    origin: 0,
                    med: 0,
                    ebgp: false,
                    bgp_identifier: 0,
                    source: None,
                    communities: vec![100, 200],
                    large_communities: vec![proto::bio::route::LargeCommunity {
                        global_administrator: 64500,
                        data_part1: 10,
                        data_part2: 20,
                    }],
                    originator_id: 0,
                    cluster_list: vec![],
                    unknown_attributes: vec![],
                    bmp_post_policy: false,
                    only_to_customer: 0,
                }),
                hidden_reason: 0,
                time_learned: 0,
                grp_path: None,
            }],
        };
        let peer = DynamicRoutingPeerKey {
            exporter: SocketAddr::new(IpAddr::V4(Ipv4Addr::new(203, 0, 113, 1)), 0),
            session_id: 0,
            peer_id: "peer".to_string(),
        };
        let update = route_to_update(route, &peer, AfiSafi::Ipv4Unicast).expect("expected update");
        assert_eq!(update.as_path, vec![64500, 64501, 64600]);
        assert_eq!(update.asn, 64600);
        assert_eq!(
            update.next_hop,
            Some(IpAddr::V4(Ipv4Addr::new(198, 51, 100, 1)))
        );
        assert_eq!(update.communities, vec![100, 200]);
        assert_eq!(update.large_communities, vec![(64500, 10, 20)]);
        assert_eq!(update.route_key, "bioris|afi=ipv4-unicast|path_id=77");
    }

    #[test]
    fn route_withdraw_keys_include_all_bgp_path_ids() {
        let route = Route {
            pfx: Some(ProtoPrefix {
                address: Some(ProtoIp {
                    higher: 0,
                    lower: u64::from(u32::from(Ipv4Addr::new(203, 0, 113, 0))),
                    version: ProtoIpVersion::IPv4 as i32,
                }),
                length: 24,
            }),
            paths: vec![
                proto::bio::route::Path {
                    r#type: 0,
                    static_path: None,
                    bgp_path: Some(BgpPath {
                        path_identifier: 7,
                        next_hop: None,
                        local_pref: 0,
                        as_path: vec![],
                        origin: 0,
                        med: 0,
                        ebgp: false,
                        bgp_identifier: 0,
                        source: None,
                        communities: vec![],
                        large_communities: vec![],
                        originator_id: 0,
                        cluster_list: vec![],
                        unknown_attributes: vec![],
                        bmp_post_policy: false,
                        only_to_customer: 0,
                    }),
                    hidden_reason: 0,
                    time_learned: 0,
                    grp_path: None,
                },
                proto::bio::route::Path {
                    r#type: 0,
                    static_path: None,
                    bgp_path: Some(BgpPath {
                        path_identifier: 42,
                        next_hop: None,
                        local_pref: 0,
                        as_path: vec![],
                        origin: 0,
                        med: 0,
                        ebgp: false,
                        bgp_identifier: 0,
                        source: None,
                        communities: vec![],
                        large_communities: vec![],
                        originator_id: 0,
                        cluster_list: vec![],
                        unknown_attributes: vec![],
                        bmp_post_policy: false,
                        only_to_customer: 0,
                    }),
                    hidden_reason: 0,
                    time_learned: 0,
                    grp_path: None,
                },
            ],
        };

        let (prefix, keys) = route_withdraw_keys(&route, AfiSafi::Ipv4Unicast).expect("keys");
        assert_eq!(
            prefix,
            "203.0.113.0/24".parse::<IpNet>().expect("expected prefix")
        );
        assert_eq!(
            keys,
            vec![
                "bioris|afi=ipv4-unicast|path_id=42".to_string(),
                "bioris|afi=ipv4-unicast|path_id=7".to_string(),
            ]
        );
    }

    #[test]
    fn route_withdraw_keys_fallback_to_path_id_zero() {
        let route = Route {
            pfx: Some(ProtoPrefix {
                address: Some(ProtoIp {
                    higher: 0,
                    lower: u64::from(u32::from(Ipv4Addr::new(198, 51, 100, 0))),
                    version: ProtoIpVersion::IPv4 as i32,
                }),
                length: 24,
            }),
            paths: vec![proto::bio::route::Path {
                r#type: 0,
                static_path: None,
                bgp_path: None,
                hidden_reason: 0,
                time_learned: 0,
                grp_path: None,
            }],
        };

        let (_, keys) = route_withdraw_keys(&route, AfiSafi::Ipv4Unicast).expect("keys");
        assert_eq!(keys, vec!["bioris|afi=ipv4-unicast|path_id=0".to_string()]);
    }

    #[test]
    fn peer_key_stability_is_deterministic() {
        let instance = RoutingDynamicBiorisRisInstanceConfig {
            grpc_addr: "127.0.0.1:50051".to_string(),
            grpc_secure: false,
            vrf_id: 10,
            vrf: "default".to_string(),
        };
        let peer_a = bioris_peer_key(
            &instance,
            IpAddr::V4(Ipv4Addr::new(192, 0, 2, 10)),
            AfiSafi::Ipv4Unicast,
        );
        let peer_b = bioris_peer_key(
            &instance,
            IpAddr::V4(Ipv4Addr::new(192, 0, 2, 10)),
            AfiSafi::Ipv4Unicast,
        );
        assert_eq!(peer_a, peer_b);
    }
}
