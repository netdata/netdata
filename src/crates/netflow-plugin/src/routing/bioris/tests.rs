use super::client::{bioris_peer_key, build_endpoint_uri, connect_bioris_client, parse_router_ip};
use super::route::{proto_ip_to_ip_addr, route_to_update, route_withdraw_keys};
use super::runtime::AfiSafi;
use super::*;
use crate::ingest::IngestMetrics;
use crate::plugin_config::RoutingDynamicBiorisConfig;
use std::sync::Arc;
use std::sync::Mutex;
use tokio::net::TcpListener;
use tokio::sync::mpsc;
use tokio_stream::wrappers::{ReceiverStream, TcpListenerStream};
use tokio_util::sync::CancellationToken;
use tonic::transport::Server;

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
fn proto_ip_conversion_rejects_unspecified_version() {
    let ip = ProtoIp {
        higher: 0,
        lower: 0,
        version: ProtoIpVersion::Unspecified as i32,
    };

    assert_eq!(proto_ip_to_ip_addr(ip), None);
}

#[test]
fn endpoint_uri_respects_explicit_scheme_and_grpc_secure() {
    let plain = RoutingDynamicBiorisRisInstanceConfig {
        grpc_addr: "127.0.0.1:50051".to_string(),
        grpc_secure: false,
        vrf_id: 0,
        vrf: String::new(),
    };
    assert_eq!(build_endpoint_uri(&plain), "http://127.0.0.1:50051");

    let secure = RoutingDynamicBiorisRisInstanceConfig {
        grpc_secure: true,
        ..plain.clone()
    };
    assert_eq!(build_endpoint_uri(&secure), "https://127.0.0.1:50051");

    let explicit = RoutingDynamicBiorisRisInstanceConfig {
        grpc_addr: "http://ris.example.test:50051".to_string(),
        grpc_secure: true,
        vrf_id: 0,
        vrf: String::new(),
    };
    assert_eq!(
        build_endpoint_uri(&explicit),
        "http://ris.example.test:50051"
    );
}

#[test]
fn parse_router_ip_accepts_plain_ips_and_socket_addresses() {
    assert_eq!(
        parse_router_ip(" 203.0.113.10 "),
        Some(IpAddr::V4(Ipv4Addr::new(203, 0, 113, 10)))
    );
    assert_eq!(
        parse_router_ip("203.0.113.10:179"),
        Some(IpAddr::V4(Ipv4Addr::new(203, 0, 113, 10)))
    );
    assert_eq!(
        parse_router_ip("[2001:db8::1]:179"),
        Some(IpAddr::V6("2001:db8::1".parse().expect("parse v6")))
    );
    assert_eq!(parse_router_ip("not an address"), None);
}

