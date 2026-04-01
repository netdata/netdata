use super::rd::{parse_configured_rds, parse_rd_text};
use super::routes::{
    flatten_as_path, flatten_as4_path, ipv4_mpls_label_routes, ipv4_mpls_vpn_routes,
    l2_evpn_routes, mp_reach_to_routes, mp_unreach_to_routes, path_id_component,
    route_distinguisher_to_u64,
};
use super::session::{BmpSessionDecision, bmp_session_decision};
use super::*;
use ipnet::{Ipv4Net, Ipv6Net};
use netgauze_bgp_pkt::nlri::{
    EthernetSegmentIdentifier, EthernetTag, Ipv4MplsVpnUnicastAddress, Ipv4NlriMplsLabelsAddress,
    Ipv4Unicast, L2EvpnIpv4PrefixRoute, L2EvpnIpv6PrefixRoute, MplsLabel,
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
    let vpn_a =
        Ipv4MplsVpnUnicastAddress::new(Some(9), rd, vec![MplsLabel::new([0, 0, 0x01])], network);
    let vpn_b =
        Ipv4MplsVpnUnicastAddress::new(Some(9), rd, vec![MplsLabel::new([0, 0, 0x45])], network);
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
