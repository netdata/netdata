use super::*;

pub(super) fn encode_network_journal_fields(
    record: &FlowRecord,
    writer: &mut JournalBufWriter<'_>,
) {
    writer.push_opt_ip("SRC_ADDR", record.src_addr);
    writer.push_opt_ip("DST_ADDR", record.dst_addr);
    writer.push_prefix("SRC_PREFIX", record.src_prefix, record.src_mask);
    writer.push_prefix("DST_PREFIX", record.dst_prefix, record.dst_mask);
    writer.push_u8("SRC_MASK", record.src_mask);
    writer.push_u8("DST_MASK", record.dst_mask);
    writer.push_u32("SRC_AS", record.src_as);
    writer.push_u32("DST_AS", record.dst_as);
    writer.push_str("SRC_AS_NAME", &record.src_as_name);
    writer.push_str("DST_AS_NAME", &record.dst_as_name);
    writer.push_str("SRC_NET_NAME", &record.src_net_name);
    writer.push_str("DST_NET_NAME", &record.dst_net_name);
    writer.push_str("SRC_NET_ROLE", &record.src_net_role);
    writer.push_str("DST_NET_ROLE", &record.dst_net_role);
    writer.push_str("SRC_NET_SITE", &record.src_net_site);
    writer.push_str("DST_NET_SITE", &record.dst_net_site);
    writer.push_str("SRC_NET_REGION", &record.src_net_region);
    writer.push_str("DST_NET_REGION", &record.dst_net_region);
    writer.push_str("SRC_NET_TENANT", &record.src_net_tenant);
    writer.push_str("DST_NET_TENANT", &record.dst_net_tenant);
    writer.push_str("SRC_COUNTRY", &record.src_country);
    writer.push_str("DST_COUNTRY", &record.dst_country);
    writer.push_str("SRC_GEO_CITY", &record.src_geo_city);
    writer.push_str("DST_GEO_CITY", &record.dst_geo_city);
    writer.push_str("SRC_GEO_STATE", &record.src_geo_state);
    writer.push_str("DST_GEO_STATE", &record.dst_geo_state);
    writer.push_str("SRC_GEO_LATITUDE", &record.src_geo_latitude);
    writer.push_str("DST_GEO_LATITUDE", &record.dst_geo_latitude);
    writer.push_str("SRC_GEO_LONGITUDE", &record.src_geo_longitude);
    writer.push_str("DST_GEO_LONGITUDE", &record.dst_geo_longitude);
    writer.push_str("DST_AS_PATH", &record.dst_as_path);
    writer.push_str("DST_COMMUNITIES", &record.dst_communities);
    writer.push_str("DST_LARGE_COMMUNITIES", &record.dst_large_communities);
}