#[tokio::test]
async fn connect_bioris_client_reports_invalid_endpoint_uri() {
    let instance = RoutingDynamicBiorisRisInstanceConfig {
        grpc_addr: "http://[::1".to_string(),
        grpc_secure: false,
        vrf_id: 0,
        vrf: String::new(),
    };

    let err = connect_bioris_client(&instance, Duration::from_millis(50))
        .await
        .expect_err("invalid URI should fail before any network connection");
    assert!(
        format!("{err:#}").contains("invalid BioRIS endpoint URI"),
        "unexpected error: {err:#}"
    );
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

#[tokio::test]
async fn bioris_listener_fetches_dump_rib_from_in_process_grpc_server() {
    let server_listener = TcpListener::bind("127.0.0.1:0")
        .await
        .expect("bind BioRIS fixture server");
    let server_addr = server_listener
        .local_addr()
        .expect("read BioRIS fixture server address");
    let server_shutdown = CancellationToken::new();
    let server_shutdown_task = server_shutdown.clone();
    let service = TestRisService::new(vec![test_bioris_route()], None);
    let server = tokio::spawn(async move {
        Server::builder()
            .add_service(
                proto::bio::ris::routing_information_service_server::RoutingInformationServiceServer::new(
                    service,
                ),
            )
            .serve_with_incoming_shutdown(
                TcpListenerStream::new(server_listener),
                server_shutdown_task.cancelled(),
            )
            .await
    });

    let runtime = DynamicRoutingRuntime::default();
    let metrics = Arc::new(IngestMetrics::default());
    let shutdown = CancellationToken::new();
    let listener_runtime = runtime.clone();
    let listener_metrics = Arc::clone(&metrics);
    let listener_shutdown = shutdown.clone();
    let listener = tokio::spawn(async move {
        run_bioris_listener(
            RoutingDynamicBiorisConfig {
                enabled: true,
                ris_instances: vec![RoutingDynamicBiorisRisInstanceConfig {
                    grpc_addr: server_addr.to_string(),
                    grpc_secure: false,
                    vrf_id: 10,
                    vrf: "default".to_string(),
                }],
                timeout: Duration::from_secs(1),
                refresh: Duration::from_secs(10),
                refresh_timeout: Duration::from_secs(1),
            },
            listener_runtime,
            listener_metrics,
            listener_shutdown,
        )
        .await
    });

    tokio::time::timeout(Duration::from_secs(3), async {
        loop {
            if let Some(route) = runtime.lookup(
                "203.0.113.42".parse().expect("parse route lookup address"),
                Some("198.51.100.1".parse().expect("parse next-hop")),
                None,
            ) {
                assert_eq!(route.asn, 64_501);
                assert_eq!(route.as_path, vec![64_500, 64_501]);
                assert_eq!(route.communities, vec![100, 200]);
                assert_eq!(route.large_communities.len(), 1);
                assert_eq!(route.large_communities[0].asn, 64_500);
                assert_eq!(route.large_communities[0].local_data1, 10);
                assert_eq!(route.large_communities[0].local_data2, 20);
                break;
            }
            tokio::time::sleep(Duration::from_millis(25)).await;
        }
    })
    .await
    .expect("BioRIS listener did not publish DumpRIB route");

    shutdown.cancel();
    listener
        .await
        .expect("join BioRIS listener")
        .expect("BioRIS listener should stop cleanly");
    server_shutdown.cancel();
    server
        .await
        .expect("join BioRIS fixture server")
        .expect("BioRIS fixture server should stop cleanly");
}

#[tokio::test]
async fn bioris_listener_sends_configured_vrf_and_applies_observe_updates() {
    let server_listener = TcpListener::bind("127.0.0.1:0")
        .await
        .expect("bind BioRIS fixture server");
    let server_addr = server_listener
        .local_addr()
        .expect("read BioRIS fixture server address");
    let (observe_tx, observe_rx) = mpsc::channel(8);
    let service = TestRisService::new(vec![test_bioris_route()], Some(observe_rx));
    let dump_requests = service.dump_requests();
    let observe_requests = service.observe_requests();
    let server_shutdown = CancellationToken::new();
    let server_shutdown_task = server_shutdown.clone();
    let server = tokio::spawn(async move {
        Server::builder()
            .add_service(
                proto::bio::ris::routing_information_service_server::RoutingInformationServiceServer::new(
                    service,
                ),
            )
            .serve_with_incoming_shutdown(
                TcpListenerStream::new(server_listener),
                server_shutdown_task.cancelled(),
            )
            .await
    });

    let runtime = DynamicRoutingRuntime::default();
    let metrics = Arc::new(IngestMetrics::default());
    let shutdown = CancellationToken::new();
    let listener_runtime = runtime.clone();
    let listener_metrics = Arc::clone(&metrics);
    let listener_shutdown = shutdown.clone();
    let listener = tokio::spawn(async move {
        run_bioris_listener(
            RoutingDynamicBiorisConfig {
                enabled: true,
                ris_instances: vec![RoutingDynamicBiorisRisInstanceConfig {
                    grpc_addr: server_addr.to_string(),
                    grpc_secure: false,
                    vrf_id: 42,
                    vrf: "blue".to_string(),
                }],
                timeout: Duration::from_secs(1),
                refresh: Duration::from_secs(10),
                refresh_timeout: Duration::from_secs(1),
            },
            listener_runtime,
            listener_metrics,
            listener_shutdown,
        )
        .await
    });

    wait_for_route_asn(&runtime, "203.0.113.42", 64_501)
        .await
        .expect("initial DumpRIB route should be published");

    let observe_route = test_bioris_observe_route();
    observe_tx
        .send(Ok(RibUpdate {
            advertisement: true,
            is_initial_dump: false,
            end_of_rib: false,
            route: Some(observe_route.clone()),
        }))
        .await
        .expect("send observe advertisement");
    let observed = wait_for_route_asn(&runtime, "203.0.113.200", 64_511)
        .await
        .expect("ObserveRIB advertisement should update runtime");
    assert_eq!(observed.as_path, vec![64_510, 64_511]);
    assert_eq!(observed.communities, vec![300]);
    assert_eq!(observed.large_communities.len(), 1);
    assert_eq!(observed.large_communities[0].asn, 64_511);
    assert_eq!(observed.large_communities[0].local_data1, 30);
    assert_eq!(observed.large_communities[0].local_data2, 40);

    observe_tx
        .send(Ok(RibUpdate {
            advertisement: false,
            is_initial_dump: false,
            end_of_rib: false,
            route: Some(observe_route),
        }))
        .await
        .expect("send observe withdrawal");
    wait_for_route_asn(&runtime, "203.0.113.200", 64_501)
        .await
        .expect("ObserveRIB withdrawal should remove more-specific route");

    {
        let requests = dump_requests.lock().expect("lock dump requests");
        assert!(
            requests.iter().any(|request| {
                request.router == "203.0.113.10"
                    && request.vrf_id == 42
                    && request.vrf == "blue"
                    && request.afisafi == AfiSafi::Ipv4Unicast.as_proto()
            }),
            "DumpRIB IPv4 request did not include configured router/VRF: {requests:?}"
        );
        assert!(
            requests
                .iter()
                .any(|request| request.afisafi == AfiSafi::Ipv6Unicast.as_proto()),
            "DumpRIB IPv6 request was not attempted: {requests:?}"
        );
    }
    {
        let requests = observe_requests.lock().expect("lock observe requests");
        assert!(
            requests.iter().any(|request| {
                request.router == "203.0.113.10"
                    && request.vrf_id == 42
                    && request.vrf == "blue"
                    && request.afisafi == AfiSafi::Ipv4Unicast.as_observe_proto()
                    && request.allow_unready_rib
            }),
            "ObserveRIB IPv4 request did not include configured router/VRF: {requests:?}"
        );
    }

    shutdown.cancel();
    listener
        .await
        .expect("join BioRIS listener")
        .expect("BioRIS listener should stop cleanly");
    server_shutdown.cancel();
    server
        .await
        .expect("join BioRIS fixture server")
        .expect("BioRIS fixture server should stop cleanly");
}

async fn wait_for_route_asn(
    runtime: &DynamicRoutingRuntime,
    lookup: &str,
    asn: u32,
) -> Option<crate::enrichment::StaticRoutingEntry> {
    let lookup = lookup.parse().expect("parse lookup address");
    tokio::time::timeout(Duration::from_secs(3), async {
        loop {
            if let Some(route) = runtime.lookup(lookup, None, None)
                && route.asn == asn
            {
                return route;
            }
            tokio::time::sleep(Duration::from_millis(25)).await;
        }
    })
    .await
    .ok()
}

#[derive(Debug)]
struct TestRisService {
    dump_routes: Vec<Route>,
    observe_rx: Arc<Mutex<Option<mpsc::Receiver<std::result::Result<RibUpdate, tonic::Status>>>>>,
    dump_requests: Arc<Mutex<Vec<DumpRibRequest>>>,
    observe_requests: Arc<Mutex<Vec<ObserveRibRequest>>>,
}

impl TestRisService {
    fn new(
        dump_routes: Vec<Route>,
        observe_rx: Option<mpsc::Receiver<std::result::Result<RibUpdate, tonic::Status>>>,
    ) -> Self {
        Self {
            dump_routes,
            observe_rx: Arc::new(Mutex::new(observe_rx)),
            dump_requests: Arc::new(Mutex::new(Vec::new())),
            observe_requests: Arc::new(Mutex::new(Vec::new())),
        }
    }

    fn dump_requests(&self) -> Arc<Mutex<Vec<DumpRibRequest>>> {
        Arc::clone(&self.dump_requests)
    }

    fn observe_requests(&self) -> Arc<Mutex<Vec<ObserveRibRequest>>> {
        Arc::clone(&self.observe_requests)
    }
}

#[tonic::async_trait]
impl proto::bio::ris::routing_information_service_server::RoutingInformationService
    for TestRisService
{
    async fn lpm(
        &self,
        _request: tonic::Request<proto::bio::ris::LpmRequest>,
    ) -> std::result::Result<tonic::Response<proto::bio::ris::LpmResponse>, tonic::Status> {
        Ok(tonic::Response::new(proto::bio::ris::LpmResponse {
            routes: Vec::new(),
        }))
    }

    async fn get(
        &self,
        _request: tonic::Request<proto::bio::ris::GetRequest>,
    ) -> std::result::Result<tonic::Response<proto::bio::ris::GetResponse>, tonic::Status> {
        Ok(tonic::Response::new(proto::bio::ris::GetResponse {
            routes: Vec::new(),
        }))
    }

    async fn get_routers(
        &self,
        _request: tonic::Request<proto::bio::ris::GetRoutersRequest>,
    ) -> std::result::Result<tonic::Response<proto::bio::ris::GetRoutersResponse>, tonic::Status>
    {
        Ok(tonic::Response::new(proto::bio::ris::GetRoutersResponse {
            routers: vec![proto::bio::ris::Router {
                sys_name: "fixture-router".to_string(),
                vrf_ids: vec![10],
                address: "203.0.113.10".to_string(),
            }],
        }))
    }

    async fn get_longer(
        &self,
        _request: tonic::Request<proto::bio::ris::GetLongerRequest>,
    ) -> std::result::Result<tonic::Response<proto::bio::ris::GetLongerResponse>, tonic::Status>
    {
        Ok(tonic::Response::new(proto::bio::ris::GetLongerResponse {
            routes: Vec::new(),
        }))
    }

    type ObserveRIBStream = ReceiverStream<std::result::Result<RibUpdate, tonic::Status>>;

    async fn observe_rib(
        &self,
        request: tonic::Request<proto::bio::ris::ObserveRibRequest>,
    ) -> std::result::Result<tonic::Response<Self::ObserveRIBStream>, tonic::Status> {
        let request = request.into_inner();
        let is_ipv4 = request.afisafi == AfiSafi::Ipv4Unicast.as_observe_proto();
        self.observe_requests
            .lock()
            .expect("lock observe request log")
            .push(request);
        let rx = if is_ipv4 {
            self.observe_rx
                .lock()
                .expect("lock observe receiver")
                .take()
                .unwrap_or_else(empty_observe_receiver)
        } else {
            empty_observe_receiver()
        };
        Ok(tonic::Response::new(ReceiverStream::new(rx)))
    }

    type DumpRIBStream = tokio_stream::Iter<
        std::vec::IntoIter<std::result::Result<proto::bio::ris::DumpRibReply, tonic::Status>>,
    >;

    async fn dump_rib(
        &self,
        request: tonic::Request<proto::bio::ris::DumpRibRequest>,
    ) -> std::result::Result<tonic::Response<Self::DumpRIBStream>, tonic::Status> {
        let request = request.into_inner();
        self.dump_requests
            .lock()
            .expect("lock dump request log")
            .push(request.clone());
        let routes = if request.afisafi == AfiSafi::Ipv4Unicast.as_proto() {
            self.dump_routes
                .iter()
                .cloned()
                .map(|route| Ok(proto::bio::ris::DumpRibReply { route: Some(route) }))
                .collect()
        } else {
            Vec::new()
        };
        Ok(tonic::Response::new(tokio_stream::iter(routes)))
    }
}

fn test_bioris_route() -> Route {
    Route {
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
                path_identifier: 1,
                next_hop: Some(ProtoIp {
                    higher: 0,
                    lower: u64::from(u32::from(Ipv4Addr::new(198, 51, 100, 1))),
                    version: ProtoIpVersion::IPv4 as i32,
                }),
                local_pref: 0,
                as_path: vec![ProtoAsPathSegment {
                    as_sequence: true,
                    asns: vec![64_500, 64_501],
                }],
                origin: 0,
                med: 0,
                ebgp: false,
                bgp_identifier: 0,
                source: None,
                communities: vec![100, 200],
                large_communities: vec![proto::bio::route::LargeCommunity {
                    global_administrator: 64_500,
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
    }
}

fn test_bioris_observe_route() -> Route {
    Route {
        pfx: Some(ProtoPrefix {
            address: Some(ProtoIp {
                higher: 0,
                lower: u64::from(u32::from(Ipv4Addr::new(203, 0, 113, 128))),
                version: ProtoIpVersion::IPv4 as i32,
            }),
            length: 25,
        }),
        paths: vec![proto::bio::route::Path {
            r#type: 0,
            static_path: None,
            bgp_path: Some(BgpPath {
                path_identifier: 2,
                next_hop: Some(ProtoIp {
                    higher: 0,
                    lower: u64::from(u32::from(Ipv4Addr::new(198, 51, 100, 2))),
                    version: ProtoIpVersion::IPv4 as i32,
                }),
                local_pref: 0,
                as_path: vec![ProtoAsPathSegment {
                    as_sequence: true,
                    asns: vec![64_510, 64_511],
                }],
                origin: 0,
                med: 0,
                ebgp: false,
                bgp_identifier: 0,
                source: None,
                communities: vec![300],
                large_communities: vec![proto::bio::route::LargeCommunity {
                    global_administrator: 64_511,
                    data_part1: 30,
                    data_part2: 40,
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
    }
}

fn empty_observe_receiver() -> mpsc::Receiver<std::result::Result<RibUpdate, tonic::Status>> {
    let (_tx, rx) = mpsc::channel(1);
    rx
}
