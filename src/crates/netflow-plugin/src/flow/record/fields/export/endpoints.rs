use super::super::super::*;

pub(super) fn insert_endpoint_fields(record: &FlowRecord, fields: &mut FlowFields) {
    // Endpoints
    fields.insert(
        "SRC_ADDR",
        super::helpers::opt_ip_to_string(record.src_addr),
    );
    fields.insert(
        "DST_ADDR",
        super::helpers::opt_ip_to_string(record.dst_addr),
    );
    fields.insert(
        "SRC_PREFIX",
        super::helpers::format_prefix(record.src_prefix, record.src_mask),
    );
    fields.insert(
        "DST_PREFIX",
        super::helpers::format_prefix(record.dst_prefix, record.dst_mask),
    );
    fields.insert("SRC_MASK", record.src_mask.to_string());
    fields.insert("DST_MASK", record.dst_mask.to_string());
    fields.insert("SRC_AS", record.src_as.to_string());
    fields.insert("DST_AS", record.dst_as.to_string());
    fields.insert("SRC_AS_NAME", record.src_as_name.clone());
    fields.insert("DST_AS_NAME", record.dst_as_name.clone());

    // Network attributes
    fields.insert("SRC_NET_NAME", record.src_net_name.clone());
    fields.insert("DST_NET_NAME", record.dst_net_name.clone());
    fields.insert("SRC_NET_ROLE", record.src_net_role.clone());
    fields.insert("DST_NET_ROLE", record.dst_net_role.clone());
    fields.insert("SRC_NET_SITE", record.src_net_site.clone());
    fields.insert("DST_NET_SITE", record.dst_net_site.clone());
    fields.insert("SRC_NET_REGION", record.src_net_region.clone());
    fields.insert("DST_NET_REGION", record.dst_net_region.clone());
    fields.insert("SRC_NET_TENANT", record.src_net_tenant.clone());
    fields.insert("DST_NET_TENANT", record.dst_net_tenant.clone());
    fields.insert("SRC_COUNTRY", record.src_country.clone());
    fields.insert("DST_COUNTRY", record.dst_country.clone());
    fields.insert("SRC_GEO_CITY", record.src_geo_city.clone());
    fields.insert("DST_GEO_CITY", record.dst_geo_city.clone());
    fields.insert("SRC_GEO_STATE", record.src_geo_state.clone());
    fields.insert("DST_GEO_STATE", record.dst_geo_state.clone());
    fields.insert("SRC_GEO_LATITUDE", record.src_geo_latitude.clone());
    fields.insert("DST_GEO_LATITUDE", record.dst_geo_latitude.clone());
    fields.insert("SRC_GEO_LONGITUDE", record.src_geo_longitude.clone());
    fields.insert("DST_GEO_LONGITUDE", record.dst_geo_longitude.clone());

    // BGP routing
    fields.insert("DST_AS_PATH", record.dst_as_path.clone());
    fields.insert("DST_COMMUNITIES", record.dst_communities.clone());
    fields.insert(
        "DST_LARGE_COMMUNITIES",
        record.dst_large_communities.clone(),
    );

    // Next hop / ports
    fields.insert(
        "NEXT_HOP",
        super::helpers::opt_ip_to_string(record.next_hop),
    );
    fields.insert("SRC_PORT", record.src_port.to_string());
    fields.insert("DST_PORT", record.dst_port.to_string());

    // Timestamps
    fields.insert("FLOW_START_USEC", record.flow_start_usec.to_string());
    fields.insert("FLOW_END_USEC", record.flow_end_usec.to_string());
    fields.insert(
        "OBSERVATION_TIME_MILLIS",
        record.observation_time_millis.to_string(),
    );

    // NAT
    fields.insert(
        "SRC_ADDR_NAT",
        super::helpers::opt_ip_to_string(record.src_addr_nat),
    );
    fields.insert(
        "DST_ADDR_NAT",
        super::helpers::opt_ip_to_string(record.dst_addr_nat),
    );
    fields.insert("SRC_PORT_NAT", record.src_port_nat.to_string());
    fields.insert("DST_PORT_NAT", record.dst_port_nat.to_string());
}
