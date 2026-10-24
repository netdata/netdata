use super::*;

pub(super) fn set_record_network_field(rec: &mut FlowRecord, key: &str, value: &str) -> bool {
    match key {
        "SRC_ADDR" => {
            if let Ok(ip) = value.parse::<IpAddr>() {
                rec.src_addr = Some(ip);
            }
            true
        }
        "DST_ADDR" => {
            if let Ok(ip) = value.parse::<IpAddr>() {
                rec.dst_addr = Some(ip);
            }
            true
        }
        "SRC_PREFIX" => {
            rec.src_prefix = parse_prefix_ip(value);
            true
        }
        "DST_PREFIX" => {
            rec.dst_prefix = parse_prefix_ip(value);
            true
        }
        "SRC_MASK" => {
            rec.src_mask = value.parse().unwrap_or(0);
            true
        }
        "DST_MASK" => {
            rec.dst_mask = value.parse().unwrap_or(0);
            true
        }
        "SRC_AS" => {
            rec.src_as = value.parse().unwrap_or(0);
            true
        }
        "DST_AS" => {
            rec.dst_as = value.parse().unwrap_or(0);
            true
        }
        "SRC_AS_NAME" => {
            rec.src_as_name = value.to_string();
            true
        }
        "DST_AS_NAME" => {
            rec.dst_as_name = value.to_string();
            true
        }
        "SRC_NET_NAME" => {
            rec.src_net_name = value.to_string();
            true
        }
        "DST_NET_NAME" => {
            rec.dst_net_name = value.to_string();
            true
        }
        "SRC_NET_ROLE" => {
            rec.src_net_role = value.to_string();
            true
        }
        "DST_NET_ROLE" => {
            rec.dst_net_role = value.to_string();
            true
        }
        "SRC_NET_SITE" => {
            rec.src_net_site = value.to_string();
            true
        }
        "DST_NET_SITE" => {
            rec.dst_net_site = value.to_string();
            true
        }
        "SRC_NET_REGION" => {
            rec.src_net_region = value.to_string();
            true
        }
        "DST_NET_REGION" => {
            rec.dst_net_region = value.to_string();
            true
        }
        "SRC_NET_TENANT" => {
            rec.src_net_tenant = value.to_string();
            true
        }
        "DST_NET_TENANT" => {
            rec.dst_net_tenant = value.to_string();
            true
        }
        "SRC_COUNTRY" => {
            rec.src_country = value.to_string();
            true
        }
        "DST_COUNTRY" => {
            rec.dst_country = value.to_string();
            true
        }
        "SRC_GEO_CITY" => {
            rec.src_geo_city = value.to_string();
            true
        }
        "DST_GEO_CITY" => {
            rec.dst_geo_city = value.to_string();
            true
        }
        "SRC_GEO_STATE" => {
            rec.src_geo_state = value.to_string();
            true
        }
        "DST_GEO_STATE" => {
            rec.dst_geo_state = value.to_string();
            true
        }
        "SRC_GEO_LATITUDE" => {
            rec.src_geo_latitude = value.to_string();
            true
        }
        "DST_GEO_LATITUDE" => {
            rec.dst_geo_latitude = value.to_string();
            true
        }
        "SRC_GEO_LONGITUDE" => {
            rec.src_geo_longitude = value.to_string();
            true
        }
        "DST_GEO_LONGITUDE" => {
            rec.dst_geo_longitude = value.to_string();
            true
        }
        "DST_AS_PATH" => {
            rec.dst_as_path = value.to_string();
            true
        }
        "DST_COMMUNITIES" => {
            rec.dst_communities = value.to_string();
            true
        }
        "DST_LARGE_COMMUNITIES" => {
            rec.dst_large_communities = value.to_string();
            true
        }
        "NEXT_HOP" => {
            if let Ok(ip) = value.parse::<IpAddr>() {
                rec.next_hop = Some(ip);
            }
            true
        }
        _ => false,
    }
}
