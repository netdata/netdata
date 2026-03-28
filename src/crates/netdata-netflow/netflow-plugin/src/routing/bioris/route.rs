use super::{runtime::AfiSafi, *};

pub(super) fn route_to_update(
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

pub(super) fn route_withdraw_keys(route: &Route, afisafi: AfiSafi) -> Option<(IpNet, Vec<String>)> {
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

pub(super) fn proto_ip_to_ip_addr(ip: ProtoIp) -> Option<IpAddr> {
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
