use super::client::bioris_peer_key;
use super::route::{proto_ip_to_ip_addr, route_to_update, route_withdraw_keys};
use super::runtime::AfiSafi;
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
